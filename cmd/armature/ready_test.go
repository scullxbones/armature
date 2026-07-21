package main

import (
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ready"
)

// TestFilterExpiredClaimsByAssignedWorker_UsesAssignedWorkerNotClaimedBy proves
// that an expired claim on an issue assigned to worker-a but claimed by
// worker-b shows up under --assigned-to worker-a (not worker-b), matching the
// AssignedWorker-based filtering ready.FilterByAssignedTo uses for the main
// ready list.
func TestFilterExpiredClaimsByAssignedWorker_UsesAssignedWorkerNotClaimedBy(t *testing.T) {
	t.Parallel()

	expiredClaims := []ready.ExpiredClaimEntry{
		{Issue: "task-01", ClaimedBy: "worker-b"},
		{Issue: "task-02", ClaimedBy: "worker-a"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {AssignedWorker: "worker-a", ClaimedBy: "worker-b"},
		"task-02": {AssignedWorker: "worker-c", ClaimedBy: "worker-a"},
	}

	got := filterExpiredClaimsByAssignedWorker(expiredClaims, issues, "worker-a")

	if len(got) != 1 || got[0].Issue != "task-01" {
		t.Fatalf("expected only task-01 (assigned to worker-a, regardless of claimant), got %+v", got)
	}
}

func TestFilterExpiredClaimsByAssignedWorker_MissingIssueExcluded(t *testing.T) {
	t.Parallel()

	expiredClaims := []ready.ExpiredClaimEntry{
		{Issue: "task-missing", ClaimedBy: "worker-a"},
	}
	issues := map[string]*materialize.Issue{}

	got := filterExpiredClaimsByAssignedWorker(expiredClaims, issues, "worker-a")

	if len(got) != 0 {
		t.Fatalf("expected no entries for an issue with no materialized state, got %+v", got)
	}
}
