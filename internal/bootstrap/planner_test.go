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

func TestBuildPlanDefaults(t *testing.T) {
	t.Parallel()
	req := bootstrap.PlanRequest{
		Platforms: nil, // empty
		Target:    "",  // empty
		WithHooks: false,
	}
	plan, err := bootstrap.BuildPlan(req)
	assert.NoError(t, err)
	assert.Equal(t, "local", plan.Target)
	assert.NotNil(t, plan.Rows)
	assert.Greater(t, len(plan.Rows), 0)
	// Verify all default platforms are present
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
	// Claude has verified skills, plugin metadata, and hook config
	assert.Equal(t, bootstrap.ActionInstall, row.Skills)
	assert.Equal(t, bootstrap.ActionInstall, row.PluginMetadata)
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
	// Skills and plugin metadata should still be install if verified
	assert.Equal(t, bootstrap.ActionInstall, row.Skills)
	assert.Equal(t, bootstrap.ActionInstall, row.PluginMetadata)
	// HarnessHookConfig should be skip when WithHooks=false
	assert.Equal(t, bootstrap.ActionSkip, row.HarnessHookConfig)
}
