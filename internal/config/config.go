// Package config loads and resolves armature's repository and worker configuration, including per-command context needed to render task specs.
package config

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
)

// PublishGateProfile is the reserved name of the publish (acceptance) gate.
const PublishGateProfile = "full"

// GatesFileName is the tracked file at the invoking checkout root that
// declares gate profiles. arm gate run reads this file, not Config.Gates.
const GatesFileName = "gates.json"

// TTLMinutes is a claim TTL counted in whole minutes. JSON default_ttl stays
// a bare number of minutes so existing config files load unchanged.
type TTLMinutes int

// Duration returns the TTL as a time.Duration. Overflow/negative counts yield 0;
// D10 rejects those values via durationFromCount before they are relied on.
func (m TTLMinutes) Duration() time.Duration {
	d, err := durationFromCount(int64(m), time.Minute)
	if err != nil {
		return 0
	}
	return d
}

// Seconds returns the TTL as whole seconds. Overflow/negative counts yield 0;
// same bounds as Duration (via durationFromCount).
func (m TTLMinutes) Seconds() int64 {
	d, err := durationFromCount(int64(m), time.Minute)
	if err != nil {
		return 0
	}
	return int64(d / time.Second)
}

func (m TTLMinutes) overflowsConversions() bool {
	if _, err := durationFromCount(int64(m), time.Minute); err != nil {
		return true
	}
	// Idle classification uses 2 × TTL seconds (arm workers).
	n := int64(m)
	return n > math.MaxInt64/(2*60)
}

// durationFromCount is the single overflow-checked conversion from an integer
// count × unit into time.Duration. D10 and every duration-ish config consumer
// use this helper so bounds stay one source of truth. Negative counts and
// products that would overflow int64 return an error — never a wrapped value.
func durationFromCount(n int64, unit time.Duration) (time.Duration, error) {
	if n < 0 {
		return 0, fmt.Errorf("duration count %d is negative", n)
	}
	if unit <= 0 {
		return 0, fmt.Errorf("duration unit must be positive")
	}
	if n > 0 && n > math.MaxInt64/int64(unit) {
		return 0, fmt.Errorf("duration count %d overflows at unit %s", n, unit)
	}
	return time.Duration(n) * unit, nil
}

// PendingOps is a count of pending low-stakes ops. JSON
// low_stakes_push_threshold stays a bare integer.
type PendingOps int

const (
	DefaultTTLMinutes             TTLMinutes = 60
	DefaultPendingOpCount         PendingOps = 5
	DefaultLocalOnlyOpsStaleHours int        = 24
)

type Config struct {
	ProjectType            string                `json:"project_type"`
	DefaultTTL             TTLMinutes            `json:"default_ttl"`
	TokenBudget            int                   `json:"token_budget"`
	LowStakesPushThreshold PendingOps            `json:"low_stakes_push_threshold"`
	LocalOnlyOpsStaleHours int                   `json:"local_only_ops_stale_hours,omitempty"`
	Hooks                  []HookConfig          `json:"hooks"`
	Gates                  map[string]GateConfig `json:"gates,omitempty"`
	IntegrationBranch      string                `json:"integration_branch,omitempty"`
}

// LocalOnlyOpsStaleAfter returns the D11 / G2.2 staleness window. Omitted
// (zero) falls back to DefaultLocalOnlyOpsStaleHours (24h). Negative values
// and hour counts that overflow time.Duration return an error — never a
// wrapped/negative duration. A present 0 is rejected by ValidatePresentFields
// / doctor D10; overflow is also a D10 finding via the same durationFromCount.
func (c Config) LocalOnlyOpsStaleAfter() (time.Duration, error) {
	hours := int64(c.LocalOnlyOpsStaleHours)
	if hours == 0 {
		hours = int64(DefaultLocalOnlyOpsStaleHours)
	}
	if hours < 0 {
		return 0, fmt.Errorf("local_only_ops_stale_hours %d is negative", hours)
	}
	return durationFromCount(hours, time.Hour)
}

const DefaultIntegrationBranch = "main"

// IntegrationBranchOrDefault returns Config.IntegrationBranch, or "main" when empty.
func (c Config) IntegrationBranchOrDefault() string {
	if c.IntegrationBranch == "" {
		return DefaultIntegrationBranch
	}
	return c.IntegrationBranch
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

// ParseGates decodes a gates.json document (map of profile name → command).
func ParseGates(data []byte) (map[string]GateConfig, error) {
	var gates map[string]GateConfig
	if err := json.Unmarshal(data, &gates); err != nil {
		return nil, fmt.Errorf("parse %s: %w", GatesFileName, err)
	}
	return gates, nil
}

func WriteConfig(path string, cfg Config) error {
	return adapters.WriteConfigFile(path, cfg)
}

func LoadConfig(path string) (Config, error) {
	data, err := adapters.ReadFile(path)
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

func DefaultConfig(projectType string) Config {
	return Config{
		ProjectType:            projectType,
		DefaultTTL:             DefaultTTLMinutes,
		TokenBudget:            1600,
		LowStakesPushThreshold: DefaultPendingOpCount,
		Hooks:                  []HookConfig{},
	}
}
