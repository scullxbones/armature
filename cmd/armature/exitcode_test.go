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

func TestExitJSON_Format(t *testing.T) {
	buf := new(bytes.Buffer)
	renderCommandFailure(buf, "json", armerrors.New("IO", "something went wrong", nil, 1))

	out := strings.TrimSpace(buf.String())
	require.True(t, json.Valid([]byte(out)), "must be valid JSON: %q", out)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &m))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "something went wrong", errObj["cause"])
	assert.Equal(t, "IO", errObj["code"])
	assert.Equal(t, float64(1), errObj["exit_code"])
}

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

func TestExitJSON_NotFound(t *testing.T) {
	stdout := new(bytes.Buffer)
	code := handleRootError(stdout, new(bytes.Buffer), "json", false, fmt.Errorf("issue not found"))
	assert.Equal(t, 1, code)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &m))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "issue not found", errObj["cause"])
	assert.Equal(t, "IO", errObj["code"])
	assert.Equal(t, float64(1), errObj["exit_code"])
}

func TestPortErrorsDoNotWrapAsGeneral1(t *testing.T) {
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
			cf := commandFailureAtPort(fmt.Errorf("%s", msg))
			require.NotNil(t, cf)
			assert.NotEqual(t, "GENERAL-1", cf.Code)
			assert.Equal(t, msg, cf.Cause)
		})
	}
}

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
	assert.Equal(t, "SHOW-1", errObj["code"])
	assert.Equal(t, float64(1), errObj["exit_code"])
}

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
	assert.Equal(t, "SHOW-1", errObj["code"])
}

func TestHandleRootError_Nil(t *testing.T) {
	stdout := new(bytes.Buffer)
	code := handleRootError(stdout, new(bytes.Buffer), "json", false, nil)
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout.String())
}

func TestHandleRootError_AdapterExitError(t *testing.T) {
	stdout := new(bytes.Buffer)
	code := handleRootError(stdout, new(bytes.Buffer), "json", false, adapterExitError{code: 42})
	assert.Equal(t, 42, code)
	assert.Empty(t, stdout.String())
}

func TestHandleRootError_ProtocolExitError_REQ_LNGHZN_S6_T1(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := skipCommandFailure(fmt.Errorf("validation failed with 1 error(s) and 0 warning(s)"))
	code := handleRootError(stdout, stderr, "json", true, err)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "validation failed with 1 error(s) and 0 warning(s)")
	assert.Contains(t, stderr.String(), "DEBUG:")
}

func TestRenderStringSlice_NonEmpty(t *testing.T) {
	result := renderStringSlice([]string{"a", "b", "c"})
	assert.Equal(t, `["a","b","c"]`, result)
}

func TestRenderStringSlice_Empty(t *testing.T) {
	result := renderStringSlice([]string{})
	assert.Equal(t, "[]", result)
}
