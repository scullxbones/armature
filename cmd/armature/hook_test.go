package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHookRunUnknown(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "hook", "run", "unknown-hook")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown hook")
}

func TestHookRunMissingArg(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "hook", "run")
	assert.Error(t, err)
}

func TestHookRunPostMerge(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "hook", "run", "post-merge")
	require.NoError(t, err)
	assert.Contains(t, out, "No merged branches detected")
}

func TestHookRunPostCommit_NoActiveClaim(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	_ = out
}

func TestHookRunPostCommit_WithActiveClaim(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

func TestHookRunPostCommit_SkipsOpsWorktree_REQ_HKDLG_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "skip probe", "--id", "task-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)

	opsWT := filepath.Join(repo, ".armature")
	require.DirExists(t, opsWT)
	branch := strings.TrimSpace(runOutput(t, opsWT, "rev-parse", "--abbrev-ref", "HEAD"))
	require.Equal(t, "_armature", branch)

	ctx := getTestContext(t, repo)
	_, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	before, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	heartbeatsBefore := 0
	for _, op := range before {
		if op.Type == ops.OpHeartbeat && op.TargetID == "task-01" {
			heartbeatsBefore++
		}
	}

	out, err := runTrls(t, opsWT, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.NotContains(t, out, "Heartbeat recorded")

	logged, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	heartbeatsAfter := 0
	for _, op := range logged {
		if op.Type == ops.OpHeartbeat && op.TargetID == "task-01" {
			heartbeatsAfter++
		}
	}
	assert.Equal(t, heartbeatsBefore, heartbeatsAfter, "ops-worktree post-commit must not record a heartbeat")
}

func TestHookRunPreCommit_SingleBranch(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	require.NoError(t, err)
}

func TestHookSubcommandHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"hook", "--help"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "hook")
}

func TestHookPostCommit_InitialCommit(t *testing.T) {
	repo := initTempRepo(t)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "create", "--title", "Scoped task", "--type", "task", "--id", "task-scope-01", "--scope", "src/foo.go")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-scope-01", "--worktree")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
}

func TestHookPostCommit_ScopeRename(t *testing.T) {
	repo := setupRepoWithScopedTask(t, "task-rename-01", "src/old.go")

	writeFile(t, repo, "src/old.go", "package old")
	run(t, repo, "git", "add", "src/old.go")
	run(t, repo, "git", "commit", "-m", "add src/old.go")

	run(t, repo, "git", "mv", "src/old.go", "src/new.go")
	run(t, repo, "git", "commit", "-m", "rename src/old.go -> src/new.go")

	_, err := runTrls(t, repo, "claim", "task-rename-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "scope-rename")
	assert.Contains(t, out, "task-rename-01")
}

func TestHookPostCommit_ScopeDelete(t *testing.T) {
	repo := setupRepoWithScopedTask(t, "task-delete-01", "src/gone.go")
	_, err := runTrls(t, repo, "amend", "task-delete-01", "--scope", "src/gone.go", "--scope", "src/keep.go")
	require.NoError(t, err)

	writeFile(t, repo, "src/gone.go", "package gone")
	run(t, repo, "git", "add", "src/gone.go")
	run(t, repo, "git", "commit", "-m", "add src/gone.go")

	run(t, repo, "git", "rm", "src/gone.go")
	run(t, repo, "git", "commit", "-m", "delete src/gone.go")

	_, err = runTrls(t, repo, "claim", "task-delete-01", "--worktree")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "scope-delete")
	assert.Contains(t, out, "task-delete-01")
}

func TestHookRunPreCommit_NoStagedFiles(t *testing.T) {
	repo := setupRepoWithTask(t)

	run(t, repo, "git", "reset", "HEAD")

	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	require.NoError(t, err)
}

func TestHookRunPreCommit_StagedNonOpsFile(t *testing.T) {
	repo := setupRepoWithTask(t)

	writeFile(t, repo, "src/main.go", "package main")
	run(t, repo, "git", "add", filepath.Join("src", "main.go"))

	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	require.NoError(t, err)
}

func TestHookRunPreCommit_BlocksStagedOpsFile(t *testing.T) {
	repo := setupRepoWithTask(t)

	writeFile(t, repo, ".armature/ops/test.log", "test ops content")
	run(t, repo, "git", "add", filepath.Join(".armature", "ops", "test.log"))

	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to commit .armature/ops/")
}

func TestHookRunPreCommit_LinkedWorktreeStagedOpsUsesInvokingIndex_REQ_HKDLG(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "worktree ops guard", "--id", "task-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "task-01")
	require.DirExists(t, wt)
	parentCached := strings.TrimSpace(runOutput(t, repo, "diff", "--cached", "--name-only"))
	require.Empty(t, parentCached, "parent checkout must be clean so a RepoPath-based guard would miss the worktree index")

	probeRel := filepath.Join("leaked", ".armature", "ops", "probe.log")
	writeFile(t, wt, probeRel, "ops must not land on a claimed worktree branch\n")
	run(t, wt, "git", "add", "--force", probeRel)

	_, err = runTrls(t, wt, "hook", "run", "pre-commit")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to commit .armature/ops/")
}

func TestHookFindActiveClaimID_UsesLatestHeartbeat(t *testing.T) {
	repo := setupRepoWithTask(t)

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)

	issuesDir := filepath.Join(repo, ".armature")
	logPath := fmt.Sprintf("%s/ops/%s.log", issuesDir, workerIdentityWithSlot(workerID))
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))

	now := time.Now().Unix()
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: now - 30,
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 60},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpHeartbeat,
		TargetID:  "task-01",
		Timestamp: now - 20,
		WorkerID:  workerID,
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpHeartbeat,
		TargetID:  "task-01",
		Timestamp: now - 10,
		WorkerID:  workerID,
	}))

	ctx := &config.Context{
		RepoPath:  repo,
		IssuesDir: issuesDir,
		Config:    config.Config{DefaultTTL: 60},
	}

	assert.Equal(t, "task-01", hookFindActiveClaimID(ctx))
}

func TestHookFindActiveClaimID_IgnoresDoneTransitions(t *testing.T) {
	repo := setupRepoWithTask(t)

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)

	issuesDir := filepath.Join(repo, ".armature")
	logPath := fmt.Sprintf("%s/ops/%s.log", issuesDir, workerIdentityWithSlot(workerID))
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))

	now := time.Now().Unix()
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: now - 30,
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 60},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpTransition,
		TargetID:  "task-01",
		Timestamp: now - 5,
		WorkerID:  workerID,
		Payload:   ops.Payload{To: ops.StatusDone},
	}))

	ctx := &config.Context{
		RepoPath:  repo,
		IssuesDir: issuesDir,
		Config:    config.Config{DefaultTTL: 60},
	}

	assert.Empty(t, hookFindActiveClaimID(ctx))
}

func TestHookDetectScopeChanges_WithExistingCheckpoint(t *testing.T) {
	repo := setupRepoWithScopedTask(t, "task-checkpoint-scope", "src/checkpoint.go")

	_, err := runTrls(t, repo, "claim", "task-checkpoint-scope", "--worktree")
	require.NoError(t, err)

	writeFile(t, repo, "src/checkpoint.go", "package checkpoint")
	run(t, repo, "git", "add", "src/checkpoint.go")
	run(t, repo, "git", "commit", "-m", "add checkpoint.go")
	run(t, repo, "git", "mv", "src/checkpoint.go", "src/checkpoint-renamed.go")
	run(t, repo, "git", "commit", "-m", "rename checkpoint.go")

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
		Scope:  []string{"src/checkpoint.go"},
	}
	newData, marshalErr := json.Marshal(index)
	require.NoError(t, marshalErr)
	require.NoError(t, os.WriteFile(indexPath, newData, 0o600))

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "scope-rename")
	assert.Contains(t, out, "task-checkpoint-scope")
	assert.Contains(t, out, "task-index-only",
		"task-index-only must appear in output, proving store.ReadIndex was used (not store.Load)")
}

func commitNoHooks(t *testing.T, repo, msg string) {
	t.Helper()
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "commit", "-m", msg)
}

func readScopeDriftOps(t *testing.T, repo string) (renames, deletes []ops.Op) {
	t.Helper()
	ctx := getTestContext(t, repo)
	_, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	logged, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	for _, op := range logged {
		switch op.Type {
		case ops.OpScopeRename:
			renames = append(renames, op)
		case ops.OpScopeDelete:
			deletes = append(deletes, op)
		}
	}
	return renames, deletes
}

func TestScopeDriftDetectionEmitsRenameOp_REQ_HKDLG_T2(t *testing.T) {
	const (
		taskID  = "task-drift-rename-01"
		oldPath = "src/scoped.go"
		newPath = "src/scoped-renamed.go"
	)
	repo := setupRepoWithScopedTask(t, taskID, oldPath)
	run(t, repo, "git", "config", "diff.renames", "false")

	writeFile(t, repo, oldPath, "package scoped")
	run(t, repo, "git", "add", oldPath)
	commitNoHooks(t, repo, "add scoped.go")

	_, err := runTrls(t, repo, "claim", taskID, "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	run(t, repo, "git", "mv", oldPath, newPath)
	commitNoHooks(t, repo, "rename scoped.go")

	renamesBefore, deletesBefore := readScopeDriftOps(t, repo)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "scope-rename")
	assert.Contains(t, out, taskID)
	assert.NotContains(t, out, "scope-delete")

	renamesAfter, deletesAfter := readScopeDriftOps(t, repo)
	require.Equal(t, len(renamesBefore)+1, len(renamesAfter), "expected exactly one new scope-rename op")
	assert.Len(t, deletesAfter, len(deletesBefore), "git mv must not emit scope-delete")

	got := renamesAfter[len(renamesAfter)-1]
	assert.Equal(t, taskID, got.TargetID)
	assert.Equal(t, oldPath, got.Payload.OldPath)
	assert.Equal(t, newPath, got.Payload.NewPath)
}

func TestScopeDriftDetectionEmitsDeleteOp_REQ_HKDLG_T2(t *testing.T) {
	const (
		taskID = "task-drift-delete-01"
		path   = "src/gone.go"
	)
	repo := setupRepoWithScopedTask(t, taskID, path)
	_, err := runTrls(t, repo, "amend", taskID, "--scope", path, "--scope", "src/keep.go")
	require.NoError(t, err)

	writeFile(t, repo, path, "package gone")
	run(t, repo, "git", "add", path)
	commitNoHooks(t, repo, "add gone.go")

	_, err = runTrls(t, repo, "claim", taskID, "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	run(t, repo, "git", "rm", path)
	commitNoHooks(t, repo, "delete gone.go")

	renamesBefore, deletesBefore := readScopeDriftOps(t, repo)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "scope-delete")
	assert.Contains(t, out, taskID)
	assert.NotContains(t, out, "scope-rename")

	renamesAfter, deletesAfter := readScopeDriftOps(t, repo)
	require.Equal(t, len(deletesBefore)+1, len(deletesAfter), "expected exactly one new scope-delete op")
	assert.Len(t, renamesAfter, len(renamesBefore), "git rm must not emit scope-rename")

	got := deletesAfter[len(deletesAfter)-1]
	assert.Equal(t, taskID, got.TargetID)
	assert.Equal(t, path, got.Payload.DeletedPath)
}

func TestScopeDriftDetectionIgnoresUnscopedPaths_REQ_HKDLG_T2(t *testing.T) {
	const taskID = "task-drift-unscoped-01"
	repo := setupRepoWithScopedTask(t, taskID, "src/scoped.go")

	writeFile(t, repo, "unscoped.txt", "unscoped")
	run(t, repo, "git", "add", "unscoped.txt")
	commitNoHooks(t, repo, "add unscoped.txt")

	writeFile(t, repo, "unscoped-rm.txt", "also unscoped")
	run(t, repo, "git", "add", "unscoped-rm.txt")
	commitNoHooks(t, repo, "add unscoped-rm.txt")

	_, err := runTrls(t, repo, "claim", taskID, "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	run(t, repo, "git", "mv", "unscoped.txt", "unscoped-moved.txt")
	commitNoHooks(t, repo, "rename unscoped.txt")

	renamesBefore, deletesBefore := readScopeDriftOps(t, repo)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.NotContains(t, out, "scope-rename")
	assert.NotContains(t, out, "scope-delete")

	renamesAfter, deletesAfter := readScopeDriftOps(t, repo)
	assert.Len(t, renamesAfter, len(renamesBefore), "unscoped git mv must not emit scope-rename")
	assert.Len(t, deletesAfter, len(deletesBefore), "unscoped git mv must not emit scope-delete")

	run(t, repo, "git", "rm", "unscoped-rm.txt")
	commitNoHooks(t, repo, "delete unscoped-rm.txt")

	out, err = runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.NotContains(t, out, "scope-rename")
	assert.NotContains(t, out, "scope-delete")

	renamesAfter, deletesAfter = readScopeDriftOps(t, repo)
	assert.Len(t, renamesAfter, len(renamesBefore), "unscoped git rm must not emit scope-rename")
	assert.Len(t, deletesAfter, len(deletesBefore), "unscoped git rm must not emit scope-delete")
}

func setupRepoWithScopedTask(t *testing.T, taskID, scopePath string) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "create", "--title", "Scoped task", "--type", "task", "--id", taskID, "--scope", scopePath)
	require.NoError(t, err)
	return repo
}

func writeFile(t *testing.T, repo, relPath, content string) {
	t.Helper()
	full := filepath.Join(repo, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0644))
}
