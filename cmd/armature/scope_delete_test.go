package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRepoWithScopedTasksForDelete(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	wellFormed := func(title string, scope []string) ops.Payload {
		return ops.Payload{
			Title:            title,
			NodeType:         "task",
			Scope:            scope,
			DefinitionOfDone: title + " is complete and tested",
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       "draft",
		}
	}
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: wellFormed("Task 1", []string{"src/old/foo.go", "src/old/bar.go"}),
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-02", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: wellFormed("Task 2", []string{"src/old/foo.go", "src/old/keep.go"}),
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-03", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: wellFormed("Task 3", []string{"src/other/qux.go"}),
	}))

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	return repo
}

func TestScopeDeleteCmd_RejectsEmptyPath(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)
	_, err := runTrls(t, repo, "scope-delete", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

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

func TestScopeDeleteCmd_ExactMatchOnlyAffectsMatchingIssues(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	out, err := runTrls(t, repo, "scope-delete", "src/old/foo.go")
	require.NoError(t, err)

	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
	assert.NotContains(t, out, "task-03")
}

func TestScopeDeleteCmd_RematerializesState(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	_, err := runTrls(t, repo, "scope-delete", "src/old/foo.go")
	require.NoError(t, err)

	workerDir := getTestStateDir(t, repo)

	issue01, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-01.json"))
	require.NoError(t, err)
	assert.NotContains(t, issue01.Scope, "src/old/foo.go", "deleted entry should be removed from task-01")
	assert.Contains(t, issue01.Scope, "src/old/bar.go", "non-deleted entry should remain in task-01")

	issue02, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-02.json"))
	require.NoError(t, err)
	assert.NotContains(t, issue02.Scope, "src/old/foo.go")
	assert.Contains(t, issue02.Scope, "src/old/keep.go")

	issue03, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-03.json"))
	require.NoError(t, err)
	assert.Equal(t, []string{"src/other/qux.go"}, issue03.Scope)
}

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

func TestScopeDeleteCmd_EmptyingLastTaskScopeIsRefused(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-last", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{
			Title:            "Last scope",
			NodeType:         "task",
			Scope:            []string{"src/old/foo.go"},
			DefinitionOfDone: "Last scope is complete and tested",
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       "draft",
		},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "scope-delete", "src/old/foo.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
	assert.Contains(t, err.Error(), "task-last")
}

func TestScopeDeleteCmd_HumanOutput(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	out, err := runTrls(t, repo, "scope-delete", "--format", "human", "src/old/foo.go")
	require.NoError(t, err)
	assert.Contains(t, out, "src/old/foo.go")
	assert.NotContains(t, out, `"deleted_path"`, "human format should not be JSON")
}

func TestScopeDeleteCmd_JSONOutput(t *testing.T) {
	repo := setupRepoWithScopedTasksForDelete(t)

	out, err := runTrls(t, repo, "scope-delete", "--format", "json", "src/old/foo.go")
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, "src/old/foo.go", result["deleted_path"])
	assert.EqualValues(t, 2, result["affected_count"])
}

func TestScopeDeleteCmd_UsesIndexForScan(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-real", "--title", "Real task", "--type", "task",
		"--scope", "src/old/foo.go", "--scope", "src/keep/bar.go")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

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

	out, err := runTrls(t, repo, "scope-delete", "src/old/foo.go")
	require.NoError(t, err)

	assert.Contains(t, out, "task-real", "real task must appear in output")
	assert.Contains(t, out, "task-index-only",
		"task-index-only must appear in output, proving store.ReadIndex was used (not store.Load)")
}
