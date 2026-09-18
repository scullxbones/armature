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

// LastActivity is the claim TTL clock in unix seconds. It is the max of
// claimed-at, last heartbeat, and claiming-worker activity.
type LastActivity int64

func (a LastActivity) String() string {
	return fmt.Sprintf("unix:%d", int64(a))
}

// FoldLastActivity collapses the three claim clocks into one LastActivity.
func FoldLastActivity(claimedAt, lastHeartbeat, claimingWorkerActivity int64) LastActivity {
	return LastActivity(max(claimedAt, lastHeartbeat, claimingWorkerActivity))
}

// IsClaimStale reports whether last plus TTL is strictly before now.
// ttlMinutes is converted to seconds. ttlMinutes <= 0 never expires.
func IsClaimStale(last LastActivity, ttlMinutes int, now int64) bool {
	if ttlMinutes <= 0 {
		return false
	}
	ttlSeconds := int64(ttlMinutes) * 60
	return now > int64(last)+ttlSeconds
}

func ShouldHeartbeat(lastHeartbeatTime, now time.Time) bool {
	if lastHeartbeatTime.IsZero() {
		return true
	}
	return now.Sub(lastHeartbeatTime) >= HeartbeatDebounceInterval
}
