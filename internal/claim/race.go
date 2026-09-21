package claim

import "github.com/scullxbones/armature/internal/ops"

// replayDefaultTTLMinutes is applyClaim's fallback when a held lease recorded
// TTL <= 0. Distinct from IsClaimStale, which treats TTL <= 0 as never-expire.
const replayDefaultTTLMinutes = 60

// HeldClaim is the replay-visible lease ClaimLostRace inspects. It is not a
// materialize.Issue: this package must not import materialize.
type HeldClaim struct {
	Status                     string
	ClaimedBy                  string
	ClaimedAt                  int64
	LastHeartbeat              int64
	LastClaimingWorkerActivity int64
	TTLMinutes                 int
}

// ClaimLostRace reports whether a challenger's claim op is a no-op because
// another worker still holds a live claimed or in-progress lease.
//
// Sequential replay, not ResolveClaim: an earlier-timestamp challenger still
// loses if the currently held lease is not stale at the challenger's timestamp.
func ClaimLostRace(held HeldClaim, challengerID string, now int64) bool {
	if held.Status != ops.StatusClaimed && held.Status != ops.StatusInProgress {
		return false
	}
	if held.ClaimedBy == "" || held.ClaimedBy == challengerID {
		return false
	}
	ttl := held.TTLMinutes
	if ttl <= 0 {
		ttl = replayDefaultTTLMinutes
	}
	last := FoldLastActivity(held.ClaimedAt, held.LastHeartbeat, held.LastClaimingWorkerActivity)
	return !IsClaimStale(last, ttl, now)
}

// ClaimantHeartbeatClocks reports whether workerID may advance LastHeartbeat
// and LastClaimingWorkerActivity. Updated is always advanced by applyHeartbeat.
func ClaimantHeartbeatClocks(claimedBy, workerID string) bool {
	return workerID == claimedBy
}
