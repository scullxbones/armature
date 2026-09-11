package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/contextreport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextReportCommandHumanAndJSON(t *testing.T) {
	t.Run("human", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"context-report", "--format", "human"})
		require.NoError(t, cmd.Execute())
		out := buf.String()
		assert.Contains(t, out, "list")
		assert.Contains(t, out, "ready")
		assert.Contains(t, out, "show")
		assert.Contains(t, out, "render-context")
		assert.Contains(t, out, "review")
		assert.Contains(t, out, "bytes/4")
		assert.Contains(t, out, "token_budget")
	})

	t.Run("json", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"context-report", "--format", "json"})
		require.NoError(t, cmd.Execute())
		var report contextreport.Report
		require.NoError(t, json.Unmarshal(buf.Bytes(), &report))
		assert.Contains(t, report.EstimationMethod, "bytes/4")
		assert.NotEmpty(t, report.Artifacts)
	})

	t.Run("agent", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"context-report", "--format", "agent"})
		require.NoError(t, cmd.Execute())
		assert.Contains(t, buf.String(), `"estimation_method"`)
		assert.Contains(t, buf.String(), `"invocation"`)
	})

	t.Run("ordinary repo", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"context-report", "--repo", t.TempDir(), "--format", "human"})
		require.NoError(t, cmd.Execute(), "embedded fixtures must not depend on --repo")
		assert.Contains(t, buf.String(), "list")
	})
}

// TestContextReportNonTTYDefaultsToJSON_REQ_NXTTN_S3_T1 verifies that when
// stdout is not a terminal and --format is not explicitly set, context-report
// emits JSON. The command's PersistentPreRunE bypasses root's hook
// (config.ResolveContext is not needed; fixtures are embedded), so it must
// call autoDetectTTYPolicy itself — the same pattern as bootstrap.
func TestContextReportNonTTYDefaultsToJSON_REQ_NXTTN_S3_T1(t *testing.T) {
	t.Setenv("TERM", "dumb")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"context-report"})
	require.NoError(t, cmd.Execute())

	output := buf.String()
	var report contextreport.Report
	require.NoError(t, json.Unmarshal(buf.Bytes(), &report),
		"non-TTY without --format must auto-detect agent/JSON, not the human table")
	assert.Contains(t, report.EstimationMethod, "bytes/4")
	assert.NotEmpty(t, report.Artifacts)
	assert.NotContains(t, output, "EST_TOKENS")
	assert.NotContains(t, output, "Context report (fixture-measured main-path CLI)",
		"should not emit the human table in non-TTY")
}
