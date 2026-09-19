package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
)

func TestMigrateDualBranchToCollapsed_NoLayout_REQ_LNGHZN_S1_T2(t *testing.T) {
	tmpDir := t.TempDir()

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

func TestUpdateGitExclude_REQ_LNGHZN_S1_T2(t *testing.T) {
	tmpDir := t.TempDir()

	infoDir := filepath.Join(tmpDir, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o750); err != nil {
		t.Fatalf("failed to create .git/info directory: %v", err)
	}

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

	if err := updateGitExclude(tmpDir, ".armature/", ""); err != nil {
		t.Fatalf("failed to update exclude: %v", err)
	}

	content, err = os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("failed to read exclude file: %v", err)
	}

	contentStr = string(content)
	count := strings.Count(contentStr, ".armature/")
	if count != 1 {
		t.Errorf("expected .armature/ to appear once, but it appears %d times", count)
	}
}

func TestUpdateGitExcludeConcurrentWritersKeepEveryPattern_REQ_LNGHZN_S9_T1(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".git"), 0o750))
	const writers = 24
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		pattern := fmt.Sprintf("/concurrent-%d/", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- updateGitExclude(tmpDir, pattern, "")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".git", "info", "exclude"))
	require.NoError(t, err)
	for i := 0; i < writers; i++ {
		assert.Contains(t, string(data), fmt.Sprintf("/concurrent-%d/", i))
	}
}

func TestMigrateDualBranchToCollapsed_DirtyWorktree_REQ_LNGHZN_S1_T2(t *testing.T) {
	tmpDir := t.TempDir()

	gitClient := adapters.New(tmpDir)

	require.NoError(t, os.Setenv("GIT_AUTHOR_NAME", "Test User"))
	require.NoError(t, os.Setenv("GIT_AUTHOR_EMAIL", "test@example.com"))
	require.NoError(t, os.Setenv("GIT_COMMITTER_NAME", "Test User"))
	require.NoError(t, os.Setenv("GIT_COMMITTER_EMAIL", "test@example.com"))

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

	if err := gitClient.CreateOrphanBranch("_armature"); err != nil {
		t.Fatalf("failed to create _armature branch: %v", err)
	}

	if err := gitClient.SetGitConfig("user.email", "test@example.com"); err != nil {
		t.Fatalf("failed to set git user.email: %v", err)
	}
	if err := gitClient.SetGitConfig("user.name", "Test User"); err != nil {
		t.Fatalf("failed to set git user.name: %v", err)
	}

	armWorktreePath := filepath.Join(tmpDir, ".arm")
	if err := gitClient.AddWorktree("_armature", armWorktreePath); err != nil {
		t.Fatalf("failed to create .arm worktree: %v", err)
	}

	innerArmaturePath := filepath.Join(armWorktreePath, config.StateDirName)
	opsDir := filepath.Join(innerArmaturePath, "ops")
	if err := os.MkdirAll(opsDir, 0o750); err != nil {
		t.Fatalf("failed to create ops directory: %v", err)
	}

	testFile := filepath.Join(armWorktreePath, "tracked-file.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0o600); err != nil {
		t.Fatalf("failed to create tracked file: %v", err)
	}

	armGitClient := adapters.New(armWorktreePath)
	if err := armGitClient.AddPaths([]string{"tracked-file.txt"}); err != nil {
		t.Fatalf("failed to add tracked file: %v", err)
	}
	if err := armGitClient.CommitWorktreeOp("tracked-file.txt", "test commit"); err != nil {
		t.Fatalf("failed to commit tracked file: %v", err)
	}

	if err := os.WriteFile(testFile, []byte("modified content"), 0o600); err != nil {
		t.Fatalf("failed to modify tracked file: %v", err)
	}

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

	if _, err := os.Stat(filepath.Join(armWorktreePath, ".git")); os.IsNotExist(err) {
		t.Error("expected .arm/ worktree to still exist after failed migration")
	}

	newWorktreePath := filepath.Join(tmpDir, config.StateDirName)
	if _, err := os.Stat(filepath.Join(newWorktreePath, ".git")); err == nil {
		t.Error("expected new .armature/ worktree to not exist after failed migration")
	}
}

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

func TestMigrateDualBranchToCollapsedReconcilesSourcesOnlyDebris(t *testing.T) {
	repo, armWorktreePath := setupDualBranchFixtureForSourcesDebris(t)

	sourcesDir := filepath.Join(armWorktreePath, config.StateDirName, "sources")

	require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "src-a.cache"), []byte("cached content a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "src-b.cache"), []byte("cached content b"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), []byte(`{"sources":{"a":{}}}`), 0o600))

	migrated, backupDir, err := migrateDualBranchToCollapsed(repo)
	require.NoError(t, err, "migration must reconcile pure sources debris instead of refusing")
	require.True(t, migrated)
	require.NotEmpty(t, backupDir)

	newWorktreePath := filepath.Join(repo, config.StateDirName)
	assert.DirExists(t, newWorktreePath)
	assert.FileExists(t, filepath.Join(newWorktreePath, "sources", "src-a.cache"))
	assert.FileExists(t, filepath.Join(newWorktreePath, "sources", "src-b.cache"))
	assert.FileExists(t, filepath.Join(newWorktreePath, "ops", "existing-issue.json"))

	logOut := runOutput(t, repo, "log", "_armature", "--oneline")
	assert.Contains(t, logOut, "reconcile pre-LNGHZN-B1 uncommitted sources state",
		"the sources debris reconciliation must be committed to the _armature branch")

	status := strings.TrimSpace(runOutput(t, newWorktreePath, "status", "--porcelain"))
	assert.Empty(t, status, "collapsed worktree must be clean after reconciling sources debris")
}

func TestMigrateDualBranchToCollapsedRefusesWhenNonSourcesPathAlsoDirty(t *testing.T) {
	repo, armWorktreePath := setupDualBranchFixtureForSourcesDebris(t)

	sourcesDir := filepath.Join(armWorktreePath, config.StateDirName, "sources")
	require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "src-a.cache"), []byte("cached content a"), 0o600))

	opsFile := filepath.Join(armWorktreePath, config.StateDirName, "ops", "existing-issue.json")
	require.NoError(t, os.WriteFile(opsFile, []byte(`{"id":"existing","modified":true}`), 0o600))

	migrated, backupDir, err := migrateDualBranchToCollapsed(repo)
	require.Error(t, err, "migration must still refuse when a tracked non-sources path is dirty")
	assert.Contains(t, err.Error(), "uncommitted changes")
	assert.False(t, migrated)
	assert.Empty(t, backupDir)

	newWorktreePath := filepath.Join(repo, config.StateDirName)
	assert.False(t, pathExists(filepath.Join(newWorktreePath, ".git")), "no collapsed worktree should be created on refusal")
	status := strings.TrimSpace(runOutput(t, armWorktreePath, "status", "--porcelain"))
	assert.Contains(t, status, "src-a.cache")
	assert.Contains(t, status, "existing-issue.json")
}

func TestMigrateDualBranchToCollapsedTreatsUntrackedNonSourcesDebrisTheSameAsBefore(t *testing.T) {
	repo, armWorktreePath := setupDualBranchFixtureForSourcesDebris(t)

	require.NoError(t, os.WriteFile(filepath.Join(armWorktreePath, "SCHEMA-like-scaffolding.txt"), []byte("scaffold"), 0o600))

	migrated, backupDir, err := migrateDualBranchToCollapsed(repo)
	require.NoError(t, err, "an untracked non-sources path must not block migration")
	assert.True(t, migrated)
	assert.NotEmpty(t, backupDir)
}

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
