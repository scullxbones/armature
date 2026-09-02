package main

import (
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
			op := ops.Op{Type: ops.OpDecision, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID, Payload: ops.Payload{Topic: topic, Choice: choice,
					Rationale: rationale, Affects: affects}}
			if err := appendLowStakesOp(state, logPath, op); err != nil {
				return err
			}
			writeCommandResult(cmd, map[string]string{"issue": issueID, "topic": topic, "choice": choice},
				"Decision recorded on %s: %s → %s\n", issueID, topic, choice)
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
