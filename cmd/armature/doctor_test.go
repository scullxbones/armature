package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorModernRepoDoesNotLeakStateDirToCWD(t *testing.T) {
	t.Parallel()
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapBuf := new(bytes.Buffer)
	bootstrapCmd := testRootCmd(t)
	bootstrapCmd.SetOut(bootstrapBuf)
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, bootstrapCmd.Execute())

	cwd, err := os.Getwd()
	require.NoError(t, err)

	stray := []string{"checkpoint.json", "index.json", "ready.json", "traceability.json", "issues"}
	for _, name := range stray {
		swallowErr(os.Remove(filepath.Join(cwd, name)))
	}
	t.Cleanup(func() {
		for _, name := range stray {
			swallowErr(os.RemoveAll(filepath.Join(cwd, name)))
		}
	})

	doctorBuf := new(bytes.Buffer)
	doctorCmd := testRootCmd(t)
	doctorCmd.SetOut(doctorBuf)
	doctorCmd.SetArgs([]string{"doctor", "--repo", repo})
	swallowErr(doctorCmd.Execute())

	for _, name := range stray {
		assert.NoFileExists(t, filepath.Join(cwd, name), "doctor must not write %s into the test process cwd", name)
	}
}

func TestDoctorLegacyRepoEmitsDiagnostic(t *testing.T) {
	t.Parallel()
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOps := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOps, 0o755))

	_, errOut, err := runTrlsWithStderr(t, repo, "doctor")
	swallowErr(err)

	assert.Contains(t, errOut, "legacy single-branch layout detected")
	assert.Contains(t, errOut, "arm bootstrap")
}

func TestDoctorModernRepoUnknownConfigKeyDoesNotUseLegacyFallback(t *testing.T) {
	t.Parallel()
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapBuf := new(bytes.Buffer)
	bootstrapCmd := testRootCmd(t)
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
	t.Parallel()
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapBuf := new(bytes.Buffer)
	bootstrapCmd := testRootCmd(t)
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
	t.Parallel()
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

func TestDoctorCheckGuidance_SuggestedRemediations_REQ_TOPTIER_S15_T1(t *testing.T) {
	t.Parallel()

	_, d4 := doctorCheckGuidance("D4")
	assert.Equal(t, "arm reparent --issue <child-id> --parent <parent-id>", d4,
		"D4 must use required --issue/--parent flags; positionals are ignored")

	_, d7 := doctorCheckGuidance("D7")
	assert.Equal(t, "mv .armature/ops/<expected>.log .armature/ops/<got>.log", d7,
		"D7 is a filename/worker_id mismatch; worker-init --check does not repair it")
	assert.NotContains(t, d7, "worker-init --check")

	_, d9 := doctorCheckGuidance("D9")
	assert.Equal(t, "arm claim --issue <issue-id> --worktree <path>; or git worktree remove --force <path>", d9,
		"D9 unrecognized paths are not GCRemovals; bind or remove the reported path")
	assert.NotContains(t, d9, "worktree gc")

	_, d11 := doctorCheckGuidance("D11")
	assert.Equal(t, "arm push-ops", d11)

	_, d12 := doctorCheckGuidance("D12")
	assert.Contains(t, d12, "origin/_armature")

	_, d14 := doctorCheckGuidance("D14")
	assert.Equal(t, d14SuggestedFallback, d14,
		"shape-agnostic D14 must not blindly recommend arm merged")
	assert.NotContains(t, d14, "arm merged --issue <issue-id>")
}

func TestDoctorCheckGuidance_D14LeafVsNonLeaf_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()

	index := materialize.Index{
		"P":  {Status: "merged", Type: "epic", Children: []string{"R", "L"}},
		"R":  {Status: "done", Type: "story", Parent: "P", Children: []string{"RC"}},
		"RC": {Status: "done", Type: "task", Parent: "R"},
		"L":  {Status: "done", Type: "task", Parent: "P"},
	}

	leafFinding := doctor.Finding{
		Check: "D14", Severity: doctor.SeverityWarning,
		Items: []string{"L -> P"},
	}
	_, leafSug := doctorCheckGuidanceFor(leafFinding, index)
	assert.Equal(t, "arm merged --issue L", leafSug)
	assert.NotContains(t, leafSug, "children first")

	nonLeafFinding := doctor.Finding{
		Check: "D14", Severity: doctor.SeverityWarning,
		Items: []string{"R -> P"},
	}
	_, nonLeafSug := doctorCheckGuidanceFor(nonLeafFinding, index)
	assert.Contains(t, nonLeafSug, "arm merged --issue RC")
	assert.Contains(t, nonLeafSug, "promote unmerged children first")
	assertNoExactArmMergedIssue(t, nonLeafSug, "R")

	mixedFinding := doctor.Finding{
		Check: "D14", Severity: doctor.SeverityWarning,
		Items: []string{"L -> P", "R -> P"},
	}
	_, mixedSug := doctorCheckGuidanceFor(mixedFinding, index)
	assert.Contains(t, mixedSug, "arm merged --issue L")
	assert.Contains(t, mixedSug, "arm merged --issue RC")
	assertNoExactArmMergedIssue(t, mixedSug, "R")
}

func assertNoExactArmMergedIssue(t *testing.T, suggested, issueID string) {
	t.Helper()
	needle := "arm merged --issue " + issueID
	for _, part := range strings.Split(suggested, ";") {
		part = strings.TrimSpace(part)
		if after, ok := strings.CutPrefix(part, "promote unmerged children first: "); ok {
			part = after
		}
		assert.NotEqual(t, needle, part,
			"must not recommend %q in %q", needle, suggested)
	}
}

func TestDoctorCheckGuidance_CoversEveryLiveCheck_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()

	defaultExpl, _ := doctorCheckGuidance("D999")
	for _, id := range doctor.LiveCheckIDs() {
		expl, sug := doctorCheckGuidance(id)
		assert.NotEqual(t, defaultExpl, expl,
			"%s must have dedicated --explain guidance (not the default fallback)", id)
		assert.NotEmpty(t, sug, "%s suggested remediation must be non-empty", id)
	}
}

func TestDoctorExplain_D14NonLeafPointsAtChildren_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithParentAndTask(t) // story-01 + task-01
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)

	// Grandparent epic P (merged) → story-01 (done, non-leaf) → task-01 (done leaf).
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "epic-P", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{Title: "Parent epic", NodeType: "epic"},
	}))
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpReparent, TargetID: "story-01", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{Parent: "epic-P"},
	}))
	for _, id := range []string{"task-01", "story-01"} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpTransition, TargetID: id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{To: ops.StatusDone, Outcome: "fixture done for D14 explain shape test"},
		}))
	}
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "epic-P", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{To: ops.StatusMerged},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	_ = executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"doctor", "--repo", repo, "--explain", "--format", "human")
	out := stdout.String()
	assert.Contains(t, out, "D14")
	assert.Contains(t, out, "story-01 -> epic-P")
	assert.Contains(t, out, "arm merged --issue task-01")
	var d14Suggested string
	inD14 := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "D14:") {
			inD14 = true
			continue
		}
		if inD14 {
			if after, ok := strings.CutPrefix(strings.TrimSpace(line), "Suggested: "); ok {
				d14Suggested = after
				break
			}
			if strings.HasPrefix(strings.TrimSpace(line), "✓") || strings.HasPrefix(strings.TrimSpace(line), "⚠") || strings.HasPrefix(strings.TrimSpace(line), "✗") {
				if !strings.Contains(line, "D14") {
					break
				}
			}
		}
	}
	require.NotEmpty(t, d14Suggested, "D14 --explain must emit Suggested; out=%s", out)
	assertNoExactArmMergedIssue(t, d14Suggested, "story-01")
}

func TestDoctorExitsWhenOpsWorktreeBehindOrigin(t *testing.T) {
	t.Parallel()
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	opsPath := filepath.Join(repo, ".armature")
	run(t, opsPath, "git", "push", "-u", "origin", "_armature")

	currentOut := new(bytes.Buffer)
	currentCode := executeThenHandleRootError(t, currentOut, new(bytes.Buffer), "doctor", "--repo", repo)
	assert.Equal(t, 0, currentCode, currentOut.String())
	assert.Contains(t, currentOut.String(), "D12")
	assert.Contains(t, currentOut.String(), "not behind origin/_armature")

	originClone := filepath.Join(t.TempDir(), "origin-clone")
	run(t, t.TempDir(), "git", "clone", fx.Origin, originClone)
	run(t, originClone, "git", "checkout", "_armature")
	run(t, originClone, "git", "config", "user.email", "test@test.com")
	run(t, originClone, "git", "config", "user.name", "Test")
	run(t, originClone, "git", "config", "commit.gpgsign", "false")
	run(t, originClone, "git", "commit", "--allow-empty", "-m", "remote ops ahead")
	run(t, originClone, "git", "push", "origin", "_armature")

	behindOut := new(bytes.Buffer)
	behindCode := executeThenHandleRootError(t, behindOut, new(bytes.Buffer), "doctor", "--repo", repo)
	assert.Equal(t, 1, behindCode, behindOut.String())
	assert.Contains(t, behindOut.String(), "D12")
	assert.Contains(t, behindOut.String(), "behind origin/_armature")
	assert.NotContains(t, behindOut.String(), "Could not fetch")
}

func TestDoctorExplainFlag_RendersGuidedNarrative_REQ_TOPTIER_S15_T1(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
		sev, ok := row["severity"].(string)
		require.True(t, ok, "severity must be a string")
		if sev == "ok" {
			_, hasExplanation := row["explanation"]
			_, hasSuggested := row["suggested"]
			assert.False(t, hasExplanation, "OK checks must omit explanation")
			assert.False(t, hasSuggested, "OK checks must omit suggested")
			continue
		}
		sawNonOK = true
		explanation, ok := row["explanation"].(string)
		require.True(t, ok, "non-OK check %v must carry explanation", row["check"])
		suggested, ok := row["suggested"].(string)
		require.True(t, ok, "non-OK check %v must carry suggested", row["check"])
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
