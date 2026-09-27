package claim

import (
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func racingLiveOps() []ops.Op {
	return []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 90, WorkerID: "worker-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 60, ClaimToken: "token-a"}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 110, WorkerID: "worker-b",
			Payload: ops.Payload{TTL: 60, ClaimToken: "token-b"}},
	}
}

func TestOwner_RacingLiveClaimsFirstKeeps_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	log := racingLiveOps()
	lease := Owner(log, "task-01")
	assert.Equal(t, "worker-a", lease.Holder)
	assert.Equal(t, "token-a", lease.Token)
	assert.True(t, LeaseLive(lease, 110))
	lost := LostRaceClaimKeys(log)
	assert.False(t, lost["task-01|worker-a"])
	assert.True(t, lost["task-01|worker-b"])
}

func TestAccept_ExactTTLBoundaryTakeable_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	claimedAt := int64(100)
	ttl := 1
	held := leaseFromClaimOp(ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: claimedAt, WorkerID: "worker-a",
		Payload: ops.Payload{TTL: ttl, ClaimToken: "a"},
	})
	liveAt := claimedAt + int64(ttl)*60 - 1
	assert.True(t, LeaseLive(held, liveAt))
	_, tookLive := Accept(held, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: liveAt, WorkerID: "worker-b",
		Payload: ops.Payload{TTL: 60, ClaimToken: "b"},
	})
	assert.False(t, tookLive, "lease is live at ttl-1s")

	takeAt := claimedAt + int64(ttl)*60
	assert.False(t, LeaseLive(held, takeAt))
	next, took := Accept(held, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: takeAt, WorkerID: "worker-b",
		Payload: ops.Payload{TTL: 60, ClaimToken: "b"},
	})
	assert.True(t, took, "takeable at exactly ttl")
	assert.Equal(t, "worker-b", next.Holder)
}

func TestValidateTTLMinutes_RejectsZeroAndNegative_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	err0 := ValidateTTLMinutes(0)
	require.Error(t, err0)
	assert.Contains(t, err0.Error(), "must be > 0 minutes")
	assert.Contains(t, err0.Error(), "ttl 0")
	errNeg := ValidateTTLMinutes(-5)
	require.Error(t, errNeg)
	assert.Contains(t, errNeg.Error(), "must be > 0 minutes")
	assert.Contains(t, errNeg.Error(), "ttl -5")
	_, err := NewClaimOp("task-01", "w", 1, 0, "", "tok")
	require.Error(t, err)
}

func TestOwner_LegacyZeroTTLReplaysAs60_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	claimedAt := int64(100)
	log := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: claimedAt, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 0, ClaimToken: "a"}},
	}
	lease := Owner(log, "task-01")
	assert.Equal(t, "worker-a", lease.Holder)
	assert.Equal(t, 0, lease.TTLMinutes, "recorded payload stays 0; replay length is 60")
	boundary := claimedAt + int64(DefaultReplayTTLMinutes)*60
	assert.True(t, LeaseLive(lease, boundary-1))
	assert.False(t, LeaseLive(lease, boundary))
	_, took := Accept(lease, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: boundary, WorkerID: "worker-b",
		Payload: ops.Payload{TTL: 60, ClaimToken: "b"},
	})
	assert.True(t, took)

	missingTTL := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: claimedAt, WorkerID: "worker-a",
			Payload: ops.Payload{ClaimToken: "a"}},
	}
	assert.False(t, LeaseLive(Owner(missingTTL, "task-01"), boundary))
}

func TestOwner_HeartbeatExtendsLease_REQ_CLAIMTTL(t *testing.T) {
	t.Parallel()
	log := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a",
			Payload: ops.Payload{TTL: 1, ClaimToken: "a"}},
		{Type: ops.OpHeartbeat, TargetID: "task-01", Timestamp: 150, WorkerID: "worker-a"},
		{Type: ops.OpHeartbeat, TargetID: "task-01", Timestamp: 150, WorkerID: "worker-b"},
	}
	lease := Owner(log, "task-01")
	assert.Equal(t, int64(150), lease.LastActivity)
	assert.True(t, LeaseLive(lease, 209))
	assert.False(t, LeaseLive(lease, 210))
}
