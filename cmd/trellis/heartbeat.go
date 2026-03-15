package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newHeartbeatCmd() *cobra.Command {
	var repoPath, issueID string

	cmd := &cobra.Command{
		Use:   "heartbeat",
		Short: "Send heartbeat for an active claim",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpHeartbeat, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID}
			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"issue": issueID, "heartbeat": "sent"}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.MarkFlagRequired("issue")
	return cmd
}
