package main

import (
	"context"
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/tui/app"
	"github.com/scullxbones/armature/internal/tui/dagtree"
	"github.com/scullxbones/armature/internal/tui/sources"
	"github.com/scullxbones/armature/internal/tui/tuivalidate"
	"github.com/scullxbones/armature/internal/tui/workers"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive kanban board with auto-refresh",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			stateDir := filepath.Join(appCtx.IssuesDir, "state", ".tui")

			workerID, _ := worker.GetWorkerID(appCtx.RepoPath) //nolint:errcheck // best-effort; missing worker ID falls back to empty
			if workerID == "" {
				workerID = "default"
			}

			if !tui.IsInteractive() {
				// Load snapshot to get materialized state
				store := newSnapshotStore(appCtx)
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

			m := app.New(issuesDir, stateDir, workerID).WithScreens(
				dagtree.New(),
				workers.New(),
				tuivalidate.New(),
				sources.New(),
			)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}
}
