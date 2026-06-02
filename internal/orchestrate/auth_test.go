package orchestrate

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAuthProbe struct {
	binaryErr error
	statusOK  bool
	statusErr error
}

func (p fakeAuthProbe) HarnessBinaryPath(string) (string, error) {
	if p.binaryErr != nil {
		return "", p.binaryErr
	}
	return "/fake/harness", nil
}

func (p fakeAuthProbe) AuthStatus(string) (bool, error) {
	return p.statusOK, p.statusErr
}

func TestResolveAuthPlan_AutoPrefersAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	plan, err := ResolveAuthPlan("codex", AuthConfig{Mode: AuthModeAuto})
	require.NoError(t, err)
	assert.Equal(t, "api-key", plan.Source)
	assert.True(t, plan.APIKeyDetected)
}

func TestResolveAuthPlan_AutoUsesOAuthSession(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	plan, err := ResolveAuthPlanWithProbe("codex", AuthConfig{Mode: AuthModeAuto}, fakeAuthProbe{statusOK: true})
	require.NoError(t, err)
	assert.Equal(t, "oauth-session", plan.Source)
	assert.True(t, plan.SessionDetected)
}

func TestResolveAuthPlan_FailsWhenUnavailable(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	_, err := ResolveAuthPlanWithProbe("codex", AuthConfig{Mode: AuthModeAuto}, fakeAuthProbe{})
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
	_, err := ResolveAuthPlanWithProbe("devin", AuthConfig{Mode: AuthModeAuto}, fakeAuthProbe{binaryErr: fmt.Errorf("devin CLI not found on PATH")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLI not found on PATH")
}
