package claim

import (
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func liveHeld(worker string, claimedAt int64, ttl int) HeldClaim {
	return HeldClaim{
		Status:                     ops.StatusClaimed,
		ClaimedBy:                  worker,
		ClaimedAt:                  claimedAt,
		LastHeartbeat:              claimedAt,
		LastClaimingWorkerActivity: claimedAt,
		TTLMinutes:                 ttl,
	}
}

func TestForeignLiveLeaseBlocksChallenger_REQ_MATENC_S1_T2(t *testing.T) {
	t.Parallel()

	t.Run("different worker loses against live claimed lease", func(t *testing.T) {
		t.Parallel()
		assert.True(t, ForeignLiveLeaseBlocksChallenger(liveHeld("worker-a", 200, 60), "worker-b", 210))
	})

	t.Run("in-progress live lease also blocks a different worker", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("worker-a", 200, 60)
		held.Status = ops.StatusInProgress
		assert.True(t, ForeignLiveLeaseBlocksChallenger(held, "worker-b", 210))
	})

	t.Run("stale lease is not a lost race", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ForeignLiveLeaseBlocksChallenger(liveHeld("worker-a", 200, 60), "worker-b", 200+60*60+1))
	})

	t.Run("same worker is never a lost race", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ForeignLiveLeaseBlocksChallenger(liveHeld("worker-a", 200, 60), "worker-a", 210))
	})

	t.Run("empty holder is not a lost race", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("", 200, 60)
		assert.False(t, ForeignLiveLeaseBlocksChallenger(held, "worker-b", 210))
	})

	t.Run("open status is not a lost race", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("worker-a", 200, 60)
		held.Status = ops.StatusOpen
		assert.False(t, ForeignLiveLeaseBlocksChallenger(held, "worker-b", 210))
	})

	t.Run("blocked status is not a lost race", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("worker-a", 200, 60)
		held.Status = ops.StatusBlocked
		assert.False(t, ForeignLiveLeaseBlocksChallenger(held, "worker-b", 210))
	})

	t.Run("zero TTL uses replay default 60 minutes", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("worker-a", 100, 0)
		assert.True(t, ForeignLiveLeaseBlocksChallenger(held, "worker-b", 100+60*60-1),
			"legacy ttl 0 is live one second before the 60-minute default")
		assert.False(t, ForeignLiveLeaseBlocksChallenger(held, "worker-b", 100+60*60),
			"legacy ttl 0 is takeable at exactly 60 minutes")
		assert.True(t, IsClaimStale(FoldLastActivity(100, 100, 100), 0, 100+60*60),
			"IsClaimStale replays ttl 0 as 60 minutes")
	})

	t.Run("heartbeat and activity clocks fold into staleness", func(t *testing.T) {
		t.Parallel()
		held := HeldClaim{
			Status:                     ops.StatusClaimed,
			ClaimedBy:                  "worker-a",
			ClaimedAt:                  100,
			LastHeartbeat:              150,
			LastClaimingWorkerActivity: 0,
			TTLMinutes:                 1,
		}
		assert.True(t, ForeignLiveLeaseBlocksChallenger(held, "worker-b", 209))
		assert.False(t, ForeignLiveLeaseBlocksChallenger(held, "worker-b", 210))
	})

	t.Run("does not use earliest-timestamp winner", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("worker-a", 200, 60)
		assert.True(t, ForeignLiveLeaseBlocksChallenger(held, "worker-b", 100))
		_, took := Accept(LeaseFromClocks(
			held.Status, held.ClaimedBy, "", held.ClaimedAt, held.LastHeartbeat, held.LastClaimingWorkerActivity, held.TTLMinutes, "",
		), ops.Op{Type: ops.OpClaim, WorkerID: "worker-b", Timestamp: 100, Payload: ops.Payload{TTL: 60}})
		assert.False(t, took, "an earlier foreign timestamp does not steal a later live held lease")
	})
}

func TestClaimantHeartbeatClocks_REQ_MATENC_S1_T2(t *testing.T) {
	t.Parallel()

	t.Run("claimant advances clocks", func(t *testing.T) {
		t.Parallel()
		assert.True(t, ClaimantHeartbeatClocks("claimant", "claimant"))
	})

	t.Run("non-claimant does not advance clocks", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ClaimantHeartbeatClocks("claimant", "other-worker"))
	})

	t.Run("empty holder matches only empty worker", func(t *testing.T) {
		t.Parallel()
		assert.True(t, ClaimantHeartbeatClocks("", ""))
		assert.False(t, ClaimantHeartbeatClocks("", "worker-a"))
	})
}
