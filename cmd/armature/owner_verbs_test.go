package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/ops"
)

func TestLoserHeartbeatFailsNamedError_REQ_CLAIMORD_W13(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	before := countHeartbeatOps(t, repo)

	_, err = runTrls(t, repo, "heartbeat", "--issue", "task-01", "--worker-id", "loser-worker-w13")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT-CLAIM-OWNER")
	assert.Equal(t, before, countHeartbeatOps(t, repo), "loser must not append a heartbeat")
}

func TestLoserTransitionFailsNamedError_REQ_CLAIMORD_W13(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)
	before := countTransitionOps(t, repo, "task-01")

	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done",
		"--skip-delivery-gate", "--force", "--outcome", "should fail", "--worker-id", "loser-worker-w13")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT-CLAIM-OWNER")
	assert.Equal(t, before, countTransitionOps(t, repo, "task-01"), "loser must not mark done")
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

func countTransitionOps(t *testing.T, repo string, issueID string) int {
	t.Helper()
	allOps, err := readAllOpsFromDir(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
	require.NoError(t, err)
	n := 0
	for _, op := range allOps {
		if op.Type == ops.OpTransition && op.TargetID == issueID {
			n++
		}
	}
	return n
}
