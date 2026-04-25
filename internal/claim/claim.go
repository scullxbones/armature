package claim

import (
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

// IsClaimStale checks if a claim has expired based on TTL and heartbeat.
func IsClaimStale(claimedAt, lastHeartbeat int64, ttlMinutes int, now int64) bool {
	if ttlMinutes <= 0 {
		return false
	}
	lastActivity := claimedAt
	if lastHeartbeat > lastActivity {
		lastActivity = lastHeartbeat
	}
	ttlSeconds := int64(ttlMinutes) * 60
	return now > lastActivity+ttlSeconds
}
