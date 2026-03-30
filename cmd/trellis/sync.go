package main

import (
	"fmt"

	"github.com/scullxbones/trellis/internal/git"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	trellissync "github.com/scullxbones/trellis/internal/sync"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var targetBranch string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Detect merged branches and auto-transition done issues to merged",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			// Materialize to ensure state files are up to date
			if _, err := materialize.Materialize(issuesDir, appCtx.StateDir, singleBranch); err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			if targetBranch == "" {
				gc := git.New(appCtx.RepoPath)
				branch, err := gc.CurrentBranch()
				if err != nil {
					return fmt.Errorf("detect current branch: %w", err)
				}
				targetBranch = branch
			}

			gc := git.New(appCtx.RepoPath)
			mergedIDs, err := trellissync.DetectMerges(appCtx.StateDir, targetBranch, gc)
			if err != nil {
				return fmt.Errorf("detect merges: %w", err)
			}

			if len(mergedIDs) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No merged branches detected.")
				return nil
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			for _, id := range mergedIDs {
				op := ops.Op{
					Type:      ops.OpTransition,
					TargetID:  id,
					WorkerID:  workerID,
					Timestamp: nowEpoch(),
					Payload: ops.Payload{
						To:      ops.StatusMerged,
						Outcome: "auto-detected merge into " + targetBranch,
					},
				}
				if err := appendOp(logPath, op); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to transition %s: %v\n", id, err)
					continue
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Transitioned %s to merged\n", id)
			}

			// Re-materialize so state files reflect the new merged status
			if _, err := materialize.Materialize(issuesDir, appCtx.StateDir, singleBranch); err != nil {
				return fmt.Errorf("re-materialize: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&targetBranch, "into", "", "target branch to check merges against (default: current branch)")
	return cmd
}
