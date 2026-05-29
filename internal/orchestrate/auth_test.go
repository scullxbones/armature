package orchestrate

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAuthPlan_AutoPrefersAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	plan, err := ResolveAuthPlan("codex", AuthConfig{Mode: AuthModeAuto})
	require.NoError(t, err)
	assert.Equal(t, "api-key", plan.Source)
	assert.True(t, plan.APIKeyDetected)
}

func TestResolveAuthPlan_AutoUsesOAuthSession(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	orig := authStatusCommand
	authStatusCommand = func(harness string) (bool, error) { return harness == "codex", nil }
	t.Cleanup(func() { authStatusCommand = orig })

	plan, err := ResolveAuthPlan("codex", AuthConfig{Mode: AuthModeAuto})
	require.NoError(t, err)
	assert.Equal(t, "oauth-session", plan.Source)
	assert.True(t, plan.SessionDetected)
}

func TestResolveAuthPlan_FailsWhenUnavailable(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	orig := authStatusCommand
	authStatusCommand = func(string) (bool, error) { return false, nil }
	t.Cleanup(func() { authStatusCommand = orig })

	_, err := ResolveAuthPlan("codex", AuthConfig{Mode: AuthModeAuto})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "codex auth unavailable")
	assert.Contains(t, err.Error(), "codex login")
}

func TestResolveAuthPlan_EnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(path, []byte("OPENAI_API_KEY=file-key\n"), 0o644))

	plan, err := ResolveAuthPlan("codex", AuthConfig{Mode: AuthModeEnvFile, EnvFile: path})
	require.NoError(t, err)
	assert.Equal(t, "api-key", plan.Source)
	assert.Equal(t, "file-key", plan.Env["OPENAI_API_KEY"])
}

func TestResolveAuthPlan_MissingBinaryFirst(t *testing.T) {
	t.Setenv("DEVIN_API_KEY", "")
	origLookup := lookPathCommand
	lookPathCommand = func(file string) (string, error) {
		if file == "devin" {
			return "", fmt.Errorf("not found")
		}
		return origLookup(file)
	}
	t.Cleanup(func() { lookPathCommand = origLookup })
	_, err := ResolveAuthPlan("devin", AuthConfig{Mode: AuthModeAuto})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLI not found on PATH")
}
