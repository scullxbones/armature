package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func markIssueDoneLegacy(t *testing.T, repo, issueID string) {
	t.Helper()
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type:      ops.OpTransition,
		TargetID:  issueID,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload:   ops.Payload{To: ops.StatusDone, Outcome: "legacy done without snapshot"},
	}))
}

func doneWithoutAssessment(t *testing.T) string {
	t.Helper()
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done",
		"--skip-delivery-gate", "--force", "--outcome", "complete")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	landDelivery(t, repo, "task-01")
	return repo
}

func decodeSyncIssues(t *testing.T, stdout string) (help []string, rows []output.SyncIssue) {
	t.Helper()
	decoded := decodeContractEnvelope(t, stdout, "issues")
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NoError(t, json.Unmarshal(decoded["issues"], &rows))
	require.NotEmpty(t, help)
	assert.True(t, strings.HasPrefix(help[0], "arm "), "help[0] must name a concrete arm command, got %q", help[0])
	return help, rows
}

func TestSyncReportsLegacyDoneWithoutFailing_REQ_LNGHZN_S11_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	markIssueDoneLegacy(t, repo, "task-01")

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"sync", "--repo", repo, "--format", "agent")
	assert.Equal(t, 0, code)
	assert.NotContains(t, stdout.String(), `"error"`)

	help, rows := decodeSyncIssues(t, stdout.String())
	require.Len(t, rows, 1)
	assert.Equal(t, "task-01", rows[0].ID)
	assert.Equal(t, "legacy", rows[0].Kind)
	assert.Equal(t, "done", rows[0].Status)
	assert.Contains(t, rows[0].NextAction, "arm delivery record --issue task-01")
	assert.Equal(t, rows[0].NextAction, help[0])
}

func TestSyncExitNonZeroWhenLandedDeliveryLacksAssessment_REQ_LNGHZN_S11_T3(t *testing.T) {
	repo := doneWithoutAssessment(t)

	ctx := getTestContext(t, repo)
	_, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	statBefore, err := os.Stat(logPath)
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"sync", "--repo", repo, "--dry-run", "--format", "agent")
	assert.Equal(t, 1, code, "dry-run uses the same exit rule")
	assert.NotContains(t, stdout.String(), `"error"`, "completed sync is an ADR 0017 envelope")
	statAfterDry, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, statBefore.Size(), statAfterDry.Size(), "--dry-run writes nothing")

	stdout.Reset()
	code = executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"sync", "--repo", repo, "--format", "agent")
	assert.Equal(t, 1, code)
	assert.NotContains(t, stdout.String(), `"error"`)

	help, rows := decodeSyncIssues(t, stdout.String())
	require.Len(t, rows, 1)
	assert.Equal(t, "task-01", rows[0].ID)
	assert.Equal(t, "missing-assessment", rows[0].Kind)
	assert.Equal(t, "done", rows[0].Status)
	assert.Equal(t, "arm review record --issue task-01 --assessment <assessment.json>", rows[0].NextAction)
	assert.Equal(t, rows[0].NextAction, help[0])

	status, showErr := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, showErr)
	assert.Equal(t, "done\n", status)

	logged, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	foundCheck := false
	for _, op := range logged {
		if op.Type == ops.OpPromotionCheck && op.TargetID == "task-01" {
			foundCheck = true
			assert.Equal(t, "missing-assessment", op.Payload.Result)
		}
	}
	assert.True(t, foundCheck, "live sync appends a promotion-check for the refusal")
}

func TestSyncAgentEnvelopeShape_REQ_LNGHZN_S11_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"sync", "--repo", repo, "--format", "agent")
	assert.Equal(t, 0, code)
	decoded := decodeContractEnvelope(t, stdout.String(), "issues")
	var count int
	require.NoError(t, json.Unmarshal(decoded["count"], &count))
	var issues []output.SyncIssue
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	assert.Equal(t, count, len(issues))
	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NotEmpty(t, help)
	assert.True(t, strings.HasPrefix(help[0], "arm "))
}

func TestSyncDryRunWritesNothingOnReadOnlyState_REQ_LNGHZN_S11_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	markIssueDoneLegacy(t, repo, "task-01")
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	stateDir := getTestStateDir(t, repo)
	issuesDir := filepath.Join(stateDir, "issues")
	entries, err := os.ReadDir(issuesDir)
	require.NoError(t, err)
	for _, e := range entries {
		require.NoError(t, os.Chmod(filepath.Join(issuesDir, e.Name()), 0o444))
	}
	require.NoError(t, os.Chmod(filepath.Join(stateDir, "index.json"), 0o444))
	require.NoError(t, os.Chmod(filepath.Join(stateDir, "checkpoint.json"), 0o444))
	require.NoError(t, os.Chmod(issuesDir, 0o555))
	require.NoError(t, os.Chmod(stateDir, 0o555))
	t.Cleanup(func() {
		if chmodErr := os.Chmod(stateDir, 0o755); chmodErr != nil {
			t.Logf("restore state dir perms: %v", chmodErr)
		}
		if chmodErr := os.Chmod(issuesDir, 0o755); chmodErr != nil {
			t.Logf("restore issues dir perms: %v", chmodErr)
		}
		for _, name := range []string{"index.json", "checkpoint.json"} {
			if chmodErr := os.Chmod(filepath.Join(stateDir, name), 0o644); chmodErr != nil {
				t.Logf("restore %s perms: %v", name, chmodErr)
			}
		}
		restored, readErr := os.ReadDir(issuesDir)
		if readErr != nil {
			t.Logf("readdir issues after restore: %v", readErr)
			return
		}
		for _, e := range restored {
			if chmodErr := os.Chmod(filepath.Join(issuesDir, e.Name()), 0o644); chmodErr != nil {
				t.Logf("restore issue perms %s: %v", e.Name(), chmodErr)
			}
		}
	})

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"sync", "--repo", repo, "--dry-run", "--format", "agent")
	assert.Equal(t, 0, code, "dry-run must not require a writable state directory")
	_, rows := decodeSyncIssues(t, stdout.String())
	require.Len(t, rows, 1)
	assert.Equal(t, "legacy", rows[0].Kind)
}

func TestSyncLegacyOnlySucceedsWithoutWorker_REQ_LNGHZN_S11_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	markIssueDoneLegacy(t, repo, "task-01")
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	unsetWorkerIDConfig(t, repo)
	t.Setenv("ARM_WORKER_ID", "")
	t.Setenv("ARM_LOG_SLOT", "")

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, stderr,
		"sync", "--repo", repo, "--format", "agent")
	assert.Equal(t, 0, code, "legacy-only sync must list without a registered worker; stderr=%s", stderr.String())
	assert.NotContains(t, stdout.String(), `"error"`)
	assert.NotContains(t, stdout.String(), "SYNC-1")
	_, rows := decodeSyncIssues(t, stdout.String())
	require.Len(t, rows, 1)
	assert.Equal(t, "legacy", rows[0].Kind)
}
