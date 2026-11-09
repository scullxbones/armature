package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/output"
	"github.com/spf13/cobra"
)

func TestArmMcpIsProtocolOutput_REQ_MCPSTDIO_S1_T4(t *testing.T) {
	t.Parallel()
	cmd := newMCPCmd()
	require.True(t, cmd.Hidden)
	require.Equal(t, output.ChannelProtocolOutput, output.Classify(cmd.Annotations))
	require.NotNil(t, cmd.PersistentPreRunE)
	require.NoError(t, cmd.PersistentPreRunE(cmd, nil))
}

func TestProtocolOutputCommandsAreHarnessHookAndMcp_REQ_MCPSTDIO_S1_T4(t *testing.T) {
	t.Parallel()
	root := testRootCmd(t)
	var protocol []string
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if output.Classify(c.Annotations) == output.ChannelProtocolOutput {
			protocol = append(protocol, c.Name())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	require.ElementsMatch(t, []string{"harness-hook", "mcp"}, protocol)

	roots := platformProtocolRoots()
	assert.True(t, roots["mcp"])
	assert.True(t, roots["harness-hook"])
	assert.True(t, roots["hook"])
}

func TestAdmitToolArgsRefusesWorkerInit_REQ_MCPSTDIO_S1_T4(t *testing.T) {
	t.Parallel()
	require.Error(t, admitToolArgs([]string{"worker-init"}))
	require.Error(t, admitToolArgs([]string{"--format=json", "worker-init"}))
	require.Error(t, admitToolArgs([]string{"--format", "json", "worker-init"}))
	require.Error(t, admitToolArgs([]string{"--repo", "/tmp", "worker-init"}))
	require.Error(t, admitToolArgs([]string{"harness-hook"}))
	require.Error(t, admitToolArgs([]string{"--format=json", "harness-hook"}))
	require.Error(t, admitToolArgs([]string{"hook", "run", "pre-commit"}))
	require.Error(t, admitToolArgs([]string{"mcp"}))
	require.Error(t, admitToolArgs([]string{"review", "prepare"}))
	require.Error(t, admitToolArgs([]string{"completion", "bash"}))
	require.NoError(t, admitToolArgs([]string{"ready"}))
	require.NoError(t, admitToolArgs([]string{"--format=json", "ready"}))
	require.NoError(t, admitToolArgs([]string{"list", "--external-ref"}))
}

func TestToolRunnerSurfacesStderrOnlyFailures_REQ_MCPSTDIO_S1(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Unknown flag: cobra writes the reason to stderr and exits non-zero with empty stdout.
	text, exitCode, raw, err := runMCPToolCallRaw(t, repo, []string{"ready", "--not-a-real-flag"})
	require.NoError(t, err)
	assert.NotEqual(t, 0, exitCode)
	assert.NotEmpty(t, text, "stderr-only failures must fold diagnostics into tool result text/stdout")
	assert.Contains(t, strings.ToLower(text), "flag")
	assert.Contains(t, raw, `"stdout"`)
	assert.Contains(t, raw, `"isError":true`)
	// Protocol stream is JSON-RPC only: no bare stderr line outside the response object.
	for _, line := range nonEmptyMCPLines(raw) {
		require.True(t, strings.HasPrefix(strings.TrimSpace(line), "{"), "protocol stdout must stay JSON-RPC: %q", line)
	}
}

func TestMCPRefusesWorkerIDFlag(t *testing.T) {
	t.Parallel()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := Run(context.Background(), Invocation{
		Args:   []string{"mcp", "--worker-id", "x"},
		Stdin:  strings.NewReader(""),
		Stdout: stdout,
		Stderr: stderr,
	})
	require.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String()+stdout.String(), "worker-id")
}

func TestMCPToolsListRoundTrip(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	req := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		``,
	}, "\n")
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := Run(context.Background(), Invocation{
		Args:    []string{"mcp", "--repo", repo},
		Dir:     repo,
		Env:     OverlayEnv(OSEnv(), cliEnvFor(t)),
		Stdin:   strings.NewReader(req),
		Stdout:  stdout,
		Stderr:  stderr,
		TempDir: t.TempDir(),
	})
	require.Equal(t, 0, code, "stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), `"name":"arm"`)

	var last map[string]any
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &last))
}
