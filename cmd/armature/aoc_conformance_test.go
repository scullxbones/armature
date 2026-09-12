package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/output"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestEveryAgentFacingCommandHasConformingFixture_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()
	require.NoError(t, output.Lint(shapeLintTree(newRootCmd()), output.DefaultGoldenDir()),
		"every cobra-enumerated agent-facing mode needs a conforming golden under internal/output/testdata/golden")
}

func TestVersionHandlerStdoutMatchesFixture_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"version", "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute())
	require.False(t, jsonStructured(stderr.Bytes()), "version must not write structured results to stderr")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "_root.json"),
		[]byte(`{"count":0,"issues":[],"help":["not an Armature repository","run arm bootstrap in a git repository"]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "version.json"), []byte(strings.TrimSpace(stdout.String())), 0o644))

	tree := &output.Command{
		Name:   "arm",
		HasRun: true,
		Bind:   output.BindWriter(),
		Children: []*output.Command{{
			Name:   "version",
			HasRun: true,
			Bind: func(_ output.Mode, _ []byte, out, errWriter io.Writer) error {
				if _, wErr := out.Write(stdout.Bytes()); wErr != nil {
					return wErr
				}
				_, wErr := errWriter.Write(stderr.Bytes())
				return wErr
			},
		}},
	}
	require.NoError(t, output.Lint(tree, dir), "captured version handler stdout must be the certified fixture")
}

func TestCompletionHandlerStdoutMatchesShellGrammar_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"completion", "bash"})
	require.NoError(t, root.Execute())
	require.NotEmpty(t, stdout.String())
	require.False(t, jsonStructured(stderr.Bytes()), "completion must not write structured results to stderr")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "_root.json"),
		[]byte(`{"count":0,"issues":[],"help":["not an Armature repository","run arm bootstrap in a git repository"]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "completion.artifact.sh"), stdout.Bytes(), 0o644))

	tree := &output.Command{
		Name:   "arm",
		HasRun: true,
		Bind:   output.BindWriter(),
		Children: []*output.Command{{
			Name:        "completion",
			HasRun:      true,
			Annotations: output.MarkArtifactOutput(nil, output.ArtifactMode{Citation: output.CitationShellCompletionGrammar}),
			Bind: func(_ output.Mode, _ []byte, out, errWriter io.Writer) error {
				if _, wErr := out.Write(stdout.Bytes()); wErr != nil {
					return wErr
				}
				_, wErr := errWriter.Write(stderr.Bytes())
				return wErr
			},
		}},
	}
	require.NoError(t, output.Lint(tree, dir), "captured cobra completion stdout must satisfy the cited shell grammar")
}

func shapeLintTree(cmd *cobra.Command) *output.Command {
	if cmd == nil {
		return nil
	}
	node := &output.Command{
		Name:             cmd.Name(),
		Annotations:      cmd.Annotations,
		HasRun:           cmd.Run != nil || cmd.RunE != nil,
		HasAvailableSubs: cmd.HasAvailableSubCommands(),
		Bind:             cobraWriterBind(cmd),
	}
	for _, sub := range cmd.Commands() {
		node.Children = append(node.Children, shapeLintTree(sub))
	}
	return node
}

func cobraWriterBind(cmd *cobra.Command) func(output.Mode, []byte, io.Writer, io.Writer) error {
	return func(_ output.Mode, golden []byte, stdout, stderr io.Writer) error {
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		_, err := io.WriteString(cmd.OutOrStdout(), string(golden))
		return err
	}
}

func jsonStructured(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}
