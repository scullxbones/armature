package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeTestFS builds a minimal in-memory FS that mirrors the embedded layout:
//
//	skills/
//	  demo-skill/
//	    SKILL.md
func makeTestFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"skills/demo-skill/SKILL.md": {
			Data: []byte("# demo-skill\nA demo skill.\n"),
		},
	}
}

// TestInstallSkillsDeploysFiles verifies that deploySkills copies every skill
// entry from the provided FS into the target directory.
func TestInstallSkillsDeploysFiles(t *testing.T) {
	src := makeTestFS(t)
	dest := t.TempDir()

	err := deploySkills(src, dest)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dest, "demo-skill", "SKILL.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill")
}

// TestInstallSkillsCreatesDestDir verifies that deploySkills creates the
// destination directory when it does not exist.
func TestInstallSkillsCreatesDestDir(t *testing.T) {
	src := makeTestFS(t)
	dest := filepath.Join(t.TempDir(), "nonexistent", "skills")

	err := deploySkills(src, dest)
	require.NoError(t, err)

	info, statErr := os.Stat(dest)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// TestInstallSkillsIdempotent verifies that running deploySkills twice does not
// produce an error and that files are updated on the second run.
func TestInstallSkillsIdempotent(t *testing.T) {
	src := makeTestFS(t)
	dest := t.TempDir()

	require.NoError(t, deploySkills(src, dest))
	require.NoError(t, deploySkills(src, dest))

	content, readErr := os.ReadFile(filepath.Join(dest, "demo-skill", "SKILL.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill")
}

// TestInstallSkillsCommandLocal verifies the CLI command deploys skills to the
// local .claude/skills/ directory when --global is not set.
func TestInstallSkillsCommandLocal(t *testing.T) {
	repo := initTempRepo(t)

	out, err := runTrls(t, repo, "install-skills")
	require.NoError(t, err)
	assert.Contains(t, out, ".claude/skills")
}

// TestInstallSkillsCommandGlobal verifies the CLI command deploys skills to
// ~/.claude/skills/ when --global is set.
func TestInstallSkillsCommandGlobal(t *testing.T) {
	repo := initTempRepo(t)

	// Override HOME to a temp dir so we don't pollute the real home dir.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	out, err := runTrls(t, repo, "install-skills", "--global")
	require.NoError(t, err)
	assert.Contains(t, out, ".claude/skills")
}

// TestDeployPluginCreatesPluginJSON verifies that deployPlugin creates the
// plugin directory and copies plugin.json.
func TestDeployPluginCreatesPluginJSON(t *testing.T) {
	src := makeTestFSWithPlugin(t)
	dest := t.TempDir()

	err := deployPlugin(src, dest)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dest, "plugin.json"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "armature")
}

// TestInstallSkillsDeploysPluginLocal verifies the CLI command deploys plugin.json
// to .claude/plugins/armature when --global is not set.
func TestInstallSkillsDeploysPluginLocal(t *testing.T) {
	repo := initTempRepo(t)

	out, err := runTrls(t, repo, "install-skills")
	require.NoError(t, err)
	assert.Contains(t, out, ".claude/plugins/armature")

	pluginPath := filepath.Join(repo, ".claude", "plugins", "armature", "plugin.json")
	_, statErr := os.Stat(pluginPath)
	require.NoError(t, statErr, "plugin.json should be deployed to .claude/plugins/armature/")
}

// TestInstallSkillsDeploysPluginGlobal verifies the CLI command deploys plugin.json
// to ~/.claude/plugins/armature when --global is set.
func TestInstallSkillsDeploysPluginGlobal(t *testing.T) {
	repo := initTempRepo(t)

	// Override HOME to a temp dir so we don't pollute the real home dir.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	out, err := runTrls(t, repo, "install-skills", "--global")
	require.NoError(t, err)
	assert.Contains(t, out, ".claude/plugins/armature")

	pluginPath := filepath.Join(fakeHome, ".claude", "plugins", "armature", "plugin.json")
	_, statErr := os.Stat(pluginPath)
	require.NoError(t, statErr, "plugin.json should be deployed to ~/.claude/plugins/armature/")
}

// TestDeployFlatSkillsCreatesFlatMDFiles verifies that deployFlatSkills writes a flat
// <name>.md file (SKILL.md body) alongside each skill directory so the Skill tool can
// load skills by name.
func TestDeployFlatSkillsCreatesFlatMDFiles(t *testing.T) {
	src := makeTestFS(t)
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

// TestDeployFlatSkillsIdempotent verifies that running deployFlatSkills twice does not
// produce an error and that files are overwritten.
func TestDeployFlatSkillsIdempotent(t *testing.T) {
	src := makeTestFS(t)
	dest := t.TempDir()

	require.NoError(t, deploySkills(src, dest))
	require.NoError(t, deployFlatSkills(src, dest))
	require.NoError(t, deployFlatSkills(src, dest))

	content, readErr := os.ReadFile(filepath.Join(dest, "demo-skill.md"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "demo-skill")
}

// TestInstallSkillsDeploysFlatMDLocal verifies the CLI command writes flat <name>.md
// files to .claude/skills/ so the Skill tool can load skills by name.
func TestInstallSkillsDeploysFlatMDLocal(t *testing.T) {
	repo := initTempRepo(t)

	_, err := runTrls(t, repo, "install-skills")
	require.NoError(t, err)

	// Verify flat .md files exist for all bundled skills.
	skillsDir := filepath.Join(repo, ".claude", "skills")
	entries, readErr := os.ReadDir(skillsDir)
	require.NoError(t, readErr)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		flatPath := filepath.Join(skillsDir, e.Name()+".md")
		_, statErr := os.Stat(flatPath)
		require.NoError(t, statErr, "flat md should exist for skill %s", e.Name())
	}
}

// TestDeployFlatSkillsRewritesReferencePaths verifies that deployFlatSkills rewrites
// relative reference paths in skill files so they resolve correctly from the flat file location.
func TestDeployFlatSkillsRewritesReferencePaths(t *testing.T) {
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

// makeTestFSWithPlugin builds an in-memory FS that includes plugin.json
func makeTestFSWithPlugin(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"plugin.json": {
			Data: []byte(`{"name":"armature","description":"Test plugin"}`),
		},
	}
}
