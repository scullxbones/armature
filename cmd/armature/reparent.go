package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
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
			allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(appCtx.IssuesDir, "ops"))
			if err != nil {
				return fmt.Errorf("read ops: %w", err)
			}
			if _, err := materialize.Materialize(appCtx.StateDir, allOps, appCtx.Mode == "single-branch", offsets); err != nil {
				return err
			}

			// Look up the issue being reparented.
			issue, err := materialize.LoadIssue(filepath.Join(appCtx.StateDir, "issues", issueID+".json"))
			if err != nil {
				return fmt.Errorf("issue %s not found: %w", issueID, err)
			}

			if newParent != "" {
				// Look up the new parent and validate the hierarchy.
				parentIssue, err := materialize.LoadIssue(filepath.Join(appCtx.StateDir, "issues", newParent+".json"))
				if err != nil {
					return fmt.Errorf("parent %s not found: %w", newParent, err)
				}

				allowed, ok := validParentChildTypes[parentIssue.Type]
				if !ok || !allowed[issue.Type] {
					return fmt.Errorf("invalid parent: %s (%s) cannot contain %s", newParent, parentIssue.Type, issue.Type)
				}
			}
			// Suppress unused variable warning: issue is used above for hierarchy check.
			_ = issue

			workerID, logPath, err := resolveWorkerAndLog()
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

			if err := appendOp(logPath, op); err != nil {
				return err
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "new_parent": newParent, "status": "reparented"}
				data, _ := json.Marshal(result)
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
