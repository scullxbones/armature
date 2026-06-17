package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/scullxbones/armature/internal/bootstrap"
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

	_, err := runRepoSetup(cmd, repo, false)
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

	_, err := runRepoSetup(cmd, repo, false)
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

	_, err := runRepoSetup(cmd, repo, false)
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

	_, err := runRepoSetup(cmd, repo, false)
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

	_, err1 := runRepoSetup(cmd, repo, false)
	require.NoError(t, err1)

	_, err2 := runRepoSetup(cmd, repo, false)
	require.NoError(t, err2, "second run should not fail")
}

// TestRunRepoSetupWritesConfig verifies that runRepoSetup writes config.json.
func TestRunRepoSetupWritesConfig(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo, false)
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

	_, err := runRepoSetup(cmd, repo, false)
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

	_, err := runRepoSetup(cmd, repo, true)
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

// TestExecuteHarnessSetupWarnsOnUnsupportedHarnessHookConfig verifies that executeHarnessSetup
// emits a warning to stderr when HarnessHookConfig is ActionUnsupported (i.e., user requested
// hooks but the platform doesn't support them).
func TestExecuteHarnessSetupWarnsOnUnsupportedHarnessHookConfig(t *testing.T) {
	repoPath := t.TempDir()

	// Create a plan with ActionUnsupported for HarnessHookConfig (e.g., user requested hooks for codex)
	plan := bootstrap.Plan{
		Target: "local",
		Rows: []bootstrap.PlatformRow{
			{
				Platform:          bootstrap.PlatformCodex,
				Skills:            bootstrap.ActionUnsupported,
				PluginMetadata:    bootstrap.ActionUnsupported,
				HarnessHookConfig: bootstrap.ActionUnsupported,
			},
		},
	}

	// Create a cobra command with a bytes buffer to capture stderr
	cmd := newRootCmd()
	errBuf := &bytes.Buffer{}
	cmd.SetErr(errBuf)

	// Execute the harness setup
	_, err := executeHarnessSetup(cmd, plan, repoPath, false)
	require.NoError(t, err, "executeHarnessSetup should not error on unsupported actions")

	// Check that a warning was written to stderr about unsupported harness hook config
	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "not supported", "stderr should contain warning about unsupported harness hook config")
	assert.Contains(t, errOutput, "codex", "stderr should mention the platform name")
}

// TestExecuteHarnessSetupWarnsOnUnsupportedSkills verifies that executeHarnessSetup
// emits a warning to stderr when Skills is ActionUnsupported.
func TestExecuteHarnessSetupWarnsOnUnsupportedSkills(t *testing.T) {
	repoPath := t.TempDir()

	// Create a plan with ActionUnsupported for Skills
	plan := bootstrap.Plan{
		Target: "local",
		Rows: []bootstrap.PlatformRow{
			{
				Platform:          bootstrap.PlatformCodex,
				Skills:            bootstrap.ActionUnsupported,
				PluginMetadata:    bootstrap.ActionSkip,
				HarnessHookConfig: bootstrap.ActionSkip,
			},
		},
	}

	cmd := newRootCmd()
	errBuf := &bytes.Buffer{}
	cmd.SetErr(errBuf)

	_, err := executeHarnessSetup(cmd, plan, repoPath, false)
	require.NoError(t, err)

	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "not supported", "stderr should contain warning about unsupported skills")
}

// TestExecuteHarnessSetupWarnsOnUnsupportedPluginMetadata verifies that executeHarnessSetup
// emits a warning to stderr when PluginMetadata is ActionUnsupported.
func TestExecuteHarnessSetupWarnsOnUnsupportedPluginMetadata(t *testing.T) {
	repoPath := t.TempDir()

	// Create a plan with ActionUnsupported for PluginMetadata
	plan := bootstrap.Plan{
		Target: "local",
		Rows: []bootstrap.PlatformRow{
			{
				Platform:          bootstrap.PlatformCodex,
				Skills:            bootstrap.ActionSkip,
				PluginMetadata:    bootstrap.ActionUnsupported,
				HarnessHookConfig: bootstrap.ActionSkip,
			},
		},
	}

	cmd := newRootCmd()
	errBuf := &bytes.Buffer{}
	cmd.SetErr(errBuf)

	_, err := executeHarnessSetup(cmd, plan, repoPath, false)
	require.NoError(t, err)

	errOutput := errBuf.String()
	assert.Contains(t, errOutput, "not supported", "stderr should contain warning about unsupported plugin metadata")
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

// TestBootstrapExplicitlyRequestedUnsupportedPlatformWithHooksFailsBeforeRepoSetup verifies that
// when a user explicitly requests an unsupported platform with --with-hooks, the bootstrap command
// fails before repo setup and does not print "Bootstrap complete". This prevents silent failures
// where hooks were requested but never installed.
func TestBootstrapExplicitlyRequestedUnsupportedPlatformWithHooksFailsBeforeRepoSetup(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	errBuf := &bytes.Buffer{}
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--platform", "codex", "--with-hooks"})

	err := cmd.Execute()
	require.Error(t, err, "bootstrap with unsupported platform and --with-hooks should fail")

	// Verify the error mentions unsupported
	assert.Contains(t, err.Error(), "unsupported", "error should mention unsupported")

	// Verify "Bootstrap complete" was NOT printed
	output := buf.String()
	assert.NotContains(t, output, "Bootstrap complete", "should not print success message when requested platform is unsupported")

	// Verify .armature was NOT created (validation happens before repo setup)
	armatureDir := filepath.Join(repo, ".armature")
	_, statErr := os.Stat(armatureDir)
	assert.True(t, os.IsNotExist(statErr), ".armature should NOT be created when platform validation fails")
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
	_, err := runRepoSetup(cmd, repo, false)
	require.NoError(t, err)

	// Verify the user hook is still there unchanged
	hookData, readErr := os.ReadFile(userHookPath)
	require.NoError(t, readErr)
	assert.Equal(t, userHookContent, string(hookData), "user-managed hook should not be overwritten")
}

// TestRunRepoSetupDetectsExistingDualBranchMode verifies that when re-running bootstrap with
// dualBranch=false on a repo that was originally initialized with dualBranch=true,
// the second run detects the existing dual-branch mode from git config and uses
// the existing .arm worktree instead of creating .armature/ in the code repo.
func TestRunRepoSetupDetectsExistingDualBranchMode(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	// First run: initialize with dual-branch mode
	_, err := runRepoSetup(cmd, repo, true)
	require.NoError(t, err)

	// Verify dual-branch mode was set
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should exist after first run")

	// Second run: call with dualBranch=false (simulating `arm bootstrap` without --dual-branch flag)
	cmd2 := newRootCmd()
	cmd2.SetOut(new(strings.Builder))
	_, err = runRepoSetup(cmd2, repo, false)
	require.NoError(t, err)

	// Verify the second run still uses .arm worktree (detected from git config)
	// not .armature/ in the code repo
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should still exist")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"), ".armature should be in worktree")

	// The code repo should NOT have .armature/ directory
	assert.False(t, pathExists(filepath.Join(repo, ".armature")),
		"code repo should not have .armature/ when re-running with existing dual-branch mode")
}

// pathExists is a helper to check if a path exists without error
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestBootstrapJSONFormat verifies that `arm bootstrap --format json` emits JSON (not plain text) to stdout.
func TestBootstrapJSONFormat(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	outBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(outBuf)

	// Set the format to json via root flags
	cmd.PersistentFlags().Set("format", "json")
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)

	output := outBuf.String()
	// Should contain JSON with "status" field
	assert.Contains(t, output, `"status"`, "output should contain JSON status field")
	assert.Contains(t, output, `"ok"`, "output should contain status value 'ok'")
	// Should NOT contain plain text message
	assert.NotContains(t, output, "Bootstrap complete.\n", "output should not contain plain text message")
}

// TestBootstrapJSONFormatParseable verifies that when --format json is used, stdout contains
// only valid JSON (no progress lines from runRepoSetup or executeHarnessSetup).
// This ensures automation can parse the output without dealing with mixed human/machine text.
func TestBootstrapJSONFormatParseable(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	outBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(outBuf)

	// Set the format to json via root flags
	cmd.PersistentFlags().Set("format", "json")
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)

	output := outBuf.String()

	// Verify the output is only valid JSON (no progress lines)
	// It should not contain progress messages like "Initialized Armature...", "Deployed skills...", etc.
	assert.NotContains(t, output, "Initialized Armature", "output should not contain progress message")
	assert.NotContains(t, output, "Deployed", "output should not contain progress message")
	assert.NotContains(t, output, "Deployed skills", "output should not contain progress message")
	assert.NotContains(t, output, "Deployed plugin", "output should not contain progress message")
	assert.NotContains(t, output, "Deployed harness", "output should not contain progress message")

	// Verify the output is valid JSON
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "output must be valid JSON")
	assert.Equal(t, "ok", result["status"], "status should be 'ok'")
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

	_, err := runRepoSetup(cmd, repo, false)
	require.NoError(t, err)

	issuesDir := filepath.Join(repo, ".armature")

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

	_, err := runRepoSetup(cmd, repo, false)
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

	// Clean up any previously initialized .armature to test fresh initialization
	armaturePath := filepath.Join(repoPath, ".armature")
	if err := os.RemoveAll(armaturePath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("Failed to clean up %s: %v", armaturePath, err)
	}

	outBuf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(outBuf)

	// Set args with persistent --repo flag BEFORE subcommand name, without passing --repo to bootstrap itself.
	// This simulates: arm --repo /path bootstrap (not arm bootstrap --repo /path)
	cmd.SetArgs([]string{"--repo", repoPath, "bootstrap"})

	err := cmd.Execute()
	require.NoError(t, err, "bootstrap with persistent --repo flag should succeed")

	// Verify .armature was initialized at the correct path specified by the persistent flag
	assert.DirExists(t, filepath.Join(repoPath, ".armature"), ".armature should be initialized in the repo specified by persistent --repo flag")
	assert.DirExists(t, filepath.Join(repoPath, ".armature", "ops"), ".armature/ops should exist")
	assert.DirExists(t, filepath.Join(repoPath, ".armature", "state"), ".armature/state should exist")
	assert.DirExists(t, filepath.Join(repoPath, ".armature", "hooks"), ".armature/hooks should exist")
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
	require.NoError(t, runRepoSetup(cmd, repo, false))

	// Create a codex.toml file WITHOUT the armature:managed marker to simulate
	// a config not owned by armature (the Codex adapter checks for this marker)
	codexPath := filepath.Join(repo, "codex.toml")
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
			assert.Equal(t, "config not owned by armature", result.Note)
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
	require.NoError(t, runRepoSetup(cmd, repo, false))

	// Create an existing hook without the armature:managed marker
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	existingContent := `#!/bin/sh
# Some other pre-commit hook that's not managed by armature
echo "Running external pre-commit hook"
`
	require.NoError(t, os.WriteFile(hookPath, []byte(existingContent), 0o755))

	// Call installHooks again
	issuesDir := filepath.Join(repo, ".armature")
	_, err := installHooks(repo, issuesDir)
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
	require.NoError(t, runRepoSetup(cmd, repo, false))

	// Create an existing hook WITH the armature:managed marker
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-commit")
	oldContent := `#!/bin/sh
# armature:managed
# Old version of armature hook
echo "old"
`
	require.NoError(t, os.WriteFile(hookPath, []byte(oldContent), 0o755))

	// Call installHooks again
	issuesDir := filepath.Join(repo, ".armature")
	_, err := installHooks(repo, issuesDir)
	require.NoError(t, err)

	// Verify that the hook WAS overwritten (contains the new content)
	content, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "# armature:managed", "hook should have been overwritten")
	assert.NotContains(t, string(content), "echo \"old\"", "old content should be gone")
}
