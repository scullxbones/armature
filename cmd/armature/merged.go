package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newMergedCmd() *cobra.Command {
	var issueID, pr string

	cmd := &cobra.Command{
		Use:   "merged",
		Short: "Mark a done issue as merged after its branch/PR is merged",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			// Materialize to get current state
			if _, err := materialize.Materialize(issuesDir, appCtx.StateDir, singleBranch); err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			index, err := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
			if err != nil {
				return fmt.Errorf("load index: %w", err)
			}

			entry, ok := index[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found", issueID)
			}

			// In dual-branch mode, require current status to be "done"
			if !singleBranch && entry.Status != ops.StatusDone {
				return fmt.Errorf("issue %s is in status %q; arm merged requires status=done (transition it to done first)", issueID, entry.Status)
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			op := ops.Op{
				Type:      ops.OpTransition,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   ops.Payload{To: ops.StatusMerged, PR: pr},
			}
			if err := appendOp(logPath, op); err != nil {
				return err
			}

			if singleBranch {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note: in single-branch mode, done→merged is automatic. Op recorded for %s.\n", issueID)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Marked %s as merged", issueID)
				if pr != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), " (PR #%s)", pr)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&pr, "pr", "", "PR number or URL")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
