package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newDecisionCmd() *cobra.Command {
	var issueID, topic, choice, rationale string
	var affects []string

	cmd := &cobra.Command{
		Use:   "decision [issue-id]",
		Short: "Record an architectural decision",
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
			op := ops.Op{Type: ops.OpDecision, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Topic: topic, Choice: choice,
					Rationale: rationale, Affects: affects}}
			if err := appendLowStakesOp(logPath, op); err != nil {
				return err
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "topic": topic, "choice": choice}
				data, _ := json.Marshal(result)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Decision recorded on %s: %s → %s\n", issueID, topic, choice)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&topic, "topic", "", "decision topic")
	cmd.Flags().StringVar(&choice, "choice", "", "chosen option")
	cmd.Flags().StringVar(&rationale, "rationale", "", "why this choice")
	cmd.Flags().StringSliceVar(&affects, "affects", nil, "affected scope globs")
	_ = cmd.MarkFlagRequired("topic")
	_ = cmd.MarkFlagRequired("choice")
	return cmd
}
