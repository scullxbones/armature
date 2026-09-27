package gittest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Exit(Main(m))
}

func TestIsolateGitOverridesHostileHostEnv(t *testing.T) {
	other := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(other, "KEEP"), []byte("victim"), 0o600))
	t.Setenv("GIT_DIR", other)
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, "index"))
	t.Setenv("GIT_COMMON_DIR", other)
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(other, "objects"))
	hostile := filepath.Join(t.TempDir(), "hostile-config")
	require.NoError(t, os.WriteFile(hostile, []byte("[gc]\n\tauto = 1\n"), 0o600))
	t.Setenv("GIT_CONFIG_GLOBAL", hostile)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "0")
	t.Setenv("GIT_CONFIG", hostile)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "commit.gpgsign")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_CONFIG_KEY_1", "tag.gpgsign")
	t.Setenv("GIT_CONFIG_VALUE_1", "true")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'commit.gpgsign=true'")
	t.Setenv("GIT_TEMPLATE_DIR", filepath.Join(other, "template"))

	require.NoError(t, IsolateGit())

	assert.NotEqual(t, other, os.Getenv("GIT_DIR"))
	assert.Empty(t, os.Getenv("GIT_DIR"))
	assert.Empty(t, os.Getenv("GIT_WORK_TREE"))
	assert.Empty(t, os.Getenv("GIT_INDEX_FILE"))
	assert.Empty(t, os.Getenv("GIT_COMMON_DIR"))
	assert.Empty(t, os.Getenv("GIT_OBJECT_DIRECTORY"))
	assert.Empty(t, os.Getenv("GIT_CONFIG"))
	assert.Empty(t, os.Getenv("GIT_CONFIG_COUNT"))
	assert.Empty(t, os.Getenv("GIT_CONFIG_KEY_0"))
	assert.Empty(t, os.Getenv("GIT_CONFIG_VALUE_0"))
	assert.Empty(t, os.Getenv("GIT_CONFIG_KEY_1"))
	assert.Empty(t, os.Getenv("GIT_CONFIG_VALUE_1"))
	assert.Empty(t, os.Getenv("GIT_CONFIG_PARAMETERS"))
	assert.Empty(t, os.Getenv("GIT_TEMPLATE_DIR"))
	assert.Equal(t, "1", os.Getenv("GIT_CONFIG_NOSYSTEM"))
	assert.Equal(t, "0", os.Getenv("GIT_TERMINAL_PROMPT"))
	cfg := os.Getenv("GIT_CONFIG_GLOBAL")
	require.NotEqual(t, hostile, cfg)
	data, err := os.ReadFile(cfg) //nolint:gosec // G703: cfg is GIT_CONFIG_GLOBAL IsolateGit just wrote
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, isolationMarker)
	assert.Contains(t, text, "defaultBranch = main")
	assert.Contains(t, text, "gpgsign = false")
	assert.Contains(t, text, "auto = 0")
	assert.Contains(t, text, "autoDetach = false")
	assert.Contains(t, text, "autogc = false")
	assert.True(t, strings.Contains(text, "helper ="))
	keep, err := os.ReadFile(filepath.Join(other, "KEEP"))
	require.NoError(t, err)
	assert.Equal(t, "victim", string(keep))
}

func TestIsolateGitReusesExistingIsolatedConfig(t *testing.T) { //nolint:paralleltest // IsolateGit mutates process env
	require.NoError(t, IsolateGit())
	first := os.Getenv("GIT_CONFIG_GLOBAL")
	require.NoError(t, IsolateGit())
	assert.Equal(t, first, os.Getenv("GIT_CONFIG_GLOBAL"))
}
