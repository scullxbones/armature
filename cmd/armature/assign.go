package main

import (
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newAssignCmd() *cobra.Command {
	var issueID, workerID string

	cmd := &cobra.Command{
		Use:   "assign [issue-id]",
		Short: "Assign an issue to a worker",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)
			var err error
			issueID, err = resolveIssueID(issueID, args)
			if err != nil {
				return err
			}

			myWorkerID, logPath, err := resolveWorkerAndLog(ctx)
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
			if err := appendHighStakesOp(mustState(cmd), logPath, op); err != nil {
				return err
			}
			writeCommandResult(cmd, map[string]string{"issue": issueID, "assigned_to": workerID},
				"Assigned %s to %s\n", issueID, workerID)
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to assign")
	cmd.Flags().StringVar(&workerID, "worker", "", "worker ID to assign to")
	_ = cmd.MarkFlagRequired("worker")
	return cmd
}

func newUnassignCmd() *cobra.Command {
	var issueID string

	cmd := &cobra.Command{
		Use:   "unassign [issue-id]",
		Short: "Remove worker assignment from an issue",
		Long: `Unassign an issue to release its worker assignment.

If the issue was claimed, it will automatically transition back to open status.
This allows the issue to be claimed again by another worker.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)
			var err error
			issueID, err = resolveIssueID(issueID, args)
			if err != nil {
				return err
			}

			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}

			// Check current status before unassigning so we can release claimed → open.
			// Use ReadIndex to avoid premature rematerialization; gracefully degrade to
			// an empty index if the index file is missing.
			store := newSnapshotStore(ctx)
			index, _ := store.ReadIndex() //nolint:errcheck // missing index treated as empty; access uses ok-check
			if index == nil {
				index = make(materialize.Index)
			}
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
			if err := appendHighStakesOp(mustState(cmd), logPath, op); err != nil {
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
				appendOp(ctx, logPath, transitionOp) //nolint:errcheck,gosec
			}

			writeCommandResult(cmd, map[string]string{"issue": issueID, "assigned_to": ""},
				"Unassigned %s\n", issueID)
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to unassign")
	return cmd
}
