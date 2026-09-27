package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/config"
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

func maxAcceptedTTLMinutes() int64 {
	max := math.MaxInt64 / int64(time.Minute)
	doubledSeconds := int64(math.MaxInt64 / (2 * 60))
	if doubledSeconds < max {
		max = doubledSeconds
	}
	return max
}

func TestIdleWindowAtMaxAcceptedTTL_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	maxMinutes := maxAcceptedTTLMinutes()
	if maxMinutes > int64(math.MaxInt) {
		t.Skip("TTLMinutes cannot hold the duration-safe bound on this platform")
	}

	problems := config.ValidatePresentFields([]byte(`{"default_ttl":` + strconv.FormatInt(maxMinutes, 10) + `}`))
	require.Empty(t, problems, "largest representable default_ttl must be accepted")

	now := int64(1000)
	allOps := []ops.Op{
		{Type: ops.OpNote, TargetID: "T-001", Timestamp: now - 1, WorkerID: "worker-a"},
	}
	status := foldWorkerStatusFromClaimOwnerActivity("worker-a", allOps, allOps, config.TTLMinutes(maxMinutes), now)
	assert.Equal(t, "idle", status.Status)
	idleWindowSeconds := 2 * maxMinutes * 60
	assert.Positive(t, idleWindowSeconds)
	assert.LessOrEqual(t, now-(now-1), idleWindowSeconds)

	problems = config.ValidatePresentFields([]byte(`{"default_ttl":` + strconv.FormatInt(maxMinutes+1, 10) + `}`))
	require.NotEmpty(t, problems, "one minute past the representable bound must be rejected")
	assert.Contains(t, fmt.Sprintf("%v", problems), "default_ttl")
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
	status := foldWorkerStatusFromClaimOwnerActivity("worker-a", allOps, allOps, 60, now)
	assert.Equal(t, "stale", status.Status)
	assert.Empty(t, status.ActiveIssue)
}
