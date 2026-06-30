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
	d := New()
	task1 := &Node{ID: "task-1", Title: "Task 1", Type: "task", BlockedBy: []string{"task-2"}}
	task2 := &Node{ID: "task-2", Title: "Task 2", Type: "task", BlockedBy: []string{"task-1"}}

	require.NoError(t, d.AddNode(task1))
	require.NoError(t, d.AddNode(task2))

	assert.True(t, d.HasCycle())
}

// TestPropertyNoSelfCycles: Generate arbitrary nodes and verify no node blocks itself.
func TestPropertyNoSelfCycles(t *testing.T) {
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
func BenchmarkCycleDetection(b *testing.B) {
	d := New()

	// Create a tree of 100 nodes
	for i := 0; i < 100; i++ {
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
	for i := 0; i < b.N; i++ {
		d.HasCycle()
	}
}
