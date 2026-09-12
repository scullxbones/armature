package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoctorModernRepoDoesNotLeakStateDirToCWD verifies that running `arm doctor`
// against a modern (dual-branch, bootstrapped) repo resolves appCtx.StateDir via
// root's PersistentPreRunE (delegation), rather than leaving it empty. Regression
// guard for a bug where doctor's own PersistentPreRunE skipped stateDirFor, causing
// internal/doctor's Materialize to write checkpoint.json/index.json/ready.json/
// traceability.json/issues/ as relative paths into the process's current working
// directory instead of the resolved state dir.
func TestDoctorModernRepoDoesNotLeakStateDirToCWD(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap into modern dual-branch layout.
	bootstrapBuf := new(bytes.Buffer)
	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetOut(bootstrapBuf)
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, bootstrapCmd.Execute())

	// Snapshot the test process's own cwd before running doctor, and confirm
	// doctor does not write any of the well-known materialize artifacts there.
	cwd, err := os.Getwd()
	require.NoError(t, err)

	stray := []string{"checkpoint.json", "index.json", "ready.json", "traceability.json", "issues"}
	for _, name := range stray {
		_ = os.Remove(filepath.Join(cwd, name)) //nolint:errcheck // best-effort pre-clean in case of prior failed run
	}
	t.Cleanup(func() {
		for _, name := range stray {
			_ = os.RemoveAll(filepath.Join(cwd, name)) //nolint:errcheck // best-effort cleanup
		}
	})

	doctorBuf := new(bytes.Buffer)
	doctorCmd := newRootCmd()
	doctorCmd.SetOut(doctorBuf)
	doctorCmd.SetArgs([]string{"doctor", "--repo", repo})
	// doctor may return a non-nil error if it finds warnings/errors in the fresh
	// repo; what matters here is that it doesn't write into the cwd.
	_ = doctorCmd.Execute() //nolint:errcheck // intentionally ignored: doctor may report warnings/errors, only cwd-leak matters here

	for _, name := range stray {
		assert.NoFileExists(t, filepath.Join(cwd, name), "doctor must not write %s into the test process cwd", name)
	}
}

// TestDoctorLegacyRepoEmitsDiagnostic verifies that when doctor falls back to the
// legacy single-branch layout detection path, it emits a clear diagnostic to
// stderr pointing the user at `arm bootstrap` to migrate.
func TestDoctorLegacyRepoEmitsDiagnostic(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Hand-construct a legacy single-branch .armature/ops layout (no dual-branch
	// worktree, no armature.ops-worktree-path git config), which root's
	// PersistentPreRunE / config.ResolveContext cannot resolve.
	legacyOps := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOps, 0o755))

	_, errOut, _ := runTrlsWithStderr(t, repo, "doctor") //nolint:errcheck // intentionally ignored: only stderr diagnostic matters here

	assert.Contains(t, errOut, "legacy single-branch layout detected")
	assert.Contains(t, errOut, "arm bootstrap")
}

// TestDoctorModernRepoUnknownConfigKeyDoesNotUseLegacyFallback verifies that a
// modern ops worktree with a strict-decode failure is not mistaken for legacy
// single-branch (empty WorktreePath). Live dual-branch repos have .armature/ops.
func TestDoctorModernRepoUnknownConfigKeyDoesNotUseLegacyFallback(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapBuf := new(bytes.Buffer)
	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetOut(bootstrapBuf)
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, bootstrapCmd.Execute())

	configPath := filepath.Join(repo, ".armature", "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"project_type":"go","mystery_knob":1}`), 0o600))

	out, errOut, err := runTrlsWithStderr(t, repo, "doctor")
	require.Error(t, err)
	assert.NotContains(t, errOut, "legacy single-branch layout detected")
	assert.Contains(t, out, "mystery_knob")
}

func TestDoctorFixReportsOutOfRangeConfig(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapBuf := new(bytes.Buffer)
	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetOut(bootstrapBuf)
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, bootstrapCmd.Execute())

	configPath := filepath.Join(repo, ".armature", "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"project_type": "go",
		"default_ttl": 60,
		"token_budget": -1,
		"low_stakes_push_threshold": 5,
		"hooks": []
	}`), 0o600))

	out, errOut, err := runTrlsWithStderr(t, repo, "doctor", "--fix", "--dry-run")
	require.Error(t, err)
	joined := out + errOut + err.Error()
	assert.Contains(t, joined, "D10")
	assert.Contains(t, joined, "token_budget")
}

func TestDoctorAgentEnvelope_AOC_REQ_TOPTIER_S15_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	agentOut, err := runTrls(t, repo, "doctor", "--format", "agent")
	require.NoError(t, err)
	agentPayload := assertSingleJSONObject(t, agentOut)
	assertDoctorAOCEnvelope(t, agentPayload)

	jsonOut, err := runTrls(t, repo, "doctor", "--format", "json")
	require.NoError(t, err)
	jsonPayload := assertSingleJSONObject(t, jsonOut)
	assertDoctorAOCEnvelope(t, jsonPayload)

	assertNoExplainFields(t, agentPayload)
	assertNoExplainFields(t, jsonPayload)

	golden, err := os.ReadFile(filepath.Join(output.DefaultGoldenDir(), "doctor.json"))
	require.NoError(t, err)
	var goldenPayload map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(golden), &goldenPayload))
	assertDoctorAOCEnvelope(t, goldenPayload)
	_, hasFindings := goldenPayload["findings"]
	assert.False(t, hasFindings, "golden must use checks[], not the stub findings[] payload")
}

func TestDoctorExplainFlag_RendersGuidedNarrative_REQ_TOPTIER_S15_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	plain, err := runTrls(t, repo, "doctor", "--format", "human")
	require.NoError(t, err)
	assert.NotContains(t, plain, "Suggested:")
	assert.NotContains(t, plain, "Explanation:")

	out, err := runTrls(t, repo, "doctor", "--explain", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "D6")
	assert.Contains(t, out, "Explanation:")
	assert.Contains(t, out, "Suggested:")
	assert.Contains(t, out, "arm sources")
}

func TestDoctorExplainFlag_AgentFormatAddsStructuredFields_REQ_TOPTIER_S15_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "doctor", "--explain", "--format", "agent")
	require.NoError(t, err)
	payload := assertSingleJSONObject(t, out)
	assertDoctorAOCEnvelope(t, payload)
	_, hasError := payload["error"]
	assert.False(t, hasError, "doctor checks must not be presented as a Command Failure")

	checks, ok := payload["checks"].([]any)
	require.True(t, ok)
	var sawNonOK bool
	for _, raw := range checks {
		row, ok := raw.(map[string]any)
		require.True(t, ok)
		sev, _ := row["severity"].(string)
		if sev == "ok" {
			_, hasExplanation := row["explanation"]
			_, hasSuggested := row["suggested"]
			assert.False(t, hasExplanation, "OK checks must omit explanation")
			assert.False(t, hasSuggested, "OK checks must omit suggested")
			continue
		}
		sawNonOK = true
		explanation, _ := row["explanation"].(string)
		suggested, _ := row["suggested"].(string)
		assert.NotEmpty(t, explanation, "non-OK check %v must carry explanation", row["check"])
		assert.NotEmpty(t, suggested, "non-OK check %v must carry suggested", row["check"])
	}
	require.True(t, sawNonOK, "fixture repo must include at least one non-OK doctor check")

	jsonOut, err := runTrls(t, repo, "doctor", "--explain", "--format", "json")
	require.NoError(t, err)
	jsonPayload := assertSingleJSONObject(t, jsonOut)
	assertDoctorAOCEnvelope(t, jsonPayload)
}

func assertDoctorAOCEnvelope(t *testing.T, payload map[string]any) {
	t.Helper()
	_, hasCount := payload["count"]
	_, hasChecks := payload["checks"]
	_, hasHelp := payload["help"]
	_, hasError := payload["error"]
	assert.True(t, hasCount, "AOC envelope must include count")
	assert.True(t, hasChecks, "AOC envelope payload key must be checks")
	assert.True(t, hasHelp, "AOC envelope must include help")
	assert.False(t, hasError, "doctor report must not be a Command Failure")
	help, ok := payload["help"].([]any)
	require.True(t, ok, "help must be an array")
	require.NotEmpty(t, help, "help must not be empty")
	checks, ok := payload["checks"].([]any)
	require.True(t, ok, "checks must be an array")
	count, ok := payload["count"].(float64)
	require.True(t, ok, "count must be a number")
	assert.Equal(t, float64(len(checks)), count)
}

func assertNoExplainFields(t *testing.T, payload map[string]any) {
	t.Helper()
	checks, ok := payload["checks"].([]any)
	require.True(t, ok)
	for _, raw := range checks {
		row, ok := raw.(map[string]any)
		require.True(t, ok)
		_, hasExplanation := row["explanation"]
		_, hasSuggested := row["suggested"]
		assert.False(t, hasExplanation, "without --explain, Finding fields stay as today (no explanation)")
		assert.False(t, hasSuggested, "without --explain, Finding fields stay as today (no suggested)")
	}
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"explanation"`)
	assert.NotContains(t, string(encoded), `"suggested"`)
}
