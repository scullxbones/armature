package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendHighStakesOp_PublishFailureKeepsLocalCommit_REQ_OPS_PUBLISH(t *testing.T) {
	_, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	state := lowStakesState(t, repo, worktree, 5)
	logPath := filepath.Join(worktree, "ops", "high-stakes-publish.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))

	headBefore := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	breakOrigin(t, repo)

	op := ops.Op{Type: ops.OpNote, TargetID: "T-HS-1", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "loud publish"}}
	err = appendHighStakesOp(state, logPath, op)
	require.Error(t, err)
	assert.True(t, isOpsPublishError(err), "got %v", err)
	assert.Contains(t, err.Error(), "publish _armature")

	logged, readErr := ops.ReadLog(logPath)
	require.NoError(t, readErr)
	require.NotEmpty(t, logged)
	assert.Equal(t, "T-HS-1", logged[len(logged)-1].TargetID)

	headAfter := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	assert.NotEqual(t, headBefore, headAfter, "local ops commit must remain after publish failure")

	tracker, ok := state.tracker.(*fakePendingPushTracker)
	require.True(t, ok)
	assert.Equal(t, 0, tracker.resetCalls, "failed high-stakes publish must not Reset the pending tracker")
}

func TestAppendHighStakesOp_SuccessfulPushResetsTracker_REQ_OPS_PUBLISH(t *testing.T) {
	_, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	state := lowStakesState(t, repo, worktree, 5)
	logPath := filepath.Join(worktree, "ops", "high-stakes-ok.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	op := ops.Op{Type: ops.OpNote, TargetID: "T-HS-2", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "ok"}}
	require.NoError(t, appendHighStakesOp(state, logPath, op))

	tracker, ok := state.tracker.(*fakePendingPushTracker)
	require.True(t, ok)
	assert.Equal(t, 1, tracker.resetCalls)
}

func TestAppendLowStakesOps_PublishFailureIsBestEffort_REQ_OPS_PUBLISH(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	refBefore := showArmatureRef(t, bareDir)
	breakOrigin(t, repo)

	state := lowStakesState(t, repo, worktree, 1)
	logPath := filepath.Join(worktree, "ops", "low-stakes-be.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	op := ops.Op{Type: ops.OpNote, TargetID: "T-LOW-BE", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "best-effort"}}
	require.NoError(t, appendLowStakesOp(state, logPath, op), "low-stakes coalesced publish must swallow git errors")

	assert.Equal(t, refBefore, showArmatureRef(t, bareDir), "failed best-effort push must not update origin")
	tracker, ok := state.tracker.(*fakePendingPushTracker)
	require.True(t, ok)
	assert.Equal(t, 1, tracker.resetCalls)
}

func TestPushOps_PushFailureStillPUSHOPS1_REQ_OPS_PUBLISH(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	dropOrigin(t, repo)

	out, err := runTrls(t, repo, "push-ops", "--format", "json")
	require.Error(t, err)
	assert.NotContains(t, out, `"status":"pushed"`)
	assert.Contains(t, err.Error(), "push-ops: push failed")
}

func breakOrigin(t *testing.T, repo string) {
	t.Helper()
	run(t, repo, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
}
