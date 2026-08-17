package main

import (
	"strings"
	"testing"

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
