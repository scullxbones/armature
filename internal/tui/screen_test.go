package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
)

type mockScreen struct{}

func (m mockScreen) Init() tea.Cmd                        { return nil }
func (m mockScreen) Update(msg tea.Msg) (Screen, tea.Cmd) { return m, nil }
func (m mockScreen) View() string                         { return "" }
func (m mockScreen) HelpBar() string                      { return "" }
func (m mockScreen) SetSize(width, height int)            {}
func (m mockScreen) SetState(state *materialize.State)    {}

func TestScreenInterface(t *testing.T) {
	var _ Screen = (*mockScreen)(nil)
}
