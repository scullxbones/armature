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
			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			targetConfidence := to
			if targetConfidence == "" {
				targetConfidence = "verified"
			}
			if targetConfidence != "draft" && targetConfidence != "verified" {
				return fmt.Errorf("invalid --to confidence value %q: must be one of draft, verified", targetConfidence)
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
			if err := appendOp(ctx, logPath, op); err != nil {
				return err
			}

			result := map[string]string{"issue": issueID, "promoted_to": targetConfidence}
			data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "root issue ID of the subtree to promote")
	cmd.Flags().StringVar(&to, "to", "", "target confidence level: draft or verified (default: verified)")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
