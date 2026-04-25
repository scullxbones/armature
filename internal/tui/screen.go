package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
)

// Screen is implemented by every TUI sub-screen (dagtree, workers, validate, sources).
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View() string
	HelpBar() string
	SetSize(width, height int)
	SetState(state *materialize.State)
}
