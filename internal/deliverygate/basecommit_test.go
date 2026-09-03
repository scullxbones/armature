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

// writeIssueBindingFile writes the armature-issue-id marker file into the
// worktree's resolved git directory, mirroring updateIssueIDFile in
// cmd/armature/claim.go, so tests can simulate a claimed worktree without
// importing the (unimportable) cmd/armature main package.
func writeIssueBindingFile(t *testing.T, worktreePath, issueID string) {
	t.Helper()
	gitDir, err := worktree.ResolveGitDir(worktreePath)
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

	// issueType with no branch mapping and not currently claimed skips the
	// check entirely.
	assert.NoError(t, VerifyIssueBranchBinding(tmpDir, "issue-1", "epic", ""))

	// On an unrelated branch: fail closed.
	runGit(t, tmpDir, "checkout", "-b", "scratch")
	err := VerifyIssueBranchBinding(tmpDir, "issue-1", "task", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scratch")

	// On the expected task branch: pass.
	runGit(t, tmpDir, "checkout", "-b", "task/issue-1")
	assert.NoError(t, VerifyIssueBranchBinding(tmpDir, "issue-1", "task", ""))
}

// TestVerifyIssueBranchBinding_FailsClosedWhenClaimedAndNoRecordOrMapping_REQ_LNGHZN_S4
// verifies the fix for the second (previously silent-pass) half of the
// no-record fallback: a pre-migration worktree (claimed before the
// armature-claimed-branch marker mechanism existed) for an issue that is
// STILL claimed, whose current type has no branch mapping (e.g. amended
// task -> epic without releasing the claim), must fail closed rather than
// return nil. Absence of a record for a claimed issue is never evidence
// there is nothing to check.
func TestVerifyIssueBranchBinding_FailsClosedWhenClaimedAndNoRecordOrMapping_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	// No armature-claimed-branch marker recorded at all (pre-migration
	// worktree), and the current type ("epic") has no branch mapping.
	// Previously this returned nil unconditionally; now it must fail closed
	// because the issue is still claimed.
	err := VerifyIssueBranchBinding(tmpDir, "issue-1", "epic", "worker-a")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue-1")
	assert.Contains(t, err.Error(), "worker-a")

	// Same setup, but NOT claimed (claimedBy == ""): this is the legitimate
	// case (e.g. a genuinely branchless epic that was never claimed into the
	// worktree workflow) and must still pass.
	assert.NoError(t, VerifyIssueBranchBinding(tmpDir, "issue-1", "epic", ""))
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
	gitDir, err := worktree.ResolveGitDir(tmpDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, BaseCommitFileName), []byte(sha+"\n"), 0o600))

	got, err := RecordedBaseCommit(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, sha, got)
}

// TestVerifyIssueBranchBinding_FailsClosedWhenAmendedTypeHasNoBranchMapping_REQ_LNGHZN_S4
// verifies the fix for the open PR review comment on basecommit.go:143: if
// a task is claimed (recording armature-claimed-branch), then its type is
// amended to an unmapped type (e.g. epic) WITHOUT releasing the claim, and a
// clean in-scope commit lands on a scratch branch that was never
// task/<ID>, VerifyIssueBranchBinding must fail closed — not silently skip
// verification because DeriveBranchName(current type) now returns "".
func TestVerifyIssueBranchBinding_FailsClosedWhenAmendedTypeHasNoBranchMapping_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	// Simulate claim time: record the branch the issue was actually claimed
	// under (task/issue-1), as writeClaimedBranchFileIfAbsent would.
	gitDir, err := worktree.ResolveGitDir(tmpDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, ClaimedBranchFileName), []byte("task/issue-1"), 0o600))

	// Check out a scratch branch that was never the claimed task branch.
	runGit(t, tmpDir, "checkout", "-b", "scratch")

	// The issue's type is amended to "epic" (unmapped) without releasing the
	// claim. DeriveBranchName("epic", ...) returns "", but that must NOT
	// cause verification to be skipped now that a claimed-branch record
	// exists.
	err = VerifyIssueBranchBinding(tmpDir, "issue-1", "epic", "worker-a")
	assert.Error(t, err, "must fail closed: recorded claimed branch task/issue-1 does not match current branch scratch")
	assert.Contains(t, err.Error(), "task/issue-1")
	assert.Contains(t, err.Error(), "scratch")
}

// TestRecordedClaimedBranch_REQ_LNGHZN_S4 verifies the claim-time recorded
// claimed-branch marker is read back correctly, and that a missing file (or
// a not-found-worktree) surfaces "not found" so callers can fall back to
// re-deriving the expected branch from the current issue type.
func TestRecordedClaimedBranch_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")

	// Absent: not found, no error.
	branch, found, err := RecordedClaimedBranch(tmpDir)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, branch)

	// Present: returns the recorded branch name.
	gitDir, err := worktree.ResolveGitDir(tmpDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, ClaimedBranchFileName), []byte("task/issue-1\n"), 0o600))

	branch, found, err = RecordedClaimedBranch(tmpDir)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "task/issue-1", branch)
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

// TestGatedBaseCommit_REQ_LNGHZN_S4 verifies GatedBaseCommit trusts claim-time
// recorded facts (the dynamically-recomputed parent-branch merge-base, or the
// SHA recorded once at claim time) but fails closed — rather than falling
// through to GetBaseCommit's default-branch guess — when NEITHER recorded
// fact is available, even though that guess would happily resolve in this
// repo shape.
func TestGatedBaseCommit_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "init")
	baseSHA := getHeadSHA(t, tmpDir)
	runGit(t, tmpDir, "checkout", "-b", "task/issue-1")
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "task work")

	git := adapters.New(tmpDir)

	// Neither a parent-branch config (dynamic tier) nor a recorded
	// base-commit file exists yet: GatedBaseCommit must fail closed, even
	// though GetBaseCommit's default-branch guess would resolve via "main"
	// in this same repo shape.
	_, err := GatedBaseCommit(tmpDir, "issue-1", git)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue-1")

	// Sanity check: GetBaseCommit DOES succeed in this same repo shape,
	// proving GatedBaseCommit is deliberately more conservative, not that
	// both merely fail for an unrelated reason.
	guessed, guessErr := GetBaseCommit(git)
	require.NoError(t, guessErr)
	assert.Equal(t, baseSHA, guessed)

	// Once the recorded SHA file exists, GatedBaseCommit returns it (tier:
	// RecordedBaseCommit, since no parent-branch config is set).
	gitDir, err := worktree.ResolveGitDir(tmpDir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, BaseCommitFileName), []byte(baseSHA), 0o600))

	got, err := GatedBaseCommit(tmpDir, "issue-1", git)
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
		gitDir, err := worktree.ResolveGitDir(worktreePath)
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
