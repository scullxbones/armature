package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNonInteractiveFlag_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	flag := root.PersistentFlags().Lookup("non-interactive")
	require.NotNil(t, flag, "--non-interactive flag must be registered as a PersistentFlag on root")
	assert.Equal(t, "bool", flag.Value.Type())
}

func TestNonInteractiveFlag_AutoSetByFormatAgent(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"ready", "--repo", repo, "--format", "agent"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.True(t, json.Valid([]byte(out)), "expected valid JSON output when --format=agent, got: %q", out)
}

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

func TestDAGSummaryCmd_NonInteractiveFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "summary", "--repo", repo, "--non-interactive"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.NotEmpty(t, out)
}

func TestDAGSummaryCmd_ApproveAllFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "summary", "--repo", repo, "--non-interactive", "--approve-all"})
	err := root.Execute()
	require.NoError(t, err, "--approve-all should succeed and exit 0")
}

func TestDAGSummaryCmd_ApproveAllFlag_JSON(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "summary", "--repo", repo, "--non-interactive", "--approve-all", "--format", "json"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.True(t, json.Valid([]byte(out)), "expected valid JSON, got: %q", out)
}

func TestStaleReviewCmd_NonInteractiveFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo, "--non-interactive"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.NotEmpty(t, out)
}

func TestStaleReviewCmd_NonInteractiveFlag_EmitsJSON(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"sources", "stale-review", "--repo", repo, "--non-interactive", "--format", "json"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.NotEmpty(t, out)
}

func TestTUICmd_NonInteractiveFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"tui", "--repo", repo, "--non-interactive"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "board:")
}
