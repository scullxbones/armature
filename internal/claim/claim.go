// Package claim resolves task claim ownership from the op log, detecting stale claims
// (expired TTL with no heartbeat) and overlapping claims across concurrent workers.
package claim

import (
	"fmt"
	"time"
)

// HeartbeatDebounceInterval is the fixed debounce interval for rate-limited
// heartbeat emission from the harness hook. Heartbeats are emitted at most
// once per interval, independent of claim TTL. Not configurable.
const HeartbeatDebounceInterval = 5 * time.Minute

func sameWorkerDismissalMsg(otherID string) string {
	return fmt.Sprintf("Serial claim: scope overlap with %s (same worker, dismissed)", otherID)
}

func foldLastActivity(claimedAt, lastHeartbeat, claimingWorkerActivity int64) int64 {
	return max(claimedAt, lastHeartbeat, claimingWorkerActivity)
}

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
