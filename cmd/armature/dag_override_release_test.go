package main

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRepoWithDirtyDraftNode(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "draft-task-01",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Title:            "Draft task",
			NodeType:         "task",
			Scope:            []string{"cmd/armature/draft.go"},
			DefinitionOfDone: "Do this properly so the override is justified",
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       "draft",
		},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	return repo
}

func TestOverrideReleaseRequiresTty_REQ_LNGHZN_S10_T12(t *testing.T) {
	repo := setupRepoWithDirtyDraftNode(t)

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

func TestOverrideReleaseRequiresReasonBeforeTTY(t *testing.T) {
	repo := setupRepoWithDirtyDraftNode(t)
	ttyOpened := false
	orig := openControllingTTY
	t.Cleanup(func() { openControllingTTY = orig })
	openControllingTTY = func() (*os.File, error) {
		ttyOpened = true
		return nil, errors.New("no controlling terminal")
	}

	_, err := runTrls(t, repo, "dag", "override-release", "draft-task-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--reason is required")
	assert.False(t, ttyOpened, "flags must be validated before touching the TTY")
}

func TestOverrideReleaseRequiresExistingDirtyDraft(t *testing.T) {
	repo := setupRepoWithValidDraftNode(t)
	orig := openControllingTTY
	t.Cleanup(func() { openControllingTTY = orig })
	openControllingTTY = func() (*os.File, error) {
		return nil, errors.New("no controlling terminal")
	}

	_, err := runTrls(t, repo, "dag", "override-release", "does-not-exist", "--reason", "emergency")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	_, err = runTrls(t, repo, "dag", "override-release", "draft-task-01", "--reason", "emergency")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no blocking findings")
}

func TestOverrideReleaseAllowsWhenForeignFindingBlocksPlanRelease(t *testing.T) {
	repo := setupRepoWithValidDraftNode(t)
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, appendRawCreate(logPath, workerID, "OTHER-9", "Do this properly for the foreign draft", "internal/other.go"))

	orig := openControllingTTY
	t.Cleanup(func() { openControllingTTY = orig })
	openControllingTTY = func() (*os.File, error) {
		return nil, errors.New("no controlling terminal")
	}

	_, err = runTrls(t, repo, "dag", "override-release", "draft-task-01", "--reason", "waive foreign W4")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "controlling terminal")
	assert.NotContains(t, err.Error(), "no blocking findings")
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
		"--scope", "cmd/armature/draft.go",
		"--dod", "Draft birth is recorded and tested",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)
	assert.NotContains(t, out, "draft-born-01", "create without --confidence must emit draft")
}

func TestCreateRejectsConfidenceFlag(t *testing.T) {
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dag transition")
	assert.Contains(t, err.Error(), "--confidence")
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
