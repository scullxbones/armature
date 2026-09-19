package main

import (
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkersEmitsEnvelopeNotJSONL_REQ_AOC_S2_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree")
	require.NoError(t, err)

	for _, extra := range [][]string{
		{"--format", "json"},
		{"--format", "agent"},
		{"--json"},
	} {
		args := append([]string{"workers"}, extra...)
		out, err := runTrls(t, repo, args...)
		require.NoError(t, err, "args=%v", extra)

		decoded := decodeContractEnvelope(t, out, "workers")
		var workers []WorkerStatus
		require.NoError(t, json.Unmarshal(decoded["workers"], &workers))
		require.NotEmpty(t, workers)
		assert.NotEmpty(t, workers[0].WorkerID)
		assert.NotEmpty(t, workers[0].Status)
	}
}

func TestClaimingWorkerActivityIfAuthorOwnsLease(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int64(100), claimingWorkerActivityIfAuthorOwnsLease("worker-b", "worker-a", 500, 100))
	assert.Equal(t, int64(500), claimingWorkerActivityIfAuthorOwnsLease("worker-a", "worker-a", 500, 100))
	assert.Equal(t, int64(100), claimingWorkerActivityIfAuthorOwnsLease("worker-a", "worker-a", 50, 100))
}

func TestFoldWorkerStatusFromClaimOwnerActivity_ForeignTransitionDoesNotExtendLease(t *testing.T) {
	t.Parallel()
	now := int64(10000)
	allOps := []ops.Op{
		{Type: ops.OpClaim, TargetID: "T-001", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 10}},
		{Type: ops.OpTransition, TargetID: "T-001", Timestamp: 9800, WorkerID: "worker-b",
			Payload: ops.Payload{To: "in-progress"}},
	}
	status := foldWorkerStatusFromClaimOwnerActivity("worker-a", allOps, 60, now, map[string]string{"T-001": "worker-a"})
	assert.Equal(t, "stale", status.Status)
	assert.Empty(t, status.ActiveIssue)
}
