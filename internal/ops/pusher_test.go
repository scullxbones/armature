package ops_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePusher struct {
	pushCalls    int
	rebaseCalls  int
	pushErr      error
	pushErrAfter int // return error for first N calls
}

func (f *fakePusher) Push(branch string) error {
	f.pushCalls++
	if f.pushErrAfter > 0 && f.pushCalls <= f.pushErrAfter {
		return f.pushErr
	}
	return nil
}

func (f *fakePusher) FetchAndRebase(branch string) error {
	f.rebaseCalls++
	return nil
}

func TestNoPusher_SingleBranch_SkipsPush(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	pusher := ops.NoPusher{}
	op := ops.Op{Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Msg: "hello"}}

	// In single-branch mode (worktreePath=""), NoPusher just appends
	err := pusher.Push(logPath, "", op, nil)
	require.NoError(t, err)

	data, _ := os.ReadFile(logPath)
	assert.Contains(t, string(data), "note")
}

func TestAppendCommitAndPush_DualBranch_PushesAndResetsTracker(t *testing.T) {
	dir := t.TempDir()
	worktreePath := dir
	logPath := filepath.Join(worktreePath, "worker.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fp := &fakePusher{}
	fc := &fakeCommitter{}
	pusher := &ops.AppendCommitAndPush{
		Pusher:  fp,
		Branch:  "_trellis",
		Backoff: []time.Duration{}, // no backoff for test
	}

	op := ops.Op{Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Msg: "hello"}}

	err := pusher.Push(logPath, worktreePath, op, fc)
	require.NoError(t, err)
	assert.Equal(t, 1, fp.pushCalls)
}

func TestAppendCommitAndPush_AllAttemptsFail_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	worktreePath := dir
	logPath := filepath.Join(worktreePath, "worker.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fp := &fakePusher{
		pushErr:      errors.New("rejected"),
		pushErrAfter: 10, // always fail
	}
	fc := &fakeCommitter{}
	pusher := &ops.AppendCommitAndPush{
		Pusher:  fp,
		Branch:  "_trellis",
		Backoff: []time.Duration{0, 0, 0}, // no actual sleep in tests
	}

	op := ops.Op{Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Msg: "hello"}}

	err := pusher.Push(logPath, worktreePath, op, fc)
	assert.Error(t, err)
	assert.Equal(t, 4, fp.pushCalls) // 1 initial + 3 retries
}
