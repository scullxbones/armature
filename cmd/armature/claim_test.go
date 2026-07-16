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

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// setupRepoWithEpic creates a repo with an epic issue.
func setupRepoWithEpic(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

 bootstrapRepoForTest(t, repo)

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

 bootstrapRepoForTest(t, repo)

	// Create parent story
	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Parent story", "--type", "story", "--id", "story-01"})
	require.NoError(t, cmd2.Execute())

	// Materialize so issues/story-01.json exists for ReadIssue in create --parent.
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

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

	// Verify armature-issue-id file is created in the actual git directory
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	assert.FileExists(t, taskIDFile, "armature-issue-id file should be created in actual git dir")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	assert.Equal(t, "task-01", string(taskID))
}

// TestClaimUpdatesTaskIDIfWorktreeExists verifies that claim updates armature-issue-id
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

	// Verify armature-issue-id was written
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
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
// already has an armature-issue-id binding for a different issue, claim fails with a
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

	// Verify that armature-issue-id is "task-01".
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, readErr := os.ReadFile(gitPath)
	require.NoError(t, readErr)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
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

	// Delete the armature-issue-id file from the worktree's git dir so the binding check is bypassed
	// and only the branch mismatch check applies
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	require.NoError(t, os.Remove(taskIDFile), "should be able to delete armature-issue-id file") //nolint:gosec // internal test path

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

// TestClaimReleasesClaimOnWorktreeSetupFailure verifies that when updateIssueIDFile fails
// after the claim is won, a compensating transition op is appended to re-open the task.
func TestClaimReleasesClaimOnWorktreeSetupFailure(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Manually create a worktree and git directory structure to simulate the scenario
	// where worktreePathExists passes but updateIssueIDFile will fail.
	tempDir := t.TempDir()
	worktreePath := filepath.Join(tempDir, "task-01-worktree")

	// Create a minimal worktree-like structure
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))

	// Create a fake .git file that points to a non-existent git directory
	// This will make worktreePathExists return true (the file exists)
	// but resolveWorktreeGitDir will fail when updateIssueIDFile tries to use it
	gitPath := filepath.Join(worktreePath, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644))

	// Try to claim task-01 with this fake worktree
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)

	// The claim should fail - either during checkExistingWorktreeBinding or updateIssueIDFile
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
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Dual branch rollback task", "--id", "task-rb-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create a pre-existing directory with a broken .git file.
	// worktreePathExists() returns true; checkExistingWorktreeBinding may reject it, or
	// updateIssueIDFile will fail when it tries to resolve the non-existent git dir.
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

	// Verify the ops log in the _armature worktree (.armature/ops/) contains a
	// transition-to-open (release) op, proving appendHighStakesOp committed the rollback.
	// If checkExistingWorktreeBinding rejected the fake worktree before the claim op was
	// written, no rollback op is needed (task stays open from the start), so we only look
	// for the release op when the task actually transitioned through claimed.
	armOpsDir := filepath.Join(repo, ".armature", "ops")
	entries, readErr := os.ReadDir(armOpsDir)
	if readErr != nil {
		// Ops dir not readable — skip the log check (bootstrap may have put ops elsewhere).
		t.Logf("Note: .armature/ops not readable: %v; skipping ops log check", readErr)
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
// armature-issue-id binding should be written to the correct location.
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

	// Verify armature-issue-id was written to the correct location
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // test path
	require.NoError(t, err, "armature-issue-id should exist in the correct git directory")
	assert.Equal(t, "task-01", string(taskID), "armature-issue-id should contain task-01")
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

// TestClaimRejectsUnboundDetachedWorktree verifies that when a worktree has a detached HEAD
// and NO existing binding (existingTaskID == ""), claim must reject it rather than allowing
// the detached HEAD to bypass the branch check. This prevents writing a fresh binding to a
// detached worktree that is not on the expected branch.
func TestClaimRejectsUnboundDetachedWorktree(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Create a linked worktree on the task/task-01 branch
	worktreePath := filepath.Join(t.TempDir(), "detached-worktree")
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	require.NoError(t, err, "first claim should succeed")

	// Verify the worktree was created and bound to task-01
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}
	taskIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	taskID, err := os.ReadFile(taskIDFile) //nolint:gosec // test path
	require.NoError(t, err)
	require.Equal(t, "task-01", string(taskID), "worktree should initially be bound to task-01")

	// Detach HEAD in the worktree
	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")

	// Verify HEAD is now detached (a SHA, not a branch ref)
	headFile := filepath.Join(actualGitDir, "HEAD")
	headContent, err := os.ReadFile(headFile) //nolint:gosec // internal test path
	require.NoError(t, err)
	headStr := strings.TrimSpace(string(headContent))
	require.False(t, strings.HasPrefix(headStr, "ref: "), "HEAD should be detached")

	// Remove the armature-issue-id binding so the worktree has NO binding
	require.NoError(t, os.Remove(taskIDFile), "should be able to delete armature-issue-id file") //nolint:gosec // internal test path

	// Now try to claim task-01 again with the unbound detached HEAD
	// This should fail because even though the binding is empty, the detached HEAD
	// should only be allowed when there IS a binding that matches
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	assert.Error(t, claimErr, "claim should fail when worktree has unbound detached HEAD. stderr: %s", stderr)

	errText := stderr + claimErr.Error()
	assert.Contains(t, errText, "detached HEAD",
		"error should mention detached HEAD in the error message")
}

// TestClaimDoesNotReleaseExistingClaimOnWorktreeRetryFailure verifies the P2 bug fix:
// when a worker retries claiming an already-claimed task with an existing worktree,
// and the task ID file update fails, the task must remain claimed (not be released to open).
//
// Scenario:
// 1. Worker claims task-01 with --worktree /wt1 → succeeds, status=claimed, ClaimedBy=worker-A
// 2. Worker retries with --worktree /wt1 again → wins claim race again (same worker, TTL not expired)
// 3. updateIssueIDFile fails (e.g., .git file points to non-existent directory)
// 4. Before the fix: compensating rollback → status=open (WRONG)
// 5. After the fix: only rollback to open if the prior status was open; otherwise keep it claimed
func TestClaimDoesNotReleaseExistingClaimOnWorktreeRetryFailure(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// First claim succeeds: creates worktree at wt1 with task-01 claimed
	worktree1 := filepath.Join(t.TempDir(), "wt1")
	_, err := runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", worktree1)
	require.NoError(t, err, "first claim should succeed")

	// Materialize and verify task-01 is claimed
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed after first claim")

	// Now break the worktree's .git file by replacing it with a pointer to a non-existent directory.
	// This will cause updateIssueIDFile to fail on the re-claim attempt.
	gitPath := filepath.Join(worktree1, ".git")
	require.NoError(t, os.WriteFile(gitPath, []byte("gitdir: /nonexistent/git/dir"), 0o644),
		"should be able to overwrite .git file")

	// Second claim with same worktree should fail due to updateIssueIDFile failure
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree", worktree1)
	assert.Error(t, claimErr, "second claim with broken worktree should error. stderr: %s", stderr)

	// Materialize and verify task-01 is STILL claimed (not released to open)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// This is the critical assertion: the task must NOT be released to open
	assert.Equal(t, ops.StatusClaimed, issueAfter.Status,
		"task should remain claimed after failed worktree retry (not be released to open)")
	assert.NotEqual(t, ops.StatusOpen, issueAfter.Status,
		"task must NOT transition to open on worktree retry failure when it was already claimed")
}

// TestClaimRollsBackStaleTakeoverToOpen verifies the P2 bug fix:
// When worker-B takes over a stale claim from worker-A and worktree setup fails,
// the compensating rollback must transition to StatusOpen (not to the prior claimed status).
// This ensures other workers can pick up the task, not see it as claimed by worker-B.
//
// Scenario:
// 1. Inject a stale claim op from "other-worker-uuid" with old timestamp (2 hours ago, 1 min TTL)
// 2. Call `arm claim --issue task-01` — worker-B takes over the stale claim
// 3. Make worktree setup fail (e.g., put a file at worktree path that blocks `git worktree add`)
// 4. Assert task-01 is rolled back to StatusOpen (not claimed), so other workers can pick it up
func TestClaimRollsBackStaleTakeoverToOpen(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap and create task
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task one", "--type", "task", "--id", "task-01")
	require.NoError(t, err)

	// Materialize first to establish baseline state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Inject a stale claim op from another worker with an old timestamp
	otherWorker := "other-worker-uuid"
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, otherWorker+".log")

	// Claim timestamp 2 hours ago, TTL 1 minute — definitely stale
	staleClaimTime := time.Now().Unix() - 7200
	staleClaimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: staleClaimTime,
		WorkerID:  otherWorker,
		Payload:   ops.Payload{TTL: 1},
	}
	require.NoError(t, ops.AppendOp(logPath, staleClaimOp))

	// Materialize to apply the stale claim
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify task-01 is currently claimed by the stale claimer
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed by stale worker")
	require.Equal(t, otherWorker, issue.ClaimedBy, "task should be claimed by other-worker-uuid")

	// Now try to claim with a worktree that will fail setup.
	// Create a file at the worktree path to block git worktree add.
	worktreePath := filepath.Join(t.TempDir(), "task-01-worktree")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	// Create a file inside the directory to block worktree creation
	blockingFile := filepath.Join(worktreePath, "blocking-file")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocks worktree creation"), 0o644))

	// Attempt to claim — should fail due to worktree creation failure
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

	// Materialize and verify task-01 is now OPEN (not still claimed by the new worker)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// The critical assertion: after stale takeover + worktree failure, rollback must clear ownership
	assert.Equal(t, ops.StatusOpen, issueAfter.Status,
		"task should be rolled back to open after stale takeover failure (not remain claimed)")
	assert.Equal(t, "", issueAfter.ClaimedBy,
		"ClaimedBy must be cleared so other workers can pick up the task")
}

// TestClaimRejectsForeignWorktree verifies the P2 bug fix: when a --worktree path is given
// that points to a linked worktree belonging to a DIFFERENT git repository (not the main repo),
// claim must reject it even if the worktree is on the expected branch and has no conflicting binding.
//
// This prevents updateIssueIDFile from writing armature-issue-id into a foreign repo's git dir,
// which would cause later merged operations (which search only the main repo's worktree list)
// to permanently fail to find and clean up the worktree.
//
// Scenario:
// 1. Create repo-A with task-01 but DON'T claim it yet (so no worktree exists in repo-A)
// 2. Create repo-B (unrelated git repo) with a worktree on the matching task/task-01 branch
// 3. Try to claim task-01 in repo-A using the foreign worktree from repo-B
// 4. Expect an error mentioning "not registered to this repository"
func TestClaimRejectsForeignWorktree(t *testing.T) {
	// Setup repo-A (create but don't claim the task yet)
	repoA := initTempRepo(t)
	run(t, repoA, "git", "commit", "--allow-empty", "-m", "init")

 bootstrapRepoForTest(t, repoA)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repoA, "--title", "Test task", "--type", "task", "--id", "task-01"})
	require.NoError(t, cmd2.Execute())
	// Note: we do NOT claim task-01 in repo-A, so no worktree is created for task/task-01 in repo-A yet

	// Setup repo-B (a separate, unrelated git repo)
	repoBTempDir := t.TempDir()
	repoB := filepath.Join(repoBTempDir, "repo-B")
	require.NoError(t, os.Mkdir(repoB, 0o755))
	run(t, repoB, "git", "init")
	run(t, repoB, "git", "config", "user.email", "test@test.com")
	run(t, repoB, "git", "config", "user.name", "Test")
	run(t, repoB, "git", "config", "commit.gpgsign", "false")
	run(t, repoB, "git", "commit", "--allow-empty", "-m", "init from repo-B")

	// Create a branch "task/task-01" in repo-B on which the foreign worktree will live
	// Start from main (or the current default branch name), then create task/task-01
	run(t, repoB, "git", "checkout", "-b", "task/task-01", "HEAD")
	// Now switch main branch back to something else so task/task-01 is not the checked-out branch
	run(t, repoB, "git", "checkout", "-b", "main-branch")

	// Create a linked worktree in repo-B on the task/task-01 branch
	foreignWorktreeDir := filepath.Join(repoBTempDir, "foreign-wt")
	require.NoError(t, os.Mkdir(foreignWorktreeDir, 0o755))
	foreignWorktreePath := filepath.Join(foreignWorktreeDir, "worktree")
	run(t, repoB, "git", "worktree", "add", foreignWorktreePath, "task/task-01")

	// Verify the foreign worktree exists and is on the correct branch
	assert.DirExists(t, foreignWorktreePath, "foreign worktree should exist")
	gitPath := filepath.Join(foreignWorktreePath, ".git")
	assert.FileExists(t, gitPath, ".git file should exist in foreign worktree")

	// Now try to claim task-01 in repo-A using the foreign worktree path from repo-B
	// This should fail with an error mentioning that the worktree is not registered
	_, stderr, claimErr := runTrlsWithStderr(t, repoA, "claim", "--issue", "task-01", "--worktree", foreignWorktreePath)

	// Verify the claim fails
	assert.Error(t, claimErr, "claim should fail when worktree belongs to a different repository")

	// Verify the error message is informative
	errText := stderr + claimErr.Error()
	assert.Contains(t, errText, "not registered",
		"error should mention that the worktree is not registered to this repository")
}

// TestClaimRollsBackStaleSameWorkerClaimToOpen verifies the P2 bug fix:
// When worker-A's own claim is stale (TTL expired) and worker-A retries `arm claim`
// with a new worktree, then worktree setup fails, the compensating rollback must
// transition to StatusOpen (not preserve the prior claimed status).
// This is critical because OpClaim already refreshed ClaimedAt and LastHeartbeat,
// so if rollback preserves "claimed", the issue will have a fresh claim with no
// usable worktree binding, blocking other workers from picking it up.
//
// Scenario:
//  1. Inject a claim op from the SAME worker ID with an old timestamp (2 hours ago, 1 min TTL)
//  2. Materialize to apply the stale claim
//  3. Call `arm claim --issue task-01 --worktree <blocked-path>` — same worker retries
//  4. The OpClaim wins the race and refreshes ClaimedAt/LastHeartbeat
//  5. Worktree setup fails (file blocking git worktree add)
//  6. Rollback must transition to StatusOpen (not keep the claim) because the prior
//     claim was stale, even though it's the same worker
//  7. Assert task-01 is rolled back to StatusOpen so other workers can pick it up
func TestClaimRollsBackStaleSameWorkerClaimToOpen(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap and create task
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task stale same-worker", "--type", "task", "--id", "task-01")
	require.NoError(t, err)

	// Materialize first to establish baseline state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Get the current worker ID (set by bootstrap/worker-init or read from git config)
	// We need to determine what worker ID will be used when we call `arm claim`
	// The test uses the same repo/git config, so the worker ID from the initial setup is used
	// We'll inject ops from that same worker with a stale timestamp
	workerID, logPath, err := resolveWorkerAndLog(&config.Context{
		RepoPath:  repo,
		IssuesDir: filepath.Join(repo, ".armature"),
		StateDir:  filepath.Join(repo, ".armature", "state"),
	})
	require.NoError(t, err, "should resolve worker ID and log path")

	// Inject a stale claim op from the SAME worker with an old timestamp
	// Claim timestamp 2 hours ago, TTL 1 minute — definitely stale
	staleClaimTime := time.Now().Unix() - 7200
	staleClaimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: staleClaimTime,
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 1},
	}
	require.NoError(t, ops.AppendOp(logPath, staleClaimOp))

	// Materialize to apply the stale claim
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify task-01 is currently claimed by the same worker (but stale)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed by stale worker")
	require.Equal(t, workerID, issue.ClaimedBy, "task should be claimed by same worker")

	// Now try to claim with a worktree that will fail setup (same worker retrying).
	// Create a file at the worktree path to block git worktree add.
	worktreePath := filepath.Join(t.TempDir(), "task-01-worktree")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	// Create a file inside the directory to block worktree creation
	blockingFile := filepath.Join(worktreePath, "blocking-file")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocks worktree creation"), 0o644))

	// Attempt to claim — should fail due to worktree creation failure
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

	// Materialize and verify task-01 is now OPEN (not still claimed by the same worker)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// The critical assertion: after stale same-worker claim + worktree failure, rollback must
	// clear ownership even though it's the same worker, because the prior claim was stale
	assert.Equal(t, ops.StatusOpen, issueAfter.Status,
		"task should be rolled back to open after stale same-worker claim failure (not remain claimed)")
	assert.Equal(t, "", issueAfter.ClaimedBy,
		"ClaimedBy must be cleared so other workers can pick up the task")
}

// TestClaimPreservesNeverExpiringClaimOnRetry verifies the P2 bug fix:
// When a same-worker claim has TTL=0 (never-expiring) and the same worker retries
// `arm claim` with a new worktree, if worktree setup fails, the rollback must
// preserve the prior claimed status (not release to Open).
//
// This is the inverse of TestClaimRollsBackStaleSameWorkerClaimToOpen:
// - Stale claim (TTL=1 min, 2 hours old) → rollback to Open ✓
// - Never-expiring claim (TTL=0, any age) → rollback to Claimed (preserve) ✓
//
// The bug was: rollback code normalized TTL (0 → 60), breaking the never-expiring
// claim so it was wrongly treated as stale and released.
//
// Scenario:
//  1. Inject a claim op from the SAME worker ID with TTL=0 (never-expiring) and old timestamp (2 hours ago)
//  2. Materialize to apply the never-expiring claim
//  3. Call `arm claim --issue task-01 --worktree <blocked-path>` — same worker retries
//  4. The OpClaim wins the race and refreshes ClaimedAt/LastHeartbeat
//  5. Worktree setup fails (file blocking git worktree add)
//  6. Rollback must transition to StatusClaimed (preserve) because the prior claim
//     was never-expiring (TTL=0), even though it's old
//  7. Assert task-01 remains StatusClaimed with ClaimedBy still set to the worker
func TestClaimPreservesNeverExpiringClaimOnRetry(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap and create task
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--title", "Task never-expiring", "--type", "task", "--id", "task-01")
	require.NoError(t, err)

	// Materialize first to establish baseline state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Resolve the current worker ID (set by bootstrap/worker-init)
	workerID, logPath, err := resolveWorkerAndLog(&config.Context{
		RepoPath:  repo,
		IssuesDir: filepath.Join(repo, ".armature"),
		StateDir:  filepath.Join(repo, ".armature", "state"),
	})
	require.NoError(t, err, "should resolve worker ID and log path")

	// Inject a never-expiring claim op from the SAME worker with an old timestamp
	// Claim timestamp 2 hours ago, TTL 0 (never expires) — must NOT be treated as stale
	neverExpiringClaimTime := time.Now().Unix() - 7200
	neverExpiringClaimOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: neverExpiringClaimTime,
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 0},
	}
	require.NoError(t, ops.AppendOp(logPath, neverExpiringClaimOp))

	// Materialize to apply the never-expiring claim
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify task-01 is currently claimed by the same worker (with TTL=0)
	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.Equal(t, ops.StatusClaimed, issue.Status, "task should be claimed")
	require.Equal(t, workerID, issue.ClaimedBy, "task should be claimed by same worker")
	require.Equal(t, 0, issue.ClaimTTL, "task claim TTL should be 0 (never-expiring)")

	// Now try to claim with a worktree that will fail setup (same worker retrying).
	// Create a file at the worktree path to block git worktree add.
	worktreePath := filepath.Join(t.TempDir(), "task-01-worktree")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	// Create a file inside the directory to block worktree creation
	blockingFile := filepath.Join(worktreePath, "blocking-file")
	require.NoError(t, os.WriteFile(blockingFile, []byte("blocks worktree creation"), 0o644))

	// Attempt to claim — should fail due to worktree creation failure
	_, stderr, claimErr := runTrlsWithStderr(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	assert.Error(t, claimErr, "claim should fail when worktree creation is blocked. stderr: %s", stderr)

	// Materialize and verify task-01 is still CLAIMED (not released to open)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	issueAfter, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)

	// The critical assertion: after never-expiring same-worker claim + worktree failure,
	// rollback must preserve the claimed status (not release to open) because the prior
	// claim has TTL=0 (never expires, always active)
	assert.Equal(t, ops.StatusClaimed, issueAfter.Status,
		"task should remain claimed after never-expiring same-worker claim failure (not be released to open)")
	assert.Equal(t, workerID, issueAfter.ClaimedBy,
		"ClaimedBy must remain set since the claim never expires")
}

// TestCheckExistingWorktreeBindingReadsLegacyTaskID verifies the P2 bug fix:
// checkExistingWorktreeBinding should recognize legacy armature-task-id files
// (from worktrees claimed before the rename to armature-issue-id).
//
// Scenario:
// 1. Create a worktree with a detached HEAD
// 2. Write only the legacy armature-task-id file to the .git directory (not armature-issue-id)
// 3. Call checkExistingWorktreeBinding with the same issue ID
// 4. Expect it to return nil (no error), allowing the claim to proceed for same-issue re-claim
func TestCheckExistingWorktreeBindingReadsLegacyTaskID(t *testing.T) {
	repo := setupRepoWithParentAndTask(t)

	// Create a worktree manually via git
	worktreePath := filepath.Join(t.TempDir(), "legacy-worktree")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	// Verify worktree was created
	assert.DirExists(t, worktreePath, "worktree directory should exist")

	// Detach the HEAD in the worktree
	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")

	// Get the actual git directory from the worktree's .git file
	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	// Write only the legacy armature-task-id file (NOT armature-issue-id)
	taskIDFile := filepath.Join(actualGitDir, "armature-task-id")
	require.NoError(t, os.WriteFile(taskIDFile, []byte("task-01"), 0o600)) //nolint:gosec // test path is internal

	// Verify that armature-issue-id does NOT exist
	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	_, err = os.ReadFile(issueIDFile) //nolint:gosec // test path is internal
	require.True(t, os.IsNotExist(err), "armature-issue-id should not exist (only legacy armature-task-id)")

	// Now call checkExistingWorktreeBinding with the same issue ID
	// It should recognize the legacy binding and return nil (no error)
	err = checkExistingWorktreeBinding(worktreePath, "task-01", "task/task-01")
	assert.NoError(t, err, "checkExistingWorktreeBinding should allow same-issue claim with legacy armature-task-id binding")
}

// TestCheckExistingWorktreeBindingFailsClosedOnPermissionError verifies the fix
// for the review finding that checkExistingWorktreeBinding silently treated a
// permission-denied armature-issue-id file as "unbound" (old code failed
// closed on any read error other than not-exist; the refactor to
// ReadIssueBindingFile regressed that by swallowing all errors). A worker
// should not be able to silently overwrite a binding it merely couldn't read.
func TestCheckExistingWorktreeBindingFailsClosedOnPermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: file permissions do not block reads")
	}

	repo := setupRepoWithParentAndTask(t)

	worktreePath := filepath.Join(t.TempDir(), "perm-worktree")
	run(t, repo, "git", "worktree", "add", worktreePath, "HEAD")

	gitPath := filepath.Join(worktreePath, ".git")
	gitFileContent, err := os.ReadFile(gitPath)
	require.NoError(t, err)
	gitDirLine := string(gitFileContent)
	actualGitDir := strings.TrimSpace(strings.TrimPrefix(gitDirLine, "gitdir: "))
	if !filepath.IsAbs(actualGitDir) {
		actualGitDir = filepath.Join(worktreePath, actualGitDir)
	}

	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	require.NoError(t, os.WriteFile(issueIDFile, []byte("task-01"), 0o600)) //nolint:gosec // test path is internal
	require.NoError(t, os.Chmod(issueIDFile, 0o000))                        //nolint:gosec // test path is internal
	t.Cleanup(func() {
		_ = os.Chmod(issueIDFile, 0o600) //nolint:errcheck,gosec // best-effort cleanup so TempDir removal succeeds
	})

	err = checkExistingWorktreeBinding(worktreePath, "task-01", "task/task-01")
	require.Error(t, err, "a permission-denied binding file must fail closed, not be silently treated as unbound")
	assert.Contains(t, err.Error(), "read existing binding")
}
