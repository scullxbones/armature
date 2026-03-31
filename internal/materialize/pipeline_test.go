package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMaterialize_MkdirAllErrorPropagated verifies that when os.MkdirAll fails
// (because the state directory cannot be created), Materialize returns an error.
func TestMaterialize_MkdirAllErrorPropagated(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission restrictions do not apply")
	}
	dir := t.TempDir()
	// Make the stateDir's parent read-only so os.MkdirAll cannot create subdirs
	readOnlyDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(readOnlyDir, 0555))
	t.Cleanup(func() { _ = os.Chmod(readOnlyDir, 0755) })

	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	stateDir := filepath.Join(readOnlyDir, "state")

	_, err := Materialize(issuesDir, stateDir, false)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails, got nil")
	}
}

// TestMaterializeAndReturn_MkdirAllErrorPropagated verifies that MaterializeAndReturn
// also propagates the MkdirAll error.
func TestMaterializeAndReturn_MkdirAllErrorPropagated(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission restrictions do not apply")
	}
	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(readOnlyDir, 0555))
	t.Cleanup(func() { _ = os.Chmod(readOnlyDir, 0755) })

	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	stateDir := filepath.Join(readOnlyDir, "state")

	_, _, err := MaterializeAndReturn(issuesDir, stateDir, false)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails, got nil")
	}
}

// TestMaterialize_SlottedLogsIncluded verifies that ops in <worker>~slot.log files
// are included in a normal Materialize call.
func TestMaterialize_SlottedLogsIncluded(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "state", "issues"), 0755))
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	workerID := "worker-x"

	// Write a create op to the plain log
	plainLog := filepath.Join(opsDir, workerID+".log")
	require.NoError(t, ops.AppendOp(plainLog, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: workerID,
		Payload: ops.Payload{Title: "My task", NodeType: "task"},
	}))

	// Write a transition op to the slotted log
	slottedLog := filepath.Join(opsDir, workerID+"~slot-a.log")
	require.NoError(t, ops.AppendOp(slottedLog, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: workerID,
		Payload: ops.Payload{TTL: 60},
	}))
	require.NoError(t, ops.AppendOp(slottedLog, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300, WorkerID: workerID,
		Payload: ops.Payload{To: "done", Outcome: "finished"},
	}))

	result, err := Materialize(dir, filepath.Join(dir, "state"), true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount)
	assert.Equal(t, 3, result.OpsProcessed)
}

// TestMaterializeExcludeWorker_AlsoExcludesSlottedLogs verifies that excluding
// worker-x also skips worker-x~slot-a.log.
func TestMaterializeExcludeWorker_AlsoExcludesSlottedLogs(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	workerA := "worker-a"
	workerB := "worker-b"

	// worker-a creates task-01 in plain log
	logA := filepath.Join(opsDir, workerA+".log")
	require.NoError(t, ops.AppendOp(logA, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: workerA,
		Payload: ops.Payload{Title: "Task one", NodeType: "task"},
	}))
	// worker-a also writes a transition in a slotted log
	logASlot := filepath.Join(opsDir, workerA+"~s1.log")
	require.NoError(t, ops.AppendOp(logASlot, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: 200, WorkerID: workerA,
		Payload: ops.Payload{To: "done"},
	}))

	// worker-b creates task-02
	logB := filepath.Join(opsDir, workerB+".log")
	require.NoError(t, ops.AppendOp(logB, ops.Op{
		Type: ops.OpCreate, TargetID: "task-02", Timestamp: 300, WorkerID: workerB,
		Payload: ops.Payload{Title: "Task two", NodeType: "task"},
	}))

	// Exclude worker-a: task-01 should not appear as done (or at all)
	state, result, err := MaterializeExcludeWorker(dir, filepath.Join(dir, "state"), workerA, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount, "only worker-b's issue should be present")
	_, hasTaskOne := state.Issues["task-01"]
	assert.False(t, hasTaskOne, "task-01 created by excluded worker must not appear")
}

// TestIncremental_MatchesFullReplay verifies that incremental materialization
// produces identical state to a full replay. This test:
// 1. Runs a full replay to establish baseline state
// 2. Appends new ops to the log file
// 3. Runs an incremental replay using the checkpoint
// 4. Asserts both final states are identical
func TestIncremental_MatchesFullReplay(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	workerID := "worker-x"
	logPath := filepath.Join(opsDir, workerID+".log")

	// Initial ops: create two tasks
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: workerID,
		Payload: ops.Payload{Title: "Task one", NodeType: "task"},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-02", Timestamp: 200, WorkerID: workerID,
		Payload: ops.Payload{Title: "Task two", NodeType: "task"},
	}))

	// Run full replay to get baseline state
	baselineState, baselineResult, err := MaterializeAndReturn(dir, stateDir, false)
	require.NoError(t, err)
	assert.Equal(t, 2, len(baselineState.Issues))
	assert.Equal(t, 2, baselineResult.OpsProcessed)
	assert.True(t, baselineResult.FullReplay, "baseline should be a full replay")

	// Verify checkpoint was written
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	cp, err := LoadCheckpoint(checkpointPath)
	require.NoError(t, err)
	assert.Greater(t, len(cp.ByteOffsets), 0, "checkpoint should have saved byte offsets")

	// Append new ops to the log
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 300, WorkerID: workerID,
		Payload: ops.Payload{TTL: 60},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: 400, WorkerID: workerID,
		Payload: ops.Payload{To: "done", Outcome: "completed"},
	}))

	// Run incremental replay
	incrementalState, incrementalResult, err := MaterializeAndReturn(dir, stateDir, false)
	require.NoError(t, err)
	assert.Equal(t, 2, len(incrementalState.Issues))
	assert.Equal(t, 2, incrementalResult.OpsProcessed, "should have processed 2 new ops in incremental run")
	assert.False(t, incrementalResult.FullReplay, "incremental replay should set FullReplay=false")

	// Now run full replay again from scratch in a different directory
	dir2 := t.TempDir()
	opsDir2 := filepath.Join(dir2, "ops")
	stateDir2 := filepath.Join(dir2, "state")
	require.NoError(t, os.MkdirAll(opsDir2, 0755))

	logPath2 := filepath.Join(opsDir2, workerID+".log")
	// Write all ops to the new log
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: workerID,
		Payload: ops.Payload{Title: "Task one", NodeType: "task"},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpCreate, TargetID: "task-02", Timestamp: 200, WorkerID: workerID,
		Payload: ops.Payload{Title: "Task two", NodeType: "task"},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 300, WorkerID: workerID,
		Payload: ops.Payload{TTL: 60},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: 400, WorkerID: workerID,
		Payload: ops.Payload{To: "done", Outcome: "completed"},
	}))

	// Run fresh full replay with all ops
	fullReplayState, fullReplayResult, err := MaterializeAndReturn(dir2, stateDir2, false)
	require.NoError(t, err)
	assert.Equal(t, 2, len(fullReplayState.Issues))
	assert.Equal(t, 4, fullReplayResult.OpsProcessed)
	assert.True(t, fullReplayResult.FullReplay)

	// Assert that incremental and full replay produce identical state
	assert.Equal(t, len(fullReplayState.Issues), len(incrementalState.Issues), "issue count must match")
	for issueID, fullIssue := range fullReplayState.Issues {
		incrementalIssue, ok := incrementalState.Issues[issueID]
		assert.True(t, ok, "issue %s must exist in incremental state", issueID)
		assert.Equal(t, fullIssue.ID, incrementalIssue.ID, "issue ID must match")
		assert.Equal(t, fullIssue.Title, incrementalIssue.Title, "title must match")
		assert.Equal(t, fullIssue.Status, incrementalIssue.Status, "status must match: %v vs %v", fullIssue.Status, incrementalIssue.Status)
		assert.Equal(t, fullIssue.ClaimedBy, incrementalIssue.ClaimedBy, "claimed_by must match")
		assert.Equal(t, fullIssue.Outcome, incrementalIssue.Outcome, "outcome must match")
	}
}
