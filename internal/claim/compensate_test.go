package claim_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func liveSameWorkerInput() claim.CompensationInput {
	return claim.CompensationInput{
		Prior: claim.LeaseFacts{
			Status:                 ops.StatusInProgress,
			ClaimedBy:              "worker-a",
			ClaimedAt:              100,
			LastHeartbeat:          140,
			ClaimTTL:               1,
			ClaimingWorkerActivity: 145,
			WorktreePath:           "/repo/.worktrees/TASK-1",
			ClaimToken:             "prior-token",
		},
		WorkerID:     "worker-a",
		Now:          150,
		IfClaimToken: "won-token",
	}
}

func TestPlanCompensation_RestoreLiveSameWorker_REQ_ARCHIMP_S20_T3(t *testing.T) {
	t.Parallel()

	in := liveSameWorkerInput()
	got, err := claim.PlanCompensation(in)
	require.NoError(t, err)

	assert.Equal(t, ops.StatusInProgress, got.To)
	assert.True(t, got.RestoreClaim)
	assert.Equal(t, in.Prior.ClaimedBy, got.RestoreClaimedBy)
	assert.Equal(t, in.Prior.ClaimedAt, got.RestoreClaimedAt)
	assert.Equal(t, in.Prior.ClaimTTL, got.RestoreClaimTTL)
	assert.Equal(t, in.Prior.LastHeartbeat, got.RestoreLastHeartbeat)
	assert.Equal(t, in.Prior.ClaimingWorkerActivity, got.RestoreLastClaimingWorkerActivity)
	assert.Equal(t, in.Prior.ClaimToken, got.RestoreClaimToken)
	assert.Equal(t, in.Prior.WorktreePath, got.WorktreePath)
	assert.False(t, got.ClearWorktreePath)
	assert.Equal(t, in.IfClaimToken, got.IfClaimToken)
}

func TestPlanCompensation_ReleaseStaleSameWorker_REQ_ARCHIMP_S20_T3(t *testing.T) {
	t.Parallel()

	in := liveSameWorkerInput()
	in.Now = 206
	got, err := claim.PlanCompensation(in)
	require.NoError(t, err)

	assert.Equal(t, ops.StatusOpen, got.To)
	assert.True(t, got.RestoreClaim)
	assert.Empty(t, got.RestoreClaimedBy)
	assert.Zero(t, got.RestoreClaimedAt)
	assert.Zero(t, got.RestoreClaimTTL)
	assert.Zero(t, got.RestoreLastHeartbeat)
	assert.Zero(t, got.RestoreLastClaimingWorkerActivity)
	assert.Empty(t, got.RestoreClaimToken)
	assert.Equal(t, in.Prior.WorktreePath, got.WorktreePath)
	assert.False(t, got.ClearWorktreePath)
	assert.Equal(t, in.IfClaimToken, got.IfClaimToken)
}

func TestPlanCompensation_ReleaseForeign_REQ_ARCHIMP_S20_T3(t *testing.T) {
	t.Parallel()

	in := liveSameWorkerInput()
	in.Prior.ClaimedBy = "worker-b"
	got, err := claim.PlanCompensation(in)
	require.NoError(t, err)

	assert.Equal(t, ops.StatusOpen, got.To)
	assert.True(t, got.RestoreClaim)
	assert.Empty(t, got.RestoreClaimedBy)
	assert.Zero(t, got.RestoreClaimedAt)
	assert.Zero(t, got.RestoreClaimTTL)
	assert.Zero(t, got.RestoreLastHeartbeat)
	assert.Zero(t, got.RestoreLastClaimingWorkerActivity)
	assert.Empty(t, got.RestoreClaimToken)
	assert.Equal(t, in.IfClaimToken, got.IfClaimToken)
}

func TestPlanCompensation_ClearsEmptyWorktreePath_REQ_ARCHIMP_S20_T3(t *testing.T) {
	t.Parallel()

	in := liveSameWorkerInput()
	in.Prior.WorktreePath = ""
	got, err := claim.PlanCompensation(in)
	require.NoError(t, err)

	assert.Empty(t, got.WorktreePath)
	assert.True(t, got.ClearWorktreePath)
	assert.Equal(t, ops.StatusInProgress, got.To)
}

func TestPlanCompensation_SetsIfClaimToken_REQ_ARCHIMP_S20_T3(t *testing.T) {
	t.Parallel()

	in := liveSameWorkerInput()
	in.Prior.ClaimedBy = "worker-b"
	in.IfClaimToken = "token-of-won-claim"
	got, err := claim.PlanCompensation(in)
	require.NoError(t, err)

	assert.Equal(t, "token-of-won-claim", got.IfClaimToken)
	assert.NotEqual(t, in.Prior.ClaimToken, got.IfClaimToken)
}

func TestPlanCompensation_InvalidInput_REQ_ARCHIMP_S20_T3(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   claim.CompensationInput
	}{
		{name: "empty worker ID", in: claim.CompensationInput{IfClaimToken: "tok"}},
		{name: "empty claim token", in: claim.CompensationInput{WorkerID: "worker-a"}},
		{name: "both empty", in: claim.CompensationInput{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.NotPanics(t, func() {
				got, err := claim.PlanCompensation(tc.in)
				require.Error(t, err)
				assert.Equal(t, ops.Payload{}, got)
			})
		})
	}
}

func TestPlanCompensation_InputsUnchanged_REQ_ARCHIMP_S20_T3(t *testing.T) {
	t.Parallel()

	in := liveSameWorkerInput()
	before := in
	_, err := claim.PlanCompensation(in)
	require.NoError(t, err)
	assert.Equal(t, before, in)
}
