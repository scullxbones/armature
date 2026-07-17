package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReadyCommand_WavesFlagGroupedOutput_REQ_LNGHZN_S2_T1 verifies that
// `arm ready --waves --format json` groups ready issues into scope-disjoint
// waves and emits the documented {"waves": [[...], ...]} shape.
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
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs([]string{
			"create", "--repo", repo,
			"--title", tk.title,
			"--type", "task",
			"--id", tk.id,
			"--dod", "Task implementation is complete and verified",
			"--scope", tk.scope,
		})
		require.NoError(t, cmd.Execute())
	}

	out, err := runTrls(t, repo, "ready", "--waves", "--format", "json")
	require.NoError(t, err)

	var result struct {
		Waves [][]map[string]any `json:"waves"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result), "output should be valid JSON matching {\"waves\": [[...], ...]}: %s", out)

	require.NotEmpty(t, result.Waves, "expected at least one wave")

	totalIssues := 0
	for _, wave := range result.Waves {
		totalIssues += len(wave)
	}
	require.GreaterOrEqual(t, totalIssues, 1, "expected at least one issue across all waves")

	// task-01 and task-04 share scope, so they must not land in the same wave.
	for _, wave := range result.Waves {
		hasTask01 := false
		hasTask04 := false
		for _, entry := range wave {
			issue, ok := entry["issue"].(string)
			if ok && issue == "task-01" {
				hasTask01 = true
			}
			if issue == "task-04" {
				hasTask04 = true
			}
		}
		require.False(t, hasTask01 && hasTask04, "task-01 and task-04 share scope and must not be in the same wave")
	}
}
