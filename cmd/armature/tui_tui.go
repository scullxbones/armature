package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/tui/app"
	"github.com/scullxbones/armature/internal/tui/dagtree"
	"github.com/scullxbones/armature/internal/tui/sources"
	"github.com/scullxbones/armature/internal/tui/tuivalidate"
	"github.com/scullxbones/armature/internal/tui/workers"
)

// runBoardTUI is the interactive boundary for `arm tui`: wire the kanban
// app and screens, then run the alt-screen program. The non-interactive
// board summary stays in tui.go.
func runBoardTUI(issuesDir, stateDir, workerID string) error {
	m := app.New(issuesDir, stateDir, workerID).WithScreens(
		dagtree.New(),
		workers.New(),
		tuivalidate.New(),
		sources.New(),
	)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
