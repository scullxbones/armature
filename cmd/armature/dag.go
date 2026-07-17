package main

import (
	"github.com/spf13/cobra"
)

func newDAGCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dag",
		Short: "Manage the Directed Acyclic Graph (DAG) of issues",
	}

	cmd.AddCommand(newDAGSummaryCmd())
	cmd.AddCommand(newDAGTransitionCmd())
	cmd.AddCommand(newDecomposeApplyCmd())
	cmd.AddCommand(newDecomposeContextCmd())
	cmd.AddCommand(newDecomposeRevertCmd())

	return cmd
}
