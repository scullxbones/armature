package oporder

import (
	"testing"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteDoesNotStealLivePublishedOwner_REQ_CLAIMORD_W13(t *testing.T) {
	t.Parallel()
	aClaim := ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
		Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}}
	aBeat := ops.Op{Type: ops.OpHeartbeat, TargetID: "task-01", Timestamp: 110, WorkerID: "worker-a"}
	bClaim := ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 120, WorkerID: "worker-b",
		Payload: ops.Payload{TTL: 60, ClaimToken: "tok-b"}}
	held := claim.ApplyAt(claim.Lease{}, aClaim, 1_700_000_000)
	held = claim.ApplyAt(held, aBeat, 1_700_000_010)
	assert.Equal(t, "tok-a", held.Token)
	assert.True(t, claim.LeaseLive(held, 1_700_000_020))
	next, took := claim.AcceptAt(held, bClaim, 1_700_000_020)
	assert.False(t, took)
	assert.Equal(t, "tok-a", next.Token)
	assert.Equal(t, "worker-a", next.Holder)
}

func TestRequireOwner_LoserNamedError(t *testing.T) {
	t.Parallel()
	located := []LocatedOp{
		{Op: ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "tok-a"}}, Published: true},
	}
	require.NoError(t, RequireOwner(located, "task-01", "worker-a"))
	err := RequireOwner(located, "task-01", "worker-b")
	require.Error(t, err)
	assert.ErrorIs(t, err, claim.ErrNotClaimOwner)
	assert.Equal(t, "NOT-CLAIM-OWNER", err.Error())
}
