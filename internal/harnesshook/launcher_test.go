package harnesshook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLauncherInstallWritesClaudeConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	launcher := NewLauncher()

	require.NoError(t, launcher.Install(dir, "claude"))

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
}

func TestLauncherInstallWritesCodexConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	launcher := NewLauncher()

	require.NoError(t, launcher.Install(dir, "codex"))

	data, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
}

func TestLauncherInstallWritesDevinConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	launcher := NewLauncher()

	require.NoError(t, launcher.Install(dir, "devin"))

	data, err := os.ReadFile(filepath.Join(dir, ".devin", "hooks.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
}

func TestLauncherBuildEnvIncludesIssueIDAndPlatform(t *testing.T) {
	t.Parallel()
	launcher := NewLauncher()

	env := launcher.BuildEnv(map[string]string{"PATH": "/usr/bin"}, "TASK-1", "codex")

	assert.Equal(t, "/usr/bin", env["PATH"])
	assert.Equal(t, "TASK-1", env["ARMATURE_ISSUE_ID"])
	assert.Equal(t, "codex", env["ARMATURE_HOOK_PLATFORM"])
}
