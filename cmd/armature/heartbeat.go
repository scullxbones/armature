package main

import (
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newHeartbeatCmd() *cobra.Command {
	var issueID string

	cmd := &cobra.Command{
		Use:   "heartbeat [issue-id]",
		Short: "Send heartbeat for an active claim",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			issueID, err = resolveIssueID(issueID, args)
			if err != nil {
				return err
			}

			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpHeartbeat, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID}
			if err := appendLowStakesOp(state, logPath, op); err != nil {
				return err
			}
			writeCommandResult(cmd, map[string]string{"issue": issueID, "heartbeat": "sent"},
				"Heartbeat recorded for %s\n", issueID)
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	return cmd
}
