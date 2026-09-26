package bootstrap_test

import (
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, bootstrap.TargetLocal, plan.Target)
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
	assert.Equal(t, bootstrap.TargetLocal, plan.Target)
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
	assert.Equal(t, bootstrap.TargetGlobal, plan.Target)
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
	assert.Equal(t, bootstrap.ActionInstall, result.Action)
}

func TestHarnessArtifactResultActionSkipped(t *testing.T) {
	t.Parallel()
	result := bootstrap.HarnessArtifactResult{
		Platform: "claude",
		Artifact: "harness_hook_config",
		Status:   "skipped",
		Action:   "install",
	}
	assert.Equal(t, bootstrap.ActionInstall, result.Action)
}

func TestHarnessArtifactResultActionUnsupported(t *testing.T) {
	t.Parallel()
	result := bootstrap.HarnessArtifactResult{
		Platform: "devin",
		Artifact: "skills",
		Status:   "unsupported",
		Action:   "unsupported",
	}
	assert.Equal(t, bootstrap.ActionUnsupported, result.Action)
}

func TestHarnessArtifactResultJSONEnvelope_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	result := bootstrap.HarnessArtifactResult{
		Platform: "claude",
		Artifact: "skills",
		Status:   "ok",
		Action:   "install",
		Note:     "Deployed to .claude/skills",
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"platform": "claude",
		"artifact": "skills",
		"status": "ok",
		"action": "install",
		"note": "Deployed to .claude/skills"
	}`, string(data))
}

func TestBuildPlanUnknownTargetPreserved_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{bootstrap.PlatformClaude},
		Target:    "elsewhere",
	})
	require.NoError(t, err)
	assert.Equal(t, "elsewhere", string(plan.Target))
}

func TestBuildPlanUnknownPlatformError_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	_, err := bootstrap.BuildPlan(bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{"codxe"},
	})
	require.Error(t, err)
	assert.Equal(t, "unknown platform: codxe", err.Error())
}

func TestParseUnknownArtifactJSON_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	var result bootstrap.HarnessArtifactResult
	require.NoError(t, json.Unmarshal([]byte(`{
		"platform": "mystery",
		"artifact": "not-an-artifact",
		"status": "weird",
		"action": "nope"
	}`), &result))
	assert.Equal(t, "mystery", string(result.Platform))
	assert.Equal(t, "not-an-artifact", string(result.Artifact))
	assert.Equal(t, "weird", string(result.Status))
	assert.Equal(t, "nope", string(result.Action))
}
