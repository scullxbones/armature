package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigUnitFieldsLoadFromDisk_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"project_type": "go",
		"default_ttl": 90,
		"token_budget": 1600,
		"low_stakes_push_threshold": 7,
		"hooks": []
	}`), 0o600))

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, int64(90), int64(loaded.DefaultTTL), "default_ttl JSON minutes must load as 90 minutes")
	assert.Equal(t, 90*time.Minute, loaded.DefaultTTL.Duration())
	assert.Equal(t, int64(7), int64(loaded.LowStakesPushThreshold), "low_stakes_push_threshold JSON count must load as 7")

	require.NoError(t, WriteConfig(configPath, loaded))
	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var disk map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &disk))
	assert.JSONEq(t, `90`, string(disk["default_ttl"]))
	assert.JSONEq(t, `7`, string(disk["low_stakes_push_threshold"]))
}

func TestDefaultConfigDiskUnits_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	require.NoError(t, WriteConfig(configPath, DefaultConfig("go")))
	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var disk struct {
		DefaultTTL             json.RawMessage `json:"default_ttl"`
		LowStakesPushThreshold json.RawMessage `json:"low_stakes_push_threshold"`
	}
	require.NoError(t, json.Unmarshal(raw, &disk))
	assert.JSONEq(t, `60`, string(disk.DefaultTTL))
	assert.JSONEq(t, `5`, string(disk.LowStakesPushThreshold))
}

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
	assert.Equal(t, TTLMinutes(60), loaded.DefaultTTL)
}

func TestLoadConfigRejectsUnknownField_REQ_LNGHZN_S7_T6(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"project_type":"go","token_budegt":1600}`), 0o600))

	_, err := LoadConfig(configPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token_budegt")
}

func TestLoadConfigAcceptsRetiredModeField(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	// Live _armature config.json still carries pre-SB-ELIM "mode".
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"mode": "dual-branch",
		"project_type": "go",
		"default_ttl": 60,
		"token_budget": 1600,
		"low_stakes_push_threshold": 5,
		"hooks": []
	}`), 0o600))

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "go", loaded.ProjectType)
	assert.Equal(t, TTLMinutes(60), loaded.DefaultTTL)
	assert.Equal(t, 1600, loaded.TokenBudget)
	assert.Equal(t, PendingOps(5), loaded.LowStakesPushThreshold)
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
	full, ok := loaded.Gates[PublishGateProfile]
	require.True(t, ok)
	assert.Equal(t, []string{"make", "check"}, full.Command)
	_, ok = loaded.Gates["missing"]
	assert.False(t, ok)
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
