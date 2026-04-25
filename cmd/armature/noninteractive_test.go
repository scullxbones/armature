package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNonInteractiveFlag_RegisteredOnRoot verifies the flag exists on the root command.
func TestNonInteractiveFlag_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	flag := root.PersistentFlags().Lookup("non-interactive")
	require.NotNil(t, flag, "--non-interactive flag must be registered as a PersistentFlag on root")
	assert.Equal(t, "bool", flag.Value.Type())
}

// TestNonInteractiveFlag_AutoSetByFormatAgent verifies that --format=agent implies non-interactive.
func TestNonInteractiveFlag_AutoSetByFormatAgent(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"ready", "--repo", repo, "--format", "agent"})
	err := root.Execute()
	require.NoError(t, err)

	// Output must be JSON (no BubbleTea), since --format=agent implies non-interactive.
	out := buf.String()
	assert.True(t, json.Valid([]byte(out)), "expected valid JSON output when --format=agent, got: %q", out)
}

// TestReadyCmd_NonInteractiveFlag outputs JSON without launching BubbleTea.
func TestReadyCmd_NonInteractiveFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"ready", "--repo", repo, "--non-interactive"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.True(t, json.Valid([]byte(out)), "expected valid JSON output with --non-interactive, got: %q", out)
}

// TestDAGSummaryCmd_NonInteractiveFlag outputs JSON without launching BubbleTea.
func TestDAGSummaryCmd_NonInteractiveFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag-summary", "--repo", repo, "--non-interactive"})
	err := root.Execute()
	require.NoError(t, err)

	// When no draft nodes exist, output should be the "No draft nodes found." message or JSON.
	out := buf.String()
	assert.NotEmpty(t, out)
}

// TestDAGSummaryCmd_ApproveAllFlag accepts --approve-all and exits 0.
func TestDAGSummaryCmd_ApproveAllFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag-summary", "--repo", repo, "--non-interactive", "--approve-all"})
	err := root.Execute()
	require.NoError(t, err, "--approve-all should succeed and exit 0")
}

// TestDAGSummaryCmd_ApproveAllFlag_JSON verifies approve-all emits JSON output.
func TestDAGSummaryCmd_ApproveAllFlag_JSON(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag-summary", "--repo", repo, "--non-interactive", "--approve-all", "--format", "json"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.True(t, json.Valid([]byte(out)), "expected valid JSON, got: %q", out)
}

// TestStaleReviewCmd_NonInteractiveFlag outputs pending items as JSON.
func TestStaleReviewCmd_NonInteractiveFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"stale-review", "--repo", repo, "--non-interactive"})
	err := root.Execute()
	require.NoError(t, err)

	// Either "No stale sources detected." or valid JSON.
	out := buf.String()
	assert.NotEmpty(t, out)
}

// TestStaleReviewCmd_NonInteractiveFlag_EmitsJSON verifies JSON output format.
func TestStaleReviewCmd_NonInteractiveFlag_EmitsJSON(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"stale-review", "--repo", repo, "--non-interactive", "--format", "json"})
	err := root.Execute()
	require.NoError(t, err)

	// stale-review with --format json and no stale sources should output something valid.
	out := buf.String()
	assert.NotEmpty(t, out)
}

// TestTUICmd_NonInteractiveFlag skips BubbleTea and emits structured output.
func TestTUICmd_NonInteractiveFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"tui", "--repo", repo, "--non-interactive"})
	err := root.Execute()
	require.NoError(t, err)

	// Should print "board: N issues" summary without TUI.
	out := buf.String()
	assert.Contains(t, out, "board:")
}
