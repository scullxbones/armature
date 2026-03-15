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
	assert.True(t, IsClaimStale(100, 0, 60, 161))
	assert.False(t, IsClaimStale(100, 0, 60, 159))
	assert.False(t, IsClaimStale(100, 150, 60, 200))
	assert.True(t, IsClaimStale(100, 150, 60, 211))
}

func TestScopeOverlap(t *testing.T) {
	assert.True(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{"src/auth/**"}, []string{"src/api/handler.go"}))
	assert.True(t, ScopesOverlap([]string{"src/**"}, []string{"src/auth/login.go"}))
	assert.False(t, ScopesOverlap([]string{}, []string{"src/auth/login.go"}))
}
