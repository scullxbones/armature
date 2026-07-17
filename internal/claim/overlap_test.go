package claim

import (
	"testing"

	"github.com/scullxbones/armature/internal/dag"
	"github.com/stretchr/testify/assert"
)

// TestScopesOverlap_ExcludesAncestorDescendantPairs_REQ_TOPTIER_S17_T1 verifies that
// ScopesOverlapEx excludes ancestor/descendant issue pairs from conflict comparison.
// A parent story's scope is by design the union of its children's scopes,
// so comparing a child against its parent should never be a conflict.
func TestScopesOverlap_ExcludesAncestorDescendantPairs_REQ_TOPTIER_S17_T1(t *testing.T) {
	t.Parallel()

	// Build a simple hierarchy:
	// story-01 (scope: ["src/**"])
	//   └─ task-01 (scope: ["src/auth/**"])
	nodes := map[string]*dag.Node{
		"story-01": {
			ID:        "story-01",
			Title:     "Parent Story",
			Type:      "story",
			Parent:    "",
			Children:  []string{"task-01"},
			BlockedBy: []string{},
			Blocks:    []string{},
		},
		"task-01": {
			ID:        "task-01",
			Title:     "Child Task",
			Type:      "task",
			Parent:    "story-01",
			Children:  []string{},
			BlockedBy: []string{},
			Blocks:    []string{},
		},
	}
	graph := dag.FromIndex(nodes)

	// Even though the scopes overlap (task-01's scope is subset of story-01's),
	// ScopesOverlapEx should return false because they are parent and child
	scopeParent := []string{"src/**"}
	scopeChild := []string{"src/auth/**"}

	// Child claiming against parent should not report overlap
	result := ScopesOverlapEx(scopeChild, scopeParent, graph, "task-01", "story-01")
	assert.False(t, result, "child task should not conflict with parent story despite scope overlap")

	// Parent claiming against child should not report overlap either
	result = ScopesOverlapEx(scopeParent, scopeChild, graph, "story-01", "task-01")
	assert.False(t, result, "parent story should not conflict with child task despite scope overlap")

	// Non-ancestor/descendant pairs with overlapping scopes should still report overlap
	sibling := &dag.Node{
		ID:        "task-02",
		Title:     "Sibling Task",
		Type:      "task",
		Parent:    "story-01",
		Children:  []string{},
		BlockedBy: []string{},
		Blocks:    []string{},
	}
	nodes["task-02"] = sibling
	graph = dag.FromIndex(nodes)

	result = ScopesOverlapEx(scopeChild, scopeChild, graph, "task-01", "task-02")
	assert.True(t, result, "sibling tasks with same scope should conflict")
}

// TestScopesOverlap_StillDetectsNonAncestorOverlaps_REQ_TOPTIER_S17_T1 verifies that
// non-ancestor/descendant pairs with overlapping scopes are still detected.
func TestScopesOverlap_StillDetectsNonAncestorOverlaps_REQ_TOPTIER_S17_T1(t *testing.T) {
	t.Parallel()

	// Build a graph with unrelated tasks
	nodes := map[string]*dag.Node{
		"task-a": {
			ID:        "task-a",
			Title:     "Task A",
			Type:      "task",
			Parent:    "",
			Children:  []string{},
			BlockedBy: []string{},
			Blocks:    []string{},
		},
		"task-b": {
			ID:        "task-b",
			Title:     "Task B",
			Type:      "task",
			Parent:    "",
			Children:  []string{},
			BlockedBy: []string{},
			Blocks:    []string{},
		},
	}
	graph := dag.FromIndex(nodes)

	// Tasks with overlapping scopes should still conflict
	scopeA := []string{"src/auth/**"}
	scopeB := []string{"src/auth/login.go"}

	result := ScopesOverlapEx(scopeA, scopeB, graph, "task-a", "task-b")
	assert.True(t, result, "unrelated tasks with overlapping scopes should conflict")
}
