package config

import (
	"math"
	"strconv"
	"strings"
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

func TestStrictDecodeAcceptsRetiredModeField(t *testing.T) {
	t.Parallel()

	cfg, err := StrictDecode([]byte(`{
		"mode": "dual-branch",
		"project_type": "go",
		"default_ttl": 120,
		"token_budget": 3200,
		"low_stakes_push_threshold": 10,
		"hooks": []
	}`))
	require.NoError(t, err)
	assert.Equal(t, "go", cfg.ProjectType)
	assert.Equal(t, 120, cfg.DefaultTTL)
	assert.Equal(t, 3200, cfg.TokenBudget)
	assert.Equal(t, 10, cfg.LowStakesPushThreshold)
	assert.Empty(t, cfg.Hooks)
}

func TestStrictDecodeStillRejectsUnknownAlongsideRetiredMode(t *testing.T) {
	t.Parallel()

	_, err := StrictDecode([]byte(`{"mode":"dual-branch","mystery_knob":1}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mystery_knob")
	assert.NotContains(t, err.Error(), `"mode"`)
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

func TestStrictDecodeRejectsNonObjectDocument(t *testing.T) {
	t.Parallel()

	_, err := StrictDecode([]byte(`null`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "object")
}

func TestStrictDecodeRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	_, err := StrictDecode([]byte(`{"project_type":"go"}{"mystery_knob":1}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing")
}

func TestValidatePresentFieldsRejectsZeroPushThreshold(t *testing.T) {
	t.Parallel()

	problems := ValidatePresentFields([]byte(`{"low_stakes_push_threshold":0}`))
	require.NotEmpty(t, problems)
	assert.Contains(t, strings.Join(problems, "\n"), "low_stakes_push_threshold")
}

func TestValidatePresentFieldsRejectsEmptyHookExecutable(t *testing.T) {
	t.Parallel()

	problems := ValidatePresentFields([]byte(`{"hooks":[{"name":"lint","command":[""]}]}`))
	require.NotEmpty(t, problems)
	assert.Contains(t, strings.Join(problems, "\n"), "hooks[0].command[0]")
}

func TestValidatePresentFieldsRejectsOverflowTTL(t *testing.T) {
	t.Parallel()
	if int64(math.MaxInt) <= math.MaxInt64/60 {
		t.Skip("int cannot hold a TTL that overflows seconds on this platform")
	}

	problems := ValidatePresentFields([]byte(`{"default_ttl":` + strconv.FormatInt(math.MaxInt64, 10) + `}`))
	require.NotEmpty(t, problems)
	assert.Contains(t, strings.Join(problems, "\n"), "default_ttl")
}

func TestValidatePresentFieldsIgnoresRetiredMode(t *testing.T) {
	t.Parallel()

	problems := ValidatePresentFields([]byte(`{"mode":"dual-branch","project_type":"go"}`))
	assert.Empty(t, problems)
}
