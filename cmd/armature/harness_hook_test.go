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
}

func TestHarnessHookBlocksOutOfScopeEdit(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

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

	require.NoError(t, err)
	assert.Contains(t, out.String(), `"decision":"block"`)
	assert.Contains(t, out.String(), "outside task scope")
}

func TestHarnessHookAllowsInScopeEdit(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

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

	require.NoError(t, err)
	assert.Contains(t, out.String(), `"decision":"block"`)
	assert.Contains(t, out.String(), "unverifiable")
}

func TestAdapterExitError(t *testing.T) {
	err := adapterExitError{code: 42}
	assert.Equal(t, 42, err.code, "code should be preserved on the struct")

	errMsg := err.Error()
	assert.Contains(t, errMsg, "hook blocked with exit code 42")

	var ace adapterExitError
	require.True(t, errors.As(err, &ace))
	assert.Equal(t, 42, ace.code)
}

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

func TestResolveIssueBinding_FromFile(t *testing.T) {
	gitDir := t.TempDir()
	taskIDPath := filepath.Join(gitDir, "armature-issue-id")
	err := os.WriteFile(taskIDPath, []byte("task-from-file"), 0o644)
	require.NoError(t, err)

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "task-from-file", taskID)
}

func TestResolveIssueBinding_FromEnv(t *testing.T) {
	gitDir := t.TempDir()
	t.Setenv("ARMATURE_ISSUE_ID", "task-from-env")

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "task-from-env", taskID)
}

func TestResolveIssueBinding_Empty(t *testing.T) {
	gitDir := t.TempDir()
	t.Setenv("ARMATURE_ISSUE_ID", "")

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "", taskID)
}

func TestResolveIssueBinding_FilePreferredOverEnv(t *testing.T) {
	gitDir := t.TempDir()
	taskIDPath := filepath.Join(gitDir, "armature-issue-id")
	err := os.WriteFile(taskIDPath, []byte("task-from-file"), 0o644)
	require.NoError(t, err)
	t.Setenv("ARMATURE_ISSUE_ID", "task-from-env")

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "task-from-file", taskID)
}

func TestResolveIssueBinding_FallsBackToLegacyTaskIDFile(t *testing.T) {
	gitDir := t.TempDir()
	taskIDPath := filepath.Join(gitDir, "armature-task-id")
	err := os.WriteFile(taskIDPath, []byte("legacy-task-id"), 0o644)
	require.NoError(t, err)

	taskID := resolveIssueBinding(gitDir)

	assert.Equal(t, "legacy-task-id", taskID)
}

func TestHarnessHookUntrustedPathResolvedGitDir_FallsBackToSessionBinding(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

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
	assert.Contains(t, logContent, "issue_id=task-01", "should evaluate under the session binding, not an empty binding")
}

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
	content := string(data)
	assert.Contains(t, content, "pass-through: test reason")
	assert.Contains(t, content, "Z pass-through:")
}

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
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	assert.Len(t, lines, 2)
}

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

func TestIsBindingStale_Missing(t *testing.T) {
	snap := &snapshot.Snapshot{
		Issues: make(map[string]*materialize.Issue),
	}

	stale := isBindingStale(snap, "task-01", 1000)

	assert.True(t, stale)
}

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

func TestIsBindingStale_ClaimedWithExpiredTTL(t *testing.T) {
	now := int64(2000)
	claimedAt := int64(1000)
	lastHeartbeat := int64(1100)
	ttlMinutes := 10

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

	stale := isBindingStale(snap, "task-01", now)

	assert.True(t, stale, "claimed task with expired TTL should be stale")
}

func TestIsBindingStale_ClaimedWithinTTLWindow(t *testing.T) {
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

func TestHarnessHookReadsBindingFromFileWithoutEnv(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	gitPath := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreeDir, actualGitDir)
	}
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	require.FileExists(t, taskIDFile, "armature-issue-id must exist in worktree git dir")

	t.Setenv("ARMATURE_ISSUE_ID", "")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	hookCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"internal/harnesshook/evaluator.go"}]}}`)) //nolint:lll
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", worktreeDir})

	err = hookCmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), `"decision":"approve"`, "hook must read binding from file and approve in-scope edit")
}

func TestDecisionLoggedToResolvedWorktree_REQ_HOOKBIND_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	claimedWorktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = claimCmd.Execute()
	require.NoError(t, err)

	gitFile := filepath.Join(claimedWorktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(claimedWorktreeDir, actualGitDir)
	}

	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	filePath := filepath.Join(claimedWorktreeDir, "internal/harnesshook/hook.go")
	fileDir := filepath.Dir(filePath)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	var out bytes.Buffer
	hookCmd := newRootCmd()
	hookCmd.SetIn(strings.NewReader(fmt.Sprintf(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"%s"}]}}`, filePath)))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	require.NoError(t, err)

	resolvedLogPath := filepath.Join(actualGitDir, "armature-hook.log")
	//nolint:gosec // G703: path derived from test worktree git dir
	logData, err := os.ReadFile(resolvedLogPath)
	require.NoError(t, err, "log must exist in resolved worktree's git dir")
	logContent := string(logData)

	assert.Contains(t, logContent, "decision:", "log must contain decision entry")
	assert.Contains(t, logContent, "task-01", "log must contain resolved issue ID")
	assert.Contains(t, logContent, "pre-tool-use", "log must contain event kind")
	assert.Contains(t, logContent, "apply_patch", "log must contain tool name")
	assert.Contains(t, logContent, "resolution_step=file_path", "log must contain the actual resolution step (file_path for path-based resolution)")

	mainRepoLogPath := filepath.Join(repo, ".git", "armature-hook.log")
	_, err = os.ReadFile(mainRepoLogPath)
	assert.Error(t, err, "log must NOT exist in main repo's git dir when binding resolves to different worktree")
}

func TestUnboundFileWriteLogsViolation_REQ_HOOKBIND_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

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

	assert.Contains(t, logContent, "violation:", "unbound file write must log violation entry")
	assert.NotContains(t, logContent, "pass-through:", "unbound file write must not log pass-through")
}

func TestStaleBindingPassThroughLogsScopeViolation_REQ_TOPTIER_S5_T2(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
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

func TestResolverErrorFailsOpen_REQ_HOOKBIND_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

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

func TestFailOpenOnEventDecodeError_REQ_HOOKBIND_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out, errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetIn(strings.NewReader(`{invalid json`))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err, "event decode error should fail-open with exit 0")

	stderrOutput := errOut.String()
	assert.NotEmpty(t, stderrOutput, "should write warning to stderr on decode error")
	assert.Contains(t, stderrOutput, "error", "stderr should contain error indication")

	gitDir := filepath.Join(repo, ".git")
	logPath := filepath.Join(gitDir, "armature-hook.log")
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)
	logContent := string(logData)
	assert.Contains(t, logContent, "pass-through:", "event decode error should log pass-through")
	assert.Contains(t, logContent, "decode", "log should mention decode in error description")
}

func TestSnapshotErrorFailsOpen_REQ_HOOKBIND_T3(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod 0o000 has no effect for the root user; cannot exercise unreadable-dir fail-open this way")
	}
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "amend", "task-01", "--scope", "internal/harnesshook/", "--acceptance", `["go test ./... passes"]`)
	require.NoError(t, err)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	err = cmd.Execute()
	require.NoError(t, err)

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	opsDir := filepath.Join(repo, ".armature", "ops")
	require.DirExists(t, opsDir, "opsDir should exist")

	err = os.Chmod(opsDir, 0o000)
	require.NoError(t, err, "should be able to chmod ops directory")
	t.Cleanup(func() {
		swallowErr(os.Chmod(opsDir, 0o755))
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
	require.NoError(t, err, "snapshot load error should fail-open with exit 0")

	stderrOutput := errOut.String()
	assert.Contains(t, stderrOutput, "error:", "stderr must contain error indication")
	assert.Contains(t, stderrOutput, "snapshot", "stderr must mention snapshot")

	gitDir := filepath.Join(repo, ".git")
	logPath := filepath.Join(gitDir, "armature-hook.log")
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err, "log must exist")
	logContent := string(logData)
	assert.Contains(t, logContent, "pass-through:", "snapshot load error should log pass-through entry")
	assert.Contains(t, logContent, "snapshot load failed", "log should describe the failure reason")
}

func TestHarnessHookCapturesActivityForBashPostToolUse_REQ_EXECEV_T1(t *testing.T) {
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
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	payload := `{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"echo test"},"tool_response":{"exit_code":0,"output":"test output\n"}}`
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", worktreeDir})

	err = hookCmd.Execute()
	require.NoError(t, err)

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

func TestHarnessHookActivityLogTruncatesLargeOutput_REQ_EXECEV_T1(t *testing.T) {
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
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

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

	activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
	activityData, err := os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
	require.NoError(t, err)

	activityContent := string(activityData)
	assert.Contains(t, activityContent, `"output_hash"`, "activity entry must contain output hash for verification")
}

func TestHarnessHookActivityKillSwitchDisablesCapture_REQ_EXECEV_T1(t *testing.T) {
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

	activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
	_, err = os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
	assert.Error(t, err, "activity log should not exist when the repo-level kill-switch is set")
	assert.True(t, os.IsNotExist(err), "activity log should not exist (not exist error)")
}

func TestHarnessHookActivityFailOpenOnError_REQ_EXECEV_T1(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod 0o000 has no effect for the root user; cannot exercise write-error fail-open this way")
	}

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

	err = os.Chmod(actualGitDir, 0o500) //nolint:gosec // G703: test code to make directory read-only
	require.NoError(t, err)
	t.Cleanup(func() {
		swallowErr(os.Chmod(actualGitDir, 0o755)) //nolint:gosec
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
	require.NoError(t, err, "activity logging error should fail-open with exit 0")

}

func TestHarnessHookNoActivityForNonBashTools_REQ_EXECEV_T1(t *testing.T) {
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
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	payload := `{"hook_event_name":"PostToolUse","tool_name":"Edit",` +
		`"tool_input":{"file_path":"internal/harnesshook/hook.go"},"tool_response":{"exit_code":0,"output":""}}`
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	require.NoError(t, err)

	activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
	_, err = os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
	assert.Error(t, err, "activity log should not exist for non-Bash PostToolUse events")
	assert.True(t, os.IsNotExist(err), "activity log should not exist (not exist error)")
}

func TestHarnessHookNoActivityForPreToolUse_REQ_EXECEV_T1(t *testing.T) {
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
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
	payload := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"echo test"}}`
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(&out)
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})

	err = hookCmd.Execute()
	require.NoError(t, err)

	activityLogPath := filepath.Join(actualGitDir, "armature-activity.log")
	_, err = os.ReadFile(activityLogPath) //nolint:gosec // G703: safe to read test worktree activity log
	assert.Error(t, err, "activity log should not exist for PreToolUse events")
	assert.True(t, os.IsNotExist(err), "activity log should not exist (not exist error)")
}

func TestHeartbeatRateLimitStateRoundTrip(t *testing.T) {
	workerID := fmt.Sprintf("test-worker-%d", time.Now().UnixNano())
	issueID := "task-round-trip"
	defer func() { swallowErr(os.Remove(rateLimitStateFilePath(workerID, issueID))) }()

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
	defer func() { swallowErr(os.Remove(stateFile)) }()

	require.NoError(t, os.WriteFile(stateFile, []byte("not json"), 0o600))

	assert.True(t, readHeartbeatRateLimitState(workerID, issueID).IsZero())
}

func TestTryEmitHeartbeatFailsOpenWhenWorkerUnset(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	armatureDir := filepath.Join(repo, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(armatureDir, "ops"), 0o755))

	issuesDir := filepath.Join(repo, ".armature")
	assert.NotPanics(t, func() {
		tryEmitHeartbeat(repo, issuesDir, "", "task-emit-04", harnesshook.EventPreToolUse)
	})
}

func heartbeatOpsForWorker(t *testing.T, repo, workerID string) []ops.Op {
	t.Helper()
	logPath := opsLogPath(filepath.Join(repo, ".armature"), slottedWorkerID(workerID).String())
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

func backdateAllOps(t *testing.T, repo, workerID string, delta time.Duration) {
	t.Helper()
	logPath := opsLogPath(filepath.Join(repo, ".armature"), slottedWorkerID(workerID).String())
	loggedOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)

	require.NoError(t, os.Remove(logPath))
	for _, op := range loggedOps {
		op.Timestamp -= int64(delta.Seconds())
		require.NoError(t, ops.AppendOp(logPath, op))
	}
}

func TestHookEmitsRateLimitedHeartbeat_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	defer func() { swallowErr(os.Remove(rateLimitStateFilePath(workerID, "task-01"))) }()

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

func TestHookEmitsHeartbeatMatchingSlottedClaimant_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	t.Setenv("ARM_LOG_SLOT", "slot-1")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	defer func() { swallowErr(os.Remove(rateLimitStateFilePath(slottedWorkerID(workerID).String(), "task-01"))) }()

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")
	payload := `{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"changes":[{"path":"cmd/armature/main.go"}]}}`

	hookCmd := newRootCmd()
	hookCmd.SetIn(strings.NewReader(payload))
	hookCmd.SetOut(new(bytes.Buffer))
	hookCmd.SetErr(new(bytes.Buffer))
	hookCmd.SetArgs([]string{"harness-hook", "--repo", repo})
	require.NoError(t, hookCmd.Execute())

	slottedID := slottedWorkerID(workerID).String()
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

func TestHookSkipsHeartbeatWhenUnbound_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
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

func TestShouldHeartbeatSkipsAlreadyStaleClaim_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree", "--ttl", "10"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	defer func() { swallowErr(os.Remove(rateLimitStateFilePath(workerID, "task-01"))) }()

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

func TestHookHeartbeatSourceIsAutoHook_REQ_LNGHZN_S3_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, cmd.Execute())

	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	defer func() { swallowErr(os.Remove(rateLimitStateFilePath(workerID, "task-01"))) }()

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
	defer func() { swallowErr(os.Remove(rateLimitStateFilePath(workerID, "task-01"))) }()

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")
	t.Setenv("ARMATURE_HOOK_PLATFORM", "codex")

	var out bytes.Buffer
	hookCmd := newRootCmd()
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
