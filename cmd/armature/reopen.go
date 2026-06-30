package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newReopenCmd() *cobra.Command {
	var issueID string

	cmd := &cobra.Command{
		Use:   "reopen [issue-id]",
		Short: "Reopen a done or blocked issue",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

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
	return cmd
}
