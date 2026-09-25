package bootstrap_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/stretchr/testify/assert"
)

func TestDefaultPlatformsIncludesClaude(t *testing.T) {
	t.Parallel()
	platforms := bootstrap.DefaultPlatforms()
	assert.Contains(t, platforms, bootstrap.PlatformClaude)
}

func TestDefaultPlatformsDoesNotIncludeUnverified(t *testing.T) {
	t.Parallel()
	platforms := bootstrap.DefaultPlatforms()
	assert.NotContains(t, platforms, bootstrap.PlatformAntigravity)
}

func TestDefaultPlatformsExcludesHooksOnlyPlatform(t *testing.T) {
	t.Parallel()
	platforms := bootstrap.DefaultPlatforms()
	assert.NotContains(t, platforms, bootstrap.PlatformCodex)
}

func TestBuildPlanDefaults(t *testing.T) {
	t.Parallel()
	req := bootstrap.PlanRequest{}
	plan, err := bootstrap.BuildPlan(req)
	assert.NoError(t, err)
	assert.Equal(t, "local", plan.Target)
	assert.Greater(t, len(plan.Rows), 0)
	defaultPlatforms := bootstrap.DefaultPlatforms()
	assert.Equal(t, len(defaultPlatforms), len(plan.Rows))
	for _, row := range plan.Rows {
		assert.Contains(t, defaultPlatforms, row.Platform)
	}
}

func TestBuildPlanUnknownPlatform(t *testing.T) {
	t.Parallel()
	req := bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{"unknown_platform"},
		Target:    "local",
		WithHooks: false,
	}
	_, err := bootstrap.BuildPlan(req)
	assert.Error(t, err)
}

func TestBuildPlanWithHooks(t *testing.T) {
	t.Parallel()
	req := bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{bootstrap.PlatformClaude},
		Target:    "local",
		WithHooks: true,
	}
	plan, err := bootstrap.BuildPlan(req)
	assert.NoError(t, err)
	assert.Equal(t, "local", plan.Target)
	assert.Equal(t, 1, len(plan.Rows))
	row := plan.Rows[0]
	assert.Equal(t, bootstrap.PlatformClaude, row.Platform)
	assert.Equal(t, bootstrap.ActionInstall, row.Skills)
	assert.Equal(t, bootstrap.ActionInstall, row.PluginMetadata)
	assert.Equal(t, bootstrap.ActionInstall, row.HarnessHookConfig)
}

func TestBuildPlanCodexRow(t *testing.T) {
	t.Parallel()
	req := bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{bootstrap.PlatformCodex},
		Target:    "local",
		WithHooks: true,
	}
	plan, err := bootstrap.BuildPlan(req)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(plan.Rows))
	row := plan.Rows[0]
	assert.Equal(t, bootstrap.ActionUnsupported, row.Skills)
	assert.Equal(t, bootstrap.ActionUnsupported, row.PluginMetadata)
	assert.Equal(t, bootstrap.ActionInstall, row.HarnessHookConfig)
}

func TestBuildPlanWithoutHooks(t *testing.T) {
	t.Parallel()
	req := bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{bootstrap.PlatformClaude},
		Target:    "global",
		WithHooks: false,
	}
	plan, err := bootstrap.BuildPlan(req)
	assert.NoError(t, err)
	assert.Equal(t, "global", plan.Target)
	assert.Equal(t, 1, len(plan.Rows))
	row := plan.Rows[0]
	assert.Equal(t, bootstrap.PlatformClaude, row.Platform)
	assert.Equal(t, bootstrap.ActionInstall, row.Skills)
	assert.Equal(t, bootstrap.ActionInstall, row.PluginMetadata)
	assert.Equal(t, bootstrap.ActionSkip, row.HarnessHookConfig)
}

func TestHarnessArtifactResultIncludesAction(t *testing.T) {
	t.Parallel()
	result := bootstrap.HarnessArtifactResult{
		Platform: "claude",
		Artifact: "skills",
		Status:   "ok",
		Action:   "install",
	}
	assert.Equal(t, "install", result.Action)
}

func TestHarnessArtifactResultActionSkipped(t *testing.T) {
	t.Parallel()
	result := bootstrap.HarnessArtifactResult{
		Platform: "claude",
		Artifact: "harness_hook_config",
		Status:   "skipped",
		Action:   "install",
	}
	assert.Equal(t, "install", result.Action)
}

func TestHarnessArtifactResultActionUnsupported(t *testing.T) {
	t.Parallel()
	result := bootstrap.HarnessArtifactResult{
		Platform: "devin",
		Artifact: "skills",
		Status:   "unsupported",
		Action:   "unsupported",
	}
	assert.Equal(t, "unsupported", result.Action)
}
