package main

import (
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newReopenCmd() *cobra.Command {
	var issueID string

	cmd := &cobra.Command{
		Use:   "reopen",
		Short: "Reopen a done or blocked issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}
			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{To: ops.StatusOpen},
			}
			return appendOp(logPath, op)
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to reopen")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
