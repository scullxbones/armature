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

func TestClaimLostRace_REQ_MATENC_S1_T2(t *testing.T) {
	t.Parallel()

	t.Run("different worker loses against live claimed lease", func(t *testing.T) {
		t.Parallel()
		assert.True(t, ClaimLostRace(liveHeld("worker-a", 200, 60), "worker-b", 210))
	})

	t.Run("in-progress live lease also blocks a different worker", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("worker-a", 200, 60)
		held.Status = ops.StatusInProgress
		assert.True(t, ClaimLostRace(held, "worker-b", 210))
	})

	t.Run("stale lease is not a lost race", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ClaimLostRace(liveHeld("worker-a", 200, 60), "worker-b", 200+60*60+1))
	})

	t.Run("same worker is never a lost race", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ClaimLostRace(liveHeld("worker-a", 200, 60), "worker-a", 210))
	})

	t.Run("empty holder is not a lost race", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("", 200, 60)
		assert.False(t, ClaimLostRace(held, "worker-b", 210))
	})

	t.Run("open status is not a lost race", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("worker-a", 200, 60)
		held.Status = ops.StatusOpen
		assert.False(t, ClaimLostRace(held, "worker-b", 210))
	})

	t.Run("blocked status is not a lost race", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("worker-a", 200, 60)
		held.Status = ops.StatusBlocked
		assert.False(t, ClaimLostRace(held, "worker-b", 210))
	})

	t.Run("zero TTL uses replay default 60 minutes not never-expire", func(t *testing.T) {
		t.Parallel()
		held := liveHeld("worker-a", 100, 0)
		assert.True(t, ClaimLostRace(held, "worker-b", 100+60*60),
			"exact default-TTL boundary is still live (IsClaimStale is strict-after)")
		assert.False(t, ClaimLostRace(held, "worker-b", 100+60*60+1),
			"TTL<=0 on a held lease defaults to 60 minutes at replay, unlike IsClaimStale")
		assert.False(t, IsClaimStale(FoldLastActivity(100, 100, 100), 0, 100+60*60+1),
			"sanity: IsClaimStale TTL<=0 never expires — ClaimLostRace must not copy that")
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
		assert.True(t, ClaimLostRace(held, "worker-b", 209))
		assert.False(t, ClaimLostRace(held, "worker-b", 211))
	})

	t.Run("does not unify with ResolveClaim earliest-timestamp winner", func(t *testing.T) {
		t.Parallel()
		// Held lease applied at t=200; challenger op is timestamped earlier.
		// ResolveClaim would pick the earlier op; sequential replay keeps the
		// live holder.
		held := liveHeld("worker-a", 200, 60)
		assert.True(t, ClaimLostRace(held, "worker-b", 100))
		winner := ResolveClaim([]ops.Op{
			{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a"},
			{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b"},
		})
		assert.Equal(t, "worker-b", winner.WorkerID)
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
