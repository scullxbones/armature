package main

import (
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newUnlinkCmd() *cobra.Command {
	var sourceID, dep string

	cmd := &cobra.Command{
		Use:   "unlink",
		Short: "Remove a dependency link between issues",
		Long: `Remove a dependency relationship between two issues.

Unlink a source issue from a dependency (typically a blocked_by relationship).
This removes erroneous dependency links that were previously created with the link command.`,
		Example: `  # Remove blocked_by dependency
  $ arm unlink --source E6-S4-T2 --dep E6-S4-T1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpUnlink, TargetID: sourceID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Dep: dep, Rel: "blocked_by"}}
			if err := appendOp(ctx, logPath, op); err != nil {
				return err
			}
			writeCommandResult(cmd, map[string]string{"source": sourceID, "dep": dep},
				"Unlinked %s from %s\n", sourceID, dep)
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceID, "source", "", "source issue ID")
	cmd.Flags().StringVar(&dep, "dep", "", "dependency issue ID to remove")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("dep")
	return cmd
}
