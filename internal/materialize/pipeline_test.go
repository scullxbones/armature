package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	result, err := Materialize(dir, true)
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
	state, result, err := MaterializeExcludeWorker(dir, workerA, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount, "only worker-b's issue should be present")
	_, hasTaskOne := state.Issues["task-01"]
	assert.False(t, hasTaskOne, "task-01 created by excluded worker must not appear")
}
