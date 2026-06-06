package claim

import (
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
)

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

// IsClaimStale checks if a claim has expired based on TTL and heartbeat.
func IsClaimStale(claimedAt, lastHeartbeat int64, ttlMinutes int, now int64) bool {
	if ttlMinutes <= 0 {
		return false
	}
	lastActivity := max(claimedAt, lastHeartbeat)
	ttlSeconds := int64(ttlMinutes) * 60
	return now > lastActivity+ttlSeconds
}
