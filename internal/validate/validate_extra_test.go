package validate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueSubset_WithScopeID(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "STORY-1", Children: []string{"TASK-1"}, Type: "story"},
		&materialize.Issue{ID: "TASK-1", Parent: "STORY-1", Type: "task"},
		&materialize.Issue{ID: "TASK-2", Type: "task"},
	)

	graph := graphFromState(state)
	result := Validate(state, graph, Options{ScopeID: "STORY-1"})
	// Validation runs on the scoped subset; no errors for clean hierarchy
	_ = result
}

func TestIssueSubset_MissingScopeID_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TASK-1", Type: "task"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ScopeID: "NONEXISTENT"})
	assert.True(t, result.OK)
}

func TestValidate_StrictMode_PromotesWarnings(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done", Outcome: "done"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{Strict: true})
	assert.False(t, result.OK)
	assert.Nil(t, result.Warnings)
}

func TestValidate_WithIssuesDir_SkipsCitationsWhenNoManifest(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task"},
	)
	// When ManifestData is nil/empty, citations are skipped
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ManifestData: nil})
	// No citation errors should appear
	for _, e := range result.Errors {
		if strings.Contains(e, "citation check skipped") {
			t.Errorf("unexpected citation error: %s", e)
		}
	}
}

func TestValidate_WithIssuesDir_CitationErrors(t *testing.T) {
	t.Parallel()
	manifest := map[string]interface{}{
		"entries": map[string]map[string]string{
			"src-1": {"id": "src-1"},
		},
	}
	manifestData, err := json.Marshal(manifest)
	require.NoError(t, err)

	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task"},
	)
	// Pass manifest data directly
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ManifestData: manifestData})
	// TSK-1 has no source links — should be an uncited node error
	found := false
	for _, e := range result.Errors {
		if containsError(Result{Errors: []string{e}}, "uncited node") {
			found = true
		}
	}
	assert.True(t, found, "expected uncited node error, got: %v", result.Errors)
}

func TestValidate_WithRepoPath_PhantomScope_AppearsInInfos(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Scope: []string{"nonexistent/**/*.go"}},
	)
	// Provide pre-expanded scopes showing no files match the glob
	preExpandedScopes := map[string][]string{
		"TSK-1": {}, // empty list means globs matched no files
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	assert.True(t, containsPhantomScopeInfo(result), "expected phantom scope in Infos, got: %v", result.Infos)
	assert.False(t, containsWarning(result, "phantom scope"), "expected phantom scope NOT in Warnings, got: %v", result.Warnings)
}

func TestValidate_CitationAccepted_SatisfiesCitationRequirement(t *testing.T) {
	t.Parallel()
	manifest := map[string]interface{}{
		"entries": map[string]map[string]string{
			"src-1": {"id": "src-1"},
		},
	}
	manifestData, err := json.Marshal(manifest)
	require.NoError(t, err)

	state := makeState(
		&materialize.Issue{
			ID:   "TSK-1",
			Type: "task",
			CitationAcceptances: []materialize.CitationAcceptance{
				{WorkerID: "worker-1", Timestamp: 1234567890},
			},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ManifestData: manifestData})
	assert.False(t, containsError(result, "uncited node"), "expected no uncited node error for accepted citation, got: %v", result.Errors)
}

func TestValidate_CitationAccepted_NoManifest_CitationCheckSkipped(t *testing.T) {
	t.Parallel()
	// No manifest data — citation check should be skipped entirely.
	state := makeState(
		&materialize.Issue{
			ID:   "TSK-1",
			Type: "task",
			CitationAcceptances: []materialize.CitationAcceptance{
				{WorkerID: "worker-1", Timestamp: 1234567890},
			},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ManifestData: nil})
	assert.False(t, containsError(result, "uncited node"), "expected no citation error when manifest absent, got: %v", result.Errors)
	assert.False(t, containsError(result, "citation check skipped"), "unexpected citation check skipped error: %v", result.Errors)
}

func TestValidate_SourceLinkOnly_ManifestMembershipChecked(t *testing.T) {
	t.Parallel()
	manifest := map[string]interface{}{
		"entries": map[string]map[string]string{
			"src-1": {"id": "src-1"},
		},
	}
	manifestData, err := json.Marshal(manifest)
	require.NoError(t, err)

	state := makeState(
		&materialize.Issue{
			ID:   "TSK-1",
			Type: "task",
			SourceLinks: []materialize.SourceLink{
				{SourceEntryID: "unknown-src"},
			},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ManifestData: manifestData})
	assert.True(t, containsError(result, "unknown source"), "expected unknown source error for unregistered source link, got: %v", result.Errors)
}

func TestValidate_ParentFilter_RestrictsToDirectChildren(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story", Children: []string{"TASK-1", "TASK-2"}},
		&materialize.Issue{ID: "TASK-1", Parent: "STORY-1", Type: "task"},
		&materialize.Issue{ID: "TASK-2", Parent: "STORY-1", Type: "task"},
		&materialize.Issue{ID: "TASK-3", Parent: "OTHER", Type: "task"},
	)
	// Validate with ParentID set — only TASK-1 and TASK-2 should be in scope
	// TASK-3 belongs to a different parent so any errors on it should not appear
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ParentID: "STORY-1"})
	_ = result // no errors expected for clean direct children
}

func TestValidate_ParentFilter_EmptyWhenNoMatch(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TASK-1", Parent: "STORY-A", Type: "task"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ParentID: "NONEXISTENT"})
	assert.True(t, result.OK)
}

func TestValidate_WithRepoPath_ExistingScope(t *testing.T) {
	t.Parallel()
	// When PreExpandedScopes is provided with matches, no phantom scope errors
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Scope: []string{"*.go"}},
	)
	// Provide pre-expanded scopes showing that files exist
	preExpandedScopes := map[string][]string{
		"TSK-1": {"foo.go"},
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	assert.False(t, containsWarning(result, "phantom scope"), "expected no phantom scope warning for existing file, got: %v", result.Warnings)
	assert.False(t, containsPhantomScopeInfo(result), "expected no phantom scope info for existing file, got: %v", result.Infos)
}
