package context

import (
	"strings"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssembleContext_CoreSpec(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TST-001"] = &materialize.Issue{
		ID:               "TST-001",
		Title:            "Fix the thing",
		Type:             "task",
		Scope:            []string{"backend"},
		Priority:         "high",
		DefinitionOfDone: "All tests pass",
		Status:           "open",
		Children:         []string{},
		BlockedBy:        []string{},
		Blocks:           []string{},
		DecisionRefs:     []string{},
	}

	ctx, err := Assemble("TST-001", "/tmp/fake", state)
	require.NoError(t, err)
	require.NotEmpty(t, ctx.Layers)

	layer := ctx.Layers[0]
	assert.Equal(t, "core_spec", layer.Name)
	assert.Equal(t, 1, layer.Priority)
	assert.Contains(t, layer.Content, "Fix the thing")
	assert.Contains(t, layer.Content, "task")
	assert.Contains(t, layer.Content, "backend")
	assert.Contains(t, layer.Content, "high")
	assert.Contains(t, layer.Content, "All tests pass")
}

func TestAssembleContext_BlockerOutcomes(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TST-B"] = &materialize.Issue{
		ID:           "TST-B",
		Title:        "Blocker issue",
		Type:         "task",
		Status:       "done",
		Outcome:      "fixed",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{"TST-A"},
		DecisionRefs: []string{},
	}
	state.Issues["TST-A"] = &materialize.Issue{
		ID:           "TST-A",
		Title:        "Main issue",
		Type:         "task",
		Status:       "open",
		BlockedBy:    []string{"TST-B"},
		Blocks:       []string{},
		Children:     []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-A", "/tmp/fake", state)
	require.NoError(t, err)

	var blockerLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "blocker_outcomes" {
			blockerLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, blockerLayer)
	assert.Contains(t, blockerLayer.Content, "fixed")
}

func TestAssembleContext_ParentChain(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TST-P"] = &materialize.Issue{
		ID:           "TST-P",
		Title:        "Parent Story",
		Type:         "story",
		Status:       "in-progress",
		Children:     []string{"TST-C"},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-C"] = &materialize.Issue{
		ID:           "TST-C",
		Title:        "Child task",
		Type:         "task",
		Status:       "open",
		Parent:       "TST-P",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-C", "/tmp/fake", state)
	require.NoError(t, err)

	var parentLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "parent_chain" {
			parentLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, parentLayer)
	assert.Contains(t, parentLayer.Content, "Parent Story")
}

func TestAssembleContext_Truncation(t *testing.T) {
	ctx := &Context{
		IssueID: "TST-001",
		Layers: []Layer{
			{Name: "core_spec", Priority: 1, Content: strings.Repeat("a", 100)},
			{Name: "decisions", Priority: 5, Content: strings.Repeat("b", 100)},
			{Name: "notes", Priority: 6, Content: strings.Repeat("c", 100)},
		},
	}

	// total chars = 300; budget chars = tokenBudget * 4
	// Set budget so that 300 > budget*4 but 200 <= budget*4
	// budget = 60 => charBudget = 240 => 300 > 240, remove priority 6
	// After removal: 200 <= 240, done
	truncated := Truncate(ctx, 60)

	assert.Len(t, truncated.Layers, 2)
	for _, l := range truncated.Layers {
		assert.NotEqual(t, "notes", l.Name, "notes layer (priority 6) should have been removed")
	}
}
