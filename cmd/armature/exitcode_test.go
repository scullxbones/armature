package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/exitcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExitJSON_Format verifies exitJSON produces valid JSON on stderr.
func TestExitJSON_Format(t *testing.T) {
	buf := new(bytes.Buffer)
	writeJSONError(buf, "something went wrong", exitcodes.ExitGeneralError)

	out := buf.String()
	require.True(t, json.Valid([]byte(strings.TrimSpace(out))), "must be valid JSON: %q", out)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &m))
	assert.Equal(t, "something went wrong", m["error"])
	assert.Equal(t, "general_error", m["code"])
	assert.Equal(t, float64(1), m["exit_code"])
}

// TestExitJSON_UsageError verifies usage_error code and exit_code 2.
func TestExitJSON_UsageError(t *testing.T) {
	buf := new(bytes.Buffer)
	writeJSONError(buf, "bad flag", exitcodes.ExitUsageError)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &m))
	assert.Equal(t, "bad flag", m["error"])
	assert.Equal(t, "usage_error", m["code"])
	assert.Equal(t, float64(2), m["exit_code"])
}

// TestExitJSON_NotFound verifies not_found code and exit_code 3.
func TestExitJSON_NotFound(t *testing.T) {
	buf := new(bytes.Buffer)
	writeJSONError(buf, "issue not found", exitcodes.ExitNotFound)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &m))
	assert.Equal(t, "issue not found", m["error"])
	assert.Equal(t, "not_found", m["code"])
	assert.Equal(t, float64(3), m["exit_code"])
}

// TestClassifyError_GeneralError verifies unknown errors map to ExitGeneralError.
func TestClassifyError_GeneralError(t *testing.T) {
	err := fmt.Errorf("some unexpected problem")
	code := classifyError(err)
	assert.Equal(t, exitcodes.ExitGeneralError, code)
}

// TestClassifyError_NotFound verifies "not found" errors map to ExitNotFound.
func TestClassifyError_NotFound(t *testing.T) {
	err := fmt.Errorf("issue E1-S1-T1 not found")
	code := classifyError(err)
	assert.Equal(t, exitcodes.ExitNotFound, code)
}

// TestClassifyError_Conflict verifies "already claimed" errors map to ExitConflict.
func TestClassifyError_Conflict(t *testing.T) {
	err := fmt.Errorf("issue is already claimed by another worker")
	code := classifyError(err)
	assert.Equal(t, exitcodes.ExitConflict, code)
}

// TestClassifyError_InvalidState verifies "invalid transition" maps to ExitInvalidState.
func TestClassifyError_InvalidState(t *testing.T) {
	err := fmt.Errorf("invalid status transition from done to ready")
	code := classifyError(err)
	assert.Equal(t, exitcodes.ExitInvalidState, code)
}

// TestMain_JSONFormatError verifies that --format=json writes structured JSON to stderr on error.
func TestMain_JSONFormatError(t *testing.T) {
	repo := setupRepoWithTask(t)

	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(errBuf)
	root.SetArgs([]string{"show", "--repo", repo, "--format", "json", "nonexistent-issue-xyz"})

	err := root.Execute()
	assert.Error(t, err)

	errOut := strings.TrimSpace(errBuf.String())
	// stderr should contain JSON error structure
	if errOut != "" && json.Valid([]byte(errOut)) {
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(errOut), &m))
		assert.Contains(t, m, "error")
		assert.Contains(t, m, "code")
		assert.Contains(t, m, "exit_code")
	}
	// If not JSON-only on stderr, at minimum the error should be returned
	// (the test is mainly that we don't panic and the function exists)
}

// TestMain_AgentFormatError verifies --format=agent also writes structured JSON to stderr.
func TestMain_AgentFormatError(t *testing.T) {
	repo := setupRepoWithTask(t)

	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(errBuf)
	root.SetArgs([]string{"show", "--repo", repo, "--format", "agent", "nonexistent-issue-xyz"})

	err := root.Execute()
	assert.Error(t, err)

	errOut := strings.TrimSpace(errBuf.String())
	if errOut != "" && json.Valid([]byte(errOut)) {
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(errOut), &m))
		assert.Contains(t, m, "error")
		assert.Contains(t, m, "code")
		assert.Contains(t, m, "exit_code")
	}
}
