package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHarnessHookCommandIsRegistered(t *testing.T) {
	root := newRootCmd()

	cmd, _, err := root.Find([]string{"harness-hook"})

	require.NoError(t, err)
	assert.Equal(t, "harness-hook", cmd.Name())
}

func TestHarnessHookRequiresTaskID(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(`{}`))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ARMATURE_TASK_ID is required")
}

func TestHarnessHookBlocksOutOfScopeEdit(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	t.Setenv("ARMATURE_TASK_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"cmd/armature/main.go"}]}}`))
	cmd.SetOut(&out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = cmd.Execute()

	// Block decisions exit 0 with structured JSON; the platform reads the JSON decision.
	require.NoError(t, err)
	assert.Contains(t, out.String(), `"decision":"block"`)
	assert.Contains(t, out.String(), "outside task scope")
}

func TestHarnessHookAllowsInScopeEdit(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	t.Setenv("ARMATURE_TASK_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"internal/harnesshook/evaluator.go"}]}}`)) //nolint:lll
	cmd.SetOut(&out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), `"decision":"approve"`)
}

func TestHarnessHookBlocksStopWhenVerificationFails(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["human review only"]`)
	require.NoError(t, err)

	t.Setenv("ARMATURE_TASK_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"Stop","tool_name":"","tool_input":{}}`))
	cmd.SetOut(&out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = cmd.Execute()

	// Block decisions exit 0 with structured JSON; the platform reads the JSON decision.
	require.NoError(t, err)
	assert.Contains(t, out.String(), `"decision":"block"`)
	assert.Contains(t, out.String(), "unverifiable")
}

func TestAdapterExitError(t *testing.T) {
	// Test that adapterExitError implements error interface
	err := adapterExitError{code: 42}
	assert.NotNil(t, err)

	// Test Error() method
	errMsg := err.Error()
	assert.Contains(t, errMsg, "hook blocked with exit code 42")

	// Test errors.As
	var ace adapterExitError
	require.True(t, errors.As(err, &ace))
	assert.Equal(t, 42, ace.code)
}

// TestHarnessHookCmdSilencesErrors verifies that the harness-hook command
// has SilenceErrors: true to prevent cobra from printing error messages to stderr
// for adapterExitError returns.
func TestHarnessHookCmdSilencesErrors(t *testing.T) {
	cmd := newHarnessHookCmd()
	assert.True(t, cmd.SilenceErrors, "harness-hook command must have SilenceErrors: true to suppress cobra error output")
}

// TestApplyRunResult_PropagatesExitCode verifies that applyRunResult correctly
// converts a non-zero ExitCode into an adapterExitError that can be detected
// with errors.As, and that output is still written before returning the error.
func TestApplyRunResult_PropagatesExitCode(t *testing.T) {
	result := harnesshook.RunResult{
		Output:   []byte(`{"decision":"block"}`),
		ExitCode: 2,
	}
	var buf bytes.Buffer
	err := applyRunResult(&buf, result)
	require.Error(t, err)
	var ace adapterExitError
	require.True(t, errors.As(err, &ace), "error must be adapterExitError")
	assert.Equal(t, 2, ace.code)
	assert.Equal(t, `{"decision":"block"}`, buf.String())
}

// TestApplyRunResult_ZeroExitCode verifies that applyRunResult returns nil
// when ExitCode is 0, after writing output.
func TestApplyRunResult_ZeroExitCode(t *testing.T) {
	result := harnesshook.RunResult{
		Output:   []byte(`{"decision":"approve"}`),
		ExitCode: 0,
	}
	var buf bytes.Buffer
	err := applyRunResult(&buf, result)
	require.NoError(t, err)
	assert.Equal(t, `{"decision":"approve"}`, buf.String())
}
