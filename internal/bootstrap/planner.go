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

// ArtifactKind identifies a bootstrap artifact category.
type ArtifactKind string

const (
	ArtifactSkills            ArtifactKind = "skills"
	ArtifactPluginMetadata    ArtifactKind = "plugin_metadata"
	ArtifactHarnessHookConfig ArtifactKind = "harness_hook_config"
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
	Target string // "local" or "global"
	Rows   []PlatformRow
}

// HarnessArtifactResult captures the outcome of deploying a single artifact.
type HarnessArtifactResult struct {
	Platform string `json:"platform"`         // e.g., "claude", "codex"
	Artifact string `json:"artifact"`         // e.g., "skills", "plugin_metadata", "harness_hook_config"
	Status   string `json:"status"`           // e.g., "ok", "skipped", "unsupported"
	Action   string `json:"action,omitempty"` // e.g., "install", "skip", "unsupported" — the planned action from the cell
	Note     string `json:"note,omitempty"`   // human-readable details
	Error    string `json:"error,omitempty"`  // error message if Status is "error"
}

// PlanRequest holds the inputs to BuildPlan.
type PlanRequest struct {
	Platforms []Platform // empty = DefaultPlatforms()
	Target    string     // "local" or "global"; defaults to "local"
	WithHooks bool
}

// allKnownPlatforms is the exhaustive set of platforms Armature recognises.
var allKnownPlatforms = []Platform{
	PlatformClaude, PlatformCodex, PlatformAntigravity, PlatformDevin,
}

// Verification contract: a platform/artifact is "verified" and may appear in a
// verified* map only when ALL of the following are true:
//
//  1. A writer function for that artifact exists in arm and targets the correct
//     platform-specific path.
//  2. An integration test exercises `arm bootstrap [--platform <p>]` and asserts
//     that the artifact appears at the correct path.
//  3. For harness_hook_config: additionally, ownership tests exist for both the
//     managed-file and unmanaged-file cases.
//
// To add a platform, implement the writer, add the integration test, and update
// the map below. Do not add a platform entry based on future intent alone.

// verifiedSkills lists platforms with a verified arm-level skills+flat deploy path.
var verifiedSkills = map[Platform]bool{
	PlatformClaude: true,
}

// verifiedPluginMetadata lists platforms with a verified plugin metadata deploy path.
var verifiedPluginMetadata = map[Platform]bool{
	PlatformClaude: true,
}

// verifiedHarnessHookConfig lists platforms whose WriteConfig and OwnsConfig are
// implemented and tested in harnesshook.
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
