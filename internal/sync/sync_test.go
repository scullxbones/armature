package sync_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	trellissync "github.com/scullxbones/armature/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMergeChecker struct {
	merged map[string]bool
}

func (f *fakeMergeChecker) BranchMergedInto(branch, target string) (bool, error) {
	return f.merged[branch], nil
}

func TestDetectMerges_ReturnsMergedIssueIDs(t *testing.T) {
	dir := t.TempDir()
	issuesStateDir := filepath.Join(dir, "state", "issues")
	require.NoError(t, os.MkdirAll(issuesStateDir, 0755))

	// done + merged branch
	issue1 := materialize.Issue{
		ID: "T-001", Status: "done", Branch: "feature/merged-work", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	// done + unmerged branch
	issue2 := materialize.Issue{
		ID: "T-002", Status: "done", Branch: "feature/unmerged-work", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	// in-progress — should be skipped regardless of branch status
	issue3 := materialize.Issue{
		ID: "T-003", Status: "in-progress", Branch: "feature/wip", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	require.NoError(t, materialize.WriteIssue(issuesStateDir, issue1))
	require.NoError(t, materialize.WriteIssue(issuesStateDir, issue2))
	require.NoError(t, materialize.WriteIssue(issuesStateDir, issue3))

	mc := &fakeMergeChecker{merged: map[string]bool{
		"feature/merged-work": true,
	}}

	ids, err := trellissync.DetectMerges("unused-issues-dir", filepath.Join(dir, "state"), "main", mc)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"T-001"}, ids)
}

func TestDetectMerges_NoBranch_Skipped(t *testing.T) {
	dir := t.TempDir()
	issuesStateDir := filepath.Join(dir, "state", "issues")
	require.NoError(t, os.MkdirAll(issuesStateDir, 0755))

	issue := materialize.Issue{
		ID: "T-001", Status: "done", Branch: "", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	require.NoError(t, materialize.WriteIssue(issuesStateDir, issue))

	mc := &fakeMergeChecker{merged: map[string]bool{}}

	ids, err := trellissync.DetectMerges("unused-issues-dir", filepath.Join(dir, "state"), "main", mc)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestDetectMerges_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// No state/issues dir — should return nil, nil

	mc := &fakeMergeChecker{merged: map[string]bool{}}
	ids, err := trellissync.DetectMerges("unused-issues-dir", filepath.Join(dir, "state"), "main", mc)
	assert.NoError(t, err)
	assert.Empty(t, ids)
}

func TestSyncDetectMergesUsesStateDir(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "specific-state")
	issuesStateDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesStateDir, 0755))

	issue := materialize.Issue{
		ID: "T-001", Status: "done", Branch: "feature/merged", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	require.NoError(t, materialize.WriteIssue(issuesStateDir, issue))

	mc := &fakeMergeChecker{merged: map[string]bool{"feature/merged": true}}

	// If it was hardcoded to something else, this would fail.
	ids, err := trellissync.DetectMerges("unused", stateDir, "main", mc)
	require.NoError(t, err)
	assert.Equal(t, []string{"T-001"}, ids)
}
