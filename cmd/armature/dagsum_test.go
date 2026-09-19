package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/traceability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An unlinked draft must still be presented as uncited in dag summary, so the
// TUI keeps its type-the-ID acknowledgment (CITEGATE-T2 / ADR 0021).
func TestUncitedLookup_IncludesDraftUncited_REQ_CITEGATE_T2(t *testing.T) {
	t.Parallel()
	cov := traceability.Coverage{
		Uncited:      []string{"VER-1"},
		DraftUncited: []string{"DRAFT-1"},
	}

	lookup := uncitedLookup(cov)

	assert.Contains(t, lookup, "VER-1")
	assert.Contains(t, lookup, "DRAFT-1")
	assert.NotContains(t, lookup, "DRAFT-2")
}

func setupRepoWithDraftNode(t *testing.T) string {
	t.Helper()
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "create",
		"--title", "Draft task",
		"--type", "task",
		"--id", "draft-task-01",
		"--scope", "cmd/armature/draft.go",
		"--dod", "Draft task is complete and tested",
		"--acceptance", `[{"type":"test_passes"}]`,
	)
	require.NoError(t, err)
	return repo
}

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

func TestDAGSummaryCmd_IssueFlag_WithDraftSubtree(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "create",
		"--title", "Draft epic",
		"--type", "epic",
		"--id", "epic-draft-01",
	)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create",
		"--title", "Draft subtask",
		"--type", "task",
		"--id", "task-draft-sub-01",
		"--parent", "epic-draft-01",
	)
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "summary", "--repo", repo, "--format", "json", "--issue", "epic-draft-01"})
	require.NoError(t, root.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	count, ok := result["count"].(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, count, float64(1))
}

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
