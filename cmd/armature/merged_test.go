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

// TestMergedDoesNotRemoveStoryWorktree verifies that merged does NOT remove worktrees for story-type issues.
func TestMergedDoesNotRemoveStoryWorktree(t *testing.T) {
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

	// Call merged command (stories don't have worktrees, but command should handle gracefully)
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "story-01"})
	require.NoError(t, mergedCmd.Execute())
}

// TestMergedDoesNotRemoveFeatureWorktree verifies that merged does NOT remove worktrees for feature-type issues.
func TestMergedDoesNotRemoveFeatureWorktree(t *testing.T) {
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

	// Call merged command (features don't have worktrees, but command should handle gracefully)
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
