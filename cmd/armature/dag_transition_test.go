package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		"--scope", "cmd/armature/draft.go",
		"--dod", "Draft task is complete and tested",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)
	return repo
}

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

func TestDagTransitionValidateFailureDistinguishesWarnings_REQ_LNGHZN_S10_T4(t *testing.T) {
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
		"--scope", "internal/ops/*.go",
		"--dod", "Implement first overlapping draft",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)
	createOverlappingTask(t, repo, "open-b", "Implement second overlapping task")

	_, err = runTrls(t, repo, "dag", "transition", "--issue", "draft-a", "--to", "verified")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "warning(s)")
	assert.NotContains(t, err.Error(), "--skip-validate-gate", "happy-path errors must not advertise the override")
	assert.NotContains(t, err.Error(), "override-release", "happy-path errors must not name the override command")
}
