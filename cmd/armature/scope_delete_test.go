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

// setupRepoWithScopedTasksForDelete creates a temp repo with tasks for scope-delete tests.
// task-01: scope = ["src/old/foo.go", "src/old/bar.go"]
// task-02: scope = ["src/old/foo.go"]          (exact match for the path we'll delete)
// task-03: scope = ["src/other/qux.go"]        (no match)
// task-04: scope = ["src/old/foo.go"]          (will become empty after delete; non-terminal)
func setupRepoWithScopedTasksForDelete(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd.Execute())

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// task-01: two scope entries, one of which matches
	_, err = runTrls(t, repo, "create", "--id", "task-01", "--title", "Task 1", "--type", "task",
		"--scope", "src/old/foo.go",
		"--scope", "src/old/bar.go")
	require.NoError(t, err)

	// task-02: single matching scope entry
	_, err = runTrls(t, repo, "create", "--id", "task-02", "--title", "Task 2", "--type", "task",
		"--scope", "src/old/foo.go")
	require.NoError(t, err)

	// task-03: no matching scope entry (different path)
	_, err = runTrls(t, repo, "create", "--id", "task-03", "--title", "Task 3", "--type", "task",
		"--scope", "src/other/qux.go")
	require.NoError(t, err)

	return repo
}

// TestScopeDeleteCmd_RejectsEmptyPath verifies that an empty path argument returns an error.
func TestScopeDeleteCmd_RejectsEmptyPath(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)
	_, err := runTrls(t, repo, "scope-delete", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestScopeDeleteCmd_NoMatchWarnsAndExitsZero verifies no-match emits a warning but returns no error.
func TestScopeDeleteCmd_NoMatchWarnsAndExitsZero(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"scope-delete", "src/nonexistent/path.go", "--repo", repo})

	err := root.Execute()
	require.NoError(t, err, "no-match should exit 0")
	assert.Contains(t, errBuf.String(), "no issues")
}

// TestScopeDeleteCmd_ExactMatchOnlyAffectsMatchingIssues verifies only issues with an exact
// scope entry are affected, and substring matches are not removed.
func TestScopeDeleteCmd_ExactMatchOnlyAffectsMatchingIssues(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	out, err := runTrls(t, repo, "scope-delete", "src/old/foo.go")
	require.NoError(t, err)

	// task-01 and task-02 have an exact "src/old/foo.go" entry
	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
	// task-03 only has "src/other/qux.go" — not affected
	assert.NotContains(t, out, "task-03")
}

// TestScopeDeleteCmd_RematerializesState verifies that the materialized issue files are updated.
func TestScopeDeleteCmd_RematerializesState(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	_, err := runTrls(t, repo, "scope-delete", "src/old/foo.go")
	require.NoError(t, err)

	workerDir := getTestStateDir(t, repo)

	// task-01 should still have "src/old/bar.go" but not "src/old/foo.go"
	issue01, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-01.json"))
	require.NoError(t, err)
	assert.NotContains(t, issue01.Scope, "src/old/foo.go", "deleted entry should be removed from task-01")
	assert.Contains(t, issue01.Scope, "src/old/bar.go", "non-deleted entry should remain in task-01")

	// task-02 had only "src/old/foo.go"; scope should now be empty
	issue02, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-02.json"))
	require.NoError(t, err)
	assert.Empty(t, issue02.Scope, "task-02 scope should be empty after deletion")

	// task-03 scope should be unchanged
	issue03, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-03.json"))
	require.NoError(t, err)
	assert.Equal(t, []string{"src/other/qux.go"}, issue03.Scope)
}

// TestScopeDeleteCmd_SameTimestampForAllOps verifies all ops share the same timestamp.
func TestScopeDeleteCmd_SameTimestampForAllOps(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	_, err := runTrls(t, repo, "scope-delete", "src/old/foo.go")
	require.NoError(t, err)

	workerDir := getTestStateDir(t, repo)
	issue01, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-01.json"))
	require.NoError(t, err)
	issue02, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-02.json"))
	require.NoError(t, err)

	assert.Equal(t, issue01.Updated, issue02.Updated,
		"both affected issues should have the same Updated timestamp")
}

// TestScopeDeleteCmd_EmptyScopeWarningForNonTerminal verifies that a warning is emitted to
// stderr when a non-terminal issue ends up with an empty scope after deletion.
func TestScopeDeleteCmd_EmptyScopeWarningForNonTerminal(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	// task-02 has only "src/old/foo.go" and is open (non-terminal), so should warn
	root.SetArgs([]string{"scope-delete", "src/old/foo.go", "--repo", repo})

	err := root.Execute()
	require.NoError(t, err)
	// A warning about the empty scope for task-02 (open, non-terminal) should appear
	assert.Contains(t, errBuf.String(), "empty scope")
	assert.Contains(t, errBuf.String(), "task-02")
}

// TestScopeDeleteCmd_HumanOutput verifies human-readable output format.
func TestScopeDeleteCmd_HumanOutput(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	out, err := runTrls(t, repo, "scope-delete", "--format", "human", "src/old/foo.go")
	require.NoError(t, err)
	assert.Contains(t, out, "src/old/foo.go")
	assert.NotContains(t, out, `"deleted_path"`, "human format should not be JSON")
}

// TestScopeDeleteCmd_JSONOutput verifies JSON output format.
func TestScopeDeleteCmd_JSONOutput(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	out, err := runTrls(t, repo, "scope-delete", "--format", "json", "src/old/foo.go")
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "src/old/foo.go", result["deleted_path"])
	assert.EqualValues(t, 2, result["affected_count"])
}
