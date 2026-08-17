package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecondaryStatePaths(t *testing.T) {
	repo := initTempRepo(t)
	// Create an initial commit so git is fully initialized
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// 1. Initialize armature and worker
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// 2. Create an issue and materialize
	_, err = runTrls(t, repo, "create", "--title", "Test Issue", "--id", "TASK-1")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// 3. Verify secondary commands work using the worker-specific state

	// list
	out, err := runTrls(t, repo, "list")
	require.NoError(t, err)
	require.Contains(t, out, "TASK-1")

	// show
	out, err = runTrls(t, repo, "show", "TASK-1")
	require.NoError(t, err)
	require.Contains(t, out, "Test Issue")

	// merged (requires done status first)
	_, err = runTrls(t, repo, "transition", "--issue", "TASK-1", "--to", "done", "--force", "--skip-delivery-gate")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "merged", "--issue", "TASK-1", "--force")
	require.NoError(t, err)

	// dag-summary
	// Needs a draft node. TASK-1 is verified by default. Let's create a draft.
	_, err = runTrls(t, repo, "create", "--title", "Draft Issue", "--id", "TASK-2")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	out, err = runTrls(t, repo, "dag", "summary", "--format", "json")
	require.NoError(t, err)
	require.Contains(t, out, "TASK-2")

	// render-context
	out, err = runTrls(t, repo, "render-context", "TASK-1")
	require.NoError(t, err)
	require.Contains(t, out, "# Issue: Test Issue")
}
