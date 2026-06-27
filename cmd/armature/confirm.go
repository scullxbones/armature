package main

import (
	"context"
	"fmt"

	"github.com/scullxbones/armature/internal/materialize"
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
			// Load snapshot to get materialized state
			store := newSnapshotStore(appCtx)
			snap, err := store.Load(context.Background())
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			state := snap.State
			if state == nil {
				state = &materialize.State{Issues: make(map[string]*materialize.Issue)}
			}
			if _, ok := state.Issues[nodeID]; !ok {
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
