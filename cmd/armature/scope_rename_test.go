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

func setupRepoWithScopedTasks(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-01", "--title", "Task 1", "--type", "task",
		"--scope", "src/old/foo.go",
		"--scope", "src/old/bar.go")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-02", "--title", "Task 2", "--type", "task",
		"--scope", "src/old/baz.go")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-03", "--title", "Task 3", "--type", "task",
		"--scope", "src/other/qux.go")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	return repo
}

func TestScopeRenameCmd_RejectsEmptyOldPath(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)
	_, err := runTrls(t, repo, "scope-rename", "", "src/new")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestScopeRenameCmd_RejectsEmptyNewPath(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)
	_, err := runTrls(t, repo, "scope-rename", "src/old", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestScopeRenameCmd_RejectsEqualArgs(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)
	_, err := runTrls(t, repo, "scope-rename", "src/old", "src/old")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identical")
}

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

func TestScopeRenameCmd_SubstringMatchAffectsCorrectIssues(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	out, err := runTrls(t, repo, "scope-rename", "src/old", "src/new")
	require.NoError(t, err)

	assert.Contains(t, out, "task-01")
	assert.Contains(t, out, "task-02")
	assert.NotContains(t, out, "task-03")
}

func TestScopeRenameCmd_RematerializesState(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	_, err := runTrls(t, repo, "scope-rename", "src/old", "src/new")
	require.NoError(t, err)

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
	assert.Equal(t, []string{"src/other/qux.go"}, issue03.Scope)
}

func TestScopeRenameCmd_SameTimestampForAllOps(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	_, err := runTrls(t, repo, "scope-rename", "src/old", "src/new")
	require.NoError(t, err)

	workerDir := getTestStateDir(t, repo)
	issue01, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-01.json"))
	require.NoError(t, err)
	issue02, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-02.json"))
	require.NoError(t, err)

	assert.Equal(t, issue01.Updated, issue02.Updated,
		"both affected issues should have the same Updated timestamp")
}

func TestScopeRenameCmd_RefusesBatchWhenAnyRenameIntroducesFinding(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	ts := nowEpoch()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{
			Type: ops.OpCreate, TargetID: "task-01", Timestamp: ts, WorkerID: workerID,
			Payload: ops.Payload{
				Title: "Already broad", NodeType: "task",
				Scope:            []string{"**/*", "src/old/foo.go"},
				DefinitionOfDone: "Already broad is complete and tested",
				Acceptance:       json.RawMessage(testAcceptance),
				Confidence:       "draft",
			},
		},
		{
			Type: ops.OpCreate, TargetID: "task-02", Timestamp: ts, WorkerID: workerID,
			Payload: ops.Payload{
				Title: "Narrow", NodeType: "task",
				Scope:            []string{"src/old"},
				DefinitionOfDone: "Narrow is complete and tested",
				Acceptance:       json.RawMessage(testAcceptance),
				Confidence:       "draft",
			},
		},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "scope-rename", "src/old", "**")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Graph Finding")

	workerDir := getTestStateDir(t, repo)
	issue01, err := materialize.LoadIssue(filepath.Join(workerDir, "issues", "task-01.json"))
	require.NoError(t, err)
	assert.Contains(t, issue01.Scope, "src/old/foo.go", "batch refusal must not land a prefix of the rename")
}

func TestScopeRenameCmd_HumanOutput(t *testing.T) {
	repo := setupRepoWithScopedTasks(t)

	out, err := runTrls(t, repo, "scope-rename", "--format", "human", "src/old", "src/new")
	require.NoError(t, err)
	assert.Contains(t, out, "src/old")
	assert.Contains(t, out, "src/new")
	assert.NotContains(t, out, `"old_path"`, "human format should not be JSON")
}

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

func TestScopeRenameCmd_UsesIndexForScan(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "task-real", "--title", "Real task", "--type", "task",
		"--scope", "src/old/real.go")
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
		Scope:  []string{"src/old/index.go"},
	}

	newData, marshalErr := json.Marshal(index)
	require.NoError(t, marshalErr)
	require.NoError(t, os.WriteFile(indexPath, newData, 0o644))

	out, err := runTrls(t, repo, "scope-rename", "src/old", "src/new")
	require.NoError(t, err)

	assert.Contains(t, out, "task-real", "real task must appear in output")
	assert.Contains(t, out, "task-index-only",
		"task-index-only must appear in output, proving store.ReadIndex was used (not store.Load)")
}
