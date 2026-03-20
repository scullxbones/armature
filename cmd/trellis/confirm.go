package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newConfirmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "confirm <node-id>",
		Short: "Promote an inferred node from draft to verified confidence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeID := args[0]
			state, _, err := materialize.MaterializeAndReturn(appCtx.IssuesDir, true)
			if err != nil {
				return err
			}
			if _, ok := state.Issues[nodeID]; !ok {
				return fmt.Errorf("node %q not found", nodeID)
			}
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}
			o := ops.Op{
				Type:      ops.OpDAGTransition,
				TargetID:  nodeID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
			}
			if err := appendLowStakesOp(logPath, o); err != nil {
				return fmt.Errorf("emit dag-transition op: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "confirmed %s (inferred → verified)\n", nodeID)
			return nil
		},
	}
}
