package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandFailureAgentEnvelope_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	cf := armerrors.New("GENERAL-1", "disk full", []string{"arm doctor"}, 1)
	buf := new(bytes.Buffer)
	renderCommandFailure(buf, "agent", cf)

	raw := strings.TrimSpace(buf.String())
	require.True(t, json.Valid([]byte(raw)), "envelope must be valid JSON: %q", raw)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
	_, hasCount := envelope["count"]
	_, hasPayload := envelope["payload"]
	_, hasHelp := envelope["help"]
	assert.False(t, hasCount, "failure must not nest in the AOC success envelope")
	assert.False(t, hasPayload, "failure must not nest in the AOC success envelope")
	assert.False(t, hasHelp, "failure must not nest in the AOC success envelope")

	errObj, ok := envelope["error"].(map[string]any)
	require.True(t, ok, "stdout object must be {error:{...}}")
	assert.Equal(t, "GENERAL-1", errObj["code"])
	assert.Equal(t, "disk full", errObj["cause"])
	assert.Equal(t, float64(1), errObj["exit_code"])
	actions, ok := errObj["next_actions"].([]any)
	require.True(t, ok, "next_actions must be a JSON array")
	require.Len(t, actions, 1)
	assert.Equal(t, "arm doctor", actions[0])

	// json ≡ agent
	jsonBuf := new(bytes.Buffer)
	renderCommandFailure(jsonBuf, "json", cf)
	assert.JSONEq(t, raw, strings.TrimSpace(jsonBuf.String()))

	// round-trip the inner CommandFailure documented by ADR 0020
	inner, err := json.Marshal(cf)
	require.NoError(t, err)
	var round armerrors.CommandFailure
	require.NoError(t, json.Unmarshal(inner, &round))
	assert.Equal(t, cf.Code, round.Code)
	assert.Equal(t, cf.Cause, round.Cause)
	assert.Equal(t, cf.NextActions, round.NextActions)
	assert.Equal(t, cf.ExitCode, round.ExitCode)
}

func TestCommandFailureHumanRendering_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	cf := armerrors.New("GENERAL-1", "disk full", []string{"arm doctor", "arm --help"}, 1)
	buf := new(bytes.Buffer)
	renderCommandFailure(buf, "human", cf)
	got := buf.String()
	assert.Equal(t, "Error [GENERAL-1]: disk full\nTry: arm doctor\nTry: arm --help\n", got)

	empty := armerrors.New("GENERAL-1", "boom", nil, 1)
	emptyBuf := new(bytes.Buffer)
	renderCommandFailure(emptyBuf, "human", empty)
	assert.Equal(t, "Error [GENERAL-1]: boom\n", emptyBuf.String())
	assert.NotContains(t, emptyBuf.String(), "Try:")
}

func TestRootSilenceErrors_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	assert.True(t, root.SilenceErrors, "root must SilenceErrors so cobra does not print a duplicate Error: line")
}

func TestClassifyErrorRemoved_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for _, name := range []string{"helpers.go", "main.go"} {
		src, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.NotContains(t, string(src), "func classifyError", "%s must not define classifyError", name)
		assert.NotContains(t, string(src), "func writeJSONError", "%s must not define writeJSONError", name)
		assert.NotContains(t, string(src), "jsonErrorPayload", "%s must not keep the stderr JSON error payload", name)
	}
}

func TestHandleRootErrorWritesAgentEnvelopeToStdout_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := handleRootError(stdout, stderr, "agent", true, fmt.Errorf("issue missing"))
	assert.Equal(t, 1, code)
	assert.NotContains(t, stdout.String(), `"count"`)
	assert.Contains(t, stdout.String(), `"code":"GENERAL-1"`)
	assert.Contains(t, stderr.String(), "DEBUG:")
	assert.NotContains(t, stderr.String(), `"error"`)
}

func executeThenHandleRootError(t *testing.T, stdout, stderr *bytes.Buffer, args ...string) int {
	t.Helper()
	runTrlsMu.Lock()
	defer runTrlsMu.Unlock()
	root := newRootCmd()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	err := root.Execute()
	format, _ := root.PersistentFlags().GetString("format")
	return handleRootError(stdout, stderr, format, false, err)
}

func assertSingleJSONObject(t *testing.T, stdout string) map[string]any {
	t.Helper()
	raw := strings.TrimSpace(stdout)
	require.True(t, json.Valid([]byte(raw)), "stdout must be exactly one JSON object, got %q", stdout)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	return payload
}

func TestValidateFailingReportIsNotCommandFailure_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	createOverlappingTask(t, repo, "tsk-a", "Implement ops overlap case")
	createOverlappingTask(t, repo, "tsk-b", "Implement sibling ops overlap")

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, stderr, "validate", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	payload := assertSingleJSONObject(t, stdout.String())
	_, hasError := payload["error"]
	assert.False(t, hasError, "graph findings must not be presented as a Command Failure")
	_, hasWarnings := payload["warnings"]
	assert.True(t, hasWarnings, "stdout must remain the validation report")

	humanOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, humanOut, new(bytes.Buffer), "validate", "--repo", repo, "--format", "agent")
	assert.Equal(t, 1, code)
	assert.Contains(t, humanOut.String(), "WARNING:")
	assert.NotContains(t, humanOut.String(), `"code":"GENERAL-1"`)
	assert.NotContains(t, humanOut.String(), "Error [GENERAL-1]")
}

func TestDoctorFailingReportIsNotCommandFailure_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	plantVerifiedTaskUnder(t, repo, "task-orphan", "src/orphan.go", "NO-SUCH-PARENT")

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer), "doctor", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	payload := assertSingleJSONObject(t, stdout.String())
	_, hasError := payload["error"]
	assert.False(t, hasError, "doctor checks must not be presented as a Command Failure")
	_, hasChecks := payload["checks"]
	assert.True(t, hasChecks, "stdout must remain the doctor report")
}

func TestBootstrapJSONFailureIsNotCommandFailure_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	writeFile(t, repo, "tracked.txt", "v1")
	run(t, repo, "git", "add", "tracked.txt")
	run(t, repo, "git", "commit", "-m", "add tracked")
	writeFile(t, repo, "tracked.txt", "dirty")

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer), "bootstrap", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	payload := assertSingleJSONObject(t, stdout.String())
	_, hasErrorEnvelope := payload["error"]
	assert.False(t, hasErrorEnvelope, "bootstrap JSON result must not be concatenated with a Command Failure")
	_, hasRepoSetup := payload["repo_setup"]
	assert.True(t, hasRepoSetup, "stdout must remain the BootstrapResult")
}

func TestHookErrorsSkipCommandFailure_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	unknownOut := new(bytes.Buffer)
	unknownErr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, unknownOut, unknownErr, "hook", "run", "unknown-hook", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	assert.Empty(t, unknownOut.String(), "arm hook stays on the git protocol; stdout must not get a Command Failure")
	assert.NotContains(t, unknownErr.String(), `"code":"GENERAL-1"`)

	writeFile(t, repo, ".armature/ops/test.log", "test ops content")
	run(t, repo, "git", "add", filepath.Join(".armature", "ops", "test.log"))

	refuseOut := new(bytes.Buffer)
	refuseErr := new(bytes.Buffer)
	code = executeThenHandleRootError(t, refuseOut, refuseErr, "hook", "run", "pre-commit", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	assert.Empty(t, refuseOut.String(), "pre-commit refusal must not write a Command Failure to stdout")
	assert.Contains(t, refuseErr.String(), "Refusing to commit .armature/ops/")
}
