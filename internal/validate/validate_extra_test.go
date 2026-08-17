package validate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_StrictMode_PromotesWarnings(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done", Outcome: "done"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{Strict: true})
	assert.False(t, result.OK, "strict treats warnings as a failed run")
	require.NotEmpty(t, result.Warnings, "strict must keep W-codes in Warnings")
	assert.Empty(t, result.Errors, "strict must not move W-codes into Errors")
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
