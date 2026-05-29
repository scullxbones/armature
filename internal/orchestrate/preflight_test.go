package orchestrate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: build a minimal valid PreflightInput for mutation.
func validPreflightInput(t *testing.T) orchestrate.PreflightInput {
	t.Helper()

	dir := t.TempDir()

	// Create scope paths so they exist on disk.
	scopeFile := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(scopeFile, []byte("package main"), 0o644))

	// Build a manifest with a known source entry.
	manifest := sources.Manifest{
		Entries: map[string]sources.SourceEntry{
			"src-1": {
				ID:          "src-1",
				URL:         "https://example.com/doc",
				Title:       "Example",
				Fingerprint: "abc123",
			},
		},
	}

	return orchestrate.PreflightInput{
		ScopePaths:  []string{scopeFile},
		Acceptance:  json.RawMessage(`["make check green"]`),
		CitationIDs: []string{"src-1"},
		Manifest:    manifest,
		TokenBudget: 1600,
		WorkDir:     dir,
	}
}

// --- RunPreflight: happy path ---

func TestRunPreflight_AllChecksPass(t *testing.T) {
	in := validPreflightInput(t)
	result := orchestrate.RunPreflight(in)
	assert.True(t, result.OK, "all valid inputs should pass: %v", result.Errors)
	assert.Empty(t, result.Errors, "no errors expected")
}

// --- scope path checks ---

func TestRunPreflight_MissingScope(t *testing.T) {
	in := validPreflightInput(t)
	in.ScopePaths = []string{"/nonexistent/path/foo.go"}

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "scope path does not exist")
	assert.Contains(t, result.Errors[0], "/nonexistent/path/foo.go")
}

func TestRunPreflight_EmptyScope(t *testing.T) {
	in := validPreflightInput(t)
	in.ScopePaths = nil

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	assert.NotEmpty(t, result.Errors)
	assert.Contains(t, result.Errors[0], "scope paths")
}

// Multiple missing paths: each should produce its own error entry.
func TestRunPreflight_MultipleMissingScopes(t *testing.T) {
	in := validPreflightInput(t)
	in.ScopePaths = []string{"/missing/a.go", "/missing/b.go"}

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	assert.Len(t, result.Errors, 2)
}

// --- acceptance criteria checks ---

func TestRunPreflight_UnparseableAcceptance(t *testing.T) {
	in := validPreflightInput(t)
	in.Acceptance = json.RawMessage(`not-valid-json`)

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "acceptance criteria")
}

func TestRunPreflight_NoVerifiableAcceptance(t *testing.T) {
	in := validPreflightInput(t)
	in.Acceptance = json.RawMessage(`["The UI looks nice", "Human reviewer approves"]`)

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "acceptance criteria")
}

func TestRunPreflight_EmptyAcceptanceArray(t *testing.T) {
	in := validPreflightInput(t)
	in.Acceptance = json.RawMessage(`[]`)

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	assert.NotEmpty(t, result.Errors)
}

func TestRunPreflight_NilAcceptance(t *testing.T) {
	in := validPreflightInput(t)
	in.Acceptance = nil

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	assert.NotEmpty(t, result.Errors)
}

// --- citation / manifest checks ---

func TestRunPreflight_CitationNotInManifest(t *testing.T) {
	in := validPreflightInput(t)
	in.CitationIDs = []string{"unknown-src-99"}

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "source citation")
	assert.Contains(t, result.Errors[0], "unknown-src-99")
}

func TestRunPreflight_NoCitationsRequired(t *testing.T) {
	in := validPreflightInput(t)
	in.CitationIDs = nil // empty means nothing to verify

	result := orchestrate.RunPreflight(in)
	assert.True(t, result.OK, "no citations required — should pass")
}

func TestRunPreflight_MultipleUnresolvableCitations(t *testing.T) {
	in := validPreflightInput(t)
	in.CitationIDs = []string{"missing-a", "missing-b"}

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	assert.Len(t, result.Errors, 2)
}

// --- token budget ---

func TestRunPreflight_ZeroTokenBudget(t *testing.T) {
	in := validPreflightInput(t)
	in.TokenBudget = 0

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "token budget")
}

func TestRunPreflight_NegativeTokenBudget(t *testing.T) {
	in := validPreflightInput(t)
	in.TokenBudget = -1

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	assert.NotEmpty(t, result.Errors)
}

// --- sandbox ---

func TestRunPreflight_SandboxUnavailable(t *testing.T) {
	in := validPreflightInput(t)
	in.SandboxRequired = true
	in.SandboxOK = false

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "sandbox")
}

func TestRunPreflight_SandboxAvailable(t *testing.T) {
	in := validPreflightInput(t)
	in.SandboxRequired = true
	in.SandboxOK = true

	result := orchestrate.RunPreflight(in)
	// All other inputs are valid; sandbox passes.
	assert.True(t, result.OK, "sandbox available — all checks should pass")
}

func TestRunPreflight_SandboxNotRequired(t *testing.T) {
	in := validPreflightInput(t)
	in.SandboxRequired = false
	in.SandboxOK = false // irrelevant when not required

	result := orchestrate.RunPreflight(in)
	assert.True(t, result.OK, "sandbox not required — unavailability should not fail")
}

func TestRunPreflight_AuthRequiredUnavailable(t *testing.T) {
	in := validPreflightInput(t)
	in.AuthRequired = true
	in.AuthOK = false
	in.AuthError = "missing credentials"

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	assert.Contains(t, result.Error(), "auth: missing credentials")
}

func TestRunPreflight_AuthRequiredAvailable(t *testing.T) {
	in := validPreflightInput(t)
	in.AuthRequired = true
	in.AuthOK = true

	result := orchestrate.RunPreflight(in)
	assert.True(t, result.OK)
}

// --- multiple errors at once ---

func TestRunPreflight_MultipleFailures(t *testing.T) {
	in := validPreflightInput(t)
	in.ScopePaths = []string{"/missing/file.go"}
	in.Acceptance = json.RawMessage(`["no verifiable criterion here"]`)
	in.CitationIDs = []string{"ghost-src"}
	in.TokenBudget = 0
	in.SandboxRequired = true
	in.SandboxOK = false

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	// Expect at least 4 errors: scope, acceptance, citation, token budget, sandbox.
	assert.GreaterOrEqual(t, len(result.Errors), 4)
}

// --- PreflightResult.Error helper ---

func TestPreflightResult_ErrorCollectsAll(t *testing.T) {
	in := validPreflightInput(t)
	in.ScopePaths = []string{"/missing/a.go"}
	in.TokenBudget = 0

	result := orchestrate.RunPreflight(in)
	assert.False(t, result.OK)
	errStr := result.Error()
	assert.Contains(t, errStr, "scope path does not exist")
	assert.Contains(t, errStr, "token budget")
}

func TestPreflightResult_ErrorEmptyWhenOK(t *testing.T) {
	in := validPreflightInput(t)
	result := orchestrate.RunPreflight(in)
	assert.True(t, result.OK)
	assert.Empty(t, result.Error())
}
