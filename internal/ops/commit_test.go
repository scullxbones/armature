package ops_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendAndCommit_SingleBranch_NoCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ops", "abc.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fc := &FakeCommitter{}
	op := ops.Op{Type: ops.OpNote, TargetID: "T1", Timestamp: 1000, WorkerID: "abc",
		Payload: ops.Payload{Msg: "hello"}}

	err := ops.AppendAndCommit(logPath, "", op, fc)
	require.NoError(t, err)

	// File should contain the op
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "note")

	// No commit was called (worktreePath is "")
	assert.Len(t, fc.Calls, 0)
}

func TestAppendAndCommit_DualBranch_Commits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	worktreePath := filepath.Join(dir, ".arm")
	logPath := filepath.Join(worktreePath, ".issues", "ops", "abc.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fc := &FakeCommitter{}
	op := ops.Op{Type: ops.OpClaim, TargetID: "T1", Timestamp: 1000, WorkerID: "abc-def-ghi-jkl",
		Payload: ops.Payload{TTL: 60}}

	err := ops.AppendAndCommit(logPath, worktreePath, op, fc)
	require.NoError(t, err)

	// Commit was called once
	require.Len(t, fc.Calls, 1)
	assert.Contains(t, fc.Calls[0].Message, "claim")
	assert.Contains(t, fc.Calls[0].Message, "T1")
}

func TestAppendAndCommit_ShortWorkerID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	worktreePath := filepath.Join(dir, ".arm")
	logPath := filepath.Join(worktreePath, ".issues", "ops", "x.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fc := &FakeCommitter{}
	// WorkerID shorter than 8 chars must not panic
	op := ops.Op{Type: ops.OpNote, TargetID: "T2", Timestamp: 1000, WorkerID: "abc",
		Payload: ops.Payload{Msg: "hi"}}

	assert.NotPanics(t, func() {
		if err := ops.AppendAndCommit(logPath, worktreePath, op, fc); err != nil {
			t.Fatal(err)
		}
	})
}
