package main

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverrideReleaseRequiresTty_REQ_LNGHZN_S10_T12(t *testing.T) {
	repo := setupRepoWithValidDraftNode(t)

	orig := openControllingTTY
	t.Cleanup(func() { openControllingTTY = orig })
	openControllingTTY = func() (*os.File, error) {
		return nil, errors.New("no controlling terminal")
	}

	_, err := runTrls(t, repo, "dag", "override-release", "draft-task-01", "--reason", "emergency")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "controlling terminal")
	assert.NotContains(t, err.Error(), "skip-validate-gate")
}

func TestCreateEmitsDraft_REQ_LNGHZN_S10_T12(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create",
		"--type", "task",
		"--title", "Always draft",
		"--id", "draft-born-01",
		"--confidence", "verified",
		"--scope", "cmd/armature/draft.go",
		"--dod", "Draft birth is recorded and tested",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, out, "draft-born-01", "create must emit draft even when --confidence verified is passed")
}

func TestConfirmRunsPlanReleaseGate_REQ_LNGHZN_S10_T12(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	createOverlappingTask(t, repo, "draft-a", "Implement first overlapping draft")
	createOverlappingTask(t, repo, "open-b", "Implement second overlapping task")
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "confirm", "draft-a")
	require.Error(t, err, "confirm must run the whole-graph Plan Release gate")
	assert.Contains(t, err.Error(), "validation failed")
	assert.NotContains(t, err.Error(), "override-release")
	assert.NotContains(t, err.Error(), "skip-validate-gate")
}
