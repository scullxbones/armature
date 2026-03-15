package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newDecisionCmd() *cobra.Command {
	var issueID, topic, choice, rationale string
	var affects []string

	cmd := &cobra.Command{
		Use:   "decision",
		Short: "Record an architectural decision",
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpDecision, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Topic: topic, Choice: choice,
					Rationale: rationale, Affects: affects}}
			if err := ops.AppendOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"issue": issueID, "topic": topic, "choice": choice}
			data, _ := json.Marshal(result)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&topic, "topic", "", "decision topic")
	cmd.Flags().StringVar(&choice, "choice", "", "chosen option")
	cmd.Flags().StringVar(&rationale, "rationale", "", "why this choice")
	cmd.Flags().StringSliceVar(&affects, "affects", nil, "affected scope globs")
	cmd.MarkFlagRequired("issue")
	cmd.MarkFlagRequired("topic")
	cmd.MarkFlagRequired("choice")
	return cmd
}
