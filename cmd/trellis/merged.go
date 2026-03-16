package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newMergedCmd() *cobra.Command {
	var issueID string

	cmd := &cobra.Command{
		Use:   "merged",
		Short: "Mark an issue as merged (no-op in single-branch mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}
			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{To: ops.StatusMerged},
			}
			if err := appendOp(logPath, op); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Note: in single-branch mode, done→merged is automatic. Op recorded for compatibility.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.MarkFlagRequired("issue")
	return cmd
}
