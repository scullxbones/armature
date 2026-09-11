package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/contextreport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextReportEnvelope struct {
	Count                int                      `json:"count"`
	Artifacts            []contextreport.Artifact `json:"artifacts"`
	EstimationMethod     string                   `json:"estimation_method"`
	TotalBytes           int                      `json:"total_bytes"`
	TotalEstimatedTokens int                      `json:"total_estimated_tokens"`
	Help                 []string                 `json:"help"`
}

func decodeContextReportEnvelope(t *testing.T, raw []byte) contextReportEnvelope {
	t.Helper()
	var env contextReportEnvelope
	require.NoError(t, json.Unmarshal(raw, &env), "stdout must be one JSON envelope object, got %s", raw)
	assert.Equal(t, len(env.Artifacts), env.Count, "N2: count must equal artifacts length")
	assert.NotEmpty(t, env.Artifacts)
	assert.NotEmpty(t, env.Help)
	assert.Contains(t, env.EstimationMethod, "bytes/4")
	assert.Greater(t, env.TotalBytes, 0)
	assert.Greater(t, env.TotalEstimatedTokens, 0)
	assert.NotContains(t, string(raw), `"PATH"`)
	return env
}

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
		assert.Contains(t, out, "PATH")
	})

	t.Run("json", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"context-report", "--repo", rootDir, "--format", "json"})
		require.NoError(t, cmd.Execute())
		decodeContextReportEnvelope(t, buf.Bytes())
	})

	t.Run("agent", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := newRootCmd()
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"context-report", "--repo", rootDir, "--format", "agent"})
		require.NoError(t, cmd.Execute())
		decodeContextReportEnvelope(t, buf.Bytes())
	})

	t.Run("missing repo", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs([]string{"context-report", "--repo", t.TempDir()})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "skills")
		assert.NotContains(t, err.Error(), "ops-worktree-path")
	})
}

func TestContextReportStructuredUsesAgentEnvelope_REQ_NXTTN_S3_T1(t *testing.T) {
	rootDir := projectRootDir(t)

	jsonBuf := new(bytes.Buffer)
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(jsonBuf)
	jsonCmd.SetArgs([]string{"context-report", "--repo", rootDir, "--format", "json"})
	require.NoError(t, jsonCmd.Execute())
	jsonEnv := decodeContextReportEnvelope(t, jsonBuf.Bytes())

	agentBuf := new(bytes.Buffer)
	agentCmd := newRootCmd()
	agentCmd.SetOut(agentBuf)
	agentCmd.SetArgs([]string{"context-report", "--repo", rootDir, "--format", "agent"})
	require.NoError(t, agentCmd.Execute())
	agentEnv := decodeContextReportEnvelope(t, agentBuf.Bytes())

	assert.Equal(t, jsonEnv.Count, agentEnv.Count)
	assert.Equal(t, jsonEnv.Artifacts, agentEnv.Artifacts)
	assert.Equal(t, jsonEnv.EstimationMethod, agentEnv.EstimationMethod)
	assert.Equal(t, jsonEnv.TotalBytes, agentEnv.TotalBytes)
	assert.Equal(t, jsonEnv.TotalEstimatedTokens, agentEnv.TotalEstimatedTokens)
	assert.Equal(t, jsonEnv.Help, agentEnv.Help)

	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(jsonBuf.Bytes(), &keys))
	for _, want := range []string{"count", "artifacts", "help", "estimation_method", "total_bytes", "total_estimated_tokens"} {
		_, ok := keys[want]
		assert.True(t, ok, "envelope missing %s", want)
	}
	_, hasPayload := keys["payload"]
	assert.False(t, hasPayload, "payload key must be artifacts, not payload")
}

func TestContextReportNonTTYDefaultsToAgentEnvelope_REQ_NXTTN_S3_T1(t *testing.T) {
	rootDir := projectRootDir(t)
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	// Tests run non-TTY (tui.IsTerminal is false). Without --format, PersistentPreRunE
	// must still call autoDetectTTYPolicy so the default is agent, not the human table.
	cmd.SetArgs([]string{"context-report", "--repo", rootDir})
	require.NoError(t, cmd.Execute())
	out := buf.String()
	assert.NotContains(t, out, "PATH")
	assert.NotContains(t, out, "Context report (static")
	decodeContextReportEnvelope(t, buf.Bytes())
}

func TestContextReportSkipsRepoStateInit_REQ_NXTTN_S3_T1(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write("internal/skillsembed/skills/demo/SKILL.md", "abcd")
	write("CONTEXT.md", "glossary-body")
	write("docs/commands.md", "commands-body")
	write("docs/concepts.md", "concepts-body")
	write("docs/use-cases.md", "use-cases-body")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"context-report", "--repo", root, "--format", "json"})
	require.NoError(t, cmd.Execute(), "must not require armature.ops-worktree-path / ResolveContext")
	env := decodeContextReportEnvelope(t, buf.Bytes())
	assert.Equal(t, 5, env.Count)
}
