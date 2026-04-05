package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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
	_, err := runTrls(t, repo, "claim", "task-01")
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
	_, err := runTrls(t, repo, "claim", "task-01")
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
	_, err := runTrls(t, repo, "claim", "task-01")
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
	_ = cmd.Execute()
	assert.Contains(t, buf.String(), "hook")
}
