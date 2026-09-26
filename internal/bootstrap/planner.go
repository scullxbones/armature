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

// Plan is the full declarative harness setup plan.
type Plan struct {
	Target string
	Rows   []PlatformRow
}

// HarnessArtifactResult captures the outcome of deploying a single artifact.
type HarnessArtifactResult struct {
	Platform string `json:"platform"`
	Artifact string `json:"artifact"`
	Status   string `json:"status"`
	Action   string `json:"action,omitempty"`
	Note     string `json:"note,omitempty"`
	Error    string `json:"error,omitempty"`
}

// PlanRequest holds the inputs to BuildPlan.
type PlanRequest struct {
	Platforms []Platform // empty = DefaultPlatforms()
	Target    string
	WithHooks bool
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
	target := req.Target
	if target == "" {
		target = "local"
	}

	platforms := req.Platforms
	if len(platforms) == 0 {
		platforms = DefaultPlatforms()
	}

	for _, p := range platforms {
		if !slices.Contains(allKnownPlatforms, p) {
			return Plan{}, fmt.Errorf("unknown platform: %s", p)
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
