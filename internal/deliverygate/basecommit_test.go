package deliverygate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/worktree"
)

func writeIssueBindingFile(t *testing.T, worktreePath, issueID string) {
	t.Helper()
	gitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "armature-issue-id"), []byte(issueID), 0o600))
}

func TestVerifyIssueWorktreeBinding_REQ_LNGHZN_S4_T3(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	err := VerifyIssueWorktreeBinding(tmpDir, "issue-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not bound to any issue")

	writeIssueBindingFile(t, tmpDir, "issue-other")
	err = VerifyIssueWorktreeBinding(tmpDir, "issue-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue-other")

	writeIssueBindingFile(t, tmpDir, "issue-1")
	assert.NoError(t, VerifyIssueWorktreeBinding(tmpDir, "issue-1"))
}

func TestVerifyIssueBranchBinding_REQ_LNGHZN_S4_T3(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	assert.NoError(t, VerifyIssueBranchBinding(tmpDir, "issue-1", "epic", ""))

	runGit(t, tmpDir, "checkout", "-b", "scratch")
	err := VerifyIssueBranchBinding(tmpDir, "issue-1", "task", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scratch")

	runGit(t, tmpDir, "checkout", "-b", "task/issue-1")
	assert.NoError(t, VerifyIssueBranchBinding(tmpDir, "issue-1", "task", ""))
}

func TestVerifyIssueBranchBinding_FailsClosedWhenClaimedAndNoRecordOrMapping_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	err := VerifyIssueBranchBinding(tmpDir, "issue-1", "epic", "worker-a")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue-1")
	assert.Contains(t, err.Error(), "worker-a")

	assert.NoError(t, VerifyIssueBranchBinding(tmpDir, "issue-1", "epic", ""))
}

func TestRecordedBaseCommit_REQ_LNGHZN_S4_T3(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")
	sha := getHeadSHA(t, tmpDir)

	_, err := RecordedBaseCommit(tmpDir)
	assert.Error(t, err)

	gitDir, err := worktree.ResolveGitDir(tmpDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, BaseCommitFileName), []byte(sha+"\n"), 0o600))

	got, err := RecordedBaseCommit(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, sha, got)
}

func TestVerifyIssueBranchBinding_FailsClosedWhenAmendedTypeHasNoBranchMapping_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	gitDir, err := worktree.ResolveGitDir(tmpDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, ClaimedBranchFileName), []byte("task/issue-1"), 0o600))

	runGit(t, tmpDir, "checkout", "-b", "scratch")

	err = VerifyIssueBranchBinding(tmpDir, "issue-1", "epic", "worker-a")
	assert.Error(t, err, "must fail closed: recorded claimed branch task/issue-1 does not match current branch scratch")
	assert.Contains(t, err.Error(), "task/issue-1")
	assert.Contains(t, err.Error(), "scratch")
}

func TestRecordedClaimedBranch_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	branch, found, err := RecordedClaimedBranch(tmpDir)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, branch)

	gitDir, err := worktree.ResolveGitDir(tmpDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, ClaimedBranchFileName), []byte("task/issue-1\n"), 0o600))

	branch, found, err = RecordedClaimedBranch(tmpDir)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "task/issue-1", branch)
}

func TestDynamicBaseCommit_REQ_LNGHZN_S4_T3(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")
	baseSHA := getHeadSHA(t, tmpDir)
	runGit(t, tmpDir, "checkout", "-b", "main-parent")
	runGit(t, tmpDir, "checkout", "-b", "task/issue-1")
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "task work")

	git := adapters.New(tmpDir)

	_, err := dynamicBaseCommit(git)
	assert.Error(t, err)

	require.NoError(t, git.SetGitConfig(ParentBranchConfigKey("task/issue-1"), "HEAD"))
	_, err = dynamicBaseCommit(git)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "literal value")
	require.NoError(t, git.UnsetGitConfig(ParentBranchConfigKey("task/issue-1")))

	require.NoError(t, git.SetGitConfig(ParentBranchConfigKey("task/issue-1"), "main-parent"))
	got, err := dynamicBaseCommit(git)
	require.NoError(t, err)
	assert.Equal(t, baseSHA, got)
}

func TestGatedBaseCommit_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")
	baseSHA := getHeadSHA(t, tmpDir)
	runGit(t, tmpDir, "checkout", "-b", "task/issue-1")
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "task work")

	git := adapters.New(tmpDir)

	_, err := GatedBaseCommit(tmpDir, "issue-1", git)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue-1")

	gitDir, err := worktree.ResolveGitDir(tmpDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, BaseCommitFileName), []byte(baseSHA), 0o600))

	got, err := GatedBaseCommit(tmpDir, "issue-1", git)
	require.NoError(t, err)
	assert.Equal(t, baseSHA, got)
}
