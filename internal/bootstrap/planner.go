// Package bootstrap provides the harness setup planner for arm bootstrap.
// The planner is a pure functional module with no I/O.
package bootstrap

import (
	"fmt"
	"slices"
)

// Platform identifies a supported AI harness.
type Platform string

const (
	PlatformClaude      Platform = "claude"
	PlatformCodex       Platform = "codex"
	PlatformAntigravity Platform = "antigravity"
	PlatformDevin       Platform = "devin"
)

// ActionKind is the per-cell plan decision.
type ActionKind string

const (
	ActionInstall     ActionKind = "install"
	ActionSkip        ActionKind = "skip"
	ActionUnsupported ActionKind = "unsupported"
)

// PlatformRow is one row in the plan matrix.
type PlatformRow struct {
	Platform          Platform
	Skills            ActionKind
	PluginMetadata    ActionKind
	HarnessHookConfig ActionKind
}

// Target is a bootstrap deploy destination: local checkout or global home.
type Target string

const (
	TargetLocal  Target = "local"
	TargetGlobal Target = "global"
)

// ParseTarget maps CLI/API target strings onto the closed set. Empty input
// becomes TargetLocal. Unknown values are returned as-is so BuildPlan keeps
// today's accept-unknown behavior.
func ParseTarget(s string) Target {
	if s == "" {
		return TargetLocal
	}
	return Target(s)
}

// ArtifactKind is a harness artifact cell in a plan row.
type ArtifactKind string

const (
	ArtifactSkills            ArtifactKind = "skills"
	ArtifactPluginMetadata    ArtifactKind = "plugin_metadata"
	ArtifactHarnessHookConfig ArtifactKind = "harness_hook_config"
)

// ArtifactStatus is the recorded outcome of deploying one artifact.
type ArtifactStatus string

const (
	StatusOK          ArtifactStatus = "ok"
	StatusSkipped     ArtifactStatus = "skipped"
	StatusUnsupported ArtifactStatus = "unsupported"
	StatusError       ArtifactStatus = "error"
)

// Plan is the full declarative harness setup plan.
type Plan struct {
	Target Target
	Rows   []PlatformRow
}

// HarnessArtifactResult captures the outcome of deploying a single artifact.
type HarnessArtifactResult struct {
	Platform Platform       `json:"platform"`
	Artifact ArtifactKind   `json:"artifact"`
	Status   ArtifactStatus `json:"status"`
	Action   ActionKind     `json:"action,omitempty"`
	Note     string         `json:"note,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// PlanRequest holds the inputs to BuildPlan.
type PlanRequest struct {
	Platforms []Platform // empty = DefaultPlatforms()
	Target    Target
	WithHooks bool
}

// ParsePlatform maps a CLI platform flag onto the known set.
func ParsePlatform(s string) (Platform, error) {
	p := Platform(s)
	if !slices.Contains(allKnownPlatforms, p) {
		return "", fmt.Errorf("unknown platform: %s", s)
	}
	return p, nil
}

var allKnownPlatforms = []Platform{
	PlatformClaude, PlatformCodex, PlatformAntigravity, PlatformDevin,
}

var verifiedSkills = map[Platform]bool{
	PlatformClaude: true,
}

var verifiedPluginMetadata = map[Platform]bool{
	PlatformClaude: true,
}

var verifiedHarnessHookConfig = map[Platform]bool{
	PlatformClaude: true,
	PlatformCodex:  true,
}

// DefaultPlatforms returns the verified default platform set.
// A platform is included if it has verified skills or plugin_metadata support.
func DefaultPlatforms() []Platform {
	var result []Platform
	for _, p := range allKnownPlatforms {
		if verifiedSkills[p] || verifiedPluginMetadata[p] {
			result = append(result, p)
		}
	}
	return result
}

// BuildPlan validates the request and generates a declarative harness setup plan.
// Unknown platforms are rejected. An empty Platforms slice defaults to DefaultPlatforms();
// an empty Target defaults to "local".
func BuildPlan(req PlanRequest) (Plan, error) {
	target := ParseTarget(string(req.Target))

	platforms := req.Platforms
	if len(platforms) == 0 {
		platforms = DefaultPlatforms()
	}

	for _, p := range platforms {
		if _, err := ParsePlatform(string(p)); err != nil {
			return Plan{}, err
		}
	}

	var rows []PlatformRow
	for _, p := range platforms {
		row := PlatformRow{Platform: p}

		if verifiedSkills[p] {
			row.Skills = ActionInstall
		} else {
			row.Skills = ActionUnsupported
		}

		if verifiedPluginMetadata[p] {
			row.PluginMetadata = ActionInstall
		} else {
			row.PluginMetadata = ActionUnsupported
		}

		switch {
		case !req.WithHooks:
			row.HarnessHookConfig = ActionSkip
		case verifiedHarnessHookConfig[p]:
			row.HarnessHookConfig = ActionInstall
		default:
			row.HarnessHookConfig = ActionUnsupported
		}

		rows = append(rows, row)
	}

	return Plan{
		Target: target,
		Rows:   rows,
	}, nil
}
