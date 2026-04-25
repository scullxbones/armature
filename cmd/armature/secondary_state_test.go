package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecondaryStatePaths(t *testing.T) {
	repo := initTempRepo(t)
	// Create an initial commit so git is fully initialized
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// 1. Initialize trellis and worker
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Get the worker ID to know the state path
	workerID, err := runTrls(t, repo, "worker-init", "--check")
	require.NoError(t, err)
	// workerID looks like "Worker ID: <uuid>\n"
	workerID = workerID[len("Worker ID: "):]
	workerID = workerID[:len(workerID)-1]

	// 2. Create an issue and materialize
	_, err = runTrls(t, repo, "create", "--title", "Test Issue", "--id", "TASK-1")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	stateDir := filepath.Join(repo, ".armature", "state", workerID)
	require.DirExists(t, stateDir)
	require.FileExists(t, filepath.Join(stateDir, "index.json"))

	// 3. Ensure NO state exists in the old common location or other worker dirs
	// (trls init might have created some structure, let's be thorough)
	entries, err := os.ReadDir(filepath.Join(repo, ".armature", "state"))
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.Name() != workerID {
			_ = os.RemoveAll(filepath.Join(repo, ".armature", "state", entry.Name()))
		}
	}
	// Also ensure no index.json in .armature directly (though it shouldn't be there anyway)
	_ = os.Remove(filepath.Join(repo, ".armature", "index.json"))

	// 4. Verify secondary commands work using ONLY the worker-specific state

	// list
	out, err := runTrls(t, repo, "list")
	require.NoError(t, err)
	require.Contains(t, out, "TASK-1")

	// show
	out, err = runTrls(t, repo, "show", "TASK-1")
	require.NoError(t, err)
	require.Contains(t, out, "Test Issue")

	// list
	out, err = runTrls(t, repo, "list")
	require.NoError(t, err)
	require.Contains(t, out, "TASK-1")

	// merged (requires done status first)
	_, err = runTrls(t, repo, "transition", "--issue", "TASK-1", "--to", "done", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "merged", "--issue", "TASK-1")
	require.NoError(t, err)

	// dag-summary
	// Needs a draft node. TASK-1 is verified by default. Let's create a draft.
	_, err = runTrls(t, repo, "create", "--title", "Draft Issue", "--id", "TASK-2", "--confidence", "draft")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	out, err = runTrls(t, repo, "dag-summary", "--format", "json")
	require.NoError(t, err)
	require.Contains(t, out, "TASK-2")

	// render-context
	out, err = runTrls(t, repo, "render-context", "TASK-1")
	require.NoError(t, err)
	require.Contains(t, out, "# Issue: Test Issue")
}
