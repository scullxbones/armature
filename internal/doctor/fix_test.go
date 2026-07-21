package doctor_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initGitRepo creates a minimal git repo with one commit and no other branches,
// for exercising doctor.PlanFixes' missing-worktree detection.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0644))
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-q", "-m", "initial commit")
	return dir
}

// initGitRepoWithBranch is initGitRepo plus a worktree registered on the given
// branch name, so callers can assert that a live worktree suppresses a fix.
func initGitRepoWithBranch(t *testing.T, branch string) string {
	t.Helper()
	dir := initGitRepo(t)
	wtDir := t.TempDir()
	runGit(t, dir, "worktree", "add", "-b", branch, wtDir)
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func TestPlanFixes_ReleasesExpiredClaim(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	now := time.Now()
	claimedAt := now.Add(-2 * time.Hour).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "stale-claim-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Stale claim task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "stale-claim-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 60}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, "")
	require.Len(t, actions, 1)
	assert.Equal(t, "stale-claim-01", actions[0].IssueID)
	require.Len(t, actions[0].Ops, 2)
	assert.Equal(t, ops.OpTransition, actions[0].Ops[0].Type)
	assert.Equal(t, ops.StatusOpen, actions[0].Ops[0].Payload.To)
	assert.Equal(t, ops.OpNote, actions[0].Ops[1].Type)
}

func TestPlanFixes_BlocksStarvedInProgress(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	now := time.Now()
	claimedAt := now.Add(-3 * time.Hour).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "starved-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Starved task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "starved-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 60}},
		{Type: ops.OpTransition, TargetID: "starved-01", Timestamp: claimedAt + 60, WorkerID: "worker-01",
			Payload: ops.Payload{To: ops.StatusInProgress}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, "")
	require.Len(t, actions, 1)
	assert.Equal(t, "starved-01", actions[0].IssueID)
	assert.Equal(t, ops.StatusBlocked, actions[0].Ops[0].Payload.To)
}

func TestPlanFixes_DryRunListsWithoutWriting(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	now := time.Now()
	claimedAt := now.Add(-2 * time.Hour).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "dry-run-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Dry run task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "dry-run-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 60}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	actions := doctor.PlanFixes(allIssues, "fixer-01", now, "")
	require.Len(t, actions, 1)

	// Dry run: do not call ApplyFixes. The ops log must be unchanged.
	items, _, _, err := ops.LoadFromDirWithOffsetsValidated(filepath.Join(issuesDir, "ops"))
	require.NoError(t, err)
	assert.Len(t, items, 2, "dry run must not append any ops")

	// Re-planning without applying must yield the identical action set.
	_, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	actions2 := doctor.PlanFixes(allIssues2, "fixer-01", now, "")
	require.Len(t, actions2, 1)
	assert.Equal(t, actions[0].IssueID, actions2[0].IssueID)
}

func TestApplyFixes_IdempotentOnSecondRun(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	now := time.Now()
	claimedAt := now.Add(-2 * time.Hour).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "idempotent-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Idempotent task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "idempotent-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 60}},
	}))

	// Fixes are appended to the fixer's own worker log, not the original
	// claimant's — ops.AppendOps validates that an op's WorkerID matches the log
	// filename, same as the D7 worker-ID-mismatch check.
	fixerLogPath := filepath.Join(issuesDir, "ops", "fixer-01.log")

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	actions := doctor.PlanFixes(allIssues, "fixer-01", now, "")
	require.Len(t, actions, 1)
	require.NoError(t, doctor.ApplyFixes(fixerLogPath, actions))

	// Issue should now be open; doctor should be clean; a second plan should find nothing.
	index, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	require.Equal(t, ops.StatusOpen, index["idempotent-01"].Status)

	actions2 := doctor.PlanFixes(allIssues2, "fixer-01", now, "")
	assert.Empty(t, actions2, "second PlanFixes run must find nothing left to fix")

	require.NoError(t, doctor.ApplyFixes(fixerLogPath, actions2))
	items, _, _, err := ops.LoadFromDirWithOffsetsValidated(filepath.Join(issuesDir, "ops"))
	require.NoError(t, err)
	assert.Len(t, items, 4, "no-op second ApplyFixes call must not append anything")
}

func TestPlanFixes_ReleasesClaimWithMissingWorktree(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	repoDir := initGitRepo(t)

	// Active (non-expired) claim: TTL not exhausted, but no `task/missing-wt-01`
	// worktree/branch is registered against repoDir, simulating a worktree torn
	// down (or its git metadata corrupted) while still actively claimed.
	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "missing-wt-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Missing worktree task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "missing-wt-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	require.Len(t, actions, 1)
	assert.Equal(t, "missing-wt-01", actions[0].IssueID)
	assert.Equal(t, ops.StatusOpen, actions[0].Ops[0].Payload.To)
}

func TestPlanFixes_LiveWorktreeIsNotFlagged(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	repoDir := initGitRepoWithBranch(t, "task/live-wt-01")

	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "live-wt-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Live worktree task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "live-wt-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	assert.Empty(t, actions, "a claim with a live registered worktree branch must not be flagged")
}

func TestPlanFixes_GitFailure_SkipsMissingWorktreeCheckEntirely(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	// repoPath points at a directory that is not a git repo at all, so
	// GitWorktreeBranches returns a non-nil error (liveness cannot be
	// determined) rather than an empty map. This must not be conflated with
	// "confirmed no live worktree" for every claimed/in-progress issue — see
	// PlanFixes' doc comment on the missing-worktree case.
	notAGitRepo := t.TempDir()

	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "active-claim-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Active claim", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "active-claim-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, notAGitRepo)
	assert.Empty(t, actions, "a git failure while checking worktree liveness must not be treated as 'every claim's worktree is gone'")
}

// TestDoctorFix_REQ_TOPTIER_S4_T2 is the acceptance-named regression test for
// TOPTIER-S4-T2: arm doctor --fix must cover expired claims and missing
// worktrees end to end (create -> claim -> fix -> verify via materialization
// replay). Fleet-wide "half-recorded transition" (done-without-commit) recovery
// is intentionally out of scope for this pass — see the PlanFixes doc comment.
func TestDoctorFix_REQ_TOPTIER_S4_T2(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	fixerLogPath := filepath.Join(issuesDir, "ops", "fixer-01.log")
	repoDir := initGitRepo(t)

	now := time.Now()
	expiredClaimedAt := now.Add(-2 * time.Hour).Unix()
	missingWTClaimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		// Expired claim case.
		{Type: ops.OpCreate, TargetID: "req-expired-01", Timestamp: expiredClaimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Expired claim", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "req-expired-01", Timestamp: expiredClaimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 60}},
		// Missing-worktree case (active TTL, no registered worktree branch).
		{Type: ops.OpCreate, TargetID: "req-missing-wt-01", Timestamp: missingWTClaimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Missing worktree", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "req-missing-wt-01", Timestamp: missingWTClaimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	require.Len(t, actions, 2)
	require.NoError(t, doctor.ApplyFixes(fixerLogPath, actions))

	index, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	assert.Equal(t, ops.StatusOpen, index["req-expired-01"].Status)
	assert.Equal(t, ops.StatusOpen, index["req-missing-wt-01"].Status)

	// Idempotent: a second plan against the fixed state finds nothing left to do.
	assert.Empty(t, doctor.PlanFixes(allIssues2, "fixer-01", now, repoDir))
}

func TestApplyFixes_EmptyActionsIsNoOp(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	require.NoError(t, doctor.ApplyFixes(logPath, nil))
	items, _, _, err := ops.LoadFromDirWithOffsetsValidated(filepath.Join(issuesDir, "ops"))
	require.NoError(t, err)
	assert.Empty(t, items)
}
