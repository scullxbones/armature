package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newDAGTransitionCmd() *cobra.Command {
	var issueID string
	var to string

	cmd := &cobra.Command{
		Use:   "dag-transition",
		Short: "Promote all draft nodes in a subtree to verified",
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			targetConfidence := to
			if targetConfidence == "" {
				targetConfidence = "verified"
			}

			op := ops.Op{
				Type:      ops.OpDAGTransition,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					IssueID: issueID,
					To:      targetConfidence,
				},
			}
			if err := appendOp(logPath, op); err != nil {
				return err
			}

			result := map[string]string{"issue": issueID, "promoted_to": targetConfidence}
			data, _ := json.Marshal(result)                      //nolint:errcheck // result struct contains only serializable values
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data)) //nolint:errcheck // stdout write not actionable in CLI
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "root issue ID of the subtree to promote")
	cmd.Flags().StringVar(&to, "to", "", "target confidence level (default: verified)")
	_ = cmd.MarkFlagRequired("issue") //nolint:errcheck // fails only if flag absent (programming error)
	return cmd
}
