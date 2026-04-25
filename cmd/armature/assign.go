package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

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
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

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
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "assigned_to": workerID}
				data, _ := json.Marshal(result)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assigned %s to %s\n", issueID, workerID)
			}
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

			// Check current status before unassigning so we can release claimed → open.
			issuesDir := appCtx.IssuesDir
			if _, matErr := materialize.Materialize(issuesDir, appCtx.StateDir, appCtx.Mode == "single-branch"); matErr != nil {
				return matErr
			}
			index, _ := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
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

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "assigned_to": ""}
				data, _ := json.Marshal(result)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Unassigned %s\n", issueID)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to unassign")
	return cmd
}
