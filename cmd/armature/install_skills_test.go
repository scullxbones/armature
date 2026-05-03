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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
