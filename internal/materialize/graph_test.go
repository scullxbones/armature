package materialize_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphFromState_Empty(t *testing.T) {
	t.Parallel()
	state := materialize.NewState()
	graph := materialize.GraphFromState(state)
	require.NotNil(t, graph)
}

func TestGraphFromState_ParentChild(t *testing.T) {
	t.Parallel()
	state := materialize.NewState()
	state.Issues["EPIC-1"] = &materialize.Issue{
		ID:       "EPIC-1",
		Type:     "epic",
		Children: []string{"STORY-1"},
	}
	state.Issues["STORY-1"] = &materialize.Issue{
		ID:     "STORY-1",
		Type:   "story",
		Parent: "EPIC-1",
	}

	graph := materialize.GraphFromState(state)
	require.NotNil(t, graph)

	parent, children := graph.Hierarchy("EPIC-1")
	assert.Equal(t, "", parent)
	assert.Contains(t, children, "STORY-1")

	parent2, _ := graph.Hierarchy("STORY-1")
	assert.Equal(t, "EPIC-1", parent2)
}

func TestGraphFromState_DefensiveCopy(t *testing.T) {
	t.Parallel()
	state := materialize.NewState()
	state.Issues["T1"] = &materialize.Issue{
		ID:        "T1",
		Children:  []string{"T2"},
		BlockedBy: []string{"T3"},
		Blocks:    []string{"T4"},
	}

	graph := materialize.GraphFromState(state)
	require.NotNil(t, graph)

	// Mutate the original state's slices; graph must not be affected.
	state.Issues["T1"].Children[0] = "MUTATED"
	state.Issues["T1"].BlockedBy[0] = "MUTATED"
	state.Issues["T1"].Blocks[0] = "MUTATED"

	_, children := graph.Hierarchy("T1")
	assert.Contains(t, children, "T2", "graph must have defensive copy of Children")
	assert.NotContains(t, children, "MUTATED")
}

func TestGraphFromState_Descendants(t *testing.T) {
	t.Parallel()
	state := materialize.NewState()
	state.Issues["E1"] = &materialize.Issue{ID: "E1", Type: "epic", Children: []string{"S1"}}
	state.Issues["S1"] = &materialize.Issue{ID: "S1", Type: "story", Parent: "E1", Children: []string{"T1"}}
	state.Issues["T1"] = &materialize.Issue{ID: "T1", Type: "task", Parent: "S1"}

	graph := materialize.GraphFromState(state)
	descendants := graph.Descendants("E1")
	assert.Contains(t, descendants, "S1")
	assert.Contains(t, descendants, "T1")
}
