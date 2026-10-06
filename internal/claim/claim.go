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

func hasOverlapDismissalNote(allOps []ops.Op, targetID, otherID string) bool {
	expectedMsg := fmt.Sprintf("Serial claim: scope overlap with %s (same worker, dismissed)", otherID)
	for _, op := range allOps {
		if op.Type == ops.OpNote && op.TargetID == targetID && op.Payload.Msg == expectedMsg {
			return true
		}
	}
	return false
}

// foldLastActivity collapses claimed-at, last heartbeat, and claiming-worker
// activity into the TTL clock (unix seconds).
func foldLastActivity(claimedAt, lastHeartbeat, claimingWorkerActivity int64) int64 {
	return max(claimedAt, lastHeartbeat, claimingWorkerActivity)
}

// isClaimStale reports whether last plus the replay TTL is at or before now.
// ttlMinutes <= 0 replays as DefaultReplayTTLMinutes. Takeable at exactly ttl.
func isClaimStale(last int64, ttlMinutes int, now int64) bool {
	ttlSeconds := int64(replayTTLMinutes(ttlMinutes)) * 60
	return now >= last+ttlSeconds
}

func ShouldHeartbeat(lastHeartbeatTime, now time.Time) bool {
	if lastHeartbeatTime.IsZero() {
		return true
	}
	return now.Sub(lastHeartbeatTime) >= HeartbeatDebounceInterval
}
