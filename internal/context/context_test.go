package context

import (
	"fmt"
	"strings"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stateDir = "/tmp/fake"

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

	ctx, err := Assemble("TST-001", stateDir, state)
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

	ctx, err := Assemble("TST-A", stateDir, state)
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

	ctx, err := Assemble("TST-C", stateDir, state)
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

// TC-003: Tests for buildSnippets, buildDecisions, buildNotes, buildSiblingOutcomes

func TestAssembleContext_UnknownIssue(t *testing.T) {
	state := materialize.NewState()
	_, err := Assemble("MISSING-001", stateDir, state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MISSING-001")
}

func TestBuildSnippets_WithContext(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TST-001"] = &materialize.Issue{
		ID:           "TST-001",
		Title:        "Test",
		Type:         "task",
		Status:       "open",
		Context:      []byte(`{"key": "value", "foo": "bar"}`),
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-001", stateDir, state)
	require.NoError(t, err)

	var snippetsLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "snippets" {
			snippetsLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, snippetsLayer)
	assert.Contains(t, snippetsLayer.Content, "key")
	assert.Contains(t, snippetsLayer.Content, "value")
}

func TestBuildSnippets_InvalidJSON(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TST-001"] = &materialize.Issue{
		ID:           "TST-001",
		Title:        "Test",
		Type:         "task",
		Status:       "open",
		Context:      []byte(`not json`),
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-001", stateDir, state)
	require.NoError(t, err)

	for _, l := range ctx.Layers {
		if l.Name == "snippets" {
			assert.Empty(t, l.Content)
		}
	}
}

func TestBuildDecisions(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TST-001"] = &materialize.Issue{
		ID:     "TST-001",
		Title:  "Test",
		Type:   "task",
		Status: "open",
		Decisions: []materialize.Decision{
			{Topic: "db", Choice: "postgres", Rationale: "mature", Timestamp: 100},
		},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-001", stateDir, state)
	require.NoError(t, err)

	var decLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "decisions" {
			decLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, decLayer)
	assert.Contains(t, decLayer.Content, "db")
	assert.Contains(t, decLayer.Content, "postgres")
	assert.Contains(t, decLayer.Content, "mature")
}

func TestBuildNotes_WithNotes(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TST-001"] = &materialize.Issue{
		ID:     "TST-001",
		Title:  "Test",
		Type:   "task",
		Status: "open",
		Notes: []materialize.Note{
			{WorkerID: "w1", Msg: "first note", Timestamp: 1000},
			{WorkerID: "w1", Msg: "second note", Timestamp: 2000},
		},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-001", stateDir, state)
	require.NoError(t, err)

	var notesLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "notes" {
			notesLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, notesLayer)
	assert.Contains(t, notesLayer.Content, "first note")
	assert.Contains(t, notesLayer.Content, "second note")
}

func TestBuildNotes_TruncatesAtFive(t *testing.T) {
	state := materialize.NewState()
	notes := make([]materialize.Note, 7)
	for i := range notes {
		notes[i] = materialize.Note{WorkerID: "w1", Msg: fmt.Sprintf("note-%d", i), Timestamp: int64(i * 100)}
	}
	state.Issues["TST-001"] = &materialize.Issue{
		ID:           "TST-001",
		Title:        "Test",
		Type:         "task",
		Status:       "open",
		Notes:        notes,
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-001", stateDir, state)
	require.NoError(t, err)

	var notesLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "notes" {
			notesLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, notesLayer)
	assert.Contains(t, notesLayer.Content, "note-6")
	assert.Contains(t, notesLayer.Content, "note-2")
	assert.NotContains(t, notesLayer.Content, "note-0")
	assert.NotContains(t, notesLayer.Content, "note-1")
}

func TestBuildSiblingOutcomes(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TST-P"] = &materialize.Issue{
		ID:           "TST-P",
		Title:        "Parent",
		Type:         "story",
		Status:       "in-progress",
		Children:     []string{"TST-A", "TST-B"},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-A"] = &materialize.Issue{
		ID:           "TST-A",
		Title:        "Task A",
		Type:         "task",
		Status:       "done",
		Outcome:      "completed A",
		Parent:       "TST-P",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-B"] = &materialize.Issue{
		ID:           "TST-B",
		Title:        "Task B",
		Type:         "task",
		Status:       "open",
		Parent:       "TST-P",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-B", stateDir, state)
	require.NoError(t, err)

	var sibLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "sibling_outcomes" {
			sibLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, sibLayer)
	assert.Contains(t, sibLayer.Content, "TST-A")
	assert.Contains(t, sibLayer.Content, "completed A")
	assert.NotContains(t, sibLayer.Content, "TST-B")
}

func TestBuildSiblingOutcomes_NoParent(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TST-001"] = &materialize.Issue{
		ID:           "TST-001",
		Title:        "Task",
		Type:         "task",
		Status:       "open",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-001", stateDir, state)
	require.NoError(t, err)

	for _, l := range ctx.Layers {
		if l.Name == "sibling_outcomes" {
			assert.Empty(t, l.Content)
		}
	}
}

// TC-004: Tests for RenderAgent and RenderHuman

func TestRenderAgent(t *testing.T) {
	ctx := &Context{
		IssueID: "TST-001",
		Layers: []Layer{
			{Name: "core_spec", Priority: 1, Content: "Issue: Fix bug"},
		},
	}

	out, err := RenderAgent(ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "TST-001")
	assert.Contains(t, out, "core_spec")
	assert.Contains(t, out, "Fix bug")
	assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "{"))
}

func TestRenderHuman(t *testing.T) {
	ctx := &Context{
		IssueID: "TST-001",
		Layers: []Layer{
			{Name: "core_spec", Priority: 1, Content: "Issue: Fix bug"},
			{Name: "notes", Priority: 6, Content: "Some note"},
		},
	}

	out := RenderHuman(ctx)
	assert.Contains(t, out, "=== core_spec ===")
	assert.Contains(t, out, "Issue: Fix bug")
	assert.Contains(t, out, "=== notes ===")
	assert.Contains(t, out, "Some note")
}

// TC-005: Truncate boundary condition tests

func TestTruncate_ExactlyAtBudget_NoTruncation(t *testing.T) {
	ctx := &Context{
		IssueID: "TST-001",
		Layers: []Layer{
			{Name: "core_spec", Priority: 1, Content: strings.Repeat("a", 60)},
			{Name: "notes", Priority: 6, Content: strings.Repeat("b", 40)},
		},
	}

	result := Truncate(ctx, 25) // charBudget = 100, total = 100
	assert.Len(t, result.Layers, 2, "should not truncate when total == charBudget")
}

func TestTruncate_OneBelowBudget_NoTruncation(t *testing.T) {
	ctx := &Context{
		IssueID: "TST-001",
		Layers: []Layer{
			{Name: "core_spec", Priority: 1, Content: strings.Repeat("a", 59)},
			{Name: "notes", Priority: 6, Content: strings.Repeat("b", 40)},
		},
	}

	result := Truncate(ctx, 25) // charBudget = 100, total = 99
	assert.Len(t, result.Layers, 2)
}

func TestTruncate_SingleLayer_NeverRemoved(t *testing.T) {
	ctx := &Context{
		IssueID: "TST-001",
		Layers: []Layer{
			{Name: "core_spec", Priority: 1, Content: strings.Repeat("a", 1000)},
		},
	}

	result := Truncate(ctx, 1) // charBudget = 4, total = 1000
	assert.Len(t, result.Layers, 1, "single layer must never be removed")
	assert.Equal(t, "core_spec", result.Layers[0].Name)
}

func TestTruncate_EqualPriority_RemovesHigherIndex(t *testing.T) {
	ctx := &Context{
		IssueID: "TST-001",
		Layers: []Layer{
			{Name: "core_spec", Priority: 1, Content: strings.Repeat("a", 60)},
			{Name: "decisions", Priority: 5, Content: strings.Repeat("b", 60)},
			{Name: "notes", Priority: 5, Content: strings.Repeat("c", 60)},
		},
	}

	result := Truncate(ctx, 30) // charBudget = 120, total = 180
	assert.Len(t, result.Layers, 2)
	found := false
	for _, l := range result.Layers {
		if l.Name == "core_spec" {
			found = true
		}
	}
	assert.True(t, found, "core_spec (priority 1) must survive truncation")
}
