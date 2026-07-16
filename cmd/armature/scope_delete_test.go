package main

import (
	"bytes"
	"encoding/json"
	"os"
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

 bootstrapRepoForTest(t, repo)

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

	// Materialize so index.json exists with scope data before scope-delete reads it via ReadIndex.
	_, err = runTrls(t, repo, "materialize")
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

// TestScopeDeleteCmd_UsesIndexForScan proves that scope-delete reads from index.json via
// store.ReadIndex() rather than rematerializing from ops via store.Load().
//
// Mechanism: after materializing a real task, inject a fake entry directly into index.json.
// This entry has no create op — Load() would overwrite index.json and lose it; ReadIndex()
// reads the existing file and sees it.
//
// RED with store.Load(): rematerializes from ops → fake entry lost → output lacks
//
//	"task-index-only" → assertion FAILS.
//
// GREEN with store.ReadIndex(): reads existing index.json → sees fake entry → output
//
//	contains "task-index-only" → assertion PASSES.
func TestScopeDeleteCmd_UsesIndexForScan(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

 bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a real task with the scope path we'll delete.
	_, err = runTrls(t, repo, "create", "--id", "task-real", "--title", "Real task", "--type", "task",
		"--scope", "src/old/foo.go")
	require.NoError(t, err)

	// Materialize to write index.json containing task-real.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Inject a fake entry directly into index.json.
	// This entry has no create op — store.Load() rematerializes and loses it;
	// store.ReadIndex() reads the file as-is and sees it.
	stateDir := getTestStateDir(t, repo)
	indexPath := filepath.Join(stateDir, "index.json")

	indexData, readErr := os.ReadFile(indexPath)
	require.NoError(t, readErr)

	var index materialize.Index
	require.NoError(t, json.Unmarshal(indexData, &index))

	index["task-index-only"] = materialize.IndexEntry{
		Status: "open",
		Scope:  []string{"src/old/foo.go"},
	}

	newData, marshalErr := json.Marshal(index)
	require.NoError(t, marshalErr)
	require.NoError(t, os.WriteFile(indexPath, newData, 0o644))

	// Run scope-delete.
	// With store.Load() (old code): rematerializes from ops, overwrites index.json,
	//   loses task-index-only → output lacks it → assertion below FAILS (RED).
	// With store.ReadIndex() (new code): reads existing index.json, sees task-index-only
	//   → output includes it → assertion PASSES (GREEN).
	out, err := runTrls(t, repo, "scope-delete", "src/old/foo.go")
	require.NoError(t, err)

	assert.Contains(t, out, "task-real", "real task must appear in output")
	assert.Contains(t, out, "task-index-only",
		"task-index-only must appear in output, proving store.ReadIndex was used (not store.Load)")
}
