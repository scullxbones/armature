package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRepoWithDirtyDraftNode(t *testing.T) string {
	t.Helper()
	repo := gittest.InitWithOrigin(t).Dir
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

func TestOverrideReleaseRecordsDeliverySHAsAfterDone_REQ_LNGHZN_S11_T2(t *testing.T) {
	repo := doneWithoutAssessment(t)
	issuePath := filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json")
	issue, err := materialize.LoadIssue(issuePath)
	require.NoError(t, err)
	require.Equal(t, ops.StatusDone, issue.Status)
	require.NotEmpty(t, issue.Base)
	require.NotEmpty(t, issue.Tip)

	payload := releaseOverridePayload("task-01", "post-delivery waive", &issue)
	assert.Equal(t, issue.Base, payload.Base, "post-delivery override must bind delivery base")
	assert.Equal(t, issue.Tip, payload.Tip, "post-delivery override must bind delivery tip")
	assert.Equal(t, "verified", payload.To)
	assert.True(t, payload.SkippedValidateGate)

	root := newRootCmd()
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"dag", "override-release", "--repo", repo, "task-01", "--reason", "x"})
	require.NoError(t, root.PersistentFlags().Set("repo", repo))
	attachExecutionState(root, getTestContext(t, repo))
	_, checkErr := checkOverrideReleaseTarget(root, "task-01")
	require.NoError(t, checkErr, "done delivery must be an allowed override target")

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type:      ops.OpDAGTransition,
		TargetID:  "task-01",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload:   payload,
	}))

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"sync", "--repo", repo, "--format", "agent")
	assert.Equal(t, 0, code, "delivery-bound override must unlock promotion; out=%s", stdout.String())
	assert.Contains(t, stdout.String(), `"kind":"promote"`)
}

func TestReleaseOverridePayloadBindsDoneDelivery(t *testing.T) {
	t.Parallel()
	p := releaseOverridePayload("task-01", "reason", &materialize.Issue{
		Status: ops.StatusDone, Base: "aaa", Tip: "bbb",
	})
	assert.Equal(t, "aaa", p.Base)
	assert.Equal(t, "bbb", p.Tip)

	planTime := releaseOverridePayload("task-01", "reason", &materialize.Issue{
		Status: ops.StatusOpen, Base: "aaa", Tip: "bbb",
	})
	assert.Empty(t, planTime.Base)
	assert.Empty(t, planTime.Tip)
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

func TestOverrideReleasePropagatesSnapshotLoadError_REQ_LNGHZN_S11_T2(t *testing.T) {
	repo := setupRepoWithDirtyDraftNode(t)
	ctx := getTestContext(t, repo)
	opsDir := filepath.Join(ctx.IssuesDir, "ops")
	require.NoError(t, os.Chmod(opsDir, 0o000))
	t.Cleanup(func() {
		if chmodErr := os.Chmod(opsDir, 0o755); chmodErr != nil {
			t.Logf("restore ops dir perms: %v", chmodErr)
		}
	})

	root := newRootCmd()
	require.NoError(t, root.PersistentFlags().Set("repo", repo))
	attachExecutionState(root, ctx)
	_, err := checkOverrideReleaseTarget(root, "draft-task-01")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "not found", "load failures must not masquerade as missing issues")
	assert.Contains(t, err.Error(), "load")
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
	repo := gittest.InitWithOrigin(t).Dir
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
	repo := gittest.InitWithOrigin(t).Dir
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
	repo := gittest.InitWithOrigin(t).Dir
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
