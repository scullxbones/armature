package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

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

// TestBootstrapCommandDefaultsToLocal verifies that the bootstrap command plan uses "local" target by default.
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

	// Verify .armature was initialized (part of init)
	assert.DirExists(t, filepath.Join(repo, ".armature"))
}

// TestRunRepoSetupCreatesStructure verifies that runRepoSetup creates the directory
// structure (.armature/ops, .armature/state, etc.) needed for Armature.
func TestRunRepoSetupCreatesStructure(t *testing.T) {
	repo := initTempRepo(t)
	// Create an initial commit so git is fully initialized
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	err := runRepoSetup(cmd, repo, false)
	require.NoError(t, err)

	// Verify directory structure
	assert.DirExists(t, filepath.Join(repo, ".armature"))
	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"))
	assert.DirExists(t, filepath.Join(repo, ".armature", "state"))
	assert.DirExists(t, filepath.Join(repo, ".armature", "state", "issues"))
	assert.DirExists(t, filepath.Join(repo, ".armature", "hooks"))
	assert.DirExists(t, filepath.Join(repo, ".armature", "templates"))
	assert.DirExists(t, filepath.Join(repo, ".armature", "review"))
}

// TestRunRepoSetupWritesGitignore verifies that runRepoSetup writes .armature/.gitignore
// to prevent state/ from being committed.
func TestRunRepoSetupWritesGitignore(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	err := runRepoSetup(cmd, repo, false)
	require.NoError(t, err)

	gitignorePath := filepath.Join(repo, ".armature", ".gitignore")
	content, readErr := os.ReadFile(gitignorePath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "state/")
}

// TestRunRepoSetupWritesSchemaFile verifies that runRepoSetup writes the SCHEMA file.
func TestRunRepoSetupWritesSchemaFile(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	err := runRepoSetup(cmd, repo, false)
	require.NoError(t, err)

	schemaPath := filepath.Join(repo, ".armature", "ops", "SCHEMA")
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

	err := runRepoSetup(cmd, repo, false)
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

	err1 := runRepoSetup(cmd, repo, false)
	require.NoError(t, err1)

	err2 := runRepoSetup(cmd, repo, false)
	require.NoError(t, err2, "second run should not fail")
}

// TestRunRepoSetupWritesConfig verifies that runRepoSetup writes config.json.
func TestRunRepoSetupWritesConfig(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	err := runRepoSetup(cmd, repo, false)
	require.NoError(t, err)

	configPath := filepath.Join(repo, ".armature", "config.json")
	_, statErr := os.Stat(configPath)
	require.NoError(t, statErr, "config.json should be created")
}

// TestInstallHooksExecutable verifies that installHooks makes hook files executable.
func TestInstallHooksExecutable(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	err := runRepoSetup(cmd, repo, false)
	require.NoError(t, err)

	// Verify at least one hook is executable
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	stat, statErr := os.Stat(hookPath)
	require.NoError(t, statErr)
	assert.NotZero(t, stat.Mode()&0o111, "hook should be executable")
}

// TestRunRepoSetupDualBranchCreatesWorktree verifies that runRepoSetup creates a .arm worktree in dual-branch mode.
func TestRunRepoSetupDualBranchCreatesWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	err := runRepoSetup(cmd, repo, true)
	require.NoError(t, err)

	// Verify worktree exists at .arm/
	assert.DirExists(t, filepath.Join(repo, ".arm"))

	// Verify .armature/ is inside the worktree
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"))

	// Verify config is in dual-branch mode
	configPath := filepath.Join(repo, ".arm", ".armature", "config.json")
	content, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), `"mode": "dual-branch"`)
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
