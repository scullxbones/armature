package doctor_test

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorConfigCheckD9_REQ_LNGHZN_S7_T2(t *testing.T) {
	t.Parallel()

	t.Run("unknown_field_is_error_naming_key", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{"project_type":"go","mystery_knob":true}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		assert.Contains(t, f.Items[0], "mystery_knob")
	})

	t.Run("out_of_range_present_fields_are_errors", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{
			"project_type": "fortran",
			"default_ttl": 0,
			"token_budget": -1,
			"low_stakes_push_threshold": -2
		}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		joined := strings.Join(f.Items, "\n")
		assert.Contains(t, joined, "project_type")
		assert.Contains(t, joined, "default_ttl")
		assert.Contains(t, joined, "token_budget")
		assert.Contains(t, joined, "low_stakes_push_threshold")
	})

	t.Run("retired_mode_is_ok", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{
			"mode": "dual-branch",
			"project_type": "go",
			"default_ttl": 60,
			"token_budget": 1600,
			"low_stakes_push_threshold": 5,
			"hooks": []
		}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityOK, f.Severity)
		assert.Empty(t, f.Items)
	})

	t.Run("valid_config_is_ok", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{
			"project_type": "go",
			"default_ttl": 60,
			"token_budget": 1600,
			"low_stakes_push_threshold": 5,
			"hooks": [{"name": "lint", "command": ["make", "lint"], "required": false}]
		}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityOK, f.Severity)
		assert.Empty(t, f.Items)
	})

	t.Run("omitted_fields_are_not_range_checked", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityOK, f.Severity)
	})

	t.Run("empty_hook_command_is_out_of_range", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{"hooks":[{"name":"lint","command":[]}]}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		assert.Contains(t, strings.Join(f.Items, "\n"), "hooks[0].command")
	})

	t.Run("empty_hook_name_is_out_of_range", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{"hooks":[{"name":"","command":["make","lint"]}]}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		assert.Contains(t, strings.Join(f.Items, "\n"), "hooks[0].name")
	})

	t.Run("empty_gate_command_is_out_of_range", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{"gates":{"full":{"command":[]}}}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		joined := strings.Join(f.Items, "\n")
		assert.Contains(t, joined, "gates")
		assert.Contains(t, joined, "gates.json")
	})

	t.Run("present_gates_are_unsupported", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{"gates":{"full":{"command":["make","check"]}}}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		joined := strings.Join(f.Items, "\n")
		assert.Contains(t, joined, "gates")
		assert.Contains(t, joined, "gates.json")
	})

	t.Run("missing_file_fails_open", func(t *testing.T) {
		t.Parallel()
		f := doctor.CheckD10ConfigHealth(filepath.Join(t.TempDir(), "config.json"))
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityOK, f.Severity)
	})

	t.Run("empty_path_fails_open", func(t *testing.T) {
		t.Parallel()
		f := doctor.CheckD10ConfigHealth("")
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityOK, f.Severity)
	})

	t.Run("null_document_is_error", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `null`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		assert.Contains(t, f.Items[0], "object")
	})

	t.Run("trailing_extra_json_is_error", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{"project_type":"go"}{"mystery_knob":1}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		assert.Contains(t, f.Items[0], "trailing")
	})

	t.Run("zero_push_threshold_is_out_of_range", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{"low_stakes_push_threshold":0}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		assert.Contains(t, strings.Join(f.Items, "\n"), "low_stakes_push_threshold")
	})

	t.Run("empty_hook_executable_is_out_of_range", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{"hooks":[{"name":"lint","command":[""]}]}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		assert.Contains(t, strings.Join(f.Items, "\n"), "hooks[0].command[0]")
	})

	t.Run("overflow_token_budget_is_out_of_range", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `{"token_budget":`+strconv.Itoa(math.MaxInt)+`}`)
		f := doctor.CheckD10ConfigHealth(path)
		assert.Equal(t, "D10", f.Check)
		assert.Equal(t, doctor.SeverityError, f.Severity)
		require.NotEmpty(t, f.Items)
		assert.Contains(t, strings.Join(f.Items, "\n"), "token_budget")
	})
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}
