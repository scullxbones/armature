package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive kanban board with auto-refresh",
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			issuesDir := appCtx.IssuesDir
			stateDir := filepath.Join(appCtx.IssuesDir, "state", ".tui")

			workerID, _ := worker.GetWorkerID(appCtx.RepoPath) //nolint:errcheck // best-effort; missing worker ID falls back to empty
			if workerID == "" {
				workerID = "default"
			}

			if !tui.IsInteractive() {
				// Non-interactive path uses a scratch dir to avoid writing checkpoint/issues/index
				// into the canonical StateDir. The interactive path (runBoardTUI) uses stateDir
				// which already points to the .tui isolation dir.
				tuiOpsDir := filepath.Join(appCtx.IssuesDir, "ops")
				store := snapshot.NewStore(tuiOpsDir, stateDir)
				snap, err := store.Load(context.Background())
				if err != nil {
					return fmt.Errorf("load snapshot: %w", err)
				}
				state := snap.State
				if state == nil {
					state = &materialize.State{Issues: make(map[string]*materialize.Issue)}
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "board: %d issues\n", len(state.Issues))
				return nil
			}

			return runBoardTUI(issuesDir, stateDir, workerID)
		},
	}
}
