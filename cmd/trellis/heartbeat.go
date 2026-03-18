package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newHeartbeatCmd() *cobra.Command {
	var issueID string

	cmd := &cobra.Command{
		Use:   "heartbeat",
		Short: "Send heartbeat for an active claim",
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpHeartbeat, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID}
			if err := appendLowStakesOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"issue": issueID, "heartbeat": "sent"}
			data, _ := json.Marshal(result)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
