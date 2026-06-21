package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/spf13/cobra"
)

func newMaterializeCmd() *cobra.Command {
	var excludeWorker string

	cmd := &cobra.Command{
		Use:   "materialize",
		Short: "Replay op logs and update materialized state files",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read all ops from log files
			allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(appCtx.IssuesDir, "ops"))
			if err != nil {
				return fmt.Errorf("read ops: %w", err)
			}

			if excludeWorker != "" {
				_, result, err := materialize.MaterializeExcludeWorker(allOps, excludeWorker, appCtx.Mode == "single-branch")
				if err != nil {
					return err
				}
				msg := fmt.Sprintf("Diagnostic replay excluding worker %s: %d issues from %d ops",
					excludeWorker, result.IssueCount, result.OpsProcessed)
				if len(result.UnhandledOps) > 0 {
					msg += fmt.Sprintf(" [%d unhandled ops]", len(result.UnhandledOps))
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), msg)
				return nil
			}

			result, err := materialize.Materialize(appCtx.StateDir, allOps, appCtx.Mode == "single-branch", offsets)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"Materialized %d issues from %d ops", result.IssueCount, result.OpsProcessed)
			if result.FullReplay {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), " (full replay)")
			}
			if len(result.UnhandledOps) > 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), " [%d unhandled ops]", len(result.UnhandledOps))
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}

	cmd.Flags().StringVar(&excludeWorker, "exclude-worker", "", "Diagnostic: skip all ops from this worker ID (no state update)")

	return cmd
}
