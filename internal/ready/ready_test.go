package ready

import (
	"fmt"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadyTask_AllRulesMet(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestStaleClaims_ClaimingWorkerActivityPreventsStale(t *testing.T) {
	t.Parallel()
	now := time.Unix(200, 0)
	issues := map[string]*materialize.Issue{
		"task-a": {
			ID:                         "task-a",
			Status:                     ops.StatusClaimed,
			ClaimedAt:                  0,
			LastHeartbeat:              0,
			LastClaimingWorkerActivity: 150,
			ClaimTTL:                   1,
		},
	}
	// Naively (ignoring LastClaimingWorkerActivity) this would read stale at
	// now=200 (0+60=60 < 200), but the claimant transitioned at 150
	// (150+60=210 > 200), so it must not be reported stale.
	assert.Empty(t, StaleClaims(issues, now))
}

func TestExpiredClaims_ClaimingWorkerActivityPreventsExpiry(t *testing.T) {
	t.Parallel()
	now := time.Unix(200, 0)
	issues := map[string]*materialize.Issue{
		"task-a": {
			ID:                         "task-a",
			Status:                     ops.StatusInProgress,
			ClaimedAt:                  0,
			LastHeartbeat:              0,
			LastClaimingWorkerActivity: 150,
			ClaimTTL:                   1,
		},
	}
	assert.Empty(t, ExpiredClaims(issues, now))
}

func TestReadyTask_DraftConfidence_ExcludedFromReady(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	entries := []ReadyEntry{
		{Issue: "task-a", AssignedWorker: "worker-x"},
		{Issue: "task-b", AssignedWorker: ""},
	}
	result := FilterByAssignedTo(entries, "")
	assert.Len(t, result, 2)
}

func TestFilterByAssignedTo_NoMatches_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	entries := []ReadyEntry{
		{Issue: "task-a", AssignedWorker: "worker-x"},
	}
	result := FilterByAssignedTo(entries, "worker-z")
	assert.Len(t, result, 0)
}

func TestDepth_DeepChain_CapsAt20(t *testing.T) {
	t.Parallel()
	index := make(materialize.Index)
	// Build a chain deeper than 20
	for i := range 25 {
		id := fmt.Sprintf("issue-%02d", i)
		parent := ""
		if i > 0 {
			parent = fmt.Sprintf("issue-%02d", i-1)
		}
		index[id] = materialize.IndexEntry{Parent: parent}
	}

	graph := graphFromIndex(index)
	d := graph.Depth("issue-24")
	assert.Equal(t, 24, d, "depth should be 24 (distance to root)")
}

func TestComputeReady_AssignedWorkerFieldPopulated(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	index := materialize.Index{
		"task-01": {Parent: ""},
	}
	graph := graphFromIndex(index)
	assert.Equal(t, 0, graph.Depth("task-01"))
}

func TestDepth_MissingFromIndex(t *testing.T) {
	t.Parallel()
	index := materialize.Index{}
	graph := graphFromIndex(index)
	assert.Equal(t, 0, graph.Depth("missing"))
}

func TestAssignmentTier_AssignedToMe(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"T-001": {AssignedWorker: "worker-x"},
	}
	assert.Equal(t, 0, assignmentTier("T-001", "worker-x", index))
}

func TestAssignmentTier_Unassigned(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"T-001": {AssignedWorker: ""},
	}
	assert.Equal(t, 1, assignmentTier("T-001", "worker-x", index))
}

func TestAssignmentTier_AssignedToOther(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"T-001": {AssignedWorker: "worker-other"},
	}
	assert.Equal(t, 2, assignmentTier("T-001", "worker-x", index))
}

func TestAssignmentTier_NoWorkerContext(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"T-001": {AssignedWorker: "worker-x"},
	}
	// Empty workerID means no assignment context — treat as unassigned tier
	assert.Equal(t, 1, assignmentTier("T-001", "", index))
}

func TestReadyTask_SortByBlocksCount(t *testing.T) {
	t.Parallel()
	// Two tasks at the same depth and priority — the one that blocks more should sort first (compute.go:167)
	index := materialize.Index{
		"task-a": {Status: "open", Type: "task", Blocks: []string{"task-c", "task-d"}},
		"task-b": {Status: "open", Type: "task", Blocks: []string{}},
	}
	issues := map[string]*materialize.Issue{
		"task-a": {ID: "task-a", Status: "open", Type: "task"},
		"task-b": {ID: "task-b", Status: "open", Type: "task"},
	}
	ready := ComputeReady(index, issues, "")
	require.Len(t, ready, 2)
	// task-a blocks 2 others → it is more critical → should sort first
	assert.Equal(t, "task-a", ready[0].Issue, "task with more Blocks should sort before task with fewer")
}

func TestExplainNotReady_BlockerNotMerged(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
		"task-02":  {Status: "done", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
	}
	result := ExplainNotReady(index, issues)
	reason, ok := result["task-01"]
	require.True(t, ok, "task-01 should be present in explain map")
	assert.Contains(t, reason, "task-02", "reason should mention the unmerged blocker")
}

func TestExplainNotReady_ParentNotActive(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"story-01": {Status: "done", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{}},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01"},
	}
	result := ExplainNotReady(index, issues)
	reason, ok := result["task-01"]
	require.True(t, ok, "task-01 should be present in explain map")
	assert.Contains(t, reason, "story-01", "reason should mention the inactive parent")
}

func TestExplainNotReady_BlockerDone_AppendsHint(t *testing.T) {
	t.Parallel()
	// When a blocker is in status 'done' (not merged, not missing, not cancelled),
	// the reason string should append ' — run: arm merged --issue <BLOCKER-ID>'.
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
		"task-02":  {Status: "done", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
	}
	result := ExplainNotReady(index, issues)
	reason, ok := result["task-01"]
	require.True(t, ok, "task-01 should be present in explain map")
	assert.Contains(t, reason, "task-02", "reason should mention the blocker")
	assert.Contains(t, reason, "run: arm merged --issue task-02", "reason should append the merged hint for done blockers")
}

func TestExplainNotReady_BlockerMissing_NoHint(t *testing.T) {
	t.Parallel()
	// When a blocker is missing from the index, no merged hint should be appended.
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-missing"}},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-missing"}},
	}
	result := ExplainNotReady(index, issues)
	reason, ok := result["task-01"]
	require.True(t, ok, "task-01 should be present in explain map")
	assert.Contains(t, reason, "task-missing", "reason should mention the blocker")
	assert.NotContains(t, reason, "arm merged", "no merged hint for missing blockers")
}

func TestExplainNotReady_BlockerCancelled_NoHint(t *testing.T) {
	t.Parallel()
	// When a blocker is in 'cancelled' status, no merged hint should be appended.
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
		"task-02":  {Status: "cancelled", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02"}},
	}
	result := ExplainNotReady(index, issues)
	reason, ok := result["task-01"]
	require.True(t, ok, "task-01 should be present in explain map")
	assert.Contains(t, reason, "task-02", "reason should mention the blocker")
	assert.NotContains(t, reason, "arm merged", "no merged hint for cancelled blockers")
}

func TestExplainNotReady_MultipleDoneBlockers_HintForEach(t *testing.T) {
	t.Parallel()
	// When multiple blockers are in 'done' status, the hint appears for each one.
	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02", "task-03"}},
		"task-02":  {Status: "done", Type: "task"},
		"task-03":  {Status: "done", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-02", "task-03"}},
	}
	result := ExplainNotReady(index, issues)
	reason, ok := result["task-01"]
	require.True(t, ok, "task-01 should be present in explain map")
	assert.Contains(t, reason, "run: arm merged --issue task-02", "hint for task-02")
	assert.Contains(t, reason, "run: arm merged --issue task-03", "hint for task-03")
}

func TestExplainNotReady_MixedBlockers_HintOnlyForDone(t *testing.T) {
	t.Parallel()
	// Mixed blockers: one done (hint), one missing (no hint), one cancelled (no hint).
	index := materialize.Index{
		"story-01":       {Status: "in-progress", Type: "story"},
		"task-01":        {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-done", "task-missing", "task-cancelled"}},
		"task-done":      {Status: "done", Type: "task"},
		"task-cancelled": {Status: "cancelled", Type: "task"},
		// task-missing intentionally absent from index
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{"task-done", "task-missing", "task-cancelled"}},
	}
	result := ExplainNotReady(index, issues)
	reason, ok := result["task-01"]
	require.True(t, ok, "task-01 should be present in explain map")
	assert.Contains(t, reason, "run: arm merged --issue task-done", "hint only for done blockers")
	assert.NotContains(t, reason, "run: arm merged --issue task-missing", "no hint for missing blocker")
	assert.NotContains(t, reason, "run: arm merged --issue task-cancelled", "no hint for cancelled blocker")
}

func TestFilterByParent_IncludesDescendantsOnly(t *testing.T) {
	t.Parallel()
	// Create a hierarchy: story-01 > task-a, task-b > subtask-a1
	//                     story-02 > task-c
	// Mark story-01 as claimed so it doesn't appear in ready queue (only its descendants)
	index := materialize.Index{
		"story-01":   {Status: "claimed", Type: "story", Children: []string{"task-a", "task-b"}},
		"task-a":     {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{}},
		"task-b":     {Status: "open", Type: "task", Parent: "story-01", BlockedBy: []string{}, Children: []string{"subtask-a1"}},
		"subtask-a1": {Status: "open", Type: "task", Parent: "task-b", BlockedBy: []string{}},
		"story-02":   {Status: "claimed", Type: "story", Children: []string{"task-c"}},
		"task-c":     {Status: "open", Type: "task", Parent: "story-02", BlockedBy: []string{}},
	}
	issues := map[string]*materialize.Issue{
		"task-a":     {ID: "task-a", Status: "open", Type: "task"},
		"task-b":     {ID: "task-b", Status: "open", Type: "task"},
		"subtask-a1": {ID: "subtask-a1", Status: "open", Type: "task"},
		"task-c":     {ID: "task-c", Status: "open", Type: "task"},
	}

	ready := ComputeReady(index, issues, "")
	assert.Len(t, ready, 4, "should have 4 ready tasks before filtering")

	// Filter by story-01 (should include task-a, task-b, subtask-a1)
	descendants := CollectDescendants("story-01", index)
	filtered := ready[:0]
	for _, e := range ready {
		if descendants[e.Issue] {
			filtered = append(filtered, e)
		}
	}

	assert.Len(t, filtered, 3, "should have 3 tasks under story-01")
	ids := make(map[string]bool)
	for _, e := range filtered {
		ids[e.Issue] = true
	}
	assert.True(t, ids["task-a"], "task-a should be in descendants")
	assert.True(t, ids["task-b"], "task-b should be in descendants")
	assert.True(t, ids["subtask-a1"], "subtask-a1 should be in descendants")
	assert.False(t, ids["task-c"], "task-c should not be in descendants")
}

func TestCollectDescendants_IncludesNestedChildren(t *testing.T) {
	t.Parallel()
	// Build a hierarchy with nested descendants
	index := materialize.Index{
		"root":         {Status: "open", Type: "story", Children: []string{"child1", "child2"}},
		"child1":       {Status: "open", Type: "task", Parent: "root", Children: []string{"grandchild1a", "grandchild1b"}},
		"child2":       {Status: "open", Type: "task", Parent: "root", Children: []string{}},
		"grandchild1a": {Status: "open", Type: "task", Parent: "child1", Children: []string{}},
		"grandchild1b": {Status: "open", Type: "task", Parent: "child1", Children: []string{}},
	}

	descendants := CollectDescendants("root", index)

	assert.Len(t, descendants, 4, "should have 4 descendants")
	assert.True(t, descendants["child1"], "child1 should be a descendant")
	assert.True(t, descendants["child2"], "child2 should be a descendant")
	assert.True(t, descendants["grandchild1a"], "grandchild1a should be a descendant")
	assert.True(t, descendants["grandchild1b"], "grandchild1b should be a descendant")
	assert.False(t, descendants["root"], "root should not be a descendant of itself")
}

func TestCollectDescendants_EmptyForLeaf(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"leaf": {Status: "open", Type: "task", Children: []string{}},
	}

	descendants := CollectDescendants("leaf", index)
	assert.Len(t, descendants, 0, "leaf node should have no descendants")
}

func TestCollectDescendants_MissingRoot(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"child": {Status: "open", Type: "task", Parent: "missing-root"},
	}

	descendants := CollectDescendants("missing-root", index)
	assert.Len(t, descendants, 0, "missing root should return empty set")
}

func TestExplainNotReady_WithInjectedTime_StaleClaimExcluded(t *testing.T) {
	t.Parallel()
	// Test that ExplainNotReady accepts injected time and correctly identifies stale claims.
	// Claim was at time 0, TTL is 60 seconds, so at time 61 it should be stale and excluded.
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID:            "task-01",
			Status:        "open",
			Type:          "task",
			ClaimedBy:     "some-worker",
			ClaimedAt:     0,
			LastHeartbeat: 0,
			ClaimTTL:      1, // 1 minute TTL
		},
	}
	// Call with injected time past the TTL (61 seconds past claim at 0)
	result := ExplainNotReady(index, issues, 61)
	// Task should NOT be in the explanation map because the claim is stale
	// (stale claims are excluded from the explanation)
	_, ok := result["task-01"]
	assert.False(t, ok, "stale claimed task should not appear in ExplainNotReady output")
}

func TestExplainNotReady_WithInjectedTime_FreshClaimIncluded(t *testing.T) {
	t.Parallel()
	// Test that ExplainNotReady excludes fresh (non-stale) claims.
	// Claim was at time 0, TTL is 60 seconds, at time 30 it's still fresh.
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID:            "task-01",
			Status:        "open",
			Type:          "task",
			ClaimedBy:     "some-worker",
			ClaimedAt:     0,
			LastHeartbeat: 0,
			ClaimTTL:      1, // 1 minute TTL
		},
	}
	// Call with injected time before the TTL expires (30 seconds)
	result := ExplainNotReady(index, issues, 30)
	// Task should NOT be in the explanation map because the claim is still fresh
	_, ok := result["task-01"]
	assert.False(t, ok, "fresh claimed task should not appear in ExplainNotReady output")
}

func TestExpiredClaims_ClaimedPastTTL_Surfaced(t *testing.T) {
	t.Parallel()
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID: "task-01", Title: "Expired claimed task", Status: "claimed",
			ClaimedBy: "worker-a", ClaimedAt: 0, ClaimTTL: 1,
		},
	}
	entries := ExpiredClaims(issues, time.Unix(61, 0))
	require.Len(t, entries, 1)
	assert.Equal(t, "task-01", entries[0].Issue)
	assert.Equal(t, "worker-a", entries[0].ClaimedBy)
	assert.Equal(t, "claimed", entries[0].Status)
}

func TestExpiredClaims_MultipleEntries_SortedByIssueID(t *testing.T) {
	t.Parallel()
	issues := map[string]*materialize.Issue{
		"task-zebra": {
			ID: "task-zebra", Status: "claimed",
			ClaimedBy: "worker-a", ClaimedAt: 0, ClaimTTL: 1,
		},
		"task-alpha": {
			ID: "task-alpha", Status: "claimed",
			ClaimedBy: "worker-b", ClaimedAt: 0, ClaimTTL: 1,
		},
	}
	entries := ExpiredClaims(issues, time.Unix(61, 0))
	require.Len(t, entries, 2)
	assert.Equal(t, "task-alpha", entries[0].Issue)
	assert.Equal(t, "task-zebra", entries[1].Issue)
}

func TestExpiredClaims_InProgressPastTTL_Surfaced(t *testing.T) {
	t.Parallel()
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID: "task-01", Title: "Starved in-progress task", Status: "in-progress",
			ClaimedBy: "worker-a", ClaimedAt: 0, ClaimTTL: 1,
		},
	}
	entries := ExpiredClaims(issues, time.Unix(61, 0))
	require.Len(t, entries, 1)
	assert.Equal(t, "in-progress", entries[0].Status)
}

func TestExpiredClaims_ActiveClaim_NotSurfaced(t *testing.T) {
	t.Parallel()
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID: "task-01", Status: "claimed", ClaimedBy: "worker-a", ClaimedAt: 0, ClaimTTL: 60,
		},
	}
	entries := ExpiredClaims(issues, time.Unix(30, 0))
	assert.Empty(t, entries, "a claim within its TTL must not be surfaced as expired")
}

func TestExpiredClaims_OpenIssue_NotSurfaced(t *testing.T) {
	t.Parallel()
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open"},
	}
	entries := ExpiredClaims(issues, time.Unix(1_000_000, 0))
	assert.Empty(t, entries, "an unclaimed open issue is never an expired claim")
}

func TestExpiredClaims_DoesNotOverlapReadyQueue(t *testing.T) {
	t.Parallel()
	// A claimed+expired task is excluded from the ready queue (ComputeReady only
	// surfaces status=open), so ExpiredClaims is the sole place it's surfaced —
	// distinct from, not merged into, the ready queue.
	index := materialize.Index{
		"task-01": {Status: "claimed", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {
			ID: "task-01", Status: "claimed", Type: "task",
			ClaimedBy: "worker-a", ClaimedAt: 0, ClaimTTL: 1,
		},
	}
	ready := ComputeReady(index, issues, "", 61)
	assert.Empty(t, ready, "claimed issues never appear in the ready queue, expired or not")

	expired := ExpiredClaims(issues, time.Unix(61, 0))
	require.Len(t, expired, 1)
	assert.Equal(t, "task-01", expired[0].Issue)
}
