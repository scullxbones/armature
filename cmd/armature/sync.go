package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/snapshot"
	armsync "github.com/scullxbones/armature/internal/sync"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var targetBranch string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Detect merged branches and auto-transition done issues to merged",
		Long: `Scan merged branches and automatically transition completed issues to merged status.

This command detects which feature branches have been merged (based on branch naming
conventions or metadata) and transitions their corresponding issues from "done" to "merged".
This keeps the issue graph synchronized with the actual git history. Use --dry-run to
preview changes without committing them.`,
		Example: `  # Detect merges on the current branch and sync issue statuses
  $ arm sync

  # Check for merges into a specific target branch
  $ arm sync --into main

  # Preview which issues would be transitioned without making changes
  $ arm sync --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			// Load snapshot to ensure state is up to date
			snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), appCtx.StateDir)
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			if targetBranch == "" {
				gc := adapters.New(appCtx.RepoPath)
				branch, err := gc.CurrentBranch()
				if err != nil {
					return fmt.Errorf("detect current branch: %w", err)
				}
				targetBranch = branch
			}

			// Convert snapshot issues to slice for DetectMerges
			issues := make([]materialize.Issue, 0, len(snap.Issues))
			for _, issue := range snap.Issues {
				issues = append(issues, *issue)
			}

			gc := adapters.New(appCtx.RepoPath)
			mergedIDs, err := armsync.DetectMerges(issues, targetBranch, gc)
			if err != nil {
				return fmt.Errorf("detect merges: %w", err)
			}

			if len(mergedIDs) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No merged branches detected.")
				return nil
			}

			if dryRun {
				for _, id := range mergedIDs {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "would transition: %s -> merged\n", id)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: %d issue(s) would be transitioned to merged\n", len(mergedIDs))
				return nil
			}

			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
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
				if err := appendOp(ctx, logPath, op); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to transition %s: %v\n", id, err)
					continue
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Transitioned %s to merged\n", id)
			}

			// Re-load snapshot so state files reflect the new merged status
			snap, err = snapshot.Load(filepath.Join(issuesDir, "ops"), appCtx.StateDir)
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			for _, w := range snap.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&targetBranch, "into", "", "target branch to check merges against (default: current branch)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print which issues would be transitioned without writing ops")
	return cmd
}
