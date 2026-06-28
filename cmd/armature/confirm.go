package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newConfirmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "confirm <node-id>",
		Short: "Promote an inferred node from draft to verified confidence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			nodeID := args[0]
			// Read the issue directly from disk; no full rematerialization needed.
			store := newSnapshotStore(appCtx)
			if _, err := store.ReadIssue(nodeID); err != nil {
				return fmt.Errorf("node %q not found", nodeID)
			}
			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return err
			}
			o := ops.Op{
				Type:      ops.OpDAGTransition,
				TargetID:  nodeID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   ops.Payload{Confirmed: true},
			}
			if err := appendLowStakesOp(mustState(cmd), logPath, o); err != nil {
				return fmt.Errorf("emit dag-transition op: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "confirmed %s (inferred → verified)\n", nodeID)
			return nil
		},
	}
}
