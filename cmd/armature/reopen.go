package main

import (
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
			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{To: ops.StatusOpen},
			}
			return appendOp(ctx, logPath, op)
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to reopen")
	return cmd
}
