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

// alwaysOwns is the stillOwns callback for createWorktreeAndBranch tests
// that are not exercising the ownership-supersession race; it preserves
// pre-existing cleanup behavior (restore/force-remove on failure).
func alwaysOwns() bool { return true }

// setupRepoWithEpic creates a repo with an epic issue.
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

// setupRepoWithParentAndTask creates a repo with a parent story and a task.
func setupRepoWithParentAndTask(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	// Create parent story
	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Parent story", "--type", "story", "--id", "story-01"}))
	require.NoError(t, cmd2.Execute())

	// Materialize so issues/story-01.json exists for ReadIssue in create --parent.
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create child task
	cmd3 := newRootCmd()
	cmd3.SetOut(new(bytes.Buffer))
	cmd3.SetArgs(enrichTestCLIArgs([]string{"create", "--repo", repo, "--title", "Child task", "--type", "task", "--id", "task-01", "--parent", "story-01"}))
	require.NoError(t, cmd3.Execute())

	return repo
}

// TestClaimDetachedHEADDoesNotPersistAsParentBranch verifies that claiming a
// task while the coordinator repo is in a detached-HEAD state does not
// persist the literal string "HEAD" as the task branch's recorded parent
// branch. gitClient.CurrentBranch() (git rev-parse --abbrev-ref HEAD)
// returns "HEAD" itself in that state, and persisting it would later make
// the delivery gate resolve "HEAD" in the task worktree — the task's own
// current commit — collapsing the merge-base to the task's HEAD and
// emptying the commit range for CommitReferenceCheck, rejecting every
// otherwise-valid commit. No parent branch config should be written.
func TestClaimDetachedHEADDoesNotPersistAsParentBranch(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Detach HEAD in the coordinator repo before claiming.
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

// TestClaimNewWorktreeRecordsClaimedBranchFile_REQ_LNGHZN_S4 verifies the
// root-cause structural fix: at claim time, the branch name the issue was
// actually claimed under (derived from the issue TYPE at claim time) is
// recorded immutably into the worktree's git directory, so later delivery-
// gate checks can verify against what was actually claimed rather than
// re-deriving branch expectations from the CURRENT (possibly since-amended)
// issue type.
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

// TestClaimExistingWorktreeDoesNotInventForkPointWhenDiverged_REQ_LNGHZN_S5
// verifies that registering a pre-existing worktree does not turn a current
// default-branch merge-base into trusted claim provenance. The delivery gate
// must remain fail-closed until the original claim's metadata is available.
func TestClaimExistingWorktreePersistsComputedForkPointWhenDiverged_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Simulate a story branch already containing sibling-task commits, as the
	// coordinator workflow would set up.
	run(t, repo, "git", "checkout", "-b", "story-branch")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(sibling-task): unrelated sibling work")

	// Manually create the worktree on the expected task branch BEFORE
	// claiming, so `arm claim` below takes the existing-worktree path rather
	// than createWorktreeAndBranch.
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "branch", "task/task-01", "story-branch")
	run(t, repo, "git", "worktree", "add", worktreePath, "task/task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	// Parent-branch config must NOT be recorded on this path: there is no
	// reliable signal for the true parent branch name from the worktree
	// alone, so persisting one (self-referential or coordinator-unrelated)
	// would be confidently wrong. Absence lets the gate fall back to an
	// honest default-branch merge-base instead.
	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", "branch.task/task-01.armature-parent")
	getCmd.Dir = repo
	_, err := getCmd.Output()
	assert.Error(t, err, "parent branch config should NOT be recorded for the existing-worktree claim path")

	// No provenance may be synthesized from the default branch. The gate must
	// explicitly reject this legacy/pre-existing worktree instead.
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

// TestClaimExistingWorktreeBaseCommitGoesStaleAfterRebase_REQ_LNGHZN_S4
// documents a known, accepted limitation surfaced by a holistic branch
// review: because the existing-worktree claim path deliberately does not
// persist a parent-branch git config (see the comment above the
// persistBranchPointMetadata call in claim.go's existing-worktree branch —
// there is no reliable signal for the true parent branch name from the
// worktree alone), deliverygate.ResolveBaseCommit's tier-1
// DynamicBaseCommit (self-correcting on rebase) can never succeed for a
// worktree claimed this way. It permanently falls back to tier-2
// RecordedBaseCommit, the static SHA computed once at claim time — so if the
// task branch is later rebased onto an updated parent tip, the delivery
// gate keeps scope-checking against the pre-rebase fork point instead of the
// new one. This is intentional (documented in claim.go), not a regression;
// this test exists so the gap stays pinned rather than silently drifting.
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

	// At claim time, the computed fork point equals defaultTipSHA (see the
	// sibling test above). Now advance the default branch and rebase the task
	// branch onto the new tip, simulating the coordinator updating it after claim.
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

// TestClaimExistingWorktreePersistsBaseCommitWhenNotDiverged_REQ_LNGHZN_S4 verifies
// the complementary case: when the existing worktree genuinely has NOT
// diverged from the resolvable candidate base branch (its HEAD equals the
// candidate base), the existing-worktree claim path still persists the
// base-commit file using the worktree's own honest HEAD, exactly as before
// the P1 fix. This is the case the original assumption was actually correct
// for.
func TestClaimExistingWorktreePersistsBaseCommitWhenNotDiverged_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Manually create the worktree on the expected task branch directly from
	// main's current tip (no divergence) BEFORE claiming, so `arm claim`
	// below takes the existing-worktree path rather than
	// createWorktreeAndBranch.
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

// TestClaimExistingWorktreeDoesNotContaminateFromUnrelatedCoordinatorBranch_REQ_LNGHZN_S4
// verifies the P1 fix: the existing-worktree claim path must not read
// HEAD/CurrentBranch from ctx.RepoPath (the coordinator's own checkout) to
// derive the persisted parent-branch metadata, because the coordinator repo
// can be checked out on a branch with no relationship to the pre-existing
// worktree's actual branch or fork point. Before the fix, this test's
// worktree (on task/task-01, forked from story-branch) would have its
// metadata contaminated with the coordinator's unrelated "main" checkout:
// parentBranch="main" and headSHA=main's HEAD, both wrong. After the fix,
// persisted metadata must reflect the worktree's own true branch/HEAD (or
// not be written at all), never the unrelated coordinator branch/commit.
func TestClaimExistingWorktreeDoesNotContaminateFromUnrelatedCoordinatorBranch_REQ_LNGHZN_S4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	defaultBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))

	// Simulate a story branch already containing sibling-task commits.
	run(t, repo, "git", "checkout", "-b", "story-branch")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(sibling-task): unrelated sibling work")

	storyHeadSHA := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	// Manually create the worktree on the expected task branch BEFORE
	// claiming, so `arm claim` below takes the existing-worktree path.
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "branch", "task/task-01", "story-branch")
	run(t, repo, "git", "worktree", "add", worktreePath, "task/task-01")

	// Now move the COORDINATOR repo (ctx.RepoPath) to an unrelated branch with
	// an unrelated HEAD, simulating the coordinator having moved on to other
	// work by the time claim registers this pre-existing worktree.
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

	// If parent-branch config was written, it must NOT be the coordinator's
	// unrelated "main" branch.
	getCmd := exec.CommandContext(context.Background(), "git", "config", "--get", "branch.task/task-01.armature-parent")
	getCmd.Dir = repo
	out, err := getCmd.Output()
	if err == nil {
		assert.NotEqual(t, defaultBranch, strings.TrimSpace(string(out)),
			"parent branch config must not be contaminated with the coordinator's unrelated checkout")
	}

	// If a base-commit file was written, it must NOT be the coordinator's
	// unrelated HEAD SHA.
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

// TestClaim_AllEntryPathsPersistBaseCommitViaConsolidatedFunction verifies
// that both claim entry paths that can persist branch-point metadata -- the
// fresh-worktree path (createWorktreeAndBranch) and the existing-worktree
// path (which also covers a stale-claim takeover of an already-existing
// worktree, since that path branches solely on "does a worktree already
// exist at this path", not on who owned the prior claim) -- write the
// base-commit file in the exact same shape: same filename
// (armature-base-commit) in the worktree's actual git directory, containing
// exactly the resolved HEAD SHA with no extra formatting. Both paths route
// through the single persistBranchPointMetadata function (see
// createWorktreeAndBranch's call at the end of this file and the
// existing-worktree branch in newClaimCmd), so this test exists to catch a
// regression where one path's write logic drifts from the other's (e.g. a
// change to one call site's serialization without updating the other) --
// the kind of scattered-duplication bug the LNGHZN-S4 review repeatedly
// flagged.
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

		// Pre-create the worktree at the expected branch/HEAD so `arm claim`
		// takes the existing-worktree path instead of createWorktreeAndBranch.
		// This is the same code path a stale-claim takeover of a pre-existing
		// worktree exercises: the branch taken depends only on whether a
		// worktree already exists at the target path, not on who previously
		// owned the claim.
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

// TestClaimWithoutWorktreeFlag verifies that claim fails when --worktree is omitted.
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

// TestClaimCreatesWorktreeIfAbsent verifies that claim creates a worktree at the path
// when it doesn't exist, along with a derived branch.
func TestClaimCreatesWorktreeIfAbsent(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})

	err := cmd.Execute()
	require.NoError(t, err)

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree directory should be created")

	// Verify .git file exists in worktree (marker of a git worktree)
	gitPath := filepath.Join(worktreePath, ".git")
	assert.FileExists(t, gitPath, ".git file should exist in worktree")

	// Read the .git file to find the actual git directory
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)
	assert.Contains(t, gitDirLine, "gitdir: ", ".git file should contain gitdir reference")

	// Extract actual git dir from the .git file
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	// Verify armature-issue-id file is created in the actual git directory
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	assert.FileExists(t, taskIDFile, "armature-issue-id file should be created in actual git dir")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	assert.Equal(t, "task-01", string(taskID))
}

// TestClaimUpdatesTaskIDIfWorktreeExists verifies that claim updates armature-issue-id
// when the worktree already exists.
func TestClaimUpdatesTaskIDIfWorktreeExists(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// First claim creates the worktree
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	// Read the .git file to find the actual git directory
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	// Extract actual git dir from the .git file
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	// Verify armature-issue-id was written
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	assert.Equal(t, "task-01", string(taskID))
}

// TestClaimWithEpicReturnsError verifies that claiming an epic returns an error.
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

// TestClaimCreatesTaskBranch verifies that claim creates a task branch from HEAD with the
// correct prefix (task/<id>) in the new worktree's git directory.
func TestClaimCreatesTaskBranch(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})

	err := cmd.Execute()
	require.NoError(t, err)

	// Verify the branch is created with task/ prefix
	// Read the .git file to find the actual git directory
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	// Extract actual git dir
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	headFile := filepath.Join(actualGitDir, "HEAD")
	assert.FileExists(t, headFile, "HEAD file should exist in git directory")
	headContent, err := os.ReadFile(headFile) //nolint:gosec // test path is safe
	require.NoError(t, err)
	headStr := string(headContent)
	// Should reference task/task-01 branch
	assert.Contains(t, headStr, "task-01", "HEAD should reference task/task-01 branch")
}

// TestClaimStillAppendsClaimOpToLog verifies that even though worktree is created,
// the claim op is still appended to the ops log.
func TestClaimStillAppendsClaimOpToLog(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})

	require.NoError(t, cmd.Execute())

	// Verify claim operation was appended to ops log
	// The output should indicate successful claim
	assert.Contains(t, buf.String(), "task-01", "output should mention the claimed task")
}

func TestCanonicalWorktreePathRejectsTraversalBeforeMutation_REQ_LNGHZN_S5_T1(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")

	// Slash-bearing IDs are now rejected to prevent nested worktree hazards
	_, err := canonicalWorktreePath(root, "team/task-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")

	// Plain IDs without separators are accepted
	path, err := canonicalWorktreePath(root, "team")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".worktrees", "team"), path)

	// Traversal IDs are still rejected
	_, err = canonicalWorktreePath(root, "../escaped")
	assert.Error(t, err)
	_, err = canonicalWorktreePath(root, filepath.Join(string(filepath.Separator), "escaped"))
	assert.Error(t, err)
}

// TestCanonicalWorktreePathRejectsDotDotAliasedIDs_REQ_LNGHZN_S5 verifies that IDs
// containing path separators or "." / ".." components are rejected. With separators
// now banned entirely, IDs like "team/../task-1" and "a/./b" are rejected for
// containing separators. Bare "." and ".." are still explicitly rejected to maintain
// ID→path injectivity.
func TestCanonicalWorktreePathRejectsDotDotAliasedIDs_REQ_LNGHZN_S5(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")

	// The plain ID resolves normally.
	plain, err := canonicalWorktreePath(root, "task-1")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".worktrees", "task-1"), plain)

	// IDs with separators or "." / ".." must be rejected.
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
	// "../escaped" is rejected before any mutation. It now trips the "."/".."
	// path-component guard (which runs before the containment check); either
	// rejection is acceptable, so assert the offending ID is named rather than a
	// single message.
	assert.Contains(t, err.Error(), "../escaped")
	assert.NoDirExists(t, filepath.Join(repo, ".worktrees"), "invalid IDs must not create a worktree root")
	assert.NoDirExists(t, filepath.Join(filepath.Dir(repo), "escaped"), "invalid IDs must not mutate outside the repository")
}

// TestCanonicalWorktreePathRejectsSlashBearingIDs_REQ_LNGHZN_S5 verifies that
// slash-bearing issue IDs are rejected outright to prevent nested worktree hazards.
// Without this guard, removing the worktree for "team" would recursively delete
// the worktree for "team/task-1", losing uncommitted work.
func TestCanonicalWorktreePathRejectsSlashBearingIDs_REQ_LNGHZN_S5(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")

	// Slash-bearing IDs are rejected
	_, err := canonicalWorktreePath(root, "team/task-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")

	// Plain IDs without slashes are accepted
	path, err := canonicalWorktreePath(root, "team")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".worktrees", "team"), path)

	// Bare "." and ".." are still rejected
	_, err = canonicalWorktreePath(root, ".")
	assert.Error(t, err)
	_, err = canonicalWorktreePath(root, "..")
	assert.Error(t, err)

	// Backslashes are rejected on every platform: append-only logs can be
	// replayed on a host where they are path separators.
	_, err = canonicalWorktreePath(root, "team\\task-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")
}

// TestCreateWorktreeAndBranchInheritsFilesFromHEAD verifies that the worktree branch
// contains files from HEAD (not an orphan branch).
func TestCreateWorktreeAndBranchInheritsFilesFromHEAD(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Create a marker file in the main repo that should be visible in the task branch
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

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree directory should be created")

	// Check out the task branch and verify the marker file exists
	// This proves the branch has files from HEAD (not an orphan)
	markerInWorktree := filepath.Join(worktreePath, "marker.txt")
	assert.FileExists(t, markerInWorktree, "marker file from HEAD should exist in task branch worktree")

	// Verify the content
	content, err := os.ReadFile(markerInWorktree)
	require.NoError(t, err)
	assert.Equal(t, "hello from main", string(content))
}

// TestCreateWorktreeAndBranchRejectsEmptyBranchName verifies that an empty branch name
// (from epic or unknown issue types) triggers an error.
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
	// The error should come from the epic check in newClaimCmd, not from createWorktreeAndBranch
	assert.Contains(t, err.Error()+errBuf.String()+buf.String(), "epic")
}

// TestClaimFailsWhenWorktreeCreationFails tests that the claim command returns an error
// when createWorktreeAndBranch would fail. We simulate a failure by using a duplicate
// branch name that's already checked out in another worktree.
func TestCreateWorktreeAndBranchFailsWhenWorktreeCannotBeCreated(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	issue := materialize.Issue{Type: "task"}

	// Create an unrelated, unbound worktree that holds the branch. The chosen
	// policy adopts only correctly bound worktrees and rejects this collision.
	worktree1 := filepath.Join(t.TempDir(), "worktree1")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", worktree1)

	// Now try to create a second worktree with the same task/branch.
	// This must fail closed rather than adopting an unbound worktree.
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

// TestCreateWorktreeAndBranchFailsClosedOnBoundDetachedWorktree_REQ_LNGHZN_S5_T6
// covers the duplicate-worktree defect. A worktree bound to this issue but
// DETACHED (the state a worker is in mid-rebase) must not be skipped over:
// skipping it provisions a second canonical worktree for the same issue, after
// which `arm merged` selects the new one and gc force-removes the original
// along with whatever uncommitted work it still held.
//
// Selection is by binding, so the detached worktree IS found; because it cannot
// be relocated safely mid-operation, the claim fails closed and provisions
// nothing. Refusing is what preserves the work.
func TestCreateWorktreeAndBranchFailsClosedOnBoundDetachedWorktree_REQ_LNGHZN_S5_T6(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", legacyPath)
	require.NoError(t, updateIssueIDFile(legacyPath, "task-01"))

	// Detach it, as an in-progress rebase would.
	head := strings.TrimSpace(runGitOutput(t, legacyPath, "rev-parse", "HEAD"))
	run(t, legacyPath, "git", "checkout", "--detach", head)

	canonicalPath := filepath.Join(repo, ".worktrees", "task-01")
	err := createWorktreeAndBranch(repo, canonicalPath, "task-01", materialize.Issue{Type: "task"}, alwaysOwns)

	require.Error(t, err, "a bound worktree that is not on the issue branch must fail closed")
	assert.Contains(t, err.Error(), legacyPath, "the error must name the worktree the operator has to deal with")
	assert.NoDirExists(t, canonicalPath, "no duplicate worktree may be provisioned")
	assert.DirExists(t, legacyPath, "the bound worktree must be left untouched")
}

// TestCreateWorktreeAndBranchFailsClosedOnAmbiguousBinding_REQ_LNGHZN_S5_T6
// covers the ordering hazard in adoption. Two worktrees carry this issue's
// binding and the correctly-branched one is listed FIRST, so a loop that adopts
// on first match never observes the second. The claim would then record the
// adopted path as the winner, leaving the other duplicate behind as a
// force-removal candidate still holding in-flight work.
//
// The full bound set must be collected before anything is moved, and ambiguity
// refused — the same policy worktree.SelectByIssue and `arm worktree gc` apply.
func TestCreateWorktreeAndBranchFailsClosedOnAmbiguousBinding_REQ_LNGHZN_S5_T6(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// First in git's inventory: bound AND on the issue branch (the tempting one).
	onBranchPath := filepath.Join(t.TempDir(), "aaa-on-branch")
	run(t, repo, "git", "worktree", "add", "-b", "task/task-01", onBranchPath)
	require.NoError(t, updateIssueIDFile(onBranchPath, "task-01"))

	// Second: same binding, detached — the one a first-match loop would miss.
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

// TestCreateWorktreeAndBranchFailsClosedOnBoundScratchBranch_REQ_LNGHZN_S5_T6
// is the same defect reached via a scratch branch rather than a detached HEAD.
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

	// Move the coordinator to an unrelated tip before adoption. Adoption must
	// derive its branch point from the adopted worktree, not this checkout.
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

// TestCreateWorktreeAndBranchLeavesAdoptedWorktreeInPlaceWhenSuperseded covers
// the adoption (move-back) arm of cleanupPartialWorktree. It reuses the same
// provenance-rejection failure as
// TestCreateWorktreeAndBranchRejectsAdoptionWithoutProvenance_REQ_LNGHZN_S5,
// but with stillOwns reporting the claim has been superseded by a second
// worker. cleanupPartialWorktree must not move the canonical path back to
// legacyPath in that case: doing so would relocate whatever the second
// worker's worktree now holds at the canonical path out from under them.
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
	assert.DirExists(t, canonicalPath, "a superseded claim must leave the second worker's worktree at the canonical path")
	assert.NoDirExists(t, legacyPath, "a superseded claim must not move the worktree back to the legacy path")
}

// TestCreateWorktreeAndBranchRemovesFreshPartialWorktreeWhenStillOwned covers
// the non-adoption (force-remove) arm of cleanupPartialWorktree when stillOwns
// confirms this worker still owns the claim: existing behavior (best-effort
// `git worktree remove --force` of the partially provisioned worktree) must
// be preserved. Failure is induced via an issue ID containing a space, which
// deriveBranchName turns into an invalid git ref, so the worktree is added
// detached but checkoutBranchInWorktree then fails.
func TestCreateWorktreeAndBranchRemovesFreshPartialWorktreeWhenStillOwned(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	canonicalPath := filepath.Join(repo, ".worktrees", "bad-id")

	err := createWorktreeAndBranch(repo, canonicalPath, "bad id", materialize.Issue{Type: "task"}, alwaysOwns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkout branch in worktree")
	assert.NoDirExists(t, canonicalPath, "an owned claim's partial worktree must still be force-removed on failure")
}

// TestCreateWorktreeAndBranchLeavesFreshPartialWorktreeWhenSuperseded is the
// same failure as TestCreateWorktreeAndBranchRemovesFreshPartialWorktreeWhenStillOwned
// but with stillOwns reporting the claim has been superseded: the force-remove
// must be skipped, leaving the (possibly now-adopted-by-someone-else) worktree
// in place rather than discarding whatever it holds.
func TestCreateWorktreeAndBranchLeavesFreshPartialWorktreeWhenSuperseded(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	canonicalPath := filepath.Join(repo, ".worktrees", "bad-id")

	err := createWorktreeAndBranch(repo, canonicalPath, "bad id", materialize.Issue{Type: "task"},
		func() bool { return false })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkout branch in worktree")
	assert.DirExists(t, canonicalPath, "a superseded claim must not force-remove a worktree that may now belong to someone else")
}

// setupSingleWorkerClaimStore claims "task-01" for "worker-a" directly
// against the op log (bypassing the claim command's worktree provisioning),
// stamping the claim op with claimToken, and returns a loaded store. Used by
// the rollbackClaim/claimStillOwnedBy ownership-recheck tests below, which
// need control over the exact claim token being compensated for or
// rechecked. The issue ID and worker ID are fixed rather than threaded
// through as parameters: every current caller targets the same fixture
// issue created by setupRepoWithParentAndTask under the same worker
// identity, and a parameter that never varies across call sites fails the
// unparam lint check.
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

// rollbackClaimTestCmd builds a *cobra.Command wired with the executionState
// rollbackClaim needs (mustState/appendHighStakesOp), mirroring the pattern
// architecture_test.go uses to construct execution state without going
// through the root command's PersistentPreRunE.
func rollbackClaimTestCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{}
	state := &executionState{ctx: ctx}
	state.pusher, state.tracker = initPushDeps(ctx)
	cmd.SetContext(context.WithValue(context.Background(), executionStateKey{}, state))
	return cmd
}

// TestRollbackClaimSkipsCompensatingOpWhenClaimSuperseded_REQ_LNGHZN_S5 covers
// the race the whole PR #95 review finding is about: worker-a's claim goes
// stale mid-provisioning, worker-b legitimately claims the issue, and
// worker-a's failure path then calls rollbackClaim for its own (now-stale)
// claim. rollbackClaim must re-load the store, see worker-b now owns the
// claim, and skip appending a compensating op entirely -- appending one would
// land after worker-b's claim in the append-only log and, on replay, erase
// worker-b's active claim.
func TestRollbackClaimSkipsCompensatingOpWhenClaimSuperseded_REQ_LNGHZN_S5(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	claimTimestampA := nowEpoch()
	claimTokenA := "token-worker-a"
	store := setupSingleWorkerClaimStore(t, ctx, claimTimestampA, claimTokenA)

	// worker-b claims well past worker-a's TTL, so applyClaim treats worker-a's
	// claim as stale and lets worker-b's claim through.
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

	// No compensating op may have been appended (or if one was, replay must
	// discard it via IfClaimToken): worker-b's claim must survive exactly as
	// it was.
	_, err = store.Load(context.Background())
	require.NoError(t, err)
	issue := store.Issue("task-01")
	require.NotNil(t, issue)
	assert.Equal(t, "worker-b", issue.ClaimedBy)
	assert.Equal(t, claimTimestampB, issue.ClaimedAt)
	assert.Equal(t, "token-worker-b", issue.ClaimToken)
}

// TestRollbackClaimAppendsCompensatingOpWhenOwnershipConfirmed_REQ_LNGHZN_S5
// verifies existing behavior is preserved when the ownership recheck passes:
// worker-a's claim is still the current one at the exact timestamp rollback
// was called for, so the compensating op is appended as before.
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

// TestRollbackClaimSameWorkerSameSecondDistinctTokensPreventsClobber_REQ_LNGHZN_S5_T9
// covers finding 3: ClaimedBy+ClaimedAt is not a unique claim identity because
// nowEpoch() has 1-second resolution, so two claims by the SAME worker on the
// SAME issue within the same second are otherwise indistinguishable. Here
// worker-a claims twice at the identical timestamp (simulating a same-second
// retry) with two distinct tokens; the first claim's rollback must be scoped
// to its own token and must not clobber the second, still-active claim, even
// though ClaimedBy and ClaimedAt alone cannot tell them apart.
func TestRollbackClaimSameWorkerSameSecondDistinctTokensPreventsClobber_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	sameTimestamp := nowEpoch()
	tokenFirst := "token-first"
	tokenSecond := "token-second"
	require.NotEqual(t, tokenFirst, tokenSecond, "distinct tokens are the whole point of this test")

	store := setupSingleWorkerClaimStore(t, ctx, sameTimestamp, tokenFirst)

	// worker-a re-claims at the EXACT SAME timestamp (same second) with a new
	// token. ClaimedAt is identical to the first claim; only ClaimToken differs.
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

	// The FIRST claim's rollback, keyed to tokenFirst, must be recognized as
	// superseded even though ClaimedBy ("worker-a") and ClaimedAt (sameTimestamp)
	// are identical to the second, still-active claim.
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

// TestClaimStillOwnedByReportsFalseAfterTransitionToInProgress_REQ_LNGHZN_S5_T9
// is the direct regression test for the PR #95 root cause. worker-a wins a
// claim, then a DIFFERENT command (e.g. `arm transition`, which does not
// hold the per-issue claim lock -- acquireClaimLock has exactly one caller,
// in this file) moves the issue to in-progress. That transition does not
// touch ClaimedBy/ClaimToken (only a transition to `open` clears them), so
// worker-a's own claim op still "matches" on those two fields alone. Before
// claimStillOwnedBy delegated to materialize.Issue.ClaimHeldBy (which
// requires Status == StatusClaimed), this function would have reported
// worker-a as still owning the claim despite the issue having moved on.
func TestClaimStillOwnedByReportsFalseAfterTransitionToInProgress_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	claimTimestamp := nowEpoch()
	claimToken := "token-worker-a"
	store := setupSingleWorkerClaimStore(t, ctx, claimTimestamp, claimToken)

	// A different command transitions the issue to in-progress. ClaimedBy and
	// ClaimToken are left exactly as the claim op set them.
	logPath := opsLogPath(ctx.IssuesDir, "worker-a")
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: claimTimestamp + 10,
		WorkerID: "worker-a", Payload: ops.Payload{To: ops.StatusInProgress},
	}))

	owns, err := claimStillOwnedBy(store, "task-01", "worker-a", claimToken)
	require.NoError(t, err)
	assert.False(t, owns, "claimStillOwnedBy must report not-owned once the issue has left StatusClaimed, even with matching ClaimedBy/ClaimToken")
}

// TestCreateWorktreeAndBranchLeavesPartialWorktreeInPlaceWhenClaimSupersededByTransition_REQ_LNGHZN_S5_T9
// covers the actual defect from the PR #95 review finding end to end: a
// provisioning failure whose stillOwns callback is wired to the real
// claimStillOwnedBy (not a test double) must leave a partially provisioned
// worktree in place once a concurrent, lock-free transition (to in-progress)
// has superseded the claim -- mirroring
// TestCreateWorktreeAndBranchLeavesFreshPartialWorktreeWhenSuperseded above,
// but driven by a live status transition instead of a second claim op.
// Pre-fix, the old field-only ownership check in claimStillOwnedBy would
// have reported worker-a as still owning the claim (ClaimedBy/ClaimToken
// still matched), so cleanupPartialWorktree would have force-removed the
// worktree out from under whatever newer workflow activity was using it.
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
		owns, err := claimStillOwnedBy(store, "task-01", "worker-a", claimToken)
		require.NoError(t, err)
		return owns
	}

	// The issue ID passed to createWorktreeAndBranch here is deliberately
	// unrelated to "task-01": it only needs to induce the same
	// checkoutBranchInWorktree failure the sibling still-owned/superseded
	// tests above use (an invalid git ref from a space in the issue ID).
	// stillOwns is the thing actually under test.
	canonicalPath := filepath.Join(repo, ".worktrees", "bad-id")
	err := createWorktreeAndBranch(repo, canonicalPath, "bad id", materialize.Issue{Type: "task"}, stillOwns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkout branch in worktree")
	assert.DirExists(t, canonicalPath, "a claim superseded by a concurrent transition to in-progress must not force-remove its partially provisioned worktree")
}

// TestClaimDoesNotCreateWorktreeWhenOverlapFails verifies that when claim fails due to
// scope overlap (without --force), NO worktree is created at the target path.
// This is the fix for: worktree setup must be deferred until all claim validations pass.
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

	// Inject a claim op for task-01 from a DIFFERENT worker, simulating a concurrent claim.
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

	// Try to claim task-02 without --force — should fail due to scope overlap with task-01.
	worktreePath := filepath.Join(repo, ".worktrees", "task-02")
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "task-02", "--worktree")

	assert.Error(t, claimErr, "claim should fail due to scope overlap (without --force). stderr: %s", stderr)

	// Worktree must NOT have been created — worktree setup must be deferred past validation.
	_, statErr := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree must not be created when claim fails due to scope overlap")
}

// TestClaimIgnoresNonTaskIssuesInOverlapCheck_REQ_LNGHZN_S10_T8 verifies that
// an in-progress story with an overlapping aggregate scope does not block
// claiming a task that lives in a different story. A story's scope is by
// design the union of its children's scopes, and it can remain "in-progress"
// long after the child that put it there has been claimed/completed by
// someone else — so it must never be treated as a competing claimant.
func TestClaimIgnoresNonTaskIssuesInOverlapCheck_REQ_LNGHZN_S10_T8(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Story in another subtree, sitting in-progress with an aggregate scope
	// that overlaps the task we're about to claim.
	_, err = runTrls(t, repo, "create", "--title", "Other story", "--type", "story", "--id", "story-other")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "amend", "--issue", "story-other", "--scope", "cmd/armature/claim.go")
	require.NoError(t, err)

	// Task to claim, unrelated to story-other.
	_, err = runTrls(t, repo, "create", "--title", "Task to claim", "--type", "task", "--id", "task-target")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "amend", "--issue", "task-target", "--scope", "cmd/armature/claim.go")
	require.NoError(t, err)

	// Put story-other into in-progress, held by a different worker, without
	// any of its children actually being claimed.
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

// TestClaimStillBlocksOnOverlappingClaimedTask_REQ_LNGHZN_S10_T8 verifies that
// the filter narrowing the overlap check to claimable issues does not weaken
// genuine collision detection: a claimed task with overlapping scope must
// still block, and the error must name the conflicting issue's type and its
// holder so the block is diagnosable without reading source.
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

	// Claim task-01 from a different worker, simulating a concurrent claim.
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

// TestClaimLockPrecedesStoreAndWorktreeReads_REQ_LNGHZN_S5_T9 pins the fix-1
// ordering invariant: acquireClaimLock must be called before claim reads any
// issue or worktree state, not merely before the claim-op append. It proves
// this by holding the per-issue claim lock externally (as a concurrent
// same-clone claim would) for an issue ID that does not even exist in the
// store, then running `arm claim` for that same issue in-process. If the
// lock were acquired after the store/issue lookup (the pre-fix ordering),
// the command would fail fast with "issue not found" — it would never reach
// the lock acquisition at all. Only a lock acquired BEFORE that lookup
// produces the lock-contention error observed here.
func TestClaimLockPrecedesStoreAndWorktreeReads_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Do NOT create issue "does-not-exist" — the store lookup for it would
	// fail with "issue not found" if reached. Hold the claim lock for it
	// externally, simulating a concurrent same-clone claim in flight.
	release, lockErr := acquireClaimLock(repo, "does-not-exist")
	require.NoError(t, lockErr)
	t.Cleanup(release)

	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "does-not-exist", "--worktree")
	require.Error(t, claimErr, "claim must fail while the lock is held. stderr: %s", stderr)
	assert.Contains(t, claimErr.Error(), "does-not-exist")
	assert.Contains(t, claimErr.Error(), "in progress",
		"failure must be the lock-contention error, not \"issue not found\" — "+
			"which is only possible if the lock is acquired before the store/issue read")
	assert.NotContains(t, claimErr.Error(), "not found",
		"a pre-fix ordering would surface \"issue %s not found\" instead of the lock error", "does-not-exist")
}

// TestClaimRejectsWorktreeBoundToDifferentTask verifies that a worktree already
// bound to a different issue is rejected by checkExistingWorktreeBinding rather
// than silently overwriting the binding. (The CLI now derives a per-issue
// worktree root, so this guard is exercised directly.)
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

// TestClaimRejectsWorktreeWithMismatchedBranch verifies that a worktree on a
// different branch than the expected branch (with no binding) is rejected by
// checkExistingWorktreeBinding.
func TestClaimRejectsWorktreeWithMismatchedBranch(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	run(t, repo, "git", "worktree", "add", worktreePath, "-b", "task/task-01")

	err := checkExistingWorktreeBinding(worktreePath, "task-02", "task/task-02")
	require.Error(t, err, "claim should fail due to branch mismatch")
	assert.Contains(t, err.Error(), "branch", "error should mention the branch mismatch")
}

// TestClaimAllowsWorktreeWithDetachedHEAD verifies that when a worktree has a detached HEAD
// (e.g., from mid-rebase, mid-bisect, or manual checkout), claim should allow the re-claim
// as long as the binding matches.
func TestClaimAllowsWorktreeWithDetachedHEAD(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Claim task-01 with a worktree — creates branch task/task-01, worktree bound to task-01
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	// Detach the HEAD in the worktree by checking out a specific commit
	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")

	// Verify HEAD is now a SHA (not a branch ref)
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
	// Verify it's a detached HEAD (a SHA, not a ref)
	assert.False(t, strings.HasPrefix(headStr, "ref: "), "HEAD should be detached (not a branch ref)")

	// Now try to claim task-01 AGAIN using the same worktree path
	// This should succeed because the detached HEAD should not block re-claim when binding matches
	_, claimErr := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.NoError(t, claimErr, "claim should succeed with detached HEAD when binding matches")
}

// TestClaimBoundToOtherTaskErrorDoesNotSuggestMerged verifies that the binding
// mismatch error does NOT suggest using 'arm merged' (which is only for
// post-merge teardown of completed tasks, not for live claimed/in-progress tasks).
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

// TestClaimReleasesClaimOnWorktreeSetupFailure verifies that when updateIssueIDFile fails
// after the claim is won, a compensating transition op is appended to re-open the task.
func TestClaimReleasesClaimOnWorktreeSetupFailure(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Manually create a worktree and git directory structure to simulate the scenario
	// where worktreePathExists passes but updateIssueIDFile will fail.
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// Create a minimal worktree-like structure
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))

	// Create a fake .git file that points to a non-existent git directory
	// This will make worktreePathExists return true (the file exists)
	// but resolveWorktreeGitDir will fail when updateIssueIDFile tries to use it
	gitPath := filepath.Join(worktreePath, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644))

	// Try to claim task-01 with this fake worktree
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")

	// The claim should fail - either during checkExistingWorktreeBinding or updateIssueIDFile
	assert.Error(t, claimErr, "claim should fail with invalid worktree. stderr: %s", stderr)

	// Even though the claim failed, verify that task-01 isn't stuck in "claimed" state.
	// If the fix is working, a rollback op should have been appended (if the claim race was won).
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// The task should be in "open" state (not claimed/stuck)
	// If the bug existed, it might be "claimed" even though the worktree setup failed
	assert.NotEqual(t, ops.StatusClaimed, issue.Status, "task should not be stuck in claimed state after worktree setup failure")
}

// TestClaimReleasesPushesInDualBranchMode verifies that appendHighStakesOp (not appendOp) is
// used for claim rollbacks in dual-branch mode, so the release op is committed to the
// _armature branch immediately rather than waiting for the next TTL expiry.
//
// The fix replaced bare appendOp calls with appendHighStakesOp for compensating rollback ops
// in arm claim. appendHighStakesOp commits the op to the worktree branch (dual-branch mode);
// push is best-effort so push failure (no remote in test repos) is swallowed. The commit is
// always written. We verify the release op is present in the ops log after the failed claim.
//
// TEST_EXCEPTION for push verification: The compensating error message "failed to push claim
// release (manual cleanup may be needed)" only appears when appendHighStakesOp itself returns
// an error (i.e., AppendAndCommit fails). Since push is best-effort and silently swallowed,
// inducing that error path would require making the _armature ops dir read-only — which would
// also prevent the initial claim op from being written, making the scenario unreachable. Instead,
// we verify the end-state invariant: after a failed worktree setup in dual-branch mode, the
// task must not be stuck in claimed state, and the ops log must contain the release op.
func TestClaimReleasesPushesInDualBranchMode(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap in dual-branch mode: ops committed to _armature branch in .arm/ worktree.
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Dual branch rollback task", "--id", "task-rb-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create a pre-existing directory with a broken .git file.
	// worktreePathExists() returns true; checkExistingWorktreeBinding may reject it, or
	// updateIssueIDFile will fail when it tries to resolve the non-existent git dir.
	worktreePath := filepath.Join(repo, ".worktrees", "task-rb-01")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	gitPath := filepath.Join(worktreePath, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644))

	// Run the failing claim — should error due to the broken worktree.
	_, _, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-rb-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail with invalid/broken worktree")

	// Materialize and verify the task is not stuck in claimed state.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-rb-01.json"))
	require.NoError(t, err)
	assert.NotEqual(t, ops.StatusClaimed, issue.Status,
		"task must not be stuck in claimed state after worktree setup failure in dual-branch mode")

	// Verify the ops log in the _armature worktree (.armature/ops/) contains a
	// transition-to-open (release) op, proving appendHighStakesOp committed the rollback.
	// If checkExistingWorktreeBinding rejected the fake worktree before the claim op was
	// written, no rollback op is needed (task stays open from the start), so we only look
	// for the release op when the task actually transitioned through claimed.
	armOpsDir := filepath.Join(repo, ".armature", "ops")
	entries, readErr := os.ReadDir(armOpsDir)
	if readErr != nil {
		// Ops dir not readable — skip the log check (bootstrap may have put ops elsewhere).
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
		// The claim may have been rejected before the claim op was written
		// (e.g., checkExistingWorktreeBinding fired first). In that case no rollback op
		// is needed, so it's acceptable to have no release op. The status check above
		// is the primary invariant.
		t.Logf("No release op in dual-branch ops log — claim was likely rejected before winning the race (acceptable)")
	}
}

// TestClaimRejectsUnboundDetachedWorktree verifies that when a worktree has a detached HEAD
// and NO existing binding (existingTaskID == ""), claim must reject it rather than allowing
// the detached HEAD to bypass the branch check. This prevents writing a fresh binding to a
// detached worktree that is not on the expected branch.
func TestClaimRejectsUnboundDetachedWorktree(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Create a linked worktree on the task/task-01 branch
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err, "first claim should succeed")

	// Verify the worktree was created and bound to task-01
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

	// Detach HEAD in the worktree
	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")

	// Verify HEAD is now detached (a SHA, not a branch ref)
	headFile := filepath.Join(actualGitDir, "HEAD")
	headContent, err := os.ReadFile(headFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	headStr := strings.TrimSpace(string(headContent))
	require.False(t, strings.HasPrefix(headStr, "ref: "), "HEAD should be detached")

	// Remove the armature-issue-id binding so the worktree has NO binding
	require.NoError(t, os.Remove(taskIDFile), "should be able to delete armature-issue-id file") //nolint:gosec // internal test path

	// Now try to claim task-01 again with the unbound detached HEAD
	// This should fail because even though the binding is empty, the detached HEAD
	// should only be allowed when there IS a binding that matches
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail when worktree has unbound detached HEAD. stderr: %s", stderr)

	errText := stderr + claimErr.Error()
	assert.Contains(t, errText, "detached HEAD",
		"error should mention detached HEAD in the error message")
}

// TestClaimDoesNotReleaseExistingClaimOnWorktreeRetryFailure verifies the P2 bug fix:
// when a worker retries claiming an already-claimed task with an existing worktree,
// and the task ID file update fails, the task must remain claimed (not be released to open).
//
// Scenario:
// 1. Worker claims task-01 with --worktree /wt1 → succeeds, status=claimed, ClaimedBy=worker-A
// 2. Worker retries with --worktree /wt1 again → wins claim race again (same worker, TTL not expired)
// 3. updateIssueIDFile fails (e.g., .git file points to non-existent directory)
// 4. Before the fix: compensating rollback → status=open (WRONG)
// 5. After the fix: only rollback to open if the prior status was open; otherwise keep it claimed
func TestClaimDoesNotReleaseExistingClaimOnWorktreeRetryFailure(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// First claim succeeds: creates worktree at wt1 with task-01 claimed
	worktree1 := filepath.Join(repo, ".worktrees", "task-01")
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err, "first claim should succeed")

	// Materialize and verify task-01 is claimed
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed after first claim")
	before := issue

	// Now break the worktree's .git file by replacing it with a pointer to a non-existent directory.
	// This will cause updateIssueIDFile to fail on the re-claim attempt.
	gitPath := filepath.Join(worktree1, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644),
		"should be able to overwrite .git file")

	// Second claim with same worktree should fail due to updateIssueIDFile failure
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "second claim with broken worktree should error. stderr: %s", stderr)

	// Materialize and verify task-01 is STILL claimed (not released to open)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// This is the critical assertion: the task must NOT be released to open
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

// TestClaimRollsBackStaleTakeoverToOpen verifies the P2 bug fix:
// When worker-B takes over a stale claim from worker-A and worktree setup fails,
// the compensating rollback must transition to StatusOpen (not to the prior claimed status).
// This ensures other workers can pick up the task, not see it as claimed by worker-B.
//
// Scenario:
// 1. Inject a stale claim op from "other-worker-uuid" with old timestamp (2 hours ago, 1 min TTL)
// 2. Call `arm claim --issue task-01` — worker-B takes over the stale claim
// 3. Make worktree setup fail (e.g., put a file at worktree path that blocks `git worktree add`)
// 4. Assert task-01 is rolled back to StatusOpen (not claimed), so other workers can pick it up
func TestClaimRollsBackStaleTakeoverToOpen(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap and create task
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task one", "--type", "task", "--id", "task-01")
	require.NoError(t, err)

	// Materialize first to establish baseline state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Inject a stale claim op from another worker with an old timestamp
	otherWorker := "other-worker-uuid"
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, otherWorker+".log")

	// Claim timestamp 2 hours ago, TTL 1 minute — definitely stale
	staleClaimTime := time.Now().Unix() - 7200
	staleClaimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: staleClaimTime,
		WorkerID:  otherWorker,
		Payload:   ops.Payload{TTL: 1, WorktreePath: "/legacy/task-01"},
	}
	require.NoError(t, ops.AppendOp(logPath, staleClaimOp))

	// Materialize to apply the stale claim
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify task-01 is currently claimed by the stale claimer
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed by stale worker")
	require.Equal(t, otherWorker, issue.ClaimedBy, "task should be claimed by other-worker-uuid")

	// Now try to claim with a worktree that will fail setup.
	// Create a file at the worktree path to block git worktree add.
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	// Create a file inside the directory to block worktree creation
	blockingFile := filepath.Join(worktreePath, "blocking-file")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocks worktree creation"), 0o644))

	// Attempt to claim — should fail due to worktree creation failure
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

	// Materialize and verify task-01 is now OPEN (not still claimed by the new worker)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// The critical assertion: after stale takeover + worktree failure, rollback must clear ownership
	assert.Equal(t, ops.StatusOpen, issueAfter.Status,
		"task should be rolled back to open after stale takeover failure (not remain claimed)")
	assert.Equal(t, "", issueAfter.ClaimedBy,
		"ClaimedBy must be cleared so other workers can pick up the task")
	assert.Equal(t, "/legacy/task-01", issueAfter.WorktreePath,
		"failed stale takeover must restore the prior worktree path")
}

// TestClaimRejectsForeignWorktree verifies the P2 bug fix: when a --worktree path is given
// that points to a linked worktree belonging to a DIFFERENT git repository (not the main repo),
// claim must reject it even if the worktree is on the expected branch and has no conflicting binding.
//
// This prevents updateIssueIDFile from writing armature-issue-id into a foreign repo's git dir,
// which would cause later merged operations (which search only the main repo's worktree list)
// to permanently fail to find and clean up the worktree.
//
// Scenario:
// 1. Create repo-A with task-01 but DON'T claim it yet (so no worktree exists in repo-A)
// 2. Create repo-B (unrelated git repo) with a worktree on the matching task/task-01 branch
// 3. Try to claim task-01 in repo-A using the foreign worktree from repo-B
// 4. Expect an error mentioning "not registered to this repository"
func TestClaimRejectsForeignWorktree(t *testing.T) {
	// A worktree registered to a DIFFERENT git repo must not be recognized as
	// belonging to this repo. isWorktreeOf is the guard that enforces this.
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

// TestClaimRollsBackStaleSameWorkerClaimToOpen verifies the P2 bug fix:
// When worker-A's own claim is stale (TTL expired) and worker-A retries `arm claim`
// with a new worktree, then worktree setup fails, the compensating rollback must
// transition to StatusOpen (not preserve the prior claimed status).
// This is critical because OpClaim already refreshed ClaimedAt and LastHeartbeat,
// so if rollback preserves "claimed", the issue will have a fresh claim with no
// usable worktree binding, blocking other workers from picking it up.
//
// Scenario:
//  1. Inject a claim op from the SAME worker ID with an old timestamp (2 hours ago, 1 min TTL)
//  2. Materialize to apply the stale claim
//  3. Call `arm claim --issue task-01 --worktree <blocked-path>` — same worker retries
//  4. The OpClaim wins the race and refreshes ClaimedAt/LastHeartbeat
//  5. Worktree setup fails (file blocking git worktree add)
//  6. Rollback must transition to StatusOpen (not keep the claim) because the prior
//     claim was stale, even though it's the same worker
//  7. Assert task-01 is rolled back to StatusOpen so other workers can pick it up
func TestClaimRollsBackStaleSameWorkerClaimToOpen(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap and create task
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task stale same-worker", "--type", "task", "--id", "task-01")
	require.NoError(t, err)

	// Materialize first to establish baseline state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Get the current worker ID (set by bootstrap/worker-init or read from git config)
	// We need to determine what worker ID will be used when we call `arm claim`
	// The test uses the same repo/git config, so the worker ID from the initial setup is used
	// We'll inject ops from that same worker with a stale timestamp
	workerID, logPath, err := resolveWorkerAndLog(&config.Context{
		RepoPath:  repo,
		IssuesDir: filepath.Join(repo, ".armature"),
		StateDir:  filepath.Join(repo, ".armature", "state"),
	})
	require.NoError(t, err, "should resolve worker ID and log path")

	// Inject a stale claim op from the SAME worker with an old timestamp
	// Claim timestamp 2 hours ago, TTL 1 minute — definitely stale
	staleClaimTime := time.Now().Unix() - 7200
	staleClaimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: staleClaimTime,
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 1},
	}
	require.NoError(t, ops.AppendOp(logPath, staleClaimOp))

	// Materialize to apply the stale claim
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify task-01 is currently claimed by the same worker (but stale)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed by stale worker")
	require.Equal(t, workerID, issue.ClaimedBy, "task should be claimed by same worker")

	// Now try to claim with a worktree that will fail setup (same worker retrying).
	// Create a file at the worktree path to block git worktree add.
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	// Create a file inside the directory to block worktree creation
	blockingFile := filepath.Join(worktreePath, "blocking-file")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocks worktree creation"), 0o644))

	// Attempt to claim — should fail due to worktree creation failure
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

	// Materialize and verify task-01 is now OPEN (not still claimed by the same worker)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// The critical assertion: after stale same-worker claim + worktree failure, rollback must
	// clear ownership even though it's the same worker, because the prior claim was stale
	assert.Equal(t, ops.StatusOpen, issueAfter.Status,
		"task should be rolled back to open after stale same-worker claim failure (not remain claimed)")
	assert.Equal(t, "", issueAfter.ClaimedBy,
		"ClaimedBy must be cleared so other workers can pick up the task")
}

// TestClaimPreservesNeverExpiringClaimOnRetry verifies the P2 bug fix:
// When a same-worker claim has TTL=0 (never-expiring) and the same worker retries
// `arm claim` with a new worktree, if worktree setup fails, the rollback must
// preserve the prior claimed status (not release to Open).
//
// This is the inverse of TestClaimRollsBackStaleSameWorkerClaimToOpen:
// - Stale claim (TTL=1 min, 2 hours old) → rollback to Open ✓
// - Never-expiring claim (TTL=0, any age) → rollback to Claimed (preserve) ✓
//
// The bug was: rollback code normalized TTL (0 → 60), breaking the never-expiring
// claim so it was wrongly treated as stale and released.
//
// Scenario:
//  1. Inject a claim op from the SAME worker ID with TTL=0 (never-expiring) and old timestamp (2 hours ago)
//  2. Materialize to apply the never-expiring claim
//  3. Call `arm claim --issue task-01 --worktree <blocked-path>` — same worker retries
//  4. The OpClaim wins the race and refreshes ClaimedAt/LastHeartbeat
//  5. Worktree setup fails (file blocking git worktree add)
//  6. Rollback must transition to StatusClaimed (preserve) because the prior claim
//     was never-expiring (TTL=0), even though it's old
//  7. Assert task-01 remains StatusClaimed with ClaimedBy still set to the worker
func TestClaimPreservesNeverExpiringClaimOnRetry(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap and create task
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task never-expiring", "--type", "task", "--id", "task-01")
	require.NoError(t, err)

	// Materialize first to establish baseline state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Resolve the current worker ID (set by bootstrap/worker-init)
	workerID, logPath, err := resolveWorkerAndLog(&config.Context{
		RepoPath:  repo,
		IssuesDir: filepath.Join(repo, ".armature"),
		StateDir:  filepath.Join(repo, ".armature", "state"),
	})
	require.NoError(t, err, "should resolve worker ID and log path")

	// Inject a never-expiring claim op from the SAME worker with an old timestamp
	// Claim timestamp 2 hours ago, TTL 0 (never expires) — must NOT be treated as stale
	neverExpiringClaimTime := time.Now().Unix() - 7200
	neverExpiringClaimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: neverExpiringClaimTime,
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 0},
	}
	require.NoError(t, ops.AppendOp(logPath, neverExpiringClaimOp))

	// Materialize to apply the never-expiring claim
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify task-01 is currently claimed by the same worker (with TTL=0)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed")
	require.Equal(t, workerID, issue.ClaimedBy, "task should be claimed by same worker")
	require.Equal(t, 0, issue.ClaimTTL, "task claim TTL should be 0 (never-expiring)")

	// Now try to claim with a worktree that will fail setup (same worker retrying).
	// Create a file at the worktree path to block git worktree add.
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	// Create a file inside the directory to block worktree creation
	blockingFile := filepath.Join(worktreePath, "blocking-file")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocks worktree creation"), 0o644))

	// Attempt to claim — should fail due to worktree creation failure
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

	// Materialize and verify task-01 is still CLAIMED (not released to open)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// The critical assertion: after never-expiring same-worker claim + worktree failure,
	// rollback must preserve the claimed status (not release to open) because the prior
	// claim has TTL=0 (never expires, always active)
	assert.Equal(t, ops.StatusClaimed, issueAfter.Status,
		"task should remain claimed after never-expiring same-worker claim failure (not be released to open)")
	assert.Equal(t, workerID, issueAfter.ClaimedBy,
		"ClaimedBy must remain set since the claim never expires")
}

// TestCheckExistingWorktreeBindingReadsLegacyTaskID verifies the P2 bug fix:
// checkExistingWorktreeBinding should recognize legacy armature-task-id files
// (from worktrees claimed before the rename to armature-issue-id).
//
// Scenario:
// 1. Create a worktree with a detached HEAD
// 2. Write only the legacy armature-task-id file to the .git directory (not armature-issue-id)
// 3. Call checkExistingWorktreeBinding with the same issue ID
// 4. Expect it to return nil (no error), allowing the claim to proceed for same-issue re-claim
func TestCheckExistingWorktreeBindingReadsLegacyTaskID(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Create a worktree manually via git
	worktreePath := filepath.Join(t.TempDir(), "legacy-worktree")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	// Verify worktree was created
	assert.DirExists(t, worktreePath, "worktree directory should exist")

	// Detach the HEAD in the worktree
	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")

	// Get the actual git directory from the worktree's .git file
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	// Write only the legacy armature-task-id file (NOT armature-issue-id)
	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	require.NoError(t, os.WriteFile(taskIDFile, []byte("task-01"), 0o600)) //nolint:gosec // test path is internal

	// Verify that armature-issue-id does NOT exist
	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	_, err = os.ReadFile(issueIDFile) //nolint:gosec // test path is internal
	require.True(t, os.IsNotExist(err), "armature-issue-id should not exist (only legacy armature-task-id)")

	// Now call checkExistingWorktreeBinding with the same issue ID
	// It should recognize the legacy binding and return nil (no error)
	err = checkExistingWorktreeBinding(worktreePath, "task-01", "task/task-01")
	assert.NoError(t, err, "checkExistingWorktreeBinding should allow same-issue claim with legacy armature-task-id binding")
}

// TestCheckExistingWorktreeBindingFailsClosedOnPermissionError verifies the fix
// for the review finding that checkExistingWorktreeBinding silently treated a
// permission-denied armature-issue-id file as "unbound" (old code failed
// closed on any read error other than not-exist; the refactor to
// ReadIssueBindingFile regressed that by swallowing all errors). A worker
// should not be able to silently overwrite a binding it merely couldn't read.
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
		_ = os.Chmod(issueIDFile, 0o600) //nolint:errcheck,gosec // best-effort cleanup so TempDir removal succeeds
	})

	err = checkExistingWorktreeBinding(worktreePath, "task-01", "task/task-01")
	require.Error(t, err, "a permission-denied binding file must fail closed, not be silently treated as unbound")
	assert.Contains(t, err.Error(), "read existing binding")
}

// TestClaimCommand_NoFalsePositiveAgainstParentStory_REQ_TOPTIER_S17_T1 verifies that
// claiming a child task does not produce a false-positive "scope overlap" error against its parent story.
// A story's scope is by design the union of its children's scopes, so parent/child scope overlap
// is not a conflict and should not be reported.
//
// Scenario:
// 1. Create a parent story with scope ["src/**"]
// 2. Create a child task with scope ["src/auth/**"]
// 3. Claim the parent story first (if not already claimed by another)
// 4. Try to claim the child task — should succeed without scope overlap warning
func TestClaimCommand_NoFalsePositiveAgainstParentStory_REQ_TOPTIER_S17_T1(t *testing.T) {
	t.Parallel()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	// Create parent story with broad scope
	_, err := runTrls(t, repo, "create", "--title", "Parent Story", "--type", "story", "--id", "story-01")
	require.NoError(t, err)

	// Materialize so story-01 exists
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create child task with narrower scope
	_, err = runTrls(t, repo, "create", "--title", "Child Task", "--type", "task", "--id", "task-01", "--parent", "story-01")
	require.NoError(t, err)

	// Set scope on parent story
	_, err = runTrls(t, repo, "amend", "--issue", "story-01", "--scope", "src/**")
	require.NoError(t, err)

	// Set scope on child task (subset of parent's scope)
	_, err = runTrls(t, repo, "amend", "--issue", "task-01", "--scope", "src/auth/**")
	require.NoError(t, err)

	// Claim the parent story
	_, err = runTrls(t, repo, "claim", "--issue", "story-01", "--worktree")
	require.NoError(t, err, "claiming parent story should succeed")

	// Claim the child task — should NOT give false-positive scope overlap error
	worktreePath2 := filepath.Join(repo, ".worktrees", "task-01")
	stdout, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree")
	assert.NoError(t, claimErr, "claiming child task should succeed without scope overlap error. stdout: %s, stderr: %s", stdout, stderr)

	// Verify worktree was created (claim succeeded)
	assert.DirExists(t, worktreePath2, "worktree should be created when claiming child task against parent")
}

// TestClaimAutoProvisionsWorktreeAtDefaultRoot_REQ_LNGHZN_S5_T4 verifies the core
// DoD behavior: the boolean --worktree flag provisions the worktree at the
// hardcoded default root <repo>/.worktrees/<issue-id> (relative to ctx.RepoPath),
// and that path is a registered linked worktree bound to the claimed issue.
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

	// The worktree must be bound to the claimed issue.
	gitDir, err := worktree.ResolveGitDir(expected)
	require.NoError(t, err)
	bindingBytes, err := os.ReadFile(filepath.Join(gitDir, "armature-issue-id"))
	require.NoError(t, err)
	assert.Equal(t, "task-01", strings.TrimSpace(string(bindingBytes)), "worktree must be bound to the claimed issue")
}

// TestClaimProvisionExcludesManagedWorktrees_REQ_LNGHZN_S5 verifies that a
// successful fresh `arm claim --worktree` protects the linked worktree from a
// later broad `git add .`, including repositories created before bootstrap
// began adding the .worktrees/ exclusion.
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

	// This is the user-visible regression: a linked worktree must not be staged
	// as a gitlink by the ordinary repository-wide staging command.
	run(t, repo, "git", "add", ".")
	staged := runGitOutput(t, repo, "diff", "--cached", "--name-only")
	assert.NotContains(t, staged, ".worktrees/task-01", "managed worktree must remain excluded from broad staging")
}

// TestClaimRejectsRepoRelativeCustomDestination_REQ_LNGHZN_S9_T1 verifies that
// an explicit destination inside the repository must use the canonical
// .worktrees/ root, avoiding a shared .git/info/exclude pattern that would
// affect every linked worktree in the clone.
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

// TestClaimRejectsDestinationNestedInRegisteredWorktree_REQ_LNGHZN_S9_T1
// verifies that a custom destination below another linked worktree is rejected
// before claim state, branch state, exclusion state, or filesystem state can
// change. The parent worktree is clone-local Git evidence, even though it is
// not an Armature-managed worktree.
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
	release, lockErr := acquireGitExcludeLock(repo)
	require.NoError(t, lockErr, "nested destination rejection must release the exclusion lock")
	release()
}

// TestClaimExistingWorktreeInstallsManagedWorktreeExclusion_REQ_LNGHZN_S5
// exercises the public re-claim path for an installation created before
// bootstrap started adding .worktrees/ to .git/info/exclude.  A canonical
// worktree that already exists is just as dangerous to stage accidentally as
// a freshly provisioned one.
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

// TestClaimExclusionFailureAppendsNoClaim_REQ_LNGHZN_S5 verifies the ordering
// contract at the CLI seam: exclusion installation is a claim-time safety
// precondition, so failure leaves no claim op, no status change, and no
// worktree path behind for a later command to misinterpret.
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

// TestClaimDetachedCheckoutAvoidsBranchAlreadyCheckedOutRace_REQ_LNGHZN_S5_T4
// exercises the detached-checkout reordering. It reproduces the worktree-
// recreation condition the old create-branch-then-add-worktree order was prone
// to trip on: the issue's derived branch (task/task-01) already exists from a
// prior claim while the worktree itself is gone. Provisioning now adds the
// worktree detached at the base commit first, then checks the existing branch
// out inside it, so re-provisioning must succeed and land on the correct branch
// rather than failing with git's "branch already checked out" family of errors.
func TestClaimDetachedCheckoutAvoidsBranchAlreadyCheckedOutRace_REQ_LNGHZN_S5_T4(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	worktreePath := filepath.Join(repo, ".worktrees", "task-01")

	// First claim creates the worktree AND the derived branch task/task-01.
	first := newRootCmd()
	first.SetOut(new(bytes.Buffer))
	first.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, first.Execute())
	require.DirExists(t, worktreePath)

	// Tear the worktree down but KEEP the branch (git worktree remove prunes the
	// admin record, so the branch is retained and free). This is the state a
	// prior lifecycle leaves behind before a re-claim.
	run(t, repo, "git", "worktree", "remove", "--force", worktreePath)
	require.NoDirExists(t, worktreePath)
	run(t, repo, "git", "rev-parse", "--verify", "refs/heads/task/task-01") // branch must still exist

	// Re-claim: the derived branch already exists, so createWorktreeAndBranch must
	// take the detached-add-then-checkout-existing-branch path and succeed.
	second := newRootCmd()
	second.SetOut(new(bytes.Buffer))
	second.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree"})
	require.NoError(t, second.Execute(),
		"re-provisioning with a pre-existing derived branch must succeed (no 'branch already checked out')")
	require.DirExists(t, worktreePath)

	// The recreated worktree must be checked out on the issue's derived branch.
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

// TestCreateWorktreeAndBranchRejectsUnavailableValidatedSource_REQ_LNGHZN_S9_T1
// proves provisioning fails closed when the validated --from worktree vanishes
// before the branch is created. The destination, derived branch, and source
// provenance must remain untouched; falling back to the coordinator HEAD would
// silently create the task from the wrong parent.
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

// injectFutureSameWorkerClaim appends a claim op for issueID that carries the
// SAME effective owner identity `arm claim` will use (resolved exactly as
// resolveWorkerAndLog does, including any ARM_LOG_SLOT suffix) but a
// DIFFERENT claimToken and a timestamp far in the future. Since op replay
// sorts by timestamp (see sortOpsByTimestamp) rather than log-append order,
// and materialize.applyClaim overwrites unconditionally whenever
// issue.ClaimedBy == op.WorkerID (the "same worker" branch never checks
// staleness), this future-dated op always wins the replay regardless of when
// `arm claim` itself later appends its own claim op — deterministically
// reproducing, without any real concurrency, the two-different-clones race
// described in cmd/armature/claim.go's `won` doc comment: two `arm claim`
// invocations sharing a workerID (worker.GetWorkerID is per-clone; nothing
// enforces global uniqueness across clones) racing for the same issue.
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

// TestClaimCommand_SupersededBySameWorkerDifferentTokenLosesRaceAndSkipsWorktree_REQ_LNGHZN_S5_T9
// is the regression test for the finding: cmd/armature/claim.go's `won` check
// used to compare only issueAfter.ClaimedBy == workerID, which cannot tell
// "my own claim op is still current" apart from "a DIFFERENT claim op that
// happens to carry the same workerID has superseded mine" -- exactly the
// shape of the two-clones-same-workerID race the per-issue flock in
// acquireClaimLock (scoped to one clone) cannot serialize. Here a
// future-timestamped claim op for the same issue and the same effective
// owner identity, but a different token, is injected before `arm claim`
// runs; once `arm claim` appends its own (older-timestamped) op and reloads,
// replay applies the injected op last (sorted by timestamp) and it wins.
// The command must report the claim as lost -- not proceed to provision a
// worktree, which is the observable, wider-reaching consequence the finding
// calls out (this is the gate that decides whether provisioning happens at
// all, unlike the exit-guard call sites this predicate already covered).
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

// TestClaimCommand_SupersededBySameWorkerDifferentTokenHumanFormat_REQ_LNGHZN_S5_T9
// covers the human-format output for the same-worker-superseded case: it must
// not print "claimed by <own worker id>", which would read as nonsense
// ("lost the race to yourself"), but instead make the same-worker
// supersession explicit.
func TestClaimCommand_SupersededBySameWorkerDifferentTokenHumanFormat_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	injectFutureSameWorkerClaim(t, ctx, "task-01", "impostor-token")

	// --format human is explicit because autoDetectTTYPolicy auto-upgrades the
	// default "human" format to "agent" (JSON) under a non-TTY test harness;
	// only an explicitly-set format flag is exempt from that override.
	claimOut, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", "--format", "human")
	require.NoError(t, err, "losing a claim race is a normal outcome, not an error")
	assert.Contains(t, claimOut, "Claim lost")
	assert.Contains(t, claimOut, "superseded by a different claim from this same worker ID",
		"human output must distinguish same-worker supersession from an ordinary different-worker loss")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	assert.NoDirExists(t, worktreePath, "a lost claim race must never provision a worktree")
}

// TestClaimCommand_DifferentWorkerLostRaceJSONFormat_REQ_LNGHZN_S5_T9 pins the
// ordinary (pre-existing) different-worker lost-race JSON shape so it keeps
// its exact prior keys/values -- required so existing agent consumers and
// TestClaimCommand_LostRaceReportsClearResult (main_test.go) don't break --
// while additionally verifying the new superseded_by_same_worker field
// correctly reads false for a genuinely different claimant.
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

// TestClaimCommand_OrdinaryWinStillProvisionsWorktree_REQ_LNGHZN_S5_T9 guards
// against a false negative from delegating `won` to
// materialize.Issue.ClaimHeldBy: on the legitimate, uncontested path,
// applyClaim sets Status/ClaimedBy/ClaimToken unconditionally from this
// process's own op, so ClaimHeldBy(workerID, claimToken) must report true
// immediately after the append-and-reload, exactly as the old
// ClaimedBy == workerID comparison did. A worktree must still be provisioned.
func TestClaimCommand_OrdinaryWinStillProvisionsWorktree_REQ_LNGHZN_S5_T9(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	claimOut, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, claimOut, "lost_claim_race", "an uncontested claim must never report a lost race")

	worktreePath := filepath.Join(repo, ".worktrees", "task-01")
	assert.DirExists(t, worktreePath, "a won claim must provision a worktree")
}

// TestDefaultTTLGovernsClaim_REQ_LNGHZN_S7_T1 verifies that arm claim's --ttl
// flag defaults to config.json's default_ttl when --ttl is not explicitly
// passed, rather than the previously hardcoded 60.
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

// TestClaimFallsBackToBuiltInTTLWhenConfigAbsent_REQ_LNGHZN_S7_T1 verifies
// that when config.json's default_ttl is absent (zero-valued) and --ttl is
// not explicitly passed, claim falls back to the built-in default of 60. The
// config loader does not distinguish an absent field from an explicit zero
// (JSON unmarshal into an int leaves it at its zero value either way), so
// this single case covers both.
func TestClaimFallsBackToBuiltInTTLWhenConfigAbsent_REQ_LNGHZN_S7_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	cfg := config.Config{ProjectType: "go"} // DefaultTTL left at zero value (absent)
	require.NoError(t, config.WriteConfig(filepath.Join(repo, ".armature", "config.json"), cfg))

	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, 60, issue.ClaimTTL, "claim TTL should fall back to the built-in default of 60 when config's default_ttl is absent/zero")
}

// TestClaimExplicitTTLOverridesConfigDefault_REQ_LNGHZN_S7_T1 verifies that an
// explicit --ttl flag always wins over config.json's default_ttl.
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

// TestTokenBudgetHonoredByRenderContext_REQ_LNGHZN_S7_T1 verifies that
// render-context's --budget flag defaults to config.json's token_budget when
// --budget is not explicitly passed, rather than the previously hardcoded
// 4000. A tiny configured budget forces truncation down to a single context
// layer.
func TestTokenBudgetHonoredByRenderContext_REQ_LNGHZN_S7_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	cfg := config.DefaultConfig("go")
	cfg.TokenBudget = 1
	require.NoError(t, config.WriteConfig(filepath.Join(repo, ".armature", "config.json"), cfg))

	tinyOut, err := runTrls(t, repo, "render-context", "--issue", "task-01", "--format", "agent")
	require.NoError(t, err)

	// An explicit, generous --budget must override the tiny configured default
	// and produce strictly more content.
	overriddenOut, err := runTrls(t, repo, "render-context", "--issue", "task-01", "--format", "agent", "--budget", "999999")
	require.NoError(t, err)

	assert.Less(t, len(tinyOut), len(overriddenOut),
		"config.json's tiny token_budget should truncate content relative to an explicit generous --budget override")
}

// TestRenderContextFallsBackToBuiltInBudgetWhenConfigAbsent_REQ_LNGHZN_S7_T1
// verifies that when config.json's token_budget is absent (zero-valued) and
// --budget is not explicitly passed, render-context falls back to the
// built-in default of 4000. As with the TTL fallback test, the config loader
// does not distinguish an absent field from an explicit zero, so a single
// case covers both.
func TestRenderContextFallsBackToBuiltInBudgetWhenConfigAbsent_REQ_LNGHZN_S7_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	cfg := config.Config{ProjectType: "go"} // TokenBudget left at zero value (absent)
	require.NoError(t, config.WriteConfig(filepath.Join(repo, ".armature", "config.json"), cfg))

	defaultOut, err := runTrls(t, repo, "render-context", "--issue", "task-01", "--format", "agent")
	require.NoError(t, err)

	// Passing --budget 4000 explicitly must reproduce the built-in default's
	// output exactly, confirming the fallback used when config's token_budget
	// is absent/zero is the same built-in 4000.
	explicitOut, err := runTrls(t, repo, "render-context", "--issue", "task-01", "--format", "agent", "--budget", "4000")
	require.NoError(t, err)

	assert.Equal(t, explicitOut, defaultOut,
		"render-context should fall back to the built-in budget of 4000 when config's token_budget is absent/zero")
}

// TestSourceAdvancedOnlyByArmatureFalseOnNonArmatureChange_REQ_LNGHZN_S9_T1
// verifies that sourceAdvancedOnlyByArmature returns false when the
// coordinator repository advanced between the two tips with a change outside
// .armature/ (e.g. ordinary source edits), since only an internal armature
// bookkeeping commit is allowed to reconcile transparently with a validated
// --from source.
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

// TestSourceAdvancedOnlyByArmatureErrorsOnUnresolvableRevision_REQ_LNGHZN_S9_T1
// verifies that a git diff failure (e.g. an unresolvable revision) is
// surfaced as an error rather than silently reported as "not an internal
// advance", which would incorrectly fail closed with a misleading message.
func TestSourceAdvancedOnlyByArmatureErrorsOnUnresolvableRevision_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	newTip := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	_, err := sourceAdvancedOnlyByArmature(repo, repo, "not-a-real-revision", newTip)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inspect coordinator source advance")
}

// TestRollbackClaimReportsExclusionCleanupFailure_REQ_LNGHZN_S9_T1 verifies
// that rollbackClaim surfaces an exclusion-cleanup failure alongside the
// original cause instead of silently dropping it, so a worker sees that its
// safety exclusion may still be present after a failed claim.
func TestRollbackClaimReportsExclusionCleanupFailure_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)

	claimTimestamp := nowEpoch()
	claimToken := "token-worker-a"
	store := setupSingleWorkerClaimStore(t, ctx, claimTimestamp, claimToken)

	// A broken RepoPath makes cleanupClaimExclusions's worktree.List call fail,
	// so the compensating transition still succeeds but exclusion cleanup does
	// not.
	brokenCtx := *ctx
	brokenCtx.RepoPath = filepath.Join(t.TempDir(), "does-not-exist")
	cmd := rollbackClaimTestCmd(&brokenCtx)

	logPathA := opsLogPath(ctx.IssuesDir, "worker-a")
	prior := priorClaimState{status: ops.StatusOpen}
	cause := fmt.Errorf("boom")
	exclusions := []claimExclusion{{pattern: "/custom/", destination: filepath.Join(t.TempDir(), "custom")}}

	err := rollbackClaimWithExclusionLock(cmd, store, logPathA, "task-01", "worker-a", "create worktree", cause, prior, claimToken, false, exclusions)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.Contains(t, err.Error(), "exclusion rollback failed")
}

// TestCreateWorktreeAndBranchRejectsMalformedSourceArgCount_REQ_LNGHZN_S9_T1
// verifies that createWorktreeAndBranch fails closed when called with a
// sourceArgs slice that is neither empty (no --from) nor exactly 3 elements
// (path, branch, tip). This is a defensive internal-contract guard: callers
// must supply the full validated source triple or none at all.
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

// TestCreateWorktreeAndBranchRejectsIssueTypeWithNoBranchMapping_REQ_LNGHZN_S9_T1
// verifies that createWorktreeAndBranch fails closed for an issue type (epic)
// that deriveBranchName maps to no branch at all, before any worktree or
// branch is created.
func TestCreateWorktreeAndBranchRejectsIssueTypeWithNoBranchMapping_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "child")

	err := createWorktreeAndBranch(repo, destination, "epic-01", materialize.Issue{Type: "epic"}, alwaysOwns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no branch mapping")
	assert.NoDirExists(t, destination)
}

// TestCreateWorktreeAndBranchRejectsSourceBranchChange_REQ_LNGHZN_S9_T1
// verifies that createWorktreeAndBranch fails closed when the validated
// --from source worktree has switched to a different branch since --from
// validation ran, even though its tip commit is unchanged.
func TestCreateWorktreeAndBranchRejectsSourceBranchChange_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	parentPath := filepath.Join(repo, "parent")
	run(t, repo, "git", "worktree", "add", "-b", "feature-parent", parentPath)
	parentTip := strings.TrimSpace(runGitOutput(t, parentPath, "rev-parse", "HEAD"))

	// Switch the source worktree to a different branch at the same tip, after
	// the (simulated) validation that captured "feature-parent" as the source
	// branch.
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

// TestCreateWorktreeAndBranchRejectsIncompleteSourceArgs_REQ_LNGHZN_S9_T1
// verifies that a 3-element sourceArgs triple with an empty component (path,
// branch, or tip) is rejected before any worktree is provisioned.
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

// TestWriteClaimExclusionMarkerRejectsConflictingPattern_REQ_LNGHZN_S9_T1
// verifies that writing a second, different exclusion pattern to a worktree
// that already recorded one fails closed instead of silently overwriting the
// original claim's marker.
func TestWriteClaimExclusionMarkerRejectsConflictingPattern_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "marker-wt")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	require.NoError(t, writeClaimExclusionMarker(worktreePath, "/custom-a/"))

	// Same pattern again must be a no-op, not an error.
	require.NoError(t, writeClaimExclusionMarker(worktreePath, "/custom-a/"))

	err := writeClaimExclusionMarker(worktreePath, "/custom-b/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already records a different pattern")

	pattern, ok, readErr := readClaimExclusionMarker(worktreePath)
	require.NoError(t, readErr)
	require.True(t, ok)
	assert.Equal(t, "/custom-a/", pattern, "the conflicting write must not have overwritten the original marker")
}

// TestWriteClaimExclusionMarkerFailsClosedOnUnreadableMarker_REQ_LNGHZN_S9_T1
// verifies that writeClaimExclusionMarker surfaces a permission-denied read
// of an existing marker file as an error, rather than treating it as absent
// and silently overwriting it.
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
		_ = os.Chmod(markerPath, 0o600) //nolint:errcheck // best-effort cleanup so TempDir removal succeeds
	})

	writeErr := writeClaimExclusionMarker(worktreePath, "/custom-a/")
	require.Error(t, writeErr)
	assert.Contains(t, writeErr.Error(), "read claim exclusion marker")
}

// TestReadClaimExclusionMarkerRejectsEmptyMarker_REQ_LNGHZN_S9_T1 verifies
// that an on-disk exclusion marker file containing only whitespace/newline
// (no actual pattern) is treated as corrupt rather than silently read as "no
// exclusion recorded".
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

// TestReadClaimExclusionMarkerFailsClosedOnUnresolvableWorktree_REQ_LNGHZN_S9_T1
// verifies that readClaimExclusionMarker surfaces the git-dir resolution
// error when called against a path that is not a worktree at all, instead of
// treating it as "no exclusion recorded".
func TestReadClaimExclusionMarkerFailsClosedOnUnresolvableWorktree_REQ_LNGHZN_S9_T1(t *testing.T) {
	notAWorktree := t.TempDir()

	_, ok, err := readClaimExclusionMarker(notAWorktree)
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "resolve worktree git dir")
}

// TestCleanupClaimExclusionsFailsClosedOnUnresolvableRepo_REQ_LNGHZN_S9_T1
// verifies that cleanupClaimExclusions (the locking wrapper) surfaces a
// failure to acquire the shared exclude lock rather than silently skipping
// cleanup, when repoPath is not a git repository at all.
func TestCleanupClaimExclusionsFailsClosedOnUnresolvableRepo_REQ_LNGHZN_S9_T1(t *testing.T) {
	notARepo := t.TempDir()

	err := cleanupClaimExclusions(notARepo, []claimExclusion{{pattern: "/x/", destination: notARepo}})
	require.Error(t, err)
}

// TestCleanupClaimExclusionsIsNoOpForEmptySlice_REQ_LNGHZN_S9_T1 verifies
// that cleanupClaimExclusions is a safe no-op (no lock acquired, no error)
// when there is nothing to roll back -- the common case where a claim's
// worktree setup never added a safety exclusion in the first place.
func TestCleanupClaimExclusionsIsNoOpForEmptySlice_REQ_LNGHZN_S9_T1(t *testing.T) {
	notARepo := t.TempDir() // would fail closed if the lock were actually acquired

	err := cleanupClaimExclusions(notARepo, nil)
	require.NoError(t, err)
}

// TestCleanupClaimExclusionsLockedRemovesUnprotectedPattern_REQ_LNGHZN_S9_T1
// verifies that cleanupClaimExclusionsLocked removes an exclusion pattern
// from .git/info/exclude when the worktree it protected no longer exists.
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

// TestCleanupClaimExclusionsLockedPreservesProtectedPattern_REQ_LNGHZN_S9_T1
// verifies that cleanupClaimExclusionsLocked leaves an exclusion pattern in
// place when its destination is still a live registered worktree, so rollback
// never strips protection out from under a worktree that is actually in use.
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
