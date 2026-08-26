package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandFailureAgentEnvelope_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	cf := armerrors.New("GENERAL-1", "disk full", []string{"arm doctor"}, 1)
	buf := new(bytes.Buffer)
	renderCommandFailure(buf, "agent", cf)

	raw := strings.TrimSpace(buf.String())
	require.True(t, json.Valid([]byte(raw)), "envelope must be valid JSON: %q", raw)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
	_, hasCount := envelope["count"]
	_, hasPayload := envelope["payload"]
	_, hasHelp := envelope["help"]
	assert.False(t, hasCount, "failure must not nest in the AOC success envelope")
	assert.False(t, hasPayload, "failure must not nest in the AOC success envelope")
	assert.False(t, hasHelp, "failure must not nest in the AOC success envelope")

	errObj, ok := envelope["error"].(map[string]any)
	require.True(t, ok, "stdout object must be {error:{...}}")
	assert.Equal(t, "GENERAL-1", errObj["code"])
	assert.Equal(t, "disk full", errObj["cause"])
	assert.Equal(t, float64(1), errObj["exit_code"])
	actions, ok := errObj["next_actions"].([]any)
	require.True(t, ok, "next_actions must be a JSON array")
	require.Len(t, actions, 1)
	assert.Equal(t, "arm doctor", actions[0])

	// json ≡ agent
	jsonBuf := new(bytes.Buffer)
	renderCommandFailure(jsonBuf, "json", cf)
	assert.JSONEq(t, raw, strings.TrimSpace(jsonBuf.String()))

	// round-trip the inner CommandFailure documented by ADR 0020
	inner, err := json.Marshal(cf)
	require.NoError(t, err)
	var round armerrors.CommandFailure
	require.NoError(t, json.Unmarshal(inner, &round))
	assert.Equal(t, cf.Code, round.Code)
	assert.Equal(t, cf.Cause, round.Cause)
	assert.Equal(t, cf.NextActions, round.NextActions)
	assert.Equal(t, cf.ExitCode, round.ExitCode)
}

func TestCommandFailureHumanRendering_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	cf := armerrors.New("GENERAL-1", "disk full", []string{"arm doctor", "arm --help"}, 1)
	buf := new(bytes.Buffer)
	renderCommandFailure(buf, "human", cf)
	got := buf.String()
	assert.Equal(t, "Error [GENERAL-1]: disk full\nTry: arm doctor\nTry: arm --help\n", got)

	empty := armerrors.New("GENERAL-1", "boom", nil, 1)
	emptyBuf := new(bytes.Buffer)
	renderCommandFailure(emptyBuf, "human", empty)
	assert.Equal(t, "Error [GENERAL-1]: boom\n", emptyBuf.String())
	assert.NotContains(t, emptyBuf.String(), "Try:")
}

func TestRootSilenceErrors_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	assert.True(t, root.SilenceErrors, "root must SilenceErrors so cobra does not print a duplicate Error: line")
}

func TestClassifyErrorRemoved_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for _, name := range []string{"helpers.go", "main.go"} {
		src, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.NotContains(t, string(src), "func classifyError", "%s must not define classifyError", name)
		assert.NotContains(t, string(src), "func writeJSONError", "%s must not define writeJSONError", name)
		assert.NotContains(t, string(src), "jsonErrorPayload", "%s must not keep the stderr JSON error payload", name)
	}
}

func TestHandleRootErrorWritesAgentEnvelopeToStdout_REQ_LNGHZN_S6_T1(t *testing.T) {
	t.Parallel()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := handleRootError(stdout, stderr, "agent", true, fmt.Errorf("issue missing"))
	assert.Equal(t, 1, code)
	assert.NotContains(t, stdout.String(), `"count"`)
	assert.Contains(t, stdout.String(), `"code":"GENERAL-1"`)
	assert.Contains(t, stderr.String(), "DEBUG:")
	assert.NotContains(t, stderr.String(), `"error"`)
}
