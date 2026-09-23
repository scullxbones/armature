package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/tui"
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

			workerID := slottedWorkerIDBestEffort(appCtx.RepoPath)
			if workerID == "" {
				workerID = slottedWorkerID("default").String()
			}

			if !tui.IsInteractive() {
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
