package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// emptyFileReader is a test FileReader that returns an error for all files.
type emptyFileReader struct{}

func (f *emptyFileReader) ReadFile(relPath string) ([]byte, error) {
	return nil, fmt.Errorf("file not found: %s", relPath)
}

// realFileReader reads files from the filesystem.
type realFileReader struct {
	root string
}

func (r *realFileReader) ReadFile(relPath string) ([]byte, error) {
	fullPath := filepath.Join(r.root, relPath)
	return os.ReadFile(fullPath)
}

func TestAssembleContext_CoreSpec(t *testing.T) {
	t.Parallel()
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

	ctx, err := Assemble("TST-001", state, &emptyFileReader{})
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
	t.Parallel()
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

	ctx, err := Assemble("TST-A", state, &emptyFileReader{})
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

func TestAssembleContext_BlockerOutcomes_ShowsStatusForInProgressBlocker(t *testing.T) {
	t.Parallel()
	state := materialize.NewState()
	state.Issues["TST-B"] = &materialize.Issue{
		ID:           "TST-B",
		Title:        "Blocking task in progress",
		Type:         "task",
		Status:       "in-progress",
		Outcome:      "",
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

	ctx, err := Assemble("TST-A", state, &emptyFileReader{})
	require.NoError(t, err)

	var blockerLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "blocker_outcomes" {
			blockerLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, blockerLayer)
	// For an in-progress blocker with no outcome, should show the status
	assert.Contains(t, blockerLayer.Content, "TST-B")
	assert.Contains(t, blockerLayer.Content, "in-progress")
}

func TestAssembleContext_BlockerOutcomes_PreferOutcomeOverStatus(t *testing.T) {
	t.Parallel()
	state := materialize.NewState()
	state.Issues["TST-B"] = &materialize.Issue{
		ID:           "TST-B",
		Title:        "Blocking task",
		Type:         "task",
		Status:       "done",
		Outcome:      "fixed with edge-case handling",
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

	ctx, err := Assemble("TST-A", state, &emptyFileReader{})
	require.NoError(t, err)

	var blockerLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "blocker_outcomes" {
			blockerLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, blockerLayer)
	// When outcome is available, should show outcome (not status)
	assert.Contains(t, blockerLayer.Content, "TST-B")
	assert.Contains(t, blockerLayer.Content, "fixed with edge-case handling")
	// Outcome should be present, status should not interfere
	assert.NotContains(t, blockerLayer.Content, "outcome unknown")
}

func TestAssembleContext_ParentChain(t *testing.T) {
	t.Parallel()
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

	ctx, err := Assemble("TST-C", state, &emptyFileReader{})
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
	t.Parallel()
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
	t.Parallel()
	state := materialize.NewState()
	_, err := Assemble("MISSING-001", state, &emptyFileReader{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MISSING-001")
}

func TestBuildSnippets_WithContext(t *testing.T) {
	t.Parallel()
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
	ctx, err := Assemble("TST-001", state, &emptyFileReader{})
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

func TestBuildContextFiles_RendersStableReferenceMaterial(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guide.md"), []byte("# Guide\nuse this"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "design.md"), []byte("design context"), 0644))

	state := materialize.NewState()
	state.Issues["TST-001"] = &materialize.Issue{
		ID:           "TST-001",
		Title:        "Test",
		Type:         "task",
		Status:       "open",
		ContextFiles: []string{"guide.md", "docs/design.md"},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	ctx, err := Assemble("TST-001", state, &realFileReader{root: dir})
	require.NoError(t, err)

	var contextFilesLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "context_files" {
			contextFilesLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, contextFilesLayer)
	assert.Contains(t, contextFilesLayer.Content, "## Context Files")
	assert.Contains(t, contextFilesLayer.Content, "guide.md")
	assert.Contains(t, contextFilesLayer.Content, "# Guide")
	assert.Contains(t, contextFilesLayer.Content, "docs/design.md")
	assert.Contains(t, contextFilesLayer.Content, "design context")
	assert.NotContains(t, ctx.Layers[0].Content, "guide.md", "context files must stay separate from write scope")
}

func TestBuildContextFiles_ShowsMissingFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	state := materialize.NewState()
	state.Issues["TST-001"] = &materialize.Issue{
		ID:           "TST-001",
		Title:        "Test",
		Type:         "task",
		Status:       "open",
		ContextFiles: []string{"docs/missing.md"},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-001", state, &realFileReader{root: dir})
	require.NoError(t, err)

	for _, l := range ctx.Layers {
		if l.Name == "context_files" {
			assert.Contains(t, l.Content, "docs/missing.md")
			assert.Contains(t, l.Content, "(missing:")
		}
	}
}

func TestBuildSnippets_InvalidJSON(t *testing.T) {
	t.Parallel()
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
	ctx, err := Assemble("TST-001", state, &emptyFileReader{})
	require.NoError(t, err)

	for _, l := range ctx.Layers {
		if l.Name == "snippets" {
			assert.Empty(t, l.Content)
		}
	}
}

func TestBuildDecisions(t *testing.T) {
	t.Parallel()
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

	ctx, err := Assemble("TST-001", state, &emptyFileReader{})
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
	t.Parallel()
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

	ctx, err := Assemble("TST-001", state, &emptyFileReader{})
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
	t.Parallel()
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

	ctx, err := Assemble("TST-001", state, &emptyFileReader{})
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

func TestBuildNotes_ExcludesDeletedNotes(t *testing.T) {
	t.Parallel()
	state := materialize.NewState()
	state.Issues["TST-001"] = &materialize.Issue{
		ID:     "TST-001",
		Title:  "Test",
		Type:   "task",
		Status: "open",
		Notes: []materialize.Note{
			{ID: "note-1", WorkerID: "w1", Msg: "visible note", Timestamp: 1000},
			{ID: "note-2", WorkerID: "w1", Msg: "deleted note", Timestamp: 2000, Deleted: true},
		},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-001", state, &emptyFileReader{})
	require.NoError(t, err)

	var notesLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "notes" {
			notesLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, notesLayer)
	assert.Contains(t, notesLayer.Content, "visible note")
	assert.NotContains(t, notesLayer.Content, "deleted note")
}

func TestBuildSiblingOutcomes(t *testing.T) {
	t.Parallel()
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

	ctx, err := Assemble("TST-B", state, &emptyFileReader{})
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
	t.Parallel()
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

	ctx, err := Assemble("TST-001", state, &emptyFileReader{})
	require.NoError(t, err)

	for _, l := range ctx.Layers {
		if l.Name == "sibling_outcomes" {
			assert.Empty(t, l.Content)
		}
	}
}

func TestBuildBlockerOutcomes_FromState(t *testing.T) {
	t.Parallel()
	// Blocker must be in state (no more disk fallback)
	state := materialize.NewState()
	state.Issues["TST-BLK"] = &materialize.Issue{
		ID:           "TST-BLK",
		Type:         "task",
		Status:       "done",
		Title:        "Blocker",
		Outcome:      "unblocked successfully",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{"TST-X"},
		DecisionRefs: []string{},
	}
	state.Issues["TST-X"] = &materialize.Issue{
		ID:           "TST-X",
		Title:        "Needs blocker",
		Type:         "task",
		Status:       "open",
		BlockedBy:    []string{"TST-BLK"},
		Children:     []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-X", state, &emptyFileReader{})
	require.NoError(t, err)

	var blockerLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "blocker_outcomes" {
			blockerLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, blockerLayer)
	assert.Contains(t, blockerLayer.Content, "TST-BLK")
	assert.Contains(t, blockerLayer.Content, "unblocked successfully")
}

func TestBuildParentChain_FromState(t *testing.T) {
	t.Parallel()
	// Parent must be in state (no more disk fallback)
	state := materialize.NewState()
	state.Issues["TST-PAR"] = &materialize.Issue{
		ID:           "TST-PAR",
		Type:         "story",
		Status:       "in-progress",
		Title:        "Parent Story",
		Children:     []string{"TST-X"},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-X"] = &materialize.Issue{
		ID:           "TST-X",
		Title:        "Child task",
		Type:         "task",
		Status:       "open",
		Parent:       "TST-PAR",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-X", state, &emptyFileReader{})
	require.NoError(t, err)

	var parentLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "parent_chain" {
			parentLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, parentLayer)
	assert.Contains(t, parentLayer.Content, "TST-PAR")
	assert.Contains(t, parentLayer.Content, "Parent Story")
}

func TestBuildParentChain_WithGrandparent(t *testing.T) {
	t.Parallel()
	// Parent and grandparent are both in state
	// This tests the fix for the bug where grandparents were silently dropped
	state := materialize.NewState()
	state.Issues["TST-GRP"] = &materialize.Issue{
		ID:           "TST-GRP",
		Type:         "epic",
		Status:       "in-progress",
		Title:        "Grandparent Epic",
		Children:     []string{"TST-PAR"},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-PAR"] = &materialize.Issue{
		ID:           "TST-PAR",
		Type:         "story",
		Status:       "in-progress",
		Title:        "Parent Story",
		Parent:       "TST-GRP",
		Children:     []string{"TST-X"},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-X"] = &materialize.Issue{
		ID:           "TST-X",
		Title:        "Child task",
		Type:         "task",
		Status:       "open",
		Parent:       "TST-PAR",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-X", state, &emptyFileReader{})
	require.NoError(t, err)

	var parentLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "parent_chain" {
			parentLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, parentLayer)
	// Both parent and grandparent should be present
	assert.Contains(t, parentLayer.Content, "TST-PAR")
	assert.Contains(t, parentLayer.Content, "Parent Story")
	assert.Contains(t, parentLayer.Content, "TST-GRP")
	assert.Contains(t, parentLayer.Content, "Grandparent Epic")
}

func TestBuildSiblingOutcomes_FromState(t *testing.T) {
	t.Parallel()
	// Sibling must be in state (no more disk fallback)
	state := materialize.NewState()
	state.Issues["TST-PAR"] = &materialize.Issue{
		ID:           "TST-PAR",
		Title:        "Parent",
		Type:         "story",
		Status:       "in-progress",
		Children:     []string{"TST-X", "TST-SIB"},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-X"] = &materialize.Issue{
		ID:           "TST-X",
		Title:        "Current task",
		Type:         "task",
		Status:       "open",
		Parent:       "TST-PAR",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-SIB"] = &materialize.Issue{
		ID:           "TST-SIB",
		Type:         "task",
		Status:       "done",
		Title:        "Sibling",
		Outcome:      "sibling outcome from state",
		Parent:       "TST-PAR",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-X", state, &emptyFileReader{})
	require.NoError(t, err)

	var sibLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "sibling_outcomes" {
			sibLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, sibLayer)
	assert.Contains(t, sibLayer.Content, "TST-SIB")
	assert.Contains(t, sibLayer.Content, "sibling outcome from state")
}

func TestBuildSiblingOutcomes_MultipleParentAndSiblings(t *testing.T) {
	t.Parallel()
	// Parent and siblings are all in state
	state := materialize.NewState()
	state.Issues["TST-PAR2"] = &materialize.Issue{
		ID:           "TST-PAR2",
		Type:         "story",
		Status:       "in-progress",
		Title:        "Parent2",
		Children:     []string{"TST-X2", "TST-SIB2"},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-X2"] = &materialize.Issue{
		ID:           "TST-X2",
		Title:        "Current task",
		Type:         "task",
		Status:       "open",
		Parent:       "TST-PAR2",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	state.Issues["TST-SIB2"] = &materialize.Issue{
		ID:           "TST-SIB2",
		Type:         "task",
		Status:       "done",
		Title:        "Sibling2",
		Outcome:      "state sibling outcome",
		Parent:       "TST-PAR2",
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	ctx, err := Assemble("TST-X2", state, &emptyFileReader{})
	require.NoError(t, err)

	var sibLayer *Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "sibling_outcomes" {
			sibLayer = &ctx.Layers[i]
			break
		}
	}
	require.NotNil(t, sibLayer)
	assert.Contains(t, sibLayer.Content, "TST-SIB2")
	assert.Contains(t, sibLayer.Content, "state sibling outcome")
}

// TC-004: Tests for RenderAgent and RenderHuman

func TestRenderAgent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
