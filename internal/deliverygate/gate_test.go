package deliverygate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanTreeCheck_REQ_LNGHZN_S4_T1 verifies that the clean tree check
// correctly detects when a worktree has no uncommitted changes.
func TestCleanTreeCheck_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	// Create a temporary git repository
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create and commit a file
	cleanFile := filepath.Join(tmpDir, "clean.txt")
	require.NoError(t, os.WriteFile(cleanFile, []byte("clean content"), 0644))
	runGit(t, tmpDir, "add", "clean.txt")
	runGit(t, tmpDir, "commit", "-m", "initial commit")

	// Test with a clean tree
	result := CleanTreeCheck(tmpDir)
	assert.True(t, result.Pass, "clean tree should pass")
	assert.Empty(t, result.Remediation, "clean tree should have no remediation")

	// Add an uncommitted change
	require.NoError(t, os.WriteFile(cleanFile, []byte("modified content"), 0644))

	// Test with dirty tree
	result = CleanTreeCheck(tmpDir)
	assert.False(t, result.Pass, "dirty tree should fail")
	assert.NotEmpty(t, result.Remediation, "dirty tree should have remediation message")
}

// TestScopeContainmentCheck_AllFilesWithinScope_REQ_LNGHZN_S4_T1 verifies that
// the scope containment check passes when all changed files are within scope.
func TestScopeContainmentCheck_AllFilesWithinScope_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create a base commit
	file1 := filepath.Join(tmpDir, "pkg", "file1.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file1), 0755))
	require.NoError(t, os.WriteFile(file1, []byte("package pkg\nvar X = 1"), 0644))
	runGit(t, tmpDir, "add", "pkg/file1.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a change within scope
	require.NoError(t, os.WriteFile(file1, []byte("package pkg\nvar X = 2"), 0644))
	runGit(t, tmpDir, "add", "pkg/file1.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST): modify file1")

	// Test scope containment
	result := ScopeContainmentCheck(tmpDir, baseCommit, []string{"pkg/**"})
	assert.True(t, result.Pass, "all files within scope should pass")
	assert.Empty(t, result.Remediation)
}

// TestScopeContainmentCheck_FileOutsideScope_REQ_LNGHZN_S4_T1 verifies that
// the scope containment check fails when a changed file is outside the declared scope.
func TestScopeContainmentCheck_FileOutsideScope_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create base commit
	file1 := filepath.Join(tmpDir, "pkg", "file1.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file1), 0755))
	require.NoError(t, os.WriteFile(file1, []byte("package pkg"), 0644))
	runGit(t, tmpDir, "add", "pkg/file1.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a change outside declared scope
	file2 := filepath.Join(tmpDir, "cmd", "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file2), 0755))
	require.NoError(t, os.WriteFile(file2, []byte("package main"), 0644))
	runGit(t, tmpDir, "add", "cmd/main.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST): add main")

	// Test with scope that excludes the new file
	result := ScopeContainmentCheck(tmpDir, baseCommit, []string{"pkg/**"})
	assert.False(t, result.Pass, "file outside scope should fail")
	assert.NotEmpty(t, result.Remediation)
	assert.Contains(t, result.Remediation, "cmd/main.go")
}

// TestCommitReferenceCheck_ValidConventionalCommit_REQ_LNGHZN_S4_T1 verifies that
// the commit reference check passes when at least one commit has the proper
// conventional-commit format with the issue ID in the scope.
func TestCommitReferenceCheck_ValidConventionalCommit_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create a base commit
	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a commit with proper conventional format
	require.NoError(t, os.WriteFile(file, []byte("modified"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): add feature")

	// Test commit reference
	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "valid conventional commit should pass")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_NoMatchingCommit_REQ_LNGHZN_S4_T1 verifies that
// the commit reference check fails when no commits match the conventional format.
func TestCommitReferenceCheck_NoMatchingCommit_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create a base commit
	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a commit WITHOUT proper conventional format (no issue ID in scope)
	require.NoError(t, os.WriteFile(file, []byte("modified"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "feat: generic feature")

	// Test commit reference - should fail
	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "commits without issue ID should fail")
	assert.NotEmpty(t, result.Remediation)
}

// TestDeliveryGate_IntegrationCheck_REQ_LNGHZN_S4_T1 verifies that the
// DeliveryGate function returns correct combined results for all three checks.
func TestDeliveryGate_IntegrationCheck_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Set up initial commit with scoped file
	scopedFile := filepath.Join(tmpDir, "pkg", "file.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(scopedFile), 0755))
	require.NoError(t, os.WriteFile(scopedFile, []byte("package pkg\nvar X = 1"), 0644))
	runGit(t, tmpDir, "add", "pkg/file.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a valid change with proper commit message
	require.NoError(t, os.WriteFile(scopedFile, []byte("package pkg\nvar X = 2"), 0644))
	runGit(t, tmpDir, "add", "pkg/file.go")
	runGit(t, tmpDir, "commit", "-m", "feat(ISSUE-001): valid change")

	// Test with valid gate parameters
	gate := DeliveryGate(tmpDir, "ISSUE-001", baseCommit, []string{"pkg/**"})

	assert.True(t, gate.CleanTree.Pass, "tree is clean after commit")
	assert.True(t, gate.ScopeContainment.Pass, "all files within scope")
	assert.True(t, gate.CommitReference.Pass, "commit has proper format")
}

// Helper functions

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	// Disable commit signing to avoid GPG issues in tests
	runGit(t, dir, "config", "commit.gpgsign", "false")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
}

func getHeadSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	require.NoError(t, err, "git rev-parse HEAD failed")
	return string(output[:len(output)-1]) // trim newline
}
