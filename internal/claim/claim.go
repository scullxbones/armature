// Package claim resolves task claim ownership from the op log, detecting stale claims
// (expired TTL with no heartbeat) and overlapping claims across concurrent workers.
package claim

import (
	"fmt"
	"time"

	"github.com/scullxbones/armature/internal/ops"
)

// HeartbeatDebounceInterval is the fixed debounce interval for rate-limited
// heartbeat emission from the harness hook. Heartbeats are emitted at most
// once per interval, independent of claim TTL. Not configurable.
const HeartbeatDebounceInterval = 5 * time.Minute

// ResolveClaim resolves a claim race: earliest timestamp wins,
// lexicographic worker ID as tiebreaker.
func ResolveClaim(claims []ops.Op) ops.Op {
	if len(claims) == 0 {
		return ops.Op{}
	}
	winner := claims[0]
	for _, c := range claims[1:] {
		if c.Timestamp < winner.Timestamp ||
			(c.Timestamp == winner.Timestamp && c.WorkerID < winner.WorkerID) {
			winner = c
		}
	}
	return winner
}

// HasOverlapDismissalNote checks if a same-worker overlap dismissal note
// for the given issue pair already exists in the ops history.
// Returns true if a note with the message pattern "Serial claim: scope overlap with {otherId} (same worker, dismissed)"
// is found on the targetID.
func HasOverlapDismissalNote(allOps []ops.Op, targetID, otherID string) bool {
	expectedMsg := fmt.Sprintf("Serial claim: scope overlap with %s (same worker, dismissed)", otherID)
	for _, op := range allOps {
		if op.Type == ops.OpNote && op.TargetID == targetID && op.Payload.Msg == expectedMsg {
			return true
		}
	}
	return false
}

// IsClaimStale checks if a claim has expired based on TTL, heartbeat, and
// claimant-attributed activity. claimingWorkerActivity should be the claim's
// LastClaimingWorkerActivity value (materialize.Issue.LastClaimingWorkerActivity)
// when available, or 0 when the caller has no such value (max() with 0 is a
// no-op, preserving prior behavior at call sites that can't source it).
//
// materialize.Issue.ClaimStale delegates here so ready, doctor --fix, and
// claim races share one formula: a claim transitioned by its claimant (e.g.
// claimed->in-progress) moments before the naive TTL window closes bumps
// LastClaimingWorkerActivity without touching LastHeartbeat, which is reserved
// for explicit heartbeat ops. Folding claimingWorkerActivity into the
// staleness calculation prevents that fresh transition from being read as an
// expired claim.
func IsClaimStale(claimedAt, lastHeartbeat, claimingWorkerActivity int64, ttlMinutes int, now int64) bool {
	if ttlMinutes <= 0 {
		return false
	}
	lastActivity := max(claimedAt, lastHeartbeat, claimingWorkerActivity)
	ttlSeconds := int64(ttlMinutes) * 60
	return now > lastActivity+ttlSeconds
}

// ShouldHeartbeat checks if a heartbeat should be emitted based on the fixed
// debounce interval. Returns true if lastHeartbeatTime is zero (no heartbeat yet)
// or if enough time has passed since the last heartbeat (>= HeartbeatDebounceInterval).
// The decision logic is pure and takes no I/O.
func ShouldHeartbeat(lastHeartbeatTime, now time.Time) bool {
	if lastHeartbeatTime.IsZero() {
		return true
	}
	return now.Sub(lastHeartbeatTime) >= HeartbeatDebounceInterval
}
