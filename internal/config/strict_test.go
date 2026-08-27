package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictDecodeRejectsUnknownField_REQ_LNGHZN_S7_T2(t *testing.T) {
	t.Parallel()

	_, err := StrictDecode([]byte(`{"project_type":"go","mystery_knob":1}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mystery_knob")
}

func TestStrictDecodeAcceptsKnownFields(t *testing.T) {
	t.Parallel()

	cfg, err := StrictDecode([]byte(`{
		"project_type": "go",
		"default_ttl": 60,
		"token_budget": 1600,
		"low_stakes_push_threshold": 5,
		"hooks": [],
		"gates": {"full": {"command": ["make", "check"]}}
	}`))
	require.NoError(t, err)
	assert.Equal(t, "go", cfg.ProjectType)
	assert.Equal(t, 60, cfg.DefaultTTL)
	assert.Equal(t, 1600, cfg.TokenBudget)
	assert.Equal(t, 5, cfg.LowStakesPushThreshold)
	assert.Empty(t, cfg.Hooks)
	require.Contains(t, cfg.Gates, PublishGateProfile)
	assert.Equal(t, []string{"make", "check"}, cfg.Gates[PublishGateProfile].Command)
}
