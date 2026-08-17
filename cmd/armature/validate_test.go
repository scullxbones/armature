package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAcceptance = `[{"type":"test_passes","cmd":"go test"}]`

func createOverlappingTask(t *testing.T, repo, id, dod string) {
	t.Helper()
	// Plant via raw ops so fixtures can represent a dirty graph without
	// being refused by the write-time Introduction check.
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	err = appendRawCreate(logPath, workerID, id, dod, "internal/ops/*.go")
	require.NoError(t, err)
}

// TestValidateStrictDefault_REQ_LNGHZN_S10_T4: arm validate is strict by
// default — warnings are errors and a green run prints only a summary line.
func TestValidateStrictDefault_REQ_LNGHZN_S10_T4(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	createOverlappingTask(t, repo, "tsk-a", "Implement ops overlap case")
	createOverlappingTask(t, repo, "tsk-b", "Implement sibling ops overlap")

	out, err := runTrls(t, repo, "validate")
	require.Error(t, err, "default validate must fail closed when warnings exist")
	assert.Contains(t, err.Error(), "validation failed")
	assert.Contains(t, err.Error(), "warning(s)", "strict failure must distinguish promoted W-codes from E-codes")
	assert.Contains(t, out, "WARNING: scope overlap")
	assert.NotContains(t, out, "ERROR: scope overlap")

	_, err = runTrls(t, repo, "link", "--source", "tsk-b", "--dep", "tsk-a")
	require.NoError(t, err)

	out, err = runTrls(t, repo, "validate")
	require.NoError(t, err, "serialized overlap must be green")
	lines := nonEmptyLines(out)
	require.Len(t, lines, 1, "green output must be a single summary line, got %q", out)
	assert.True(t, strings.HasPrefix(lines[0], "OK:"), "summary must start with OK:, got %q", lines[0])
	assert.NotContains(t, out, "WARNING:")
	assert.NotContains(t, out, "INFO:")
	assert.NotContains(t, out, "ERROR:")
}

// TestValidateRejectsScopedFlags_REQ_LNGHZN_S10_T4: D7 rejects partial
// validation. --scope and --parent must not be registered on arm validate.
func TestValidateRejectsScopedFlags_REQ_LNGHZN_S10_T4(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "validate", "--scope", "EPIC-001")
	require.Error(t, err, "D7: --scope is a rejected partial-validation escape hatch")
	assert.Contains(t, err.Error(), "unknown flag: --scope")

	_, err = runTrls(t, repo, "validate", "--parent", "EPIC-001")
	require.Error(t, err, "D7: --parent is a rejected partial-validation escape hatch")
	assert.Contains(t, err.Error(), "unknown flag: --parent")
}

// TestValidateStrictFalseShowsWarnings_REQ_LNGHZN_S10_T4: --strict=false
// keeps warnings as warnings (exit 0) but human output still lists them.
func TestValidateStrictFalseShowsWarnings_REQ_LNGHZN_S10_T4(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	createOverlappingTask(t, repo, "tsk-a", "Implement ops overlap case")
	createOverlappingTask(t, repo, "tsk-b", "Implement sibling ops overlap")

	out, err := runTrls(t, repo, "validate", "--strict=false")
	require.NoError(t, err, "--strict=false must keep warnings as warnings (exit 0)")
	assert.Contains(t, out, "WARNING: scope overlap", "--strict=false human output must list warning-level findings")
	assert.NotContains(t, out, "ERROR: scope overlap")
}

// TestValidateJSONKeepsWarningBuckets_REQ_LNGHZN_S10_T4: default-strict JSON
// keeps W-codes under "warnings" so agents can triage severity.
func TestValidateJSONKeepsWarningBuckets_REQ_LNGHZN_S10_T4(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	createOverlappingTask(t, repo, "tsk-a", "Implement ops overlap case")
	createOverlappingTask(t, repo, "tsk-b", "Implement sibling ops overlap")

	out, err := runTrls(t, repo, "validate", "--format", "json")
	require.Error(t, err, "default-strict JSON must fail closed on warnings")
	assert.Contains(t, out, `"warnings"`)
	assert.Contains(t, out, "scope overlap")
	assert.NotContains(t, out, `"errors": [
    "scope overlap`)
}

// TestValidateStrictFalsePrintsInfos_REQ_LNGHZN_S10_T4: silent green is
// strict-only; --strict=false must still print INFO lines.
func TestValidateStrictFalsePrintsInfos_REQ_LNGHZN_S10_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "--issue", "task-01",
		"--scope", "nonexistent/file.go",
		"--acceptance", testAcceptance,
	)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "validate", "--strict=false")
	require.NoError(t, err)
	assert.Contains(t, out, "INFO: phantom scope", "--strict=false human output must list INFO findings")
}

// TestValidateNonStrictStillFailsOnErrors_REQ_LNGHZN_S10_T4: --strict=false
// keeps warnings as warnings but still exits non-zero on hard errors (E-codes).
func TestValidateNonStrictStillFailsOnErrors_REQ_LNGHZN_S10_T4(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	longDoD := strings.Repeat("x", 501)
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, appendRawCreate(logPath, workerID, "tsk-e9", longDoD, "internal/ops/*.go"))

	out, err := runTrls(t, repo, "validate", "--strict=false")
	require.Error(t, err, "--strict=false must still fail closed on E-codes")
	assert.Contains(t, err.Error(), "validation failed")
	assert.Contains(t, out, "ERROR:")
	assert.Contains(t, out, "definition_of_done exceeds")
}

// TestValidateCiRejectsStrictFalse_REQ_LNGHZN_S10_T4: --ci --strict=false is
// a contradiction, not a silent override.
func TestValidateCiRejectsStrictFalse_REQ_LNGHZN_S10_T4(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "validate", "--ci", "--strict=false")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--ci")
	assert.Contains(t, err.Error(), "--strict=false")
}

func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestIntroductionReplaysSortedOps_REQ_LNGHZN_S10_T12 plants an I3 two-log
// interleave where worker B's file (name-sorts first) holds a same-timestamp
// create+link whose source is created in worker A's later file. File-concat
// replay drops that link; sort (creates before same-timestamp links) keeps it.
// Closing the cycle must then be refused, matching whole-graph validate.
func TestIntroductionReplaysSortedOps_REQ_LNGHZN_S10_T12(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	ctx := getTestContext(t, repo)
	opsDir := filepath.Join(ctx.IssuesDir, "ops")
	ts := nowEpoch()

	plantSortedReplayCreate := func(logPath, workerID, id, scope string) {
		t.Helper()
		require.NoError(t, ops.AppendOp(logPath, ops.Op{
			Type:      ops.OpCreate,
			TargetID:  id,
			Timestamp: ts,
			WorkerID:  workerID,
			Payload: ops.Payload{
				Title:            id,
				NodeType:         "task",
				Scope:            []string{scope},
				DefinitionOfDone: "Task " + id + " is complete and tested",
				Acceptance:       json.RawMessage(testAcceptance),
				Confidence:       "draft",
			},
		}))
	}

	// aaa-worker.log concatenates before zzz-worker.log (os.ReadDir name order).
	bLog := filepath.Join(opsDir, "aaa-worker.log")
	plantSortedReplayCreate(bLog, "aaa-worker", "cycle-b", "cmd/armature/cycle_b.go")
	require.NoError(t, ops.AppendOp(bLog, ops.Op{
		Type:      ops.OpLink,
		TargetID:  "cycle-a",
		Timestamp: ts,
		WorkerID:  "aaa-worker",
		Payload:   ops.Payload{Dep: "cycle-b", Rel: "blocked_by"},
	}))

	aLog := filepath.Join(opsDir, "zzz-worker.log")
	plantSortedReplayCreate(aLog, "zzz-worker", "cycle-a", "cmd/armature/cycle_a.go")

	_, err = runTrls(t, repo, "link", "--source", "cycle-b", "--dep", "cycle-a")
	require.Error(t, err, "Introduction must refuse a cycle that sorted+rolled-up validate would report")
	assert.Contains(t, err.Error(), "cycle")
}
