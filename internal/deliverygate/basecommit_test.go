package deliverygate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/adapters"
)

// writeIssueBindingFile writes the armature-issue-id marker file into the
// worktree's resolved git directory, mirroring updateIssueIDFile in
// cmd/armature/claim.go, so tests can simulate a claimed worktree without
// importing the (unimportable) cmd/armature main package.
func writeIssueBindingFile(t *testing.T, worktreePath, issueID string) {
	t.Helper()
	gitDir, err := ResolveWorktreeGitDir(worktreePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "armature-issue-id"), []byte(issueID), 0o600))
}

// TestVerifyIssueWorktreeBinding_REQ_LNGHZN_S4_T3 proves the read-side
// base-commit/branch-binding resolution logic (formerly unexported in
// cmd/armature/transition.go) is independently testable via a plain package
// import now that it lives in internal/deliverygate.
func TestVerifyIssueWorktreeBinding_REQ_LNGHZN_S4_T3(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	// No marker file at all: fail closed.
	err := VerifyIssueWorktreeBinding(tmpDir, "issue-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not bound to any issue")

	// Marker file present but for a different issue: fail closed.
	writeIssueBindingFile(t, tmpDir, "issue-other")
	err = VerifyIssueWorktreeBinding(tmpDir, "issue-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue-other")

	// Marker file matches: pass.
	writeIssueBindingFile(t, tmpDir, "issue-1")
	assert.NoError(t, VerifyIssueWorktreeBinding(tmpDir, "issue-1"))
}

// TestVerifyIssueBranchBinding_REQ_LNGHZN_S4_T3 verifies branch-binding
// resolution directly via package import.
func TestVerifyIssueBranchBinding_REQ_LNGHZN_S4_T3(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	// issueType with no branch mapping skips the check entirely.
	assert.NoError(t, VerifyIssueBranchBinding(tmpDir, "issue-1", "epic"))

	// On an unrelated branch: fail closed.
	runGit(t, tmpDir, "checkout", "-b", "scratch")
	err := VerifyIssueBranchBinding(tmpDir, "issue-1", "task")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scratch")

	// On the expected task branch: pass.
	runGit(t, tmpDir, "checkout", "-b", "task/issue-1")
	assert.NoError(t, VerifyIssueBranchBinding(tmpDir, "issue-1", "task"))
}

// TestRecordedBaseCommit_REQ_LNGHZN_S4_T3 verifies the claim-time recorded
// SHA is read back correctly, and that a missing file surfaces an error so
// callers can fall through to the next tier.
func TestRecordedBaseCommit_REQ_LNGHZN_S4_T3(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")
	sha := getHeadSHA(t, tmpDir)

	// Absent: error.
	_, err := RecordedBaseCommit(tmpDir)
	assert.Error(t, err)

	// Present: returns the recorded SHA.
	gitDir, err := ResolveWorktreeGitDir(tmpDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, BaseCommitFileName), []byte(sha+"\n"), 0o600))

	got, err := RecordedBaseCommit(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, sha, got)
}

// TestDynamicBaseCommit_REQ_LNGHZN_S4_T3 verifies the dynamic merge-base
// recomputation against a recorded parent-branch git config, including the
// stale-"HEAD"-literal self-healing guard.
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

	// No parent-branch config recorded: error.
	_, err := DynamicBaseCommit(git)
	assert.Error(t, err)

	// Stale literal "HEAD" record: treated as absent.
	require.NoError(t, git.SetGitConfig(ParentBranchConfigKey("task/issue-1"), "HEAD"))
	_, err = DynamicBaseCommit(git)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "literal value")
	require.NoError(t, git.UnsetGitConfig(ParentBranchConfigKey("task/issue-1")))

	// Valid parent-branch record: resolves to the merge-base.
	require.NoError(t, git.SetGitConfig(ParentBranchConfigKey("task/issue-1"), "main-parent"))
	got, err := DynamicBaseCommit(git)
	require.NoError(t, err)
	assert.Equal(t, baseSHA, got)
}

// TestResolveBaseCommit_FallbackChain_REQ_LNGHZN_S4_T3 exercises the
// three-tier base-commit fallback chain (dynamic parent-branch merge-base ->
// claim-time recorded SHA -> default-branch merge-base) end-to-end, with
// each tier's data source deliberately missing in turn, asserting the
// correct next tier is used and produces a valid base-commit SHA.
func TestResolveBaseCommit_FallbackChain_REQ_LNGHZN_S4_T3(t *testing.T) {
	t.Parallel()

	// Shared repo shape for all scenarios: the repo's default branch ("main",
	// per this environment's init.defaultBranch) holds the base commit;
	// task/issue-1 branches off it and gets one extra commit. Using "main"
	// itself as the parent (rather than a separately named branch) lets the
	// tier-3 scenario exercise GetBaseCommit's default-branch candidate list
	// (see CandidateBaseRefs) without any extra setup.
	setup := func(t *testing.T) (worktreePath, baseSHA string, git *adapters.Client) {
		t.Helper()
		tmpDir := t.TempDir()
		initGitRepo(t, tmpDir)
		runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")
		base := getHeadSHA(t, tmpDir)
		runGit(t, tmpDir, "checkout", "-b", "task/issue-1")
		runGit(t, tmpDir, "commit", "--allow-empty", "-m", "task work")
		return tmpDir, base, adapters.New(tmpDir)
	}

	t.Run("tier1_dynamic_parent_branch_config_present", func(t *testing.T) {
		t.Parallel()
		worktreePath, baseSHA, git := setup(t)
		require.NoError(t, git.SetGitConfig(ParentBranchConfigKey("task/issue-1"), "main"))

		got, err := ResolveBaseCommit(worktreePath, git)
		require.NoError(t, err)
		assert.Equal(t, baseSHA, got, "tier 1 (dynamic merge-base) should be used when parent-branch config is present")
	})

	t.Run("tier2_recorded_sha_when_dynamic_absent", func(t *testing.T) {
		t.Parallel()
		worktreePath, baseSHA, git := setup(t)
		// git config absent (tier 1 unavailable): fall through to the
		// claim-time recorded SHA file.
		gitDir, err := ResolveWorktreeGitDir(worktreePath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, BaseCommitFileName), []byte(baseSHA), 0o600))

		got, err := ResolveBaseCommit(worktreePath, git)
		require.NoError(t, err)
		assert.Equal(t, baseSHA, got, "tier 2 (recorded SHA) should be used when tier 1 is unavailable")
	})

	t.Run("tier3_default_branch_merge_base_when_both_absent", func(t *testing.T) {
		t.Parallel()
		worktreePath, baseSHA, git := setup(t)
		// Neither git config (tier 1) nor the recorded-SHA file (tier 2)
		// exists — simulating a very old worktree that predates both
		// mechanisms. Falls through to GetBaseCommit's default-branch
		// candidate list (see CandidateBaseRefs), which resolves "main".

		got, err := ResolveBaseCommit(worktreePath, git)
		require.NoError(t, err)
		assert.Equal(t, baseSHA, got, "tier 3 (default-branch merge-base) should be used when tiers 1 and 2 are both unavailable")
	})
}
