package ready

import (
	"fmt"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/stretchr/testify/assert"
)

func TestReadyTask_AllRulesMet(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story", Children: []string{"task-01"}},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{}},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01"},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 1)
	assert.Equal(t, "task-01", ready[0].Issue)
}

func TestReadyTask_BlockerNotMerged(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
		"task-02":  {Status: "done", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 0)
}

func TestReadyTask_BlockerMerged(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
		"task-02":  {Status: "merged", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 1)
}

func TestReadyTask_ParentClaimed_AppearsInQueue(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "claimed", Type: "story", Children: []string{"task-01"}},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{}},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01"},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 1, "task should be ready when parent story is claimed")
	assert.Equal(t, "task-01", ready[0].Issue)
}

func TestReadyTask_ParentNotInProgress(t *testing.T) {
	// After the bootstrap-deadlock fix, open parent IS allowed — task should be ready.
	index := materialize.Index{
		"story-01": {Status: "open", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01"},
	}
	ready := ComputeReady(index, issues, "")
	found := false
	for _, r := range ready {
		if r.Issue == "task-01" {
			found = true
		}
	}
	assert.True(t, found, "task-01 should be ready: open parent is now allowed (bootstrap fix)")
}

func TestComputeReady_SurfacesTaskWithOpenParent(t *testing.T) {
	// Regression test: tasks whose story parent is "open" must appear in the ready queue.
	// Previously they were gated out, causing a bootstrap deadlock in fresh sessions.
	index := materialize.Index{
		"story-01": {Status: "open", Type: "story", Children: []string{"task-01"}},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{}},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01"},
	}
	ready := ComputeReady(index, issues, "")
	found := false
	for _, r := range ready {
		if r.Issue == "task-01" {
			found = true
		}
	}
	assert.True(t, found, "task with open parent story should appear in ready queue")
}

func TestReadyTask_NoParent(t *testing.T) {
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task"},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 1)
}

func TestReadyTask_InferredRequiresConfirmation(t *testing.T) {
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task",
			Provenance: materialize.Provenance{Confidence: "inferred"}},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 1)
	assert.True(t, ready[0].RequiresConfirmation)
}

func TestReadyStory_NoParent_AppearsInQueue(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "open", Type: "story"},
	}
	issues := map[string]*materialize.Issue{
		"story-01": {ID: "story-01", Status: "open", Type: "story"},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 1)
	assert.Equal(t, "story-01", ready[0].Issue)
}

func TestReadyStory_ParentInProgress_AppearsInQueue(t *testing.T) {
	index := materialize.Index{
		"epic-01":  {Status: "in-progress", Type: "feature"},
		"story-01": {Status: "open", Type: "story", Parent: "epic-01"},
	}
	issues := map[string]*materialize.Issue{
		"story-01": {ID: "story-01", Status: "open", Type: "story", Parent: "epic-01"},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 1)
	assert.Equal(t, "story-01", ready[0].Issue)
}

func TestReadyTask_PrioritySort(t *testing.T) {
	index := materialize.Index{
		"task-a": {Status: "open", Type: "task"},
		"task-b": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-a": {ID: "task-a", Status: "open", Type: "task", Priority: "medium"},
		"task-b": {ID: "task-b", Status: "open", Type: "task", Priority: "high"},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 2)
	assert.Equal(t, "task-b", ready[0].Issue)
}

func TestReadyTask_AssignedToMeFirst(t *testing.T) {
	index := materialize.Index{
		"task-a": {Status: "open", Type: "task", AssignedWorker: "other-worker"},
		"task-b": {Status: "open", Type: "task", AssignedWorker: ""},
		"task-c": {Status: "open", Type: "task", AssignedWorker: "my-worker"},
	}
	issues := map[string]*materialize.Issue{
		"task-a": {ID: "task-a", Status: "open", Type: "task"},
		"task-b": {ID: "task-b", Status: "open", Type: "task"},
		"task-c": {ID: "task-c", Status: "open", Type: "task"},
	}
	ready := ComputeReady(index, issues, "my-worker")
	assert.Len(t, ready, 3)
	// assigned-to-me first
	assert.Equal(t, "task-c", ready[0].Issue)
	// unassigned second
	assert.Equal(t, "task-b", ready[1].Issue)
	// other-assigned last
	assert.Equal(t, "task-a", ready[2].Issue)
}

func TestReadyTask_NoWorkerID_NoAssignmentOrdering(t *testing.T) {
	index := materialize.Index{
		"task-a": {Status: "open", Type: "task", AssignedWorker: "some-worker"},
		"task-b": {Status: "open", Type: "task", AssignedWorker: ""},
	}
	issues := map[string]*materialize.Issue{
		"task-a": {ID: "task-a", Status: "open", Type: "task"},
		"task-b": {ID: "task-b", Status: "open", Type: "task"},
	}
	// No workerID — both treated as tier 1 (unassigned), falls back to ID sort
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 2)
	// With no workerID, assignment tier is 1 for all, so sort falls back to ID
	assert.Equal(t, "task-a", ready[0].Issue)
	assert.Equal(t, "task-b", ready[1].Issue)
}

func TestIsClaimStale_ExactBoundary_NotStale(t *testing.T) {
	// claimedAt=0, ttl=1min, now=60 — exactly at boundary, should NOT be stale
	assert.False(t, isClaimStale(0, 0, 1, 60), "at exact TTL boundary should not be stale")
}

func TestIsClaimStale_OnePastBoundary_IsStale(t *testing.T) {
	// now=61 (1 second past 1-minute TTL)
	assert.True(t, isClaimStale(0, 0, 1, 61))
}

func TestIsClaimStale_ZeroTTL_NeverStale(t *testing.T) {
	assert.False(t, isClaimStale(0, 0, 0, 99999))
}

func TestIsClaimStale_HeartbeatExtends(t *testing.T) {
	// Claimed at 0, heartbeat at 100, TTL=1min
	// Without heartbeat: stale at now=61
	// With heartbeat: not stale until now=160
	assert.False(t, isClaimStale(0, 100, 1, 130))
	assert.True(t, isClaimStale(0, 100, 1, 161))
}

func TestReadyTask_DraftConfidence_ExcludedFromReady(t *testing.T) {
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task",
			Provenance: materialize.Provenance{Confidence: "draft"}},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 0, "draft task should be excluded from ready queue")
}

func TestReadyTask_VerifiedConfidence_IncludedInReady(t *testing.T) {
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task",
			Provenance: materialize.Provenance{Confidence: "verified"}},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 1, "verified task should appear in ready queue")
	assert.Equal(t, "task-01", ready[0].Issue)
}

func TestReadyTask_NoConfidenceField_DefaultsToVerified(t *testing.T) {
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	// Issue with empty confidence — should default to verified (appear in ready)
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task",
			Provenance: materialize.Provenance{Confidence: ""}},
	}
	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 1, "task with no confidence field should default to verified and appear in ready queue")
	assert.Equal(t, "task-01", ready[0].Issue)
}

func TestFilterByAssignedTo_ReturnsMatchingEntries(t *testing.T) {
	entries := []ReadyEntry{
		{Issue: "task-a", AssignedWorker: "worker-x"},
		{Issue: "task-b", AssignedWorker: "worker-y"},
		{Issue: "task-c", AssignedWorker: "worker-x"},
		{Issue: "task-d", AssignedWorker: ""},
	}
	result := FilterByAssignedTo(entries, "worker-x")
	assert.Len(t, result, 2)
	ids := []string{result[0].Issue, result[1].Issue}
	assert.Contains(t, ids, "task-a")
	assert.Contains(t, ids, "task-c")
}

func TestFilterByAssignedTo_EmptyWorkerID_ReturnsAll(t *testing.T) {
	entries := []ReadyEntry{
		{Issue: "task-a", AssignedWorker: "worker-x"},
		{Issue: "task-b", AssignedWorker: ""},
	}
	result := FilterByAssignedTo(entries, "")
	assert.Len(t, result, 2)
}

func TestFilterByAssignedTo_NoMatches_ReturnsEmpty(t *testing.T) {
	entries := []ReadyEntry{
		{Issue: "task-a", AssignedWorker: "worker-x"},
	}
	result := FilterByAssignedTo(entries, "worker-z")
	assert.Len(t, result, 0)
}

func TestDepth_DeepChain_CapsAt20(t *testing.T) {
	index := make(materialize.Index)
	// Build a chain deeper than 20
	for i := 0; i < 25; i++ {
		id := fmt.Sprintf("issue-%02d", i)
		parent := ""
		if i > 0 {
			parent = fmt.Sprintf("issue-%02d", i-1)
		}
		index[id] = materialize.IndexEntry{Parent: parent}
	}

	d := depth("issue-24", index)
	assert.Equal(t, 21, d, "depth should cap at 21 to break cycles")
}

func TestComputeReady_AssignedWorkerFieldPopulated(t *testing.T) {
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task", AssignedWorker: "worker-x"},
		"task-02": {Status: "open", Type: "task", AssignedWorker: ""},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task"},
		"task-02": {ID: "task-02", Status: "open", Type: "task"},
	}
	result := ComputeReady(index, issues, "")
	entryMap := make(map[string]ReadyEntry)
	for _, e := range result {
		entryMap[e.Issue] = e
	}
	assert.Equal(t, "worker-x", entryMap["task-01"].AssignedWorker)
	assert.Equal(t, "", entryMap["task-02"].AssignedWorker)
}

func TestComputeReady_AssignedWorkerFromIndex_EvenWithNoIssueEntry(t *testing.T) {
	// AssignedWorker comes from the index entry (authoritative), not the issues map.
	// Even when issues map has no entry, the index assignment is preserved.
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task", AssignedWorker: "worker-x"},
	}
	issues := map[string]*materialize.Issue{} // no issue entry
	result := ComputeReady(index, issues, "")
	assert.Len(t, result, 1)
	assert.Equal(t, "worker-x", result[0].AssignedWorker)
}

func TestDepth_NoParent(t *testing.T) {
	index := materialize.Index{
		"task-01": {Parent: ""},
	}
	assert.Equal(t, 0, depth("task-01", index))
}

func TestDepth_MissingFromIndex(t *testing.T) {
	index := materialize.Index{}
	assert.Equal(t, 0, depth("missing", index))
}

func TestAssignmentTier_AssignedToMe(t *testing.T) {
	index := materialize.Index{
		"T-001": {AssignedWorker: "worker-x"},
	}
	assert.Equal(t, 0, assignmentTier("T-001", "worker-x", index))
}

func TestAssignmentTier_Unassigned(t *testing.T) {
	index := materialize.Index{
		"T-001": {AssignedWorker: ""},
	}
	assert.Equal(t, 1, assignmentTier("T-001", "worker-x", index))
}

func TestAssignmentTier_AssignedToOther(t *testing.T) {
	index := materialize.Index{
		"T-001": {AssignedWorker: "worker-other"},
	}
	assert.Equal(t, 2, assignmentTier("T-001", "worker-x", index))
}

func TestAssignmentTier_NoWorkerContext(t *testing.T) {
	index := materialize.Index{
		"T-001": {AssignedWorker: "worker-x"},
	}
	// Empty workerID means no assignment context — treat as unassigned tier
	assert.Equal(t, 1, assignmentTier("T-001", "", index))
}
