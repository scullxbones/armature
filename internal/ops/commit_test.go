package ops_test

import (
	"os"
	"path/filepath"
	"sync"
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

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "note")

	assert.Len(t, fc.Calls, 0)
}

func TestAppendAndCommitIf_OverlappingIdenticalSkip_REQ_AOC_S4_T1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ops", "abc.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	op := ops.Op{Type: ops.OpTransition, TargetID: "T1", Timestamp: 1000, WorkerID: "abc",
		Payload: ops.Payload{To: "blocked", Outcome: "Waiting on the upstream review to finish"}}
	proceed := func() (bool, error) {
		all, err := ops.ReadLog(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, err
		}
		last, ok := ops.LastTransitionPayload(all, "T1")
		if ok && ops.PayloadsEqual(last, op.Payload) {
			return false, nil
		}
		return true, nil
	}

	start := make(chan struct{})
	wroteCh := make(chan bool, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			wrote, err := ops.AppendAndCommitIf(logPath, "", op, nil, proceed)
			errs <- err
			wroteCh <- wrote
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	close(wroteCh)
	for err := range errs {
		require.NoError(t, err)
	}
	wroteTrue := 0
	for wrote := range wroteCh {
		if wrote {
			wroteTrue++
		}
	}
	assert.Equal(t, 1, wroteTrue)

	got, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestAppendAndCommitIf_SkipDoesNotCommit_REQ_AOC_S4_T1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	worktreePath := filepath.Join(dir, ".arm")
	logPath := filepath.Join(worktreePath, ".issues", "ops", "abc.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fc := &FakeCommitter{}
	op := ops.Op{Type: ops.OpTransition, TargetID: "T1", Timestamp: 1000, WorkerID: "abc",
		Payload: ops.Payload{To: "blocked", Outcome: "Waiting on the upstream review to finish"}}

	wrote, err := ops.AppendAndCommitIf(logPath, worktreePath, op, fc, func() (bool, error) {
		return false, nil
	})
	require.NoError(t, err)
	assert.False(t, wrote)
	assert.Len(t, fc.Calls, 0)
	if _, statErr := os.Stat(logPath); statErr == nil {
		got, readErr := ops.ReadLog(logPath)
		require.NoError(t, readErr)
		assert.Empty(t, got)
	}
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

	require.Len(t, fc.Calls, 1)
	assert.Contains(t, fc.Calls[0].Message, "claim")
	assert.Contains(t, fc.Calls[0].Message, "T1")
	assert.Contains(t, fc.Calls[0].Message, ops.TruncateWorkerID("abc-def-ghi-jkl", 8))
}

func TestAppendAndCommit_ShortWorkerID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	worktreePath := filepath.Join(dir, ".arm")
	logPath := filepath.Join(worktreePath, ".issues", "ops", "x.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fc := &FakeCommitter{}
	op := ops.Op{Type: ops.OpNote, TargetID: "T2", Timestamp: 1000, WorkerID: "abc",
		Payload: ops.Payload{Msg: "hi"}}

	assert.NotPanics(t, func() {
		if err := ops.AppendAndCommit(logPath, worktreePath, op, fc); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTruncateWorkerID(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "abc", ops.TruncateWorkerID("abc", 8))
	assert.Equal(t, "abcdefgh", ops.TruncateWorkerID("abcdefgh", 8))
	assert.Equal(t, "abcdefgh", ops.TruncateWorkerID("abcdefghij", 8))
	assert.Equal(t, "", ops.TruncateWorkerID("", 8))
	wide := "αβγδε"
	assert.Equal(t, wide[:8], ops.TruncateWorkerID(wide, 8))
	assert.Equal(t, 8, len(ops.TruncateWorkerID(wide, 8)))
}
