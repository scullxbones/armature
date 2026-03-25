package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/stretchr/testify/assert"
)

func TestIssueSubset_WithScopeID(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "STORY-1", Children: []string{"TASK-1"}, Type: "story"},
		&materialize.Issue{ID: "TASK-1", Parent: "STORY-1", Type: "task"},
		&materialize.Issue{ID: "TASK-2", Type: "task"},
	)

	result := Validate(state, Options{ScopeID: "STORY-1"})
	// Validation runs on the scoped subset; no errors for clean hierarchy
	_ = result
}

func TestIssueSubset_MissingScopeID_ReturnsEmpty(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TASK-1", Type: "task"},
	)
	result := Validate(state, Options{ScopeID: "NONEXISTENT"})
	assert.True(t, result.OK)
}

func TestValidate_StrictMode_PromotesWarnings(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done", Outcome: "done"},
	)
	result := Validate(state, Options{Strict: true})
	assert.False(t, result.OK)
	assert.Nil(t, result.Warnings)
}

func TestValidate_WithIssuesDir_SkipsCitationsWhenNoManifest(t *testing.T) {
	dir := t.TempDir()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task"},
	)
	result := Validate(state, Options{IssuesDir: dir})
	// Citation check skipped when manifest absent — no citation errors
	for _, e := range result.Errors {
		if e == "citation check skipped: cannot read source manifest: open "+filepath.Join(dir, "sources", "manifest.json")+": no such file or directory" {
			t.Errorf("unexpected error: %s", e)
		}
	}
}

func TestValidate_WithIssuesDir_CitationErrors(t *testing.T) {
	dir := t.TempDir()
	sourcesDir := filepath.Join(dir, "sources")
	if err := os.MkdirAll(sourcesDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := map[string]interface{}{
		"entries": map[string]map[string]string{
			"src-1": {"id": "src-1"},
		},
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task"},
	)
	result := Validate(state, Options{IssuesDir: dir})
	// TSK-1 has no source links — should be an uncited node error
	found := false
	for _, e := range result.Errors {
		if containsError(Result{Errors: []string{e}}, "uncited node") {
			found = true
		}
	}
	assert.True(t, found, "expected uncited node error, got: %v", result.Errors)
}

func TestValidate_WithRepoPath_PhantomScope(t *testing.T) {
	dir := t.TempDir()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Scope: []string{"nonexistent/**/*.go"}},
	)
	result := Validate(state, Options{RepoPath: dir})
	found := containsWarning(result, "phantom scope")
	assert.True(t, found, "expected phantom scope warning, got: %v", result.Warnings)
}

func TestValidate_CitationAccepted_SatisfiesCitationRequirement(t *testing.T) {
	dir := t.TempDir()
	sourcesDir := filepath.Join(dir, "sources")
	if err := os.MkdirAll(sourcesDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]interface{}{
		"entries": map[string]map[string]string{
			"src-1": {"id": "src-1"},
		},
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	state := makeState(
		&materialize.Issue{
			ID:   "TSK-1",
			Type: "task",
			CitationAcceptances: []materialize.CitationAcceptance{
				{WorkerID: "worker-1", Timestamp: 1234567890},
			},
		},
	)
	result := Validate(state, Options{IssuesDir: dir})
	assert.False(t, containsError(result, "uncited node"), "expected no uncited node error for accepted citation, got: %v", result.Errors)
}

func TestValidate_CitationAccepted_NoManifest_CitationCheckSkipped(t *testing.T) {
	dir := t.TempDir()
	// No manifest written — citation check should be skipped entirely.
	state := makeState(
		&materialize.Issue{
			ID:   "TSK-1",
			Type: "task",
			CitationAcceptances: []materialize.CitationAcceptance{
				{WorkerID: "worker-1", Timestamp: 1234567890},
			},
		},
	)
	result := Validate(state, Options{IssuesDir: dir})
	assert.False(t, containsError(result, "uncited node"), "expected no citation error when manifest absent, got: %v", result.Errors)
	assert.False(t, containsError(result, "citation check skipped"), "unexpected citation check skipped error: %v", result.Errors)
}

func TestValidate_SourceLinkOnly_ManifestMembershipChecked(t *testing.T) {
	dir := t.TempDir()
	sourcesDir := filepath.Join(dir, "sources")
	if err := os.MkdirAll(sourcesDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]interface{}{
		"entries": map[string]map[string]string{
			"src-1": {"id": "src-1"},
		},
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	state := makeState(
		&materialize.Issue{
			ID:   "TSK-1",
			Type: "task",
			SourceLinks: []materialize.SourceLink{
				{SourceEntryID: "unknown-src"},
			},
		},
	)
	result := Validate(state, Options{IssuesDir: dir})
	assert.True(t, containsError(result, "unknown source"), "expected unknown source error for unregistered source link, got: %v", result.Errors)
}

func TestValidate_WithRepoPath_ExistingScope(t *testing.T) {
	dir := t.TempDir()
	// Create a Go file that matches the glob
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package p"), 0644); err != nil {
		t.Fatal(err)
	}
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Scope: []string{"*.go"}},
	)
	result := Validate(state, Options{RepoPath: dir})
	found := containsWarning(result, "phantom scope")
	assert.False(t, found, "expected no phantom scope warning for existing file, got: %v", result.Warnings)
}
