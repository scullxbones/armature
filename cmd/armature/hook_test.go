package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHookRunUnknown verifies that an unknown hook name returns an error.
func TestHookRunUnknown(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "hook", "run", "unknown-hook")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown hook")
}

// TestHookRunMissingArg verifies that hook run with no hook name returns an error.
func TestHookRunMissingArg(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "hook", "run")
	assert.Error(t, err)
}

// TestHookRunPostMerge verifies that post-merge hook runs sync logic without error.
func TestHookRunPostMerge(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "hook", "run", "post-merge")
	require.NoError(t, err)
	assert.Contains(t, out, "No merged branches detected")
}

// TestHookRunPostCommit_NoActiveClaim verifies post-commit succeeds with no active claim.
func TestHookRunPostCommit_NoActiveClaim(t *testing.T) {
	repo := setupRepoWithTask(t)

	// post-commit sends heartbeat if there's an active claim, otherwise no-ops
	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	// No active claim — should produce no output or a skip message
	_ = out
}

// TestHookRunPostCommit_WithActiveClaim verifies post-commit sends a heartbeat when a claim is active.
func TestHookRunPostCommit_WithActiveClaim(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Claim the task first
	_, err := runTrls(t, repo, "claim", "task-01", "--worktree", filepath.Join(t.TempDir(), "claim-task-01-wt"))
	require.NoError(t, err)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")
}

// TestHookRunPreCommit_SingleBranch verifies pre-commit is a no-op in single-branch mode.
func TestHookRunPreCommit_SingleBranch(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Single-branch mode — pre-commit should always allow ops commits
	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	require.NoError(t, err)
}

// TestHookRunPrepareCommitMsg_NoActiveClaim verifies prepare-commit-msg is a no-op without active claim.
func TestHookRunPrepareCommitMsg_NoActiveClaim(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Write a commit message file
	msgFile := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	require.NoError(t, os.WriteFile(msgFile, []byte("feat: my commit\n"), 0644))

	_, err := runTrls(t, repo, "hook", "run", "prepare-commit-msg", msgFile)
	require.NoError(t, err)

	// Without active claim, commit message should be unchanged
	content, err := os.ReadFile(msgFile)
	require.NoError(t, err)
	assert.Equal(t, "feat: my commit\n", string(content))
}

// TestHookRunPrepareCommitMsg_WithActiveClaim verifies prepare-commit-msg prepends claim ID.
func TestHookRunPrepareCommitMsg_WithActiveClaim(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Claim the task
	_, err := runTrls(t, repo, "claim", "task-01", "--worktree", filepath.Join(t.TempDir(), "claim-task-01-wt"))
	require.NoError(t, err)

	// Write a commit message file
	msgFile := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	require.NoError(t, os.WriteFile(msgFile, []byte("feat: my commit\n"), 0644))

	_, err = runTrls(t, repo, "hook", "run", "prepare-commit-msg", msgFile)
	require.NoError(t, err)

	// Should prepend task-01 to the commit message
	content, err := os.ReadFile(msgFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "task-01")
	assert.Contains(t, string(content), "feat: my commit")
}

// TestHookRunPrepareCommitMsg_MissingFile verifies error when commit msg file is missing.
func TestHookRunPrepareCommitMsg_MissingFile(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Claim the task
	_, err := runTrls(t, repo, "claim", "task-01", "--worktree", filepath.Join(t.TempDir(), "claim-task-01-wt"))
	require.NoError(t, err)

	_, err = runTrls(t, repo, "hook", "run", "prepare-commit-msg", "/nonexistent/COMMIT_EDITMSG")
	assert.Error(t, err)
}

// TestHookSubcommandHelp verifies the hook subcommand help text.
func TestHookSubcommandHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"hook", "--help"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "hook")
}

// TestHookPostCommit_InitialCommit verifies that post-commit does not error when HEAD~1 is absent.
func TestHookPostCommit_InitialCommit(t *testing.T) {
	// Build a repo with NO parent commit so HEAD~1 is absent.
	repo := initTempRepo(t)

	// arm init requires at least one commit; make a bare commit then immediately run arm init.
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	// Create a task with scope so detection has something to work with.
	_, err := runTrls(t, repo, "create", "--title", "Scoped task", "--type", "task", "--id", "task-scope-01", "--scope", "src/foo.go")
	require.NoError(t, err)

	// Claim it.
	_, err = runTrls(t, repo, "claim", "task-scope-01", "--worktree", filepath.Join(t.TempDir(), "claim-task-scope-01-wt"))
	require.NoError(t, err)

	// Now run post-commit on the very first real commit (HEAD~1 absent for the init commit).
	// hookDetectScopeChanges should skip silently and not error.
	_, err = runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
}

// TestHookPostCommit_ScopeRename verifies that post-commit emits scope-rename ops
// for issues whose scope contains the renamed path.
func TestHookPostCommit_ScopeRename(t *testing.T) {
	repo := setupRepoWithScopedTask(t, "task-rename-01", "src/old.go")

	// Perform a rename and commit so HEAD~1 exists.
	writeFile(t, repo, "src/old.go", "package old")
	run(t, repo, "git", "add", "src/old.go")
	run(t, repo, "git", "commit", "-m", "add src/old.go")

	run(t, repo, "git", "mv", "src/old.go", "src/new.go")
	run(t, repo, "git", "commit", "-m", "rename src/old.go -> src/new.go")

	// Claim the task so there's an active claim and a log path.
	_, err := runTrls(t, repo, "claim", "task-rename-01", "--worktree", filepath.Join(t.TempDir(), "claim-task-rename-01-wt"))
	require.NoError(t, err)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "scope-rename")
	assert.Contains(t, out, "task-rename-01")
}

// TestHookPostCommit_ScopeDelete verifies that post-commit emits scope-delete ops
// for issues whose scope exactly matches the deleted path.
func TestHookPostCommit_ScopeDelete(t *testing.T) {
	repo := setupRepoWithScopedTask(t, "task-delete-01", "src/gone.go")

	// Add a file then delete it.
	writeFile(t, repo, "src/gone.go", "package gone")
	run(t, repo, "git", "add", "src/gone.go")
	run(t, repo, "git", "commit", "-m", "add src/gone.go")

	run(t, repo, "git", "rm", "src/gone.go")
	run(t, repo, "git", "commit", "-m", "delete src/gone.go")

	// Claim the task so there's an active claim and a log path.
	_, err := runTrls(t, repo, "claim", "task-delete-01", "--worktree", filepath.Join(t.TempDir(), "claim-task-delete-01-wt"))
	require.NoError(t, err)

	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "scope-delete")
	assert.Contains(t, out, "task-delete-01")
}

// TestHookRunPreCommit_NoStagedFiles verifies that pre-commit succeeds when no
// files are staged.
func TestHookRunPreCommit_NoStagedFiles(t *testing.T) {
	repo := setupRepoWithTask(t)

	run(t, repo, "git", "reset", "HEAD")

	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	require.NoError(t, err)
}

// TestHookRunPreCommit_StagedNonOpsFile verifies that staging a non-ops file is allowed.
func TestHookRunPreCommit_StagedNonOpsFile(t *testing.T) {
	repo := setupRepoWithTask(t)

	writeFile(t, repo, "src/main.go", "package main")
	run(t, repo, "git", "add", filepath.Join("src", "main.go"))

	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	require.NoError(t, err)
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
		Mode:      "single-branch",
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
		Mode:      "single-branch",
		Config:    config.Config{DefaultTTL: 60},
	}

	assert.Empty(t, hookFindActiveClaimID(ctx))
}

// TestHookDetectScopeChanges_WithExistingCheckpoint verifies that hookDetectScopeChanges
// correctly uses ReadIndex (not Load) when checkpoint.json already exists.
// This test ensures the fix for ARCHIMP-S14 works: Store.ReadIndex avoids
// rematerializing all ops, which would re-apply non-idempotent ops and corrupt state.
func TestHookDetectScopeChanges_WithExistingCheckpoint(t *testing.T) {
	repo := setupRepoWithScopedTask(t, "task-checkpoint-scope", "src/checkpoint.go")

	// Claim the task so there's an active claim and a log path for scope-rename ops.
	_, err := runTrls(t, repo, "claim", "task-checkpoint-scope", "--worktree", filepath.Join(t.TempDir(), "claim-checkpoint-wt"))
	require.NoError(t, err)

	// Add and commit the scoped file
	writeFile(t, repo, "src/checkpoint.go", "package checkpoint")
	run(t, repo, "git", "add", "src/checkpoint.go")
	run(t, repo, "git", "commit", "-m", "add checkpoint.go")

	// First post-commit to establish checkpoint.json (triggers materialization)
	_, err = runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)

	// Verify checkpoint.json exists (evidence that materialization occurred)
	stateDir := getTestStateDir(t, repo)
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	_, err = os.Stat(checkpointPath)
	require.NoError(t, err, "checkpoint.json should exist after materialization")

	// Now rename the file and commit
	run(t, repo, "git", "mv", "src/checkpoint.go", "src/checkpoint-renamed.go")
	run(t, repo, "git", "commit", "-m", "rename checkpoint.go")

	// Second post-commit with existing checkpoint.json should not error.
	// hookDetectScopeChanges will be called.
	// With the fix (using ReadIndex), it should emit scope-rename without corrupting state.
	// With the old code (using Load), it would replay all ops and potentially corrupt state.
	out, err := runTrls(t, repo, "hook", "run", "post-commit")
	require.NoError(t, err)
	assert.Contains(t, out, "scope-rename")
	assert.Contains(t, out, "task-checkpoint-scope")
}

// setupRepoWithScopedTask initialises a repo and creates a task with the given scope path.
func setupRepoWithScopedTask(t *testing.T, taskID, scopePath string) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	_, err := runTrls(t, repo, "create", "--title", "Scoped task", "--type", "task", "--id", taskID, "--scope", scopePath)
	require.NoError(t, err)
	return repo
}

// writeFile creates (or overwrites) a file in the repo dir.
func writeFile(t *testing.T, repo, relPath, content string) {
	t.Helper()
	full := filepath.Join(repo, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0644))
}
