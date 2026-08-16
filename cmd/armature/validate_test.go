package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAcceptance = `[{"type":"test_passes","cmd":"go test"}]`

func createValidTask(t *testing.T, repo, id, scope, dod string) {
	t.Helper()
	_, err := runTrls(t, repo, "create",
		"--type", "task",
		"--title", id,
		"--id", id,
		"--scope", scope,
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

	createValidTask(t, repo, "tsk-a", "internal/ops/*.go", "Implement ops overlap case")
	createValidTask(t, repo, "tsk-b", "internal/ops/*.go", "Implement sibling ops overlap")

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

func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
