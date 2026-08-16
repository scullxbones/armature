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
	_, err := runTrls(t, repo, "create",
		"--type", "task",
		"--title", id,
		"--id", id,
		"--scope", "internal/ops/*.go",
		"--dod", dod,
		"--acceptance", testAcceptance,
	)
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
	assert.Contains(t, out, "ERROR: scope overlap")
	assert.NotContains(t, out, "WARNING: scope overlap")

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

func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
