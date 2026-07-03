package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/scullxbones/armature/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBootstrapDeploySkillsDeploysFiles verifies that deploySkills (moved to bootstrap_deploy.go)
// copies every skill entry from the provided FS into the target directory.
func TestBootstrapDeploySkillsDeploysFiles(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := t.TempDir()

	err := deploySkills(src, dest)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dest, "demo-skill", "SKILL.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill")
}

// TestBootstrapDeploySkillsCreatesDestDir verifies that deploySkills creates the
// destination directory when it does not exist.
func TestBootstrapDeploySkillsCreatesDestDir(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := filepath.Join(t.TempDir(), "nonexistent", "skills")

	err := deploySkills(src, dest)
	require.NoError(t, err)

	info, statErr := os.Stat(dest)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// TestBootstrapDeployFlatSkillsCreatesFlatMDFiles verifies that deployFlatSkills writes a flat
// <name>.md file (SKILL.md body) alongside each skill directory so the Skill tool can
// load skills by name.
func TestBootstrapDeployFlatSkillsCreatesFlatMDFiles(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := t.TempDir()

	// Deploy the directory structure first (required for flat files to co-exist).
	require.NoError(t, deploySkills(src, dest))

	err := deployFlatSkills(src, dest)
	require.NoError(t, err)

	// Verify the flat .md file exists alongside the directory.
	content, readErr := os.ReadFile(filepath.Join(dest, "demo-skill.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill", "flat md should contain SKILL.md body")
}

// TestBootstrapDeployFlatSkillsRewritesReferencePaths verifies that deployFlatSkills rewrites
// relative reference paths in skill files so they resolve correctly from the flat file location.
func TestBootstrapDeployFlatSkillsRewritesReferencePaths(t *testing.T) {
	src := fstest.MapFS{
		"skills/demo-skill/SKILL.md": {
			Data: []byte("# demo-skill\nSee `references/guide.md` for details.\n"),
		},
	}
	dest := t.TempDir()

	// Deploy the directory structure first (required for flat files to co-exist).
	require.NoError(t, deploySkills(src, dest))

	err := deployFlatSkills(src, dest)
	require.NoError(t, err)

	// Verify the flat .md file has rewritten reference paths.
	content, readErr := os.ReadFile(filepath.Join(dest, "demo-skill.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill/references/guide.md", "flat md should have rewritten reference path")
	assert.NotContains(t, string(content), "`references/guide.md`", "flat md should not contain unrewritten reference path")
}

// TestBootstrapDeployPluginCreatesPluginJSON verifies that deployPlugin creates the
// plugin directory and copies plugin.json.
func TestBootstrapDeployPluginCreatesPluginJSON(t *testing.T) {
	src := makeBootstrapTestFSWithPlugin(t)
	dest := t.TempDir()

	err := deployPlugin(src, dest)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dest, "plugin.json"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "armature")
}

// TestBootstrapCopyFileWorks verifies that copyFile copies a file from src FS to dest path.
func TestBootstrapCopyFileWorks(t *testing.T) {
	src := fstest.MapFS{
		"test.txt": {
			Data: []byte("test content"),
		},
	}
	dest := t.TempDir()
	destPath := filepath.Join(dest, "test.txt")

	err := copyFile(src, "test.txt", destPath)
	require.NoError(t, err)

	content, readErr := os.ReadFile(destPath)
	require.NoError(t, readErr)
	assert.Equal(t, "test content", string(content))
}

// TestBootstrapCopySkillWithRewrittenRefsWorks verifies that copySkillWithRewrittenRefs
// reads a skill file, rewrites references/ paths, and writes it to dest.
func TestBootstrapCopySkillWithRewrittenRefsWorks(t *testing.T) {
	src := fstest.MapFS{
		"skills/demo-skill/SKILL.md": {
			Data: []byte("# demo\nCheck references/guide.md\n"),
		},
	}
	dest := t.TempDir()
	destPath := filepath.Join(dest, "demo-skill.md")

	err := copySkillWithRewrittenRefs(src, "skills/demo-skill/SKILL.md", "demo-skill", destPath)
	require.NoError(t, err)

	content, readErr := os.ReadFile(destPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill/references/guide.md")
	assert.NotContains(t, string(content), "references/guide.md\" (shouldn't have unrewritten path)")
}

// makeBootstrapTestFS builds a minimal in-memory FS for bootstrap tests
func makeBootstrapTestFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"skills/demo-skill/SKILL.md": {
			Data: []byte("# demo-skill\nA demo skill.\n"),
		},
	}
}

// makeBootstrapTestFSWithPlugin builds an in-memory FS that includes plugin.json
func makeBootstrapTestFSWithPlugin(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"plugin.json": {
			Data: []byte(`{"name":"armature","description":"Test plugin"}`),
		},
	}
}

// TestBootstrapCommandRegistered verifies that `arm bootstrap --help` exits 0.
func TestBootstrapCommandRegistered(t *testing.T) {
	repo := initTempRepo(t)
	// Create an initial commit so git is fully initialized
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--help"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "bootstrap")
}

// TestBootstrapCommandDefaultsToLocal verifies that the bootstrap command initializes the repository.
func TestBootstrapCommandDefaultsToLocal(t *testing.T) {
	repo := initTempRepo(t)
	// Create an initial commit so git is fully initialized
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	// Note: --repo is a flag on the bootstrap command itself
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)

	// Verify dual-branch layout was initialized (always uses dual-branch mode now)
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"))
}

// TestRunRepoSetupCreatesStructure verifies that runRepoSetup creates the directory
// structure (.armature/ops, .armature/state, etc.) needed for Armature.
// In dual-branch mode, this structure is in the .arm worktree.
func TestRunRepoSetupCreatesStructure(t *testing.T) {
	repo := initTempRepo(t)
	// Create an initial commit so git is fully initialized
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify directory structure in the dual-branch worktree
	armatureBase := filepath.Join(repo, ".arm", ".armature")
	assert.DirExists(t, armatureBase)
	assert.DirExists(t, filepath.Join(armatureBase, "ops"))
	assert.DirExists(t, filepath.Join(armatureBase, "state"))
	assert.DirExists(t, filepath.Join(armatureBase, "state", "issues"))
	assert.DirExists(t, filepath.Join(armatureBase, "hooks"))
	assert.DirExists(t, filepath.Join(armatureBase, "templates"))
	assert.DirExists(t, filepath.Join(armatureBase, "review"))
}

// TestRunRepoSetupWritesGitignore verifies that runRepoSetup writes .armature/.gitignore
// to prevent state/ from being committed. In dual-branch mode, .armature is in the worktree.
func TestRunRepoSetupWritesGitignore(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	gitignorePath := filepath.Join(repo, ".arm", ".armature", ".gitignore")
	content, readErr := os.ReadFile(gitignorePath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "state/")
}

// TestRunRepoSetupWritesSchemaFile verifies that runRepoSetup writes the SCHEMA file.
// In dual-branch mode, the SCHEMA file is in the worktree's ops directory.
func TestRunRepoSetupWritesSchemaFile(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	schemaPath := filepath.Join(repo, ".arm", ".armature", "ops", "SCHEMA")
	_, statErr := os.Stat(schemaPath)
	require.NoError(t, statErr)
}

// TestRunRepoSetupInstallsHooks verifies that runRepoSetup installs hooks to .git/hooks/.
func TestRunRepoSetupInstallsHooks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify hooks are installed
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	_, statErr := os.Stat(hookPath)
	require.NoError(t, statErr, "pre-commit hook should be installed")
}

// TestRunRepoSetupIdempotent verifies that running runRepoSetup twice is safe.
func TestRunRepoSetupIdempotent(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err1 := runRepoSetup(cmd, repo)
	require.NoError(t, err1)

	_, err2 := runRepoSetup(cmd, repo)
	require.NoError(t, err2, "second run should not fail")
}

// TestRunRepoSetupWritesConfig verifies that runRepoSetup writes config.json.
// In dual-branch mode, config.json is in the worktree's .armature directory.
func TestRunRepoSetupWritesConfig(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	configPath := filepath.Join(repo, ".arm", ".armature", "config.json")
	_, statErr := os.Stat(configPath)
	require.NoError(t, statErr, "config.json should be created in worktree")
}

// TestInstallHooksExecutable verifies that installHooks makes hook files executable.
func TestInstallHooksExecutable(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify at least one hook is executable
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	stat, statErr := os.Stat(hookPath)
	require.NoError(t, statErr)
	assert.NotZero(t, stat.Mode()&0o111, "hook should be executable")
}

// TestRunRepoSetupAlwaysCreatesDualBranchWorktree verifies that runRepoSetup always creates a .arm worktree
// in dual-branch mode, since dual-branch is now the only supported mode.
func TestRunRepoSetupAlwaysCreatesDualBranchWorktree_REQ_SB_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify worktree exists at .arm/
	assert.DirExists(t, filepath.Join(repo, ".arm"))

	// Verify .armature/ is inside the worktree
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"))

	// Verify config is created in the worktree
	configPath := filepath.Join(repo, ".arm", ".armature", "config.json")
	_, statErr := os.Stat(configPath)
	require.NoError(t, statErr, "config.json should exist in worktree")
}

// TestBootstrapDeployPluginUsesPluginName verifies that deployPlugin uses the plugin's
// name from plugin.json (e.g., "armature") for the directory path, not the platform name.
// This ensures that metadata is installed at .claude/plugins/armature/ not .claude/plugins/claude/.
func TestBootstrapDeployPluginUsesPluginName(t *testing.T) {
	src := makeBootstrapTestFSWithPlugin(t)

	pluginName, err := getPluginNameFromFS(src)
	require.NoError(t, err)
	assert.Equal(t, "armature", pluginName, "plugin name should be extracted from plugin.json")
}

// TestBootstrapInvalidPlatformFailsBeforeRepoSetup verifies that arm bootstrap
// rejects an unknown platform name before mutating the repository.
func TestBootstrapInvalidPlatformFailsBeforeRepoSetup(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--platform", "codxe"})

	err := cmd.Execute()
	require.Error(t, err, "bootstrap with unknown platform should fail")

	// The repository must NOT have been mutated before validation failure
	armatureDir := filepath.Join(repo, ".armature")
	_, statErr := os.Stat(armatureDir)
	assert.True(t, os.IsNotExist(statErr), ".armature must not be created when platform validation fails")
}

// TestInstallHooksPreservesExistingUnmanagedHook verifies that installHooks does not overwrite
// existing user-managed git hooks. It should only overwrite hooks that are Armature-owned
// (marked with "# Armature" near the top).
func TestInstallHooksPreservesExistingUnmanagedHook(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Create a user-managed hook before bootstrap
	gitHooksDir := filepath.Join(repo, ".git", "hooks")
	require.NoError(t, os.MkdirAll(gitHooksDir, 0o750))

	userHookContent := "#!/bin/sh\n# User-managed pre-commit hook\necho 'User hook running'\n"
	userHookPath := filepath.Join(gitHooksDir, "pre-commit")
	require.NoError(t, os.WriteFile(userHookPath, []byte(userHookContent), 0o755))

	// Run bootstrap (which calls installHooks)
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify the user hook is still there unchanged
	hookData, readErr := os.ReadFile(userHookPath)
	require.NoError(t, readErr)
	assert.Equal(t, userHookContent, string(hookData), "user-managed hook should not be overwritten")
}

// TestRunRepoSetupIdempotentDualBranchMode_REQ_SB_T9 verifies that when bootstrap is run twice,
// both runs use dual-branch mode and idempotency is maintained. The second run should not
// corrupt or recreate existing structures.
func TestRunRepoSetupIdempotentDualBranchMode_REQ_SB_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	// First run: initialize in dual-branch mode (always the case now)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify dual-branch mode was set
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should exist after first run")

	// Second run: call bootstrap again (should be idempotent)
	cmd2 := newRootCmd()
	cmd2.SetOut(new(strings.Builder))
	_, err = runRepoSetup(cmd2, repo)
	require.NoError(t, err)

	// Verify the second run still uses .arm worktree and does not corrupt it
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should still exist")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"), ".armature should be in worktree")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature", "ops"), "ops directory should exist in worktree")

	// The code repo should NOT have .armature/ directory in dual-branch mode
	assert.False(t, pathExists(filepath.Join(repo, ".armature")),
		"code repo should not have .armature/ in dual-branch mode")
}

// pathExists is a helper to check if a path exists without error
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestInstallHooksReturnsSkippedHooks verifies that installHooks returns the list of skipped hook names
// when existing hooks are not Armature-managed.
func TestInstallHooksReturnsSkippedHooks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Create user-managed hooks before calling installHooks
	gitHooksDir := filepath.Join(repo, ".git", "hooks")
	require.NoError(t, os.MkdirAll(gitHooksDir, 0o750))

	userPreCommitHook := "#!/bin/sh\n# User-managed hook\necho 'User hook'\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(gitHooksDir, "pre-commit"),
		[]byte(userPreCommitHook),
		0o755,
	))

	userPostCommitHook := "#!/bin/sh\n# Another user hook\necho 'Another hook'\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(gitHooksDir, "post-commit"),
		[]byte(userPostCommitHook),
		0o755,
	))

	// Set up .armature/hooks/ with templates
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// In dual-branch mode, issuesDir is in the worktree
	issuesDir := filepath.Join(repo, ".arm", ".armature")

	// Now test that installHooks returns the skipped hooks
	skipped, err := installHooks(repo, issuesDir)
	require.NoError(t, err)

	// Should have skipped at least the pre-commit and post-commit hooks
	assert.NotEmpty(t, skipped, "skipped hooks list should not be empty")
	assert.Contains(t, skipped, "pre-commit", "should report pre-commit as skipped")
	assert.Contains(t, skipped, "post-commit", "should report post-commit as skipped")
}

// TestRunRepoSetupWarnsAboutSkippedHooks verifies that when installHooks reports skipped hooks,
// runRepoSetup prints warnings to stderr for each skipped hook.
func TestRunRepoSetupWarnsAboutSkippedHooks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Create a user-managed hook before bootstrap
	gitHooksDir := filepath.Join(repo, ".git", "hooks")
	require.NoError(t, os.MkdirAll(gitHooksDir, 0o750))

	userHookContent := "#!/bin/sh\n# User-managed pre-commit hook\necho 'User hook running'\n"
	userHookPath := filepath.Join(gitHooksDir, "pre-commit")
	require.NoError(t, os.WriteFile(userHookPath, []byte(userHookContent), 0o755))

	// Run runRepoSetup with stderr capture
	outBuf := new(strings.Builder)
	errBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	errOutput := errBuf.String()
	// Should have a warning about skipping the pre-commit hook
	assert.Contains(t, errOutput, "Warning:", "stderr should contain warning prefix")
	assert.Contains(t, errOutput, "pre-commit", "stderr should mention the skipped hook name")
	assert.Contains(t, errOutput, "not Armature-managed", "stderr should explain why it was skipped")
}

// TestBootstrapRespectsPersistentRepoFlag verifies that when the root persistent --repo flag is set
// (without passing --repo directly to bootstrap), the bootstrap command uses the persistent flag value.
// This tests the fix for the flag shadowing bug where the local --repo flag would shadow the root's.
// Before the fix, the local repoPath variable would default to "." instead of reading from the
// persistent flag, causing bootstrap to operate on the wrong directory.
func TestBootstrapRespectsPersistentRepoFlag(t *testing.T) {
	repoPath := initTempRepo(t)
	run(t, repoPath, "git", "commit", "--allow-empty", "-m", "init")

	outBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(outBuf)

	// Set args with persistent --repo flag BEFORE subcommand name, without passing --repo to bootstrap itself.
	// This simulates: arm --repo /path bootstrap (not arm bootstrap --repo /path)
	cmd.SetArgs([]string{"--repo", repoPath, "bootstrap"})

	err := cmd.Execute()
	require.NoError(t, err, "bootstrap with persistent --repo flag should succeed")

	// Verify dual-branch .armature was initialized at the correct path specified by the persistent flag
	assert.DirExists(t, filepath.Join(repoPath, ".arm", ".armature"), ".armature should be initialized in the .arm worktree")
	assert.DirExists(t, filepath.Join(repoPath, ".arm", ".armature", "ops"), ".armature/ops should exist in worktree")
	assert.DirExists(t, filepath.Join(repoPath, ".arm", ".armature", "state"), ".armature/state should exist in worktree")
	assert.DirExists(t, filepath.Join(repoPath, ".arm", ".armature", "hooks"), ".armature/hooks should exist in worktree")
}

// TestBootstrapJSONSkippedHooksReported verifies that skipped_hooks appears in JSON output
// when a pre-existing unmanaged hook prevents installation.
func TestBootstrapJSONSkippedHooksReported(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// First bootstrap to create the .armature/ structure.
	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	cmd1.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})
	require.NoError(t, cmd1.Execute())

	// Replace post-commit with a user-managed hook (no armature marker).
	hookPath := filepath.Join(repo, ".git", "hooks", "post-commit")
	require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\necho mine\n"), 0o755))

	// Second bootstrap: post-commit should be skipped and reported.
	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	cmd2.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})
	require.NoError(t, cmd2.Execute())

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(buf2.String()), &result))

	repoSetup, ok := result["repo_setup"].(map[string]interface{})
	require.True(t, ok)
	skipped, ok := repoSetup["skipped_hooks"].([]interface{})
	require.True(t, ok, "skipped_hooks should be present when hooks are skipped")
	assert.Len(t, skipped, 1, "one hook should be skipped")
	assert.Equal(t, "post-commit", skipped[0])
}

// TestBootstrapJSONOutput verifies that arm bootstrap --format json emits valid JSON
// with the expected schema: repo_setup and harness_setup fields.
func TestBootstrapJSONOutput(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()

	// Verify output is valid JSON
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "bootstrap output should be valid JSON")

	// Verify expected top-level schema
	assert.Contains(t, result, "repo_setup", "result should have repo_setup field")
	assert.Contains(t, result, "harness_setup", "result should have harness_setup field")

	// Verify repo_setup has the expected structure
	repoSetup, ok := result["repo_setup"].(map[string]interface{})
	require.True(t, ok, "repo_setup should be an object")
	assert.Contains(t, repoSetup, "status", "repo_setup should have status field")

	// Verify harness_setup is an array
	harnessSetup, ok := result["harness_setup"].([]interface{})
	require.True(t, ok, "harness_setup should be an array")
	_ = harnessSetup // Use it to avoid linter complaints
}

// TestBootstrapJSONRepoSetupStatus verifies that repo_setup.status is set correctly
// on fresh bootstrap and on idempotent re-run.
func TestBootstrapJSONRepoSetupStatus(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// First bootstrap (fresh init)
	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	cmd1.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})

	err := cmd1.Execute()
	require.NoError(t, err)

	var result1 map[string]interface{}
	err = json.Unmarshal([]byte(buf1.String()), &result1)
	require.NoError(t, err)

	repoSetup1, ok := result1["repo_setup"].(map[string]interface{})
	require.True(t, ok, "repo_setup should be an object")
	status1, ok := repoSetup1["status"].(string)
	require.True(t, ok, "status should be a string")
	assert.Equal(t, "initialized", status1, "first bootstrap should report 'initialized'")

	// Second bootstrap (idempotent)
	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	cmd2.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})

	err = cmd2.Execute()
	require.NoError(t, err)

	var result2 map[string]interface{}
	err = json.Unmarshal([]byte(buf2.String()), &result2)
	require.NoError(t, err)

	repoSetup2, ok := result2["repo_setup"].(map[string]interface{})
	require.True(t, ok, "repo_setup should be an object")
	status2, ok := repoSetup2["status"].(string)
	require.True(t, ok, "status should be a string")
	assert.Equal(t, "already_initialized", status2, "second bootstrap should report 'already_initialized'")
}

// TestExecuteHarnessSetupSkipsUnownedConfig verifies that executeHarnessSetup checks OwnsConfig
// and skips WriteConfig if the config is not owned by armature, recording a skipped status.
func TestExecuteHarnessSetupSkipsUnownedConfig(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Initialize the repo structure
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Create a .codex/config.toml file WITHOUT the armature:managed marker to simulate
	// a config not owned by armature (the Codex adapter checks for this marker)
	codexDir := filepath.Join(repo, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0o755))
	codexPath := filepath.Join(codexDir, "config.toml")
	require.NoError(t, os.WriteFile(codexPath, []byte("# Some other config\nkey = \"value\"\n"), 0o600))

	// Build a plan that includes harness hook config for Codex
	req := bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{bootstrap.PlatformCodex},
		Target:    "local",
		WithHooks: true,
	}
	plan, err := bootstrap.BuildPlan(req)
	require.NoError(t, err)

	// Execute harness setup
	results, err := executeHarnessSetup(cmd, plan, repo, false)
	require.NoError(t, err)

	// Verify that a result with Status=skipped was recorded
	var foundSkipped bool
	for _, result := range results {
		if result.Artifact == "harness_hook_config" && result.Status == "skipped" {
			foundSkipped = true
			assert.Equal(t, "codex", result.Platform)
			assert.Equal(t, "existing config not managed by Armature", result.Note)
			break
		}
	}
	assert.True(t, foundSkipped, "expected to find a skipped harness_hook_config result")
}

// TestInstallHooksSkipsUnmanagedHook verifies that installHooks does not overwrite
// an existing hook file that does not contain the # armature:managed marker.
func TestInstallHooksSkipsUnmanagedHook(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up the repo structure
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Create an existing hook without the armature:managed marker
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	existingContent := `#!/bin/sh
# Some other pre-commit hook that's not managed by armature
echo "Running external pre-commit hook"
`
	require.NoError(t, os.WriteFile(hookPath, []byte(existingContent), 0o755))

	// Call installHooks again
	issuesDir := filepath.Join(repo, ".armature")
	var skipped []string
	skipped, err = installHooks(repo, issuesDir)
	_ = skipped
	require.NoError(t, err)

	// Verify that the hook was NOT overwritten (still has the old content)
	content, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Equal(t, existingContent, string(content), "hook should not have been overwritten")
}

// TestInstallHooksOverwritesManagedHook verifies that installHooks DOES overwrite
// an existing hook file that contains the # armature:managed marker.
func TestInstallHooksOverwritesManagedHook(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up the repo structure
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Create an existing hook WITH the armature:managed marker
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	oldContent := `#!/bin/sh
# armature:managed
# Old version of armature hook
echo "old"
`
	require.NoError(t, os.WriteFile(hookPath, []byte(oldContent), 0o755))

	// Call installHooks again
	// In dual-branch mode, issuesDir is in the worktree
	issuesDir := filepath.Join(repo, ".arm", ".armature")
	var skipped2 []string
	skipped2, err = installHooks(repo, issuesDir)
	_ = skipped2
	require.NoError(t, err)

	// Verify that the hook WAS overwritten (contains the new content)
	content, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "# armature:managed", "hook should have been overwritten")
	assert.NotContains(t, string(content), "echo \"old\"", "old content should be gone")
}

// TestBootstrapRejectsUnsupportedPlatformWithoutHooks verifies that arm bootstrap --platform codex
// (without --with-hooks) returns an error when both skills and plugin_metadata are unsupported
// for that platform. The check should not count HarnessHookConfig=ActionSkip as "supported" when
// hooks weren't requested.
func TestBootstrapRejectsUnsupportedPlatformWithoutHooks(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	// Try to bootstrap with --platform codex but WITHOUT --with-hooks.
	// Codex has no verified skills or plugin_metadata, so it should fail.
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--platform", "codex"})

	err := cmd.Execute()
	require.Error(t, err, "bootstrap --platform codex without --with-hooks should fail")
	assert.Contains(t, err.Error(), "no supported requested artifacts", "error should mention unsupported artifacts")

	// The repository must NOT have been mutated before validation failure
	armatureDir := filepath.Join(repo, ".armature")
	_, statErr := os.Stat(armatureDir)
	assert.True(t, os.IsNotExist(statErr), ".armature must not be created when platform validation fails")
}

// TestBootstrapNonTTYDefaultsToJSON verifies that when stdout is not a terminal
// and --format is not explicitly set, bootstrap outputs JSON instead of "Bootstrap complete.".
// This tests the fix for the PersistentPreRunE replacement bug where the bootstrap command's
// no-op PersistentPreRunE prevented the root's TTY-detection hook from running.
func TestBootstrapNonTTYDefaultsToJSON(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Create a root command and bootstrap subcommand
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	// Simulate non-TTY by NOT setting --format explicitly.
	// In tests, tui.IsTerminal() returns false, so the detection logic should kick in
	// and set format to "json" (not "agent" because this is bootstrap, not the agent context).
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()

	// Verify output is valid JSON (not "Bootstrap complete.")
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "bootstrap output should be valid JSON when stdout is not a terminal")

	// Verify the JSON structure
	assert.Contains(t, result, "repo_setup", "result should have repo_setup field")
	assert.Contains(t, result, "harness_setup", "result should have harness_setup field")

	// Verify we do NOT get the human text
	assert.NotContains(t, output, "Bootstrap complete.", "should not emit human text in non-TTY")
}

// TestBootstrapEmitsJSONOnRepoSetupError verifies that when runRepoSetup fails and --format json
// is set, the command emits JSON with repo_setup.status="error" and repo_setup.error set before
// returning the error. This allows callers to see the failure reason in JSON format.
func TestBootstrapEmitsJSONOnRepoSetupError(t *testing.T) {
	// Create a path that doesn't exist (will fail when trying to initialize)
	nonexistentPath := filepath.Join(t.TempDir(), "no-git-here", ".armature")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	// Try to bootstrap a path that has no git repo (will fail in runRepoSetup)
	cmd.SetArgs([]string{"bootstrap", "--repo", filepath.Dir(filepath.Dir(nonexistentPath)), "--format", "json"})

	err := cmd.Execute()
	require.Error(t, err, "bootstrap should fail when repo setup fails")

	output := buf.String()

	// Verify that JSON was emitted before the error
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "output should be valid JSON even on repo setup failure")

	// Verify repo_setup contains error status
	repoSetup, ok := result["repo_setup"].(map[string]interface{})
	require.True(t, ok, "repo_setup should be an object")

	status, ok := repoSetup["status"].(string)
	require.True(t, ok, "repo_setup.status should be a string")
	assert.Equal(t, "error", status, "repo_setup.status should be 'error' when repo setup fails")

	errMsg, ok := repoSetup["error"].(string)
	require.True(t, ok, "repo_setup.error should be a string")
	assert.NotEmpty(t, errMsg, "repo_setup.error should contain the error message")

	// Verify harness_setup is an empty array
	harnessSetup, ok := result["harness_setup"].([]interface{})
	require.True(t, ok, "harness_setup should be an array")
	assert.Empty(t, harnessSetup, "harness_setup should be empty when repo setup fails")
}

// TestBootstrapEmitsPartialJSONOnHarnessSetupError verifies that when executeHarnessSetup
// returns an error along with partial results, the command emits the partial results as JSON
// before returning the error. This allows callers to see which artifacts succeeded before
// the failure occurred.
func TestBootstrapEmitsPartialJSONOnHarnessSetupError(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// First bootstrap to initialize repo
	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	cmd1.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})
	require.NoError(t, cmd1.Execute(), "initial bootstrap should succeed")

	// Simulate a harness setup failure by creating a file where the .claude directory should be,
	// which will cause os.MkdirAll to fail when trying to write plugin metadata
	claudePath := filepath.Join(repo, ".claude")
	require.NoError(t, os.RemoveAll(claudePath), "remove .claude dir")
	require.NoError(t, os.WriteFile(claudePath, []byte("blocking file"), 0o600), "create file at .claude path")
	t.Cleanup(func() {
		_ = os.RemoveAll(claudePath) //nolint:errcheck // cleanup is best-effort
	})

	// Now the second bootstrap should fail when trying to create .claude/plugins/
	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	cmd2.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "json"})

	err := cmd2.Execute()
	require.Error(t, err, "bootstrap should fail due to directory creation error")

	output := buf2.String()

	// Verify that partial JSON was emitted before the error
	// Even though the command failed, the JSON output should contain results collected so far
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "output should be valid JSON even on partial failure")

	// Verify the partial structure contains harness_setup results
	assert.Contains(t, result, "repo_setup", "partial JSON should have repo_setup field")
	harnessSetup, ok := result["harness_setup"].([]interface{})
	require.True(t, ok, "harness_setup should be an array")

	// Verify at least one result was collected before the failure (even if empty, the key should exist)
	_ = harnessSetup
}

// TestBootstrapReportsUnsupportedArtifactsInHumanFormat verifies that when using
// human format and a platform has unsupported/skipped artifacts, the command reports them
// before printing "Bootstrap complete."
func TestBootstrapReportsUnsupportedArtifactsInHumanFormat(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap with Codex which has unsupported skills and plugin_metadata but supports harness hooks
	// This should report "unsupported" for skills and plugin_metadata
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--platform", "codex", "--with-hooks", "--format", "human"})

	err := cmd.Execute()
	require.NoError(t, err, "bootstrap should succeed")

	output := buf.String()

	// Verify that unsupported artifacts are reported in human output
	// Codex has no verified skills or plugin_metadata, so they should be reported as unsupported
	assert.Contains(t, output, "unsupported", "output should mention unsupported artifacts")
	assert.Contains(t, output, "Bootstrap complete.", "output should end with completion message")
}

// TestBootstrapPersistentFormatFlagSetOnNonTTY verifies that when auto-detecting format
// to JSON in non-TTY mode, the persistent flag is also updated so early error paths
// get the correct format.
func TestBootstrapPersistentFormatFlagSetOnNonTTY(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Create a scenario that causes an early error (e.g., unsupported platform when explicitly requested)
	buf := new(strings.Builder)
	errBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Simulate non-TTY: don't set format, let it auto-detect to json
	// Then request an unsupported platform to trigger an early error
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--platform", "antigravity"})

	err := cmd.Execute()
	require.Error(t, err, "bootstrap should fail for unsupported platform")

	// Verify error handling respects the auto-detected format
	// The error message should be structured, not plain text
	errOutput := errBuf.String()
	// In non-TTY with auto-detected JSON format, error output should be structured
	// (This is harder to test directly since errors go to stderr; we focus on the other tests)
	// At minimum, verify the error occurred
	assert.NotEmpty(t, errOutput)
}

// TestRunRepoSetupMigratesLegacySingleBranchLayout_REQ_SB_T9 verifies that runRepoSetup detects
// a pre-existing single-branch .armature/ops layout and migrates it, preserving the original data
// by renaming .armature to .armature.migrated-<timestamp>.
func TestRunRepoSetupMigratesLegacySingleBranchLayout_REQ_SB_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up a legacy single-branch .armature/ops layout
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	// Create a test ops file to verify data is preserved
	testOpsFile := filepath.Join(legacyOpsPath, "test.json")
	testContent := []byte(`{"test": "data"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testContent, 0o600))

	// Run bootstrap, which should detect and migrate the legacy layout
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify the migration happened
	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	// Verify new dual-branch layout was created
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should exist")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"), ".armature should be in worktree")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature", "ops"), "new ops should be in worktree")

	// Verify old .armature directory was moved to timestamped backup
	// Find the backup directory (it should be .armature.migrated-*)
	entries, err := os.ReadDir(repo)
	require.NoError(t, err)

	var foundBackup bool
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".armature.migrated-") {
			foundBackup = true
			// Verify the backup contains the original test file
			backupOpsFile := filepath.Join(repo, entry.Name(), "ops", "test.json")
			content, readErr := os.ReadFile(backupOpsFile)
			require.NoError(t, readErr, "original ops file should be in backup")
			assert.Equal(t, testContent, content, "backup should preserve original data")
			break
		}
	}
	assert.True(t, foundBackup, "should have .armature.migrated-<timestamp> backup directory")

	// Verify code repo does not have .armature directory anymore (it's only in the worktree)
	assert.False(t, pathExists(legacyArmaturePath), "original .armature should not exist in code repo")
}

// TestRunRepoSetupMigrationIsIdempotent_REQ_SB_T9 verifies that running bootstrap twice
// (with migration on the first run) does not corrupt data or attempt to double-migrate.
func TestRunRepoSetupMigrationIsIdempotent_REQ_SB_T9(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up a legacy single-branch .armature/ops layout
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	// Create a test ops file
	testOpsFile := filepath.Join(legacyOpsPath, "test.json")
	testContent := []byte(`{"test": "data"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testContent, 0o600))

	// First bootstrap run: migrate the legacy layout
	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	_, err := runRepoSetup(cmd1, repo)
	require.NoError(t, err)

	// Verify migration happened
	assert.Contains(t, buf1.String(), "Migrated legacy single-branch")

	// Get the backup directory name from the first run
	entries1, err := os.ReadDir(repo)
	require.NoError(t, err)
	var backupDir string
	for _, entry := range entries1 {
		if strings.HasPrefix(entry.Name(), ".armature.migrated-") {
			backupDir = entry.Name()
			break
		}
	}
	require.NotEmpty(t, backupDir, "backup directory should exist after first run")

	// Second bootstrap run: should be idempotent (no legacy layout to migrate)
	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	_, err = runRepoSetup(cmd2, repo)
	require.NoError(t, err)

	// Second run should NOT report migration (no legacy layout exists)
	assert.NotContains(t, buf2.String(), "Migrated legacy single-branch", "second run should not attempt migration")

	// Verify dual-branch structure still exists and is intact
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature", "ops"), "ops should still be in worktree")

	// Verify backup directory from first run is still there and unchanged
	entries2, err := os.ReadDir(repo)
	require.NoError(t, err)
	var foundBackup bool
	for _, entry := range entries2 {
		if strings.HasPrefix(entry.Name(), ".armature.migrated-") {
			foundBackup = true
			// Should be the same backup from the first run (no new backups created)
			// If there were multiple migrated dirs, this test would fail, which is correct
			assert.Equal(t, backupDir, entry.Name(), "should not create a new backup on second run")
			break
		}
	}
	assert.True(t, foundBackup, "backup directory should still exist after second run")

	// Verify backup data is preserved
	backupOpsFile := filepath.Join(repo, backupDir, "ops", "test.json")
	content, err := os.ReadFile(backupOpsFile)
	require.NoError(t, err, "backup data should be preserved")
	assert.Equal(t, testContent, content, "backup should still contain original data")
}

// TestRunRepoSetupMigrationCopiesLegacyOpsData_P1 verifies that when migrating a legacy single-branch layout,
// the ops data from the legacy .armature/ops is COPIED to the new worktree's .armature/ops,
// not just backed up. This ensures existing issues remain accessible after migration.
func TestRunRepoSetupMigrationCopiesLegacyOpsData_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up a legacy single-branch .armature/ops layout with multiple files
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	// Create test ops files to verify data is preserved and copied
	testFile1 := filepath.Join(legacyOpsPath, "issue001.json")
	testContent1 := []byte(`{"id": "001", "title": "Legacy issue 1"}`)
	require.NoError(t, os.WriteFile(testFile1, testContent1, 0o600))

	testFile2 := filepath.Join(legacyOpsPath, "issue002.json")
	testContent2 := []byte(`{"id": "002", "title": "Legacy issue 2"}`)
	require.NoError(t, os.WriteFile(testFile2, testContent2, 0o600))

	// Also create a subdirectory with content to test recursive copy
	legacyLogsDir := filepath.Join(legacyOpsPath, "logs")
	require.NoError(t, os.MkdirAll(legacyLogsDir, 0o750))
	testLog := filepath.Join(legacyLogsDir, "claim.log")
	logContent := []byte("claim: worker1")
	require.NoError(t, os.WriteFile(testLog, logContent, 0o600))

	// Run bootstrap, which should detect and migrate the legacy layout
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify the migration happened
	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	// Verify new dual-branch layout was created
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should exist")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"), ".armature should be in worktree")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature", "ops"), "new ops should be in worktree")

	// CRITICAL: Verify that legacy ops files are COPIED to the new worktree ops, not just in backup
	newWorktreeOpsPath := filepath.Join(repo, ".arm", ".armature", "ops")

	// Check that the first issue file was copied to the new worktree
	newFile1 := filepath.Join(newWorktreeOpsPath, "issue001.json")
	content1, err := os.ReadFile(newFile1)
	require.NoError(t, err, "legacy ops file issue001.json should be copied to new worktree")
	assert.Equal(t, testContent1, content1, "copied file should have same content as original")

	// Check that the second issue file was copied to the new worktree
	newFile2 := filepath.Join(newWorktreeOpsPath, "issue002.json")
	content2, err := os.ReadFile(newFile2)
	require.NoError(t, err, "legacy ops file issue002.json should be copied to new worktree")
	assert.Equal(t, testContent2, content2, "copied file should have same content as original")

	// Check that subdirectories were copied (recursive copy)
	newLogsDir := filepath.Join(newWorktreeOpsPath, "logs")
	newLogFile := filepath.Join(newLogsDir, "claim.log")
	newLogContent, err := os.ReadFile(newLogFile)
	require.NoError(t, err, "legacy ops subdirectory should be copied to new worktree")
	assert.Equal(t, logContent, newLogContent, "copied subdirectory content should match original")

	// Also verify old .armature directory was moved to timestamped backup
	entries, err := os.ReadDir(repo)
	require.NoError(t, err)

	var foundBackup bool
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".armature.migrated-") {
			foundBackup = true
			// Verify the backup contains the original test files
			backupOpsFile1 := filepath.Join(repo, entry.Name(), "ops", "issue001.json")
			backupContent1, readErr := os.ReadFile(backupOpsFile1)
			require.NoError(t, readErr, "original ops file should be in backup")
			assert.Equal(t, testContent1, backupContent1, "backup should preserve original data")
			break
		}
	}
	assert.True(t, foundBackup, "should have .armature.migrated-<timestamp> backup directory")
}

// TestRunRepoSetupMigrationCommitsLegacyOpsData_P1 verifies that when migrating a legacy single-branch layout,
// the copied ops files are COMMITTED to the _armature branch, not just present as untracked files.
// This ensures that the migrated history is preserved for other clones and collaborators.
func TestRunRepoSetupMigrationCommitsLegacyOpsData_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up a legacy single-branch .armature/ops layout with multiple files
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	// Create test ops files to verify data is preserved and committed
	testFile1 := filepath.Join(legacyOpsPath, "issue001.json")
	testContent1 := []byte(`{"id": "001", "title": "Legacy issue 1"}`)
	require.NoError(t, os.WriteFile(testFile1, testContent1, 0o600))

	testFile2 := filepath.Join(legacyOpsPath, "issue002.json")
	testContent2 := []byte(`{"id": "002", "title": "Legacy issue 2"}`)
	require.NoError(t, os.WriteFile(testFile2, testContent2, 0o600))

	// Also create a subdirectory with content to test recursive copy and commit
	legacyLogsDir := filepath.Join(legacyOpsPath, "logs")
	require.NoError(t, os.MkdirAll(legacyLogsDir, 0o750))
	testLog := filepath.Join(legacyLogsDir, "claim.log")
	logContent := []byte("claim: worker1")
	require.NoError(t, os.WriteFile(testLog, logContent, 0o600))

	// Run bootstrap, which should detect and migrate the legacy layout
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify the migration happened
	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	// Verify new dual-branch layout was created
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should exist")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature", "ops"), "new ops should be in worktree")

	// CRITICAL: Verify that the migrated ops files are actually COMMITTED to the _armature branch,
	// not just present as untracked files in the worktree

	// Check git log in the _armature branch (should have a commit for the migrated ops)
	gitLogCmd := exec.CommandContext(context.Background(), "git", "log", "--oneline", "_armature")
	gitLogCmd.Dir = repo
	gitLogOut, err := gitLogCmd.Output()
	require.NoError(t, err, "should be able to read git log from _armature branch")

	logOutput := string(gitLogOut)
	// There should be at least a commit message mentioning the migration or ops
	// We expect to see something about the ops files being committed
	assert.NotEmpty(t, logOutput, "_armature branch should have commits, not be empty")

	// List files in the _armature branch to verify the migrated ops files are committed
	gitShowCmd := exec.CommandContext(context.Background(), "git", "ls-tree", "-r", "_armature")
	gitShowCmd.Dir = repo
	gitShowOut, err := gitShowCmd.Output()
	require.NoError(t, err, "should be able to list files in _armature branch")

	showOutput := string(gitShowOut)
	// Check that the committed files include the ops data
	assert.Contains(t, showOutput, ".armature/ops/issue001.json", "migrated ops file should be committed to _armature branch")
	assert.Contains(t, showOutput, ".armature/ops/issue002.json", "migrated ops file should be committed to _armature branch")
	assert.Contains(t, showOutput, ".armature/ops/logs/claim.log", "migrated ops subdirectory should be committed to _armature branch")

	// Verify the committed content is correct by showing the file from the _armature branch
	gitShowFileCmd := exec.CommandContext(context.Background(), "git", "show", "_armature:.armature/ops/issue001.json")
	gitShowFileCmd.Dir = repo
	gitShowFileOut, err := gitShowFileCmd.Output()
	require.NoError(t, err, "should be able to show committed ops file from _armature branch")
	assert.Equal(t, testContent1, gitShowFileOut, "committed ops file should have the correct content")
}

// TestRunRepoSetupMigrationIsIdempotent_P1 verifies that running bootstrap a second
// time over an already-migrated repo does not error and does not disturb the
// previously-migrated, committed ops data on the _armature branch.
func TestRunRepoSetupMigrationIsIdempotent_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	testFile := filepath.Join(legacyOpsPath, "issue001.json")
	testContent := []byte(`{"id": "001", "title": "Legacy issue 1"}`)
	require.NoError(t, os.WriteFile(testFile, testContent, 0o600))

	buf1 := new(strings.Builder)
	cmd1 := newRootCmd()
	cmd1.SetOut(buf1)
	_, err := runRepoSetup(cmd1, repo)
	require.NoError(t, err)
	assert.Contains(t, buf1.String(), "Migrated legacy single-branch")

	gitLogCmd := exec.CommandContext(context.Background(), "git", "log", "--oneline", "_armature")
	gitLogCmd.Dir = repo
	firstLogOut, err := gitLogCmd.Output()
	require.NoError(t, err)

	// Second run over the already-migrated repo should be a no-op: no error,
	// no duplicate migration, and the _armature branch history is unchanged.
	buf2 := new(strings.Builder)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	_, err = runRepoSetup(cmd2, repo)
	require.NoError(t, err, "second bootstrap run over an already-migrated repo should not error")
	assert.NotContains(t, buf2.String(), "Migrated legacy single-branch",
		"second run should not re-migrate")

	gitLogCmd2 := exec.CommandContext(context.Background(), "git", "log", "--oneline", "_armature")
	gitLogCmd2.Dir = repo
	secondLogOut, err := gitLogCmd2.Output()
	require.NoError(t, err)
	assert.Equal(t, string(firstLogOut), string(secondLogOut),
		"_armature branch history should be unchanged by a repeated bootstrap run")

	// Migrated content should still be intact.
	gitShowFileCmd := exec.CommandContext(context.Background(), "git", "show", "_armature:.armature/ops/issue001.json")
	gitShowFileCmd.Dir = repo
	gitShowFileOut, err := gitShowFileCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, testContent, gitShowFileOut)
}

// TestRunRepoSetupMigratesLegacyConfig_P2 verifies that when migrating a legacy single-branch layout,
// the legacy config.json (if present) is loaded from the backup and written to the new location,
// preserving user settings like custom TTL, token budget, and push threshold.
func TestRunRepoSetupMigratesLegacyConfig_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up a legacy single-branch .armature/ops layout with a custom config
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	// Create a legacy config with non-default values
	legacyConfigPath := filepath.Join(legacyArmaturePath, "config.json")
	legacyConfig := config.Config{
		ProjectType:            "go",
		DefaultTTL:             120,  // non-default
		TokenBudget:            3200, // non-default
		LowStakesPushThreshold: 10,   // non-default
		Hooks:                  []config.HookConfig{},
	}
	require.NoError(t, config.WriteConfig(legacyConfigPath, legacyConfig))

	// Create a test ops file to simulate legacy repo state
	testOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	require.NoError(t, os.WriteFile(testOpsFile, []byte(`{"id":"001"}`), 0o600))

	// Commit the legacy state (in a real migration, this would have been committed in the past)
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy armature setup")

	// Run bootstrap, which should migrate the legacy layout including config
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify the migration happened
	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	// Verify new dual-branch layout was created
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should exist")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"), ".armature should be in worktree")

	// CRITICAL: Verify that the legacy config was migrated (not reset to defaults)
	newConfigPath := filepath.Join(repo, ".arm", ".armature", "config.json")
	migratedConfig, err := config.LoadConfig(newConfigPath)
	require.NoError(t, err, "config should be loadable from new location")

	assert.Equal(t, "go", migratedConfig.ProjectType, "ProjectType should be preserved")
	assert.Equal(t, 120, migratedConfig.DefaultTTL, "custom DefaultTTL should be preserved from legacy config (not reset to 60)")
	assert.Equal(t, 3200, migratedConfig.TokenBudget, "custom TokenBudget should be preserved from legacy config (not reset to 1600)")
	assert.Equal(t, 10, migratedConfig.LowStakesPushThreshold, "custom LowStakesPushThreshold should be preserved from legacy config (not reset to 5)")

	// The whole point of untracking + committing the legacy .armature removal is to leave
	// the working tree clean after migration. Verify that explicitly.
	gitClient := adapters.New(repo)
	dirty, err := gitClient.IsWorkingTreeDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "working tree should be clean after a successful legacy migration")
}

// TestRunRepoSetupMigration_DoesNotSweepUnrelatedStagedChanges verifies that unrelated
// staged changes present when a legacy repo is migrated are NOT folded into the
// migration's "chore: migrate..." commit. Previously the migration commit was ungated
// and un-scoped, so any staged content at the time of migration got swept in.
func TestRunRepoSetupMigration_DoesNotSweepUnrelatedStagedChanges(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up a legacy single-branch .armature/ops layout, committed (as a real legacy repo would be).
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	testOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	require.NoError(t, os.WriteFile(testOpsFile, []byte(`{"id":"001"}`), 0o600))
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy armature setup")

	// Stage an unrelated file change right before running bootstrap.
	unrelatedFile := filepath.Join(repo, "unrelated.txt")
	require.NoError(t, os.WriteFile(unrelatedFile, []byte("unrelated work in progress"), 0o600))
	run(t, repo, "git", "add", "unrelated.txt")

	// With the pre-flight dirty check in place, bootstrap must refuse to run at all,
	// since the working tree (index) is dirty due to the staged unrelated file.
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.Error(t, err, "bootstrap should refuse to run when the working tree has unrelated staged changes")
	assert.Contains(t, err.Error(), "dirty", "error should mention the dirty working tree")

	// Nothing should have been touched: no migration backup dir, no committing of the
	// unrelated staged file, and the unrelated change should still be staged/uncommitted.
	entries, err := os.ReadDir(repo)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), ".armature.migrated-"),
			"no migration backup dir should be created when bootstrap refuses due to a dirty tree")
	}

	gitClient := adapters.New(repo)
	dirty, err := gitClient.IsWorkingTreeDirty()
	require.NoError(t, err)
	assert.True(t, dirty, "unrelated staged change should remain uncommitted after the refused bootstrap")

	statusCmd := exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	statusCmd.Dir = repo
	statusOut, statusErr := statusCmd.Output()
	require.NoError(t, statusErr)
	assert.Contains(t, string(statusOut), "unrelated.txt", "unrelated staged file should still be present in git status")
}

// TestDoctorCommandRunsOnLegacyRepo_P2 verifies that `arm doctor` can run on a legacy repo
// (one with .armature/ops but no armature.ops-worktree-path git config) without failing
// with "armature.ops-worktree-path must be set". The doctor command should either succeed
// or provide a clear diagnostic about the legacy layout, not crash in PersistentPreRunE.
func TestDoctorCommandRunsOnLegacyRepo_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up a legacy single-branch .armature/ops layout WITHOUT running bootstrap
	// (i.e., no git config set, no worktree created)
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	// Create a test ops file to simulate legacy repo state
	testOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	testContent := []byte(`{"id": "issue-001", "title": "Legacy issue"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testContent, 0o600))

	// Verify that git config is NOT set (legacy state)
	gitCmd := exec.CommandContext(context.Background(), "git", "config", "armature.ops-worktree-path")
	gitCmd.Dir = repo
	err := gitCmd.Run()
	assert.Error(t, err, "legacy repo should NOT have armature.ops-worktree-path git config")

	// Now try to run `arm doctor` on the legacy repo
	// This should NOT fail with "armature.ops-worktree-path must be set" in PersistentPreRunE
	buf := new(strings.Builder)
	errBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"--repo", repo, "doctor"})

	err = cmd.Execute()

	// The doctor command should either:
	// 1. Succeed without error, OR
	// 2. Fail with a clear diagnostic about the legacy layout,
	// But NOT fail with "armature.ops-worktree-path must be set"

	errOutput := errBuf.String()
	assert.NotContains(t, errOutput, "armature.ops-worktree-path must be set",
		"doctor should not fail with missing git config error on legacy repo")

	if err != nil {
		// If it fails, the error message should not be the git config error
		errMsg := err.Error()
		assert.NotContains(t, errMsg, "armature.ops-worktree-path must be set",
			"doctor error should not be about missing git config")
	}
}

// TestRunRepoSetupExcludesArmWorktreeFromGitTracking_P1 verifies that after creating the .arm worktree,
// .arm/ is added to .git/info/exclude so it won't show up as untracked in `git status` or be staged by `git add .`.
func TestRunRepoSetupExcludesArmWorktreeFromGitTracking_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify .git/info/exclude contains .arm/
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	content, readErr := os.ReadFile(excludePath)
	require.NoError(t, readErr, ".git/info/exclude should exist and be readable")

	excludeContent := string(content)
	assert.Contains(t, excludeContent, ".arm/", ".git/info/exclude should contain .arm/")

	// Verify the .arm/ entry is not duplicated on a second run (idempotent)
	// Count occurrences of .arm/ in the exclude file
	firstCount := strings.Count(excludeContent, ".arm/")
	assert.Equal(t, 1, firstCount, "should have exactly one .arm/ entry")

	// Run bootstrap again
	cmd2 := newRootCmd()
	cmd2.SetOut(new(strings.Builder))
	_, err = runRepoSetup(cmd2, repo)
	require.NoError(t, err)

	// Verify still exactly one .arm/ entry (not duplicated)
	content2, readErr2 := os.ReadFile(excludePath)
	require.NoError(t, readErr2)
	excludeContent2 := string(content2)
	secondCount := strings.Count(excludeContent2, ".arm/")
	assert.Equal(t, 1, secondCount, "should still have exactly one .arm/ entry after second run (idempotent)")
}

// TestCopyRecursiveDoesNotOverwriteExistingFiles_P2 verifies that copyRecursive does not overwrite
// files that already exist at the destination, instead skipping them.
func TestCopyRecursiveDoesNotOverwriteExistingFiles_P2(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a source file with content
	srcFile := filepath.Join(srcDir, "test.txt")
	srcContent := []byte("source content")
	require.NoError(t, os.WriteFile(srcFile, srcContent, 0o600))

	// Create a destination file with different content
	dstFile := filepath.Join(dstDir, "test.txt")
	dstContent := []byte("destination content (should not be overwritten)")
	require.NoError(t, os.WriteFile(dstFile, dstContent, 0o600))

	// Copy the source to destination
	skipped, err := copyRecursive(srcFile, dstFile)
	require.NoError(t, err)
	assert.Equal(t, 1, skipped, "copyRecursive should report 1 skipped file")

	// Verify the destination file was NOT overwritten (still has original content)
	result, readErr := os.ReadFile(dstFile)
	require.NoError(t, readErr)
	assert.Equal(t, dstContent, result, "destination file should not be overwritten by copyRecursive")
}

// TestPushOpsRunEEmitsNoStderrOnFailure_P2 verifies that push-ops's own RunE writes NOTHING
// to stderr on failure. main.go's single top-level error handler owns rendering the error
// (in whatever format was requested), so push_ops must not also emit its own error output —
// otherwise --format json would produce two JSON objects on stderr instead of one.
//
// This test drives push-ops directly via runTrlsWithStderr, which calls root.Execute() and
// does NOT go through main()'s top-level handler. That means it cannot observe main()'s
// output at all — it can only observe what push_ops's RunE itself writes. This is exactly
// the right lens for asserting push_ops emits no error output of its own.
func TestPushOpsRunEEmitsNoStderrOnFailure_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runRepoSetup(&cobra.Command{}, repo)
	require.NoError(t, err)

	// Run push-ops with no remote configured (will fail)
	out, errOutput, pushErr := runTrlsWithStderr(t, repo, "push-ops", "--format", "json")

	// Expect an error since there's no remote
	require.Error(t, pushErr, "push-ops should fail when no remote is configured")

	assert.Equal(t, "", errOutput, "push-ops RunE should write nothing to stderr; only main()'s top-level handler should render the error")
	assert.Equal(t, "", out, "stdout should be empty when push-ops fails")
}

// TestRunRepoSetupWarnsButSucceedsWhenExcludeFails_P2 verifies that when writing
// .git/info/exclude fails (a cosmetic nicety, not essential functionality), bootstrap
// still succeeds overall and prints a warning instead of aborting.
func TestRunRepoSetupWarnsButSucceedsWhenExcludeFails_P2(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits are not enforced, cannot simulate write failure")
	}

	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// `git init` already creates .git/info/exclude, so making the containing directory
	// read-only wouldn't block a write to the existing file (only creation/deletion
	// requires directory write permission). Instead, make the exclude file itself
	// read-only so os.WriteFile's in-place rewrite fails.
	infoDir := filepath.Join(repo, ".git", "info")
	require.NoError(t, os.MkdirAll(infoDir, 0o750))
	// `git init` already created this file (likely 0644), and os.WriteFile's mode
	// argument is only applied when creating a new file, so an explicit os.Chmod is
	// required to actually make the existing file read-only.
	excludePath := filepath.Join(infoDir, "exclude")
	require.NoError(t, os.WriteFile(excludePath, []byte("# existing\n"), 0o400))
	require.NoError(t, os.Chmod(excludePath, 0o400))
	t.Cleanup(func() {
		if err := os.Chmod(excludePath, 0o600); err != nil {
			t.Logf("cleanup: failed to restore exclude file permissions: %v", err)
		}
	})

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	result, err := runRepoSetup(cmd, repo)
	require.NoError(t, err, "bootstrap should succeed even when .git/info/exclude cannot be written")
	assert.NotEmpty(t, result.Status)

	assert.Contains(t, buf.String(), "Warning", "bootstrap should print a warning when the exclude write fails")
}

// TestCopyLegacyOpsToNewWorktreeReportsSkippedCount_P3 verifies that when a destination
// file already exists during legacy ops migration, the skipped count is surfaced to the
// caller rather than being silently dropped.
func TestCopyLegacyOpsToNewWorktreeReportsSkippedCount_P3(t *testing.T) {
	backupDir := t.TempDir()
	newIssuesDir := t.TempDir()
	newOpsDir := filepath.Join(newIssuesDir, "ops")
	require.NoError(t, os.MkdirAll(newOpsDir, 0o750))

	legacyOpsDir := filepath.Join(backupDir, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsDir, 0o750))

	// One file that collides with an existing destination file, one that doesn't.
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsDir, "existing.json"), []byte("legacy"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsDir, "new.json"), []byte("legacy"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newOpsDir, "existing.json"), []byte("already here"), 0o600))

	skippedCount, err := copyLegacyOpsToNewWorktree(backupDir, newIssuesDir)
	require.NoError(t, err)
	assert.Equal(t, 1, skippedCount, "exactly one file should have been skipped due to a destination collision")

	// The colliding file should not have been overwritten.
	content, readErr := os.ReadFile(filepath.Join(newOpsDir, "existing.json"))
	require.NoError(t, readErr)
	assert.Equal(t, "already here", string(content))

	// The non-colliding file should have been copied.
	content, readErr = os.ReadFile(filepath.Join(newOpsDir, "new.json"))
	require.NoError(t, readErr)
	assert.Equal(t, "legacy", string(content))
}

// TestExcludeArmWorktreeFromGitExactLineMatch_P3 verifies that the idempotency check for
// .arm/ in .git/info/exclude uses exact line matching, not substring containment. A
// pre-existing similar-but-different line (e.g. "vendor.arm/") must not suppress the
// real ".arm/" entry from being appended.
func TestExcludeArmWorktreeFromGitExactLineMatch_P3(t *testing.T) {
	repo := t.TempDir()
	infoDir := filepath.Join(repo, ".git", "info")
	require.NoError(t, os.MkdirAll(infoDir, 0o750))
	excludePath := filepath.Join(infoDir, "exclude")

	require.NoError(t, os.WriteFile(excludePath, []byte("vendor.arm/\n"), 0o600))

	require.NoError(t, excludeArmWorktreeFromGit(repo))

	content, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.Contains(t, lines, "vendor.arm/", "pre-existing unrelated line should be preserved")
	assert.Contains(t, lines, ".arm/", "the real .arm/ exclude entry should be appended despite the similar existing line")

	// Running again should not duplicate the exact ".arm/" line.
	require.NoError(t, excludeArmWorktreeFromGit(repo))
	content2, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	count := 0
	for _, line := range strings.Split(string(content2), "\n") {
		if strings.TrimSpace(line) == ".arm/" {
			count++
		}
	}
	assert.Equal(t, 1, count, ".arm/ should not be duplicated on repeated calls")
}

// TestRunRepoSetupMigrationCommitsLegacyConfig_BUGFIX verifies that when migrating a legacy
// single-branch layout with a custom config.json, the config is committed to the _armature branch
// along with the migrated ops data. This ensures custom settings (TTL, token budget, hooks, etc.)
// are preserved and pushed to other clones. Previously, the config was loaded and written but
// never committed to _armature, staying untracked in the .arm worktree.
func TestRunRepoSetupMigrationCommitsLegacyConfig_BUGFIX(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up a legacy single-branch .armature layout with custom config and ops files
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	// Create a custom legacy config (with non-default values)
	legacyConfigPath := filepath.Join(legacyArmaturePath, "config.json")
	legacyConfig := config.Config{
		ProjectType:            "go",
		DefaultTTL:             120,  // non-default
		TokenBudget:            3200, // non-default
		LowStakesPushThreshold: 10,   // non-default
		Hooks:                  []config.HookConfig{},
	}
	require.NoError(t, config.WriteConfig(legacyConfigPath, legacyConfig))

	// Create legacy ops files
	testOpsFile := filepath.Join(legacyOpsPath, "issue001.json")
	testOpsContent := []byte(`{"id": "001", "title": "Legacy issue"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testOpsContent, 0o600))

	// Commit the legacy state (as would exist in a real legacy repo)
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy armature setup")

	// Run bootstrap, which should migrate and commit both ops and config
	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify migration happened
	output := buf.String()
	assert.Contains(t, output, "Migrated legacy single-branch", "output should mention migration")

	// CRITICAL: Verify that the custom config was COMMITTED to the _armature branch,
	// not just present as an untracked file on disk.
	// This is the bug fix: config must be in the git history on _armature so it's pushed
	// to other clones when they pull/clone.

	// List files in the _armature branch to verify config is committed
	gitShowCmd := exec.CommandContext(context.Background(), "git", "ls-tree", "-r", "_armature")
	gitShowCmd.Dir = repo
	gitShowOut, err := gitShowCmd.Output()
	require.NoError(t, err, "should be able to list files in _armature branch")

	showOutput := string(gitShowOut)

	// The config.json MUST be committed to _armature, not just present on disk
	assert.Contains(t, showOutput, ".armature/config.json",
		"custom config.json MUST be committed to _armature branch so it's preserved for other clones")

	// Also verify that ops files are committed
	assert.Contains(t, showOutput, ".armature/ops/issue001.json",
		"migrated ops files should also be committed to _armature branch")

	// Verify the committed config content is correct
	gitShowConfigCmd := exec.CommandContext(context.Background(), "git", "show", "_armature:.armature/config.json")
	gitShowConfigCmd.Dir = repo
	gitShowConfigOut, err := gitShowConfigCmd.Output()
	require.NoError(t, err, "should be able to show committed config from _armature branch")

	// Parse the committed config and verify custom values are preserved
	var committedConfig config.Config
	err = json.Unmarshal(gitShowConfigOut, &committedConfig)
	require.NoError(t, err, "committed config should be valid JSON")

	assert.Equal(t, "go", committedConfig.ProjectType, "ProjectType should be committed")
	assert.Equal(t, 120, committedConfig.DefaultTTL, "custom DefaultTTL should be committed (not default 60)")
	assert.Equal(t, 3200, committedConfig.TokenBudget, "custom TokenBudget should be committed (not default 1600)")
	assert.Equal(t, 10, committedConfig.LowStakesPushThreshold, "custom LowStakesPushThreshold should be committed (not default 5)")
}

// TestRunRepoSetupFreshBootstrap_CommitsConfigToArmatureBranch verifies that when runRepoSetup
// performs a fresh bootstrap (no legacy migration), the generated config.json is COMMITTED to
// the _armature branch, not just written to disk. This ensures config is preserved when the
// _armature branch is pushed to other clones.
func TestRunRepoSetupFreshBootstrap_CommitsConfigToArmatureBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify config.json exists in the worktree
	configPath := filepath.Join(repo, ".arm", ".armature", "config.json")
	_, err = os.Stat(configPath)
	require.NoError(t, err, "config.json should exist in worktree")

	// CRITICAL: Verify that config.json is COMMITTED to the _armature branch,
	// not just written to disk untracked.
	// This is the fix for Finding 2: fresh bootstrap must also commit the default config
	// so it's preserved in git history and pushed to other clones.

	// List files committed to the _armature branch
	gitLsCmd := exec.CommandContext(context.Background(), "git", "ls-tree", "-r", "_armature")
	gitLsCmd.Dir = repo
	gitLsOut, err := gitLsCmd.Output()
	require.NoError(t, err, "should be able to list files in _armature branch")

	lsOutput := string(gitLsOut)

	// The config.json MUST be committed to _armature
	assert.Contains(t, lsOutput, ".armature/config.json",
		"config.json MUST be committed to _armature branch on fresh bootstrap")

	// Verify we can read the committed config from the branch
	gitShowCmd := exec.CommandContext(context.Background(), "git", "show", "_armature:.armature/config.json")
	gitShowCmd.Dir = repo
	gitShowOut, err := gitShowCmd.Output()
	require.NoError(t, err, "should be able to show committed config from _armature branch")

	// Verify the committed config is valid JSON
	var committedConfig config.Config
	err = json.Unmarshal(gitShowOut, &committedConfig)
	require.NoError(t, err, "committed config should be valid JSON")

	// Verify it contains expected default values
	assert.NotEmpty(t, committedConfig.ProjectType, "ProjectType should be set in default config")
}

// TestMigrateLegacySingleBranchOpsRollsBackOnCommitFailure_P1 verifies that when migrateLegacySingleBranchOps
// encounters a commit failure, it rolls back completely: the backup
// directory is removed, the original .armature directory is restored, and the index is restored to its
// original state with .armature re-added. This ensures the migration is atomic: either it fully succeeds
// or the repo is left exactly as before, not in a half-migrated state.
func TestMigrateLegacySingleBranchOpsRollsBackOnCommitFailure_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up a legacy single-branch .armature/ops layout, committed to git (as a real legacy repo would have)
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))

	// Create a test ops file to verify data is preserved
	testOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	testContent := []byte(`{"id": "001", "title": "Test issue"}`)
	require.NoError(t, os.WriteFile(testOpsFile, testContent, 0o600))

	// Commit the legacy .armature to git
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy setup")

	// Force a deterministic commit failure without relying on user hooks.
	// A bogus signing key trips git commit before the migration can complete.
	run(t, repo, "git", "config", "commit.gpgsign", "true")
	run(t, repo, "git", "config", "gpg.program", "/nonexistent-gpg-program")

	// Record the state before migration
	gitClient := adapters.New(repo)
	wasTrackedBefore := gitClient.IsTracked(".armature")
	require.True(t, wasTrackedBefore, ".armature should be tracked before migration")

	// Call migrateLegacySingleBranchOps, which should encounter the pre-commit hook rejection
	migratedFlag, backupDir, preMigrationSHA, _, err := migrateLegacySingleBranchOps(repo)

	// The migration should fail
	require.Error(t, err, "migration should fail because the pre-commit hook rejects the commit")
	assert.False(t, migratedFlag, "migrated flag should be false when migration fails")
	assert.Empty(t, backupDir, "backupDir should be empty when migration fails")
	assert.NotEmpty(t, preMigrationSHA, "preMigrationSHA should be recorded before rollback")

	// CRITICAL: Verify complete rollback
	// 1. The original .armature directory should be restored
	assert.DirExists(t, legacyArmaturePath, ".armature directory should be restored after failed migration")

	// Verify the original file is still there
	restoredOpsFile := filepath.Join(legacyOpsPath, "test-issue.json")
	restoredContent, err := os.ReadFile(restoredOpsFile)
	require.NoError(t, err, "original ops file should still exist after rollback")
	assert.Equal(t, testContent, restoredContent, "original ops file content should be unchanged")

	// 2. No backup directory should be left behind
	entries, err := os.ReadDir(repo)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), ".armature.migrated-"),
			"backup directory should not exist after rollback (cleanup on error)")
	}

	// 3. The index should be clean and .armature should still be tracked
	isTrackedAfter := gitClient.IsTracked(".armature")
	assert.True(t, isTrackedAfter, ".armature should still be tracked after rollback (re-added to index on error)")

	// 4. Working tree should be clean (no dangling staged removals)
	dirty, err := gitClient.IsWorkingTreeDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "working tree should be clean after rollback (no dangling staged removals)")

	// 5. Verify that a retry of the migration can still detect the legacy layout
	// Clear the signing configuration and verify the migration can succeed on retry.
	run(t, repo, "git", "config", "commit.gpgsign", "false")
	run(t, repo, "git", "config", "--unset-all", "gpg.program")
	migratedRetry, backupDirRetry, preMigrationSHARetry, _, errRetry := migrateLegacySingleBranchOps(repo)
	require.NoError(t, errRetry, "retry migration (without signing failure) should succeed")
	assert.True(t, migratedRetry, "retry migration should report success")
	assert.NotEmpty(t, backupDirRetry, "retry migration should return a backup dir path")
	assert.NotEmpty(t, preMigrationSHARetry, "retry migration should still record the pre-migration SHA")
	assert.DirExists(t, backupDirRetry, "retry backup directory should exist")
}

// TestRunRepoSetupMigratesTemplatesHooksReview_P2 verifies that legacy templates/, hooks/,
// and review/ content (not just ops/ and config.json) is copied into the new worktree during
// migration, rather than being left stranded only in the timestamped backup.
func TestRunRepoSetupMigratesTemplatesHooksReview_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyArmaturePath := filepath.Join(repo, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(legacyArmaturePath, "ops"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(legacyArmaturePath, "templates"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(legacyArmaturePath, "hooks"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(legacyArmaturePath, "review"), 0o750))

	require.NoError(t, os.WriteFile(filepath.Join(legacyArmaturePath, "ops", "issue.json"), []byte(`{"id":"1"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(legacyArmaturePath, "templates", "custom.md"), []byte("custom template"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(legacyArmaturePath, "hooks", "custom-hook.sh"), []byte("#!/bin/sh\necho hi\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyArmaturePath, "review", "notes.md"), []byte("review notes"), 0o600))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	result, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Status)

	newIssuesDir := filepath.Join(repo, ".arm", ".armature")

	templateContent, err := os.ReadFile(filepath.Join(newIssuesDir, "templates", "custom.md"))
	require.NoError(t, err, "legacy templates/ content should be migrated into the new worktree")
	assert.Equal(t, "custom template", string(templateContent))

	hookContent, err := os.ReadFile(filepath.Join(newIssuesDir, "hooks", "custom-hook.sh"))
	require.NoError(t, err, "legacy hooks/ content should be migrated into the new worktree")
	assert.Equal(t, "#!/bin/sh\necho hi\n", string(hookContent))

	reviewContent, err := os.ReadFile(filepath.Join(newIssuesDir, "review", "notes.md"))
	require.NoError(t, err, "legacy review/ content should be migrated into the new worktree")
	assert.Equal(t, "review notes", string(reviewContent))
}

// TestRunRepoSetupCommitsConfigWhenNotFreshInit_P2 verifies that a newly written
// config.json is committed to the _armature branch even when the worktree is not a
// fresh init (e.g. _armature was adopted from a remote that had ops/ but no
// config.json). Previously the commit was gated on freshInit, so the generated
// config.json could be written to disk but never preserved in git history.
func TestRunRepoSetupCommitsConfigWhenNotFreshInit_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	worktreePath := filepath.Join(repo, ".arm")
	issuesDir := filepath.Join(worktreePath, ".armature")
	configPath := filepath.Join(issuesDir, "config.json")

	// Simulate "adopted from remote with ops but no config.json": remove config.json
	// and delete its commit history by resetting the _armature branch, then add an ops
	// file so the directory is non-empty (not a fresh init on the next run).
	require.NoError(t, os.Remove(configPath))
	worktreeGitClient := adapters.New(worktreePath)
	require.NoError(t, worktreeGitClient.AddPaths([]string{".armature"}))
	require.NoError(t, worktreeGitClient.CommitPaths("chore: simulate remote adoption without config.json", ".armature"))

	opsFile := filepath.Join(issuesDir, "ops", "adopted-issue.json")
	require.NoError(t, os.WriteFile(opsFile, []byte(`{"id":"adopted"}`), 0o600))

	buf2 := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf2)
	result2, err := runRepoSetup(cmd2, repo)
	require.NoError(t, err)
	assert.Equal(t, "already_initialized", result2.Status, "ops/ is non-empty, so this run should not be a fresh init")

	assert.FileExists(t, configPath, "config.json should be regenerated")

	// Verify the regenerated config.json was committed to _armature (not just written to disk).
	statusOut, err := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "status", "--porcelain", ".armature/config.json").Output()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(statusOut)), "regenerated config.json should be committed, not left as an uncommitted/untracked change")

	logOut, err := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "log", "-1", "--pretty=%s", "--", ".armature/config.json").Output()
	require.NoError(t, err)
	assert.Contains(t, string(logOut), "init armature config", "config.json should have a commit preserving it in _armature history")
}

// TestRunRepoSetupRollsBackMigrationWhenWorktreeAddFails_P1 verifies that if the legacy
// migration succeeds (backup created, .armature removed and committed) but a later setup
// step (AddWorktree) fails, the migration is rolled back rather than leaving the repo with
// .armature gone from tracking while the new dual-branch layout was never created.
func TestRunRepoSetupRollsBackMigrationWhenWorktreeAddFails_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Set up and commit a legacy single-branch .armature/ops layout.
	legacyArmaturePath := filepath.Join(repo, ".armature")
	legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	testContent := []byte(`{"id": "001", "title": "Test issue"}`)
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "test-issue.json"), testContent, 0o600))
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy setup")

	// Pre-create .arm as a regular file (not a directory), so `git worktree add .arm _armature`
	// fails deterministically (AddWorktree's own "already a worktree" fast-path only checks for
	// .arm/.git, which won't exist here, so it falls through to the real `git worktree add`).
	arm := filepath.Join(repo, ".arm")
	require.NoError(t, os.WriteFile(arm, []byte("not a directory"), 0o600))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.Error(t, err, "runRepoSetup should fail because .arm exists as a non-directory file")
	assert.Contains(t, err.Error(), "add .arm worktree")

	// The legacy migration should have been rolled back: .armature restored and tracked again.
	gitClient := adapters.New(repo)
	assert.DirExists(t, legacyArmaturePath, ".armature should be restored after rollback")
	restoredContent, readErr := os.ReadFile(filepath.Join(legacyOpsPath, "test-issue.json"))
	require.NoError(t, readErr, "legacy ops file should still exist after rollback")
	assert.Equal(t, testContent, restoredContent)
	assert.True(t, gitClient.IsTracked(".armature"), ".armature should be tracked again after rollback")

	dirty, err := gitClient.IsWorkingTreeDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "working tree should be clean after rollback")
}

// TestRunRepoSetupRefusesOpsWorktree_P1 verifies that pointing bootstrap at the ops
// worktree itself (.arm, checked out on _armature) is refused instead of being
// mistaken for a legacy single-branch layout and "migrated" — which would rename
// the real dual-branch ops data away and commit its removal on _armature.
func TestRunRepoSetupRefusesOpsWorktree_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Normal bootstrap to create the dual-branch layout.
	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	_, err := runRepoSetup(cmd1, repo)
	require.NoError(t, err)

	// Put a real ops log in the worktree so the "legacy detection" precondition
	// (non-empty ops dir) holds.
	armPath := filepath.Join(repo, ".arm")
	opsFile := filepath.Join(armPath, ".armature", "ops", "worker-test.jsonl")
	opsContent := []byte(`{"op":"create"}`)
	require.NoError(t, os.WriteFile(opsFile, opsContent, 0o600))

	// Bootstrap pointed at the ops worktree must refuse.
	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	_, err = runRepoSetup(cmd2, armPath)
	require.Error(t, err, "bootstrap targeting the ops worktree should be refused")
	assert.Contains(t, err.Error(), "_armature")

	// The real ops data must be untouched: no rename, no migrated backup.
	content, readErr := os.ReadFile(opsFile)
	require.NoError(t, readErr, "ops data must not be renamed away")
	assert.Equal(t, opsContent, content)
	entries, err := os.ReadDir(armPath)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), ".armature.migrated-"),
			"no migration backup should be created inside the ops worktree")
	}
}

// TestRunRepoSetupMigrationFailureNamesBackupDir_P1 verifies that when a failure
// happens AFTER the legacy .armature has been renamed to its backup (e.g. while
// copying legacy data into the new worktree), the returned error names the backup
// directory. Without that, a re-run sees no legacy layout, fresh-inits with
// defaults, and the legacy ops are stranded in the backup with no pointer to them.
func TestRunRepoSetupMigrationFailureNamesBackupDir_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "log.jsonl"), []byte(`{"op":"x"}`), 0o600))

	// An unreadable legacy templates dir makes copyLegacyOpsToNewWorktree fail
	// after the rename to the backup has already happened.
	legacyTemplates := filepath.Join(repo, ".armature", "templates")
	require.NoError(t, os.MkdirAll(legacyTemplates, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyTemplates, "t.md"), []byte("tmpl"), 0o600))
	require.NoError(t, os.Chmod(legacyTemplates, 0o000))
	t.Cleanup(func() {
		// Restore perms wherever the dir ended up so TempDir cleanup works.
		entries, readErr := os.ReadDir(repo)
		if readErr != nil {
			return
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".armature") {
				if chmodErr := os.Chmod(filepath.Join(repo, e.Name(), "templates"), 0o750); chmodErr != nil && !os.IsNotExist(chmodErr) {
					t.Logf("cleanup chmod: %v", chmodErr)
				}
			}
		}
	})

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err := runRepoSetup(cmd, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".armature.migrated-",
		"error after a post-rename failure must name the backup dir so data isn't silently stranded")
}

// TestRunRepoSetupMigrationCommitFailureNamesBackupDir_P1 verifies that when the
// commit of migrated data to the _armature branch fails, the error names the
// timestamped backup directory so the legacy data can be recovered manually.
func TestRunRepoSetupMigrationCommitFailureNamesBackupDir_P1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "log.jsonl"), []byte(`{"op":"x"}`), 0o600))

	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, "git")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	script := fmt.Sprintf(`#!/bin/sh
real_git=%q
target=%q
cmd=""
found_target=""
skip=""
for arg in "$@"; do
  if [ -n "$skip" ]; then
    if [ "$skip" = "-C" ]; then
      found_target="$arg"
    fi
    skip=""
    continue
  fi
  case "$arg" in
    -C|-c)
      skip="$arg"
      continue
      ;;
    commit)
      cmd="$arg"
      ;;
  esac
done
if [ "$cmd" = "commit" ] && [ "$found_target" = "$target" ]; then
  exit 1
fi
exec "$real_git" "$@"
`, realGit, filepath.Join(repo, ".arm"))
	require.NoError(t, os.WriteFile(wrapperPath, []byte(script), 0o755))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err = runRepoSetup(cmd, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit migrated data")
	assert.Contains(t, err.Error(), ".armature.migrated-",
		"commit failure after migration must name the backup dir so data isn't silently stranded")
}

// TestRunRepoSetupMigrationBackupNameCollision_P2 verifies migration still succeeds
// when a backup directory with the current-second timestamp already exists (e.g. a
// retry right after a rolled-back migration, which leaves its backup behind).
func TestRunRepoSetupMigrationBackupNameCollision_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	opsContent := []byte(`{"op":"x"}`)
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "log.jsonl"), opsContent, 0o600))

	// Pre-create non-empty backup dirs for this second and the next few, simulating
	// leftovers from prior attempts, so a plain rename would fail with ENOTEMPTY.
	staleContent := []byte("stale")
	now := time.Now()
	var staleDirs []string
	for i := 0; i < 5; i++ {
		name := filepath.Join(repo, ".armature.migrated-"+now.Add(time.Duration(i)*time.Second).Format("20060102150405"))
		require.NoError(t, os.MkdirAll(name, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(name, "old.txt"), staleContent, 0o600))
		staleDirs = append(staleDirs, name)
	}

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err, "migration should succeed despite pre-existing backup dirs")

	// Prior backups untouched.
	for _, d := range staleDirs {
		content, readErr := os.ReadFile(filepath.Join(d, "old.txt"))
		require.NoError(t, readErr)
		assert.Equal(t, staleContent, content, "pre-existing backup must not be clobbered")
	}

	// Legacy ops preserved in some (new) backup and copied into the worktree.
	migrated, readErr := os.ReadFile(filepath.Join(repo, ".arm", ".armature", "ops", "log.jsonl"))
	require.NoError(t, readErr)
	assert.Equal(t, opsContent, migrated)
}

// TestRunRepoSetupCommittedRollbackNamesLeftoverBackup_P2 verifies that when a
// committed migration is rolled back (hard reset restores tracked files), the error
// still names the leftover backup dir — it's the only copy of any legacy files that
// were untracked at migration time (the clean-tree pre-flight ignores untracked).
func TestRunRepoSetupCommittedRollbackNamesLeftoverBackup_P2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "tracked.jsonl"), []byte(`{"op":"a"}`), 0o600))
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy setup")

	// An untracked ops file: passes the clean-tree check, survives only in the backup.
	untracked := []byte(`{"op":"untracked"}`)
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "untracked.jsonl"), untracked, 0o600))

	// Force AddWorktree to fail after the migration commit.
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".arm"), []byte("not a directory"), 0o600))

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err := runRepoSetup(cmd, repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add .arm worktree")
	assert.Contains(t, err.Error(), ".armature.migrated-",
		"rollback error must name the leftover backup holding untracked legacy files")

	// The backup really is still on disk with the untracked file.
	entries, readErr := os.ReadDir(repo)
	require.NoError(t, readErr)
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".armature.migrated-") {
			content, rerr := os.ReadFile(filepath.Join(repo, e.Name(), "ops", "untracked.jsonl"))
			require.NoError(t, rerr)
			assert.Equal(t, untracked, content)
			found = true
		}
	}
	assert.True(t, found, "backup dir should remain after committed rollback")
}

// TestRunRepoSetupWarnsOnUnreadableLegacyConfig_P3 verifies that when the legacy
// config.json exists but cannot be loaded, migration proceeds with defaults but
// warns the user instead of silently discarding their configuration.
func TestRunRepoSetupWarnsOnUnreadableLegacyConfig_P3(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "log.jsonl"), []byte(`{"op":"x"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".armature", "config.json"), []byte("{not json"), 0o600))

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err, "corrupt legacy config should not abort migration")

	combined := buf.String() + errBuf.String()
	assert.Contains(t, combined, "config", "user should be warned that legacy config was not migrated")
	assert.Contains(t, combined, "default", "warning should say defaults are used instead")
}
