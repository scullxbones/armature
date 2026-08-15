// Package config loads and resolves armature's repository and worker configuration, including per-command context needed to render task specs.
package config

import (
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
)

// PublishGateProfile is the reserved name of the publish (acceptance) gate.
const PublishGateProfile = "full"

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

func WriteConfig(path string, cfg Config) error {
	return adapters.WriteConfigFile(path, cfg)
}

func LoadConfig(path string) (Config, error) {
	var cfg Config
	if err := adapters.LoadConfigFile(path, &cfg); err != nil {
		return Config{}, err
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
