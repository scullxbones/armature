// Package config loads and resolves armature's repository and worker configuration, including per-command context needed to render task specs.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
)

// PublishGateProfile is the reserved name of the publish (acceptance) gate.
const PublishGateProfile = "full"

// GatesFileName is the tracked file at the invoking checkout root that
// declares gate profiles. arm gate run reads this file, not Config.Gates.
const GatesFileName = "gates.json"

type Config struct {
	ProjectType            string                `json:"project_type"`
	DefaultTTL             int                   `json:"default_ttl"` // minutes
	TokenBudget            int                   `json:"token_budget"`
	LowStakesPushThreshold int                   `json:"low_stakes_push_threshold"` // ops before auto-push
	Hooks                  []HookConfig          `json:"hooks"`
	Gates                  map[string]GateConfig `json:"gates,omitempty"`
}

type HookConfig struct {
	Name     string   `json:"name"`
	Command  []string `json:"command"`
	Required bool     `json:"required"`
}

// RunPreTransition runs every pre-transition hook in cfg. Returns nil when
// cfg is nil or has no hooks. Each hook is invoked with JSON HookInput on
// stdin and must emit JSON HookResult on stdout; a non-zero exit or invalid
// output blocks the transition.
func RunPreTransition(cfg *Config, input adapters.HookInput) error {
	if cfg == nil || len(cfg.Hooks) == 0 {
		return nil
	}
	for _, hook := range cfg.Hooks {
		if err := adapters.ExecuteHook(hook.Name, hook.Command, input); err != nil {
			return err
		}
	}
	return nil
}

// GateConfig is a named command profile invoked by `arm gate run`.
type GateConfig struct {
	Command []string `json:"command"`
}

// Gate returns the configured profile, or false when gates are unset or the name is unknown.
func (c Config) Gate(name string) (GateConfig, bool) {
	if len(c.Gates) == 0 {
		return GateConfig{}, false
	}
	gate, ok := c.Gates[name]
	return gate, ok
}

// ParseGates decodes a gates.json document (map of profile name → command).
func ParseGates(data []byte) (map[string]GateConfig, error) {
	var gates map[string]GateConfig
	if err := json.Unmarshal(data, &gates); err != nil {
		return nil, fmt.Errorf("parse %s: %w", GatesFileName, err)
	}
	return gates, nil
}

// LoadGates reads the worktree gates.json at checkoutRoot. A missing file is
// an empty map (no error). arm gate run does not use this: it reads the
// HEAD blob via ShowFileAtCommit so skip-worktree cannot substitute a command.
func LoadGates(checkoutRoot string) (map[string]GateConfig, error) {
	path := filepath.Join(checkoutRoot, GatesFileName)
	if !adapters.StatFile(path) {
		return nil, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is checkoutRoot/gates.json
	if err != nil {
		return nil, err
	}
	return ParseGates(data)
}

func WriteConfig(path string, cfg Config) error {
	return adapters.WriteConfigFile(path, cfg)
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the repo's config.json
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg, err := StrictDecode(data)
	if err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// DetectProjectType checks for known project marker files.
func DetectProjectType(repoPath string) string {
	markers := []struct {
		file     string
		projType string
	}{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"pyproject.toml", "python"},
		{"Cargo.toml", "rust"},
		{"Makefile", "make"},
	}
	for _, m := range markers {
		if adapters.StatFile(filepath.Join(repoPath, m.file)) {
			return m.projType
		}
	}
	return "unknown"
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig(projectType string) Config {
	return Config{
		ProjectType:            projectType,
		DefaultTTL:             60,
		TokenBudget:            1600,
		LowStakesPushThreshold: 5,
		Hooks:                  []HookConfig{},
	}
}
