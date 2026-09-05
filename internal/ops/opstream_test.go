package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatedOpStream_LoadSingleFile(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	stream := NewValidatedOpStream()
	stream.AddFile("/nonexistent/path/worker.log", "worker-a1")
	items, warnings, err := stream.Load()

	// Should fail gracefully
	assert.Error(t, err)
	assert.Len(t, items, 0)
	assert.Len(t, warnings, 0)
}

func TestValidatedOpStream_Empty(t *testing.T) {
	t.Parallel()
	stream := NewValidatedOpStream()
	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 0)
	assert.Len(t, warnings, 0)
}

func TestValidatedOpStream_MultipleFiles_MixedValidity(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestValidatedOpStream_AcceptsLegacyBaseIDInSlottedLog(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-alpha~slot-a.log")

	// Legacy op with base worker ID (no slot suffix), stored in slotted log file
	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-alpha",
		Payload: Payload{Title: "Legacy Op", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	stream := NewValidatedOpStream()
	stream.AddFile(logPath, "worker-alpha~slot-a")
	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 1, "should accept legacy base worker ID in slotted log")
	assert.Len(t, warnings, 0, "should not generate warnings for valid legacy ops")
	assert.Equal(t, "worker-alpha", items[0].Op.WorkerID)
	assert.Equal(t, logPath, items[0].LogFilename)
}

func TestLoadFile_LineNumberPopulated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-w1.log")

	// Line 1: valid op
	op1 := Op{Type: OpCreate, TargetID: "issue-1", Timestamp: 100, WorkerID: "worker-w1",
		Payload: Payload{Title: "First", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op1))

	// Line 2: mismatch op (will be rejected)
	mismatchOp := Op{Type: OpNote, TargetID: "issue-2", Timestamp: 101, WorkerID: "worker-wrong",
		Payload: Payload{Msg: "Mismatch"}}
	require.NoError(t, AppendOp(logPath, mismatchOp))

	// Line 3: valid op
	op3 := Op{Type: OpNote, TargetID: "issue-3", Timestamp: 102, WorkerID: "worker-w1",
		Payload: Payload{Msg: "Third"}}
	require.NoError(t, AppendOp(logPath, op3))

	stream := NewValidatedOpStream()
	stream.AddFile(logPath, "worker-w1")
	items, warnings, err := stream.Load()

	require.NoError(t, err)
	assert.Len(t, items, 2, "should accept 2 ops (line 1 and 3) and reject 1 (line 2)")
	assert.Len(t, warnings, 1, "should have 1 warning for the mismatch")

	// Verify physical line numbers are set correctly
	assert.Equal(t, 1, items[0].LineNumber, "first accepted op should be from physical line 1")
	assert.Equal(t, 3, items[1].LineNumber, "second accepted op should be from physical line 3")
}

// ===== Tests for package-level LoadFromDirWithOffsetsValidated =====

func TestLoadFromDirWithOffsetsValidated_DirDoesNotExist(t *testing.T) {
	t.Parallel()
	items, offsets, warnings, err := LoadFromDirWithOffsetsValidated("/nonexistent/directory/path")

	require.NoError(t, err)
	assert.Len(t, items, 0)
	assert.Len(t, offsets, 0)
	assert.Len(t, warnings, 0)
}

func TestLoadFromDirWithOffsetsValidated_DirWithValidLogs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath1 := filepath.Join(dir, "worker-a1.log")
	logPath2 := filepath.Join(dir, "worker-b2.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "A1 Op", NodeType: "task"}}
	op2 := Op{Type: OpCreate, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-b2",
		Payload: Payload{Title: "B2 Op", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath1, op1))
	require.NoError(t, AppendOp(logPath2, op2))

	items, _, warnings, err := LoadFromDirWithOffsetsValidated(dir)

	require.NoError(t, err)
	assert.Len(t, warnings, 0)
	assert.Len(t, items, 2)
	// Verify worker IDs match filenames
	assert.Equal(t, "worker-a1", items[0].Op.WorkerID)
	assert.Equal(t, "worker-b2", items[1].Op.WorkerID)
}

func TestLoadFromDirWithOffsetsValidated_ExtractsWorkerIDFromFilename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "custom-id~slot-x.log")

	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "custom-id~slot-x",
		Payload: Payload{Title: "Test", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	items, _, warnings, err := LoadFromDirWithOffsetsValidated(dir)

	require.NoError(t, err)
	assert.Len(t, warnings, 0)
	assert.Len(t, items, 1)
	assert.Equal(t, "custom-id~slot-x", items[0].Op.WorkerID)
}

func TestLoadFromDirWithOffsetsValidated_KeysMapByBasename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath1 := filepath.Join(dir, "worker-a1.log")
	logPath2 := filepath.Join(dir, "worker-b2.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "First", NodeType: "task"}}
	op2 := Op{Type: OpCreate, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-b2",
		Payload: Payload{Title: "Second", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath1, op1))
	require.NoError(t, AppendOp(logPath2, op2))

	items, offsets, warnings, err := LoadFromDirWithOffsetsValidated(dir)

	require.NoError(t, err)
	assert.Len(t, warnings, 0)
	assert.Len(t, items, 2)

	// Verify offsets map uses basenames, not full paths
	assert.Contains(t, offsets, "worker-a1.log")
	assert.Contains(t, offsets, "worker-b2.log")
	assert.Greater(t, offsets["worker-a1.log"], int64(0))
	assert.Greater(t, offsets["worker-b2.log"], int64(0))
}

func TestLoadFromDirWithOffsetsValidated_AllMismatchedOps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	// Write ops with mismatched worker IDs
	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-wrong",
		Payload: Payload{Title: "Wrong1", NodeType: "task"}}
	op2 := Op{Type: OpCreate, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-wrong",
		Payload: Payload{Title: "Wrong2", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op1))
	require.NoError(t, AppendOp(logPath, op2))

	items, offsets, warnings, err := LoadFromDirWithOffsetsValidated(dir)

	require.NoError(t, err)
	// All ops are rejected due to mismatch
	assert.Len(t, items, 0)
	assert.Len(t, warnings, 2)

	// CRITICAL: Even though all ops are mismatched, the offset should still be recorded
	// so we don't re-read the same lines forever
	logName := "worker-a1.log"
	assert.Contains(t, offsets, logName, "offset should be recorded even for all-mismatched files")
	assert.Greater(t, offsets[logName], int64(0), "offset should point past all lines")

	// Get file size to verify offset
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, info.Size(), offsets[logName], "offset should match file size")
}

func TestLoadFromDirWithOffsetsValidated_AcceptedOpsFollowedByTrailingRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	// Write one accepted op (matching worker ID)
	acceptedOp := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Accepted", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, acceptedOp))

	// Append trailing rejected line (mismatched worker ID)
	rejectedOp := Op{Type: OpNote, TargetID: "task-01", Timestamp: 101, WorkerID: "worker-wrong",
		Payload: Payload{Msg: "This op should be rejected"}}
	require.NoError(t, AppendOp(logPath, rejectedOp))

	items, offsets, warnings, err := LoadFromDirWithOffsetsValidated(dir)

	require.NoError(t, err)
	// One accepted op, one rejected
	assert.Len(t, items, 1)
	assert.Len(t, warnings, 1)

	logName := "worker-a1.log"
	assert.Contains(t, offsets, logName)

	// CRITICAL: The offset must point to the END of the file (past the trailing rejected line),
	// not just to the end of the last accepted op.
	// This ensures incremental readers resuming from this checkpoint won't re-read the rejected tail forever.
	fileInfo, err := os.Stat(logPath)
	require.NoError(t, err)
	fileSize := fileInfo.Size()

	assert.Equal(t, fileSize, offsets[logName],
		"offset must equal file size to avoid re-reading trailing rejected lines")

	// Verify offset is past the accepted op itself
	assert.Greater(t, offsets[logName], items[0].Offset,
		"offset must be past the accepted op, not just at its end")
}

func TestLoadFromDirWithOffsetsValidated_AcceptedOpsFollowedByTrailingCorrupt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	// Write one accepted op (matching worker ID)
	acceptedOp := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Accepted", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, acceptedOp))

	// Append corrupt line directly
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.WriteString("not valid json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	items, offsets, warnings, err := LoadFromDirWithOffsetsValidated(dir)

	require.NoError(t, err)
	// One accepted op, one corrupt line (skipped)
	assert.Len(t, items, 1)
	assert.Len(t, warnings, 1) // Warning about corrupt line

	logName := "worker-a1.log"
	assert.Contains(t, offsets, logName)

	// CRITICAL: Even with a corrupt trailing line, the offset must point to the physical EOF
	fileInfo, err := os.Stat(logPath)
	require.NoError(t, err)
	fileSize := fileInfo.Size()

	assert.Equal(t, fileSize, offsets[logName],
		"offset must equal file size even when trailing line is corrupt")
}

func TestExtractOps_ReturnsOpsFromItems(t *testing.T) {
	t.Parallel()
	items := []OpItem{
		{Op: Op{Type: "create", TargetID: "A"}},
		{Op: Op{Type: "transition", TargetID: "B"}},
	}
	result := ExtractOps(items)
	require.Len(t, result, 2)
	assert.Equal(t, "create", result[0].Type)
	assert.Equal(t, "transition", result[1].Type)
}

func TestExtractOps_EmptyInput(t *testing.T) {
	t.Parallel()
	result := ExtractOps([]OpItem{})
	assert.Empty(t, result)
}
