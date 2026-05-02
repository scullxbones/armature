package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRepoWithScopedTasks creates a temp repo with two tasks that have scope entries.
func setupRepoWithScopedTasks(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// task-01: scope includes old prefix
	_, err = runTrls(t, repo, "create", "--id", "task-01", "--title", "Task 1", "--type", "task",
		"--scope", "src/old/foo.go",
		"--scope", "src/old/bar.go")
	require.NoError(t, err)

	// task-02: scope includes old prefix (different entry)
	_, err = runTrls(t, repo, "create", "--id", "task-02", "--title", "Task 2", "--type", "task",
		"--scope", "src/old/baz.go")
	require.NoError(t, err)

	// task-03: scope does NOT include old prefix
	_, err = runTrls(t, repo, "create", "--id", "task-03", "--title", "Task 3", "--type", "task",
		"--scope", "src/other/qux.go")
	require.NoError(t, err)

	return repo
}

// TestScopeRenameCmd_RejectsEmptyOldPath verifies that an empty old-path returns an error.
func TestScopeRenameCmd_RejectsEmptyOldPath(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)
	_, err := runTrls(t, repo, "scope-rename", "", "src/new")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestScopeRenameCmd_RejectsEmptyNewPath verifies that an empty new-path returns an error.
func TestScopeRenameCmd_RejectsEmptyNewPath(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)
	_, err := runTrls(t, repo, "scope-rename", "src/old", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestScopeRenameCmd_RejectsEqualArgs verifies that identical old-path and new-path return an error.
func TestScopeRenameCmd_RejectsEqualArgs(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)
	_, err := runTrls(t, repo, "scope-rename", "src/old", "src/old")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identical")
}

// TestScopeRenameCmd_NoMatchWarnsAndExitsZero verifies no-match emits a warning but returns no error.
func TestScopeRenameCmd_NoMatchWarnsAndExitsZero(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"scope-rename", "src/nonexistent", "src/new", "--repo", repo})

	err := root.Execute()
	require.NoError(t, err, "no-match should exit 0")
	assert.Contains(t, errBuf.String(), "no issues")
}

// TestScopeRenameCmd_SubstringMatchAffectsCorrectIssues verifies only issues with matching scope are updated.
func TestScopeRenameCmd_SubstringMatchAffectsCorrectIssues(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	out, err := runTrls(t, repo, "scope-rename", "src/old", "src/new")
	require.NoError(t, err)

	// task-01 and task-02 should be affected; task-03 should not
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
	assert.NotContains(t, out, "task-03")
}

// TestScopeRenameCmd_RematerializesState verifies that the materialized issue files are updated.
func TestScopeRenameCmd_RematerializesState(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	_, err := runTrls(t, repo, "scope-rename", "src/old", "src/new")
	require.NoError(t, err)

	// Load materialized state and verify scope was updated
	workerDir := getTestStateDir(t, repo)
	issue01, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-01.json"))
	require.NoError(t, err)

	for _, entry := range issue01.Scope {
		assert.False(t, strings.Contains(entry, "src/old"),
			"task-01 scope entry %q should not contain old path after rename", entry)
		assert.True(t, strings.Contains(entry, "src/new"),
			"task-01 scope entry %q should contain new path after rename", entry)
	}

	issue03, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-03.json"))
	require.NoError(t, err)
	// task-03 scope should be unchanged
	assert.Equal(t, []string{"src/other/qux.go"}, issue03.Scope)
}

// TestScopeRenameCmd_SameTimestampForAllOps verifies one op per affected issue, all at the same timestamp.
func TestScopeRenameCmd_SameTimestampForAllOps(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	_, err := runTrls(t, repo, "scope-rename", "src/old", "src/new")
	require.NoError(t, err)

	// Load materialized issues and verify their Updated timestamps are equal.
	workerDir := getTestStateDir(t, repo)
	issue01, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-01.json"))
	require.NoError(t, err)
	issue02, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-02.json"))
	require.NoError(t, err)

	assert.Equal(t, issue01.Updated, issue02.Updated,
		"both affected issues should have the same Updated timestamp")
}

// TestScopeRenameCmd_HumanOutput verifies human-readable output format.
func TestScopeRenameCmd_HumanOutput(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	out, err := runTrls(t, repo, "scope-rename", "--format", "human", "src/old", "src/new")
	require.NoError(t, err)
	assert.Contains(t, out, "src/old")
	assert.Contains(t, out, "src/new")
	assert.NotContains(t, out, `"old_path"`, "human format should not be JSON")
}

// TestScopeRenameCmd_JSONOutput verifies JSON output format.
func TestScopeRenameCmd_JSONOutput(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	out, err := runTrls(t, repo, "scope-rename", "--format", "json", "src/old", "src/new")
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "src/old", result["old_path"])
	assert.Equal(t, "src/new", result["new_path"])
	assert.EqualValues(t, 2, result["affected_count"])
}
