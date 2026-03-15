package claim

import (
	"testing"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestResolveClaimRace_FirstTimestampWins(t *testing.T) {
	claims := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-b"},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
	}
	winner := ResolveClaim(claims)
	assert.Equal(t, "worker-a", winner.WorkerID)
}

func TestResolveClaimRace_LexicographicTiebreaker(t *testing.T) {
	claims := []ops.Op{
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b"},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a"},
	}
	winner := ResolveClaim(claims)
	assert.Equal(t, "worker-a", winner.WorkerID)
}

func TestIsClaimStale(t *testing.T) {
	// TTL=1 minute = 60 seconds; claimedAt=100, now=161 => stale (100+60=160 < 161)
	assert.True(t, IsClaimStale(100, 0, 1, 161))
	// now=159 => not stale (100+60=160 > 159)
	assert.False(t, IsClaimStale(100, 0, 1, 159))
	// heartbeat at 150, now=209 => not stale (150+60=210 > 209)
	assert.False(t, IsClaimStale(100, 150, 1, 209))
	// heartbeat at 150, now=211 => stale (150+60=210 < 211)
	assert.True(t, IsClaimStale(100, 150, 1, 211))
	// TTL=0 => never stale
	assert.False(t, IsClaimStale(100, 0, 0, 9999))
}

func TestScopeOverlap(t *testing.T) {
	assert.True(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/api/handler.go"}))
	assert.True(t, ScopesOverlap([]string{"src/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{}, []string{"src/auth/login.go"}))
}
