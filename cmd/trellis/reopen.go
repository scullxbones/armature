package main

import (
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newReopenCmd() *cobra.Command {
	var repoPath, issueID string

	cmd := &cobra.Command{
		Use:   "reopen",
		Short: "Reopen a done or blocked issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			workerID, logPath, err := resolveWorkerAndLog(repoPath)
			if err != nil {
				return err
			}
			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{To: ops.StatusOpen},
			}
			return ops.AppendOp(logPath, op)
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path")
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to reopen")
	cmd.MarkFlagRequired("issue")
	return cmd
}
