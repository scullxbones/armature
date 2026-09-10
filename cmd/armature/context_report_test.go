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
	rootDir := projectRootDir(t)

	t.Run("human", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"context-report", "--repo", rootDir, "--format", "human"})
		require.NoError(t, cmd.Execute())
		out := buf.String()
		assert.Contains(t, out, "armature-coordinator")
		assert.Contains(t, out, "bytes/4")
		assert.Contains(t, out, "token_budget")
	})

	t.Run("json", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"context-report", "--repo", rootDir, "--format", "json"})
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
		cmd.SetArgs([]string{"context-report", "--repo", rootDir, "--format", "agent"})
		require.NoError(t, cmd.Execute())
		assert.Contains(t, buf.String(), `"estimation_method"`)
	})

	t.Run("missing repo", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs([]string{"context-report", "--repo", t.TempDir()})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "skills")
	})
}
