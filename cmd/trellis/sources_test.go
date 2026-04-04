package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourcesAddCommand_WarnOnRelativePath verifies that adding a filesystem
// source with a relative path emits a warning to stderr.
func TestSourcesAddCommand_WarnOnRelativePath(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(stdoutBuf)
	cmd.SetErr(stderrBuf)
	cmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", "docs/relative/path.md", "--type", "filesystem", "--title", "Relative"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, stdoutBuf.String(), "added source")
	assert.Contains(t, stderrBuf.String(), "relative")
}

// TestSourcesSyncCommand_ErrorOnUnreachablePath verifies that sync emits an
// error (not a silent skip) when a filesystem source path is unreachable.
func TestSourcesSyncCommand_ErrorOnUnreachablePath(t *testing.T) {
	repo := setupRepoWithTask(t)

	// First, add a source with an unreachable path.
	addBuf := new(bytes.Buffer)
	addCmd := newRootCmd()
	addCmd.SetOut(addBuf)
	addCmd.SetErr(new(bytes.Buffer))
	addCmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", "/nonexistent/path/does_not_exist.md", "--type", "filesystem"})

	err := addCmd.Execute()
	require.NoError(t, err)

	// Now sync and expect an error (not silent skip).
	syncBuf := new(bytes.Buffer)
	syncErrBuf := new(bytes.Buffer)
	syncCmd := newRootCmd()
	syncCmd.SetOut(syncBuf)
	syncCmd.SetErr(syncErrBuf)
	syncCmd.SetArgs([]string{"sources", "sync", "--repo", repo})

	err = syncCmd.Execute()
	// The sync command should return an error due to the unreachable path.
	assert.Error(t, err, "sync should error on unreachable filesystem path")
	assert.NotContains(t, syncErrBuf.String(), "skip", "should emit error, not silent skip")
}

// TestSourcesSyncCommand_SuccessWithReachablePath verifies that sync succeeds
// when all filesystem source paths are reachable.
func TestSourcesSyncCommand_SuccessWithReachablePath(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create a temporary file to use as a source.
	tmpfile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(tmpfile, []byte("test source content"), 0600)
	require.NoError(t, err)

	// Add the source with the reachable path.
	addBuf := new(bytes.Buffer)
	addCmd := newRootCmd()
	addCmd.SetOut(addBuf)
	addCmd.SetErr(new(bytes.Buffer))
	addCmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", tmpfile, "--type", "filesystem"})

	err = addCmd.Execute()
	require.NoError(t, err)

	// Now sync and expect success.
	syncBuf := new(bytes.Buffer)
	syncCmd := newRootCmd()
	syncCmd.SetOut(syncBuf)
	syncCmd.SetErr(new(bytes.Buffer))
	syncCmd.SetArgs([]string{"sources", "sync", "--repo", repo})

	err = syncCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, syncBuf.String(), "synced")
}
