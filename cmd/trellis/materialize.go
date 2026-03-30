package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/spf13/cobra"
)

func newMaterializeCmd() *cobra.Command {
	var excludeWorker string

	cmd := &cobra.Command{
		Use:   "materialize",
		Short: "Replay op logs and update materialized state files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if excludeWorker != "" {
				_, result, err := materialize.MaterializeExcludeWorker(appCtx.IssuesDir, appCtx.StateDir, excludeWorker, appCtx.Mode == "single-branch")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Diagnostic replay excluding worker %s: %d issues from %d ops\n", excludeWorker, result.IssueCount, result.OpsProcessed)
				return nil
			}

			result, err := materialize.Materialize(appCtx.IssuesDir, appCtx.StateDir, appCtx.Mode == "single-branch")
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

	cmd.Flags().StringVar(&excludeWorker, "exclude-worker", "", "Diagnostic: skip all ops from this worker ID (no state update)")

	return cmd
}
