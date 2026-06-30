package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	issue := Issue{
		ID:     "task-01",
		Type:   "task",
		Status: "open",
		Title:  "Fix auth",
		Parent: "story-01",
		Scope:  []string{"src/auth/**"},
	}

	require.NoError(t, WriteIssue(issuesDir, issue))

	loaded, err := LoadIssue(filepath.Join(issuesDir, "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, "task-01", loaded.ID)
	assert.Equal(t, "Fix auth", loaded.Title)
}

func TestIndexRoundTrip(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")

	index := Index{
		"task-01": IndexEntry{Status: "open", Type: "task", Title: "Fix auth", Parent: "story-01"},
	}

	require.NoError(t, WriteIndex(indexPath, index))

	loaded, err := LoadIndex(indexPath)
	require.NoError(t, err)
	assert.Equal(t, "open", loaded["task-01"].Status)
}
