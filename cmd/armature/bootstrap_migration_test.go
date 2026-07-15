package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
)

// TestMigrateDualBranchToCollapsed_NoLayout_REQ_LNGHZN_S1_T2 tests that migration is skipped if layout doesn't exist.
func TestMigrateDualBranchToCollapsed_NoLayout_REQ_LNGHZN_S1_T2(t *testing.T) {
	tmpDir := t.TempDir()

	// Perform migration on repo without dual-branch layout
	migrated, backupDir, err := migrateDualBranchToCollapsed(tmpDir)
	if err != nil {
		t.Fatalf("migration should not error when no layout exists: %v", err)
	}

	if migrated {
		t.Error("expected migration to be skipped, but it was performed")
	}

	if backupDir != "" {
		t.Errorf("expected no backup directory, got %s", backupDir)
	}
}

// TestUpdateGitExclude_REQ_LNGHZN_S1_T2 tests updating .git/info/exclude file.
func TestUpdateGitExclude_REQ_LNGHZN_S1_T2(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .git/info directory
	infoDir := filepath.Join(tmpDir, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o750); err != nil {
		t.Fatalf("failed to create .git/info directory: %v", err)
	}

	// Test 1: Add new pattern to empty exclude file
	if err := updateGitExclude(tmpDir, ".arm/", ""); err != nil {
		t.Fatalf("failed to add .arm/ to exclude: %v", err)
	}

	excludePath := filepath.Join(tmpDir, ".git", "info", "exclude")
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("failed to read exclude file: %v", err)
	}

	if !strings.Contains(string(content), ".arm/") {
		t.Errorf("expected .arm/ in exclude file, got: %s", string(content))
	}

	// Test 2: Add new pattern and remove old pattern
	if err := updateGitExclude(tmpDir, ".armature/", ".arm/"); err != nil {
		t.Fatalf("failed to update exclude: %v", err)
	}

	content, err = os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("failed to read exclude file: %v", err)
	}

	contentStr := string(content)
	if strings.Contains(contentStr, ".arm/") {
		t.Errorf("expected .arm/ to be removed from exclude, but it's still there: %s", contentStr)
	}

	if !strings.Contains(contentStr, ".armature/") {
		t.Errorf("expected .armature/ in exclude file, got: %s", contentStr)
	}

	// Test 3: Adding duplicate pattern is idempotent
	if err := updateGitExclude(tmpDir, ".armature/", ""); err != nil {
		t.Fatalf("failed to update exclude: %v", err)
	}

	content, err = os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("failed to read exclude file: %v", err)
	}

	contentStr = string(content)
	// Count occurrences of .armature/
	count := strings.Count(contentStr, ".armature/")
	if count != 1 {
		t.Errorf("expected .armature/ to appear once, but it appears %d times", count)
	}
}

// TestMigrateDualBranchToCollapsed_DirtyWorktree_REQ_LNGHZN_S1_T2 tests that migration refuses if worktree is dirty.
func TestMigrateDualBranchToCollapsed_DirtyWorktree_REQ_LNGHZN_S1_T2(t *testing.T) {
	tmpDir := t.TempDir()

	gitClient := adapters.New(tmpDir)

	// Configure git for testing (use environment variables for commits)
	os.Setenv("GIT_AUTHOR_NAME", "Test User")
	os.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	os.Setenv("GIT_COMMITTER_NAME", "Test User")
	os.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	// Initialize git repo with initial commit on main branch
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("failed to set git email: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("failed to set git name: %v", err)
	}
	readmeFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# Test Repo\n"), 0o600); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "add", "README.md").Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "initial commit").Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Initialize git repo with orphan branch
	if err := gitClient.CreateOrphanBranch("_armature"); err != nil {
		t.Fatalf("failed to create _armature branch: %v", err)
	}

	// Now configure local git config after repo is created
	if err := gitClient.SetGitConfig("user.email", "test@example.com"); err != nil {
		t.Fatalf("failed to set git user.email: %v", err)
	}
	if err := gitClient.SetGitConfig("user.name", "Test User"); err != nil {
		t.Fatalf("failed to set git user.name: %v", err)
	}

	// Create the dual-branch layout
	armWorktreePath := filepath.Join(tmpDir, ".arm")
	if err := gitClient.AddWorktree("_armature", armWorktreePath); err != nil {
		t.Fatalf("failed to create .arm worktree: %v", err)
	}

	// Create inner .armature/ structure
	innerArmaturePath := filepath.Join(armWorktreePath, config.StateDirName)
	opsDir := filepath.Join(innerArmaturePath, "ops")
	if err := os.MkdirAll(opsDir, 0o750); err != nil {
		t.Fatalf("failed to create ops directory: %v", err)
	}

	// Create a tracked file in the worktree to simulate dirty state
	testFile := filepath.Join(armWorktreePath, "tracked-file.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0o600); err != nil {
		t.Fatalf("failed to create tracked file: %v", err)
	}

	// Add and commit the file to make it tracked
	armGitClient := adapters.New(armWorktreePath)
	if err := armGitClient.AddPaths([]string{"tracked-file.txt"}); err != nil {
		t.Fatalf("failed to add tracked file: %v", err)
	}
	if err := armGitClient.CommitWorktreeOp("tracked-file.txt", "test commit"); err != nil {
		t.Fatalf("failed to commit tracked file: %v", err)
	}

	// Now modify the tracked file to make worktree dirty
	if err := os.WriteFile(testFile, []byte("modified content"), 0o600); err != nil {
		t.Fatalf("failed to modify tracked file: %v", err)
	}

	// Attempt migration (should fail)
	migrated, _, err := migrateDualBranchToCollapsed(tmpDir)
	if err == nil {
		t.Errorf("expected migration to fail with dirty worktree, but it succeeded")
	}

	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("expected error message to mention uncommitted changes, got: %v", err)
	}

	if migrated {
		t.Error("expected migration to not be performed when worktree is dirty")
	}

	// Verify .arm/ worktree still exists and is unchanged
	if _, err := os.Stat(filepath.Join(armWorktreePath, ".git")); os.IsNotExist(err) {
		t.Error("expected .arm/ worktree to still exist after failed migration")
	}

	// Verify new .armature/ worktree was not created
	newWorktreePath := filepath.Join(tmpDir, config.StateDirName)
	if _, err := os.Stat(filepath.Join(newWorktreePath, ".git")); err == nil {
		t.Error("expected new .armature/ worktree to not exist after failed migration")
	}
}
