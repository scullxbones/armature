package ready

import (
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
	ready := ComputeReady(index, issues)
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
	ready := ComputeReady(index, issues)
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
	ready := ComputeReady(index, issues)
	assert.Len(t, ready, 1)
}

func TestReadyTask_ParentNotInProgress(t *testing.T) {
	index := materialize.Index{
		"story-01": {Status: "open", Type: "story"},
		"task-01":  {Status: "open", Type: "task", Parent: "story-01"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task", Parent: "story-01"},
	}
	ready := ComputeReady(index, issues)
	for _, r := range ready {
		if r.Issue == "task-01" {
			t.Errorf("task-01 should not be ready: parent story-01 is not in-progress")
		}
	}
}

func TestReadyTask_NoParent(t *testing.T) {
	index := materialize.Index{
		"task-01": {Status: "open", Type: "task"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {ID: "task-01", Status: "open", Type: "task"},
	}
	ready := ComputeReady(index, issues)
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
	ready := ComputeReady(index, issues)
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
	ready := ComputeReady(index, issues)
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
	ready := ComputeReady(index, issues)
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
	ready := ComputeReady(index, issues)
	assert.Len(t, ready, 2)
	assert.Equal(t, "task-b", ready[0].Issue)
}
