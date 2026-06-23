package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergedRemovesTaskWorktree verifies that merged removes a worktree for task-type issues.
func TestMergedRemovesTaskWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Completed", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Call merged command
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	require.NoError(t, mergedCmd.Execute())

	// Verify worktree is removed
	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

// TestMergedRemovesBugWorktree verifies that merged removes a worktree for bug-type issues.
func TestMergedRemovesBugWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test bug", "--type", "bug", "--id", "bug-01"})
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(t.TempDir(), "bug-worktree")

	// Claim the bug to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "bug-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition bug to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "bug-01", "--to", "done", "--outcome", "Fixed", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Call merged command
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "bug-01"})
	require.NoError(t, mergedCmd.Execute())

	// Verify worktree is removed
	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

// TestMergedHandlesStoryWithNoActiveWorktree verifies that merged handles gracefully
// a story-type issue when no worktree was created for it (e.g. no --worktree used at claim time).
// Stories now map to feat/<id> via deriveBranchName, so merged will attempt worktree removal,
// but must not fail when no matching worktree exists.
func TestMergedHandlesStoryWithNoActiveWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test story", "--type", "story", "--id", "story-01"})
	require.NoError(t, cmd2.Execute())

	// Transition story to done before calling merged
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "story-01", "--to", "done", "--outcome", "Delivered", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Call merged command (no worktree was created for this story; command must handle gracefully)
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "story-01"})
	require.NoError(t, mergedCmd.Execute())
}

// TestMergedRemovesStoryWorktree verifies that merged removes the worktree for a story-type
// issue when one was created via claim. Stories map to feat/<id> via deriveBranchName, so
// merged must tear down the story worktree just like task/bug/feature worktrees.
func TestMergedRemovesStoryWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test story", "--type", "story", "--id", "story-01"})
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(t.TempDir(), "story-worktree")

	// Claim the story to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "story-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition story to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "story-01", "--to", "done", "--outcome", "Delivered", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Call merged command
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "story-01"})
	require.NoError(t, mergedCmd.Execute())

	// Verify worktree is removed
	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

// TestMergedRemovesFeatureWorktree verifies that merged removes a worktree for feature-type issues
// (F2: deriveBranchName maps feature → feat/, and merged must tear down every type that claim creates).
func TestMergedRemovesFeatureWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test feature", "--type", "feature", "--id", "feature-01"})
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(t.TempDir(), "feature-worktree")

	// Claim the feature to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "feature-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition feature to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "feature-01", "--to", "done", "--outcome", "Shipped", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Call merged command
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "feature-01"})
	require.NoError(t, mergedCmd.Execute())

	// Verify worktree is removed
	assert.NoDirExists(t, worktreePath, "worktree should be removed after merged")
}

// TestMergedHandlesFeatureWithNoWorktree verifies that merged gracefully handles
// a feature-type issue with no associated worktree (e.g., not created via --worktree).
func TestMergedHandlesFeatureWithNoWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test feature", "--type", "feature", "--id", "feature-01"})
	require.NoError(t, cmd2.Execute())

	// Transition feature to done before calling merged
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "feature-01", "--to", "done", "--outcome", "Shipped", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Call merged command (no worktree was created; should handle gracefully)
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "feature-01"})
	require.NoError(t, mergedCmd.Execute())
}

// TestMergedWarnsOnPassThroughEntries verifies that merged warns to stderr when armature-hook.log contains pass-through entries.
func TestMergedWarnsOnPassThroughEntries(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Create armature-hook.log with pass-through entries
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	hookLogPath := filepath.Join(actualGitDir, "armature-hook.log")
	hookLogContent := "pass-through: no task binding found\npass-through: stale binding\n"
	err = os.WriteFile(hookLogPath, []byte(hookLogContent), 0o600) //nolint:gosec // test path under temp directory
	require.NoError(t, err)

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Completed", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Call merged command and capture stderr
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.NoError(t, err)

	// Verify warning is printed to stderr
	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "pass-through", "should warn about pass-through entries in stderr")
	assert.Contains(t, errOutput, "task-01", "warning should mention the issue ID")
}

// TestMergedNoWarningWithoutPassThroughEntries verifies that merged does not warn when hook.log has no pass-through entries.
func TestMergedNoWarningWithoutPassThroughEntries(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Optionally create armature-hook.log without pass-through entries (or don't create it at all)
	// Either way, no warning should be printed

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Completed", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Call merged command and capture stderr
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	require.NoError(t, err)

	// Verify no warning is printed (or only success message)
	errOutput := errBuf.String()
	assert.NotContains(t, errOutput, "pass-through", "should not warn about pass-through when none exist")
}

// TestMergedHandlesMissingWorktree verifies that merged handles the case where a worktree was already deleted.
func TestMergedHandlesMissingWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Completed", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Manually remove the worktree (simulating it being deleted before merged is called)
	run(t, repo, "git", "worktree", "remove", "--force", worktreePath)

	// Call merged command (should not fail even though worktree is gone)
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	// Should succeed gracefully or at least not crash
	require.NoError(t, err)
}

// TestMergedDoesNotWarnWhenWorktreeAlreadyRemoved verifies that when the worktree has
// already been deleted before `arm merged` is called, no pass-through warning is emitted
// (because the warning requires the worktree to be present to read its hook log), and
// that the command does not error or panic. Emitting the warning when the worktree is
// already gone requires persisting the git-dir path separately (future work, F6).
func TestMergedDoesNotWarnWhenWorktreeAlreadyRemoved(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Read the actual git dir from the worktree so we can write the hook log.
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	hookLogPath := filepath.Join(actualGitDir, "armature-hook.log")
	err = os.WriteFile(hookLogPath, []byte("pass-through: no task binding found\n"), 0o600) //nolint:gosec // test path
	require.NoError(t, err)

	// Transition task to done
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Completed", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Remove the worktree before calling merged
	run(t, repo, "git", "worktree", "remove", "--force", worktreePath)

	// Call merged command and capture stderr
	mergedCmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	mergedCmd.SetOut(outBuf)
	mergedCmd.SetErr(errBuf)
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err = mergedCmd.Execute()
	require.NoError(t, err)

	// The implementation can only check hook log entries when the worktree is still
	// present (because it needs to find it via git worktree list). After the worktree
	// is removed, git worktree list no longer reports it, so the hook log path cannot
	// be resolved and no warning is emitted. This is correct current behavior.
	// TODO(F6): emit warning even when worktree is already gone (requires persisting git-dir).
	assert.NotContains(t, errBuf.String(), "pass-through", "no warning expected when worktree is already gone")
}

// TestMergedRejectsSingleBranchModeWithoutMergedStatus verifies that merged requires
// status=merged (or status=done) in single-branch mode. Currently, in single-branch mode,
// the status guard is skipped, allowing arm merged to be called on in-progress tasks,
// which deletes the worktree and any uncommitted worker state (P2 bug).
func TestMergedRejectsSingleBranchModeWithoutMergedStatus(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap in single-branch mode (default, no --dual-branch flag)
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"})
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition to in-progress (NOT to done)
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "in-progress"})
	require.NoError(t, transitionCmd.Execute())

	// Call merged command — should fail because status is not merged/done in single-branch mode
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	require.Error(t, err, "merged should reject in-progress status in single-branch mode")
	assert.Contains(t, err.Error(), "status=merged", "error message should indicate merged status required")
	assert.Contains(t, err.Error(), "single-branch", "error message should mention single-branch mode")

	// Verify worktree still exists (should not be deleted on error)
	assert.DirExists(t, worktreePath, "worktree should NOT be removed when merged fails")
}

// TestMergedRecordsOpBeforeRemovingWorktree verifies the P2 bug fix.
// The bug (pre-fix): removeWorktreeForIssue is called BEFORE appendOp (lines 152-154 in buggy code),
// so if appendOp fails, the worktree is already deleted and recovery is impossible.
// The fix: move removeWorktreeForIssue to AFTER appendOp succeeds, so on failure the worktree is preserved.
//
// Happy path: merged command executes successfully → worktree is removed.
// Failure path: ops dir made read-only so appendOp fails → worktree is NOT removed (recovery possible).
func TestMergedRecordsOpBeforeRemovingWorktree(t *testing.T) {
	t.Run("happy path: op recorded and worktree removed", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		worktreePath := filepath.Join(t.TempDir(), "task-worktree")

		claimCmd := newRootCmd()
		claimCmd.SetOut(new(bytes.Buffer))
		claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
		require.NoError(t, claimCmd.Execute())
		assert.DirExists(t, worktreePath, "worktree should exist after claim")

		transitionCmd := newRootCmd()
		transitionCmd.SetOut(new(bytes.Buffer))
		transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Completed", "--force"})
		require.NoError(t, transitionCmd.Execute())

		mergedCmd := newRootCmd()
		mergedCmd.SetOut(new(bytes.Buffer))
		mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
		require.NoError(t, mergedCmd.Execute())

		// Op succeeded → worktree should be removed
		assert.NoDirExists(t, worktreePath, "worktree should be removed after successful merged")
	})

	t.Run("failure path: appendOp fails → worktree preserved", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		worktreePath := filepath.Join(t.TempDir(), "task-worktree")

		claimCmd := newRootCmd()
		claimCmd.SetOut(new(bytes.Buffer))
		claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
		require.NoError(t, claimCmd.Execute())
		assert.DirExists(t, worktreePath, "worktree should exist after claim")

		transitionCmd := newRootCmd()
		transitionCmd.SetOut(new(bytes.Buffer))
		transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Completed", "--force"})
		require.NoError(t, transitionCmd.Execute())

		// Make the ops log directory read-only so appendOp cannot write a new log entry.
		// This simulates a disk-full or permission error during the op write.
		opsDir := filepath.Join(repo, ".armature", "ops")
		require.NoError(t, os.Chmod(opsDir, 0o444))
		defer func() {
			if err := os.Chmod(opsDir, 0o755); err != nil {
				t.Logf("warning: failed to restore ops dir permissions: %v", err)
			}
		}() // restore so cleanup works

		mergedCmd := newRootCmd()
		mergedCmd.SetOut(new(bytes.Buffer))
		mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
		err := mergedCmd.Execute()
		require.Error(t, err, "merged should fail when appendOp cannot write")

		// The critical invariant: because the op write failed BEFORE removeWorktreeForIssue
		// was called, the worktree must still be present and recovery is possible.
		assert.DirExists(t, worktreePath, "worktree must NOT be removed when appendOp fails")
	})
}
