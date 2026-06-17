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
