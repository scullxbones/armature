package doctor_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	actions := doctor.PlanFixes(allIssues, "fixer-01", now)
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

	actions := doctor.PlanFixes(allIssues, "fixer-01", now)
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
	actions := doctor.PlanFixes(allIssues, "fixer-01", now)
	require.Len(t, actions, 1)

	// Dry run: do not call ApplyFixes. The ops log must be unchanged.
	items, _, _, err := ops.LoadFromDirWithOffsetsValidated(filepath.Join(issuesDir, "ops"))
	require.NoError(t, err)
	assert.Len(t, items, 2, "dry run must not append any ops")

	// Re-planning without applying must yield the identical action set.
	_, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	actions2 := doctor.PlanFixes(allIssues2, "fixer-01", now)
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
	actions := doctor.PlanFixes(allIssues, "fixer-01", now)
	require.Len(t, actions, 1)
	require.NoError(t, doctor.ApplyFixes(fixerLogPath, actions))

	// Issue should now be open; doctor should be clean; a second plan should find nothing.
	index, allIssues2, err := doctor.LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	require.Equal(t, ops.StatusOpen, index["idempotent-01"].Status)

	actions2 := doctor.PlanFixes(allIssues2, "fixer-01", now)
	assert.Empty(t, actions2, "second PlanFixes run must find nothing left to fix")

	require.NoError(t, doctor.ApplyFixes(fixerLogPath, actions2))
	items, _, _, err := ops.LoadFromDirWithOffsetsValidated(filepath.Join(issuesDir, "ops"))
	require.NoError(t, err)
	assert.Len(t, items, 4, "no-op second ApplyFixes call must not append anything")
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
