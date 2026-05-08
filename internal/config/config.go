package config

import (
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
)

type Config struct {
	Mode                   string             `json:"mode"` // "single-branch" or "dual-branch"
	ProjectType            string             `json:"project_type"`
	DefaultTTL             int                `json:"default_ttl"` // minutes
	TokenBudget            int                `json:"token_budget"`
	LowStakesPushThreshold int                `json:"low_stakes_push_threshold"` // ops before auto-push
	Hooks                  []HookConfig       `json:"hooks"`
	Orchestrator           OrchestratorConfig `json:"orchestrator,omitempty"`
}

// OrchestratorConfig holds settings for arm orchestrate.
type OrchestratorConfig struct {
	MaxParallel    int             `json:"max_parallel,omitempty"`
	SandboxEnabled bool            `json:"sandbox_enabled,omitempty"`
	Adapters       AdapterCommands `json:"adapters,omitempty"`
}

// AdapterCommands holds the shell commands used by each verification phase.
type AdapterCommands struct {
	Build    string `json:"build,omitempty"`
	Lint     string `json:"lint,omitempty"`
	Test     string `json:"test,omitempty"`
	Coverage string `json:"coverage,omitempty"`
	Mutate   string `json:"mutate,omitempty"`
}

type HookConfig struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Required bool   `json:"required"`
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

// DefaultConfig returns a config with sensible defaults for single-branch mode.
func DefaultConfig(projectType string) Config {
	return Config{
		Mode:                   "single-branch",
		ProjectType:            projectType,
		DefaultTTL:             60,
		TokenBudget:            1600,
		LowStakesPushThreshold: 5,
		Hooks:                  []HookConfig{},
	}
}
