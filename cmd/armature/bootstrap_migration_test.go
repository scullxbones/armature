package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, os.Setenv("GIT_AUTHOR_NAME", "Test User"))
	require.NoError(t, os.Setenv("GIT_AUTHOR_EMAIL", "test@example.com"))
	require.NoError(t, os.Setenv("GIT_COMMITTER_NAME", "Test User"))
	require.NoError(t, os.Setenv("GIT_COMMITTER_EMAIL", "test@example.com"))

	// Initialize git repo with initial commit on main branch
	if err := exec.CommandContext(context.Background(), "git", "-C", tmpDir, "init").Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}
	if err := exec.CommandContext(context.Background(), "git", "-C", tmpDir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("failed to set git email: %v", err)
	}
	if err := exec.CommandContext(context.Background(), "git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("failed to set git name: %v", err)
	}
	if err := exec.CommandContext(context.Background(), "git", "-C", tmpDir, "config", "commit.gpgsign", "false").Run(); err != nil {
		t.Fatalf("failed to disable gpgsign: %v", err)
	}
	readmeFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# Test Repo\n"), 0o600); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	if err := exec.CommandContext(context.Background(), "git", "-C", tmpDir, "add", "README.md").Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}
	if err := exec.CommandContext(context.Background(), "git", "-C", tmpDir, "commit", "-m", "initial commit").Run(); err != nil {
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

// setupDualBranchFixtureForSourcesDebris creates a dual-branch .arm/.armature
// worktree with a committed ops/ file (so migration has real legacy ops data
// to carry forward) and returns the repo root and the ops worktree path. The
// caller is responsible for introducing whatever dirty state the test needs
// on top of this clean, committed baseline.
func setupDualBranchFixtureForSourcesDebris(t *testing.T) (repo string, armWorktreePath string) {
	t.Helper()
	repo = initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.CreateOrphanBranch("_armature"))
	armWorktreePath = filepath.Join(repo, ".arm")
	require.NoError(t, gitClient.AddWorktree("_armature", armWorktreePath))

	innerArmaturePath := filepath.Join(armWorktreePath, config.StateDirName)
	opsDir := filepath.Join(innerArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(opsDir, "existing-issue.json"), []byte(`{"id":"existing"}`), 0o600))

	sourcesDir := filepath.Join(innerArmaturePath, "sources")
	require.NoError(t, os.MkdirAll(sourcesDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), []byte(`{"sources":{}}`), 0o600))

	armGitClient := adapters.New(armWorktreePath)
	require.NoError(t, armGitClient.AddPaths([]string{config.StateDirName}))
	require.NoError(t, armGitClient.CommitWorktreeOp(config.StateDirName, "chore: simulate pre-existing dual-branch layout"))
	require.NoError(t, gitClient.SetGitConfig("armature.ops-worktree-path", armWorktreePath))
	return repo, armWorktreePath
}

// TestMigrateDualBranchToCollapsedReconcilesSourcesOnlyDebris verifies the
// LNGHZN-B1 RCA remediation: an ops worktree that is dirty ONLY under
// .armature/sources/ (the debris pre-LNGHZN-B1 `arm sources add/sync` left
// uncommitted) is reconciled with a commit instead of refusing to migrate
// forever. Without this, real clones that ran `arm sources add/sync` before
// commit 217022ea are permanently blocked from the dual-branch->collapsed
// migration.
func TestMigrateDualBranchToCollapsedReconcilesSourcesOnlyDebris(t *testing.T) {
	repo, armWorktreePath := setupDualBranchFixtureForSourcesDebris(t)

	sourcesDir := filepath.Join(armWorktreePath, config.StateDirName, "sources")

	// Simulate pre-LNGHZN-B1 debris: cache files written directly to disk,
	// never committed (no FileCommitter wired before the fix).
	require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "src-a.cache"), []byte("cached content a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "src-b.cache"), []byte("cached content b"), 0o600))
	// And a modification to the already-committed manifest.json, also never
	// committed — matching the RCA's "10 untracked .cache files + modified
	// manifest.json" description of real debris.
	require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), []byte(`{"sources":{"a":{}}}`), 0o600))

	migrated, backupDir, err := migrateDualBranchToCollapsed(repo)
	require.NoError(t, err, "migration must reconcile pure sources debris instead of refusing")
	require.True(t, migrated)
	require.NotEmpty(t, backupDir)

	// Migration must have actually happened: collapsed worktree exists with
	// both the reconciled sources files and the pre-existing ops data.
	newWorktreePath := filepath.Join(repo, config.StateDirName)
	assert.DirExists(t, newWorktreePath)
	assert.FileExists(t, filepath.Join(newWorktreePath, "sources", "src-a.cache"))
	assert.FileExists(t, filepath.Join(newWorktreePath, "sources", "src-b.cache"))
	assert.FileExists(t, filepath.Join(newWorktreePath, "ops", "existing-issue.json"))

	// The reconciliation commit must be visible in the _armature branch's
	// history, not merely present on disk uncommitted.
	logOut := runOutput(t, repo, "log", "_armature", "--oneline")
	assert.Contains(t, logOut, "reconcile pre-LNGHZN-B1 uncommitted sources state",
		"the sources debris reconciliation must be committed to the _armature branch")

	// The collapsed worktree must end up clean (the reconciled files, plus the
	// migration's own flatten commit, leave nothing uncommitted).
	status := strings.TrimSpace(runOutput(t, newWorktreePath, "status", "--porcelain"))
	assert.Empty(t, status, "collapsed worktree must be clean after reconciling sources debris")
}

// TestMigrateDualBranchToCollapsedRefusesWhenNonSourcesPathAlsoDirty verifies
// that the sources-debris reconciliation carve-out is narrow: if a *tracked*
// dirty path falls outside .armature/sources/ — even alongside genuine sources
// debris — migration still refuses with the original message, rather than
// silently committing unrelated uncommitted work. (An untracked non-sources
// path, by contrast, is tolerated: see
// TestMigrateDualBranchToCollapsedTreatsUntrackedNonSourcesDebrisTheSameAsBefore
// for why that tolerance must be preserved.)
func TestMigrateDualBranchToCollapsedRefusesWhenNonSourcesPathAlsoDirty(t *testing.T) {
	repo, armWorktreePath := setupDualBranchFixtureForSourcesDebris(t)

	sourcesDir := filepath.Join(armWorktreePath, config.StateDirName, "sources")
	require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "src-a.cache"), []byte("cached content a"), 0o600))

	// Modify the already-committed ops/existing-issue.json without committing:
	// a tracked, non-sources dirty path.
	opsFile := filepath.Join(armWorktreePath, config.StateDirName, "ops", "existing-issue.json")
	require.NoError(t, os.WriteFile(opsFile, []byte(`{"id":"existing","modified":true}`), 0o600))

	migrated, backupDir, err := migrateDualBranchToCollapsed(repo)
	require.Error(t, err, "migration must still refuse when a tracked non-sources path is dirty")
	assert.Contains(t, err.Error(), "uncommitted changes")
	assert.False(t, migrated)
	assert.Empty(t, backupDir)

	// The .arm/ worktree must remain untouched: no new .armature/ worktree, and
	// the sources debris must still be sitting there uncommitted (not silently
	// swept up into a partial commit).
	newWorktreePath := filepath.Join(repo, config.StateDirName)
	assert.False(t, pathExists(filepath.Join(newWorktreePath, ".git")), "no collapsed worktree should be created on refusal")
	status := strings.TrimSpace(runOutput(t, armWorktreePath, "status", "--porcelain"))
	assert.Contains(t, status, "src-a.cache")
	assert.Contains(t, status, "existing-issue.json")
}

// TestMigrateDualBranchToCollapsedTreatsUntrackedNonSourcesDebrisTheSameAsBefore
// verifies that an untracked file outside .armature/sources/ does not itself
// block migration, preserving IsWorkingTreeDirty's pre-existing behavior of
// never treating untracked files as dirty. This matters beyond the sources
// carve-out: runRepoSetup's own chained migrateDualBranchToCollapsed call
// (LNGHZN-S1-T3) runs immediately after writing fresh, not-yet-committed
// .gitignore/SCHEMA/hook-template scaffolding into this same worktree, so
// treating any untracked path as refusal-worthy would break that convergence.
func TestMigrateDualBranchToCollapsedTreatsUntrackedNonSourcesDebrisTheSameAsBefore(t *testing.T) {
	repo, armWorktreePath := setupDualBranchFixtureForSourcesDebris(t)

	// An untracked file outside sources/, mirroring the untracked scaffolding
	// runRepoSetup leaves behind before its chained migration call.
	require.NoError(t, os.WriteFile(filepath.Join(armWorktreePath, "SCHEMA-like-scaffolding.txt"), []byte("scaffold"), 0o600))

	migrated, backupDir, err := migrateDualBranchToCollapsed(repo)
	require.NoError(t, err, "an untracked non-sources path must not block migration")
	assert.True(t, migrated)
	assert.NotEmpty(t, backupDir)
}

// TestMigrateDualBranchToCollapsedClearsStaleArmatureModeConfig verifies that
// a stale "armature.mode = dual-branch" git config value (written by older
// builds; nothing in the current codebase reads or writes this key anymore)
// is cleared once the migration to the collapsed layout succeeds, so a
// collapsed repo doesn't carry forward a config key describing a layout it no
// longer has.
func TestMigrateDualBranchToCollapsedClearsStaleArmatureModeConfig(t *testing.T) {
	repo, _ := setupDualBranchFixtureForSourcesDebris(t)

	gitClient := adapters.New(repo)
	require.NoError(t, gitClient.SetGitConfig("armature.mode", "dual-branch"))

	migrated, _, err := migrateDualBranchToCollapsed(repo)
	require.NoError(t, err)
	require.True(t, migrated)

	_, err = gitClient.ReadGitConfig("armature.mode")
	assert.Error(t, err, "armature.mode should be cleared after a successful collapse migration")
}
