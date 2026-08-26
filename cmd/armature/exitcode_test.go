package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExitJSON_Format verifies the Command Failure agent envelope on stdout.
func TestExitJSON_Format(t *testing.T) {
	buf := new(bytes.Buffer)
	renderCommandFailure(buf, "json", armerrors.New("GENERAL-1", "something went wrong", nil, 1))

	out := strings.TrimSpace(buf.String())
	require.True(t, json.Valid([]byte(out)), "must be valid JSON: %q", out)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &m))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "something went wrong", errObj["cause"])
	assert.Equal(t, "GENERAL-1", errObj["code"])
	assert.Equal(t, float64(1), errObj["exit_code"])
}

// TestExitJSON_UsageError verifies a mapped USAGE Command Failure keeps exit_code 2.
func TestExitJSON_UsageError(t *testing.T) {
	buf := new(bytes.Buffer)
	renderCommandFailure(buf, "json", armerrors.New("USAGE", "bad flag", []string{"arm --help"}, 2))

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &m))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "bad flag", errObj["cause"])
	assert.Equal(t, "USAGE", errObj["code"])
	assert.Equal(t, float64(2), errObj["exit_code"])
}

// TestExitJSON_NotFound verifies unmapped "not found" errors wrap as GENERAL-1.
func TestExitJSON_NotFound(t *testing.T) {
	stdout := new(bytes.Buffer)
	code := handleRootError(stdout, new(bytes.Buffer), "json", false, fmt.Errorf("issue not found"))
	assert.Equal(t, 1, code)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &m))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "issue not found", errObj["cause"])
	assert.Equal(t, "GENERAL-1", errObj["code"])
	assert.Equal(t, float64(1), errObj["exit_code"])
}

// TestUnmappedPortErrorsWrapAsGeneral1 verifies substring classification is gone.
func TestUnmappedPortErrorsWrapAsGeneral1(t *testing.T) {
	cases := []string{
		"some unexpected problem",
		"issue E1-S1-T1 not found",
		"issue is already claimed by another worker",
		"invalid status transition from done to ready",
		"issue already exists",
		"merge conflict detected",
		"invalid state for this operation",
		"required flag: --issue",
		"permission denied: /etc/shadow",
		"no such file or directory",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			cf := armerrors.Unmapped(fmt.Errorf("%s", msg))
			require.NotNil(t, cf)
			assert.Equal(t, "GENERAL-1", cf.Code)
			assert.Equal(t, 1, cf.ExitCode)
			assert.Equal(t, msg, cf.Cause)
		})
	}
}

// TestMain_JSONFormatError verifies Execute errors map to the stdout envelope.
func TestMain_JSONFormatError(t *testing.T) {
	repo := setupRepoWithTask(t)

	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(errBuf)
	root.SetArgs([]string{"show", "--repo", repo, "--format", "json", "nonexistent-issue-xyz"})

	err := root.Execute()
	assert.Error(t, err)
	assert.Empty(t, errBuf.String(), "SilenceErrors must suppress cobra Error: on stderr")

	stdout := new(bytes.Buffer)
	code := handleRootError(stdout, new(bytes.Buffer), "json", false, err)
	assert.Equal(t, 1, code)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &m))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "GENERAL-1", errObj["code"])
	assert.Equal(t, float64(1), errObj["exit_code"])
}

// TestMain_AgentFormatError verifies --format=agent writes the same stdout envelope.
func TestMain_AgentFormatError(t *testing.T) {
	repo := setupRepoWithTask(t)

	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(errBuf)
	root.SetArgs([]string{"show", "--repo", repo, "--format", "agent", "nonexistent-issue-xyz"})

	err := root.Execute()
	assert.Error(t, err)

	stdout := new(bytes.Buffer)
	code := handleRootError(stdout, new(bytes.Buffer), "agent", false, err)
	assert.Equal(t, 1, code)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &m))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "GENERAL-1", errObj["code"])
}

// TestHandleRootError_Nil verifies nil maps to exit 0 and writes nothing.
func TestHandleRootError_Nil(t *testing.T) {
	stdout := new(bytes.Buffer)
	code := handleRootError(stdout, new(bytes.Buffer), "json", false, nil)
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout.String())
}

// TestHandleRootError_AdapterExitError verifies harness-hook platform integers
// are not rendered as Command Failures.
func TestHandleRootError_AdapterExitError(t *testing.T) {
	stdout := new(bytes.Buffer)
	code := handleRootError(stdout, new(bytes.Buffer), "json", false, adapterExitError{code: 42})
	assert.Equal(t, 42, code)
	assert.Empty(t, stdout.String())
}

// TestHandleRootError_ProtocolExitError_REQ_LNGHZN_S6_T1 verifies reports and
// git-hook errors that already wrote their payload skip the Command Failure.
func TestHandleRootError_ProtocolExitError_REQ_LNGHZN_S6_T1(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := skipCommandFailure(fmt.Errorf("validation failed with 1 error(s) and 0 warning(s)"))
	code := handleRootError(stdout, stderr, "json", true, err)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "DEBUG:")
}

// TestRenderStringSlice_NonEmpty verifies non-empty slices produce JSON arrays.
func TestRenderStringSlice_NonEmpty(t *testing.T) {
	result := renderStringSlice([]string{"a", "b", "c"})
	assert.Equal(t, `["a","b","c"]`, result)
}

// TestRenderStringSlice_Empty verifies empty slices return "[]".
func TestRenderStringSlice_Empty(t *testing.T) {
	result := renderStringSlice([]string{})
	assert.Equal(t, "[]", result)
}
