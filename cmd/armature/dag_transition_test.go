package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDAGTransitionCmd_RejectsInvalidToValue verifies that --to on dag-transition
// is validated as a confidence value (draft, verified), not silently accepted as
// an arbitrary string (which would otherwise stamp a nonsensical value like
// "done" into Provenance.Confidence — see surface-census.md dag-transition --to row).
func TestDAGTransitionCmd_RejectsInvalidToValue(t *testing.T) {
	repo := setupRepoWithDraftNode(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"dag", "transition", "--repo", repo, "--issue", "draft-task-01", "--to", "done"})
	err := root.Execute()
	require.Error(t, err, "dag-transition --to done should be rejected: done is a status, not a confidence value")
	assert.Contains(t, err.Error(), "confidence")
}

// TestDAGTransitionCmd_AcceptsValidToValue verifies the legitimate confidence values
// still work on a validate-green graph.
func TestDAGTransitionCmd_AcceptsValidToValue(t *testing.T) {
	repo := setupRepoWithValidDraftNode(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "transition", "--repo", repo, "--issue", "draft-task-01", "--to", "verified"})
	require.NoError(t, root.Execute())
}

func setupRepoWithValidDraftNode(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create",
		"--type", "task",
		"--title", "Draft task",
		"--id", "draft-task-01",
		"--confidence", "draft",
		"--scope", "cmd/armature/draft.go",
		"--dod", "Draft task is complete and tested",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)
	return repo
}

// TestDagTransitionRequiresValidateGreen_REQ_LNGHZN_S10_T4: promoting a
// subtree to verified is refused while the graph has validate findings.
func TestDagTransitionRequiresValidateGreen_REQ_LNGHZN_S10_T4(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create",
		"--type", "task",
		"--title", "Draft overlap A",
		"--id", "draft-a",
		"--confidence", "draft",
		"--scope", "internal/ops/*.go",
		"--dod", "Implement first overlapping draft",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)
	createOverlappingTask(t, repo, "open-b", "Implement second overlapping task")

	_, err = runTrls(t, repo, "dag", "transition", "--issue", "draft-a", "--to", "verified")
	require.Error(t, err, "plan release must refuse a graph with validate findings")
	assert.Contains(t, err.Error(), "validation failed")

	_, err = runTrls(t, repo, "link", "--source", "open-b", "--dep", "draft-a")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "dag", "transition", "--issue", "draft-a", "--to", "verified")
	require.NoError(t, err)
	assert.Contains(t, out, "draft-a")
}
