package materialize

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMaterialize_MkdirAllErrorPropagated verifies that when os.MkdirAll fails
// (because the state directory cannot be created), Materialize returns an error.
func TestMaterialize_MkdirAllErrorPropagated(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root; permission restrictions do not apply")
	}
	dir := t.TempDir()
	// Make the stateDir's parent read-only so os.MkdirAll cannot create subdirs
	readOnlyDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(readOnlyDir, 0555))
	t.Cleanup(func() {
		if err := os.Chmod(readOnlyDir, 0755); err != nil {
			t.Fatal(err)
		}
	})

	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	stateDir := filepath.Join(readOnlyDir, "state")

	_, err := Materialize(stateDir, []ops.Op{}, false, nil)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails, got nil")
	}
}

// TestMaterializeAndReturn_MkdirAllErrorPropagated verifies that MaterializeAndReturn
// also propagates the MkdirAll error.
func TestMaterializeAndReturn_MkdirAllErrorPropagated(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root; permission restrictions do not apply")
	}
	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(readOnlyDir, 0555))
	t.Cleanup(func() {
		if err := os.Chmod(readOnlyDir, 0755); err != nil {
			t.Fatal(err)
		}
	})

	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	stateDir := filepath.Join(readOnlyDir, "state")

	_, _, err := MaterializeAndReturn(stateDir, []ops.Op{}, false, nil)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails, got nil")
	}
}

// TestMaterialize_SlottedLogsIncluded verifies that ops in <worker>~slot.log files
// are included in a normal Materialize call.
func TestMaterialize_SlottedLogsIncluded(t *testing.T) {
	t.Parallel()
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

	// Read all ops from the opsDir
	allOps, err := ops.ReadLog(plainLog)
	require.NoError(t, err)
	slottedOps, err := ops.ReadLog(slottedLog)
	require.NoError(t, err)
	allOps = append(allOps, slottedOps...)

	result, err := Materialize(filepath.Join(dir, "state"), allOps, true, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount)
	assert.Equal(t, 3, result.OpsProcessed)
}

// TestMaterializeExcludeWorker_AlsoExcludesSlottedLogs verifies that excluding
// worker-x also skips worker-x~slot-a.log.
func TestMaterializeExcludeWorker_AlsoExcludesSlottedLogs(t *testing.T) {
	t.Parallel()
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

	// Read all ops
	opsA, err := ops.ReadLog(logA)
	require.NoError(t, err)
	opsASlot, err := ops.ReadLog(logASlot)
	require.NoError(t, err)
	opsB, err := ops.ReadLog(logB)
	require.NoError(t, err)
	allOps := append(append(opsA, opsASlot...), opsB...)

	// Exclude worker-a: task-01 should not appear as done (or at all)
	state, result, err := MaterializeExcludeWorker(allOps, workerA, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount, "only worker-b's issue should be present")
	_, hasTaskOne := state.Issues["task-01"]
	assert.False(t, hasTaskOne, "task-01 created by excluded worker must not appear")
}

// TestMaterialize_UnknownOpTypeErrorSurfaced verifies that when an op with
// an unknown type is included in a replay, the op is captured in UnhandledOps
// (not silently dropped and not returned as an error from Materialize itself).
func TestMaterialize_UnknownOpTypeErrorSurfaced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	// Create a valid op and an op with an unknown type
	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "unknown_future_op_type",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
		Payload:   ops.Payload{},
	}

	allOps := []ops.Op{validOp, unknownOp}

	// Materialize with unknown op type
	result, err := Materialize(stateDir, allOps, false, nil)
	require.NoError(t, err, "Materialize should not error, but should capture unknown ops")

	// Verify the op was captured in UnhandledOps (not returned as an error)
	assert.Greater(t, len(result.UnhandledOps), 0, "unknown op type should be captured in UnhandledOps")
	assert.Equal(t, 1, len(result.UnhandledOps), "should have exactly one unhandled op")
	assert.Equal(t, "unknown_future_op_type", result.UnhandledOps[0].Type, "unhandled op should be the unknown type")
}

// TestMaterializeAndReturn_UnknownOpTypeErrorSurfaced verifies that MaterializeAndReturn
// also captures unknown op types in Result.UnhandledOps (not returned as errors).
func TestMaterializeAndReturn_UnknownOpTypeErrorSurfaced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "another_unknown_type",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
	}

	allOps := []ops.Op{validOp, unknownOp}

	_, result, err := MaterializeAndReturn(stateDir, allOps, false, nil)
	require.NoError(t, err, "MaterializeAndReturn should not error, but should capture unknown ops")

	// Verify unknown op is captured
	assert.Greater(t, len(result.UnhandledOps), 0, "unknown op type error should be captured in UnhandledOps")
	assert.Equal(t, 1, len(result.UnhandledOps), "should have exactly one unhandled op")
	assert.Equal(t, "another_unknown_type", result.UnhandledOps[0].Type, "unhandled op should be the unknown type")
}

// TestMaterializeExcludeWorker_UnknownOpTypeErrorSurfaced verifies that
// MaterializeExcludeWorker also captures unknown op types in Result.UnhandledOps.
func TestMaterializeExcludeWorker_UnknownOpTypeErrorSurfaced(t *testing.T) {
	t.Parallel()
	workerA := "worker-a"
	workerB := "worker-b"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerA,
		Payload:   ops.Payload{Title: "Task one", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "yet_another_unknown",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerB,
	}

	allOps := []ops.Op{validOp, unknownOp}

	_, result, err := MaterializeExcludeWorker(allOps, "worker-c", false)
	require.NoError(t, err, "MaterializeExcludeWorker should not error, but should capture unknown ops")

	assert.Greater(t, len(result.UnhandledOps), 0, "unknown op type error should be captured in UnhandledOps")
}

// TestMaterialize_UnhandledOpsWarningEmitted verifies that when unknown ops exist,
// a warning is emitted to stderr before checkpointing.
func TestMaterialize_UnhandledOpsWarningEmitted(t *testing.T) { //nolint:paralleltest
	// Note: Not parallel to avoid stderr capture race conditions (os.Stderr is global)
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	unknownOp1 := ops.Op{
		Type:      "unknown_type_1",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
	}

	unknownOp2 := ops.Op{
		Type:      "unknown_type_2",
		TargetID:  "task-03",
		Timestamp: 300,
		WorkerID:  workerID,
	}

	allOps := []ops.Op{validOp, unknownOp1, unknownOp2}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	// Run Materialize
	result, materializeErr := Materialize(stateDir, allOps, false, nil)

	// Restore stderr
	require.NoError(t, w.Close())
	os.Stderr = oldStderr
	stderrOutput, err := io.ReadAll(r)
	require.NoError(t, err)

	require.NoError(t, materializeErr, "Materialize should not error")
	assert.Equal(t, 2, len(result.UnhandledOps), "should have two unhandled ops")

	// Verify warning was emitted to stderr
	stderrStr := string(stderrOutput)
	assert.Contains(t, stderrStr, "warning:", "stderr should contain warning prefix")
	assert.Contains(t, stderrStr, "2", "stderr should mention count of unhandled ops")
	assert.Contains(t, stderrStr, "op(s) with unknown types skipped", "stderr should describe the issue")
}

// TestMaterializeAndReturn_UnhandledOpsWarningEmitted verifies that MaterializeAndReturn
// also emits a warning to stderr when unknown ops exist.
func TestMaterializeAndReturn_UnhandledOpsWarningEmitted(t *testing.T) { //nolint:paralleltest
	// Note: Not parallel to avoid stderr capture race conditions (os.Stderr is global)
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "unknown_op_type",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
	}

	allOps := []ops.Op{validOp, unknownOp}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	// Run MaterializeAndReturn
	_, result, funcErr := MaterializeAndReturn(stateDir, allOps, false, nil)

	// Restore stderr immediately and close the write end
	os.Stderr = oldStderr
	require.NoError(t, w.Close())

	// Read all the captured output
	stderrOutput, readErr := io.ReadAll(r)
	require.NoError(t, readErr)

	require.NoError(t, funcErr, "MaterializeAndReturn should not error")
	assert.Equal(t, 1, len(result.UnhandledOps), "should have one unhandled op")

	// Verify warning was emitted to stderr
	stderrStr := string(stderrOutput)
	assert.Contains(t, stderrStr, "warning:", "stderr should contain warning prefix")
	assert.Contains(t, stderrStr, "op(s) with unknown types skipped", "stderr should describe the issue")
}

// TestMaterializeExcludeWorker_UnhandledOpsWarningEmitted verifies that MaterializeExcludeWorker
// also emits a warning to stderr when unknown ops exist.
func TestMaterializeExcludeWorker_UnhandledOpsWarningEmitted(t *testing.T) { //nolint:paralleltest
	// Note: Not parallel to avoid stderr capture race conditions (os.Stderr is global)
	workerA := "worker-a"
	workerB := "worker-b"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerA,
		Payload:   ops.Payload{Title: "Task one", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "unknown_type_3",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerB,
	}

	allOps := []ops.Op{validOp, unknownOp}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	// Run MaterializeExcludeWorker
	_, result, funcErr := MaterializeExcludeWorker(allOps, "worker-c", false)

	// Restore stderr
	os.Stderr = oldStderr
	require.NoError(t, w.Close())
	stderrOutput, readErr := io.ReadAll(r)
	require.NoError(t, readErr)

	require.NoError(t, funcErr, "MaterializeExcludeWorker should not error")
	assert.Greater(t, len(result.UnhandledOps), 0, "should have at least one unhandled op")

	// Verify warning was emitted to stderr
	stderrStr := string(stderrOutput)
	assert.Contains(t, stderrStr, "warning:", "stderr should contain warning prefix")
	assert.Contains(t, stderrStr, "op(s) with unknown types skipped", "stderr should describe the issue")
}

// TestIncremental_MatchesFullReplay verifies that incremental materialization
// produces identical state to a full replay. This test:
// 1. Runs a full replay to establish baseline state
// 2. Appends new ops to the log file
// 3. Runs an incremental replay using the checkpoint
// 4. Asserts both final states are identical
func TestIncremental_MatchesFullReplay(t *testing.T) {
	t.Parallel()
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
	opsInitial, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	offsets := map[string]int64{filepath.Base(logPath): info.Size()}
	baselineState, baselineResult, err := MaterializeAndReturn(stateDir, opsInitial, false, offsets)
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
	opsAll, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	info2, err := os.Stat(logPath)
	require.NoError(t, err)
	offsets2 := map[string]int64{filepath.Base(logPath): info2.Size()}
	incrementalState, incrementalResult, err := MaterializeAndReturn(stateDir, opsAll, false, offsets2)
	require.NoError(t, err)
	assert.Equal(t, 2, len(incrementalState.Issues))
	assert.Equal(t, 4, incrementalResult.OpsProcessed, "should have processed all 4 ops (including the 2 new ones)")
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
	opsAll2, err := ops.ReadLog(logPath2)
	require.NoError(t, err)
	fullReplayState, fullReplayResult, err := MaterializeAndReturn(stateDir2, opsAll2, false, nil)
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
