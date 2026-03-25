package ready

import (
	"testing"
	"time"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestStaleClaims_EmptyWhenNoClaims(t *testing.T) {
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: ops.StatusOpen},
		"task-02": {ID: "task-02", Status: ops.StatusInProgress},
	}
	now := time.Unix(1000, 0)
	result := StaleClaims(issues, now)
	assert.Empty(t, result)
}

func TestStaleClaims_ReturnsStaleClaimed(t *testing.T) {
	// claimed at t=0, TTL=1min (60s), now=t+200 → stale
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID:        "task-01",
			Status:    ops.StatusClaimed,
			ClaimedBy: "worker-a",
			ClaimedAt: 0,
			ClaimTTL:  1,
		},
	}
	now := time.Unix(200, 0)
	result := StaleClaims(issues, now)
	assert.Equal(t, []string{"task-01"}, result)
}

func TestStaleClaims_DoesNotReturnFreshClaim(t *testing.T) {
	// claimed at t=0, TTL=5min (300s), now=t+100 → fresh
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID:        "task-01",
			Status:    ops.StatusClaimed,
			ClaimedBy: "worker-a",
			ClaimedAt: 0,
			ClaimTTL:  5,
		},
	}
	now := time.Unix(100, 0)
	result := StaleClaims(issues, now)
	assert.Empty(t, result)
}

func TestStaleClaims_DoesNotReturnNonClaimedStatus(t *testing.T) {
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID:        "task-01",
			Status:    ops.StatusInProgress,
			ClaimedBy: "worker-a",
			ClaimedAt: 0,
			ClaimTTL:  1,
		},
	}
	now := time.Unix(9999, 0)
	result := StaleClaims(issues, now)
	assert.Empty(t, result)
}

func TestStaleClaims_HeartbeatExtendsTTL(t *testing.T) {
	// claimed at 0, heartbeat at 500, TTL=1min (60s)
	// without heartbeat: stale at now>60
	// with heartbeat: not stale until now>560
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID:            "task-01",
			Status:        ops.StatusClaimed,
			ClaimedBy:     "worker-a",
			ClaimedAt:     0,
			ClaimTTL:      1,
			LastHeartbeat: 500,
		},
	}
	// now=530 → not yet stale (500+60=560)
	assert.Empty(t, StaleClaims(issues, time.Unix(530, 0)))
	// now=561 → stale
	assert.Equal(t, []string{"task-01"}, StaleClaims(issues, time.Unix(561, 0)))
}

func TestStaleClaims_MultipleIssues_ReturnOnlyStale(t *testing.T) {
	issues := map[string]*materialize.Issue{
		"task-stale": {
			ID:        "task-stale",
			Status:    ops.StatusClaimed,
			ClaimedBy: "worker-a",
			ClaimedAt: 0,
			ClaimTTL:  1,
		},
		"task-fresh": {
			ID:        "task-fresh",
			Status:    ops.StatusClaimed,
			ClaimedBy: "worker-b",
			ClaimedAt: 0,
			ClaimTTL:  60,
		},
		"task-open": {
			ID:     "task-open",
			Status: ops.StatusOpen,
		},
	}
	now := time.Unix(200, 0)
	result := StaleClaims(issues, now)
	assert.Len(t, result, 1)
	assert.Equal(t, "task-stale", result[0])
}
