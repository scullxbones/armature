package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRepoWithDraftNode creates a bootstrapped repo with a draft-confidence task.
func setupRepoWithDraftNode(t *testing.T) string {
	t.Helper()
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "create",
		"--title", "Draft task",
		"--type", "task",
		"--id", "draft-task-01",
		"--confidence", "draft",
		"--scope", "cmd/armature/draft.go",
		"--dod", "Draft task is complete and tested",
		"--acceptance", `[{"type":"test_passes"}]`,
	)
	require.NoError(t, err)
	return repo
}

// TestDAGSummaryCmd_WithDraftNodes_EmitsJSON verifies JSON output when draft nodes exist.
func TestDAGSummaryCmd_WithDraftNodes_EmitsJSON(t *testing.T) {
	repo := setupRepoWithDraftNode(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "summary", "--repo", repo, "--format", "json"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	count, ok := result["count"].(float64)
	require.True(t, ok, "expected count field")
	assert.Equal(t, float64(1), count, "expected one draft node")
}

// TestDAGSummaryCmd_ApproveAll_WithDraftNodes emits ops and returns JSON.
func TestDAGSummaryCmd_ApproveAll_WithDraftNodes(t *testing.T) {
	repo := setupRepoWithDraftNode(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "summary", "--repo", repo, "--format", "json", "--approve-all"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, float64(1), result["count"])
	assert.Equal(t, true, result["approve_all"])
}

// TestDAGSummaryCmd_IssueFlag_WithDraftSubtree uses --issue to limit to a subtree.
func TestDAGSummaryCmd_IssueFlag_WithDraftSubtree(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create an epic that is draft, with a draft child task.
	_, err := runTrls(t, repo, "create",
		"--title", "Draft epic",
		"--type", "epic",
		"--id", "epic-draft-01",
		"--confidence", "draft",
	)
	require.NoError(t, err)
	// Materialize so issues/epic-draft-01.json exists for ReadIssue in create --parent.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create",
		"--title", "Draft subtask",
		"--type", "task",
		"--id", "task-draft-sub-01",
		"--parent", "epic-draft-01",
		"--confidence", "draft",
	)
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "summary", "--repo", repo, "--format", "json", "--issue", "epic-draft-01"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	// Should include at least the draft nodes in the subtree.
	count, ok := result["count"].(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, count, float64(1))
}

// TestDAGSummaryCmd_IssueFlag_UnknownID returns zero draft items when the ID is unknown.
func TestDAGSummaryCmd_IssueFlag_UnknownID(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "summary", "--repo", repo, "--format", "json", "--issue", "nonexistent-id"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	count, ok := result["count"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(0), count)
}
