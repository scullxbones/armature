package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/issuetype"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/spf13/cobra"
)

func newReparentCmd() *cobra.Command {
	var issueID, newParent string

	cmd := &cobra.Command{
		Use:   "reparent",
		Short: "Move an issue to a new parent with hierarchy validation",
		Long: `Move an issue to a new parent node.

arm reparent validates that the new parent/child type combination is valid before
emitting the reparent op. Invalid combinations (e.g. task under task) are rejected
with an explicit error message.`,
		Example: `  # Move task-01 from its current parent to story-02
  $ arm reparent --issue task-01 --parent story-02

  # Remove a parent (make a top-level issue)
  $ arm reparent --issue task-01 --parent ""`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" {
				return fmt.Errorf("issue ID is required (--issue flag)")
			}

			appCtx := currentCtx(cmd)
			snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir, appCtx.Mode == "single-branch")
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			// Look up the issue being reparented.
			issue, ok := snap.Issues[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found", issueID)
			}

			if newParent != "" {
				// Look up the new parent and validate the hierarchy.
				parentIssue, ok := snap.Issues[newParent]
				if !ok {
					return fmt.Errorf("parent %s not found", newParent)
				}

				if !issuetype.IsLegalHierarchy(parentIssue.Type, issue.Type) {
					return fmt.Errorf("invalid parent: %s (%s) cannot contain %s", newParent, parentIssue.Type, issue.Type)
				}
			}

			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}

			op := ops.Op{
				Type:      ops.OpReparent,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					Parent: newParent,
				},
			}

			if err := appendOp(ctx, logPath, op); err != nil {
				return err
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "new_parent": newParent, "status": "reparented"}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				if newParent == "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reparented %s to root\n", issueID)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reparented %s to %s\n", issueID, newParent)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to reparent")
	cmd.Flags().StringVar(&newParent, "parent", "", "new parent issue ID (empty string makes issue top-level)")
	_ = cmd.MarkFlagRequired("issue")
	_ = cmd.MarkFlagRequired("parent")

	return cmd
}
