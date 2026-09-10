package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReadyCommand_WavesFlagGroupedOutput_REQ_LNGHZN_S2_T1 verifies that
// `arm ready --waves --format json` groups ready issues into scope-disjoint
// waves as a waves adjunct on the Agent Output Contract envelope.
func TestReadyCommand_WavesFlagGroupedOutput_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	armatureDir := setupArmatureLayout(t, repo)
	t.Logf("armature dir: %s", armatureDir)

	// Create three ready tasks with disjoint scopes so they can all land in
	// one wave, plus a fourth task sharing scope with the first so it must
	// be placed in a different wave.
	tasks := []struct {
		id    string
		title string
		scope string
	}{
		{"task-01", "Task One", "src/auth/**"},
		{"task-02", "Task Two", "src/api/**"},
		{"task-03", "Task Three", "src/db/**"},
		{"task-04", "Task Four", "src/auth/login.go"},
	}

	for _, tk := range tasks {
		plantVerifiedTask(t, repo, tk.id, tk.scope)
	}

	out, err := runTrls(t, repo, "ready", "--waves", "--format", "json")
	require.NoError(t, err)

	decoded := decodeReadyEnvelope(t, out)
	require.Contains(t, decoded, "waves")

	var waveIDs [][]string
	require.NoError(t, json.Unmarshal(decoded["waves"], &waveIDs), "waves adjunct must be id groups: %s", out)
	require.NotEmpty(t, waveIDs, "expected at least one wave")

	var issues []readyIssueRow
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	require.NotEmpty(t, issues, "rows stay in issues; waves is an adjunct")

	totalIssues := 0
	for _, wave := range waveIDs {
		totalIssues += len(wave)
	}
	require.GreaterOrEqual(t, totalIssues, 1, "expected at least one issue across all waves")
	require.Equal(t, len(issues), totalIssues)

	// task-01 and task-04 share scope, so they must not land in the same wave.
	for _, wave := range waveIDs {
		hasTask01 := false
		hasTask04 := false
		for _, id := range wave {
			if id == "task-01" {
				hasTask01 = true
			}
			if id == "task-04" {
				hasTask04 = true
			}
		}
		require.False(t, hasTask01 && hasTask04, "task-01 and task-04 share scope and must not be in the same wave")
	}
}
