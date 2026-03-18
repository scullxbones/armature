package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := Config{
		Mode:        "single-branch",
		ProjectType: "go",
		DefaultTTL:  60,
		TokenBudget: 1600,
		Hooks:       []HookConfig{},
	}

	require.NoError(t, WriteConfig(configPath, cfg))

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "single-branch", loaded.Mode)
	assert.Equal(t, "go", loaded.ProjectType)
	assert.Equal(t, 60, loaded.DefaultTTL)
}

func TestDetectProjectType(t *testing.T) {
	dir := t.TempDir()

	// No marker files — unknown
	assert.Equal(t, "unknown", DetectProjectType(dir))

	// Add go.mod
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644))
	assert.Equal(t, "go", DetectProjectType(dir))
}

func TestDetectProjectTypePriority(t *testing.T) {
	dir := t.TempDir()

	// Both go.mod and package.json — go wins
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644))
	assert.Equal(t, "go", DetectProjectType(dir))
}
