package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/spf13/cobra"
)

func newMaterializeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "materialize",
		Short: "Replay op logs and update materialized state files",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := materialize.Materialize(appCtx.IssuesDir, appCtx.Mode == "single-branch")
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Materialized %d issues from %d ops", result.IssueCount, result.OpsProcessed)
			if result.FullReplay {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), " (full replay)")
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}

	return cmd
}
