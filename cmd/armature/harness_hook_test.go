package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/scullxbones/armature/internal/worktree"
	"github.com/spf13/cobra"
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
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
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
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
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
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
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
	assert.Equal(t, 42, err.code, "code should be preserved on the struct")

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

func TestHarnessHookIsSoleProtocolOutput_REQ_AOC_S1_T2(t *testing.T) {
	t.Parallel()

	cmd := newHarnessHookCmd()
	require.Equal(t, output.ChannelProtocolOutput, output.Classify(cmd.Annotations),
		"harness-hook must declare itself Protocol Output at the cobra constructor")

	root := newRootCmd()
	var protocol []string
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if output.Classify(c.Annotations) == output.ChannelProtocolOutput {
			protocol = append(protocol, c.Name())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	require.Equal(t, []string{"harness-hook"}, protocol,
		"harness-hook must be the sole Protocol Output command on the tree")
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

// TestResolveIssueBinding_FallsBackToLegacyTaskIDFile verifies that
// resolveIssueBinding falls back to the legacy armature-task-id file when
// armature-issue-id is absent, matching harnesshook.ResolveBindingFromDir's
// fallback (finding P2).
func TestResolveIssueBinding_FallsBackToLegacyTaskIDFile(t *testing.T) {
	gitDir := t.TempDir()
	taskIDPath := filepath.Join(gitDir, "armature-task-id")
	err := os.WriteFile(taskIDPath, []byte("legacy-task-id"), 0o644)
	require.NoError(t, err)

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "legacy-task-id", taskID)
}

// TestHarnessHookUntrustedPathResolvedGitDir_FallsBackToSessionBinding verifies
// that when path-based resolution (steps 1-2) lands on a git dir that is not a
// known worktree of the invoking repo, the hook actually falls back to the
// session binding (per ADR-0007 fail-open) rather than resetting to an empty,
// unbound binding — matching isKnownWorktreeGitDir's doc comment (finding P3).
func TestHarnessHookUntrustedPathResolvedGitDir_FallsBackToSessionBinding(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	// An unrelated git repo, entirely outside the invoking repo's worktrees,
	// bound to a different issue. A crafted tool_input.file_path pointing here
	// must not be trusted for binding resolution.
	attackerDir := t.TempDir()
	attackerGitDir := filepath.Join(attackerDir, ".git")
	require.NoError(t, os.MkdirAll(attackerGitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(attackerGitDir, "armature-issue-id"), []byte("attacker-task"), 0o644))
	attackerFile := filepath.Join(attackerDir, "some", "file.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(attackerFile), 0o755))

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out, errOut bytes.Buffer
	cmd = newRootCmd()
	payload := fmt.Sprintf(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":%q}]}}`, attackerFile)
	cmd.SetIn(strings.NewReader(payload))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, errOut.String(), "falling back to session binding")

	sessionGitDir := filepath.Join(repo, ".git")
	logData, readErr := os.ReadFile(filepath.Join(sessionGitDir, "armature-hook.log"))
	require.NoError(t, readErr)
	logContent := string(logData)
	assert.Contains(t, logContent, "rejected as untrusted", "should log a violation for the rejected path-resolved git dir")
	// The decision must be logged under the session's own binding (task-01),
	// not left unbound, proving the fallback actually restores the session binding.
	assert.Contains(t, logContent, "issue_id=task-01", "should evaluate under the session binding, not an empty binding")
}

// TestLogPassThrough verifies that logPassThrough writes a timestamped entry to
// the armature-hook.log file in the git directory.
// TestIsKnownWorktreeGitDir_SymlinkedWorktree_NotFalselyRejected verifies the
// fix for the review finding that isKnownWorktreeGitDir used plain
// filepath.Abs while `git worktree list --porcelain` emits symlink-resolved
// paths (matching isWorktreeOf's approach in claim.go, which already used
// EvalSymlinks). Without EvalSymlinks, a worktree created under a symlinked
// temp dir (common on macOS where TMPDIR is under /var -> /private/var, and
// possible anywhere a symlinked path is used) would never string-match the
// resolved path git reports, and would be falsely rejected as untrusted.
func TestIsKnownWorktreeGitDir_SymlinkedWorktree_NotFalselyRejected(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	realParent := t.TempDir()
	linkParent := filepath.Join(t.TempDir(), "symlinked-parent")
	require.NoError(t, os.Symlink(realParent, linkParent))

	worktreePath := filepath.Join(linkParent, "wt")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	actualGitDir, err := worktree.ResolveGitDir(worktreePath)
	require.NoError(t, err)

	assert.True(t, isKnownWorktreeGitDir(repo, actualGitDir),
		"a worktree reached through a symlinked path must still be recognized as a known worktree of repo")
}

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
	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
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
	// Point --repo at the worktree so worktree.ResolveGitDir reads the .git file there.
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
	claimedWorktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
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
	// Verify that the resolution_step is logged correctly (file_path in this case since we're using file path resolution)
	assert.Contains(t, logContent, "resolution_step=file_path", "log must contain the actual resolution step (file_path for path-based resolution)")

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
	// So a file write event will have no binding and should log a violation.
	// Use an absolute path inside the (unbound) repo so path-based resolution
	// finds the repo's git dir rather than depending on the test process cwd.
	t.Setenv("ARMATURE_ISSUE_ID", "")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	targetPath := filepath.Join(repo, "internal", "harnesshook", "hook.go")

	var out, errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(fmt.Sprintf(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"%s"}]}}`, targetPath)))
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

// TestStaleBindingPassThroughLogsScopeViolation_REQ_TOPTIER_S5_T2 drives a
// genuine "bound inactive" (stale) binding through the harness-hook
// command's pass-through decision path end-to-end: a task is claimed (so its
// worktree carries a real armature-issue-id binding file), then transitioned
// to "done" so isBindingStale reports it stale on the next hook invocation.
// An out-of-scope edit event is then sent through that stale binding.
//
// This is the case the conformance-matrix docstrings claim to cover ("bound
// inactive") but, before this test, none actually exercised: it asserts both
// that enforcement passes through (fail-open, exit 0, no block decision) AND
// that a "violation: ... on pass-through (stale binding)" entry is logged
// alongside the "pass-through: stale issue binding" entry, per
// docs/harness-hook.md's Scope Violation Visibility contract.
func TestStaleBindingPassThroughLogsScopeViolation_REQ_TOPTIER_S5_T2(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task so its worktree carries a real armature-issue-id binding.
	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	// Transition the task to "done" so isBindingStale reports the binding as
	// stale (status is no longer claimed/in-progress) while the worktree's
	// armature-issue-id file still points at it.
	cmd = newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs(enrichTestCLIArgs([]string{
		"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--skip-delivery-gate", "--outcome", "done", "--force",
	}))
	require.NoError(t, cmd.Execute())

	t.Setenv("ARMATURE_ISSUE_ID", "")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	// Out-of-scope path relative to the task's declared scope (internal/harnesshook/).
	outOfScopePath := filepath.Join(worktreeDir, "cmd", "armature", "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(outOfScopePath), 0o755))

	var out, errOut bytes.Buffer
	cmd = newRootCmd()
	payload := fmt.Sprintf(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":%q}]}}`, outOfScopePath)
	cmd.SetIn(strings.NewReader(payload))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"harness-hook", "--repo", worktreeDir})

	err = cmd.Execute()
	require.NoError(t, err, "stale binding must fail open: exit 0")
	assert.NotContains(t, out.String(), `"decision":"block"`, "stale binding must pass through, not block")

	worktreeGitDir, gErr := worktree.ResolveGitDir(worktreeDir)
	require.NoError(t, gErr)
	logData, err := os.ReadFile(filepath.Join(worktreeGitDir, "armature-hook.log"))
	require.NoError(t, err)
	logContent := string(logData)

	assert.Contains(t, logContent, "pass-through: stale issue binding", "stale binding must log a pass-through entry")
	assert.Contains(t, logContent, "violation:", "stale binding pass-through with an out-of-scope path must also log a violation entry")
	assert.Contains(t, logContent, "stale binding", "the violation entry must be attributed to the stale binding reason")
	assert.Contains(t, logContent, "cmd/armature/main.go", "the violation entry must name the out-of-scope path")
}

// TestStaleBindingPassThroughScopeViolation_RelativePathBelowRoot_REQ_TOPTIER_S5_T2
// verifies that the stale pass-through scope-violation logger normalizes a
// relative event path against event.Cwd before checking it against the
// task's declared scope. Without normalization, a relative path like
// "internal/foo.go" sent with cwd below the worktree root (e.g.
// "<root>/docs") textually matches an "internal/" scope entry even though
// the actual absolute write (<root>/docs/internal/foo.go) falls outside
// that scope — so the violation gap goes unlogged.
func TestStaleBindingPassThroughScopeViolation_RelativePathBelowRoot_REQ_TOPTIER_S5_T2(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	cmd = newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs(enrichTestCLIArgs([]string{
		"transition", "--repo", repo, "--issue", "task-01", "--to", "done",
		"--skip-delivery-gate", "--outcome", "done", "--force",
	}))
	require.NoError(t, cmd.Execute())

	t.Setenv("ARMATURE_ISSUE_ID", "")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	// cwd is a subdirectory of the worktree root; the relative edit path
	// "internal/foo.go" textually matches the "internal/" scope entry, but
	// the real absolute write (<worktreeDir>/docs/internal/foo.go) is
	// outside that scope.
	subCwd := filepath.Join(worktreeDir, "docs")
	require.NoError(t, os.MkdirAll(filepath.Join(subCwd, "internal"), 0o755))

	var out, errOut bytes.Buffer
	cmd = newRootCmd()
	payload := fmt.Sprintf(`{"hook_event_name":"PreToolUse","cwd":%q,"tool_name":"apply_patch","tool_input":{"changes":[{"path":"internal/foo.go"}]}}`, subCwd)
	cmd.SetIn(strings.NewReader(payload))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"harness-hook", "--repo", worktreeDir})

	err = cmd.Execute()
	require.NoError(t, err, "stale binding must fail open: exit 0")

	worktreeGitDir, gErr := worktree.ResolveGitDir(worktreeDir)
	require.NoError(t, gErr)
	logData, err := os.ReadFile(filepath.Join(worktreeGitDir, "armature-hook.log"))
	require.NoError(t, err)
	logContent := string(logData)

	assert.Contains(t, logContent, "pass-through: stale issue binding", "stale binding must log a pass-through entry")
	assert.Contains(t, logContent, "violation:",
		"stale binding pass-through with an out-of-scope path (once normalized against cwd) must also log a violation entry")
	assert.Contains(t, logContent, "docs/internal/foo.go",
		"the violation entry must name the normalized (cwd-joined) out-of-scope path, not the raw relative path")
}

// TestResolverErrorFailsOpen_REQ_HOOKBIND_T3 verifies that an error from
// hook.Evaluate's policy resolution step (e.g. a corrupt materialized issue JSON)
// fails open: exit 0, loud stderr warning, pass-through logged (finding 3).
func TestResolverErrorFailsOpen_REQ_HOOKBIND_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Materialize so issues/task-01.json exists to corrupt.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Corrupt the materialized issue JSON so resolver.Resolve fails to decode it.
	// State lives under .arm/state/<worker-id>/issues/; glob for the worker id.
	matches, err := filepath.Glob(filepath.Join(repo, ".armature", "state", "*", "issues", "task-01.json"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "materialized issue JSON must exist")
	for _, issuePath := range matches {
		err = os.WriteFile(issuePath, []byte("{not valid json"), 0o644)
		require.NoError(t, err)
	}

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out, errOut bytes.Buffer
	hookCmd := newRootCmd()
	jsonInput := `{"hook_event_name":"PreToolUse","tool_name":"apply_patch",` +
		`"tool_input":{"changes":[{"path":"internal/harnesshook/hook.go"}]}}`
	hookCmd.SetIn(strings.NewReader(jsonInput))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(&errOut)
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	require.NoError(t, err, "resolver error should fail-open with exit 0")

	stderrOutput := errOut.String()
	assert.Contains(t, stderrOutput, "error:", "stderr must contain error indication")

	gitDir := filepath.Join(repo, ".git")
	logPath := filepath.Join(gitDir, "armature-hook.log")
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err, "log must exist")
	assert.Contains(t, string(logData), "pass-through:", "resolver error should log pass-through entry")
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
// The test removes the .arm directory after setup to make snapshot loading fail,
// then verifies the hook fails open with stderr warning and pass-through log entry.
func TestSnapshotErrorFailsOpen_REQ_HOOKBIND_T3(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod 0o000 has no effect for the root user; cannot exercise unreadable-dir fail-open this way")
	}
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task so binding is valid
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	// Make the ops directory unreadable to force snapshot load to fail.
	// This makes os.ReadDir fail when ListLogFiles is called.
	opsDir := filepath.Join(repo, ".armature", "ops")
	require.DirExists(t, opsDir, "opsDir should exist")

	// Make the directory unreadable by removing all permissions
	err = os.Chmod(opsDir, 0o000)
	require.NoError(t, err, "should be able to chmod ops directory")
	t.Cleanup(func() {
		// Restore permissions for cleanup
		_ = os.Chmod(opsDir, 0o755) //nolint:errcheck // cleanup code
	})

	var out, errOut bytes.Buffer
	hookCmd := newRootCmd()
	jsonInput := `{"hook_event_name":"PreToolUse","tool_name":"apply_patch",` +
		`"tool_input":{"changes":[{"path":"internal/harnesshook/hook.go"}]}}`
	hookCmd.SetIn(strings.NewReader(jsonInput))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(&errOut)
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	// Fail-open: should exit 0 even when snapshot load fails
	require.NoError(t, err, "snapshot load error should fail-open with exit 0")

	// Verify stderr contains loud warning about snapshot load failure
	stderrOutput := errOut.String()
	assert.Contains(t, stderrOutput, "error:", "stderr must contain error indication")
	assert.Contains(t, stderrOutput, "snapshot", "stderr must mention snapshot")

	// Verify pass-through entry was logged to armature-hook.log
	gitDir := filepath.Join(repo, ".git")
	logPath := filepath.Join(gitDir, "armature-hook.log")
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err, "log must exist")
	logContent := string(logData)
	assert.Contains(t, logContent, "pass-through:", "snapshot load error should log pass-through entry")
	assert.Contains(t, logContent, "snapshot load failed", "log should describe the failure reason")
}

// TestHarnessHookCapturesActivityForBashPostToolUse_REQ_EXECEV_T1 verifies that
// Bash PostToolUse events with a bound issue append entries to the activity log
// with all required fields: command, exit code, output (truncated), output hash, HEAD sha, timestamp.
func TestHarnessHookCapturesActivityForBashPostToolUse_REQ_EXECEV_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task so it has claimed/in-progress status
	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Get the worktree git dir to check activity log location
	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreeDir, actualGitDir)
	}

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	// PostToolUse event with Bash tool, exit code 0, and output
	payload := `{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"echo test"},"tool_response":{"exit_code":0,"output":"test output\n"}}`
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", worktreeDir})

	err = hookCmd.Execute()
	require.NoError(t, err)

	// Verify activity log was created and contains the entry
	activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
	activityData, err := os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
	require.NoError(t, err, "activity log must be created for Bash PostToolUse")

	activityContent := string(activityData)
	assert.Contains(t, activityContent, `"command"`, "activity entry must contain command")
	assert.Contains(t, activityContent, `"exit_code":0`, "activity entry must contain exit code")
	assert.Contains(t, activityContent, `"exit_code_known":true`, "activity entry must record that the exit code is known")
	assert.Contains(t, activityContent, `"output_hash"`, "activity entry must contain output hash")
	assert.Contains(t, activityContent, `"head_sha"`, "activity entry must contain HEAD sha")
	assert.Regexp(t, `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`, activityContent, "activity entry must contain RFC3339 timestamp")
}

// TestHarnessHookCapturesActivityForNonBashShellTools_REQ_EXECEV_T2 verifies that
// PostToolUse events from platforms whose shell tool isn't literally named "Bash"
// (Codex's "shell"/"local_shell", Devin's "exec") still get captured to the
// activity log, using each platform's SupportedShellTools capability rather than
// a hardcoded "Bash" tool-name check.
func TestHarnessHookCapturesActivityForNonBashShellTools_REQ_EXECEV_T2(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		tool     string
	}{
		{name: "devin exec", platform: "devin", tool: "exec"},
		{name: "codex shell", platform: "codex", tool: "shell"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupRepoWithTask(t)
			_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
			require.NoError(t, err)

			worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
			cmd := newRootCmd()
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
			err = cmd.Execute()
			require.NoError(t, err)

			gitFile := filepath.Join(worktreeDir, ".git")
			gitFileContent, err := os.ReadFile(gitFile)
			require.NoError(t, err)
			actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
			if !filepath.IsAbs(actualGitDir) {
				actualGitDir = filepath.Join(worktreeDir, actualGitDir)
			}

			t.Setenv("ARMATURE_ISSUE_ID", "task-01")
			t.Setenv("ARMATURE_HOOK_PLATFORM", tc.platform)

			var out bytes.Buffer
			hookCmd := newRootCmd()
			payload := fmt.Sprintf(
				`{"hook_event_name":"PostToolUse","tool_name":%q,"tool_input":{"command":"echo test"},"tool_response":{"exit_code":0,"output":"test output\n"}}`,
				tc.tool)
			hookCmd.SetIn(strings.NewReader(payload))
			hookCmd.SetOut(&out)
			hookCmd.SetErr(new(bytes.Buffer))
			hookCmd.SetArgs([]string{"harness-hook", "--repo", worktreeDir})

			err = hookCmd.Execute()
			require.NoError(t, err)

			activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
			activityData, err := os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
			require.NoError(t, err, "activity log must be created for %s PostToolUse on platform %s", tc.tool, tc.platform)
			assert.Contains(t, string(activityData), `"command"`, "activity entry must contain command")
		})
	}
}

// TestHarnessHookActivityLogTruncatesLargeOutput_REQ_EXECEV_T1 verifies that
// activity log entries truncate large output to head+tail format with a marker and hash.
func TestHarnessHookActivityLogTruncatesLargeOutput_REQ_EXECEV_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task
	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Get the worktree git dir
	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreeDir, actualGitDir)
	}

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	// Create large output (over 2KB to trigger truncation)
	largeOutput := strings.Repeat("x", 3000)

	var out bytes.Buffer
	hookCmd := newRootCmd()
	payload := fmt.Sprintf(
		`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"echo test"},"tool_response":{"exit_code":0,"output":%q}}`,
		largeOutput)
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", worktreeDir})

	err = hookCmd.Execute()
	require.NoError(t, err)

	// Verify activity log contains truncation info
	activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
	activityData, err := os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
	require.NoError(t, err)

	activityContent := string(activityData)
	// For truncated output, should mention truncation or show both head and tail
	assert.Contains(t, activityContent, `"output_hash"`, "activity entry must contain output hash for verification")
}

// TestHarnessHookActivityKillSwitchDisablesCapture_REQ_EXECEV_T1 verifies that
// activity logging can be disabled via the repo-level git config kill-switch
// (armature.disable-activity-logging). There is deliberately no environment
// variable override (M8): an env var would be settable by the worker process
// mid-session, letting it curate failure-then-success sequences out of the log.
func TestHarnessHookActivityKillSwitchDisablesCapture_REQ_EXECEV_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task
	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Get the worktree git dir
	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreeDir, actualGitDir)
	}

	// Enable the repo-level kill-switch (shared --local config store for all worktrees).
	killSwitchCmd := exec.CommandContext(context.Background(), "git", "--git-dir="+actualGitDir,
		"config", "--local", "--bool", "armature.disable-activity-logging", "true")
	require.NoError(t, killSwitchCmd.Run())

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	payload := `{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"echo test"},"tool_response":{"exit_code":0,"output":"test output\n"}}`
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	require.NoError(t, err)

	// Verify activity log was NOT created when kill-switch is enabled
	activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
	_, err = os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
	assert.Error(t, err, "activity log should not exist when the repo-level kill-switch is set")
	assert.True(t, os.IsNotExist(err), "activity log should not exist (not exist error)")
}

// TestHarnessHookActivityFailOpenOnError_REQ_EXECEV_T1 verifies that
// activity capture errors fail open: the hook logs to stderr but doesn't block the operation.
// This test creates a read-only git directory to force a write error.
func TestHarnessHookActivityFailOpenOnError_REQ_EXECEV_T1(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod 0o000 has no effect for the root user; cannot exercise write-error fail-open this way")
	}

	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task
	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Get the worktree git dir
	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreeDir, actualGitDir)
	}

	// Make the git dir read-only to force activity write to fail
	err = os.Chmod(actualGitDir, 0o500) //nolint:gosec // G703: test code to make directory read-only
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chmod(actualGitDir, 0o755) //nolint:errcheck,gosec // cleanup code
	})

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out, errOut bytes.Buffer
	hookCmd := newRootCmd()
	payload := `{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"echo test"},"tool_response":{"exit_code":0,"output":"test output\n"}}`
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(&errOut)
	hookCmd.SetArgs([]string{"harness-hook", "--repo", worktreeDir})

	err = hookCmd.Execute()
	// Fail-open: hook should exit 0 even when activity logging fails
	require.NoError(t, err, "activity logging error should fail-open with exit 0")

	// The activity logging will fail silently (fail-open), so we just verify
	// that the hook completes successfully despite the write error.
	// (The warning goes to os.Stderr directly, which isn't captured by the
	// command's SetErr, so we don't check for it here.)
}

// TestHarnessHookNoActivityForNonBashTools_REQ_EXECEV_T1 verifies that
// activity logging only happens for Bash PostToolUse events, not for other tools.
func TestHarnessHookNoActivityForNonBashTools_REQ_EXECEV_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task
	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Get the worktree git dir
	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreeDir, actualGitDir)
	}

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	// PostToolUse event with Edit tool (not Bash), should NOT log activity
	payload := `{"hook_event_name":"PostToolUse","tool_name":"Edit",` +
		`"tool_input":{"file_path":"internal/harnesshook/hook.go"},"tool_response":{"exit_code":0,"output":""}}`
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	require.NoError(t, err)

	// Verify activity log was NOT created for non-Bash tools
	activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
	_, err = os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
	assert.Error(t, err, "activity log should not exist for non-Bash PostToolUse events")
	assert.True(t, os.IsNotExist(err), "activity log should not exist (not exist error)")
}

// TestHarnessHookNoActivityForPreToolUse_REQ_EXECEV_T1 verifies that
// activity logging only happens for PostToolUse events, not PreToolUse.
func TestHarnessHookNoActivityForPreToolUse_REQ_EXECEV_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	// Claim the task
	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Get the worktree git dir
	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreeDir, actualGitDir)
	}

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	// PreToolUse event with Bash tool, should NOT log activity
	payload := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"echo test"}}`
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	require.NoError(t, err)

	// Verify activity log was NOT created for PreToolUse events
	activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
	_, err = os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
	assert.Error(t, err, "activity log should not exist for PreToolUse events")
	assert.True(t, os.IsNotExist(err), "activity log should not exist (not exist error)")
}

func TestHeartbeatRateLimitStateRoundTrip(t *testing.T) {
	workerID := fmt.Sprintf("test-worker-%d", time.Now().UnixNano())
	issueID := "task-round-trip"
	defer os.Remove(rateLimitStateFilePath(workerID, issueID)) //nolint:errcheck // best-effort cleanup

	// No state file yet: zero time.
	assert.True(t, readHeartbeatRateLimitState(workerID, issueID).IsZero())

	now := time.Now().Truncate(time.Second)
	require.NoError(t, writeHeartbeatRateLimitState(workerID, issueID, now))

	got := readHeartbeatRateLimitState(workerID, issueID)
	assert.Equal(t, now.Unix(), got.Unix())
}

func TestHeartbeatRateLimitStateReadMalformedFileReturnsZero(t *testing.T) {
	workerID := fmt.Sprintf("test-worker-%d", time.Now().UnixNano())
	issueID := "task-malformed"
	stateFile := rateLimitStateFilePath(workerID, issueID)
	defer os.Remove(stateFile) //nolint:errcheck // best-effort cleanup

	require.NoError(t, os.WriteFile(stateFile, []byte("not json"), 0o600))

	assert.True(t, readHeartbeatRateLimitState(workerID, issueID).IsZero())
}

func TestTryEmitHeartbeatFailsOpenWhenWorkerUnset(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	armatureDir := filepath.Join(repo, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(armatureDir, "ops"), 0o755))
	// Deliberately skip setting armature.worker-id so GetWorkerID fails.

	issuesDir := filepath.Join(repo, ".armature")
	assert.NotPanics(t, func() {
		tryEmitHeartbeat(repo, issuesDir, "", "task-emit-04", harnesshook.EventPreToolUse)
	})
}

// heartbeatOpsForWorker reads the heartbeat ops recorded for workerID's ops log
// in repo, or an empty slice if the log doesn't exist yet.
func heartbeatOpsForWorker(t *testing.T, repo, workerID string) []ops.Op {
	t.Helper()
	logPath := opsLogPath(filepath.Join(repo, ".armature"), workerIdentityWithSlot(workerID))
	loggedOps, err := ops.ReadLog(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		require.NoError(t, err)
	}
	var heartbeats []ops.Op
	for _, op := range loggedOps {
		if op.Type == ops.OpHeartbeat {
			heartbeats = append(heartbeats, op)
		}
	}
	return heartbeats
}

// backdateAllOps rewrites every op in workerID's ops log, shifting each
// Timestamp back by delta while preserving relative order between ops. Used to
// simulate a claim aging past its TTL without sleeping in the test or breaking
// materialize's global timestamp-sort replay ordering (which requires a task's
// create op to sort before any later op on the same target).
func backdateAllOps(t *testing.T, repo, workerID string, delta time.Duration) {
	t.Helper()
	logPath := opsLogPath(filepath.Join(repo, ".armature"), workerIdentityWithSlot(workerID))
	loggedOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)

	require.NoError(t, os.Remove(logPath))
	for _, op := range loggedOps {
		op.Timestamp -= int64(delta.Seconds())
		require.NoError(t, ops.AppendOp(logPath, op))
	}
}

// TestHookEmitsRateLimitedHeartbeat_REQ_LNGHZN_S3_T1 verifies that the hook emits
// at most one heartbeat op across two back-to-back PreToolUse events for a
// bound, non-stale claim (fixed debounce window, not per-call).
func TestHookEmitsRateLimitedHeartbeat_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	defer os.Remove(rateLimitStateFilePath(workerID, "task-01")) //nolint:errcheck // best-effort cleanup

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")
	payload := `{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"cmd/armature/main.go"}]}}`

	for i := 0; i < 2; i++ {
		hookCmd := newRootCmd()
		hookCmd.SetIn(strings.NewReader(payload))
		hookCmd.SetOut(new(bytes.Buffer))
		hookCmd.SetErr(new(bytes.Buffer))
		hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})
		require.NoError(t, hookCmd.Execute())
	}

	heartbeats := heartbeatOpsForWorker(t, repo, workerID)
	require.Len(t, heartbeats, 1, "second call within the debounce window must not emit another heartbeat")
	assert.Equal(t, "task-01", heartbeats[0].TargetID)
}

// TestHookEmitsHeartbeatMatchingSlottedClaimant_REQ_LNGHZN_S3_T1 verifies that
// when ARM_LOG_SLOT is set (parallel dispatch mode), the hook-emitted heartbeat
// op's WorkerID matches the slotted identity recorded as ClaimedBy at claim
// time, so applyHeartbeat's op.WorkerID == issue.ClaimedBy check actually
// advances LastHeartbeat instead of silently no-op'ing.
func TestHookEmitsHeartbeatMatchingSlottedClaimant_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	t.Setenv("ARM_LOG_SLOT", "slot-1")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	defer os.Remove(rateLimitStateFilePath(workerIdentityWithSlot(workerID), "task-01")) //nolint:errcheck // best-effort cleanup

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")
	payload := `{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"cmd/armature/main.go"}]}}`

	hookCmd := newRootCmd()
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(new(bytes.Buffer))
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})
	require.NoError(t, hookCmd.Execute())

	slottedID := workerIdentityWithSlot(workerID)
	heartbeats := heartbeatOpsForWorker(t, repo, workerID)
	require.Len(t, heartbeats, 1)
	assert.Equal(t, slottedID, heartbeats[0].WorkerID,
		"heartbeat op.WorkerID must match the slotted identity recorded as ClaimedBy, or applyHeartbeat silently ignores it")

	materializeCmd := newRootCmd()
	materializeCmd.SetOut(new(bytes.Buffer))
	materializeCmd.SetArgs([]string{"materialize", "--repo", repo})
	require.NoError(t, materializeCmd.Execute())

	issues, err := materialize.LoadAllIssues(filepath.Join(repo, ".armature", "state", slottedID, "issues"))
	require.NoError(t, err)
	issue, ok := issues["task-01"]
	require.True(t, ok, "task-01 must be materialized")

	assert.Equal(t, slottedID, issue.ClaimedBy, "claim must record the slotted identity")
	assert.GreaterOrEqual(t, issue.LastHeartbeat, heartbeats[0].Timestamp,
		"hook-emitted heartbeat from the slotted claimant must advance LastHeartbeat")
}

// TestHookSkipsHeartbeatWhenUnbound_REQ_LNGHZN_S3_T1 verifies that no heartbeat
// op is emitted when the event resolves to no issue binding at all.
func TestHookSkipsHeartbeatWhenUnbound_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	// Deliberately do not claim task-01 and do not set ARMATURE_ISSUE_ID, so the
	// event resolves to no binding.
	t.Setenv("ARMATURE_ISSUE_ID", "")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	hookCmd := newRootCmd()
	hookCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"echo hi"}}`))
	hookCmd.SetOut(new(bytes.Buffer))
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})
	require.NoError(t, hookCmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	assert.Empty(t, heartbeatOpsForWorker(t, repo, workerID), "unbound event must not emit a heartbeat")
}

// TestShouldHeartbeatSkipsAlreadyStaleClaim_REQ_LNGHZN_S3_T1 verifies that once a
// claim's TTL has expired, the hook treats it as stale and does not emit a heartbeat,
// end to end (claim, backdate it past its TTL, then invoke the hook).
func TestShouldHeartbeatSkipsAlreadyStaleClaim_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree", "--ttl", "10"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	defer os.Remove(rateLimitStateFilePath(workerID, "task-01")) //nolint:errcheck // best-effort cleanup

	// Materialize sorts all ops globally by Timestamp before replay, so backdating
	// only the claim op would sort it before the task's create op and break
	// replay ("issue not found"). Instead, shift every existing op's Timestamp
	// back by an hour, preserving relative order, so the whole history (and thus
	// ClaimedAt/LastHeartbeat) is old enough to exceed the 10-minute TTL relative
	// to the real wall-clock time isBindingStale checks against.
	backdateAllOps(t, repo, workerID, time.Hour)

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	hookCmd := newRootCmd()
	hookCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"cmd/armature/main.go"}]}}`))
	hookCmd.SetOut(new(bytes.Buffer))
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})
	require.NoError(t, hookCmd.Execute())

	assert.Empty(t, heartbeatOpsForWorker(t, repo, workerID), "stale claim must not emit a heartbeat")
}

// TestHookHeartbeatSourceIsAutoHook_REQ_LNGHZN_S3_T1 verifies the hook-emitted
// heartbeat op carries Source="hook", distinguishing it from a manual heartbeat.
func TestHookHeartbeatSourceIsAutoHook_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	defer os.Remove(rateLimitStateFilePath(workerID, "task-01")) //nolint:errcheck // best-effort cleanup

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	hookCmd := newRootCmd()
	hookCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"cmd/armature/main.go"}]}}`))
	hookCmd.SetOut(new(bytes.Buffer))
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})
	require.NoError(t, hookCmd.Execute())

	heartbeats := heartbeatOpsForWorker(t, repo, workerID)
	require.Len(t, heartbeats, 1)
	assert.Equal(t, "hook", heartbeats[0].Payload.Source)
}

// TestManualHeartbeatSourceIsManual_REQ_LNGHZN_S3_T1 verifies that a manual
// `arm heartbeat` call omits the Source field (distinct from hook-emitted
// heartbeats, which carry Source="hook").
func TestManualHeartbeatSourceIsManual_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)

	heartbeatCmd := newRootCmd()
	heartbeatCmd.SetOut(new(bytes.Buffer))
	heartbeatCmd.SetArgs([]string{"heartbeat", "--repo", repo, "task-01"})
	require.NoError(t, heartbeatCmd.Execute())

	heartbeats := heartbeatOpsForWorker(t, repo, workerID)
	require.Len(t, heartbeats, 1)
	assert.Empty(t, heartbeats[0].Payload.Source, "manual heartbeat must not carry Source=\"hook\"")
}

// TestHookHeartbeatFiresOnBlockedToolCall_REQ_LNGHZN_S3_T1 verifies that the
// heartbeat is emitted even when the tool call itself is ultimately blocked as
// out-of-scope: the heartbeat runs before the allow/block decision and does not
// depend on its outcome.
func TestHookHeartbeatFiresOnBlockedToolCall_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	defer os.Remove(rateLimitStateFilePath(workerID, "task-01")) //nolint:errcheck // best-effort cleanup

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	// "cmd/armature/main.go" is outside the task's narrowed scope, so this call is blocked.
	hookCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"cmd/armature/main.go"}]}}`))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})
	require.NoError(t, hookCmd.Execute())
	require.Contains(t, out.String(), `"decision":"block"`, "sanity check: this call must actually be blocked")

	heartbeats := heartbeatOpsForWorker(t, repo, workerID)
	require.Len(t, heartbeats, 1, "heartbeat must still be emitted even though the tool call was blocked")
	assert.Equal(t, "hook", heartbeats[0].Payload.Source)
}
