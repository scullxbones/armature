package audit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/trellis/internal/audit"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLog(t *testing.T, opsDir, workerID string, entries []ops.Op) {
	t.Helper()
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logPath := filepath.Join(opsDir, workerID+".log")
	for _, op := range entries {
		require.NoError(t, ops.AppendOp(logPath, op))
	}
}

func TestLoad_AllOps(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	writeLog(t, opsDir, "worker-a", []ops.Op{
		{Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "Task 1", NodeType: "task"}},
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 200, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "hello"}},
	})
	writeLog(t, opsDir, "worker-b", []ops.Op{
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 150, WorkerID: "worker-b",
			Payload: ops.Payload{Msg: "from b"}},
	})

	entries, err := audit.Load(opsDir, audit.Filter{})
	require.NoError(t, err)
	assert.Len(t, entries, 3)
	// Sorted by timestamp
	assert.Equal(t, int64(100), entries[0].Timestamp)
	assert.Equal(t, int64(150), entries[1].Timestamp)
	assert.Equal(t, int64(200), entries[2].Timestamp)
}

func TestLoad_FilterByIssue(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	writeLog(t, opsDir, "worker-a", []ops.Op{
		{Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "Task 1", NodeType: "task"}},
		{Type: ops.OpCreate, TargetID: "T2", Timestamp: 101, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "Task 2", NodeType: "task"}},
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 200, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "about T1"}},
	})

	entries, err := audit.Load(opsDir, audit.Filter{IssueID: "T1"})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.Equal(t, "T1", e.TargetID)
	}
}

func TestLoad_FilterByWorker(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	writeLog(t, opsDir, "worker-a", []ops.Op{
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "from a"}},
	})
	writeLog(t, opsDir, "worker-b", []ops.Op{
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 200, WorkerID: "worker-b",
			Payload: ops.Payload{Msg: "from b"}},
	})

	entries, err := audit.Load(opsDir, audit.Filter{WorkerID: "worker-b"})
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "worker-b", entries[0].WorkerID)
}

func TestLoad_FilterBySince(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	writeLog(t, opsDir, "worker-a", []ops.Op{
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "old"}},
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 200, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "new"}},
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 300, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "newer"}},
	})

	since := time.Unix(200, 0)
	entries, err := audit.Load(opsDir, audit.Filter{Since: since})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, int64(200), entries[0].Timestamp)
	assert.Equal(t, int64(300), entries[1].Timestamp)
}

func TestLoad_LostRace(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")

	// Two workers claim the same task; worker-a wins (earlier timestamp)
	writeLog(t, opsDir, "worker-a", []ops.Op{
		{Type: ops.OpClaim, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60}},
	})
	writeLog(t, opsDir, "worker-b", []ops.Op{
		{Type: ops.OpClaim, TargetID: "T1", Timestamp: 200, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: 60}},
	})

	entries, err := audit.Load(opsDir, audit.Filter{})
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// worker-a's claim is the winner (not lost race)
	var aEntry, bEntry audit.Entry
	for _, e := range entries {
		if e.WorkerID == "worker-a" {
			aEntry = e
		} else {
			bEntry = e
		}
	}
	assert.False(t, aEntry.LostRace, "worker-a should be the winner")
	assert.True(t, bEntry.LostRace, "worker-b should be the loser")
}

func TestLoad_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	entries, err := audit.Load(opsDir, audit.Filter{})
	require.NoError(t, err)
	assert.Len(t, entries, 0)
}

func TestLoad_NonExistentDir(t *testing.T) {
	entries, err := audit.Load("/nonexistent/ops", audit.Filter{})
	require.NoError(t, err)
	assert.Len(t, entries, 0)
}
