package ops

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTornFinalLineIgnored_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	valid := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Valid", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, valid))

	before, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(before), "\n"))

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(`["note","task-01",200,"worker-a1",{"msg":"torn"`)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.False(t, strings.HasSuffix(string(raw), "\n"))

	got, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, OpCreate, got[0].Type)

	loaded, err := LoadFromDirValidated(dir)
	require.NoError(t, err)
	require.Len(t, loaded.Items, 1)
	require.NotEmpty(t, loaded.Warnings)
	assert.Contains(t, loaded.Warnings[0], "torn final line")
	assert.Contains(t, loaded.Warnings[0], "worker-a1.log")

	after, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, raw, after)
}

func TestNewerSchemaUpgradeMessage_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	valid := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Valid", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, valid))

	newer := CurrentSchemaVersion + 1
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = fmt.Fprintf(f, `["note","task-01",200,"worker-a1",{"msg":"future"},%d]`+"\n", newer)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = LoadFromDirValidated(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upgrade arm")
	var newerErr *NewerSchemaVersionError
	require.ErrorAs(t, err, &newerErr)
	assert.Equal(t, newer, newerErr.Seen)

	_, readErr := ReadLogFromOffset(logPath, 0)
	require.Error(t, readErr)
	assert.Contains(t, readErr.Error(), "upgrade arm")
}

func TestCorruptLineStillFailsLoad_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	valid := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Valid", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, valid))

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("not json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	before, err := os.ReadFile(logPath)
	require.NoError(t, err)

	_, err = LoadFromDirValidated(dir)
	require.Error(t, err)
	var located *ParseLoadError
	require.ErrorAs(t, err, &located)
	assert.Equal(t, 2, located.Line)
	assert.Equal(t, logPath, located.LogPath)
	assert.Contains(t, err.Error(), "worker-a1.log:2:")
	assert.Equal(t, "arm doctor quarantine --log worker-a1.log --line 2", located.RepairCommand())

	after, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestQuarantineCorruptLineLoadSucceeds_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	valid := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Valid", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, valid))

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("not json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	after := Op{Type: OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a1",
		Payload: Payload{Msg: "after corrupt"}}
	require.NoError(t, AppendOp(logPath, after))

	before, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Contains(t, string(before), "not json")

	_, err = LoadFromDirValidated(dir)
	require.Error(t, err)

	require.NoError(t, QuarantineLogLine(logPath, 2, 300))
	require.NoError(t, QuarantineLogLine(logPath, 2, 301), "quarantine must be idempotent")

	sidecar, err := os.ReadFile(quarantinePath(logPath))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(sidecar), "not json"))

	afterBytes, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(afterBytes, before), "original log bytes must remain")
	assert.Contains(t, string(afterBytes), "not json")
	assert.Equal(t, 1, strings.Count(string(afterBytes), `"log-quarantine"`))

	loaded, err := LoadFromDirValidated(dir)
	require.NoError(t, err)
	types := make([]string, 0, len(loaded.Items))
	for _, item := range loaded.Items {
		types = append(types, item.Op.Type)
	}
	assert.Contains(t, types, OpCreate)
	assert.Contains(t, types, OpNote)
	assert.Contains(t, types, OpLogQuarantine)
	assert.NotContains(t, types, "")
}
