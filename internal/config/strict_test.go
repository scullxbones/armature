package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

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
	assert.Equal(t, TTLMinutes(120), cfg.DefaultTTL)
	assert.Equal(t, 3200, cfg.TokenBudget)
	assert.Equal(t, PendingOps(10), cfg.LowStakesPushThreshold)
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
	assert.Equal(t, TTLMinutes(60), cfg.DefaultTTL)
	assert.Equal(t, 1600, cfg.TokenBudget)
	assert.Equal(t, PendingOps(5), cfg.LowStakesPushThreshold)
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

func TestValidatePresentFieldsRejectsZeroLocalOnlyOpsStaleHours(t *testing.T) {
	t.Parallel()

	problems := ValidatePresentFields([]byte(`{"local_only_ops_stale_hours":0}`))
	require.NotEmpty(t, problems)
	assert.Contains(t, strings.Join(problems, "\n"), "local_only_ops_stale_hours")
}

func TestLocalOnlyOpsStaleAfterDefaultAndOverride(t *testing.T) {
	t.Parallel()

	got, err := (Config{}).LocalOnlyOpsStaleAfter()
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, got)
	got, err = (Config{LocalOnlyOpsStaleHours: 6}).LocalOnlyOpsStaleAfter()
	require.NoError(t, err)
	assert.Equal(t, 6*time.Hour, got)
}

func TestDurationConfigFields_D10AndConsumerAgree_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	type fieldCase struct {
		name    string
		jsonKey string
		unit    time.Duration
		// consumer converts a Config with the field set (no omit-default).
		consumer func(n int) (time.Duration, error)
		// d10RejectSubstring is required in ValidatePresentFields output when rejecting.
		d10RejectSubstring string
	}

	maxSafeMinutes := math.MaxInt64 / int64(time.Minute)
	// Idle window also bounds default_ttl; keep the consumer/D10 pair aligned.
	if doubled := int64(math.MaxInt64 / (2 * 60)); doubled < maxSafeMinutes {
		maxSafeMinutes = doubled
	}
	maxSafeHours := math.MaxInt64 / int64(time.Hour)

	fields := []fieldCase{
		{
			name:    "default_ttl_minutes",
			jsonKey: "default_ttl",
			unit:    time.Minute,
			consumer: func(n int) (time.Duration, error) {
				return durationFromCount(int64(TTLMinutes(n)), time.Minute)
			},
			d10RejectSubstring: "default_ttl",
		},
		{
			name:    "local_only_ops_stale_hours",
			jsonKey: "local_only_ops_stale_hours",
			unit:    time.Hour,
			// Same path D11 uses: ValidatePresentFields then LocalOnlyOpsStaleAfter.
			consumer: func(n int) (time.Duration, error) {
				raw := []byte(fmt.Sprintf(`{"local_only_ops_stale_hours":%d}`, n))
				for _, p := range ValidatePresentFields(raw) {
					if strings.Contains(p, "local_only_ops_stale_hours") {
						return 0, fmt.Errorf("%s", p)
					}
				}
				cfg, err := StrictDecode(raw)
				if err != nil {
					return 0, err
				}
				return cfg.LocalOnlyOpsStaleAfter()
			},
			d10RejectSubstring: "local_only_ops_stale_hours",
		},
	}

	for _, field := range fields {
		maxSafe := maxSafeHours
		if field.unit == time.Minute {
			maxSafe = maxSafeMinutes
		}
		if maxSafe > int64(math.MaxInt) {
			// Platform int cannot express overflow cases; still cover 0/neg/normal.
			maxSafe = int64(math.MaxInt)
		}
		overflow := maxSafe + 1
		if overflow < maxSafe {
			overflow = math.MaxInt64 // saturated
		}
		if field.unit == time.Hour {
			// Review example: 2562048 hours overflows on 64-bit Duration.
			overflow = maxSafeHours + 1
		}

		cases := []struct {
			name          string
			n             int64
			wantD10Reject bool
			wantConsumer  bool // true = consumer must succeed
			wantDuration  time.Duration
		}{
			{name: "zero", n: 0, wantD10Reject: true, wantConsumer: false},
			{name: "negative", n: -1, wantD10Reject: true, wantConsumer: false},
			{name: "normal", n: 6, wantD10Reject: false, wantConsumer: true, wantDuration: 6 * field.unit},
			{name: "max_safe", n: maxSafe, wantD10Reject: false, wantConsumer: true, wantDuration: time.Duration(maxSafe) * field.unit},
			{name: "overflow", n: overflow, wantD10Reject: true, wantConsumer: false},
		}

		for _, tc := range cases {
			t.Run(field.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				if tc.n > int64(math.MaxInt) || tc.n < int64(math.MinInt) {
					t.Skip("value not representable as int on this platform")
				}
				n := int(tc.n)
				raw := fmt.Sprintf(`{%q:%d}`, field.jsonKey, n)
				problems := ValidatePresentFields([]byte(raw))
				if tc.wantD10Reject {
					require.NotEmpty(t, problems, "D10 must reject %s", raw)
					assert.Contains(t, strings.Join(problems, "\n"), field.d10RejectSubstring)
				} else {
					assert.Empty(t, problems, "D10 must accept %s: %v", raw, problems)
				}

				got, err := field.consumer(n)
				if tc.wantConsumer {
					require.NoError(t, err, "consumer must accept value D10 accepts")
					assert.Equal(t, tc.wantDuration, got)
					assert.Empty(t, problems, "D10 and consumer must agree on acceptance")
					return
				}
				require.NotEmpty(t, problems, "D10 must reject")
				// Present 0 for default_ttl: durationFromCount(0) is representable,
				// but D10 still rejects. Hours consumer uses ValidatePresentFields
				// (same as D11) so present 0 errors in lockstep with D10.
				if field.jsonKey == "default_ttl" && n == 0 {
					got, err = durationFromCount(0, time.Minute)
					require.NoError(t, err)
					assert.Equal(t, time.Duration(0), got)
					return
				}
				require.Error(t, err, "consumer must reject value D10 rejects (incl. present 0)")
			})
		}
	}
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

func TestValidatePresentFieldsRejectsOverflowTokenBudget(t *testing.T) {
	t.Parallel()

	problems := ValidatePresentFields([]byte(`{"token_budget":` + strconv.Itoa(math.MaxInt) + `}`))
	require.NotEmpty(t, problems)
	assert.Contains(t, strings.Join(problems, "\n"), "token_budget")
}

func TestValidatePresentFieldsRejectsRetiredGates(t *testing.T) {
	t.Parallel()

	problems := ValidatePresentFields([]byte(`{"gates":{"full":{"command":["make","check"]}}}`))
	require.NotEmpty(t, problems)
	joined := strings.Join(problems, "\n")
	assert.Contains(t, joined, "gates")
	assert.Contains(t, joined, GatesFileName)
	assert.NotContains(t, joined, "command")
}

func TestStrictDecodeAcceptsIntegrationBranch(t *testing.T) {
	t.Parallel()

	cfg, err := StrictDecode([]byte(`{"project_type":"go","integration_branch":"develop"}`))
	require.NoError(t, err)
	assert.Equal(t, "develop", cfg.IntegrationBranch)
	assert.Equal(t, "develop", cfg.IntegrationBranchOrDefault())
}

func TestIntegrationBranchDefaultsToMainWhenEmpty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "main", Config{}.IntegrationBranchOrDefault())
	assert.Equal(t, DefaultIntegrationBranch, Config{IntegrationBranch: ""}.IntegrationBranchOrDefault())
}

func TestValidatePresentFieldsIgnoresRetiredMode(t *testing.T) {
	t.Parallel()

	problems := ValidatePresentFields([]byte(`{"mode":"dual-branch","project_type":"go"}`))
	assert.Empty(t, problems)
}
