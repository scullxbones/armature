package main

import (
	"path/filepath"
	"testing"

	claimpkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoserHeartbeatFailsNamedError_REQ_CLAIMORD_W13(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	before := countHeartbeatOps(t, repo)

	run(t, repo, "git", "config", "armature.worker-id", "loser-worker-w13")
	_, err = runTrls(t, repo, "heartbeat", "--issue", "task-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT-CLAIM-OWNER")
	assert.Equal(t, before, countHeartbeatOps(t, repo), "loser must not append a heartbeat")
}

func TestLoserTransitionFailsNamedError_REQ_CLAIMORD_W13(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	before := countTransitionOps(t, repo)

	run(t, repo, "git", "config", "armature.worker-id", "loser-worker-w13")
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done",
		"--skip-delivery-gate", "--force", "--outcome", "should fail")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT-CLAIM-OWNER")
	assert.Equal(t, before, countTransitionOps(t, repo), "loser must not mark done")
}

func TestTransitionOwnerGateBeforeIdenticalNoOp_REQ_CLAIMORD_W13(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "in-progress")
	require.NoError(t, err)
	before := countTransitionOps(t, repo)

	run(t, repo, "git", "config", "armature.worker-id", "loser-worker-w13")
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "in-progress")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT-CLAIM-OWNER")
	assert.Equal(t, before, countTransitionOps(t, repo, "task-01"), "loser identical transition must not no-op as success")
}

func TestRenderContextOwnerDenied_OnlyLiveHolder_REQ_CLAIMORD_W13(t *testing.T) {
	t.Parallel()
	now := int64(1_700_000_000)
	live := claimpkg.Lease{
		Holder: "owner-a", Token: "tok-a", Status: ops.StatusClaimed,
		LastActivity: now, TTLMinutes: 60,
	}
	assert.True(t, renderContextOwnerDenied(live, "other-worker", now))
	assert.False(t, renderContextOwnerDenied(live, "owner-a", now))

	done := live
	done.Status = ops.StatusDone
	assert.False(t, renderContextOwnerDenied(done, "other-worker", now), "done holder is not a live owner")

	expired := live
	assert.False(t, renderContextOwnerDenied(expired, "other-worker", now+int64(60)*60), "expired lease must not deny others")
}

func TestRenderContextFailsClosedOnLocateError_REQ_CLAIMORD_W13(t *testing.T) {
	t.Parallel()
	err := enforceRenderContextOwner(&config.Context{}, "task-01", "worker-a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ops worktree path is required")
	assert.NotErrorIs(t, err, claimpkg.ErrNotClaimOwner)
}

func TestLoserRenderContextFailsWhileLive_REQ_CLAIMORD_W13(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	run(t, repo, "git", "config", "armature.worker-id", "loser-worker-w13")
	_, err = runTrls(t, repo, "render-context", "--issue", "task-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT-CLAIM-OWNER")
}

func TestRenderContextAllowsOthersAfterDone_REQ_CLAIMORD_W13(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done",
		"--skip-delivery-gate", "--force", "--outcome", "shipped")
	require.NoError(t, err)

	run(t, repo, "git", "config", "armature.worker-id", "other-worker-w13")
	_, err = runTrls(t, repo, "render-context", "--issue", "task-01")
	require.NoError(t, err)
}

func countHeartbeatOps(t *testing.T, repo string) int {
	t.Helper()
	allOps, err := readAllOpsFromDir(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
	require.NoError(t, err)
	n := 0
	for _, op := range allOps {
		if op.Type == ops.OpHeartbeat {
			n++
		}
	}
	return n
}

func countTransitionOps(t *testing.T, repo string) int {
	t.Helper()
	allOps, err := readAllOpsFromDir(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
	require.NoError(t, err)
	n := 0
	for _, op := range allOps {
		if op.Type == ops.OpTransition && op.TargetID == "task-01" {
			n++
		}
	}
	return n
}
