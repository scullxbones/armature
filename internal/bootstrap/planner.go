// Package bootstrap provides the harness setup planner for arm bootstrap.
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
// becomes TargetLocal. Unknown values are rejected.
func ParseTarget(s string) (Target, error) {
	if s == "" {
		return TargetLocal, nil
	}
	t := Target(s)
	if !slices.Contains(allKnownTargets, t) {
		return "", fmt.Errorf("unknown target %q: allowed values are %q, %q", s, TargetLocal, TargetGlobal)
	}
	return t, nil
}

var allKnownTargets = []Target{TargetLocal, TargetGlobal}

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

type platformArtifacts struct {
	skills, pluginMetadata, harnessHookConfig bool
}

var verifiedArtifacts = map[Platform]platformArtifacts{
	PlatformClaude: {skills: true, pluginMetadata: true, harnessHookConfig: true},
	PlatformCodex:  {harnessHookConfig: true},
}

func installOrUnsupported(verified bool) ActionKind {
	if verified {
		return ActionInstall
	}
	return ActionUnsupported
}

// DefaultPlatforms returns the verified default platform set.
// A platform is included if it has verified skills or plugin_metadata support.
func DefaultPlatforms() []Platform {
	var result []Platform
	for _, p := range allKnownPlatforms {
		a := verifiedArtifacts[p]
		if a.skills || a.pluginMetadata {
			result = append(result, p)
		}
	}
	return result
}

// BuildPlan validates the request and generates a declarative harness setup plan.
// Unknown platforms are rejected. An empty Platforms slice defaults to DefaultPlatforms();
// an empty Target defaults to local. Unknown Target values are rejected.
func BuildPlan(req PlanRequest) (Plan, error) {
	target, err := ParseTarget(string(req.Target))
	if err != nil {
		return Plan{}, err
	}

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
		a := verifiedArtifacts[p]
		row := PlatformRow{
			Platform:       p,
			Skills:         installOrUnsupported(a.skills),
			PluginMetadata: installOrUnsupported(a.pluginMetadata),
		}

		switch {
		case !req.WithHooks:
			row.HarnessHookConfig = ActionSkip
		default:
			row.HarnessHookConfig = installOrUnsupported(a.harnessHookConfig)
		}

		rows = append(rows, row)
	}

	return Plan{
		Target: target,
		Rows:   rows,
	}, nil
}
