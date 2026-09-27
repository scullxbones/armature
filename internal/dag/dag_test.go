package dag

import (
	"fmt"
	"slices"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddNodeDuplicate(t *testing.T) {
	t.Parallel()
	d := newGraph()
	node := &Node{ID: "task-1", Title: "Test", Type: "task"}

	err := d.addNode(node)
	require.NoError(t, err)

	err = d.addNode(node)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestNoCycleInAcyclicDAG(t *testing.T) {
	t.Parallel()
	d := newGraph()
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story := &Node{ID: "story-1", Title: "Story", Type: "story", Parent: "epic-1"}
	task := &Node{ID: "task-1", Title: "Task", Type: "task", Parent: "story-1"}

	require.NoError(t, d.addNode(epic))
	require.NoError(t, d.addNode(story))
	require.NoError(t, d.addNode(task))

	epic.Children = []string{"story-1"}
	story.Children = []string{"task-1"}

	assert.False(t, d.HasCycle())
}

func TestCycleDetection(t *testing.T) {
	t.Parallel()
	d := newGraph()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", BlockedBy: []string{"task-2"}}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}

	require.NoError(t, d.addNode(task1))
	require.NoError(t, d.addNode(task2))

	assert.True(t, d.HasCycle())
}

func TestPropertyNoSelfCycles(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("no node can block itself", prop.ForAll(
		func(nodeID string) bool {
			d := newGraph()
			node := &Node{
				ID:        nodeID,
				Title:     "Test",
				Type:      "task",
				BlockedBy: []string{nodeID},
			}
			if err := d.addNode(node); err != nil {
				return false
			}
			return d.HasCycle()
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

func TestPropertyParentChildConsistency(t *testing.T) {
	t.Parallel()
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50

	properties := gopter.NewProperties(parameters)

	properties.Property("parent-child consistency is maintained", prop.ForAll(
		func(parentID, childID string) bool {
			if parentID == childID || parentID == "" || childID == "" {
				return true
			}

			d := newGraph()
			parent := &Node{ID: parentID, Title: "Parent", Type: "story"}
			child := &Node{ID: childID, Title: "Child", Type: "task", Parent: parentID}

			parent.Children = []string{childID}

			if err := d.addNode(parent); err != nil {
				return false
			}
			if err := d.addNode(child); err != nil {
				return false
			}

			parentNode := d.nodes[parentID]
			return parentNode != nil && slices.Contains(parentNode.Children, childID)
		},
		gen.AlphaString(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

func BenchmarkCycleDetection(b *testing.B) {
	d := newGraph()

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
		if err := d.addNode(node); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for b.Loop() {
		d.HasCycle()
	}
}

func TestGraphAncestry(t *testing.T) {
	t.Parallel()
	d := newGraph()
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story := &Node{ID: "story-1", Title: "Story", Type: "story", Parent: "epic-1"}
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", Parent: "story-1"}

	require.NoError(t, d.addNode(epic))
	require.NoError(t, d.addNode(story))
	require.NoError(t, d.addNode(task1))

	epic.Children = []string{"story-1"}
	story.Children = []string{"task-1"}

	g := d

	ancestors := g.Ancestry("task-1")
	assert.ElementsMatch(t, []string{"story-1", "epic-1"}, ancestors)

	ancestors = g.Ancestry("story-1")
	assert.ElementsMatch(t, []string{"epic-1"}, ancestors)

	ancestors = g.Ancestry("epic-1")
	assert.Empty(t, ancestors)
}

func TestGraphDescendants(t *testing.T) {
	t.Parallel()
	d := newGraph()
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story1 := &Node{ID: "story-1", Title: "Story 1", Type: "story", Parent: "epic-1"}
	story2 := &Node{ID: "story-2", Title: "Story 2", Type: "story", Parent: "epic-1"}
	task := &Node{ID: "task-1", Title: "Task 1", Type: "task", Parent: "story-1"}

	require.NoError(t, d.addNode(epic))
	require.NoError(t, d.addNode(story1))
	require.NoError(t, d.addNode(story2))
	require.NoError(t, d.addNode(task))

	epic.Children = []string{"story-1", "story-2"}
	story1.Children = []string{"task-1"}
	story2.Children = []string{}

	g := d

	descendants := g.Descendants("epic-1")
	assert.ElementsMatch(t, []string{"story-1", "story-2", "task-1"}, descendants)

	descendants = g.Descendants("story-1")
	assert.ElementsMatch(t, []string{"task-1"}, descendants)

	descendants = g.Descendants("task-1")
	assert.Empty(t, descendants)
}

func TestGraphBlockers(t *testing.T) {
	t.Parallel()
	d := newGraph()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task"}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}
	task3 := &Node{ID: "task-3", Title: "Task 3", Type: "task", BlockedBy: []string{"task-1", "task-2"}}

	require.NoError(t, d.addNode(task1))
	require.NoError(t, d.addNode(task2))
	require.NoError(t, d.addNode(task3))

	task1.Blocks = []string{"task-2", "task-3"}
	task2.Blocks = []string{"task-3"}

	g := d

	blockers := g.blockers("task-2")
	assert.ElementsMatch(t, []string{"task-1"}, blockers)

	blockers = g.blockers("task-3")
	assert.ElementsMatch(t, []string{"task-1", "task-2"}, blockers)

	blockers = g.blockers("task-1")
	assert.Empty(t, blockers)
}

func TestGraphHierarchy(t *testing.T) {
	t.Parallel()
	d := newGraph()
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story := &Node{ID: "story-1", Title: "Story", Type: "story", Parent: "epic-1"}
	task := &Node{ID: "task-1", Title: "Task", Type: "task", Parent: "story-1"}

	require.NoError(t, d.addNode(epic))
	require.NoError(t, d.addNode(story))
	require.NoError(t, d.addNode(task))

	epic.Children = []string{"story-1"}
	story.Children = []string{"task-1"}

	g := d

	parent, children := g.Hierarchy("story-1")
	assert.Equal(t, "epic-1", parent)
	assert.ElementsMatch(t, []string{"task-1"}, children)

	parent, children = g.Hierarchy("epic-1")
	assert.Equal(t, "", parent)
	assert.ElementsMatch(t, []string{"story-1"}, children)

	parent, children = g.Hierarchy("task-1")
	assert.Equal(t, "story-1", parent)
	assert.Empty(t, children)
}

func TestGraphHasCycle(t *testing.T) {
	t.Parallel()
	d := newGraph()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", BlockedBy: []string{"task-2"}}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task"}

	require.NoError(t, d.addNode(task1))
	require.NoError(t, d.addNode(task2))
	task2.Blocks = []string{"task-1"}

	g := d
	assert.False(t, g.HasCycle())

	d2 := newGraph()
	task3 := &Node{ID: "task-3", Title: "Task 3", Type: "task", BlockedBy: []string{"task-4"}}
	task4 := &Node{ID: "task-4", Title: "Task 4", Type: "task", BlockedBy: []string{"task-3"}}

	require.NoError(t, d2.addNode(task3))
	require.NoError(t, d2.addNode(task4))

	task3.Blocks = []string{"task-4"}
	task4.Blocks = []string{"task-3"}

	g2 := d2
	assert.True(t, g2.HasCycle())
}

func TestGraphDepth(t *testing.T) {
	t.Parallel()
	d := newGraph()
	epic := &Node{ID: "epic-1", Title: "Epic", Type: "epic"}
	story := &Node{ID: "story-1", Title: "Story", Type: "story", Parent: "epic-1"}
	task := &Node{ID: "task-1", Title: "Task", Type: "task", Parent: "story-1"}

	require.NoError(t, d.addNode(epic))
	require.NoError(t, d.addNode(story))
	require.NoError(t, d.addNode(task))

	epic.Children = []string{"story-1"}
	story.Children = []string{"task-1"}

	g := d

	assert.Equal(t, 0, g.Depth("epic-1"))
	assert.Equal(t, 1, g.Depth("story-1"))
	assert.Equal(t, 2, g.Depth("task-1"))
}

func TestGraphDepthWithMultipleRoots(t *testing.T) {
	t.Parallel()
	d := newGraph()
	epic1 := &Node{ID: "epic-1", Title: "Epic 1", Type: "epic"}
	epic2 := &Node{ID: "epic-2", Title: "Epic 2", Type: "epic"}
	story1 := &Node{ID: "story-1", Title: "Story 1", Type: "story", Parent: "epic-1"}

	require.NoError(t, d.addNode(epic1))
	require.NoError(t, d.addNode(epic2))
	require.NoError(t, d.addNode(story1))

	epic1.Children = []string{"story-1"}

	g := d

	assert.Equal(t, 0, g.Depth("epic-1"))
	assert.Equal(t, 0, g.Depth("epic-2"))
	assert.Equal(t, 1, g.Depth("story-1"))
}

func TestGraphAncestryNonexistentNode(t *testing.T) {
	t.Parallel()
	d := newGraph()
	g := d

	ancestors := g.Ancestry("nonexistent")
	assert.Empty(t, ancestors)
}

func TestGraphDescendantsNonexistentNode(t *testing.T) {
	t.Parallel()
	d := newGraph()
	g := d

	descendants := g.Descendants("nonexistent")
	assert.Empty(t, descendants)
}

func TestGraphBlockersNonexistentNode(t *testing.T) {
	t.Parallel()
	d := newGraph()
	g := d

	blockers := g.blockers("nonexistent")
	assert.Nil(t, blockers)
}

func TestGraphHierarchyNonexistentNode(t *testing.T) {
	t.Parallel()
	d := newGraph()
	g := d

	parent, children := g.Hierarchy("nonexistent")
	assert.Equal(t, "", parent)
	assert.Nil(t, children)
}

func TestGraphDepthNonexistentNode(t *testing.T) {
	t.Parallel()
	d := newGraph()
	g := d

	depth := g.Depth("nonexistent")
	assert.Equal(t, 0, depth)
}

func TestGraphBlockersEmptyNode(t *testing.T) {
	t.Parallel()
	d := newGraph()
	task := &Node{ID: "task-1", Title: "Task 1", Type: "task"}

	require.NoError(t, d.addNode(task))

	g := d

	blockers := g.blockers("task-1")
	assert.Empty(t, blockers)
}

func TestGraphNode(t *testing.T) {
	t.Parallel()
	d := newGraph()
	task := &Node{ID: "task-1", Title: "Task 1", Type: "task"}

	require.NoError(t, d.addNode(task))

	retrieved := d.nodes["task-1"]
	assert.NotNil(t, retrieved)
	assert.Equal(t, "task-1", retrieved.ID)

	notFound := d.nodes["nonexistent"]
	assert.Nil(t, notFound)
}

func TestHasCycle_DanglingChildReference(t *testing.T) {
	t.Parallel()
	d := newGraph()
	node := &Node{ID: "a", Title: "Node A", Type: "task", Children: []string{"nonexistent"}}
	require.NoError(t, d.addNode(node))

	assert.False(t, d.HasCycle())
}

func TestBlockersMutationSafety(t *testing.T) {
	t.Parallel()
	d := newGraph()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task"}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}

	require.NoError(t, d.addNode(task1))
	require.NoError(t, d.addNode(task2))

	g := d

	original := g.blockers("task-2")

	mutated := append(g.blockers("task-2"), "injected")

	assert.ElementsMatch(t, original, g.blockers("task-2"))
	assert.NotContains(t, g.blockers("task-2"), "injected")
	assert.NotEqual(t, len(g.blockers("task-2")), len(mutated))
}

func TestHierarchyMutationSafety(t *testing.T) {
	t.Parallel()
	d := newGraph()
	parent := &Node{ID: "parent-1", Title: "Parent", Type: "story"}
	child := &Node{ID: "child-1", Title: "Child", Type: "task", Parent: "parent-1"}

	parent.Children = []string{"child-1"}

	require.NoError(t, d.addNode(parent))
	require.NoError(t, d.addNode(child))

	g := d

	_, originalChildren := g.Hierarchy("parent-1")

	_, returned := g.Hierarchy("parent-1")
	mutated := append(returned, "injected") //nolint:gocritic // intentional separate slice to test immutability

	_, currentChildren := g.Hierarchy("parent-1")
	assert.ElementsMatch(t, originalChildren, currentChildren)
	assert.NotContains(t, currentChildren, "injected")
	assert.NotEqual(t, len(currentChildren), len(mutated))
}

func TestGraph_Depth_CycleGuard(t *testing.T) {
	t.Parallel()
	d := newGraph()
	nodeA := &Node{ID: "a", Title: "Node A", Type: "task", Parent: "b"}
	nodeB := &Node{ID: "b", Title: "Node B", Type: "task", Parent: "a"}

	require.NoError(t, d.addNode(nodeA))
	require.NoError(t, d.addNode(nodeB))

	g := d

	depth := g.Depth("a")
	assert.GreaterOrEqual(t, depth, 0)
	assert.Equal(t, 2, depth)
}

func TestGraph_Ancestry_CycleGuard(t *testing.T) {
	t.Parallel()
	d := newGraph()
	nodeA := &Node{ID: "a", Title: "Node A", Type: "task", Parent: "b"}
	nodeB := &Node{ID: "b", Title: "Node B", Type: "task", Parent: "a"}

	require.NoError(t, d.addNode(nodeA))
	require.NoError(t, d.addNode(nodeB))

	g := d

	ancestors := g.Ancestry("a")
	assert.NotNil(t, ancestors)
	assert.ElementsMatch(t, []string{"b"}, ancestors)
}

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

	ancestors := g.Ancestry("task-1")
	assert.ElementsMatch(t, []string{"story-1", "epic-1"}, ancestors)

	descendants := g.Descendants("epic-1")
	assert.ElementsMatch(t, []string{"story-1", "task-1"}, descendants)
}

func TestFromIndexEmpty(t *testing.T) {
	t.Parallel()
	index := make(map[string]*Node)

	g := FromIndex(index)
	require.NotNil(t, g)

	ancestors := g.Ancestry("nonexistent")
	assert.Empty(t, ancestors)

	descendants := g.Descendants("nonexistent")
	assert.Empty(t, descendants)
}

func TestScopedHasCycleCrossScope(t *testing.T) {
	t.Parallel()
	d := newGraph()
	nodeA := &Node{ID: "A", Title: "A", Type: "task", BlockedBy: []string{"C"}}
	nodeB := &Node{ID: "B", Title: "B", Type: "task", BlockedBy: []string{"A"}}
	nodeC := &Node{ID: "C", Title: "C", Type: "task", BlockedBy: []string{"B"}}

	require.NoError(t, d.addNode(nodeA))
	require.NoError(t, d.addNode(nodeB))
	require.NoError(t, d.addNode(nodeC))

	nodeA.Blocks = []string{"B"}
	nodeB.Blocks = []string{"C"}
	nodeC.Blocks = []string{"A"}

	g := d

	scope := map[string]bool{"A": true}
	hasCycle := g.ScopedHasCycle("A", scope)
	assert.True(t, hasCycle, "expected ScopedHasCycle to detect cycle closing within scope")
}

func TestScopedHasCycleOutOfScope(t *testing.T) {
	t.Parallel()
	d := newGraph()
	nodeA := &Node{ID: "A", Title: "A", Type: "task"}
	nodeB := &Node{ID: "B", Title: "B", Type: "task", BlockedBy: []string{"C"}}
	nodeC := &Node{ID: "C", Title: "C", Type: "task", BlockedBy: []string{"B"}}

	require.NoError(t, d.addNode(nodeA))
	require.NoError(t, d.addNode(nodeB))
	require.NoError(t, d.addNode(nodeC))

	nodeB.Blocks = []string{"C"}
	nodeC.Blocks = []string{"B"}

	g := d

	scope := map[string]bool{"A": true}
	hasCycle := g.ScopedHasCycle("A", scope)
	assert.False(t, hasCycle, "expected ScopedHasCycle to return false for out-of-scope cycles")
}

func TestScopedHasCycleWithChildrenEdges(t *testing.T) {
	t.Parallel()
	d := newGraph()
	nodeA := &Node{ID: "A", Title: "A", Type: "epic", Children: []string{"B"}}
	nodeB := &Node{ID: "B", Title: "B", Type: "story", Parent: "A", Children: []string{"C", "D"}}
	nodeC := &Node{ID: "C", Title: "C", Type: "task", Parent: "B", BlockedBy: []string{"A"}}
	nodeD := &Node{ID: "D", Title: "D", Type: "task", Parent: "B"}

	require.NoError(t, d.addNode(nodeA))
	require.NoError(t, d.addNode(nodeB))
	require.NoError(t, d.addNode(nodeC))
	require.NoError(t, d.addNode(nodeD))

	nodeC.Blocks = []string{"A"}

	g := d

	scope := map[string]bool{"A": true, "B": true, "C": true}
	hasCycle := g.ScopedHasCycle("A", scope)
	assert.True(t, hasCycle, "expected ScopedHasCycle to detect cycle in parent-child + blocker edges within scope")
}

func TestScopedHasCycleNoCycle(t *testing.T) {
	t.Parallel()
	d := newGraph()
	nodeA := &Node{ID: "A", Title: "A", Type: "task", BlockedBy: []string{"B"}}
	nodeB := &Node{ID: "B", Title: "B", Type: "task"}

	require.NoError(t, d.addNode(nodeA))
	require.NoError(t, d.addNode(nodeB))

	nodeB.Blocks = []string{"A"}

	g := d

	scope := map[string]bool{"A": true, "B": true}
	hasCycle := g.ScopedHasCycle("A", scope)
	assert.False(t, hasCycle, "expected ScopedHasCycle to return false for acyclic graph")
}
