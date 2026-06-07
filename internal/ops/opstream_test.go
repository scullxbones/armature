package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatedOpStream_LoadSingleFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	// Create test ops
	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Test", NodeType: "task"}}
	op2 := Op{Type: OpClaim, TargetID: "task-01", Timestamp: 101, WorkerID: "worker-a1",
		Payload: Payload{TTL: 60}}

	require.NoError(t, AppendOps(logPath, []Op{op1, op2}))

	// Load via ValidatedOpStream
	stream := NewValidatedOpStream()
	entry := stream.AddFile(logPath, "worker-a1")
	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, warnings, 0)
	assert.Len(t, items, 2)
	assert.Equal(t, OpCreate, items[0].Op.Type)
	assert.Equal(t, OpClaim, items[1].Op.Type)
	assert.Equal(t, logPath, items[0].LogFilename)
	assert.Equal(t, entry, items[0].Source)
}

func TestValidatedOpStream_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	logPath1 := filepath.Join(dir, "worker-a1.log")
	logPath2 := filepath.Join(dir, "worker-b2.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "From A", NodeType: "task"}}
	op2 := Op{Type: OpCreate, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-b2",
		Payload: Payload{Title: "From B", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath1, op1))
	require.NoError(t, AppendOp(logPath2, op2))

	stream := NewValidatedOpStream()
	entry1 := stream.AddFile(logPath1, "worker-a1")
	entry2 := stream.AddFile(logPath2, "worker-b2")
	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, warnings, 0)
	assert.Len(t, items, 2)
	assert.Equal(t, entry1, items[0].Source)
	assert.Equal(t, entry2, items[1].Source)
	assert.Equal(t, "From A", items[0].Op.Payload.Title)
	assert.Equal(t, "From B", items[1].Op.Payload.Title)
}

func TestValidatedOpStream_RejectsWorkerIDMismatch(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	// Op with mismatched worker ID
	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b2",
		Payload: Payload{Title: "Bad", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	stream := NewValidatedOpStream()
	stream.AddFile(logPath, "worker-a1") // expect worker-a1, not worker-b2

	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 0)    // Op rejected
	assert.Len(t, warnings, 1) // One warning about mismatch
	assert.Contains(t, warnings[0], "worker ID mismatch")
}

func TestValidatedOpStream_ReturnsOffsets(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "First", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op1))

	op2 := Op{Type: OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a1",
		Payload: Payload{Msg: "Second"}}
	require.NoError(t, AppendOp(logPath, op2))

	stream := NewValidatedOpStream()
	stream.AddFile(logPath, "worker-a1")
	items, _, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Greater(t, items[0].Offset, int64(0))
	assert.Greater(t, items[1].Offset, items[0].Offset)

	// Get final file size
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, info.Size(), items[1].Offset)
}

func TestValidatedOpStream_PreservesLogFilename(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "custom-worker-id~slot.log")

	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "custom-worker-id~slot",
		Payload: Payload{Title: "Test", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	stream := NewValidatedOpStream()
	stream.AddFile(logPath, "custom-worker-id~slot")
	items, _, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, logPath, items[0].LogFilename)
}

func TestValidatedOpStream_SkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	// Write a valid op
	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Valid", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op1))

	// Append corrupt line manually
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.WriteString("not valid json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Write another valid op
	op2 := Op{Type: OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a1",
		Payload: Payload{Msg: "Also valid"}}
	require.NoError(t, AppendOp(logPath, op2))

	stream := NewValidatedOpStream()
	stream.AddFile(logPath, "worker-a1")
	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 2)    // Only valid ops loaded
	assert.Len(t, warnings, 1) // Warning about corrupt line
	assert.Contains(t, warnings[0], "corrupt")
}

func TestValidatedOpStream_FileNotFound(t *testing.T) {
	stream := NewValidatedOpStream()
	stream.AddFile("/nonexistent/path/worker.log", "worker-a1")
	items, warnings, err := stream.Load()

	// Should fail gracefully
	assert.Error(t, err)
	assert.Len(t, items, 0)
	assert.Len(t, warnings, 0)
}

func TestValidatedOpStream_Empty(t *testing.T) {
	stream := NewValidatedOpStream()
	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 0)
	assert.Len(t, warnings, 0)
}

func TestValidatedOpStream_MultipleFiles_MixedValidity(t *testing.T) {
	dir := t.TempDir()
	logPath1 := filepath.Join(dir, "worker-a1.log")
	logPath2 := filepath.Join(dir, "worker-b2.log")

	// Valid op in first file
	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Good", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath1, op1))

	// Mismatched op in second file
	op2 := Op{Type: OpCreate, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-wrong",
		Payload: Payload{Title: "Bad", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath2, op2))

	stream := NewValidatedOpStream()
	stream.AddFile(logPath1, "worker-a1")
	stream.AddFile(logPath2, "worker-b2") // expect b2, not worker-wrong

	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 1)    // Only first op accepted
	assert.Len(t, warnings, 1) // One warning about mismatch
	assert.Equal(t, "task-01", items[0].Op.TargetID)
}

func TestValidatedOpStream_SlottedLogFilename(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "3357fe85~a.log")

	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "3357fe85~a",
		Payload: Payload{Title: "Test", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	stream := NewValidatedOpStream()
	stream.AddFile(logPath, "3357fe85~a")
	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Len(t, warnings, 0)
	assert.Equal(t, logPath, items[0].LogFilename)
}
