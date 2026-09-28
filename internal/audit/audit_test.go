package audit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/audit"
	"github.com/scullxbones/armature/internal/ops"
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

func readLogContents(t *testing.T, opsDir string) []audit.Input {
	t.Helper()
	var logs []audit.Input

	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".log" {
			logPath := filepath.Join(opsDir, entry.Name())
			logOps, err := ops.ReadLog(logPath)
			require.NoError(t, err)
			var lines []string
			for _, op := range logOps {
				line, err := ops.MarshalOp(op)
				require.NoError(t, err)
				lines = append(lines, string(line))
			}
			logs = append(logs, audit.Input{File: entry.Name(), Lines: lines})
		}
	}

	return logs
}

func mustLoad(t *testing.T, logs []audit.Input, f audit.Filter) []audit.Entry {
	t.Helper()
	entries, warnings, err := audit.Load(logs, f)
	require.NoError(t, err)
	require.Empty(t, warnings)
	return entries
}

func TestLoad_AllOps(t *testing.T) {
	t.Parallel()
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

	entries := mustLoad(t, readLogContents(t, opsDir), audit.Filter{})
	assert.Len(t, entries, 3)
	assert.Equal(t, int64(100), entries[0].Timestamp)
	assert.Equal(t, int64(150), entries[1].Timestamp)
	assert.Equal(t, int64(200), entries[2].Timestamp)
}

func TestLoad_SortsTiesByWorkerID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	writeLog(t, opsDir, "worker-b", []ops.Op{
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "worker-b",
			Payload: ops.Payload{Msg: "from b"}},
	})
	writeLog(t, opsDir, "worker-a", []ops.Op{
		{Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{Msg: "from a"}},
	})

	entries := mustLoad(t, readLogContents(t, opsDir), audit.Filter{})
	require.Len(t, entries, 2)
	assert.Equal(t, "worker-a", entries[0].WorkerID)
	assert.Equal(t, "worker-b", entries[1].WorkerID)
}

func TestLoad_FilterByIssue(t *testing.T) {
	t.Parallel()
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

	entries := mustLoad(t, readLogContents(t, opsDir), audit.Filter{IssueID: "T1"})
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.Equal(t, "T1", e.TargetID)
	}
}

func TestLoad_FilterByWorker(t *testing.T) {
	t.Parallel()
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

	entries := mustLoad(t, readLogContents(t, opsDir), audit.Filter{WorkerID: "worker-b"})
	assert.Len(t, entries, 1)
	assert.Equal(t, "worker-b", entries[0].WorkerID)
}

func TestLoad_FilterBySince(t *testing.T) {
	t.Parallel()
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
	entries := mustLoad(t, readLogContents(t, opsDir), audit.Filter{Since: since})
	assert.Len(t, entries, 2)
	assert.Equal(t, int64(200), entries[0].Timestamp)
	assert.Equal(t, int64(300), entries[1].Timestamp)
}

func TestLoad_LostRace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")

	writeLog(t, opsDir, "worker-a", []ops.Op{
		{Type: ops.OpClaim, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60}},
	})
	writeLog(t, opsDir, "worker-b", []ops.Op{
		{Type: ops.OpClaim, TargetID: "T1", Timestamp: 200, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: 60}},
	})

	entries := mustLoad(t, readLogContents(t, opsDir), audit.Filter{})
	assert.Len(t, entries, 2)

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

func TestLoad_LostRaceLoseThenWin_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	claimedAt := int64(100)
	ttl := 1
	takeAt := claimedAt + int64(ttl)*60
	logContents := make([]string, 0, 3)
	for _, op := range []ops.Op{
		{Type: ops.OpClaim, TargetID: "T1", Timestamp: claimedAt, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: ttl, ClaimToken: "a"}},
		{Type: ops.OpClaim, TargetID: "T1", Timestamp: claimedAt + 10, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: ttl, ClaimToken: "b-lose"}},
		{Type: ops.OpClaim, TargetID: "T1", Timestamp: takeAt, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: ttl, ClaimToken: "b-win"}},
	} {
		line, err := ops.MarshalOp(op)
		require.NoError(t, err)
		logContents = append(logContents, string(line))
	}
	entries := mustLoad(t, []audit.Input{{File: "claims.log", Lines: logContents}}, audit.Filter{})
	require.Len(t, entries, 3)
	byToken := map[string]audit.Entry{}
	for _, e := range entries {
		byToken[e.Payload.ClaimToken] = e
	}
	assert.False(t, byToken["a"].LostRace)
	assert.True(t, byToken["b-lose"].LostRace)
	assert.False(t, byToken["b-win"].LostRace)
}

func TestLoad_EmptyDir(t *testing.T) {
	t.Parallel()
	entries := mustLoad(t, []audit.Input{}, audit.Filter{})
	assert.Len(t, entries, 0)
}

func TestLoad_NonExistentDir(t *testing.T) {
	t.Parallel()
	entries := mustLoad(t, []audit.Input{}, audit.Filter{})
	assert.Len(t, entries, 0)
}

func TestLoad_CorruptLineWarning_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	valid, err := ops.MarshalOp(ops.Op{
		Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a",
		Payload: ops.Payload{Msg: "ok"},
	})
	require.NoError(t, err)

	entries, warnings, err := audit.Load([]audit.Input{{
		File:  "worker-a.log",
		Lines: []string{string(valid), "this is not json", ""},
	}}, audit.Filter{})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "T1", entries[0].TargetID)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "worker-a.log")
	assert.Contains(t, warnings[0], ":2:")
}

func TestLoad_CorruptLineAfterBlankReportsPhysicalLine_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	valid, err := ops.MarshalOp(ops.Op{
		Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a",
		Payload: ops.Payload{Msg: "ok"},
	})
	require.NoError(t, err)

	entries, warnings, err := audit.Load([]audit.Input{{
		File:  "worker-a.log",
		Lines: []string{string(valid), "", "this is not json"},
	}}, audit.Filter{})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "worker-a.log:3:")
	assert.NotContains(t, warnings[0], "worker-a.log:2:")
}
