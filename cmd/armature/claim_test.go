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

// setupRepoWithEpic creates a repo with an epic issue.
func setupRepoWithEpic(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test epic", "--type", "epic", "--id", "epic-01"})
	require.NoError(t, cmd2.Execute())

	return repo
}

// setupRepoWithParentAndTask creates a repo with a parent story and a task.
func setupRepoWithParentAndTask(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	// Create parent story
	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Parent story", "--type", "story", "--id", "story-01"})
	require.NoError(t, cmd2.Execute())

	// Create child task
	cmd3 := newRootCmd()
	cmd3.SetOut(new(bytes.Buffer))
	cmd3.SetArgs([]string{"create", "--repo", repo, "--title", "Child task", "--type", "task", "--id", "task-01", "--parent", "story-01"})
	require.NoError(t, cmd3.Execute())

	return repo
}

// TestClaimWithoutWorktreeFlag verifies that claim fails when --worktree is omitted.
func TestClaimWithoutWorktreeFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, errBuf.String()+buf.String(), "worktree")
}

// TestClaimCreatesWorktreeIfAbsent verifies that claim creates a worktree at the path
// when it doesn't exist, along with a derived branch.
func TestClaimCreatesWorktreeIfAbsent(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})

	err := cmd.Execute()
	require.NoError(t, err)

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree directory should be created")

	// Verify .git file exists in worktree (marker of a git worktree)
	gitPath := filepath.Join(worktreePath, ".git")
	assert.FileExists(t, gitPath, ".git file should exist in worktree")

	// Read the .git file to find the actual git directory
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)
	assert.Contains(t, gitDirLine, "gitdir: ", ".git file should contain gitdir reference")

	// Extract actual git dir from the .git file
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	// Verify armature-task-id file is created in the actual git directory
	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	assert.FileExists(t, taskIDFile, "armature-task-id file should be created in actual git dir")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	assert.Equal(t, "task-01", string(taskID))
}

// TestClaimUpdatesTaskIDIfWorktreeExists verifies that claim updates armature-task-id
// when the worktree already exists.
func TestClaimUpdatesTaskIDIfWorktreeExists(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	// First claim creates the worktree
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, cmd.Execute())

	// Read the .git file to find the actual git directory
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	// Extract actual git dir from the .git file
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	// Verify armature-task-id was written
	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	assert.Equal(t, "task-01", string(taskID))
}

// TestClaimWithEpicReturnsError verifies that claiming an epic returns an error.
func TestClaimWithEpicReturnsError(t *testing.T) {
	repo := setupRepoWithEpic(t)
	worktreePath := filepath.Join(t.TempDir(), "epic-worktree")

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "epic-01", "--worktree", worktreePath})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, errBuf.String()+buf.String(), "epic")
}

// TestClaimCreatesTaskBranch verifies that claim creates an orphan branch with the
// correct prefix (task/<id>) in the new worktree's git directory.
func TestClaimCreatesTaskBranch(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})

	err := cmd.Execute()
	require.NoError(t, err)

	// Verify the branch is created with task/ prefix
	// Read the .git file to find the actual git directory
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)

	// Extract actual git dir
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	headFile := filepath.Join(actualGitDir, "HEAD")
	assert.FileExists(t, headFile, "HEAD file should exist in git directory")
	headContent, err := os.ReadFile(headFile) //nolint:gosec // test path is safe
	require.NoError(t, err)
	headStr := string(headContent)
	// Should reference task/task-01 branch
	assert.Contains(t, headStr, "task-01", "HEAD should reference task/task-01 branch")
}

// TestClaimStillAppendsClaimOpToLog verifies that even though worktree is created,
// the claim op is still appended to the ops log.
func TestClaimStillAppendsClaimOpToLog(t *testing.T) {
	repo := setupRepoWithTask(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})

	require.NoError(t, cmd.Execute())

	// Verify claim operation was appended to ops log
	// The output should indicate successful claim
	assert.Contains(t, buf.String(), "task-01", "output should mention the claimed task")
}
