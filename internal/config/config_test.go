package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigRoundTrip(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	dir := t.TempDir()

	// No marker files — unknown
	assert.Equal(t, "unknown", DetectProjectType(dir))

	// Add go.mod
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644))
	assert.Equal(t, "go", DetectProjectType(dir))
}

func TestDetectProjectTypePriority(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Both go.mod and package.json — go wins
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644))
	assert.Equal(t, "go", DetectProjectType(dir))
}

func TestOrchestratorConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := Config{
		Mode:        "single-branch",
		ProjectType: "go",
		DefaultTTL:  60,
		TokenBudget: 1600,
		Hooks:       []HookConfig{},
		Orchestrator: OrchestratorConfig{
			MaxParallel:    4,
			SandboxEnabled: true,
			Adapters: AdapterCommands{
				Build:    "go build ./...",
				Lint:     "golangci-lint run",
				Test:     "go test ./...",
				Coverage: "go test -cover ./...",
				Mutate:   "go-mutesting ./...",
			},
		},
	}

	require.NoError(t, WriteConfig(configPath, cfg))

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, 4, loaded.Orchestrator.MaxParallel)
	assert.True(t, loaded.Orchestrator.SandboxEnabled)
	assert.Equal(t, "go build ./...", loaded.Orchestrator.Adapters.Build)
	assert.Equal(t, "golangci-lint run", loaded.Orchestrator.Adapters.Lint)
	assert.Equal(t, "go test ./...", loaded.Orchestrator.Adapters.Test)
	assert.Equal(t, "go test -cover ./...", loaded.Orchestrator.Adapters.Coverage)
	assert.Equal(t, "go-mutesting ./...", loaded.Orchestrator.Adapters.Mutate)
}

func TestOrchestratorConfigAbsentKeyIsZeroValue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	// Write a config without the orchestrator key
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
	assert.Equal(t, OrchestratorConfig{}, loaded.Orchestrator)
	assert.Equal(t, 0, loaded.Orchestrator.MaxParallel)
	assert.False(t, loaded.Orchestrator.SandboxEnabled)
	assert.Equal(t, AdapterCommands{}, loaded.Orchestrator.Adapters)
}
