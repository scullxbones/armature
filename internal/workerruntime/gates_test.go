package workerruntime

import (
	"context"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/ready"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollAdapterStartsCooldownWhenReadyQueueIsEmpty(t *testing.T) {
	policy := DefaultPolicy()
	policy.Cooldown.NoReadyWorkDelay = 45 * time.Second

	result, err := PollAdapter(context.Background(), PollInput{
		WorkerID: "worker-1",
		Policy:   policy,
	}, StaticReadyQueue{Entries: nil})
	require.NoError(t, err)
	assert.Equal(t, PollNoReadyWork, result.Outcome)
	assert.Equal(t, StateIdle, result.NextState)
	assert.Equal(t, 45*time.Second, result.RecheckAfter)
	assert.Nil(t, result.Candidate)
}

func TestClaimGateRejectsInferredNodesAndClassifiesClaimLoss(t *testing.T) {
	ctx := context.Background()

	blocked, err := ClaimGate(ctx, ClaimGateInput{
		WorkerID:   "worker-1",
		TTLMinutes: 60,
		IssueID:    "task-inferred",
	}, StaticClaimer{}, StaticIssueMeta{
		Values: map[string]string{"task-inferred": "inferred"},
	})
	require.NoError(t, err)
	assert.Equal(t, ClaimBlocked, blocked.Outcome)
	assert.Equal(t, "inferred_node", blocked.ReasonCode)
	assert.Equal(t, StateClaimLost, blocked.NextState)

	lost, err := ClaimGate(ctx, ClaimGateInput{
		WorkerID:   "worker-1",
		TTLMinutes: 60,
		IssueID:    "task-race",
	}, StaticClaimer{Err: ErrClaimConflict}, StaticIssueMeta{
		Values: map[string]string{"task-race": "verified"},
	})
	require.NoError(t, err)
	assert.Equal(t, ClaimLost, lost.Outcome)
	assert.Equal(t, "claim_conflict", lost.ReasonCode)
	assert.Equal(t, StateClaimLost, lost.NextState)
}

func TestPollAdapterReturnsReadyCandidate(t *testing.T) {
	entry := ready.ReadyEntry{Issue: "task-1", Title: "Task 1"}
	result, err := PollAdapter(context.Background(), PollInput{
		WorkerID: "worker-1",
		Policy:   DefaultPolicy(),
	}, StaticReadyQueue{Entries: []ready.ReadyEntry{entry}})
	require.NoError(t, err)
	assert.Equal(t, PollReadyWork, result.Outcome)
	require.NotNil(t, result.Candidate)
	candidate, ok := result.Candidate.(*ready.ReadyEntry)
	require.True(t, ok)
	assert.Equal(t, "task-1", candidate.Issue)
}
