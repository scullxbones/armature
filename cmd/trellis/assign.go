package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newAssignCmd() *cobra.Command {
	var issueID, workerID string

	cmd := &cobra.Command{
		Use:   "assign",
		Short: "Assign an issue to a worker",
		RunE: func(cmd *cobra.Command, args []string) error {
			myWorkerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}
			op := ops.Op{
				Type:      ops.OpAssign,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  myWorkerID,
				Payload:   ops.Payload{AssignedTo: workerID},
			}
			if err := appendHighStakesOp(logPath, op); err != nil {
				return err
			}
			result := map[string]string{"issue": issueID, "assigned_to": workerID}
			data, _ := json.Marshal(result)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to assign")
	cmd.Flags().StringVar(&workerID, "worker", "", "worker ID to assign to")
	_ = cmd.MarkFlagRequired("issue")
	_ = cmd.MarkFlagRequired("worker")
	return cmd
}

func newUnassignCmd() *cobra.Command {
	var issueID string

	cmd := &cobra.Command{
		Use:   "unassign",
		Short: "Remove worker assignment from an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			// Check current status before unassigning so we can release claimed → open.
			issuesDir := appCtx.IssuesDir
			if _, matErr := materialize.Materialize(issuesDir, appCtx.Mode == "single-branch"); matErr != nil {
				return matErr
			}
			index, _ := materialize.LoadIndex(issuesDir + "/state/index.json")
			currentStatus := ""
			if entry, ok := index[issueID]; ok {
				currentStatus = entry.Status
			}

			op := ops.Op{
				Type:      ops.OpAssign,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   ops.Payload{AssignedTo: ""},
			}
			if err := appendHighStakesOp(logPath, op); err != nil {
				return err
			}

			// If the issue was claimed, release it back to open.
			if currentStatus == ops.StatusClaimed {
				transitionOp := ops.Op{
					Type:      ops.OpTransition,
					TargetID:  issueID,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload:   ops.Payload{To: ops.StatusOpen},
				}
				appendOp(logPath, transitionOp) //nolint:errcheck
			}

			result := map[string]string{"issue": issueID, "assigned_to": ""}
			data, _ := json.Marshal(result)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to unassign")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
