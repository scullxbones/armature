package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDagApplyHonorsARMLogSlot_REQ_NOCOMMENTS(t *testing.T) {
	t.Run("with ARM_LOG_SLOT set, dag apply writes the same slot as create", func(t *testing.T) {
		t.Setenv("ARM_LOG_SLOT", "beta")
		repo := plantDagApplyLogSlotRepo(t)

		_, err := runTrls(t, repo, "create",
			"--type", "task",
			"--title", "create path",
			"--id", "SLOT-CREATE")
		require.NoError(t, err)

		applyPlanIssue(t, repo, "SLOT-APPLY")

		createLogs := logFilesContaining(t, repo, "SLOT-CREATE")
		applyLogs := logFilesContaining(t, repo, "SLOT-APPLY")
		require.Equal(t, createLogs, applyLogs,
			"dag apply must write to the same log file as create under ARM_LOG_SLOT")
		require.Len(t, applyLogs, 1)
		assert.Contains(t, applyLogs[0], "~beta")
	})

	t.Run("with ARM_LOG_SLOT unset, dag apply writes the unslotted log like create", func(t *testing.T) {
		t.Setenv("ARM_LOG_SLOT", "")
		repo := plantDagApplyLogSlotRepo(t)

		_, err := runTrls(t, repo, "create",
			"--type", "task",
			"--title", "create path",
			"--id", "SLOT-CREATE")
		require.NoError(t, err)

		applyPlanIssue(t, repo, "SLOT-APPLY")

		createLogs := logFilesContaining(t, repo, "SLOT-CREATE")
		applyLogs := logFilesContaining(t, repo, "SLOT-APPLY")
		require.Equal(t, createLogs, applyLogs,
			"unset ARM_LOG_SLOT must keep dag apply on the same unslotted log as create")
		require.Len(t, applyLogs, 1)
		assert.NotContains(t, applyLogs[0], "~")
	})
}

func plantDagApplyLogSlotRepo(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	return repo
}

func applyPlanIssue(t *testing.T, repo, issueID string) {
	t.Helper()
	planData := `{"version":1,"title":"Log slot plan","issues":[{` +
		`"id":"` + issueID + `","title":"apply path","type":"task","source":"src-test",` +
		`"scope":"internal/` + issueID + `.go","dod":"Apply path is complete and tested",` +
		`"acceptance":[{"type":"test_passes"}]}]}`
	planFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte(planData), 0o644))
	_, err := runTrls(t, repo, "dag", "apply", "--plan", planFile)
	require.NoError(t, err)
}

func logFilesContaining(t *testing.T, repo, needle string) []string {
	t.Helper()
	opsDir := filepath.Join(repo, ".armature", "ops")
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(opsDir, e.Name()))
		require.NoError(t, err)
		if strings.Contains(string(data), needle) {
			names = append(names, e.Name())
		}
	}
	return names
}
