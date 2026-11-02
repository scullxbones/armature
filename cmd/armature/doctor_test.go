package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/promotion"
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
	require.Len(t, d4, 1)
	assert.Equal(t, "arm reparent --issue <child-id> --parent <parent-id>", d4[0],
		"D4 must use required --issue/--parent flags; positionals are ignored")

	_, d7 := doctorCheckGuidance("D7")
	require.Len(t, d7, 1)
	assert.Equal(t, "mv .armature/ops/<expected>.log .armature/ops/<got>.log", d7[0],
		"D7 is a filename/worker_id mismatch; worker-init --check does not repair it")
	assert.NotContains(t, d7[0], "worker-init --check")

	_, d9 := doctorCheckGuidance("D9")
	require.Len(t, d9, 1)
	assert.Equal(t, "arm claim --issue <issue-id> --worktree <path>; or git worktree remove --force <path>", d9[0],
		"D9 unrecognized paths are not GCRemovals; bind or remove the reported path")
	assert.NotContains(t, d9[0], "worktree gc")

	_, d11 := doctorCheckGuidance("D11")
	require.Equal(t, []string{"arm push-ops"}, d11)

	_, d12 := doctorCheckGuidance("D12")
	require.Len(t, d12, 1)
	assert.Contains(t, d12[0], "origin/_armature")

	_, d14 := doctorCheckGuidance("D14")
	require.Equal(t, []string{promotion.FallbackGuidance}, d14,
		"shape-agnostic D14 must not blindly recommend arm merged")
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

func TestDoctorCLI_ReportsOpenUnderMergedAncestor_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo := setupMixedD14Tree(t)
	d14 := doctorD14Explain(t, repo)
	assert.Equal(t, "warning", d14.severity)
	assert.Contains(t, d14.items, "R -> E")
	assert.Contains(t, d14.items, "RC2 under merged E (open)",
		"cmd enrichD14TreeConsistency must fold TreeConsistencyViolations into D14")
}

func TestEnrichD14TreeConsistency_Messages_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// Done-only: TreeConsistencyViolations empty → enrich returns early; message stays done-only.
	doneOnly := &doctor.Report{Checks: []doctor.Finding{{
		Check: "D14", Severity: doctor.SeverityWarning,
		Message: "Done work under a merged ancestor",
		Items:   []string{"T -> P"},
	}}}
	enrichD14TreeConsistency(doneOnly, promotion.Graph{
		"P": {Status: ops.StatusMerged, Children: []string{"T"}},
		"T": {Status: ops.StatusDone, Parent: "P"},
	})
	assert.Equal(t, "Done work under a merged ancestor", doneOnly.Checks[0].Message)
	assert.NotContains(t, doneOnly.Checks[0].Message, "open/claimed")

	// Open under merged → enrich folds viol and sets the broad message.
	openUnder := &doctor.Report{Checks: []doctor.Finding{{
		Check: "D14", Severity: doctor.SeverityOK,
		Message: "No done issues under a merged parent",
	}}}
	enrichD14TreeConsistency(openUnder, promotion.Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"O"}},
		"O": {Status: ops.StatusOpen, Parent: "E"},
	})
	assert.Equal(t, "Done or open/claimed work under a merged ancestor", openUnder.Checks[0].Message)
	assert.Equal(t, doctor.SeverityWarning, openUnder.Checks[0].Severity)
	require.NotEmpty(t, openUnder.Checks[0].Items)
}

func TestDoctorCheckGuidance_D14FallbackAndTreeRepair_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	_, fallback := doctorCheckGuidance("D14")
	assert.Equal(t, []string{promotion.FallbackGuidance}, fallback)

	// Done under merged parent with rollup-eligible children: reopen only —
	// never cancel done (single Alternative renders without Option labels).
	allMerged := promotion.Graph{
		"P": {Status: "merged", Children: []string{"R"}},
		"R": {Status: "done", Parent: "P", Children: []string{"C"}},
		"C": {Status: "merged", Parent: "R"},
	}
	expl, sug := doctorCheckGuidanceFor(doctor.Finding{
		Check: "D14", Severity: doctor.SeverityWarning, Items: []string{"R -> P"},
	}, allMerged, promotion.PlanOpts{})
	assert.Equal(t, promotion.FindingHeaderPrefix+"R -> P", sug[0])
	assert.Equal(t, "arm reopen --issue P", sug[1])
	assert.NotContains(t, sug, "Option B:")
	assert.NotContains(t, sug, "arm transition --issue R --to cancelled --force")
	assert.NotContains(t, expl, "children are merged")

	// Done R under merged with open child: reopen only for R's finding.
	repair := promotion.Graph{
		"E": {Status: "merged", Children: []string{"R"}},
		"R": {Status: "done", Parent: "E", Children: []string{"O"}},
		"O": {Status: "open", Parent: "R"},
	}
	expl, sug = doctorCheckGuidanceFor(doctor.Finding{
		Check: "D14", Severity: doctor.SeverityWarning, Items: []string{"R -> E"},
	}, repair, promotion.PlanOpts{})
	assert.Contains(t, expl, promotion.TreeRepairGuidance)
	assert.Equal(t, promotion.FindingHeaderPrefix+"R -> E", sug[0])
	assert.Equal(t, "arm reopen --issue E", sug[1])
	assert.NotContains(t, sug, "Option B:")
	assert.NotContains(t, sug, "arm transition --issue R --to cancelled --force")
	assert.Contains(t, sug, "arm claim --issue O --worktree")
	assert.NotContains(t, sug, "arm claim --issue R")

	// Open O under merged: Option A reopen + Option B cancel O.
	_, sugO := doctorCheckGuidanceFor(doctor.Finding{
		Check: "D14", Severity: doctor.SeverityWarning, Items: []string{"O under merged E (open)"},
	}, repair, promotion.PlanOpts{})
	assert.Equal(t, "Option A:", sugO[1])
	assert.Equal(t, "arm reopen --issue E", sugO[2])
	assert.Equal(t, "Option B:", sugO[3])
	assert.Equal(t, "arm transition --issue O --to cancelled --force", sugO[4])
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
		suggested, ok := row["suggested"].([]any)
		require.True(t, ok, "non-OK check %v must carry suggested[]", row["check"])
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
