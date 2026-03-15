package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/spf13/cobra"
)

func newMaterializeCmd() *cobra.Command {
	var repoPath string

	cmd := &cobra.Command{
		Use:   "materialize",
		Short: "Replay op logs and update materialized state files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			issuesDir := repoPath + "/.issues"

			result, err := materialize.Materialize(issuesDir, true)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Materialized %d issues from %d ops", result.IssueCount, result.OpsProcessed)
			if result.FullReplay {
				fmt.Fprint(cmd.OutOrStdout(), " (full replay)")
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")
	return cmd
}
