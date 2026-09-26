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

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Test", NodeType: "task"}}
	op2 := Op{Type: OpClaim, TargetID: "task-01", Timestamp: 101, WorkerID: "worker-a1",
		Payload: Payload{TTL: 60}}

	require.NoError(t, AppendOps(logPath, []Op{op1, op2}))

	stream := newValidatedOpStream()
	entry := stream.addFile(logPath, "worker-a1")
	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

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

	stream := newValidatedOpStream()
	entry1 := stream.addFile(logPath1, "worker-a1")
	entry2 := stream.addFile(logPath2, "worker-b2")
	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

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

	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b2",
		Payload: Payload{Title: "Bad", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	stream := newValidatedOpStream()
	stream.addFile(logPath, "worker-a1")

	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

	require.NoError(t, err)
	assert.Len(t, items, 0)
	assert.Len(t, warnings, 1)
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

	stream := newValidatedOpStream()
	stream.addFile(logPath, "worker-a1")
	loaded, err := stream.loadAll()
	items := loaded.Items

	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Greater(t, items[0].Offset, int64(0))
	assert.Greater(t, items[1].Offset, items[0].Offset)

	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, info.Size(), items[1].Offset)
	assert.Equal(t, info.Size(), loaded.PhysicalEOF[filepath.Base(logPath)])
}

func TestValidatedOpStream_PreservesLogFilename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "custom-worker-id~slot.log")

	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "custom-worker-id~slot",
		Payload: Payload{Title: "Test", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	stream := newValidatedOpStream()
	stream.addFile(logPath, "custom-worker-id~slot")
	loaded, err := stream.loadAll()
	items := loaded.Items

	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, logPath, items[0].LogFilename)
}

func TestValidatedOpStream_SkipsCorruptLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Valid", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op1))

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.WriteString("not valid json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	op2 := Op{Type: OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a1",
		Payload: Payload{Msg: "Also valid"}}
	require.NoError(t, AppendOp(logPath, op2))

	stream := newValidatedOpStream()
	stream.addFile(logPath, "worker-a1")
	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "corrupt")
}

func TestValidatedOpStream_FileNotFound(t *testing.T) {
	t.Parallel()
	stream := newValidatedOpStream()
	stream.addFile("/nonexistent/path/worker.log", "worker-a1")
	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

	assert.Error(t, err)
	assert.Len(t, items, 0)
	assert.Len(t, warnings, 0)
}

func TestValidatedOpStream_Empty(t *testing.T) {
	t.Parallel()
	stream := newValidatedOpStream()
	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

	require.NoError(t, err)
	assert.Len(t, items, 0)
	assert.Len(t, warnings, 0)
}

func TestValidatedOpStream_MultipleFiles_MixedValidity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath1 := filepath.Join(dir, "worker-a1.log")
	logPath2 := filepath.Join(dir, "worker-b2.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Good", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath1, op1))

	op2 := Op{Type: OpCreate, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-wrong",
		Payload: Payload{Title: "Bad", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath2, op2))

	stream := newValidatedOpStream()
	stream.addFile(logPath1, "worker-a1")
	stream.addFile(logPath2, "worker-b2")

	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Len(t, warnings, 1)
	assert.Equal(t, "task-01", items[0].Op.TargetID)
}

func TestValidatedOpStream_SlottedLogFilename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "3357fe85~a.log")

	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "3357fe85~a",
		Payload: Payload{Title: "Test", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	stream := newValidatedOpStream()
	stream.addFile(logPath, "3357fe85~a")
	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Len(t, warnings, 0)
	assert.Equal(t, logPath, items[0].LogFilename)
}

func TestValidatedOpStream_AcceptsLegacyBaseIDInSlottedLog(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-alpha~slot-a.log")

	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-alpha",
		Payload: Payload{Title: "Legacy Op", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	stream := newValidatedOpStream()
	stream.addFile(logPath, "worker-alpha~slot-a")
	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

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

	op1 := Op{Type: OpCreate, TargetID: "issue-1", Timestamp: 100, WorkerID: "worker-w1",
		Payload: Payload{Title: "First", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op1))

	mismatchOp := Op{Type: OpNote, TargetID: "issue-2", Timestamp: 101, WorkerID: "worker-wrong",
		Payload: Payload{Msg: "Mismatch"}}
	require.NoError(t, AppendOp(logPath, mismatchOp))

	op3 := Op{Type: OpNote, TargetID: "issue-3", Timestamp: 102, WorkerID: "worker-w1",
		Payload: Payload{Msg: "Third"}}
	require.NoError(t, AppendOp(logPath, op3))

	stream := newValidatedOpStream()
	stream.addFile(logPath, "worker-w1")
	loaded, err := stream.loadAll()
	items, warnings := loaded.Items, loaded.Warnings

	require.NoError(t, err)
	assert.Len(t, items, 2, "should accept 2 ops (line 1 and 3) and reject 1 (line 2)")
	assert.Len(t, warnings, 1, "should have 1 warning for the mismatch")

	assert.Equal(t, 1, items[0].LineNumber, "first accepted op should be from physical line 1")
	assert.Equal(t, 3, items[1].LineNumber, "second accepted op should be from physical line 3")
}

func TestLoadFromDirValidated_DirDoesNotExist(t *testing.T) {
	t.Parallel()
	got, err := LoadFromDirValidated("/nonexistent/directory/path")

	require.NoError(t, err)
	assert.Len(t, got.Items, 0)
	assert.Len(t, got.PhysicalEOF, 0)
	assert.Len(t, got.Warnings, 0)
}

func TestLoadFromDirValidated_DirWithValidLogs(t *testing.T) {
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

	got, err := LoadFromDirValidated(dir)

	require.NoError(t, err)
	assert.Len(t, got.Warnings, 0)
	assert.Len(t, got.Items, 2)
	assert.Equal(t, "worker-a1", got.Items[0].Op.WorkerID)
	assert.Equal(t, "worker-b2", got.Items[1].Op.WorkerID)
}

func TestLoadFromDirValidated_ExtractsWorkerIDFromFilename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "custom-id~slot-x.log")

	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "custom-id~slot-x",
		Payload: Payload{Title: "Test", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op))

	got, err := LoadFromDirValidated(dir)

	require.NoError(t, err)
	assert.Len(t, got.Warnings, 0)
	assert.Len(t, got.Items, 1)
	assert.Equal(t, "custom-id~slot-x", got.Items[0].Op.WorkerID)
}

func TestLoadFromDirValidated_KeysMapByBasename(t *testing.T) {
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

	got, err := LoadFromDirValidated(dir)

	require.NoError(t, err)
	assert.Len(t, got.Warnings, 0)
	assert.Len(t, got.Items, 2)

	assert.Contains(t, got.PhysicalEOF, "worker-a1.log")
	assert.Contains(t, got.PhysicalEOF, "worker-b2.log")
	assert.Greater(t, got.PhysicalEOF["worker-a1.log"], int64(0))
	assert.Greater(t, got.PhysicalEOF["worker-b2.log"], int64(0))
}

func TestLoadFromDirValidated_AllMismatchedOps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-wrong",
		Payload: Payload{Title: "Wrong1", NodeType: "task"}}
	op2 := Op{Type: OpCreate, TargetID: "task-02", Timestamp: 101, WorkerID: "worker-wrong",
		Payload: Payload{Title: "Wrong2", NodeType: "task"}}

	require.NoError(t, AppendOp(logPath, op1))
	require.NoError(t, AppendOp(logPath, op2))

	got, err := LoadFromDirValidated(dir)

	require.NoError(t, err)
	assert.Len(t, got.Items, 0)
	assert.Len(t, got.Warnings, 2)

	logName := "worker-a1.log"
	assert.Contains(t, got.PhysicalEOF, logName)
	assert.Greater(t, got.PhysicalEOF[logName], int64(0))

	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, info.Size(), got.PhysicalEOF[logName])
}

func TestLoadFromDirValidated_AcceptedOpsFollowedByTrailingRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	acceptedOp := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Accepted", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, acceptedOp))

	rejectedOp := Op{Type: OpNote, TargetID: "task-01", Timestamp: 101, WorkerID: "worker-wrong",
		Payload: Payload{Msg: "This op should be rejected"}}
	require.NoError(t, AppendOp(logPath, rejectedOp))

	got, err := LoadFromDirValidated(dir)

	require.NoError(t, err)
	assert.Len(t, got.Items, 1)
	assert.Len(t, got.Warnings, 1)

	logName := "worker-a1.log"
	assert.Contains(t, got.PhysicalEOF, logName)

	fileInfo, err := os.Stat(logPath)
	require.NoError(t, err)
	fileSize := fileInfo.Size()

	assert.Equal(t, fileSize, got.PhysicalEOF[logName])
	assert.Greater(t, got.PhysicalEOF[logName], got.Items[0].Offset)
}

func TestLoadFromDirValidated_AcceptedOpsFollowedByTrailingCorrupt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	acceptedOp := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Accepted", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, acceptedOp))

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.WriteString("not valid json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := LoadFromDirValidated(dir)

	require.NoError(t, err)
	assert.Len(t, got.Items, 1)
	assert.Len(t, got.Warnings, 1)

	logName := "worker-a1.log"
	assert.Contains(t, got.PhysicalEOF, logName)

	fileInfo, err := os.Stat(logPath)
	require.NoError(t, err)
	fileSize := fileInfo.Size()

	assert.Equal(t, fileSize, got.PhysicalEOF[logName])
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
