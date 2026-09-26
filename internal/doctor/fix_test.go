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

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func applyFixActions(t *testing.T, logPath string, actions []doctor.FixAction) {
	t.Helper()
	for _, a := range actions {
		for _, op := range a.Ops {
			require.NoError(t, ops.AppendAndCommit(logPath, "", op, nil))
		}
	}
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

func TestPlanFixes_AmbiguousBindingDoesNotReleaseLiveClaim_REQ_LNGHZN_S5_T6(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "fixer-01.log")
	repoDir := initGitRepo(t)

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

func TestPlanFixes_LegacyMarkerWorktreeWithoutRecordedPathSuppressesFix_REQ_LNGHZN_S5(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "fixer-01.log")
	repoDir := initGitRepo(t)

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

func TestPlanFixes_InProgressTransitionCountsAsActivity(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

	now := time.Now()
	claimedAt := now.Add(-61 * time.Minute).Unix()
	transitionedAt := now.Add(-2 * time.Minute).Unix()
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

func TestPlanFixes_ThirdPartyNoteDoesNotResetClaimExpiry(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	coordinatorLogPath := filepath.Join(issuesDir, "ops", "coordinator-01.log")

	now := time.Now()
	claimedAt := now.Add(-61 * time.Minute).Unix()
	noteAt := now.Add(-20 * time.Minute).Unix()
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

	items, err := ops.LoadFromDirValidated(filepath.Join(issuesDir, "ops"))
	require.NoError(t, err)
	assert.Len(t, items.Items, 2, "dry run must not append any ops")

	_, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	actions2 := doctor.PlanFixes(allIssues2, "fixer-01", now, "")
	require.Len(t, actions2, 1)
	assert.Equal(t, actions[0].IssueID, actions2[0].IssueID)
}

func TestPlanFixes_IdempotentAfterApply(t *testing.T) {
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

	fixerLogPath := filepath.Join(issuesDir, "ops", "fixer-01.log")

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	actions := doctor.PlanFixes(allIssues, "fixer-01", now, "")
	require.Len(t, actions, 1)
	applyFixActions(t, fixerLogPath, actions)

	index, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	require.Equal(t, ops.StatusOpen, index["idempotent-01"].Status)

	actions2 := doctor.PlanFixes(allIssues2, "fixer-01", now, "")
	assert.Empty(t, actions2, "second PlanFixes run must find nothing left to fix")

	applyFixActions(t, fixerLogPath, actions2)
	items, err := ops.LoadFromDirValidated(filepath.Join(issuesDir, "ops"))
	require.NoError(t, err)
	assert.Len(t, items.Items, 4, "no-op second apply must not append anything")
}

func TestPlanFixes_ReleasesClaimWithMissingWorktree(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "fixer-01.log")
	repoDir := initGitRepo(t)

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

func TestPlanFixes_MissingWorktreeSkipsOtherWorkersClaims(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")

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

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	assert.Empty(t, actions, "missing-worktree remediation must not touch another worker's claim")
}

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

		{Type: ops.OpCreate, TargetID: "req-expired-01", Timestamp: expiredClaimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Expired claim", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "req-expired-01", Timestamp: expiredClaimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 60}},
		{Type: ops.OpCreate, TargetID: "req-missing-wt-01", Timestamp: missingWTClaimedAt, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Missing worktree", NodeType: "task"}},
	}))

	require.NoError(t, ops.AppendOps(fixerLogPath, []ops.Op{
		{Type: ops.OpClaim, TargetID: "req-missing-wt-01", Timestamp: missingWTClaimedAt, WorkerID: "fixer-01",
			Payload: ops.Payload{TTL: 240}},
	}))

	_, allIssues, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)

	actions := doctor.PlanFixes(allIssues, "fixer-01", now, repoDir)
	require.Len(t, actions, 2)
	applyFixActions(t, fixerLogPath, actions)

	index, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	assert.Equal(t, ops.StatusOpen, index["req-expired-01"].Status)
	assert.Equal(t, ops.StatusOpen, index["req-missing-wt-01"].Status)

	assert.Empty(t, doctor.PlanFixes(allIssues2, "fixer-01", now, repoDir))
}
