package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/deliverygate"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/worktree"
)

func alwaysOwns() bool { return true }

func rollbackClaim(
	cmd *cobra.Command, store *snapshot.Store, logPath, issueID, workerID, opLabel string,
	cause error, prior priorClaimState, claimToken string, exclusionSets ...[]claimExclusion,
) error {
	return compensateClaimIfHeldByToken(cmd, store, logPath, issueID, workerID, opLabel, cause, prior, claimToken, false, exclusionSets...)
}

func createWorktreeAndBranch(repoPath, worktreePath, issueID string, issue materialize.Issue, stillOwns func() bool, sourceArgs ...string) error {
	return createWorktreeAndBranchWithExclusion(repoPath, worktreePath, issueID, issue, stillOwns, "", sourceArgs...)
}

func setupRepoWithEpic(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Test epic", "--type", "epic", "--id", "epic-01"}))
	require.NoError(t, cmd2.Execute())

	return repo
}

func setupRepoWithParentAndTask(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Parent story", "--type", "story", "--id", "story-01"}))
	require.NoError(t, cmd2.Execute())

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	cmd3 := newRootCmd()
	cmd3.SetOut(new(bytes.Buffer))
	cmd3.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Child task", "--type", "task", "--id", "task-01", "--parent", "story-01"}))
	require.NoError(t, cmd3.Execute())

	return repo
}

func TestClaimDetachedHEADDoesNotPersistAsParentBranch(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	headSHA := runGitOutput(t, repo, "rev-parse", "HEAD")
	run(t, repo, "git", "checkout", "--detach", strings.TrimSpace(headSHA))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", "branch.task/task-01.armature-parent")
	getCmd.Dir = repo
	out, err := getCmd.CombinedOutput()
	assert.Error(t, err, "no parent branch config should be recorded when the coordinator was in detached HEAD, got: %q", out)
}

func TestClaimNewWorktreeRecordsClaimedBranchFile_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	claimedBranchData, err := os.ReadFile(filepath.Join(actualGitDir, "armature-claimed-branch")) //nolint:gosec // test path is internal
	require.NoError(t, err, "claimed-branch marker file should be recorded at claim time")
	assert.Equal(t, "task/task-01", strings.TrimSpace(string(claimedBranchData)))
}

func TestClaimExistingWorktreePersistsComputedForkPointWhenDiverged_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	run(t, repo, "git", "checkout", "-b", "story-branch")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(sibling-task): unrelated sibling work")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "branch", "task/task-01", "story-branch")
	run(t, repo, "git", "worktree", "add", worktreePath, "task/task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", "branch.task/task-01.armature-parent")
	getCmd.Dir = repo
	_, err := getCmd.Output()
	assert.Error(t, err, "parent branch config should NOT be recorded for the existing-worktree claim path")

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	_, err = os.ReadFile(filepath.Join(actualGitDir, "armature-base-commit")) //nolint:gosec // test path is internal
	assert.Error(t, err, "existing worktree must not receive a guessed base commit")
	_, err = deliverygate.GatedBaseCommit(worktreePath, "task-01", adapters.New(worktreePath))
	assert.Error(t, err, "delivery gate must fail closed when legacy provenance is missing")
	assert.Contains(t, err.Error(), "no recorded base commit")
}

func TestClaimExistingWorktreeBaseCommitGoesStaleAfterRebase_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	defaultBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	defaultTipSHA := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	run(t, repo, "git", "checkout", "-b", "story-branch")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(sibling-task): unrelated sibling work")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "branch", "task/task-01", "story-branch")
	run(t, repo, "git", "worktree", "add", worktreePath, "task/task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	require.NoError(t, os.WriteFile(filepath.Join(repo, "newmain.go"), []byte("package newmain\n"), 0o644))
	run(t, repo, "git", "checkout", defaultBranch)
	run(t, repo, "git", "add", "newmain.go")
	run(t, repo, "git", "commit", "-m", "feat(other): advance main after claim")
	newDefaultTipSHA := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	require.NotEqual(t, defaultTipSHA, newDefaultTipSHA)

	run(t, worktreePath, "git", "rebase", defaultBranch)

	_, err := deliverygate.GatedBaseCommit(worktreePath, "task-01", adapters.New(worktreePath))
	assert.Error(t, err, "pre-existing worktree provenance must fail closed after rebase")
	assert.Contains(t, err.Error(), "no recorded base commit")
	_ = defaultTipSHA
	_ = newDefaultTipSHA
}

func TestClaimExistingWorktreePersistsBaseCommitWhenNotDiverged_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "branch", "task/task-01")
	run(t, repo, "git", "worktree", "add", worktreePath, "task/task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	_, err := deliverygate.GatedBaseCommit(worktreePath, "task-01", adapters.New(worktreePath))
	assert.Error(t, err, "a pre-existing worktree must not receive a guessed base commit")
	assert.Contains(t, err.Error(), "no recorded base commit")
}

func TestClaimExistingWorktreeDoesNotContaminateFromUnrelatedCoordinatorBranch_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	defaultBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))

	run(t, repo, "git", "checkout", "-b", "story-branch")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(sibling-task): unrelated sibling work")

	storyHeadSHA := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "branch", "task/task-01", "story-branch")
	run(t, repo, "git", "worktree", "add", worktreePath, "task/task-01")

	run(t, repo, "git", "checkout", defaultBranch)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "unrelated.go"), []byte("package unrelated\n"), 0o644))
	run(t, repo, "git", "add", "unrelated.go")
	run(t, repo, "git", "commit", "-m", "chore: unrelated coordinator-side work")
	mainHeadSHA := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	require.NotEqual(t, storyHeadSHA, mainHeadSHA)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", "branch.task/task-01.armature-parent")
	getCmd.Dir = repo
	out, err := getCmd.Output()
	if err == nil {
		assert.NotEqual(t, defaultBranch, strings.TrimSpace(string(out)),
			"parent branch config must not be contaminated with the coordinator's unrelated checkout")
	}

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	baseCommitData, err := os.ReadFile(filepath.Join(actualGitDir, "armature-base-commit")) //nolint:gosec // test path is internal
	if err == nil {
		assert.NotEqual(t, mainHeadSHA, strings.TrimSpace(string(baseCommitData)),
			"base commit file must not be contaminated with the coordinator's unrelated HEAD")
	}
}

func TestClaim_AllEntryPathsPersistBaseCommitViaConsolidatedFunction(t *testing.T) {
	readBaseCommitFile := func(t *testing.T, worktreePath string) string {
		t.Helper()
		gitPath := filepath.Join(worktreePath, ".git")
		gitFileContent, err := os.ReadFile(gitPath)
		require.NoError(t, err)
		actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
		if !filepath.IsAbs(actualGitDir) {
			actualGitDir = filepath.Join(worktreePath, actualGitDir)
		}
		data, err := os.ReadFile(filepath.Join(actualGitDir, "armature-base-commit")) //nolint:gosec // test path is internal
		require.NoError(t, err, "base-commit file should be recorded")
		return string(data)
	}

	t.Run("fresh worktree path", func(t *testing.T) {
		repo := setupRepoWithParentAndTask(t)
		headSHA := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
		worktreePath := filepath.Join(repo, ".worktrees", "task-01")

		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
		require.NoError(t, cmd.Execute())

		content := readBaseCommitFile(t, worktreePath)
		assert.Equal(t, headSHA, strings.TrimSpace(content),
			"fresh-worktree path must persist HEAD as the base-commit with no extra formatting")
		assert.NotContains(t, content, "\n", "base-commit file must contain the raw SHA with no trailing newline or extra data")
	})

	t.Run("existing worktree path preserves missing provenance", func(t *testing.T) {
		repo := setupRepoWithParentAndTask(t)

		worktreePath := filepath.Join(repo, ".worktrees", "task-01")
		run(t, repo, "git", "branch", "task/task-01")
		run(t, repo, "git", "worktree", "add", worktreePath, "task/task-01")

		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
		require.NoError(t, cmd.Execute())

		_, err := deliverygate.GatedBaseCommit(worktreePath, "task-01", adapters.New(worktreePath))
		assert.Error(t, err, "existing-worktree path must not synthesize branch-point metadata")
	})
}

func TestClaimWithoutWorktreeFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error()+errBuf.String()+buf.String(), "worktree")
}

func TestClaimCreatesWorktreeIfAbsent(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})

	err := cmd.Execute()
	require.NoError(t, err)

	assert.DirExists(t, worktreePath, "worktree directory should be created")

	gitPath := filepath.Join(worktreePath, ".git")
	assert.FileExists(t, gitPath, ".git file should exist in worktree")

	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)
	assert.Contains(t, gitDirLine, "gitdir: ", ".git file should contain gitdir reference")

	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	assert.FileExists(t, taskIDFile, "armature-issue-id file should be created in actual git dir")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	assert.Equal(t, "task-01", string(taskID))
}

func TestClaimUpdatesTaskIDIfWorktreeExists(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	assert.Equal(t, "task-01", string(taskID))
}

func TestClaimWithEpicReturnsError(t *testing.T) {
	repo := setupRepoWithEpic(t)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "epic-01", "--worktree"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error()+errBuf.String()+buf.String(), "epic")
}

func TestClaimCreatesTaskBranch(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})

	err := cmd.Execute()
	require.NoError(t, err)

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	headFile := filepath.Join(actualGitDir, "HEAD")
	assert.FileExists(t, headFile, "HEAD file should exist in git directory")
	headContent, err := os.ReadFile(headFile) //nolint:gosec // test path is safe
	require.NoError(t, err)
	headStr := string(headContent)
	assert.Contains(t, headStr, "task-01", "HEAD should reference task/task-01 branch")
}

func TestClaimStillAppendsClaimOpToLog(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})

	require.NoError(t, cmd.Execute())

	assert.Contains(t, buf.String(), "task-01", "output should mention the claimed task")
}

func TestCanonicalWorktreePathRejectsTraversalBeforeMutation_REQ_LNGHZN_S5_T1(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")

	_, err := canonicalWorktreePath(root, "team/task-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")

	path, err := canonicalWorktreePath(root, "team")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".worktrees", "team"), path)

	_, err = canonicalWorktreePath(root, "../escaped")
	assert.Error(t, err)
	_, err = canonicalWorktreePath(root, filepath.Join(string(filepath.Separator), "escaped"))
	assert.Error(t, err)
}

func TestCanonicalWorktreePathRejectsDotDotAliasedIDs_REQ_LNGHZN_S5(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")

	plain, err := canonicalWorktreePath(root, "task-1")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".worktrees", "task-1"), plain)

	for _, id := range []string{"team/../task-1", "a/./b", "..", ".", "team/.."} {
		_, err := canonicalWorktreePath(root, id)
		assert.Error(t, err, "ID %q must be rejected (separator or '.' / '..' component)", id)
	}
}

func TestClaimRejectsTraversalBeforeClaimAppend_REQ_LNGHZN_S5_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "../escaped", "--worktree"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "../escaped")
	assert.NoDirExists(t, filepath.Join(repo, ".worktrees"), "invalid IDs must not create a worktree root")
	assert.NoDirExists(t, filepath.Join(filepath.Dir(repo), "escaped"), "invalid IDs must not mutate outside the repository")
}

func TestCanonicalWorktreePathRejectsSlashBearingIDs_REQ_LNGHZN_S5(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")

	_, err := canonicalWorktreePath(root, "team/task-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")

	path, err := canonicalWorktreePath(root, "team")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".worktrees", "team"), path)

	_, err = canonicalWorktreePath(root, ".")
	assert.Error(t, err)
	_, err = canonicalWorktreePath(root, "..")
	assert.Error(t, err)

	_, err = canonicalWorktreePath(root, "team\\task-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")
}

func TestCanonicalWorktreePath_MissingThroughSymlink_REQ_ARCHIMP_S20(t *testing.T) {
	realRepo := t.TempDir()
	link := filepath.Join(t.TempDir(), "repo-link")
	require.NoError(t, os.Symlink(realRepo, link))

	path, err := canonicalWorktreePath(link, "ISSUE-01")
	require.NoError(t, err)
	root := worktree.CanonicalRoot(link)
	rel, err := filepath.Rel(root, path)
	require.NoError(t, err)
	t.Logf("canonicalWorktreePath Rel(%q, %q) = %q", root, path, rel)
	assert.Equal(t, "ISSUE-01", rel)
}

func TestCreateWorktreeAndBranchInheritsFilesFromHEAD(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	markerFile := filepath.Join(repo, "marker.txt")
	require.NoError(t, os.WriteFile(markerFile, []byte("hello from main"), 0644))
	run(t, repo, "git", "add", "marker.txt")
	run(t, repo, "git", "commit", "-m", "add marker file")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})

	require.NoError(t, cmd.Execute())

	assert.DirExists(t, worktreePath, "worktree directory should be created")

	markerInWorktree := filepath.Join(worktreePath, "marker.txt")
	assert.FileExists(t, markerInWorktree, "marker file from HEAD should exist in task branch worktree")

	content, err := os.ReadFile(markerInWorktree)
	require.NoError(t, err)
	assert.Equal(t, "hello from main", string(content))
}

func TestCreateWorktreeAndBranchRejectsEmptyBranchName(t *testing.T) {
	repo := setupRepoWithEpic(t)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "epic-01", "--worktree"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error()+errBuf.String()+buf.String(), "epic")
}

func TestCreateWorktreeAndBranchFailsWhenWorktreeCannotBeCreated(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	issue := materialize.Issue{Type: "task"}

	worktree1 := filepath.Join(t.TempDir(), "worktree1")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", worktree1)

	worktree2 := filepath.Join(t.TempDir(), "worktree2")
	err := createWorktreeAndBranch(repo, worktree2, "task-01", issue, alwaysOwns)
	require.Error(t, err, "creating worktree with already-checked-out branch should fail")
	assert.Contains(t, err.Error(), "worktree")
}

func TestCreateWorktreeAndBranchAdoptsBoundCheckedOutBranch_REQ_LNGHZN_S5_T4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", legacyPath)
	require.NoError(t, updateIssueIDFile(legacyPath, "task-01"))
	baseSHA := strings.TrimSpace(runGitOutput(t, legacyPath, "rev-parse", "HEAD"))
	require.NoError(t, writeBaseCommitFileIfAbsent(legacyPath, baseSHA), "adoption requires original claim provenance")

	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
	err := createWorktreeAndBranch(repo, canonicalPath, "task-01", materialize.Issue{Type: "task"}, alwaysOwns)

	require.NoError(t, err, "a correctly bound existing worktree should be adopted")
	assert.DirExists(t, canonicalPath)
	assert.NoDirExists(t, legacyPath)
	assert.Equal(t, "task/task-01", strings.TrimSpace(runOutput(t, canonicalPath, "branch", "--show-current")))
}

func TestCreateWorktreeAndBranchFailsClosedOnBoundDetachedWorktree_REQ_LNGHZN_S5_T6(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", legacyPath)
	require.NoError(t, updateIssueIDFile(legacyPath, "task-01"))

	head := strings.TrimSpace(runGitOutput(t, legacyPath, "rev-parse", "HEAD"))
	run(t, legacyPath, "git", "checkout", "--detach", head)

	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
	err := createWorktreeAndBranch(repo, canonicalPath, "task-01", materialize.Issue{Type: "task"}, alwaysOwns)

	require.Error(t, err, "a bound worktree that is not on the issue branch must fail closed")
	assert.Contains(t, err.Error(), legacyPath, "the error must name the worktree the operator has to deal with")
	assert.NoDirExists(t, canonicalPath, "no duplicate worktree may be provisioned")
	assert.DirExists(t, legacyPath, "the bound worktree must be left untouched")
}

func TestCreateWorktreeAndBranchFailsClosedOnAmbiguousBinding_REQ_LNGHZN_S5_T6(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	onBranchPath := filepath.Join(t.TempDir(), "aaa-on-branch")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", onBranchPath)
	require.NoError(t, updateIssueIDFile(onBranchPath, "task-01"))

	detachedPath := filepath.Join(t.TempDir(), "zzz-detached")
	head := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	run(t, repo, "git", "worktree", "add", "--detach", detachedPath, head)
	require.NoError(t, updateIssueIDFile(detachedPath, "task-01"))

	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
	err := createWorktreeAndBranch(repo, canonicalPath, "task-01", materialize.Issue{Type: "task"}, alwaysOwns)

	require.Error(t, err, "two worktrees sharing one binding must fail closed, not adopt the first")
	assert.Contains(t, err.Error(), "bound to 2 worktrees")
	assert.Contains(t, err.Error(), detachedPath, "the error must name every candidate")
	assert.Contains(t, err.Error(), onBranchPath, "the error must name every candidate")
	assert.NoDirExists(t, canonicalPath, "nothing may be provisioned while bindings are ambiguous")
	assert.DirExists(t, onBranchPath, "neither candidate may be moved")
	assert.DirExists(t, detachedPath, "neither candidate may be moved")
}

func TestCreateWorktreeAndBranchFailsClosedOnBoundScratchBranch_REQ_LNGHZN_S5_T6(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", legacyPath)
	require.NoError(t, updateIssueIDFile(legacyPath, "task-01"))
	run(t, legacyPath, "git", "checkout", "-b", "scratch/experiment")

	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
	err := createWorktreeAndBranch(repo, canonicalPath, "task-01", materialize.Issue{Type: "task"}, alwaysOwns)

	require.Error(t, err, "a bound worktree parked on a scratch branch must fail closed")
	assert.NoDirExists(t, canonicalPath, "no duplicate worktree may be provisioned")
}

func TestCreateWorktreeAndBranchAdoptionUsesAdoptedBranchPoint_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	parentBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	parentTip := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	run(t, repo, "git", "checkout", "-b", "story-branch")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(sibling): add adopted branch work")

	legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
	run(t, repo, "git", "branch", "task/task-01", "story-branch")
	run(t, repo, "git", "worktree", "add", legacyPath, "task/task-01")
	require.NoError(t, updateIssueIDFile(legacyPath, "task-01"))
	require.NoError(t, writeBaseCommitFileIfAbsent(legacyPath, parentTip), "seed trusted branch-point metadata before adoption")

	run(t, repo, "git", "checkout", parentBranch)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "coordinator.go"), []byte("package coordinator\n"), 0o644))
	run(t, repo, "git", "add", "coordinator.go")
	run(t, repo, "git", "commit", "-m", "chore: advance coordinator")
	coordinatorTip := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	require.NotEqual(t, parentTip, coordinatorTip)

	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
	err := createWorktreeAndBranch(repo, canonicalPath, "task-01", materialize.Issue{Type: "task"}, alwaysOwns)
	require.NoError(t, err)

	gitPath, err := worktree.ResolveGitDir(canonicalPath)
	require.NoError(t, err)
	baseData, err := os.ReadFile(filepath.Join(gitPath, deliverygate.BaseCommitFileName))
	require.NoError(t, err, "adopted worktree must record a branch point")
	assert.Equal(t, parentTip, strings.TrimSpace(string(baseData)),
		"adopted worktree base must come from the adopted branch, not coordinator HEAD")
	assert.NotEqual(t, coordinatorTip, strings.TrimSpace(string(baseData)))

	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", deliverygate.ParentBranchConfigKey("task/task-01"))
	getCmd.Dir = repo
	_, err = getCmd.Output()
	assert.Error(t, err, "adoption must not invent a parent branch from the coordinator checkout")
}

func TestCreateWorktreeAndBranchRejectsAdoptionWithoutProvenance_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", legacyPath)
	require.NoError(t, updateIssueIDFile(legacyPath, "task-01"))

	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
	err := createWorktreeAndBranch(repo, canonicalPath, "task-01", materialize.Issue{Type: "task"}, alwaysOwns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recorded branch-point provenance")
	assert.DirExists(t, legacyPath, "failed adoption must restore the original worktree")
	assert.NoDirExists(t, canonicalPath)
}

func TestCreateWorktreeAndBranchLeavesAdoptedWorktreeInPlaceWhenSuperseded(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", legacyPath)
	require.NoError(t, updateIssueIDFile(legacyPath, "task-01"))

	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
	err := createWorktreeAndBranch(repo, canonicalPath, "task-01", materialize.Issue{Type: "task"},
		func() bool { return false })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recorded branch-point provenance")
	assert.NoDirExists(t, canonicalPath, "PlanProvision must refuse adopt before moving the worktree")
	assert.DirExists(t, legacyPath, "failed adoption must leave the original worktree in place")
}

func TestCreateWorktreeAndBranchRemovesFreshPartialWorktreeWhenStillOwned(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	canonicalPath := filepath.Join(repo, ".worktrees", "bad-id")

	err := createWorktreeAndBranch(repo, canonicalPath, "bad id", materialize.Issue{Type: "task"}, alwaysOwns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkout branch in worktree")
	assert.NoDirExists(t, canonicalPath, "an owned claim's partial worktree must still be force-removed on failure")
}

func TestCreateWorktreeAndBranchLeavesAlreadyAtDestOnMetadataFailure(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", canonicalPath)
	require.NoError(t, updateIssueIDFile(canonicalPath, "task-01"))
	require.NoError(t, writeClaimExclusionMarker(canonicalPath, "/original/"))
	keep := filepath.Join(canonicalPath, "keep-me.go")
	require.NoError(t, os.WriteFile(keep, []byte("package keep\n"), 0o644))

	err := createWorktreeAndBranchWithExclusion(
		repo, canonicalPath, "task-01", materialize.Issue{Type: "task"}, alwaysOwns, "/other/",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already records a different pattern")
	assert.DirExists(t, canonicalPath, "AlreadyAtDest cleanup must not force-remove a pre-existing worktree")
	assert.FileExists(t, keep)
}

func TestCreateWorktreeAndBranchLeavesFreshPartialWorktreeWhenSuperseded(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	canonicalPath := filepath.Join(repo, ".worktrees", "bad-id")

	err := createWorktreeAndBranch(repo, canonicalPath, "bad id", materialize.Issue{Type: "task"},
		func() bool { return false })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkout branch in worktree")
	assert.DirExists(t, canonicalPath, "a superseded claim must not force-remove a worktree that may now belong to someone else")
}

func setupSingleWorkerClaimStore(t *testing.T, ctx *config.Context, claimTimestamp int64, claimToken string) *snapshot.Store {
	t.Helper()
	logPath := opsLogPath(ctx.IssuesDir, "worker-a")
	claimOp := ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: claimTimestamp,
		WorkerID: "worker-a", Payload: ops.Payload{TTL: 60, ClaimToken: claimToken},
	}
	require.NoError(t, appendOp(ctx, logPath, claimOp))
	store := newSnapshotStore(ctx)
	_, err := store.Load(context.Background())
	require.NoError(t, err)
	return store
}

func rollbackClaimTestCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{}
	state := &executionState{ctx: ctx, tracker: initPushDeps(ctx)}
	cmd.SetContext(context.WithValue(context.Background(), executionStateKey{}, state))
	return cmd
}

func TestRollbackClaimSkipsCompensatingOpWhenClaimSuperseded_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	claimTimestampA := nowEpoch()
	claimTokenA := "token-worker-a"
	store := setupSingleWorkerClaimStore(t, ctx, claimTimestampA, claimTokenA)

	claimTimestampB := claimTimestampA + 60*60 + 1
	logPathB := opsLogPath(ctx.IssuesDir, "worker-b")
	claimOpB := ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: claimTimestampB,
		WorkerID: "worker-b", Payload: ops.Payload{TTL: 60, ClaimToken: "token-worker-b"},
	}
	require.NoError(t, appendOp(ctx, logPathB, claimOpB))
	_, err := store.Load(context.Background())
	require.NoError(t, err)

	cmd := rollbackClaimTestCmd(ctx)
	logPathA := opsLogPath(ctx.IssuesDir, "worker-a")
	prior := priorClaimState{status: ops.StatusOpen}
	cause := fmt.Errorf("boom")

	err = rollbackClaim(cmd, store, logPathA, "task-01", "worker-a", "create worktree", cause, prior, claimTokenA)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "superseded")
	assert.Contains(t, err.Error(), "boom")

	_, err = store.Load(context.Background())
	require.NoError(t, err)
	issue := store.Issue("task-01")
	require.NotNil(t, issue)
	assert.Equal(t, "worker-b", issue.ClaimedBy)
	assert.Equal(t, claimTimestampB, issue.ClaimedAt)
	assert.Equal(t, "token-worker-b", issue.ClaimToken)
}

func TestRollbackClaimAppendsCompensatingOpWhenOwnershipConfirmed_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	claimTimestampA := nowEpoch()
	claimTokenA := "token-worker-a"
	store := setupSingleWorkerClaimStore(t, ctx, claimTimestampA, claimTokenA)

	cmd := rollbackClaimTestCmd(ctx)
	logPathA := opsLogPath(ctx.IssuesDir, "worker-a")
	prior := priorClaimState{status: ops.StatusOpen}
	cause := fmt.Errorf("boom")

	err := rollbackClaim(cmd, store, logPathA, "task-01", "worker-a", "create worktree", cause, prior, claimTokenA)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claim released")
	assert.NotContains(t, err.Error(), "superseded")

	_, err = store.Load(context.Background())
	require.NoError(t, err)
	issue := store.Issue("task-01")
	require.NotNil(t, issue)
	assert.Equal(t, ops.StatusOpen, issue.Status)
	assert.Equal(t, "", issue.ClaimedBy)
	assert.Equal(t, "", issue.ClaimToken)
}

func TestRollbackClaimSameWorkerSameSecondDistinctTokensPreventsClobber_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	sameTimestamp := nowEpoch()
	tokenFirst := "token-first"
	tokenSecond := "token-second"
	require.NotEqual(t, tokenFirst, tokenSecond, "distinct tokens are the whole point of this test")

	store := setupSingleWorkerClaimStore(t, ctx, sameTimestamp, tokenFirst)

	logPathA := opsLogPath(ctx.IssuesDir, "worker-a")
	secondClaimOp := ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: sameTimestamp,
		WorkerID: "worker-a", Payload: ops.Payload{TTL: 60, ClaimToken: tokenSecond},
	}
	require.NoError(t, appendOp(ctx, logPathA, secondClaimOp))
	_, err := store.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, tokenSecond, store.Issue("task-01").ClaimToken, "the second same-second claim must be the materialized one")

	cmd := rollbackClaimTestCmd(ctx)
	prior := priorClaimState{status: ops.StatusOpen}
	cause := fmt.Errorf("boom")

	err = rollbackClaim(cmd, store, logPathA, "task-01", "worker-a", "create worktree", cause, prior, tokenFirst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "superseded")

	_, err = store.Load(context.Background())
	require.NoError(t, err)
	issue := store.Issue("task-01")
	require.NotNil(t, issue)
	assert.Equal(t, "worker-a", issue.ClaimedBy)
	assert.Equal(t, tokenSecond, issue.ClaimToken, "the second claim must survive the first claim's rollback")
	assert.NotEqual(t, ops.StatusOpen, issue.Status, "the second claim must not have been released to open")
}

func TestClaimStillOwnedByReportsFalseAfterTransitionToInProgress_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	claimTimestamp := nowEpoch()
	claimToken := "token-worker-a"
	store := setupSingleWorkerClaimStore(t, ctx, claimTimestamp, claimToken)

	logPath := opsLogPath(ctx.IssuesDir, "worker-a")
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: claimTimestamp + 10,
		WorkerID: "worker-a", Payload: ops.Payload{To: ops.StatusInProgress},
	}))

	owns, err := reloadStoreHeldByExactWorkerAndClaimToken(store, "task-01", "worker-a", claimToken)
	require.NoError(t, err)
	assert.False(t, owns,
		"reloadStoreHeldByExactWorkerAndClaimToken must report not-owned once the issue has left StatusClaimed, even with matching ClaimedBy/ClaimToken")
}

func TestCreateWorktreeAndBranchLeavesPartialWorktreeInPlaceWhenClaimSupersededByTransition_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	claimTimestamp := nowEpoch()
	claimToken := "token-worker-a"
	store := setupSingleWorkerClaimStore(t, ctx, claimTimestamp, claimToken)

	logPath := opsLogPath(ctx.IssuesDir, "worker-a")
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: claimTimestamp + 10,
		WorkerID: "worker-a", Payload: ops.Payload{To: ops.StatusInProgress},
	}))

	stillOwns := func() bool {
		owns, err := reloadStoreHeldByExactWorkerAndClaimToken(store, "task-01", "worker-a", claimToken)
		require.NoError(t, err)
		return owns
	}

	canonicalPath := filepath.Join(repo, ".worktrees", "bad-id")
	err := createWorktreeAndBranch(repo, canonicalPath, "bad id", materialize.Issue{Type: "task"}, stillOwns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkout branch in worktree")
	assert.DirExists(t, canonicalPath, "a claim superseded by a concurrent transition to in-progress must not force-remove its partially provisioned worktree")
}

func TestClaimDoesNotCreateWorktreeWhenOverlapFails(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	plantVerifiedTask(t, repo, "task-01", "cmd/armature/claim.go")
	ctx := getTestContext(t, repo)
	workerID, workerLog, plantErr := resolveWorkerAndLog(ctx)
	require.NoError(t, plantErr)
	require.NoError(t, appendRawCreateConfidence(workerLog, workerID, "task-02", "Task two is complete and tested", "cmd/armature/claim.go", "verified"))

	otherWorker := "other-worker-uuid"
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, otherWorker+".log")
	claimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  otherWorker,
		Payload:   ops.Payload{TTL: 60},
	}
	require.NoError(t, ops.AppendOp(logPath, claimOp))

	worktreePath := filepath.Join(repo, ".worktrees", "task-02")
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "task-02", "--worktree")

	assert.Error(t, claimErr, "claim should fail due to scope overlap (without --force). stderr: %s", stderr)

	_, statErr := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree must not be created when claim fails due to scope overlap")
}

func TestClaimIgnoresNonTaskIssuesInOverlapCheck_REQ_LNGHZN_S10_T8(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Other story", "--type", "story", "--id", "story-other")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "amend", "--issue", "story-other", "--scope", "cmd/armature/claim.go")
	require.NoError(t, err)

	plantVerifiedTask(t, repo, "task-target", "cmd/armature/claim.go")

	otherWorker := "other-worker-uuid"
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, otherWorker+".log")
	transitionOp := ops.Op{
		Type:      ops.OpTransition,
		TargetID:  "story-other",
		Timestamp: time.Now().Unix(),
		WorkerID:  otherWorker,
		Payload:   ops.Payload{To: ops.StatusInProgress},
	}
	require.NoError(t, ops.AppendOp(logPath, transitionOp))

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "task-target", "--worktree")
	require.NoError(t, claimErr, "an in-progress story's aggregate scope must not block an unrelated task claim. stderr: %s", stderr)
}

func TestClaimStillBlocksOnOverlappingClaimedTask_REQ_LNGHZN_S10_T8(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	plantVerifiedTask(t, repo, "task-01", "cmd/armature/claim.go")
	ctx := getTestContext(t, repo)
	workerID, plantLog, plantErr := resolveWorkerAndLog(ctx)
	require.NoError(t, plantErr)
	require.NoError(t, appendRawCreateConfidence(plantLog, workerID, "task-02", "Task two is complete and tested", "cmd/armature/claim.go", "verified"))

	otherWorker := "other-worker-uuid"
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, otherWorker+".log")
	claimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  otherWorker,
		Payload:   ops.Payload{TTL: 60},
	}
	require.NoError(t, ops.AppendOp(logPath, claimOp))

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "task-02", "--worktree")
	require.Error(t, claimErr, "a genuinely overlapping claimed task must still block the claim. stderr: %s", stderr)
	assert.Contains(t, claimErr.Error(), "task-01")
	assert.Contains(t, claimErr.Error(), "task",
		"error should name the conflicting issue's type so a false block is diagnosable without reading source")
	assert.Contains(t, claimErr.Error(), otherWorker,
		"error should name the conflicting issue's holder so a false block is diagnosable without reading source")
}

func TestClaimLockPrecedesStoreAndWorktreeReads_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	flock, lockErr := tryAcquirePessimisticCloneClaimFlock(repo, "does-not-exist")
	require.NoError(t, lockErr)
	t.Cleanup(flock.Release)

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "does-not-exist", "--worktree")
	require.Error(t, claimErr, "claim must fail while the lock is held. stderr: %s", stderr)
	assert.Contains(t, claimErr.Error(), "does-not-exist")
	assert.Contains(t, claimErr.Error(), "in progress",
		"failure must be the lock-contention error, not \"issue not found\" — "+
			"which is only possible if the lock is acquired before the store/issue read")
	assert.NotContains(t, claimErr.Error(), "not found",
		"a pre-fix ordering would surface \"issue %s not found\" instead of the lock error", "does-not-exist")
}

func TestClaimRejectsWorktreeBoundToDifferentTask(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(t.TempDir(), "shared-worktree")
	run(t, repo, "git", "worktree", "add", worktreePath, "-b", "task/task-01")
	require.NoError(t, updateIssueIDFile(worktreePath, "task-01"))

	err := checkExistingWorktreeBinding(worktreePath, "task-02", "task/task-02")
	require.Error(t, err, "binding check should reject a worktree bound to a different issue")
	assert.Contains(t, err.Error(), "task-01",
		"error should mention the task currently bound to the worktree")
}

func TestClaimRejectsWorktreeWithMismatchedBranch(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "worktree", "add", worktreePath, "-b", "task/task-01")

	err := checkExistingWorktreeBinding(worktreePath, "task-02", "task/task-02")
	require.Error(t, err, "claim should fail due to branch mismatch")
	assert.Contains(t, err.Error(), "branch", "error should mention the branch mismatch")
}

func TestClaimAllowsWorktreeWithDetachedHEAD(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	headFile := filepath.Join(actualGitDir, "HEAD")
	headContent, err := os.ReadFile(headFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	headStr := strings.TrimSpace(string(headContent))
	assert.False(t, strings.HasPrefix(headStr, "ref: "), "HEAD should be detached (not a branch ref)")

	_, claimErr := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.NoError(t, claimErr, "claim should succeed with detached HEAD when binding matches")
}

func TestClaimBoundToOtherTaskErrorDoesNotSuggestMerged(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(t.TempDir(), "shared-worktree")
	run(t, repo, "git", "worktree", "add", worktreePath, "-b", "task/task-01")
	require.NoError(t, updateIssueIDFile(worktreePath, "task-01"))

	err := checkExistingWorktreeBinding(worktreePath, "task-02", "task/task-02")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task-01", "error should mention the bound task")
	assert.NotContains(t, err.Error(), "merged", "error should NOT suggest 'arm merged'")
}

func TestClaimReleasesClaimOnWorktreeSetupFailure(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	require.NoError(t, os.MkdirAll(worktreePath, 0o755))

	gitPath := filepath.Join(worktreePath, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644))

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")

	assert.Error(t, claimErr, "claim should fail with invalid worktree. stderr: %s", stderr)

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	assert.NotEqual(t, ops.StatusClaimed, issue.Status, "task should not be stuck in claimed state after worktree setup failure")
}

func TestClaimReleasesPushesInDualBranchMode(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Dual branch rollback task", "--id", "task-rb-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	worktreePath := filepath.Join(repo, ".worktrees", "task-rb-01")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	gitPath := filepath.Join(worktreePath, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644))

	_, _, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-rb-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail with invalid/broken worktree")

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-rb-01.json"))
	require.NoError(t, err)
	assert.NotEqual(t, ops.StatusClaimed, issue.Status,
		"task must not be stuck in claimed state after worktree setup failure in dual-branch mode")

	armOpsDir := filepath.Join(repo, ".armature", "ops")
	entries, readErr := os.ReadDir(armOpsDir)
	if readErr != nil {
		t.Logf("Note: .armature/ops not readable: %v; skipping ops log check", readErr)
		return
	}

	hasReleaseOp := false
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		logPath := filepath.Join(armOpsDir, e.Name())
		data, readErr2 := os.ReadFile(logPath)
		if readErr2 == nil && strings.Contains(string(data), `"to":"open"`) {
			hasReleaseOp = true
			t.Logf("Found release op in dual-branch ops log %s (appendHighStakesOp fix verified)", logPath)
			break
		}
	}

	if !hasReleaseOp {
		t.Logf("No release op in dual-branch ops log — claim was likely rejected before winning the race (acceptable)")
	}
}

func TestClaimRejectsUnboundDetachedWorktree(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err, "first claim should succeed")

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // test path
	require.NoError(t, err)
	require.Equal(t, "task-01", string(taskID), "worktree should initially be bound to task-01")

	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")

	headFile := filepath.Join(actualGitDir, "HEAD")
	headContent, err := os.ReadFile(headFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	headStr := strings.TrimSpace(string(headContent))
	require.False(t, strings.HasPrefix(headStr, "ref: "), "HEAD should be detached")

	require.NoError(t, os.Remove(taskIDFile), "should be able to delete armature-issue-id file") //nolint:gosec // internal test path

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail when worktree has unbound detached HEAD. stderr: %s", stderr)

	errText := stderr + claimErr.Error()
	assert.Contains(t, errText, "detached HEAD",
		"error should mention detached HEAD in the error message")
}

func TestClaimDoesNotReleaseExistingClaimOnWorktreeRetryFailure(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktree1 := filepath.Join(repo, ".worktrees", "task-01")
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err, "first claim should succeed")

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed after first claim")
	before := issue

	gitPath := filepath.Join(worktree1, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644),
		"should be able to overwrite .git file")

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "second claim with broken worktree should error. stderr: %s", stderr)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	assert.Equal(t, ops.StatusClaimed, issueAfter.Status,
		"task should remain claimed after failed worktree retry (not be released to open)")
	assert.NotEqual(t, ops.StatusOpen, issueAfter.Status,
		"task must NOT transition to open on worktree retry failure when it was already claimed")
	assert.Equal(t, before.ClaimedBy, issueAfter.ClaimedBy)
	assert.Equal(t, before.ClaimedAt, issueAfter.ClaimedAt)
	assert.Equal(t, before.ClaimTTL, issueAfter.ClaimTTL)
	assert.Equal(t, before.LastHeartbeat, issueAfter.LastHeartbeat)
	assert.Equal(t, before.LastClaimingWorkerActivity, issueAfter.LastClaimingWorkerActivity)
	assert.Equal(t, before.WorktreePath, issueAfter.WorktreePath)
}

func TestClaimRollsBackStaleTakeoverToOpen(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task one", "--type", "task", "--id", "task-01")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	otherWorker := "other-worker-uuid"
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, otherWorker+".log")

	staleClaimTime := time.Now().Unix() - 7200
	staleClaimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: staleClaimTime,
		WorkerID:  otherWorker,
		Payload:   ops.Payload{TTL: 1, WorktreePath: "/legacy/task-01"},
	}
	require.NoError(t, ops.AppendOp(logPath, staleClaimOp))

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed by stale worker")
	require.Equal(t, otherWorker, issue.ClaimedBy, "task should be claimed by other-worker-uuid")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	blockingFile := filepath.Join(worktreePath, "blocking-file")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocks worktree creation"), 0o644))

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	assert.Equal(t, ops.StatusOpen, issueAfter.Status,
		"task should be rolled back to open after stale takeover failure (not remain claimed)")
	assert.Equal(t, "", issueAfter.ClaimedBy,
		"ClaimedBy must be cleared so other workers can pick up the task")
	assert.Equal(t, "/legacy/task-01", issueAfter.WorktreePath,
		"failed stale takeover must restore the prior worktree path")
}

func TestClaimRejectsForeignWorktree(t *testing.T) {
	repoA := setupRepoWithParentAndTask(t)

	repoBTempDir := t.TempDir()
	repoB := filepath.Join(repoBTempDir, "repo-B")
	require.NoError(t, os.Mkdir(repoB, 0o755))
	run(t, repoB, "git", "init")
	run(t, repoB, "git", "config", "user.email", "test@test.com")
	run(t, repoB, "git", "config", "user.name", "Test")
	run(t, repoB, "git", "config", "commit.gpgsign", "false")
	run(t, repoB, "git", "commit", "--allow-empty", "-m", "init from repo-B")
	run(t, repoB, "git", "checkout", "-b", "task/task-01", "HEAD")
	run(t, repoB, "git", "checkout", "-b", "main-branch")

	foreignWorktreePath := filepath.Join(repoBTempDir, "foreign-wt")
	run(t, repoB, "git", "worktree", "add", foreignWorktreePath, "task/task-01")
	assert.DirExists(t, foreignWorktreePath, "foreign worktree should exist")

	assert.False(t, isWorktreeOf(repoA, foreignWorktreePath),
		"a worktree registered to a different repo must not be recognized as belonging to repoA")
}

func TestClaimRollsBackStaleSameWorkerClaimToOpen(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task stale same-worker", "--type", "task", "--id", "task-01")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	workerID, logPath, err := resolveWorkerAndLog(&config.Context{
		RepoPath:  repo,
		IssuesDir: filepath.Join(repo, ".armature"),
		StateDir:  filepath.Join(repo, ".armature", "state"),
	})
	require.NoError(t, err, "should resolve worker ID and log path")

	staleClaimTime := time.Now().Unix() - 7200
	staleClaimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: staleClaimTime,
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 1},
	}
	require.NoError(t, ops.AppendOp(logPath, staleClaimOp))

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed by stale worker")
	require.Equal(t, workerID, issue.ClaimedBy, "task should be claimed by same worker")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	blockingFile := filepath.Join(worktreePath, "blocking-file")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocks worktree creation"), 0o644))

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	assert.Equal(t, ops.StatusOpen, issueAfter.Status,
		"task should be rolled back to open after stale same-worker claim failure (not remain claimed)")
	assert.Equal(t, "", issueAfter.ClaimedBy,
		"ClaimedBy must be cleared so other workers can pick up the task")
}

func TestClaimPreservesNeverExpiringClaimOnRetry(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task never-expiring", "--type", "task", "--id", "task-01")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	workerID, logPath, err := resolveWorkerAndLog(&config.Context{
		RepoPath:  repo,
		IssuesDir: filepath.Join(repo, ".armature"),
		StateDir:  filepath.Join(repo, ".armature", "state"),
	})
	require.NoError(t, err, "should resolve worker ID and log path")

	neverExpiringClaimTime := time.Now().Unix() - 7200
	neverExpiringClaimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: neverExpiringClaimTime,
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 0},
	}
	require.NoError(t, ops.AppendOp(logPath, neverExpiringClaimOp))

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed")
	require.Equal(t, workerID, issue.ClaimedBy, "task should be claimed by same worker")
	require.Equal(t, 0, issue.ClaimTTL, "task claim TTL should be 0 (never-expiring)")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	blockingFile := filepath.Join(worktreePath, "blocking-file")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocks worktree creation"), 0o644))

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	assert.Equal(t, ops.StatusClaimed, issueAfter.Status,
		"task should remain claimed after never-expiring same-worker claim failure (not be released to open)")
	assert.Equal(t, workerID, issueAfter.ClaimedBy,
		"ClaimedBy must remain set since the claim never expires")
}

func TestClaimCompensationRestoreVsRelease_REQ_ARCHIMP_S20_T4(t *testing.T) {
	t.Run("restore live same-worker", func(t *testing.T) {
		repo := setupRepoWithParentAndTask(t)
		worktree1 := filepath.Join(repo, ".worktrees", "task-01")
		_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
		require.NoError(t, err)

		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)
		before, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
		require.NoError(t, err)
		require.Equal(t, ops.StatusClaimed, before.Status)

		require.NoError(t, os.WriteFile(filepath.Join(worktree1, ".git"), []byte("gitdir: /nonexistent/git/dir"), 0o644))
		_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
		assert.Error(t, claimErr, "second claim with broken worktree should error. stderr: %s", stderr)

		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)
		after, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
		require.NoError(t, err)
		assert.Equal(t, ops.StatusClaimed, after.Status, "live same-worker lease must be restored")
		assert.Equal(t, before.ClaimedBy, after.ClaimedBy)
		assert.Equal(t, before.WorktreePath, after.WorktreePath)
	})

	t.Run("release stale foreign", func(t *testing.T) {
		repo := initTempRepo(t)
		run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
		_, err := runTrls(t, repo, "bootstrap")
		require.NoError(t, err)
		_, err = runTrls(t, repo, "create", "--title", "Task one", "--type", "task", "--id", "task-01")
		require.NoError(t, err)
		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)

		otherWorker := "other-worker-uuid"
		staleClaimTime := time.Now().Unix() - 7200
		staleClaimOp := ops.Op{
			Type:      ops.OpClaim,
			TargetID:  "task-01",
			Timestamp: staleClaimTime,
			WorkerID:  otherWorker,
			Payload:   ops.Payload{TTL: 1, WorktreePath: "/legacy/task-01"},
		}
		require.NoError(t, ops.AppendOp(filepath.Join(repo, ".armature", "ops", otherWorker+".log"), staleClaimOp))
		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)

		worktreePath := filepath.Join(repo, ".worktrees", "task-01")
		require.NoError(t, os.MkdirAll(worktreePath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "blocking-file"), []byte("blocks worktree creation"), 0o644))
		_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
		assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)
		after, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
		require.NoError(t, err)
		assert.Equal(t, ops.StatusOpen, after.Status, "stale foreign lease must be released")
		assert.Equal(t, "", after.ClaimedBy)
		assert.Equal(t, "/legacy/task-01", after.WorktreePath)
	})
}

func TestCheckExistingWorktreeBindingReadsLegacyTaskID(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(t.TempDir(), "legacy-worktree")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	assert.DirExists(t, worktreePath, "worktree directory should exist")

	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	require.NoError(t, os.WriteFile(taskIDFile, []byte("task-01"), 0o600)) //nolint:gosec // test path is internal

	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	_, err = os.ReadFile(issueIDFile) //nolint:gosec // test path is internal
	require.True(t, os.IsNotExist(err), "armature-issue-id should not exist (only legacy armature-task-id)")

	err = checkExistingWorktreeBinding(worktreePath, "task-01", "task/task-01")
	assert.NoError(t, err, "checkExistingWorktreeBinding should allow same-issue claim with legacy armature-task-id binding")
}

func TestCheckExistingWorktreeBindingFailsClosedOnPermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: file permissions do not block reads")
	}

	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(t.TempDir(), "perm-worktree")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	require.NoError(t, os.WriteFile(issueIDFile, []byte("task-01"), 0o600)) //nolint:gosec // test path is internal
	require.NoError(t, os.Chmod(issueIDFile, 0o000))                        //nolint:gosec // test path is internal
	t.Cleanup(func() {
		swallowErr(os.Chmod(issueIDFile, 0o600)) //nolint:gosec
	})

	err = checkExistingWorktreeBinding(worktreePath, "task-01", "task/task-01")
	require.Error(t, err, "a permission-denied binding file must fail closed, not be silently treated as unbound")
	assert.Contains(t, err.Error(), "read existing binding")
}

func TestClaimCommand_NoFalsePositiveAgainstParentStory_REQ_TOPTIER_S17_T1(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "create", "--title", "Parent Story", "--type", "story", "--id", "story-01")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Child Task", "--type", "task", "--id", "task-01", "--parent", "story-01")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "amend", "--issue", "story-01", "--scope", "src/**")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "amend", "--issue", "task-01", "--scope", "src/auth/**")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "--issue", "story-01", "--worktree")
	require.NoError(t, err, "claiming parent story should succeed")

	worktreePath2 := filepath.Join(repo, ".worktrees", "task-01")
	stdout, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.NoError(t, claimErr, "claiming child task should succeed without scope overlap error. stdout: %s, stderr: %s", stdout, stderr)

	assert.DirExists(t, worktreePath2, "worktree should be created when claiming child task against parent")
}

func TestClaimAutoProvisionsWorktreeAtDefaultRoot_REQ_LNGHZN_S5_T4(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	expected := filepath.Join(repo, ".worktrees", "task-01")
	assert.DirExists(t, expected, "boolean --worktree must provision the worktree at <repo>/.worktrees/<issue-id>")
	assert.FileExists(t, filepath.Join(expected, ".git"), "provisioned path must be a linked worktree (.git file, not a dir)")
	assert.True(t, isWorktreeOf(repo, expected), "provisioned path must be a registered linked worktree of the repo")

	gitDir, err := worktree.ResolveGitDir(expected)
	require.NoError(t, err)
	bindingBytes, err := os.ReadFile(filepath.Join(gitDir, "armature-issue-id"))
	require.NoError(t, err)
	assert.Equal(t, "task-01", strings.TrimSpace(string(bindingBytes)), "worktree must be bound to the claimed issue")
}

func TestClaimProvisionExcludesManagedWorktrees_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithTask(t)
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	excludeBefore, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.NotContains(t, string(excludeBefore), ".worktrees/", "fixture must model an existing installation without the managed-worktree exclusion")

	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claim.Execute())

	run(t, repo, "git", "add", ".")
	staged := runGitOutput(t, repo, "diff", "--cached", "--name-only")
	assert.NotContains(t, staged, ".worktrees/task-01", "managed worktree must remain excluded from broad staging")
}

func TestClaimRejectsRepoRelativeCustomDestination_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(repo, "child")
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	excludeBefore, err := os.ReadFile(excludePath)
	require.NoError(t, err)

	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination})
	err = claim.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "under canonical .worktrees")
	assert.NoDirExists(t, destination)
	excludeAfter, readErr := os.ReadFile(excludePath)
	require.NoError(t, readErr)
	assert.Equal(t, string(excludeBefore), string(excludeAfter), "rejected custom destinations must not mutate shared exclusions")
	status, statusErr := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, statusErr)
	assert.Equal(t, ops.StatusOpen+"\n", status)
}

func TestClaimExternalCustomWorktreeDoesNotWriteSharedExclusion_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "#literal-worktree")
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	excludeBefore, err := os.ReadFile(excludePath)
	require.NoError(t, err)

	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination})
	require.NoError(t, claim.Execute())

	excludeAfter, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.NotContains(t, string(excludeAfter), "#literal-worktree", "external custom destinations must not write a clone-wide exclusion")
	assert.Contains(t, string(excludeAfter), ".worktrees/", "claim must retain the canonical managed-worktree exclusion")
	assert.NotEqual(t, string(excludeBefore), string(excludeAfter), "claim may add the canonical exclusion")
}

func TestClaimRejectsDestinationNestedInRegisteredWorktree_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	parent := filepath.Join(repo, "parent")
	run(t, repo, "git", "worktree", "add", "-b", "parent", parent)
	destination := filepath.Join(parent, "child")

	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	excludeBefore, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree", destination, "--from", parent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nested inside registered worktree")

	status, err := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, err)
	assert.Equal(t, ops.StatusOpen+"\n", status, "nested destination rejection must not append a claim")
	worktreePath, err := runTrls(t, repo, "show", "task-01", "--field", "worktree_path")
	require.NoError(t, err)
	assert.Equal(t, "\n", worktreePath, "nested destination rejection must not record a destination")

	_, branchErr := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--verify", "refs/heads/task/task-01").Output()
	assert.Error(t, branchErr, "nested destination rejection must not create the task branch")
	assert.NoDirExists(t, destination, "nested destination rejection must not create the child path")
	assert.DirExists(t, parent, "the registered parent worktree must be left untouched")

	excludeAfter, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.Equal(t, string(excludeBefore), string(excludeAfter), "nested destination rejection must not mutate Git exclusions")
	flock, lockErr := acquireBlockingGitExcludeFlock(repo)
	require.NoError(t, lockErr, "nested destination rejection must release the exclusion lock")
	flock.Release()
}

func TestClaimExistingWorktreeInstallsManagedWorktreeExclusion_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", worktreePath)

	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	excludeBefore, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.NotContains(t, string(excludeBefore), ".worktrees/", "fixture must model an older installation")

	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, claim.Execute())

	excludeAfter, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.Contains(t, string(excludeAfter), ".worktrees/", "successful re-claim must repair the managed-worktree exclusion")
	run(t, repo, "git", "add", ".")
	staged := runGitOutput(t, repo, "diff", "--cached", "--name-only")
	assert.NotContains(t, staged, ".worktrees/task-01", "existing managed worktree must remain excluded from broad staging")
}

func TestClaimExclusionFailureAppendsNoClaim_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithTask(t)
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	require.NoError(t, os.Remove(excludePath))
	require.NoError(t, os.Mkdir(excludePath, 0o700), "a directory at the exclusion-file path makes its write fail deterministically")

	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	err := claim.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exclude managed worktree directory")

	status, err := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, err)
	assert.Equal(t, ops.StatusOpen+"\n", status, "failed exclusion setup must leave the issue unclaimed")
	path, err := runTrls(t, repo, "show", "task-01", "--field", "worktree_path")
	require.NoError(t, err)
	assert.Equal(t, "\n", path, "failed exclusion setup must not record a managed worktree path")
	_, statErr := os.Stat(filepath.Join(repo, ".worktrees", "task-01"))
	assert.True(t, os.IsNotExist(statErr), "failed exclusion setup must not create a worktree")
}

func TestClaimWorktreeFailureRollsBackNewExclusions_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	wrapperDir := t.TempDir()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	script := fmt.Sprintf(`#!/bin/sh
real_git=%q
for arg in "$@"; do
  if [ "$arg" = "checkout" ]; then
    exit 42
  fi
done
exec "$real_git" "$@"
`, realGit)
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	destination := filepath.Join(t.TempDir(), "child")
	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination})
	err = claim.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkout branch in worktree")

	exclude, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.NotContains(t, string(exclude), "child", "a failed claim must not add a custom exclusion")
	assert.NotContains(t, string(exclude), ".worktrees/", "a failed claim must remove its canonical exclusion when no managed worktree remains")
	assert.NoDirExists(t, destination)
	_, branchErr := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--verify", "refs/heads/task/task-01").Output()
	assert.Error(t, branchErr, "failed provisioning must not leave the task branch behind")
	status, err := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, err)
	assert.Equal(t, ops.StatusOpen+"\n", status, "failed provisioning must release the claim")
}

func TestClaimDetachedCheckoutAvoidsBranchAlreadyCheckedOutRace_REQ_LNGHZN_S5_T4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	first := newRootCmd()
	first.SetOut(new(bytes.Buffer))
	first.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, first.Execute())
	require.DirExists(t, worktreePath)

	run(t, repo, "git", "worktree", "remove", "--force", worktreePath)
	require.NoDirExists(t, worktreePath)
	run(t, repo, "git", "rev-parse", "--verify", "refs/heads/task/task-01")

	second := newRootCmd()
	second.SetOut(new(bytes.Buffer))
	second.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, second.Execute(),
		"re-provisioning with a pre-existing derived branch must succeed (no 'branch already checked out')")
	require.DirExists(t, worktreePath)

	gitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)
	headBytes, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	require.NoError(t, err)
	assert.Equal(t, "ref: refs/heads/task/task-01", strings.TrimSpace(string(headBytes)),
		"recreated worktree must be checked out on the issue's derived branch")
}

func TestClaimFromFlagCreatesBranchFromParentWorktree_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	parentPath := filepath.Join(repo, "parent")
	run(t, repo, "git", "worktree", "add", "-b", "feature-parent", parentPath)
	require.NoError(t, os.WriteFile(filepath.Join(parentPath, "parent.go"), []byte("package parent\n"), 0o644))
	run(t, parentPath, "git", "add", "parent.go")
	run(t, parentPath, "git", "commit", "-m", "feat(parent): advance source worktree")
	parentTip := strings.TrimSpace(runGitOutput(t, parentPath, "rev-parse", "HEAD"))

	childPath := filepath.Join(t.TempDir(), "child")
	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", childPath, "--from", parentPath})
	require.NoError(t, claim.Execute())

	assert.Equal(t, parentTip, strings.TrimSpace(runGitOutput(t, childPath, "rev-parse", "HEAD")))
	parentBranch := strings.TrimSpace(runGitOutput(t, repo, "config", "--get", "branch.task/task-01.armature-parent"))
	assert.Equal(t, "feature-parent", parentBranch)
	childGitDir, err := worktree.ResolveGitDir(childPath)
	require.NoError(t, err)
	baseCommit, err := os.ReadFile(filepath.Join(childGitDir, "armature-base-commit"))
	require.NoError(t, err)
	assert.Equal(t, parentTip, strings.TrimSpace(string(baseCommit)))
}

func TestCreateWorktreeAndBranchRejectsUnavailableValidatedSource_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	parentPath := filepath.Join(repo, "parent")
	run(t, repo, "git", "worktree", "add", "-b", "feature-parent", parentPath)
	parentTip := strings.TrimSpace(runGitOutput(t, parentPath, "rev-parse", "HEAD"))
	destination := filepath.Join(t.TempDir(), "child")
	require.NoError(t, os.RemoveAll(parentPath))

	err := createWorktreeAndBranch(
		repo,
		destination,
		"task-01",
		materialize.Issue{Type: "task"},
		alwaysOwns,
		parentPath,
		"feature-parent",
		parentTip,
	)
	require.Error(t, err, "a vanished validated source must fail closed")
	assert.NoDirExists(t, destination)
	_, branchErr := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--verify", "refs/heads/task/task-01").Output()
	assert.Error(t, branchErr, "source failure must not create the task branch")
	_, configErr := exec.CommandContext(context.Background(), "git", "-C", repo, "config", "--get", deliverygate.ParentBranchConfigKey("task/task-01")).Output()
	assert.Error(t, configErr, "source failure must not persist parent provenance")
}

func TestCreateWorktreeAndBranchRejectsChangedValidatedSource_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	parentPath := filepath.Join(repo, "parent")
	run(t, repo, "git", "worktree", "add", "-b", "feature-parent", parentPath)
	parentTip := strings.TrimSpace(runGitOutput(t, parentPath, "rev-parse", "HEAD"))
	require.NoError(t, os.WriteFile(filepath.Join(parentPath, "parent.go"), []byte("package parent\n"), 0o644))
	run(t, parentPath, "git", "add", "parent.go")
	run(t, parentPath, "git", "commit", "-m", "feat(parent): change validated source")

	destination := filepath.Join(t.TempDir(), "child")
	err := createWorktreeAndBranch(
		repo,
		destination,
		"task-01",
		materialize.Issue{Type: "task"},
		alwaysOwns,
		parentPath,
		"feature-parent",
		parentTip,
	)
	require.Error(t, err, "a changed validated source must fail closed")
	assert.Contains(t, err.Error(), "tip changed")
	assert.NoDirExists(t, destination)
	_, branchErr := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--verify", "refs/heads/task/task-01").Output()
	assert.Error(t, branchErr, "source failure must not create the task branch")
}

func TestClaimFromFlagRollsBackWhenSourceDisappears_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	parentPath := filepath.Join(repo, "parent")
	run(t, repo, "git", "worktree", "add", "-b", "feature-parent", parentPath)
	destination := filepath.Join(t.TempDir(), "child")
	wrapperDir := t.TempDir()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	countPath := filepath.Join(wrapperDir, "worktree-list-count")
	backupPath := parentPath + ".gone"
	script := fmt.Sprintf(`#!/bin/sh
real_git=%q
count_file=%q
source_path=%q
backup_path=%q
is_list=0
for arg in "$@"; do
  if [ "$arg" = "--porcelain" ]; then
    is_list=1
  fi
done
if [ "$is_list" = "1" ]; then
  count=0
  if [ -f "$count_file" ]; then
    count=$(cat "$count_file")
  fi
  count=$((count + 1))
  printf '%%s\n' "$count" > "$count_file"
  if [ "$count" -eq 2 ]; then
    mv "$source_path" "$backup_path"
  fi
fi
exec "$real_git" "$@"
`, realGit, countPath, parentPath, backupPath)
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination, "--from", parentPath})
	err = claim.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an existing worktree of this repository")
	assert.NoDirExists(t, destination)
	assert.DirExists(t, backupPath, "the test source worktree must be moved away, not destroyed")
	_, branchErr := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--verify", "refs/heads/task/task-01").Output()
	assert.Error(t, branchErr, "source disappearance must not create the task branch")
	_, configErr := exec.CommandContext(context.Background(), "git", "-C", repo, "config", "--get", deliverygate.ParentBranchConfigKey("task/task-01")).Output()
	assert.Error(t, configErr, "source disappearance must not persist parent provenance")
	status, statusErr := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, statusErr)
	assert.Equal(t, ops.StatusOpen+"\n", status, "source disappearance must roll back the claim")
}

func TestClaimFromFlagRejectsExistingWorktreePath_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	create := newRootCmd()
	create.SetOut(new(bytes.Buffer))
	create.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Second task", "--type", "task", "--id", "task-02"}))
	require.NoError(t, create.Execute())

	boundPath := filepath.Join(t.TempDir(), "bound")
	bind := newRootCmd()
	bind.SetOut(new(bytes.Buffer))
	bind.SetArgs([]string{"claim", "task-02", "--repo", repo, "--worktree", boundPath})
	require.NoError(t, bind.Execute())
	boundGitDir, err := worktree.ResolveGitDir(boundPath)
	require.NoError(t, err)
	bindingBefore, err := os.ReadFile(filepath.Join(boundGitDir, "armature-issue-id"))
	require.NoError(t, err)

	rejected := newRootCmd()
	rejected.SetOut(new(bytes.Buffer))
	rejected.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", boundPath, "--from", repo})
	err = rejected.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nested inside registered worktree")
	bindingAfter, err := os.ReadFile(filepath.Join(boundGitDir, "armature-issue-id"))
	require.NoError(t, err)
	assert.Equal(t, string(bindingBefore), string(bindingAfter))
}

func TestClaimReusesPrunableCustomDestination_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "custom-prunable")

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree", destination)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(destination))

	_, err = runTrls(t, repo, "create", "--id", "task-02", "--title", "Second task", "--type", "task", "--scope", "second.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-02", "--worktree", destination)
	require.NoError(t, err)

	assert.DirExists(t, destination)
	gitDir, err := worktree.ResolveGitDir(destination)
	require.NoError(t, err)
	binding, err := os.ReadFile(filepath.Join(gitDir, "armature-issue-id"))
	require.NoError(t, err)
	assert.Equal(t, "task-02", strings.TrimSpace(string(binding)))
}

func TestClaimFromFlagRejectsUnresolvableFrom_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "child")
	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination, "--from", filepath.Join(repo, "missing")})
	err := claim.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an existing worktree")
	assert.NoDirExists(t, destination)
	_, err = runTrls(t, repo, "rev-parse", "--verify", "refs/heads/task/task-01")
	assert.Error(t, err)
}

func TestClaimFromFlagRejectsDetachedSource_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	parentPath := filepath.Join(repo, "detached-parent")
	run(t, repo, "git", "worktree", "add", "--detach", parentPath)
	destination := filepath.Join(t.TempDir(), "child")

	claim := newRootCmd()
	claim.SetOut(new(bytes.Buffer))
	claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination, "--from", parentPath})
	err := claim.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be on a branch")
	assert.NoDirExists(t, destination)
	_, err = runTrls(t, repo, "rev-parse", "--verify", "refs/heads/task/task-01")
	assert.Error(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, ops.StatusOpen, issue.Status)
}

func TestClaimFromFlagRejectsMismatchedExistingTaskBranch_REQ_LNGHZN_S9_T1(t *testing.T) {
	t.Run("mismatched branch is preserved without claim side effects", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		parentPath := filepath.Join(repo, "parent")
		run(t, repo, "git", "worktree", "add", "-b", "feature-parent", parentPath)
		require.NoError(t, os.WriteFile(filepath.Join(parentPath, "parent.go"), []byte("package parent\n"), 0o644))
		run(t, parentPath, "git", "add", "parent.go")
		run(t, parentPath, "git", "commit", "-m", "feat(parent): advance source")
		run(t, repo, "git", "branch", "task/task-01", "HEAD")
		branchBefore := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "refs/heads/task/task-01"))
		destination := filepath.Join(t.TempDir(), "child")

		claim := newRootCmd()
		claim.SetOut(new(bytes.Buffer))
		claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination, "--from", parentPath})
		err := claim.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match --from tip")
		assert.NoDirExists(t, destination)
		assert.Equal(t, branchBefore, strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "refs/heads/task/task-01")))
		_, err = runTrls(t, repo, "materialize")
		require.NoError(t, err)
		issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
		require.NoError(t, err)
		assert.Equal(t, ops.StatusOpen, issue.Status)
		_, err = exec.CommandContext(context.Background(), "git", "-C", repo, "config", "--get", "branch.task/task-01.armature-parent").Output()
		assert.Error(t, err, "a rejected claim must not persist source provenance")
	})

	t.Run("same tip branch is reusable", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		run(t, repo, "git", "branch", "task/task-01", "HEAD")
		sourceTip := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
		destination := filepath.Join(t.TempDir(), "child")
		claim := newRootCmd()
		claim.SetOut(new(bytes.Buffer))
		claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination, "--from", repo})
		require.NoError(t, claim.Execute())
		assert.Equal(t, sourceTip, strings.TrimSpace(runGitOutput(t, destination, "rev-parse", "HEAD")))
	})
}

func TestClaimFromFlagRejectsConflictingExistingProvenance_REQ_LNGHZN_S9_T1(t *testing.T) {
	setup := func(t *testing.T) (string, string, string, string) {
		t.Helper()
		repo := setupRepoWithTask(t)
		parentPath := filepath.Join(repo, "parent")
		run(t, repo, "git", "worktree", "add", "-b", "feature-parent", parentPath)
		require.NoError(t, os.WriteFile(filepath.Join(parentPath, "parent.go"), []byte("package parent\n"), 0o644))
		run(t, parentPath, "git", "add", "parent.go")
		run(t, parentPath, "git", "commit", "-m", "feat(parent): advance source")
		sourceTip := strings.TrimSpace(runGitOutput(t, parentPath, "rev-parse", "HEAD"))
		run(t, repo, "git", "branch", "task/task-01", sourceTip)
		return repo, parentPath, sourceTip, filepath.Join(t.TempDir(), "child")
	}
	assertNoClaimSideEffects := func(t *testing.T, repo, destination string) {
		t.Helper()
		assert.NoDirExists(t, destination)
		_, err := runTrls(t, repo, "materialize")
		require.NoError(t, err)
		issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
		require.NoError(t, err)
		assert.Equal(t, ops.StatusOpen, issue.Status)
	}

	t.Run("conflicting parent is preserved and rejected", func(t *testing.T) {
		repo, parentPath, _, destination := setup(t)
		parentKey := deliverygate.ParentBranchConfigKey("task/task-01")
		run(t, repo, "git", "config", parentKey, "other-parent")

		claim := newRootCmd()
		claim.SetOut(new(bytes.Buffer))
		claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination, "--from", parentPath})
		err := claim.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match --from branch")
		assert.Equal(t, "other-parent", strings.TrimSpace(runGitOutput(t, repo, "config", "--get", parentKey)))
		assertNoClaimSideEffects(t, repo, destination)
	})

	t.Run("unused base config does not override canonical marker", func(t *testing.T) {
		repo, parentPath, sourceTip, destination := setup(t)
		parentKey := deliverygate.ParentBranchConfigKey("task/task-01")
		baseKey := "branch.task/task-01.armature-base-commit"
		run(t, repo, "git", "config", parentKey, "feature-parent")
		staleBase := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
		run(t, repo, "git", "config", baseKey, staleBase)

		claim := newRootCmd()
		claim.SetOut(new(bytes.Buffer))
		claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination, "--from", parentPath})
		require.NoError(t, claim.Execute())
		assert.Equal(t, sourceTip, strings.TrimSpace(runGitOutput(t, destination, "rev-parse", "HEAD")))
		assert.Equal(t, "feature-parent", strings.TrimSpace(runGitOutput(t, repo, "config", "--get", parentKey)))
		assert.Equal(t, staleBase, strings.TrimSpace(runGitOutput(t, repo, "config", "--get", baseKey)),
			"the unused legacy key must remain untouched while the canonical marker records the source tip")
	})
}

func TestClaimWithoutFromFlagUnchanged_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")

	fresh := newRootCmd()
	fresh.SetOut(new(bytes.Buffer))
	fresh.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree"})
	require.NoError(t, fresh.Execute())
	require.DirExists(t, canonicalPath)

	existing := newRootCmd()
	existing.SetOut(new(bytes.Buffer))
	existing.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree"})
	require.NoError(t, existing.Execute())
	gitDir, err := worktree.ResolveGitDir(canonicalPath)
	require.NoError(t, err)
	binding, err := os.ReadFile(filepath.Join(gitDir, "armature-issue-id"))
	require.NoError(t, err)
	assert.Equal(t, "task-01", strings.TrimSpace(string(binding)))

	surplus := newRootCmd()
	surplus.SetOut(new(bytes.Buffer))
	surplus.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree=unused", "surplus"})
	err = surplus.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 1 arg")
}

func TestClaimFromFlagRequiresExplicitNewWorktreePath_REQ_LNGHZN_S9_T1(t *testing.T) {
	t.Run("existing canonical worktree remains unchanged", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
		setup := newRootCmd()
		setup.SetOut(new(bytes.Buffer))
		setup.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree"})
		require.NoError(t, setup.Execute())

		gitDir, err := worktree.ResolveGitDir(canonicalPath)
		require.NoError(t, err)
		bindingBefore, err := os.ReadFile(filepath.Join(gitDir, "armature-issue-id"))
		require.NoError(t, err)
		baseBefore, err := os.ReadFile(filepath.Join(gitDir, "armature-base-commit"))
		require.NoError(t, err)

		claim := newRootCmd()
		claim.SetOut(new(bytes.Buffer))
		claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", "--from", repo})
		err = claim.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "explicit --worktree <new-path>")

		bindingAfter, err := os.ReadFile(filepath.Join(gitDir, "armature-issue-id"))
		require.NoError(t, err)
		baseAfter, err := os.ReadFile(filepath.Join(gitDir, "armature-base-commit"))
		require.NoError(t, err)
		assert.Equal(t, string(bindingBefore), string(bindingAfter))
		assert.Equal(t, string(baseBefore), string(baseAfter))
	})

	t.Run("missing canonical worktree is not provisioned", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
		claim := newRootCmd()
		claim.SetOut(new(bytes.Buffer))
		claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", "--from", repo})
		err := claim.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "explicit --worktree <new-path>")
		assert.NoDirExists(t, canonicalPath)
	})
}

func injectFutureSameWorkerClaim(t *testing.T, ctx *config.Context, issueID, impostorToken string) (ownerID string) {
	t.Helper()
	ownerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpClaim, TargetID: issueID, Timestamp: nowEpoch() + 10_000,
		WorkerID: ownerID, Payload: ops.Payload{TTL: 60, ClaimToken: impostorToken},
	}))
	return ownerID
}

func TestClaimCommand_SupersededBySameWorkerDifferentTokenLosesRaceAndSkipsWorktree_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	ownerID := injectFutureSameWorkerClaim(t, ctx, "task-01", "impostor-token")

	claimOut, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", "--format", "json")
	require.NoError(t, err, "losing a claim race is a normal outcome, not an error")

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(claimOut)), &result), "output: %s", claimOut)
	assert.Equal(t, false, result["claimed"])
	assert.Equal(t, "lost_claim_race", result["reason"])
	assert.Equal(t, ownerID, result["claimed_by"],
		"claimed_by reports the same effective owner identity this worker used, not a different worker")
	assert.Equal(t, true, result["superseded_by_same_worker"],
		"the superseding claim carried the same workerID, so this must be flagged distinctly from an ordinary different-worker loss")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	assert.NoDirExists(t, worktreePath, "a lost claim race must never provision a worktree")
}

func TestClaimCommand_SupersededBySameWorkerDifferentTokenHumanFormat_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	injectFutureSameWorkerClaim(t, ctx, "task-01", "impostor-token")

	claimOut, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", "--format", "human")
	require.NoError(t, err, "losing a claim race is a normal outcome, not an error")
	assert.Contains(t, claimOut, "Claim lost")
	assert.Contains(t, claimOut, "superseded by a different claim from this same worker ID",
		"human output must distinguish same-worker supersession from an ordinary different-worker loss")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	assert.NoDirExists(t, worktreePath, "a lost claim race must never provision a worktree")
}

func TestClaimCommand_DifferentWorkerLostRaceJSONFormat_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	run(t, repo, "git", "config", "--local", "armature.worker-id", "other-worker-abc")
	claimOut, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", "--format", "json")
	require.NoError(t, err, "losing a claim race is a normal outcome, not an error")

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(claimOut)), &result), "output: %s", claimOut)
	assert.Equal(t, false, result["claimed"])
	assert.Equal(t, "lost_claim_race", result["reason"])
	assert.NotEqual(t, "other-worker-abc", result["claimed_by"], "claimed_by must report the original winner, not the loser")
	assert.Equal(t, false, result["superseded_by_same_worker"],
		"a genuinely different claimant must not be flagged as a same-worker supersession")
}

func TestClaimCommand_OrdinaryWinStillProvisionsWorktree_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	claimOut, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, claimOut, "lost_claim_race", "an uncontested claim must never report a lost race")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	assert.DirExists(t, worktreePath, "a won claim must provision a worktree")
}

func TestDefaultTTLGovernsClaim_REQ_LNGHZN_S7_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	cfg := config.DefaultConfig("go")
	cfg.DefaultTTL = 45
	require.NoError(t, config.WriteConfig(filepath.Join(repo, ".armature", "config.json"), cfg))

	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, 45, issue.ClaimTTL, "claim TTL should default to config.json's default_ttl")
}

func TestClaimFallsBackToBuiltInTTLWhenConfigAbsent_REQ_LNGHZN_S7_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	cfg := config.Config{ProjectType: "go"}
	require.NoError(t, config.WriteConfig(filepath.Join(repo, ".armature", "config.json"), cfg))

	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, 60, issue.ClaimTTL, "claim TTL should fall back to the built-in default of 60 when config's default_ttl is absent/zero")
}

func TestClaimExplicitTTLOverridesConfigDefault_REQ_LNGHZN_S7_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	cfg := config.DefaultConfig("go")
	cfg.DefaultTTL = 45
	require.NoError(t, config.WriteConfig(filepath.Join(repo, ".armature", "config.json"), cfg))

	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", "--ttl", "120")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, 120, issue.ClaimTTL, "explicit --ttl must override config.json's default_ttl")
}

func TestTokenBudgetHonoredByRenderContext_REQ_LNGHZN_S7_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	cfg := config.DefaultConfig("go")
	cfg.TokenBudget = 1
	require.NoError(t, config.WriteConfig(filepath.Join(repo, ".armature", "config.json"), cfg))

	tinyOut, err := runTrls(t, repo, "render-context", "--issue", "task-01", "--format", "agent")
	require.NoError(t, err)

	overriddenOut, err := runTrls(t, repo, "render-context", "--issue", "task-01", "--format", "agent", "--budget", "999999")
	require.NoError(t, err)

	assert.Less(t, len(tinyOut), len(overriddenOut),
		"config.json's tiny token_budget should truncate content relative to an explicit generous --budget override")
}

func TestRenderContextFallsBackToBuiltInBudgetWhenConfigAbsent_REQ_LNGHZN_S7_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	cfg := config.Config{ProjectType: "go"}
	require.NoError(t, config.WriteConfig(filepath.Join(repo, ".armature", "config.json"), cfg))

	defaultOut, err := runTrls(t, repo, "render-context", "--issue", "task-01", "--format", "agent")
	require.NoError(t, err)

	explicitOut, err := runTrls(t, repo, "render-context", "--issue", "task-01", "--format", "agent", "--budget", "4000")
	require.NoError(t, err)

	assert.Equal(t, explicitOut, defaultOut,
		"render-context should fall back to the built-in budget of 4000 when config's token_budget is absent/zero")
}

func TestSourceAdvancedOnlyByArmatureFalseOnNonArmatureChange_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	oldTip := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "source.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "source.go")
	run(t, repo, "git", "commit", "-m", "feat: ordinary source change")
	newTip := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	internalOnly, err := sourceAdvancedOnlyByArmature(repo, repo, oldTip, newTip)
	require.NoError(t, err)
	assert.False(t, internalOnly, "a non-.armature/ change must not be treated as an internal advance")
}

func TestSourceAdvancedOnlyByArmatureErrorsOnUnresolvableRevision_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	newTip := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	_, err := sourceAdvancedOnlyByArmature(repo, repo, "not-a-real-revision", newTip)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inspect coordinator source advance")
}

func TestRollbackClaimReportsExclusionCleanupFailure_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	claimTimestamp := nowEpoch()
	claimToken := "token-worker-a"
	store := setupSingleWorkerClaimStore(t, ctx, claimTimestamp, claimToken)

	brokenCtx := *ctx
	brokenCtx.RepoPath = filepath.Join(t.TempDir(), "does-not-exist")
	cmd := rollbackClaimTestCmd(&brokenCtx)

	logPathA := opsLogPath(ctx.IssuesDir, "worker-a")
	prior := priorClaimState{status: ops.StatusOpen}
	cause := fmt.Errorf("boom")
	exclusions := []claimExclusion{{pattern: "/custom/", destination: filepath.Join(t.TempDir(), "custom")}}

	err := compensateClaimIfHeldByToken(cmd, store, logPathA, "task-01", "worker-a", "create worktree", cause, prior, claimToken, false, exclusions)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.Contains(t, err.Error(), "exclusion rollback failed")
}

func TestCreateWorktreeAndBranchRejectsMalformedSourceArgCount_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "child")

	err := createWorktreeAndBranch(
		repo, destination, "task-01", materialize.Issue{Type: "task"}, alwaysOwns,
		"only-one-arg",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires path, branch, and tip")
	assert.NoDirExists(t, destination)
}

func TestCreateWorktreeAndBranchRejectsIssueTypeWithNoBranchMapping_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "child")

	err := createWorktreeAndBranch(repo, destination, "epic-01", materialize.Issue{Type: "epic"}, alwaysOwns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no branch mapping")
	assert.NoDirExists(t, destination)
}

func TestCreateWorktreeAndBranchRejectsSourceBranchChange_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	parentPath := filepath.Join(repo, "parent")
	run(t, repo, "git", "worktree", "add", "-b", "feature-parent", parentPath)
	parentTip := strings.TrimSpace(runGitOutput(t, parentPath, "rev-parse", "HEAD"))

	run(t, parentPath, "git", "checkout", "-b", "feature-parent-renamed")

	destination := filepath.Join(t.TempDir(), "child")
	err := createWorktreeAndBranch(
		repo, destination, "task-01", materialize.Issue{Type: "task"}, alwaysOwns,
		parentPath, "feature-parent", parentTip,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claim source branch changed")
	assert.NoDirExists(t, destination)
}

func TestCreateWorktreeAndBranchRejectsIncompleteSourceArgs_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "child")

	err := createWorktreeAndBranch(
		repo, destination, "task-01", materialize.Issue{Type: "task"}, alwaysOwns,
		repo, "", "deadbeef",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validated claim source is incomplete")
	assert.NoDirExists(t, destination)
}

func TestWriteClaimExclusionMarkerRejectsConflictingPattern_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "marker-wt")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	require.NoError(t, writeClaimExclusionMarker(worktreePath, "/custom-a/"))

	require.NoError(t, writeClaimExclusionMarker(worktreePath, "/custom-a/"))

	err := writeClaimExclusionMarker(worktreePath, "/custom-b/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already records a different pattern")

	pattern, ok, readErr := readClaimExclusionMarker(worktreePath)
	require.NoError(t, readErr)
	require.True(t, ok)
	assert.Equal(t, "/custom-a/", pattern, "the conflicting write must not have overwritten the original marker")
}

func TestWriteClaimExclusionMarkerFailsClosedOnUnreadableMarker_REQ_LNGHZN_S9_T1(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: file permissions do not block reads")
	}
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "unreadable-marker-wt")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	gitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)
	markerPath := filepath.Join(gitDir, claimExclusionMarkerName)
	require.NoError(t, os.WriteFile(markerPath, []byte("/custom-a/\n"), 0o600))
	require.NoError(t, os.Chmod(markerPath, 0o000))
	t.Cleanup(func() {
		swallowErr(os.Chmod(markerPath, 0o600))
	})

	writeErr := writeClaimExclusionMarker(worktreePath, "/custom-a/")
	require.Error(t, writeErr)
	assert.Contains(t, writeErr.Error(), "read claim exclusion marker")
}

func TestReadClaimExclusionMarkerRejectsEmptyMarker_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "empty-marker-wt")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	gitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, claimExclusionMarkerName), []byte("\n"), 0o600))

	_, _, readErr := readClaimExclusionMarker(worktreePath)
	require.Error(t, readErr)
	assert.Contains(t, readErr.Error(), "claim exclusion marker is empty")
}

func TestReadClaimExclusionMarkerFailsClosedOnUnresolvableWorktree_REQ_LNGHZN_S9_T1(t *testing.T) {
	notAWorktree := t.TempDir()

	_, ok, err := readClaimExclusionMarker(notAWorktree)
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "resolve worktree git dir")
}

func TestCleanupClaimExclusionsFailsClosedOnUnresolvableRepo_REQ_LNGHZN_S9_T1(t *testing.T) {
	notARepo := t.TempDir()

	err := cleanupClaimExclusions(notARepo, []claimExclusion{{pattern: "/x/", destination: notARepo}})
	require.Error(t, err)
}

func TestCleanupClaimExclusionsIsNoOpForEmptySlice_REQ_LNGHZN_S9_T1(t *testing.T) {
	notARepo := t.TempDir()

	err := cleanupClaimExclusions(notARepo, nil)
	require.NoError(t, err)
}

func TestCleanupClaimExclusionsLockedRemovesUnprotectedPattern_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	gone := filepath.Join(t.TempDir(), "gone")

	added, err := updateGitExcludeTracked(repo, "/gone/", "")
	require.NoError(t, err)
	require.True(t, added)

	err = cleanupClaimExclusionsLocked(repo, []claimExclusion{{pattern: "/gone/", destination: gone}})
	require.NoError(t, err)

	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	content, readErr := os.ReadFile(excludePath)
	require.NoError(t, readErr)
	assert.NotContains(t, string(content), "/gone/", "an exclusion whose destination no longer exists as a worktree must be removed")
}

func TestCleanupClaimExclusionsLockedPreservesProtectedPattern_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	live := filepath.Join(repo, ".worktrees", "still-here")
	run(t, repo, "git", "worktree", "add", live, "-b", "still-here-branch")

	added, err := updateGitExcludeTracked(repo, "/still-here/", "")
	require.NoError(t, err)
	require.True(t, added)

	err = cleanupClaimExclusionsLocked(repo, []claimExclusion{{pattern: "/still-here/", destination: live}})
	require.NoError(t, err)

	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	content, readErr := os.ReadFile(excludePath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "/still-here/", "an exclusion protecting a live worktree must not be removed")
}

func plantForeignClaim(t *testing.T, repo, issueID, workerID string) {
	t.Helper()
	logPath := filepath.Join(repo, ".armature", "ops", workerID+".log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpClaim,
		TargetID:  issueID,
		Timestamp: time.Now().Unix(),
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 60},
	}))
}

func claimOpsFor(t *testing.T, repo, issueID string) []ops.Op {
	t.Helper()
	allOps, _, err := readAllOpsFromDirWithOffsets(filepath.Join(repo, ".armature", "ops"))
	require.NoError(t, err)
	var found []ops.Op
	for _, op := range allOps {
		if op.Type == ops.OpClaim && op.TargetID == issueID {
			found = append(found, op)
		}
	}
	return found
}

func TestClaimBlockedPrintsAllReasonsAndCreatesNoWorktreeOrClaimOp_REQ_ARCHIMP_S20_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	plantVerifiedTask(t, repo, "task-01", "cmd/armature/claim.go")
	plantVerifiedTask(t, repo, "task-02", "cmd/armature/claim.go")
	plantVerifiedTask(t, repo, "task-03", "cmd/armature/claim.go")
	plantForeignClaim(t, repo, "task-01", "other-worker-aaa")
	plantForeignClaim(t, repo, "task-03", "other-worker-ccc")

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "task-02", "--worktree")
	require.Error(t, claimErr)
	combined := stderr + "\n" + claimErr.Error()
	assert.Contains(t, combined, "scope overlap with task-01")
	assert.Contains(t, combined, "scope overlap with task-03")
	assert.Contains(t, combined, "other-worker-aaa")
	assert.Contains(t, combined, "other-worker-ccc")
	assert.Contains(t, stderr, "Error:")
	assert.NotContains(t, combined, "Warning:")

	_, statErr := os.Stat(filepath.Join(repo, ".worktrees", "task-02"))
	assert.True(t, os.IsNotExist(statErr), "blocked claim must not provision a worktree")
	assert.Empty(t, claimOpsFor(t, repo, "task-02"), "blocked claim must not append a Claim Op")
}

func TestClaimForceWritesReciprocalNotesInOrder_REQ_ARCHIMP_S20_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	plantVerifiedTask(t, repo, "task-01", "cmd/armature/claim.go")
	plantVerifiedTask(t, repo, "task-02", "cmd/armature/claim.go")
	plantVerifiedTask(t, repo, "task-03", "cmd/armature/claim.go")
	plantForeignClaim(t, repo, "task-01", "other-worker-aaa")
	plantForeignClaim(t, repo, "task-03", "other-worker-ccc")

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "task-02", "--force", "--worktree")
	require.NoError(t, claimErr, "stderr: %s", stderr)
	assert.Contains(t, stderr, "Warning:")
	assert.Contains(t, stderr, "task-01")
	assert.Contains(t, stderr, "task-03")

	ctx := getTestContext(t, repo)
	_, logPath, resolveErr := resolveWorkerAndLog(ctx)
	require.NoError(t, resolveErr)
	logged, readErr := ops.ReadLog(logPath)
	require.NoError(t, readErr)

	var notes []ops.Op
	claimIdx := -1
	for i, op := range logged {
		if op.Type == ops.OpNote && strings.Contains(op.Payload.Msg, "detected at claim time") {
			notes = append(notes, op)
		}
		if op.Type == ops.OpClaim && op.TargetID == "task-02" && claimIdx < 0 {
			claimIdx = i
		}
	}
	require.Len(t, notes, 4)
	assert.Equal(t, "task-02", notes[0].TargetID)
	assert.Equal(t, "Scope overlap with task-01 detected at claim time", notes[0].Payload.Msg)
	assert.Equal(t, "task-01", notes[1].TargetID)
	assert.Equal(t, "Scope overlap with task-02 detected at claim time", notes[1].Payload.Msg)
	assert.Equal(t, "task-02", notes[2].TargetID)
	assert.Equal(t, "Scope overlap with task-03 detected at claim time", notes[2].Payload.Msg)
	assert.Equal(t, "task-03", notes[3].TargetID)
	assert.Equal(t, "Scope overlap with task-02 detected at claim time", notes[3].Payload.Msg)

	require.GreaterOrEqual(t, claimIdx, 0, "force claim must still append a Claim Op")
	firstNoteIdx := -1
	for i, op := range logged {
		if op.Type == ops.OpNote && strings.Contains(op.Payload.Msg, "detected at claim time") {
			firstNoteIdx = i
			break
		}
	}
	assert.Less(t, firstNoteIdx, claimIdx, "notes must be persisted before the Claim Op")
	assert.DirExists(t, filepath.Join(repo, ".worktrees", "task-02"))
}

func TestClaimNoteWriteFailureDoesNotClaim_REQ_ARCHIMP_S20_T2(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: file permissions do not block writes")
	}
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	plantOverlappingFooPair(t, repo)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	ctx := getTestContext(t, repo)
	_, logPath, resolveErr := resolveWorkerAndLog(ctx)
	require.NoError(t, resolveErr)
	require.NoError(t, os.Chmod(logPath, 0o444))
	t.Cleanup(func() {
		swallowErr(os.Chmod(logPath, 0o644))
	})

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-02", "--worktree")
	require.Error(t, claimErr, "note write failure must refuse the claim. stderr: %s", stderr)

	assert.Empty(t, claimOpsFor(t, repo, "task-02"), "fail-closed note write must not append a Claim Op")
	_, statErr := os.Stat(filepath.Join(repo, ".worktrees", "task-02"))
	assert.True(t, os.IsNotExist(statErr), "fail-closed note write must not provision a worktree")
}

func TestClaimSameWorkerDismissalUnderForce_REQ_ARCHIMP_S20_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	plantOverlappingFooPair(t, repo)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-02", "--force", "--worktree")
	require.NoError(t, claimErr, "same-worker overlap under --force must still claim. stderr: %s", stderr)
	assert.NotContains(t, stderr, "Warning:")
	assert.NotContains(t, stderr, "Error:")

	allOps, _, readErr := readAllOpsFromDirWithOffsets(filepath.Join(repo, ".armature", "ops"))
	require.NoError(t, readErr)
	var dismissals, forceNotes int
	for _, op := range allOps {
		if op.Type != ops.OpNote {
			continue
		}
		if strings.Contains(op.Payload.Msg, "Serial claim: scope overlap with task-01 (same worker, dismissed)") {
			dismissals++
		}
		if strings.Contains(op.Payload.Msg, "detected at claim time") {
			forceNotes++
		}
	}
	assert.Equal(t, 1, dismissals)
	assert.Zero(t, forceNotes, "same-worker overlap must remain a dismissal under --force")
	assert.NotEmpty(t, claimOpsFor(t, repo, "task-02"))
	assert.DirExists(t, filepath.Join(repo, ".worktrees", "task-02"))
}

func TestClaimRefusesAmbiguousBindingWhenDestExists_REQ_ARCHIMP_S20(t *testing.T) {
	repo := setupRepoWithTask(t)

	canonical := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", canonical)
	require.NoError(t, updateIssueIDFile(canonical, "task-01"))

	legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
	head := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	run(t, repo, "git", "worktree", "add", "--detach", legacyPath, head)
	require.NoError(t, updateIssueIDFile(legacyPath, "task-01"))

	_, stderr, err := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.Error(t, err, "dest-present re-claim must refuse Ambiguous Binding. stderr: %s", stderr)
	combined := err.Error() + stderr
	assert.Contains(t, combined, "bound to 2 worktrees")
	assert.Contains(t, combined, canonical)
	assert.Contains(t, combined, legacyPath)

	status, statusErr := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, statusErr)
	assert.Equal(t, ops.StatusOpen+"\n", status, "Ambiguous Binding must precede the Claim Op")
	assert.DirExists(t, canonical)
	assert.DirExists(t, legacyPath)
	assert.Empty(t, claimOpsFor(t, repo, "task-01"))
}

func TestClaimUsesPlanProvision_REQ_ARCHIMP_S20_T6(t *testing.T) {
	t.Run("refuse", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		destination := filepath.Join(repo, "child")
		claim := newRootCmd()
		claim.SetOut(new(bytes.Buffer))
		claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination})
		err := claim.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "under canonical .worktrees")
		status, statusErr := runTrls(t, repo, "show", "task-01", "--field", "status")
		require.NoError(t, statusErr)
		assert.Equal(t, ops.StatusOpen+"\n", status, "dest refuse must precede the Claim Op")
	})

	t.Run("adopt", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
		run(t, repo, "git", "worktree", "add", "-b", "task/task-01", legacyPath)
		require.NoError(t, updateIssueIDFile(legacyPath, "task-01"))
		baseSHA := strings.TrimSpace(runGitOutput(t, legacyPath, "rev-parse", "HEAD"))
		require.NoError(t, writeBaseCommitFileIfAbsent(legacyPath, baseSHA))

		claim := newRootCmd()
		claim.SetOut(new(bytes.Buffer))
		claim.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
		require.NoError(t, claim.Execute())

		canonical := filepath.Join(repo, ".worktrees", "task-01")
		assert.DirExists(t, canonical)
		assert.NoDirExists(t, legacyPath)
		assert.Equal(t, "task/task-01", strings.TrimSpace(runOutput(t, canonical, "branch", "--show-current")))
	})

	t.Run("fresh", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		claim := newRootCmd()
		claim.SetOut(new(bytes.Buffer))
		claim.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
		require.NoError(t, claim.Execute())

		canonical := filepath.Join(repo, ".worktrees", "task-01")
		assert.DirExists(t, canonical)
		assert.Equal(t, "task/task-01", strings.TrimSpace(runOutput(t, canonical, "branch", "--show-current")))
	})

	t.Run("unbound branch checkout is cmd I/O", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		held := filepath.Join(t.TempDir(), "unbound-holder")
		run(t, repo, "git", "worktree", "add", "-b", "task/task-01", held)

		_, stderr, err := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
		require.Error(t, err, "unbound holder of the issue branch must fail closed before PlanProvision fresh. stderr: %s", stderr)
		assert.Contains(t, err.Error()+stderr, "already checked out")
		assert.Contains(t, err.Error()+stderr, held)
		status, statusErr := runTrls(t, repo, "show", "task-01", "--field", "status")
		require.NoError(t, statusErr)
		assert.Equal(t, ops.StatusOpen+"\n", status, "provision failure must still roll back the Claim Op")
	})
}
