package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLineNewerSchemaFailsLoad_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	valid := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Valid", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, valid))

	newer := CurrentSchemaVersion + 1
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = fmt.Fprintf(f, `["note","task-01",200,"worker-a1",{"msg":"future"},%d]`+"\n", newer)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	before, err := os.ReadFile(logPath)
	require.NoError(t, err)

	got, err := LoadFromDirValidated(dir)
	require.Error(t, err)
	assert.Empty(t, got.Items)
	var newerErr *NewerSchemaVersionError
	require.ErrorAs(t, err, &newerErr)
	assert.Equal(t, newer, newerErr.Seen)
	assert.Equal(t, CurrentSchemaVersion, newerErr.Supported)

	after, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestCorruptLineFailsReadLogFromOffset_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	before := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Valid", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, before))

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.WriteString("not json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	after := Op{Type: OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a1",
		Payload: Payload{Msg: "after corrupt"}}
	require.NoError(t, AppendOp(logPath, after))

	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)

	ops, err := ReadLogFromOffset(logPath, 0)
	require.Error(t, err)
	assert.Nil(t, ops)
	assert.Contains(t, err.Error(), "invalid JSONL line")

	got, loadErr := LoadFromDirValidated(dir)
	require.Error(t, loadErr)
	assert.Empty(t, got.Items)

	unchanged, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, raw, unchanged)
}
