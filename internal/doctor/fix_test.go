package doctor_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
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

func worktreeGitDir(t *testing.T, worktreePath string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "rev-parse", "--git-dir")
	out, err := cmd.Output()
	require.NoError(t, err)
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktreePath, gitDir)
	}
	return gitDir
}

func addCanonicalMarkerWorktree(t *testing.T, repoDir, issueID string) string {
	t.Helper()
	worktreePath := filepath.Join(repoDir, ".worktrees", issueID)
	require.NoError(t, os.MkdirAll(filepath.Dir(worktreePath), 0755))
	runGit(t, repoDir, "worktree", "add", "-b", "task/"+issueID, worktreePath)
	require.NoError(t, os.WriteFile(filepath.Join(worktreeGitDir(t, worktreePath), "armature-issue-id"), []byte(issueID+"\n"), 0644))
	return worktreePath
}

func TestPlanFixes_UsesCanonicalMarkerInventory_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "fixer-01.log")
	repoDir := initGitRepo(t)
	worktreePath := addCanonicalMarkerWorktree(t, repoDir, "canonical-01")

	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "canonical-01", Timestamp: claimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{Title: "Canonical worktree task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "canonical-01", Timestamp: claimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{TTL: 240, WorktreePath: worktreePath}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	assert.Empty(t, actions, "a canonical binding-bound worktree must satisfy doctor --fix")
}

func TestPlanFixes_LiveRecordedLegacyWorktreeIsNotFlagged_REQ_LNGHZN_S5(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "fixer-01.log")
	repoDir := initGitRepo(t)
	legacyPath := filepath.Join(t.TempDir(), "legacy-task-01")
	runGit(t, repoDir, "worktree", "add", "-b", "task/legacy-task-01", legacyPath)
	gitDir := worktreeGitDir(t, legacyPath)
	// The inventory must retain the legacy binding while honoring the
	// explicit path recorded by claims made before canonical provisioning.
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "armature-task-id"), []byte("legacy-task-01\n"), 0o644))

	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "legacy-task-01", Timestamp: claimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{Title: "Legacy worktree task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "legacy-task-01", Timestamp: claimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{TTL: 240, WorktreePath: legacyPath}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	assert.Empty(t, actions, "a live binding-bound worktree at the recorded legacy path must not be repaired")
}

// TestPlanFixes_AmbiguousBindingDoesNotReleaseLiveClaim_REQ_LNGHZN_S5_T6 pins
// why doctor asks the EXISTENCE question and not the selection one. Two
// worktrees carry this issue's binding and no recorded path picks between them,
// so selection is Ambiguous and fails closed. If doctor resolved through
// selection it would read that refusal as "no worktree" and release a live
// worker's claim — turning a fail-closed guard into a destructive act.
func TestPlanFixes_AmbiguousBindingDoesNotReleaseLiveClaim_REQ_LNGHZN_S5_T6(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "fixer-01.log")
	repoDir := initGitRepo(t)

	// Two worktrees, one binding, no recorded path to disambiguate them.
	for i, name := range []string{"legacy-dup-01", "canonical-dup-01"} {
		path := filepath.Join(t.TempDir(), name)
		runGit(t, repoDir, "worktree", "add", "-b", "task/dup-"+strconv.Itoa(i), path)
		gitDir := worktreeGitDir(t, path)
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "armature-task-id"), []byte("dup-task-01\n"), 0o644))
	}

	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "dup-task-01", Timestamp: claimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{Title: "Duplicate binding task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "dup-task-01", Timestamp: claimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	require.Len(t, actions, 1, "an ambiguous binding must be reported without releasing the live claim")
	assert.Empty(t, actions[0].Ops, "the advisory must not append a release op")
	assert.Contains(t, actions[0].Reason, "ambiguous worktree binding")
}

// TestPlanFixes_LegacyMarkerWorktreeWithoutRecordedPathSuppressesFix_REQ_LNGHZN_S5
// covers the binding-is-authoritative policy: a claimed issue owned by the fixer
// with issue.WorktreePath == "" but a live worktree binding-bound to it at a
// non-canonical (legacy) path must NOT be false-released. Before the fix the
// loop skipped any non-canonical worktree when no path was recorded, so an
// active legacy claim was wrongly reset to open.
func TestPlanFixes_LegacyMarkerWorktreeWithoutRecordedPathSuppressesFix_REQ_LNGHZN_S5(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "fixer-01.log")
	repoDir := initGitRepo(t)
	// Live worktree bound to the issue at a legacy path outside
	// .worktrees, while the claim recorded NO WorktreePath.
	legacyPath := filepath.Join(t.TempDir(), "legacy-nopath-01")
	runGit(t, repoDir, "worktree", "add", "-b", "task/legacy-nopath-01", legacyPath)
	require.NoError(t, os.WriteFile(filepath.Join(worktreeGitDir(t, legacyPath), "armature-issue-id"), []byte("legacy-nopath-01\n"), 0644))

	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "legacy-nopath-01", Timestamp: claimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{Title: "Legacy worktree, no recorded path", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "legacy-nopath-01", Timestamp: claimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	assert.Empty(t, actions, "a live binding-bound legacy worktree must suppress remediation even without a recorded WorktreePath")
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

// TestPlanFixes_InProgressTransitionCountsAsActivity reproduces the Codex
// review finding on PR #84 (fix.go:100): materialization leaves LastHeartbeat
// at the original claim timestamp when applying a claimed->in-progress
// transition, so a claim transitioned to in-progress moments before the TTL
// window closes must not be treated as claim-expired just because
// LastHeartbeat wasn't separately bumped.
func TestPlanFixes_InProgressTransitionCountsAsActivity(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	now := time.Now()
	claimedAt := now.Add(-61 * time.Minute).Unix()
	transitionedAt := now.Add(-2 * time.Minute).Unix() // minute 59 of a 60-minute TTL
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "fresh-transition-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Freshly transitioned task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "fresh-transition-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 60}},
		{Type: ops.OpTransition, TargetID: "fresh-transition-01", Timestamp: transitionedAt, WorkerID: "worker-01",
			Payload: ops.Payload{To: ops.StatusInProgress}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, "")
	assert.Empty(t, actions, "a claim just transitioned to in-progress must not be treated as claim-expired")
}

// TestPlanFixes_ThirdPartyNoteDoesNotResetClaimExpiry reproduces the P1 finding
// from the deep review of PR #84: claim expiry originally folded
// issue.Updated into its liveness formula, but Updated is bumped by every op
// handler in internal/materialize/engine.go (applyNote, applyLink, etc.), none
// of which check op.WorkerID against issue.ClaimedBy. So a coordinator (or any
// worker other than the claim owner) leaving an unrelated note on the issue
// shortly before the TTL closes would silently reset the staleness clock as
// far as doctor --fix is concerned, even though the CLAIMING worker did
// nothing. A claim must be treated as expired here based only on activity
// attributable to the claiming worker itself.
func TestPlanFixes_ThirdPartyNoteDoesNotResetClaimExpiry(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	coordinatorLogPath := filepath.Join(issuesDir, "ops", "coordinator-01.log")

	now := time.Now()
	claimedAt := now.Add(-61 * time.Minute).Unix() // TTL 60m: already stale as of the claim/transition
	noteAt := now.Add(-20 * time.Minute).Unix()    // a third party touches Updated recently
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "third-party-note-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Claimed task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "third-party-note-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 60}},
		{Type: ops.OpTransition, TargetID: "third-party-note-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{To: ops.StatusInProgress}},
	}))
	require.NoError(t, ops.AppendOps(coordinatorLogPath, []ops.Op{
		{Type: ops.OpNote, TargetID: "third-party-note-01", Timestamp: noteAt, WorkerID: "coordinator-01",
			Payload: ops.Payload{Msg: "checking in, unrelated to worker-01's claim"}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, "")
	require.Len(t, actions, 1, "the claim must still be recognized as expired despite the coordinator's unrelated note")
	assert.Equal(t, "third-party-note-01", actions[0].IssueID)
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
	require.NoError(t, doctor.ApplyFixes(fixerLogPath, "", actions, nil))

	// Issue should now be open; doctor should be clean; a second plan should find nothing.
	index, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	require.Equal(t, ops.StatusOpen, index["idempotent-01"].Status)

	actions2 := doctor.PlanFixes(allIssues2, "fixer-01", now, "")
	assert.Empty(t, actions2, "second PlanFixes run must find nothing left to fix")

	require.NoError(t, doctor.ApplyFixes(fixerLogPath, "", actions2, nil))
	items, _, _, err := ops.LoadFromDirWithOffsetsValidated(filepath.Join(issuesDir, "ops"))
	require.NoError(t, err)
	assert.Len(t, items, 4, "no-op second ApplyFixes call must not append anything")
}

func TestPlanFixes_ReleasesClaimWithMissingWorktree(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "fixer-01.log")
	repoDir := initGitRepo(t)

	// Active (non-expired) claim: TTL not exhausted, but no `task/missing-wt-01`
	// worktree/branch is registered against repoDir, simulating a worktree torn
	// down (or its git metadata corrupted) while still actively claimed. Claimed
	// by "fixer-01" (the worker that will run PlanFixes below), since the
	// missing-worktree remediation is scoped to the current worker's own claims.
	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "missing-wt-01", Timestamp: claimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{Title: "Missing worktree task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "missing-wt-01", Timestamp: claimedAt, WorkerID: "fixer-01",
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

func TestPlanFixes_LiveFixBranchForBugIsNotFlagged_REQ_TOPTIER_S4_PRFIX(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	// A bug's worktree branch is derived as fix/<id>, not task/<id>. PlanFixes
	// must recognize this live fix/ branch instead of hardcoding "task/".
	repoDir := initGitRepoWithBranch(t, "fix/live-bug-01")

	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "live-bug-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Live bug worktree", NodeType: "bug"}},
		{Type: ops.OpClaim, TargetID: "live-bug-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	assert.Empty(t, actions, "a bug claim with a live fix/ worktree branch must not be flagged as missing")
}

func TestPlanFixes_LiveFeatBranchForFeatureIsNotFlagged_REQ_TOPTIER_S4_PRFIX(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	// A feature/story's worktree branch is derived as feat/<id>, not task/<id>.
	repoDir := initGitRepoWithBranch(t, "feat/live-feature-01")

	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "live-feature-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Live feature worktree", NodeType: "feature"}},
		{Type: ops.OpClaim, TargetID: "live-feature-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	assert.Empty(t, actions, "a feature claim with a live feat/ worktree branch must not be flagged as missing")
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

// TestPlanFixes_MissingWorktreeSkipsOtherWorkersClaims reproduces the Codex
// review finding on PR #84 (fix.go:119): git worktree list only reports
// worktrees registered in the local repository, so in a coordinator clone (or
// any clone that has pulled another worker's claim ops) a claim owned by
// another worker will always look like it has no live worktree locally, even
// though the claiming worker's own machine has one. The missing-worktree
// remediation must be scoped to claims owned by the worker running doctor
// --fix, not any claimed/in-progress issue in the graph.
func TestPlanFixes_MissingWorktreeSkipsOtherWorkersClaims(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	// repoDir is the *fixer's* local clone: it has no worktree registered for
	// this branch because the claim belongs to a different worker/machine.
	repoDir := initGitRepo(t)

	now := time.Now()
	claimedAt := now.Add(-1 * time.Minute).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "other-worker-claim-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Other worker's claim", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "other-worker-claim-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	// "fixer-01" is running doctor --fix, but the claim is owned by "worker-01" —
	// a different worker whose worktree simply isn't visible in this local clone.
	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	assert.Empty(t, actions, "missing-worktree remediation must not touch another worker's claim")
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
		{Type: ops.OpCreate, TargetID: "req-missing-wt-01", Timestamp: missingWTClaimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Missing worktree", NodeType: "task"}},
	}))
	// Missing-worktree case (active TTL, no registered worktree branch). Claimed
	// by "fixer-01" itself, in fixer-01's own log: the missing-worktree
	// remediation is scoped to claims owned by the worker running doctor --fix
	// (see PlanFixes doc comment), so this must match the workerID passed to
	// PlanFixes below, and per the D7 worker-ID-mismatch check the claim op's
	// WorkerID must match the log file it's appended to.
	require.NoError(t, ops.AppendOps(fixerLogPath, []ops.Op{
		{Type: ops.OpClaim, TargetID: "req-missing-wt-01", Timestamp: missingWTClaimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	require.Len(t, actions, 2)
	require.NoError(t, doctor.ApplyFixes(fixerLogPath, "", actions, nil))

	index, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	assert.Equal(t, ops.StatusOpen, index["req-expired-01"].Status)
	assert.Equal(t, ops.StatusOpen, index["req-missing-wt-01"].Status)

	// Idempotent: a second plan against the fixed state finds nothing left to do.
	assert.Empty(t, doctor.PlanFixes(allIssues2, "fixer-01", now, repoDir))
}

// TestApplyFixes_CommitsToWorktreeBranch proves that ApplyFixes, given a
// worktree path and a GitCommitter (the wiring `arm doctor --fix` uses in
// dual-branch mode), actually commits the appended ops to the worktree's
// branch — not just writes the local ops log file — the same way
// appendHighStakesOp commits claim/transition/assign ops.
func TestApplyFixes_CommitsToWorktreeBranch(t *testing.T) {
	t.Parallel()
	repoDir := initGitRepo(t)
	issuesDir := filepath.Join(repoDir, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(issuesDir, "ops"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(issuesDir, "state", "issues"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(issuesDir, "config.json"),
		[]byte(`{"mode":"dual-branch"}`),
		0644,
	))
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	now := time.Now()
	claimedAt := now.Add(-2 * time.Hour).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "commit-test-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Commit test task", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "commit-test-01", Timestamp: claimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 60}},
	}))

	fixerLogPath := filepath.Join(issuesDir, "ops", "fixer-01.log")

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	actions := doctor.PlanFixes(allIssues, "fixer-01", now, "")
	require.Len(t, actions, 1)

	// Baseline: no uncommitted diff for the fixer log yet (it doesn't exist).
	beforeLog := currentCommitSubject(t, repoDir)

	gc := adapters.New(repoDir)
	require.NoError(t, doctor.ApplyFixes(fixerLogPath, repoDir, actions, gc))

	// The fix ops must be committed to the worktree's branch (HEAD advanced,
	// working tree clean), not merely written to the log file on disk.
	afterLog := currentCommitSubject(t, repoDir)
	assert.NotEqual(t, beforeLog, afterLog, "ApplyFixes must create a new commit for the appended ops")

	// The fixer's own ops log file specifically must be committed (not merely
	// present on disk) — other newly created scaffolding (state/, config.json)
	// is irrelevant to this assertion.
	relFixerLog, err := filepath.Rel(repoDir, fixerLogPath)
	require.NoError(t, err)
	status := runGitOutput(t, repoDir, "status", "--porcelain", "--", relFixerLog)
	assert.Empty(t, status, "fixer log must be committed, not left untracked/modified")
}

func currentCommitSubject(t *testing.T, dir string) string {
	t.Helper()
	return runGitOutput(t, dir, "log", "-1", "--format=%H")
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	require.NoError(t, err)
	return string(out)
}

func TestApplyFixes_EmptyActionsIsNoOp(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	require.NoError(t, doctor.ApplyFixes(logPath, "", nil, nil))
	items, _, _, err := ops.LoadFromDirWithOffsetsValidated(filepath.Join(issuesDir, "ops"))
	require.NoError(t, err)
	assert.Empty(t, items)
}
