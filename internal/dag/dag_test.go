package dag

import (
	"fmt"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddNodeDuplicate tests that adding a duplicate node fails.
func TestAddNodeDuplicate(t *testing.T) {
	t.Parallel()
	d := New()
	node := &Node{ID: "task-1", Title: "Test", Type: "task"}

	err := d.AddNode(node)
	require.NoError(t, err)

	err = d.AddNode(node)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestNoCycleInAcyclicDAG tests that acyclic DAGs are detected correctly.
func TestNoCycleInAcyclicDAG(t *testing.T) {
	t.Parallel()
	d := New()
	// Create a simple tree: epic -> story -> task
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story := &Node{ID: "story-1", Title: "Story", Type: "story", Parent: "epic-1"}
	task := &Node{ID: "task-1", Title: "Task", Type: "task", Parent: "story-1"}

	require.NoError(t, d.AddNode(epic))
	require.NoError(t, d.AddNode(story))
	require.NoError(t, d.AddNode(task))

	// Set up children relationships
	epic.Children = []string{"story-1"}
	story.Children = []string{"task-1"}

	assert.False(t, d.HasCycle())
}

// TestCycleDetection tests that cycles are detected.
func TestCycleDetection(t *testing.T) {
	t.Parallel()
	d := New()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", BlockedBy: []string{"task-2"}}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}

	require.NoError(t, d.AddNode(task1))
	require.NoError(t, d.AddNode(task2))

	assert.True(t, d.HasCycle())
}

// TestPropertyNoSelfCycles: Generate arbitrary nodes and verify no node blocks itself.
func TestPropertyNoSelfCycles(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("no node can block itself", prop.ForAll(
		func(nodeID string) bool {
			d := New()
			node := &Node{
				ID:        nodeID,
				Title:     "Test",
				Type:      "task",
				BlockedBy: []string{nodeID}, // Self-blocking
			}
			if err := d.AddNode(node); err != nil {
				return false
			}
			// A self-blocking node creates a cycle
			return d.HasCycle()
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// TestPropertyParentChildConsistency: Verify that if A lists parent B,
// then B must list A as a child.
func TestPropertyParentChildConsistency(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	properties.Property("parent-child consistency is maintained", prop.ForAll(
		func(parentID, childID string) bool {
			if parentID == childID || parentID == "" || childID == "" {
				return true // Skip invalid cases
			}

			d := New()
			parent := &Node{ID: parentID, Title: "Parent", Type: "story"}
			child := &Node{ID: childID, Title: "Child", Type: "task", Parent: parentID}

			parent.Children = []string{childID}

			if err := d.AddNode(parent); err != nil {
				return false
			}
			if err := d.AddNode(child); err != nil {
				return false
			}

			err := d.ValidateParentChild()
			return err == nil
		},
		gen.AlphaString(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// BenchmarkCycleDetection benchmarks cycle detection on a larger DAG.
// Note: This benchmark measures cycle detection on the BlockedBy path only;
// nodes are added without populating Children slices, so parent-child edges
// are not exercised in the DFS traversal.
func BenchmarkCycleDetection(b *testing.B) {
	d := New()

	// Create a tree of 100 nodes
	for i := range 100 {
		parent := fmt.Sprintf("node-%d", i/2)
		if i == 0 {
			parent = ""
		}
		node := &Node{
			ID:     fmt.Sprintf("node-%d", i),
			Title:  fmt.Sprintf("Node %d", i),
			Type:   "task",
			Parent: parent,
		}
		if err := d.AddNode(node); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for b.Loop() {
		d.HasCycle()
	}
}

// TestGraphAncestry tests that Graph.Ancestry returns all upstream nodes.
func TestGraphAncestry(t *testing.T) {
	t.Parallel()
	d := New()
	// Create a chain: epic -> story -> task1
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story := &Node{ID: "story-1", Title: "Story", Type: "story", Parent: "epic-1"}
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", Parent: "story-1"}

	require.NoError(t, d.AddNode(epic))
	require.NoError(t, d.AddNode(story))
	require.NoError(t, d.AddNode(task1))

	epic.Children = []string{"story-1"}
	story.Children = []string{"task-1"}

	g := d

	// task-1 ancestors should be story-1 and epic-1
	ancestors := g.Ancestry("task-1")
	assert.ElementsMatch(t, []string{"story-1", "epic-1"}, ancestors)

	// story-1 ancestors should be epic-1
	ancestors = g.Ancestry("story-1")
	assert.ElementsMatch(t, []string{"epic-1"}, ancestors)

	// epic-1 has no ancestors
	ancestors = g.Ancestry("epic-1")
	assert.Empty(t, ancestors)
}

// TestGraphDescendants tests that Graph.Descendants returns all downstream nodes.
func TestGraphDescendants(t *testing.T) {
	t.Parallel()
	d := New()
	// Create a tree: epic -> story1, story2 -> task
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story1 := &Node{ID: "story-1", Title: "Story 1", Type: "story", Parent: "epic-1"}
	story2 := &Node{ID: "story-2", Title: "Story 2", Type: "story", Parent: "epic-1"}
	task := &Node{ID: "task-1", Title: "Task 1", Type: "task", Parent: "story-1"}

	require.NoError(t, d.AddNode(epic))
	require.NoError(t, d.AddNode(story1))
	require.NoError(t, d.AddNode(story2))
	require.NoError(t, d.AddNode(task))

	epic.Children = []string{"story-1", "story-2"}
	story1.Children = []string{"task-1"}
	story2.Children = []string{}

	g := d

	// epic-1 descendants should be story1, story2, task
	descendants := g.Descendants("epic-1")
	assert.ElementsMatch(t, []string{"story-1", "story-2", "task-1"}, descendants)

	// story-1 descendants should be task-1
	descendants = g.Descendants("story-1")
	assert.ElementsMatch(t, []string{"task-1"}, descendants)

	// task-1 has no descendants
	descendants = g.Descendants("task-1")
	assert.Empty(t, descendants)
}

// TestGraphBlockers tests that Graph.Blockers returns direct blocked_by dependencies.
func TestGraphBlockers(t *testing.T) {
	t.Parallel()
	d := New()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task"}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}
	task3 := &Node{ID: "task-3", Title: "Task 3", Type: "task", BlockedBy: []string{"task-1", "task-2"}}

	require.NoError(t, d.AddNode(task1))
	require.NoError(t, d.AddNode(task2))
	require.NoError(t, d.AddNode(task3))

	task1.Blocks = []string{"task-2", "task-3"}
	task2.Blocks = []string{"task-3"}

	g := d

	// task-2 blockers should be task-1
	blockers := g.Blockers("task-2")
	assert.ElementsMatch(t, []string{"task-1"}, blockers)

	// task-3 blockers should be task-1 and task-2
	blockers = g.Blockers("task-3")
	assert.ElementsMatch(t, []string{"task-1", "task-2"}, blockers)

	// task-1 has no blockers
	blockers = g.Blockers("task-1")
	assert.Empty(t, blockers)
}

// TestGraphBlocks tests that Graph.Blocks returns nodes that this node directly blocks.
func TestGraphBlocks(t *testing.T) {
	t.Parallel()
	d := New()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", Blocks: []string{"task-2", "task-3"}}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}
	task3 := &Node{ID: "task-3", Title: "Task 3", Type: "task", BlockedBy: []string{"task-1", "task-2"}}

	require.NoError(t, d.AddNode(task1))
	require.NoError(t, d.AddNode(task2))
	require.NoError(t, d.AddNode(task3))

	task2.Blocks = []string{"task-3"}

	g := d

	// task-1 blocks task-2 and task-3
	blocks := g.Blocks("task-1")
	assert.ElementsMatch(t, []string{"task-2", "task-3"}, blocks)

	// task-2 blocks task-3
	blocks = g.Blocks("task-2")
	assert.ElementsMatch(t, []string{"task-3"}, blocks)

	// task-3 blocks nothing
	blocks = g.Blocks("task-3")
	assert.Empty(t, blocks)
}

// TestGraphHierarchy tests that Graph.Hierarchy returns parent and children.
func TestGraphHierarchy(t *testing.T) {
	t.Parallel()
	d := New()
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story := &Node{ID: "story-1", Title: "Story", Type: "story", Parent: "epic-1"}
	task := &Node{ID: "task-1", Title: "Task", Type: "task", Parent: "story-1"}

	require.NoError(t, d.AddNode(epic))
	require.NoError(t, d.AddNode(story))
	require.NoError(t, d.AddNode(task))

	epic.Children = []string{"story-1"}
	story.Children = []string{"task-1"}

	g := d

	// story-1 parent should be epic-1, children should be task-1
	parent, children := g.Hierarchy("story-1")
	assert.Equal(t, "epic-1", parent)
	assert.ElementsMatch(t, []string{"task-1"}, children)

	// epic-1 has no parent, children should be story-1
	parent, children = g.Hierarchy("epic-1")
	assert.Equal(t, "", parent)
	assert.ElementsMatch(t, []string{"story-1"}, children)

	// task-1 has parent story-1, no children
	parent, children = g.Hierarchy("task-1")
	assert.Equal(t, "story-1", parent)
	assert.Empty(t, children)
}

// TestGraphHasCycle tests that Graph.HasCycle detects cycles.
func TestGraphHasCycle(t *testing.T) {
	t.Parallel()
	// Test acyclic DAG
	d := New()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", BlockedBy: []string{"task-2"}}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task"}

	require.NoError(t, d.AddNode(task1))
	require.NoError(t, d.AddNode(task2))
	task2.Blocks = []string{"task-1"}

	g := d
	assert.False(t, g.HasCycle())

	// Test cyclic DAG
	d2 := New()
	task3 := &Node{ID: "task-3", Title: "Task 3", Type: "task", BlockedBy: []string{"task-4"}}
	task4 := &Node{ID: "task-4", Title: "Task 4", Type: "task", BlockedBy: []string{"task-3"}}

	require.NoError(t, d2.AddNode(task3))
	require.NoError(t, d2.AddNode(task4))

	task3.Blocks = []string{"task-4"}
	task4.Blocks = []string{"task-3"}

	g2 := d2
	assert.True(t, g2.HasCycle())
}

// TestGraphDepth tests that Graph.Depth returns the depth of a node from root.
func TestGraphDepth(t *testing.T) {
	t.Parallel()
	d := New()
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story := &Node{ID: "story-1", Title: "Story", Type: "story", Parent: "epic-1"}
	task := &Node{ID: "task-1", Title: "Task", Type: "task", Parent: "story-1"}

	require.NoError(t, d.AddNode(epic))
	require.NoError(t, d.AddNode(story))
	require.NoError(t, d.AddNode(task))

	epic.Children = []string{"story-1"}
	story.Children = []string{"task-1"}

	g := d

	// Depth is measured from root (no parent)
	assert.Equal(t, 0, g.Depth("epic-1"))
	assert.Equal(t, 1, g.Depth("story-1"))
	assert.Equal(t, 2, g.Depth("task-1"))
}

// TestGraphDepthWithMultipleRoots tests depth calculation with multiple root nodes.
func TestGraphDepthWithMultipleRoots(t *testing.T) {
	t.Parallel()
	d := New()
	epic1 := &Node{ID: "epic-1", Title: "Epic 1", Type: "epic"}
	epic2 := &Node{ID: "epic-2", Title: "Epic 2", Type: "epic"}
	story1 := &Node{ID: "story-1", Title: "Story 1", Type: "story", Parent: "epic-1"}

	require.NoError(t, d.AddNode(epic1))
	require.NoError(t, d.AddNode(epic2))
	require.NoError(t, d.AddNode(story1))

	epic1.Children = []string{"story-1"}

	g := d

	assert.Equal(t, 0, g.Depth("epic-1"))
	assert.Equal(t, 0, g.Depth("epic-2"))
	assert.Equal(t, 1, g.Depth("story-1"))
}

// TestGraphAncestryNonexistentNode tests that Ancestry handles nonexistent nodes.
func TestGraphAncestryNonexistentNode(t *testing.T) {
	t.Parallel()
	d := New()
	g := d

	ancestors := g.Ancestry("nonexistent")
	assert.Empty(t, ancestors)
}

// TestGraphDescendantsNonexistentNode tests that Descendants handles nonexistent nodes.
func TestGraphDescendantsNonexistentNode(t *testing.T) {
	t.Parallel()
	d := New()
	g := d

	descendants := g.Descendants("nonexistent")
	assert.Empty(t, descendants)
}

// TestGraphBlockersNonexistentNode tests that Blockers returns nil for nonexistent nodes.
func TestGraphBlockersNonexistentNode(t *testing.T) {
	t.Parallel()
	d := New()
	g := d

	blockers := g.Blockers("nonexistent")
	assert.Nil(t, blockers)
}

// TestGraphBlocksNonexistentNode tests that Blocks returns nil for nonexistent nodes.
func TestGraphBlocksNonexistentNode(t *testing.T) {
	t.Parallel()
	d := New()
	g := d

	blocks := g.Blocks("nonexistent")
	assert.Nil(t, blocks)
}

// TestGraphHierarchyNonexistentNode tests that Hierarchy handles nonexistent nodes.
func TestGraphHierarchyNonexistentNode(t *testing.T) {
	t.Parallel()
	d := New()
	g := d

	parent, children := g.Hierarchy("nonexistent")
	assert.Equal(t, "", parent)
	assert.Nil(t, children)
}

// TestGraphDepthNonexistentNode tests that Depth returns 0 for nonexistent nodes.
func TestGraphDepthNonexistentNode(t *testing.T) {
	t.Parallel()
	d := New()
	g := d

	depth := g.Depth("nonexistent")
	assert.Equal(t, 0, depth)
}

// TestGraphBlockersEmptyNode tests that a node with no blockers returns empty slice.
func TestGraphBlockersEmptyNode(t *testing.T) {
	t.Parallel()
	d := New()
	task := &Node{ID: "task-1", Title: "Task 1", Type: "task"}

	require.NoError(t, d.AddNode(task))

	g := d

	blockers := g.Blockers("task-1")
	assert.Empty(t, blockers)
}

// TestGraphBlocksEmptyNode tests that a node with no blocks returns empty slice.
func TestGraphBlocksEmptyNode(t *testing.T) {
	t.Parallel()
	d := New()
	task := &Node{ID: "task-1", Title: "Task 1", Type: "task"}

	require.NoError(t, d.AddNode(task))

	g := d

	blocks := g.Blocks("task-1")
	assert.Empty(t, blocks)
}

// TestGraphNode uses the Node method to test node retrieval.
func TestGraphNode(t *testing.T) {
	t.Parallel()
	d := New()
	task := &Node{ID: "task-1", Title: "Task 1", Type: "task"}

	require.NoError(t, d.AddNode(task))

	retrieved := d.Node("task-1")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "task-1", retrieved.ID)

	notFound := d.Node("nonexistent")
	assert.Nil(t, notFound)
}

// TestHasCycle_DanglingChildReference tests that HasCycle does not panic
// when a node has a child reference to a node that does not exist in the DAG.
func TestHasCycle_DanglingChildReference(t *testing.T) {
	t.Parallel()
	d := New()
	node := &Node{ID: "a", Title: "Node A", Type: "task", Children: []string{"nonexistent"}}
	require.NoError(t, d.AddNode(node))

	// Should not panic and should return false (no cycle)
	assert.False(t, d.HasCycle())
}

// TestBlockersMutationSafety tests that mutating the returned slice from Blockers
// does not affect the graph's internal state.
func TestBlockersMutationSafety(t *testing.T) {
	t.Parallel()
	d := New()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task"}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}

	require.NoError(t, d.AddNode(task1))
	require.NoError(t, d.AddNode(task2))

	g := d

	// Get the blockers and capture the original list
	original := g.Blockers("task-2")

	// Mutate the returned slice by appending to it
	mutated := append(g.Blockers("task-2"), "injected")

	// Verify that the mutation did not affect the graph's internal state
	assert.ElementsMatch(t, original, g.Blockers("task-2"))
	assert.NotContains(t, g.Blockers("task-2"), "injected")
	// Verify that the mutated slice is different from the graph's current state
	assert.NotEqual(t, len(g.Blockers("task-2")), len(mutated))
}

// TestBlocksMutationSafety tests that mutating the returned slice from Blocks
// does not affect the graph's internal state.
func TestBlocksMutationSafety(t *testing.T) {
	t.Parallel()
	d := New()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", Blocks: []string{"task-2"}}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}

	require.NoError(t, d.AddNode(task1))
	require.NoError(t, d.AddNode(task2))

	g := d

	// Get the blocks and capture the original list
	original := g.Blocks("task-1")

	// Mutate the returned slice by appending to it
	mutated := append(g.Blocks("task-1"), "injected")

	// Verify that the mutation did not affect the graph's internal state
	assert.ElementsMatch(t, original, g.Blocks("task-1"))
	assert.NotContains(t, g.Blocks("task-1"), "injected")
	// Verify that the mutated slice is different from the graph's current state
	assert.NotEqual(t, len(g.Blocks("task-1")), len(mutated))
}

// TestHierarchyMutationSafety tests that mutating the returned children slice from Hierarchy
// does not affect the graph's internal state.
func TestHierarchyMutationSafety(t *testing.T) {
	t.Parallel()
	d := New()
	parent := &Node{ID: "parent-1", Title: "Parent", Type: "story"}
	child := &Node{ID: "child-1", Title: "Child", Type: "task", Parent: "parent-1"}

	parent.Children = []string{"child-1"}

	require.NoError(t, d.AddNode(parent))
	require.NoError(t, d.AddNode(child))

	g := d

	// Get the hierarchy and capture the original children
	_, originalChildren := g.Hierarchy("parent-1")

	// Mutate the returned children slice by appending to it
	_, returned := g.Hierarchy("parent-1")
	mutated := append(returned, "injected") //nolint:gocritic // intentional separate slice to test immutability

	// Verify that the mutation did not affect the graph's internal state
	_, currentChildren := g.Hierarchy("parent-1")
	assert.ElementsMatch(t, originalChildren, currentChildren)
	assert.NotContains(t, currentChildren, "injected")
	// Verify that the mutated slice is different from the graph's current state
	assert.NotEqual(t, len(currentChildren), len(mutated))
}

// TestGraph_Depth_CycleGuard tests that Depth handles parent cycles without hanging.
func TestGraph_Depth_CycleGuard(t *testing.T) {
	t.Parallel()
	d := New()
	// Create a cycle: a.Parent=b, b.Parent=a
	nodeA := &Node{ID: "a", Title: "Node A", Type: "task", Parent: "b"}
	nodeB := &Node{ID: "b", Title: "Node B", Type: "task", Parent: "a"}

	require.NoError(t, d.AddNode(nodeA))
	require.NoError(t, d.AddNode(nodeB))

	g := d

	// Depth should return a finite value and not hang
	depth := g.Depth("a")
	assert.GreaterOrEqual(t, depth, 0)
	// a -> b (depth=1), b -> a (depth=2), then cycle guard breaks
	assert.Equal(t, 2, depth)
}

// TestGraph_Ancestry_CycleGuard tests that Ancestry handles parent cycles without hanging.
func TestGraph_Ancestry_CycleGuard(t *testing.T) {
	t.Parallel()
	d := New()
	// Create a cycle: a.Parent=b, b.Parent=a
	nodeA := &Node{ID: "a", Title: "Node A", Type: "task", Parent: "b"}
	nodeB := &Node{ID: "b", Title: "Node B", Type: "task", Parent: "a"}

	require.NoError(t, d.AddNode(nodeA))
	require.NoError(t, d.AddNode(nodeB))

	g := d

	// Ancestry should return a finite slice and not hang
	ancestors := g.Ancestry("a")
	assert.NotNil(t, ancestors)
	// a -> b, then cycle detected, so we only get b
	assert.ElementsMatch(t, []string{"b"}, ancestors)
}

// TestFromIndexBasic tests that FromIndex constructs a Graph with Ancestry and Descendants working correctly.
func TestFromIndexBasic(t *testing.T) {
	t.Parallel()
	index := map[string]*Node{
		"epic-1": {
			ID:       "epic-1",
			Title:    "Epic",
			Type:     "epic",
			Parent:   "",
			Children: []string{"story-1"},
		},
		"story-1": {
			ID:       "story-1",
			Title:    "Story",
			Type:     "story",
			Parent:   "epic-1",
			Children: []string{"task-1"},
		},
		"task-1": {
			ID:       "task-1",
			Title:    "Task",
			Type:     "task",
			Parent:   "story-1",
			Children: []string{},
		},
	}

	g := FromIndex(index)
	require.NotNil(t, g)

	// Test Ancestry: task-1 should have story-1 and epic-1 as ancestors
	ancestors := g.Ancestry("task-1")
	assert.ElementsMatch(t, []string{"story-1", "epic-1"}, ancestors)

	// Test Descendants: epic-1 should have story-1 and task-1 as descendants
	descendants := g.Descendants("epic-1")
	assert.ElementsMatch(t, []string{"story-1", "task-1"}, descendants)
}

// TestFromIndexEmpty tests that FromIndex handles an empty index.
func TestFromIndexEmpty(t *testing.T) {
	t.Parallel()
	index := make(map[string]*Node)

	g := FromIndex(index)
	require.NotNil(t, g)

	// Empty index should return empty results
	ancestors := g.Ancestry("nonexistent")
	assert.Empty(t, ancestors)

	descendants := g.Descendants("nonexistent")
	assert.Empty(t, descendants)
}

// TestScopedHasCycleCrossScope tests that ScopedHasCycle detects cycles that close within scope,
// even if the cycle path traverses nodes outside the scope.
func TestScopedHasCycleCrossScope(t *testing.T) {
	t.Parallel()
	d := New()
	// Create a cycle: A -> B -> C -> A, where only A is in scope
	nodeA := &Node{ID: "A", Title: "A", Type: "task", BlockedBy: []string{"C"}}
	nodeB := &Node{ID: "B", Title: "B", Type: "task", BlockedBy: []string{"A"}}
	nodeC := &Node{ID: "C", Title: "C", Type: "task", BlockedBy: []string{"B"}}

	require.NoError(t, d.AddNode(nodeA))
	require.NoError(t, d.AddNode(nodeB))
	require.NoError(t, d.AddNode(nodeC))

	nodeA.Blocks = []string{"B"}
	nodeB.Blocks = []string{"C"}
	nodeC.Blocks = []string{"A"}

	g := d

	// Scope contains only A; the cycle A->B->C->A closes within scope (at A)
	scope := map[string]bool{"A": true}
	hasCycle := g.ScopedHasCycle("A", scope)
	assert.True(t, hasCycle, "expected ScopedHasCycle to detect cycle closing within scope")
}

// TestScopedHasCycleOutOfScope tests that ScopedHasCycle ignores cycles entirely outside the scope.
func TestScopedHasCycleOutOfScope(t *testing.T) {
	t.Parallel()
	d := New()
	// Create cycle B -> C -> B, with A not in the cycle
	nodeA := &Node{ID: "A", Title: "A", Type: "task"}
	nodeB := &Node{ID: "B", Title: "B", Type: "task", BlockedBy: []string{"C"}}
	nodeC := &Node{ID: "C", Title: "C", Type: "task", BlockedBy: []string{"B"}}

	require.NoError(t, d.AddNode(nodeA))
	require.NoError(t, d.AddNode(nodeB))
	require.NoError(t, d.AddNode(nodeC))

	nodeB.Blocks = []string{"C"}
	nodeC.Blocks = []string{"B"}

	g := d

	// Scope contains only A; the cycle B->C->B is entirely outside scope
	scope := map[string]bool{"A": true}
	hasCycle := g.ScopedHasCycle("A", scope)
	assert.False(t, hasCycle, "expected ScopedHasCycle to return false for out-of-scope cycles")
}

// TestScopedHasCycleWithChildrenEdges tests that ScopedHasCycle respects scope boundaries on Children edges.
func TestScopedHasCycleWithChildrenEdges(t *testing.T) {
	t.Parallel()
	d := New()
	// Create hierarchy: A -> B -> C, all in scope
	// Also B -> D (out of scope), C -> A (closing parent-child cycle within scope)
	nodeA := &Node{ID: "A", Title: "A", Type: "epic", Children: []string{"B"}}
	nodeB := &Node{ID: "B", Title: "B", Type: "story", Parent: "A", Children: []string{"C", "D"}}
	nodeC := &Node{ID: "C", Title: "C", Type: "task", Parent: "B", BlockedBy: []string{"A"}}
	nodeD := &Node{ID: "D", Title: "D", Type: "task", Parent: "B"}

	require.NoError(t, d.AddNode(nodeA))
	require.NoError(t, d.AddNode(nodeB))
	require.NoError(t, d.AddNode(nodeC))
	require.NoError(t, d.AddNode(nodeD))

	nodeC.Blocks = []string{"A"}

	g := d

	// Scope contains A, B, C (not D)
	scope := map[string]bool{"A": true, "B": true, "C": true}
	hasCycle := g.ScopedHasCycle("A", scope)
	assert.True(t, hasCycle, "expected ScopedHasCycle to detect cycle in parent-child + blocker edges within scope")
}

// TestScopedHasCycleNoCycle tests that ScopedHasCycle returns false when there's no cycle.
func TestScopedHasCycleNoCycle(t *testing.T) {
	t.Parallel()
	d := New()
	nodeA := &Node{ID: "A", Title: "A", Type: "task", BlockedBy: []string{"B"}}
	nodeB := &Node{ID: "B", Title: "B", Type: "task"}

	require.NoError(t, d.AddNode(nodeA))
	require.NoError(t, d.AddNode(nodeB))

	nodeB.Blocks = []string{"A"}

	g := d

	scope := map[string]bool{"A": true, "B": true}
	hasCycle := g.ScopedHasCycle("A", scope)
	assert.False(t, hasCycle, "expected ScopedHasCycle to return false for acyclic graph")
}
