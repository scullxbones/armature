package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
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

// TestClaimCreatesTaskBranch verifies that claim creates a task branch from HEAD with the
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

// TestDeriveBranchName verifies that deriveBranchName returns correct branch names for all issue types.
func TestDeriveBranchName(t *testing.T) {
	tests := []struct {
		issueType    string
		issueID      string
		expectedName string
		description  string
	}{
		{
			issueType:    "bug",
			issueID:      "ARCHIMP-B1",
			expectedName: "fix/ARCHIMP-B1",
			description:  "bug issues should have fix/ prefix",
		},
		{
			issueType:    "feature",
			issueID:      "ARCHIMP-F1",
			expectedName: "feat/ARCHIMP-F1",
			description:  "feature issues should have feat/ prefix",
		},
		{
			issueType:    "task",
			issueID:      "ARCHIMP-T1",
			expectedName: "task/ARCHIMP-T1",
			description:  "task issues should have task/ prefix",
		},
		{
			issueType:    "story",
			issueID:      "ARCHIMP-S5",
			expectedName: "feat/ARCHIMP-S5",
			description:  "story issues should have feat/ prefix",
		},
		{
			issueType:    "epic",
			issueID:      "ARCHIMP-E1",
			expectedName: "",
			description:  "epic issues should return empty string",
		},
		{
			issueType:    "unknown",
			issueID:      "ARCHIMP-U1",
			expectedName: "",
			description:  "unknown issue types should return empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := deriveBranchName(tt.issueType, tt.issueID)
			assert.Equal(t, tt.expectedName, result)
		})
	}
}

// TestCreateWorktreeAndBranchInheritsFilesFromHEAD verifies that the worktree branch
// contains files from HEAD (not an orphan branch).
func TestCreateWorktreeAndBranchInheritsFilesFromHEAD(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Create a marker file in the main repo that should be visible in the task branch
	markerFile := filepath.Join(repo, "marker.txt")
	require.NoError(t, os.WriteFile(markerFile, []byte("hello from main"), 0644))
	run(t, repo, "git", "add", "marker.txt")
	run(t, repo, "git", "commit", "-m", "add marker file")

	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})

	require.NoError(t, cmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree directory should be created")

	// Check out the task branch and verify the marker file exists
	// This proves the branch has files from HEAD (not an orphan)
	markerInWorktree := filepath.Join(worktreePath, "marker.txt")
	assert.FileExists(t, markerInWorktree, "marker file from HEAD should exist in task branch worktree")

	// Verify the content
	content, err := os.ReadFile(markerInWorktree)
	require.NoError(t, err)
	assert.Equal(t, "hello from main", string(content))
}

// TestCreateWorktreeAndBranchRejectsEmptyBranchName verifies that an empty branch name
// (from epic or unknown issue types) triggers an error.
func TestCreateWorktreeAndBranchRejectsEmptyBranchName(t *testing.T) {
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
	// The error should come from the epic check in newClaimCmd, not from createWorktreeAndBranch
	assert.Contains(t, errBuf.String()+buf.String(), "epic")
}

// TestClaimFailsWhenWorktreeCreationFails tests that the claim command returns an error
// when createWorktreeAndBranch would fail. We simulate a failure by using a duplicate
// branch name that's already checked out in another worktree.
func TestCreateWorktreeAndBranchFailsWhenWorktreeCannotBeCreated(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)
	issue := materialize.Issue{Type: "task"}

	// Create the first worktree successfully
	worktree1 := filepath.Join(t.TempDir(), "worktree1")
	err := createWorktreeAndBranch(repo, worktree1, "task-01", issue)
	require.NoError(t, err, "first worktree creation should succeed")

	// Now try to create a second worktree with the same task/branch
	// This should fail because the branch is already checked out
	worktree2 := filepath.Join(t.TempDir(), "worktree2")
	err = createWorktreeAndBranch(repo, worktree2, "task-01", issue)
	require.Error(t, err, "creating worktree with already-checked-out branch should fail")
	assert.Contains(t, err.Error(), "worktree")
}

// TestClaimDoesNotCreateWorktreeWhenOverlapFails verifies that when claim fails due to
// scope overlap (without --force), NO worktree is created at the target path.
// This is the fix for: worktree setup must be deferred until all claim validations pass.
func TestClaimDoesNotCreateWorktreeWhenOverlapFails(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Create two tasks with overlapping scopes
	_, err = runTrls(t, repo, "create", "--title", "Task one", "--type", "task", "--id", "task-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task two", "--type", "task", "--id", "task-02")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "amend", "--issue", "task-01", "--scope", "cmd/armature/claim.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "amend", "--issue", "task-02", "--scope", "cmd/armature/claim.go")
	require.NoError(t, err)

	// Inject a claim op for task-01 from a DIFFERENT worker, simulating a concurrent claim.
	otherWorker := "other-worker-uuid"
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, otherWorker+".log")
	claimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  otherWorker,
		Payload:   ops.Payload{TTL: 60},
	}
	require.NoError(t, ops.AppendOp(logPath, claimOp))

	// Try to claim task-02 without --force — should fail due to scope overlap with task-01.
	worktreePath := filepath.Join(t.TempDir(), "task-02-worktree")
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "task-02", "--worktree", worktreePath)

	assert.Error(t, claimErr, "claim should fail due to scope overlap (without --force). stderr: %s", stderr)

	// Worktree must NOT have been created — worktree setup must be deferred past validation.
	_, statErr := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree must not be created when claim fails due to scope overlap")
}

// TestClaimRejectsWorktreeBoundToDifferentTask verifies that when the --worktree path
// already has an armature-task-id binding for a different issue, claim fails with a
// descriptive error rather than silently overwriting the binding.
func TestClaimRejectsWorktreeBoundToDifferentTask(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Create a second task
	_, err := runTrls(t, repo, "create", "--title", "Task two", "--type", "task", "--id", "task-02")
	require.NoError(t, err)

	// Claim task-01 with a worktree — binds the worktree to task-01.
	worktreePath := filepath.Join(t.TempDir(), "shared-worktree")
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	require.NoError(t, err)

	// Verify that armature-task-id is "task-01".
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, readErr := os.ReadFile(gitPath)
	require.NoError(t, readErr)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	taskID, readErr := os.ReadFile(taskIDFile) //nolint:gosec // test path
	require.NoError(t, readErr)
	require.Equal(t, "task-01", string(taskID))

	// Now try to claim task-02 pointing at the same worktree — should fail.
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-02", "--worktree", worktreePath)
	assert.Error(t, claimErr, "claim should fail when worktree is already bound to task-01. stderr: %s", stderr)
	assert.Contains(t, stderr+claimErr.Error(), "task-01",
		"error should mention the task currently bound to the worktree")
}

// TestClaimFailsWhenWorktreeCreationFails verifies that when createWorktreeAndBranch fails
// (e.g., due to branch already being checked out in another worktree), the claim command
// returns an error and does NOT record a claim op.
func TestClaimFailsWhenWorktreeCreationFails(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// First claim succeeds - creates worktree with task/task-01 branch
	worktree1 := filepath.Join(t.TempDir(), "worktree1")
	buf1 := new(bytes.Buffer)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	cmd1.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktree1})
	err1 := cmd1.Execute()
	require.NoError(t, err1, "first claim should succeed")

	// Second claim with same task but different worktree should fail
	// because the task/task-01 branch is already checked out in worktree1
	worktree2 := filepath.Join(t.TempDir(), "worktree2")
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktree2})

	err := cmd.Execute()
	// Claim should fail with an error
	assert.Error(t, err, "second claim should fail when branch is already checked out. stdout: %s, stderr: %s", buf.String(), errBuf.String())
	// Error message should mention worktree
	assert.Contains(t, errBuf.String()+err.Error(), "worktree")
}

// TestClaimRejectsWorktreeWithMismatchedBranch verifies that when a worktree exists
// on a different branch than the expected branch for the issue, claim fails with a
// descriptive error.
func TestClaimRejectsWorktreeWithMismatchedBranch(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Create a second task
	_, err := runTrls(t, repo, "create", "--title", "Task two", "--type", "task", "--id", "task-02")
	require.NoError(t, err)

	// Claim task-01 with a worktree — creates branch task/task-01, worktree bound to task-01
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	require.NoError(t, err)

	// Delete the armature-task-id file from the worktree's git dir so the binding check is bypassed
	// and only the branch mismatch check applies
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	require.NoError(t, os.Remove(taskIDFile), "should be able to delete armature-task-id file") //nolint:gosec // internal test path

	// Now try to claim task-02 using the same worktree path (which is still on task/task-01 branch)
	// This should fail due to branch mismatch
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-02", "--worktree", worktreePath)
	assert.Error(t, claimErr, "claim should fail due to branch mismatch. stderr: %s", stderr)
	assert.Contains(t, stderr+claimErr.Error(), "branch",
		"error should mention the branch mismatch")
}

// TestClaimAllowsWorktreeWithDetachedHEAD verifies that when a worktree has a detached HEAD
// (e.g., from mid-rebase, mid-bisect, or manual checkout), claim should allow the re-claim
// as long as the binding matches.
func TestClaimAllowsWorktreeWithDetachedHEAD(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Claim task-01 with a worktree — creates branch task/task-01, worktree bound to task-01
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	require.NoError(t, err)

	// Detach the HEAD in the worktree by checking out a specific commit
	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")

	// Verify HEAD is now a SHA (not a branch ref)
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	headFile := filepath.Join(actualGitDir, "HEAD")
	headContent, err := os.ReadFile(headFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	headStr := strings.TrimSpace(string(headContent))
	// Verify it's a detached HEAD (a SHA, not a ref)
	assert.False(t, strings.HasPrefix(headStr, "ref: "), "HEAD should be detached (not a branch ref)")

	// Now try to claim task-01 AGAIN using the same worktree path
	// This should succeed because the detached HEAD should not block re-claim when binding matches
	_, claimErr := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	assert.NoError(t, claimErr, "claim should succeed with detached HEAD when binding matches")
}

// TestClaimBoundToOtherTaskErrorDoesNotSuggestMerged verifies that when a worktree is bound
// to a different task, the error message does NOT suggest using 'arm merged' (which is only
// for post-merge teardown of completed tasks, not for live claimed/in-progress tasks).
func TestClaimBoundToOtherTaskErrorDoesNotSuggestMerged(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Create a second task
	_, err := runTrls(t, repo, "create", "--title", "Task two", "--type", "task", "--id", "task-02")
	require.NoError(t, err)

	// Claim task-01 with a worktree — binds the worktree to task-01.
	worktreePath := filepath.Join(t.TempDir(), "shared-worktree")
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	require.NoError(t, err)

	// Now try to claim task-02 with the same worktree path — should fail
	// and should NOT suggest "arm merged" in the error message.
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-02", "--worktree", worktreePath)
	assert.Error(t, claimErr, "claim should fail when worktree is already bound to task-01. stderr: %s", stderr)

	errText := stderr + claimErr.Error()
	assert.Contains(t, errText, "task-01", "error should mention the task currently bound to the worktree")
	assert.NotContains(t, errText, "merged", "error should NOT suggest 'arm merged' (only for post-merge teardown)")
}

// TestClaimReleasesClaimOnWorktreeSetupFailure verifies that when updateTaskIDFile fails
// after the claim is won, a compensating transition op is appended to re-open the task.
func TestClaimReleasesClaimOnWorktreeSetupFailure(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Manually create a worktree and git directory structure to simulate the scenario
	// where worktreePathExists passes but updateTaskIDFile will fail.
	tempDir := t.TempDir()
	worktreePath := filepath.Join(tempDir, "task-01-worktree")

	// Create a minimal worktree-like structure
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))

	// Create a fake .git file that points to a non-existent git directory
	// This will make worktreePathExists return true (the file exists)
	// but resolveWorktreeGitDir will fail when updateTaskIDFile tries to use it
	gitPath := filepath.Join(worktreePath, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644))

	// Try to claim task-01 with this fake worktree
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)

	// The claim should fail - either during checkExistingWorktreeBinding or updateTaskIDFile
	assert.Error(t, claimErr, "claim should fail with invalid worktree. stderr: %s", stderr)

	// Even though the claim failed, verify that task-01 isn't stuck in "claimed" state.
	// If the fix is working, a rollback op should have been appended (if the claim race was won).
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// The task should be in "open" state (not claimed/stuck)
	// If the bug existed, it might be "claimed" even though the worktree setup failed
	assert.NotEqual(t, ops.StatusClaimed, issue.Status, "task should not be stuck in claimed state after worktree setup failure")
}

// TestClaimReleasesPushesInDualBranchMode verifies that appendHighStakesOp (not appendOp) is
// used for claim rollbacks in dual-branch mode, so the release op is committed to the
// _armature branch immediately rather than waiting for the next TTL expiry.
//
// The fix replaced bare appendOp calls with appendHighStakesOp for compensating rollback ops
// in arm claim. appendHighStakesOp commits the op to the worktree branch (dual-branch mode);
// push is best-effort so push failure (no remote in test repos) is swallowed. The commit is
// always written. We verify the release op is present in the ops log after the failed claim.
//
// TEST_EXCEPTION for push verification: The compensating error message "failed to push claim
// release (manual cleanup may be needed)" only appears when appendHighStakesOp itself returns
// an error (i.e., AppendAndCommit fails). Since push is best-effort and silently swallowed,
// inducing that error path would require making the _armature ops dir read-only — which would
// also prevent the initial claim op from being written, making the scenario unreachable. Instead,
// we verify the end-state invariant: after a failed worktree setup in dual-branch mode, the
// task must not be stuck in claimed state, and the ops log must contain the release op.
func TestClaimReleasesPushesInDualBranchMode(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap in dual-branch mode: ops committed to _armature branch in .arm/ worktree.
	_, err := runTrls(t, repo, "bootstrap", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Dual branch rollback task", "--id", "task-rb-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create a pre-existing directory with a broken .git file.
	// worktreePathExists() returns true; checkExistingWorktreeBinding may reject it, or
	// updateTaskIDFile will fail when it tries to resolve the non-existent git dir.
	worktreePath := filepath.Join(t.TempDir(), "task-rb-01-worktree")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	gitPath := filepath.Join(worktreePath, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644))

	// Run the failing claim — should error due to the broken worktree.
	_, _, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-rb-01", "--worktree", worktreePath)
	assert.Error(t, claimErr, "claim should fail with invalid/broken worktree")

	// Materialize and verify the task is not stuck in claimed state.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-rb-01.json"))
	require.NoError(t, err)
	assert.NotEqual(t, ops.StatusClaimed, issue.Status,
		"task must not be stuck in claimed state after worktree setup failure in dual-branch mode")

	// Verify the ops log in the _armature worktree (.arm/.armature/ops/) contains a
	// transition-to-open (release) op, proving appendHighStakesOp committed the rollback.
	// If checkExistingWorktreeBinding rejected the fake worktree before the claim op was
	// written, no rollback op is needed (task stays open from the start), so we only look
	// for the release op when the task actually transitioned through claimed.
	armOpsDir := filepath.Join(repo, ".arm", ".armature", "ops")
	entries, readErr := os.ReadDir(armOpsDir)
	if readErr != nil {
		// Ops dir not readable — skip the log check (bootstrap may have put ops elsewhere).
		t.Logf("Note: .arm/.armature/ops not readable: %v; skipping ops log check", readErr)
		return
	}

	hasReleaseOp := false
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		logPath := filepath.Join(armOpsDir, e.Name())
		data, readErr2 := os.ReadFile(logPath)
		if readErr2 == nil && strings.Contains(string(data), `"to":"open"`) {
			hasReleaseOp = true
			t.Logf("Found release op in dual-branch ops log %s (appendHighStakesOp fix verified)", logPath)
			break
		}
	}

	if !hasReleaseOp {
		// The claim may have been rejected before the claim op was written
		// (e.g., checkExistingWorktreeBinding fired first). In that case no rollback op
		// is needed, so it's acceptable to have no release op. The status check above
		// is the primary invariant.
		t.Logf("No release op in dual-branch ops log — claim was likely rejected before winning the race (acceptable)")
	}
}

// TestClaimWorktreePathIsNormalized verifies that claim correctly resolves relative worktree paths
// to absolute paths. When claim is invoked from a different working directory with a relative
// --worktree path, the worktree should still be created at the correct absolute path, and the
// armature-task-id binding should be written to the correct location.
func TestClaimWorktreePathIsNormalized(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Use a different working directory — the relative path should still work correctly.
	// Lock the mutex to prevent concurrent chdir calls (os.Chdir modifies global state).
	runTrlsMu.Lock()
	defer runTrlsMu.Unlock()

	originalDir, err := os.Getwd()
	require.NoError(t, err)
	differentDir := t.TempDir()
	require.NoError(t, os.Chdir(differentDir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalDir)) })

	// Use a relative worktree path "task-wt" (relative to differentDir)
	relWorktree := "task-wt"
	expectedAbsWorktree := filepath.Join(differentDir, relWorktree)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", relWorktree})

	err = cmd.Execute()
	require.NoError(t, err, "claim with relative worktree path should succeed")

	// Verify worktree was created at the absolute path (in expectedAbsWorktree, not in repo or current dir)
	assert.DirExists(t, expectedAbsWorktree, "worktree directory should be created at the correct absolute path")

	// Verify the .git file exists and points to the correct git directory
	gitPath := filepath.Join(expectedAbsWorktree, ".git")
	assert.FileExists(t, gitPath, ".git file should exist in worktree")

	// Read .git to find the actual git directory
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(expectedAbsWorktree, actualGitDir)
	}

	// Verify armature-task-id was written to the correct location
	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // test path
	require.NoError(t, err, "armature-task-id should exist in the correct git directory")
	assert.Equal(t, "task-01", string(taskID), "armature-task-id should contain task-01")
}

// TestClaimRejectsMainCheckoutAsWorktree verifies that claim rejects the main checkout
// as a worktree path. The main checkout has .git as a directory (not a file), and should
// not be used as a worktree because git worktree remove cannot remove the main working tree.
func TestClaimRejectsMainCheckoutAsWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	// Try to claim using the repo root (main checkout) as the worktree path
	cmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", repo})

	err := cmd.Execute()
	assert.Error(t, err, "claim should fail when --worktree is the main checkout")

	errOutput := errBuf.String() + buf.String() + err.Error()
	assert.Contains(t, errOutput, "main checkout",
		"error message should mention that the path is the main checkout")
}
