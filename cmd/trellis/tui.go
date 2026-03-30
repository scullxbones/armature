package main

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui"
	"github.com/scullxbones/trellis/internal/tui/board"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive kanban board with auto-refresh",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			stateDir := filepath.Join(appCtx.IssuesDir, "state", ".tui")

			state, _, err := materialize.MaterializeAndReturn(issuesDir, stateDir, true)
			if err != nil {
				return err
			}

			var issues []*materialize.Issue
			for _, issue := range state.Issues {
				issues = append(issues, issue)
			}

			if !tui.IsInteractive() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "board: %d issues\n", len(issues))
				return nil
			}

			m := board.NewWithRefresh(issues, 0, 0, issuesDir, stateDir)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
}
