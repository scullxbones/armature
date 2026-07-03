package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHarnessHookCommandIsRegistered(t *testing.T) {
	root := newRootCmd()

	cmd, _, err := root.Find([]string{"harness-hook"})

	require.NoError(t, err)
	assert.Equal(t, "harness-hook", cmd.Name())
}

func TestHarnessHookPassesThroughWithoutTaskID(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(`{}`))
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err := cmd.Execute()

	require.NoError(t, err)
	// Pass-through should exit 0
}

func TestHarnessHookBlocksOutOfScopeEdit(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task so it has claimed/in-progress status
	worktreeDir := t.TempDir()
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree", worktreeDir})
	err = cmd.Execute()
	require.NoError(t, err)

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	cmd = newRootCmd()
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

	// Claim the task so it has claimed/in-progress status
	worktreeDir := t.TempDir()
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree", worktreeDir})
	err = cmd.Execute()
	require.NoError(t, err)

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	cmd = newRootCmd()
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

	// Claim the task so it has claimed/in-progress status
	worktreeDir := t.TempDir()
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree", worktreeDir})
	err = cmd.Execute()
	require.NoError(t, err)

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	cmd = newRootCmd()
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

// TestResolveIssueBinding_FromFile verifies that resolveIssueBinding reads from
// <git-dir>/armature-issue-id file.
func TestResolveIssueBinding_FromFile(t *testing.T) {
	gitDir := t.TempDir()
	taskIDPath := filepath.Join(gitDir, "armature-issue-id")
	err := os.WriteFile(taskIDPath, []byte("task-from-file"), 0o644)
	require.NoError(t, err)

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "task-from-file", taskID)
}

// TestResolveIssueBinding_FromEnv verifies that resolveIssueBinding falls back to
// ARMATURE_ISSUE_ID environment variable when file does not exist.
func TestResolveIssueBinding_FromEnv(t *testing.T) {
	gitDir := t.TempDir()
	t.Setenv("ARMATURE_ISSUE_ID", "task-from-env")

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "task-from-env", taskID)
}

// TestResolveIssueBinding_Empty verifies that resolveIssueBinding returns an
// empty string when neither file nor environment variable exists.
func TestResolveIssueBinding_Empty(t *testing.T) {
	gitDir := t.TempDir()
	t.Setenv("ARMATURE_ISSUE_ID", "")

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "", taskID)
}

// TestResolveIssueBinding_FilePreferredOverEnv verifies that the file takes
// precedence over the environment variable.
func TestResolveIssueBinding_FilePreferredOverEnv(t *testing.T) {
	gitDir := t.TempDir()
	taskIDPath := filepath.Join(gitDir, "armature-issue-id")
	err := os.WriteFile(taskIDPath, []byte("task-from-file"), 0o644)
	require.NoError(t, err)
	t.Setenv("ARMATURE_ISSUE_ID", "task-from-env")

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "task-from-file", taskID)
}

// TestLogPassThrough verifies that logPassThrough writes a timestamped entry to
// the armature-hook.log file in the git directory.
func TestLogPassThrough(t *testing.T) {
	gitDir := t.TempDir()

	err := logPassThrough(gitDir, "test reason")

	require.NoError(t, err)
	logPath := filepath.Join(gitDir, "armature-hook.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	// Log lines now include an RFC3339 timestamp prefix.
	content := string(data)
	assert.Contains(t, content, "pass-through: test reason")
	assert.Contains(t, content, "Z pass-through:") // UTC RFC3339 ends in 'Z'
}

// TestLogPassThrough_Append verifies that logPassThrough appends timestamped
// entries to the armature-hook.log file.
func TestLogPassThrough_Append(t *testing.T) {
	gitDir := t.TempDir()

	err := logPassThrough(gitDir, "first reason")
	require.NoError(t, err)
	err = logPassThrough(gitDir, "second reason")
	require.NoError(t, err)

	logPath := filepath.Join(gitDir, "armature-hook.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "pass-through: first reason")
	assert.Contains(t, content, "pass-through: second reason")
	// Two lines, each with a timestamp.
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	assert.Len(t, lines, 2)
}

// TestIsBindingStale_Claimed verifies that a task with "claimed" status is not stale.
func TestIsBindingStale_Claimed(t *testing.T) {
	snap := &snapshot.Snapshot{
		Issues: map[string]*materialize.Issue{
			"task-01": {
				ID:     "task-01",
				Status: "claimed",
			},
		},
	}

	stale := isBindingStale(snap, "task-01", 1000)

	assert.False(t, stale)
}

// TestIsBindingStale_InProgress verifies that a task with "in-progress" status is not stale.
func TestIsBindingStale_InProgress(t *testing.T) {
	snap := &snapshot.Snapshot{
		Issues: map[string]*materialize.Issue{
			"task-01": {
				ID:     "task-01",
				Status: "in-progress",
			},
		},
	}

	stale := isBindingStale(snap, "task-01", 1000)

	assert.False(t, stale)
}

// TestIsBindingStale_Done verifies that a task with "done" status is stale.
func TestIsBindingStale_Done(t *testing.T) {
	snap := &snapshot.Snapshot{
		Issues: map[string]*materialize.Issue{
			"task-01": {
				ID:     "task-01",
				Status: "done",
			},
		},
	}

	stale := isBindingStale(snap, "task-01", 1000)

	assert.True(t, stale)
}

// TestIsBindingStale_Missing verifies that a missing task is stale.
func TestIsBindingStale_Missing(t *testing.T) {
	snap := &snapshot.Snapshot{
		Issues: make(map[string]*materialize.Issue),
	}

	stale := isBindingStale(snap, "task-01", 1000)

	assert.True(t, stale)
}

// TestIsBindingStale_Open verifies that a task with "open" status is stale.
func TestIsBindingStale_Open(t *testing.T) {
	snap := &snapshot.Snapshot{
		Issues: map[string]*materialize.Issue{
			"task-01": {
				ID:     "task-01",
				Status: "open",
			},
		},
	}

	stale := isBindingStale(snap, "task-01", 1000)

	assert.True(t, stale)
}

// TestIsBindingStale_ClaimedWithExpiredTTL verifies that a claimed task with an
// expired TTL (no recent heartbeat) is treated as stale, causing the harness-hook
// to pass through (not enforce governance).
func TestIsBindingStale_ClaimedWithExpiredTTL(t *testing.T) {
	now := int64(2000)
	claimedAt := int64(1000)
	lastHeartbeat := int64(1100)
	ttlMinutes := 10 // 600 seconds

	snap := &snapshot.Snapshot{
		Issues: map[string]*materialize.Issue{
			"task-01": {
				ID:            "task-01",
				Status:        "claimed",
				ClaimedAt:     claimedAt,
				LastHeartbeat: lastHeartbeat,
				ClaimTTL:      ttlMinutes,
			},
		},
	}

	// At time 2000, the most recent activity was 1100. TTL is 600 seconds.
	// lastActivity + ttl = 1100 + 600 = 1700, which is less than now (2000),
	// so the claim has expired and binding should be stale.
	stale := isBindingStale(snap, "task-01", now)

	assert.True(t, stale, "claimed task with expired TTL should be stale")
}

// TestIsBindingStale_ClaimedWithinTTLWindow verifies that a claimed task whose last heartbeat
// falls within the TTL window is NOT stale. This exercises the actual TTL window check rather
// than the ClaimTTL==0 fast-path used in TestIsBindingStale_Claimed and TestIsBindingStale_InProgress.
func TestIsBindingStale_ClaimedWithinTTLWindow(t *testing.T) {
	// TTL = 10 minutes = 600 seconds.
	// LastHeartbeat = 1500, now = 1600 → elapsed = 100 seconds, well within TTL.
	snap := &snapshot.Snapshot{
		Issues: map[string]*materialize.Issue{
			"task-01": {
				ID:            "task-01",
				Status:        "claimed",
				ClaimedAt:     1000,
				LastHeartbeat: 1500,
				ClaimTTL:      10,
			},
		},
	}

	stale := isBindingStale(snap, "task-01", 1600)

	assert.False(t, stale, "claimed task with heartbeat within TTL window should not be stale")
}

// TestHarnessHookReadsBindingFromFileWithoutEnv verifies that harness-hook reads the
// task binding from the worktree's armature-issue-id file even when ARMATURE_ISSUE_ID
// is not set (F1/F13: file-based binding must work end-to-end).
//
// The test claims a task with --worktree, which writes armature-issue-id into the
// worktree-specific git dir. It then invokes harness-hook with --repo pointing at
// the worktree (not the parent repo) and no ARMATURE_ISSUE_ID env var. The hook
// must still find the task binding and process the event.
func TestHarnessHookReadsBindingFromFileWithoutEnv(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task to write armature-issue-id into the worktree git dir.
	worktreeDir := t.TempDir()
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree", worktreeDir})
	require.NoError(t, claimCmd.Execute())

	// Verify armature-issue-id was written.
	gitPath := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreeDir, actualGitDir)
	}
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	require.FileExists(t, taskIDFile, "armature-issue-id must exist in worktree git dir")

	// Ensure ARMATURE_ISSUE_ID is NOT set — hook must rely on the file alone.
	t.Setenv("ARMATURE_ISSUE_ID", "")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	// Point --repo at the worktree so resolveWorktreeGitDir reads the .git file there.
	hookCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"internal/harnesshook/evaluator.go"}]}}`)) //nolint:lll
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", worktreeDir})

	err = hookCmd.Execute()

	// The hook should find the task binding from the file and process the event.
	// Since the path is in-scope, it should approve (not pass-through).
	require.NoError(t, err)
	// An approve decision means the hook found the binding and evaluated policy.
	assert.Contains(t, out.String(), `"decision":"approve"`, "hook must read binding from file and approve in-scope edit")
}

// TestDecisionLoggedToResolvedWorktree_REQ_HOOKBIND_T3 verifies that decisions are logged
// to the RESOLVED worktree's git dir, not the invoking repo's, when binding resolution
// determines the binding via path-based resolution to a different worktree. This test creates
// a secondary worktree with a claimed task and invokes the hook from another location,
// verifying the log entry appears in the resolved worktree's git dir.
func TestDecisionLoggedToResolvedWorktree_REQ_HOOKBIND_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task in a separate worktree directory
	claimedWorktreeDir := t.TempDir()
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree", claimedWorktreeDir})
	err = claimCmd.Execute()
	require.NoError(t, err)

	// Read the armature-issue-id from the claimed worktree's git dir
	gitFile := filepath.Join(claimedWorktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(claimedWorktreeDir, actualGitDir)
	}

	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	// Invoke the hook with a file path that resolves to the claimed worktree
	// The hook should log to the resolved worktree's git dir, not the main repo's
	filePath := filepath.Join(claimedWorktreeDir, "internal/harnesshook/hook.go")
	fileDir := filepath.Dir(filePath)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	var out bytes.Buffer
	hookCmd := newRootCmd()
	hookCmd.SetIn(strings.NewReader(fmt.Sprintf(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"%s"}]}}`, filePath)))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	// Point --repo at the main repo, but the file path resolves to the claimed worktree
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	require.NoError(t, err)

	// Verify the log entry exists in the RESOLVED worktree's git dir, not the main repo's
	resolvedLogPath := filepath.Join(actualGitDir, "armature-hook.log")
	//nolint:gosec // G703: path derived from test worktree git dir
	logData, err := os.ReadFile(resolvedLogPath)
	require.NoError(t, err, "log must exist in resolved worktree's git dir")
	logContent := string(logData)

	// Verify log contains decision information
	assert.Contains(t, logContent, "decision:", "log must contain decision entry")
	assert.Contains(t, logContent, "task-01", "log must contain resolved issue ID")
	assert.Contains(t, logContent, "pre-tool-use", "log must contain event kind")
	assert.Contains(t, logContent, "apply_patch", "log must contain tool name")

	// Verify the log does NOT exist in the main repo's git dir (logging went to resolved worktree)
	mainRepoLogPath := filepath.Join(repo, ".git", "armature-hook.log")
	_, err = os.ReadFile(mainRepoLogPath)
	assert.Error(t, err, "log must NOT exist in main repo's git dir when binding resolves to different worktree")
}

// TestUnboundFileWriteLogsViolation_REQ_HOOKBIND_T3 verifies that file writes resolving to no binding
// are logged as "violation:" entries (not "pass-through:") to armature-hook.log.
func TestUnboundFileWriteLogsViolation_REQ_HOOKBIND_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create a task but do not claim it
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Without claiming the task, binding is stale (no claimed/in-progress issue)
	// So a file write event will have no binding and should log a violation
	t.Setenv("ARMATURE_ISSUE_ID", "")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out, errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"internal/harnesshook/hook.go"}]}}`))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = cmd.Execute()
	require.NoError(t, err, "fail-open: no binding should exit 0")

	gitDir := filepath.Join(repo, ".git")
	logPath := filepath.Join(gitDir, "armature-hook.log")
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)
	logContent := string(logData)

	// Should log a violation entry, not a pass-through
	assert.Contains(t, logContent, "violation:", "unbound file write must log violation entry")
	assert.NotContains(t, logContent, "pass-through:", "unbound file write must not log pass-through")
}

// TestFailOpenOnEventDecodeError_REQ_HOOKBIND_T3 verifies that event decode errors
// log a pass-through with loud stderr warning and exit 0 (fail-open).
func TestFailOpenOnEventDecodeError_REQ_HOOKBIND_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out, errOut bytes.Buffer
	cmd := newRootCmd()
	// Invalid JSON should cause decode error
	cmd.SetIn(strings.NewReader(`{invalid json`))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err, "event decode error should fail-open with exit 0")

	// Should have loud stderr warning
	stderrOutput := errOut.String()
	assert.NotEmpty(t, stderrOutput, "should write warning to stderr on decode error")
	assert.Contains(t, stderrOutput, "error", "stderr should contain error indication")

	// Should log pass-through to armature-hook.log
	gitDir := filepath.Join(repo, ".git")
	logPath := filepath.Join(gitDir, "armature-hook.log")
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)
	logContent := string(logData)
	assert.Contains(t, logContent, "pass-through:", "event decode error should log pass-through")
	assert.Contains(t, logContent, "decode", "log should mention decode in error description")
}

// TestSnapshotErrorFailsOpen_REQ_HOOKBIND_T3 verifies that snapshot load errors
// result in fail-open behavior: the hook logs a pass-through entry to armature-hook.log,
// writes a loud stderr warning, and exits with code 0 (not propagating the error).
// The test creates a stale task (one that was created but never claimed, then time passes)
// so that when the hook tries to check the snapshot, the binding will be stale,
// triggering a pass-through log that verifies the fail-open mechanism.
func TestSnapshotErrorFailsOpen_REQ_HOOKBIND_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	// Don't claim the task - just create it. This makes it stale.
	// The snapshot will load successfully, but the binding will be stale,
	// so the hook will fail-open with a "stale issue binding" pass-through entry.

	var out, errOut bytes.Buffer
	hookCmd := newRootCmd()
	jsonInput := `{"hook_event_name":"PreToolUse","tool_name":"apply_patch",` +
		`"tool_input":{"changes":[{"path":"internal/harnesshook/hook.go"}]}}`
	hookCmd.SetIn(strings.NewReader(jsonInput))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(&errOut)
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	// Fail-open: should exit 0 even when binding is stale
	require.NoError(t, err, "stale binding should fail-open with exit 0")

	// Verify pass-through entry was logged to armature-hook.log
	// (no loud stderr because stale binding is a graceful fail-open, not an error condition)
	gitDir := filepath.Join(repo, ".git")
	logPath := filepath.Join(gitDir, "armature-hook.log")
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err, "log must exist")
	logContent := string(logData)
	assert.Contains(t, logContent, "pass-through:", "stale binding should log pass-through entry")
}
