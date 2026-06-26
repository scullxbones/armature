package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, exec.CommandContext(context.Background(), "git", "-C", dir, "init").Run())
	return dir
}

func TestNewGitConfigPort_GetAndSet(t *testing.T) {
	t.Parallel()
	repo := initTestGitRepo(t)

	port := NewGitConfigPort(repo)
	require.NoError(t, port.Set("armature.test-key", "hello"))

	val, err := port.Get("armature.test-key")
	require.NoError(t, err)
	assert.Equal(t, "hello", val)
}

func TestNewGitConfigPort_GetMissingKey_ReturnsError(t *testing.T) {
	t.Parallel()
	repo := initTestGitRepo(t)

	port := NewGitConfigPort(repo)
	_, err := port.Get("armature.nonexistent-key")
	assert.Error(t, err)
}

func TestNewGitConfigPort_WritesConfigFile(t *testing.T) {
	t.Parallel()
	repo := initTestGitRepo(t)

	port := NewGitConfigPort(repo)
	require.NoError(t, port.Set("armature.marker", "true"))

	data, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "marker")
}
