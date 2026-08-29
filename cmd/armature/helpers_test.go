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
	"time"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/ops"
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
	return executeRoot(root, args, stdout, stderr)
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
	assert.Contains(t, unknownErr.String(), "unknown hook", "git protocol is non-zero exit plus a stderr reason")

	writeFile(t, repo, ".armature/ops/test.log", "test ops content")
	run(t, repo, "git", "add", filepath.Join(".armature", "ops", "test.log"))

	refuseOut := new(bytes.Buffer)
	refuseErr := new(bytes.Buffer)
	code = executeThenHandleRootError(t, refuseOut, refuseErr, "hook", "run", "pre-commit", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	assert.Empty(t, refuseOut.String(), "pre-commit refusal must not write a Command Failure to stdout")
	assert.Contains(t, refuseErr.String(), "Refusing to commit .armature/ops/")
}

func TestHookPreRunAndArgsStayOnGitProtocol_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	argsOut := new(bytes.Buffer)
	argsErr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, argsOut, argsErr, "hook", "run", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	assert.NotContains(t, argsOut.String(), `"code":"GENERAL-1"`)
	assert.NotContains(t, argsOut.String(), "Error [GENERAL-1]")
	assert.Empty(t, argsOut.String(), "MinimumNArgs must not emit a Command Failure on stdout")
	assert.NotEmpty(t, argsErr.String(), "git protocol requires a stderr reason when args are missing")
	assert.NotContains(t, argsErr.String(), `"code":"GENERAL-1"`)

	notRepo := t.TempDir()
	preOut := new(bytes.Buffer)
	preErr := new(bytes.Buffer)
	code = executeThenHandleRootError(t, preOut, preErr, "hook", "run", "pre-commit", "--repo", notRepo, "--format", "json")
	assert.Equal(t, 1, code)
	assert.NotContains(t, preOut.String(), `"code":"GENERAL-1"`)
	assert.NotContains(t, preOut.String(), "Error [GENERAL-1]")
	assert.Empty(t, preOut.String(), "hook PersistentPreRunE failures must stay on the git protocol")
	assert.NotEmpty(t, preErr.String(), "git protocol requires a stderr reason when context resolution fails")
	assert.NotContains(t, preErr.String(), `"code":"GENERAL-1"`)

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 60},
	}))

	ioOut := new(bytes.Buffer)
	ioErr := new(bytes.Buffer)
	missing := filepath.Join(notRepo, "COMMIT_EDITMSG")
	code = executeThenHandleRootError(t, ioOut, ioErr, "hook", "run", "prepare-commit-msg", missing, "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	assert.Empty(t, ioOut.String(), "prepare-commit-msg IO must not emit a Command Failure on stdout")
	assert.Contains(t, ioErr.String(), "commit message file")
	assert.NotContains(t, ioErr.String(), `"code":"GENERAL-1"`)
}

func TestDoctorFixDoesNotConcatenateCommandFailure_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "stale-worker.log")
	staleClaim := time.Now().Add(-2 * time.Hour).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "fixconcat-01", Timestamp: staleClaim, WorkerID: "stale-worker",
			Payload: ops.Payload{Title: "Doctor fix concat test", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "fixconcat-01", Timestamp: staleClaim, WorkerID: "stale-worker",
			Payload: ops.Payload{TTL: 5}},
	}))
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)
	for _, e := range entries {
		require.NoError(t, os.Chmod(filepath.Join(opsDir, e.Name()), 0o444))
	}
	require.NoError(t, os.Chmod(opsDir, 0o555))
	t.Cleanup(func() {
		if chmodErr := os.Chmod(opsDir, 0o755); chmodErr != nil {
			t.Logf("restore ops dir perms: %v", chmodErr)
		}
		restored, readErr := os.ReadDir(opsDir)
		if readErr != nil {
			t.Logf("readdir ops after restore: %v", readErr)
			return
		}
		for _, e := range restored {
			p := filepath.Join(opsDir, e.Name())
			mode := os.FileMode(0o644)
			if e.IsDir() {
				mode = 0o755
			}
			if chmodErr := os.Chmod(p, mode); chmodErr != nil {
				t.Logf("restore perms %s: %v", p, chmodErr)
			}
			if !e.IsDir() {
				continue
			}
			children, childErr := os.ReadDir(p)
			if childErr != nil {
				t.Logf("readdir %s: %v", p, childErr)
				continue
			}
			for _, child := range children {
				cp := filepath.Join(p, child.Name())
				if chmodErr := os.Chmod(cp, 0o644); chmodErr != nil {
					t.Logf("restore perms %s: %v", cp, chmodErr)
				}
			}
		}
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, stderr, "doctor", "--fix", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	raw := strings.TrimSpace(stdout.String())
	require.True(t, json.Valid([]byte(raw)), "doctor --fix must emit exactly one JSON value on stdout, got %q", stdout.String())
	assert.NotContains(t, raw, `"code":"GENERAL-1"`)
	assert.NotContains(t, raw, `"error"`)
	assert.NotEmpty(t, stderr.String(), "apply failure after the plan is written must still have a stderr cause")
}

// TestWorktreeGCReportIsNotCommandFailure_REQ_LNGHZN_S6_T1 verifies that a
// nonzero `arm worktree gc --format=json` run keeps stdout as exactly one
// structured report. gc writes its result and then returns a nonzero error;
// appending a Command Failure object to that same stream would make stdout
// invalid JSON for the agent consumers this contract targets, so the gc exit
// must be classified as a protocol exit like the doctor/validate reports.
func TestWorktreeGCReportIsNotCommandFailure_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo, issueID := setupAmbiguousGCRepo(t)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, stderr, "worktree", "gc", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	payload := assertSingleJSONObject(t, stdout.String())
	_, hasError := payload["error"]
	assert.False(t, hasError, "a nonzero gc report must not be presented as a Command Failure")
	assert.Contains(t, payload, "ambiguous", "stdout must remain the gc report")
	assert.Contains(t, stderr.String(), "ambiguous", "the reason belongs on stderr")

	dryOut := new(bytes.Buffer)
	code = executeThenHandleRootError(t, dryOut, new(bytes.Buffer), "worktree", "gc", "--dry-run", "--repo", repo, "--format", "json")
	assert.Equal(t, 1, code)
	dryPayload := assertSingleJSONObject(t, dryOut.String())
	_, hasDryError := dryPayload["error"]
	assert.False(t, hasDryError, "a nonzero gc dry-run report must not be presented as a Command Failure")
	assert.Contains(t, dryPayload["ambiguous"], issueID)
}

// TestHookFlagParseErrorStaysOnGitProtocol_REQ_LNGHZN_S6_T1 verifies that a
// malformed `arm hook` invocation keeps the git-hook protocol. Cobra fails
// during flag parsing, before either PersistentPreRunE or the Args wrapper can
// classify the error, so the classification has to happen at the Execute seam.
func TestHookFlagParseErrorStaysOnGitProtocol_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, stderr, "hook", "run", "pre-commit", "--repo", repo, "--format", "json", "--bad-flag")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String(), "a flag error under `arm hook` must not put a Command Failure on git's stdout")
	assert.Contains(t, stderr.String(), "bad-flag", "the git protocol is a non-zero exit plus a stderr reason")
}

// TestHarnessHookEarlyErrorStaysOnPlatformProtocol_REQ_LNGHZN_S6_T1 verifies
// that a harness-hook failure raised before RunE can return an adapterExitError
// leaves stdout alone. stdout is reserved for the harness's platform-native
// decision (ADR 0020 §6), so an unexpected Command Failure object there can be
// rejected or misread by the invoking harness.
func TestHarnessHookEarlyErrorStaysOnPlatformProtocol_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	flagOut := new(bytes.Buffer)
	flagErr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, flagOut, flagErr, "harness-hook", "--repo", repo, "--format", "json", "--bad-flag")
	assert.Equal(t, 1, code)
	assert.Empty(t, flagOut.String(), "a flag error under harness-hook must not write to the platform's stdout")
	assert.Contains(t, flagErr.String(), "bad-flag")

	// A context-resolution failure in PersistentPreRunE is the same class.
	notRepo := t.TempDir()
	ctxOut := new(bytes.Buffer)
	ctxErr := new(bytes.Buffer)
	code = executeThenHandleRootError(t, ctxOut, ctxErr, "harness-hook", "--repo", notRepo, "--format", "json")
	assert.Equal(t, 1, code)
	assert.Empty(t, ctxOut.String(), "a context-resolution failure under harness-hook must not write to the platform's stdout")
	assert.NotEmpty(t, ctxErr.String(), "the reason belongs on stderr")
}

// TestParseErrorUsesImplicitAgentFormat_REQ_LNGHZN_S6_T1 verifies that a
// non-TTY invocation with no explicit --format still renders its Command
// Failure as the promised JSON object. Cobra returns before PersistentPreRunE
// runs autoDetectTTYPolicy, so the implicit-format decision must be applied at
// the Execute seam too, or agent consumers parsing stdout get a human line.
func TestParseErrorUsesImplicitAgentFormat_REQ_LNGHZN_S6_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer), "show", "--bad-flag", "--repo", repo)
	assert.Equal(t, 1, code)
	payload := assertSingleJSONObject(t, stdout.String())
	assert.Contains(t, payload, "error", "a parse failure must render the JSON failure object under the implicit agent format")
	assert.NotContains(t, stdout.String(), "Error [GENERAL-1]")
}

// TestPlatformProtocolSurvivesFlagBeforeSubcommand_REQ_LNGHZN_S6_T1 verifies the
// platform protocol still holds when the offending flag precedes the subtree
// token. Cobra's Find strips flags before matching a subcommand and skips the
// token after one that takes a value (or that it does not recognize), so
// `arm --bad-flag hook run pre-commit` never enters the hook subtree and
// ExecuteC hands back the root command — the parent-chain walk alone would
// classify it as an ordinary Command Failure and put a JSON object on the
// stdout git owns.
func TestPlatformProtocolSurvivesFlagBeforeSubcommand_REQ_LNGHZN_S6_T1(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"unknown flag before the git hook subtree", []string{"--bad-flag", "hook", "run", "pre-commit"}},
		{"unknown flag before harness-hook", []string{"--bad-flag", "harness-hook"}},
		{"value-taking flag swallows the subtree token", []string{"--repo", "hook", "run", "pre-commit"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			code := executeThenHandleRootError(t, stdout, stderr, tc.args...)
			assert.Equal(t, 1, code)
			assert.Empty(t, stdout.String(), "the platform owns this stdout; a Command Failure must not land on it")
			assert.NotEmpty(t, stderr.String(), "the platform protocol is a non-zero exit plus a stderr reason")
		})
	}
}
