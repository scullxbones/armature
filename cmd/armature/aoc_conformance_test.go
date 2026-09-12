package main

import (
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

func shapeLintTree(cmd *cobra.Command) *output.Command {
	if cmd == nil {
		return nil
	}
	node := &output.Command{
		Name:             cmd.Name(),
		Annotations:      cmd.Annotations,
		HasRun:           cmd.Run != nil || cmd.RunE != nil,
		HasAvailableSubs: cmd.HasAvailableSubCommands(),
	}
	for _, sub := range cmd.Commands() {
		node.Children = append(node.Children, shapeLintTree(sub))
	}
	return node
}
