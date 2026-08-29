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
		ProjectType: "go",
		DefaultTTL:  60,
		TokenBudget: 1600,
		Hooks:       []HookConfig{},
	}

	require.NoError(t, WriteConfig(configPath, cfg))

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "go", loaded.ProjectType)
	assert.Equal(t, 60, loaded.DefaultTTL)
}

func TestLoadConfigRejectsUnknownField(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"project_type":"go","token_budegt":1600}`), 0o600))

	_, err := LoadConfig(configPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token_budegt")
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

func TestConfigGatesRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := Config{
		ProjectType: "go",
		Gates: map[string]GateConfig{
			PublishGateProfile: {Command: []string{"make", "check"}},
			"fast":             {Command: []string{"make", "check-fast"}},
		},
	}
	require.NoError(t, WriteConfig(configPath, cfg))

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	full, ok := loaded.Gate(PublishGateProfile)
	require.True(t, ok)
	assert.Equal(t, []string{"make", "check"}, full.Command)
	_, ok = loaded.Gate("missing")
	assert.False(t, ok)
}

func TestLoadGates_MissingFileIsEmpty_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	gates, err := LoadGates(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, gates)
}

func TestLoadGates_EmptyMap_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, GatesFileName), []byte("{}"), 0o644))
	gates, err := LoadGates(dir)
	require.NoError(t, err)
	assert.Empty(t, gates)
}

func TestLoadGates_ReadsTrackedFile_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, GatesFileName),
		[]byte(`{"full":{"command":["make","check"]}}`), 0o644))
	gates, err := LoadGates(dir)
	require.NoError(t, err)
	require.Contains(t, gates, PublishGateProfile)
	assert.Equal(t, []string{"make", "check"}, gates[PublishGateProfile].Command)
}

func TestLoadGates_InvalidJSON_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, GatesFileName), []byte("{"), 0o644))
	_, err := LoadGates(dir)
	require.Error(t, err)
}

func TestParseGates_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	gates, err := ParseGates([]byte(`{"full":{"command":["make","check"]}}`))
	require.NoError(t, err)
	require.Contains(t, gates, PublishGateProfile)
	assert.Equal(t, []string{"make", "check"}, gates[PublishGateProfile].Command)

	_, err = ParseGates([]byte("{"))
	require.Error(t, err)
}

func TestDefaultConfigHasNoOrchestratorSection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := DefaultConfig("go")
	require.NoError(t, WriteConfig(configPath, cfg))

	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "orchestrator", "default config must not contain orchestrator section")

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "go", loaded.ProjectType)
}
