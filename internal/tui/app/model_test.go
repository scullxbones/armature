package app_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/app"
)

func TestInitialScreenIsDAGTree(t *testing.T) {
	m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
	if m.CurrentScreen() != app.ScreenDAGTree {
		t.Errorf("initial screen = %v, want ScreenDAGTree", m.CurrentScreen())
	}
}

func TestScreenSwitchByNumber(t *testing.T) {
	m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
	for key, want := range map[string]app.ScreenID{
		"1": app.ScreenDAGTree,
		"2": app.ScreenWorkers,
		"3": app.ScreenValidate,
		"4": app.ScreenSources,
	} {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		got := updated.(app.Model).CurrentScreen()
		if got != want {
			t.Errorf("key %q: screen = %v, want %v", key, got, want)
		}
	}
}

func TestSetStatePropagates(t *testing.T) {
	m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
	state := &materialize.State{Issues: map[string]*materialize.Issue{
		"T1": {ID: "T1", Status: "open"},
	}}
	m = m.WithState(state)
	// Nav bar should render without panic.
	v := m.View()
	if !strings.Contains(v, "[1]") {
		t.Errorf("nav bar missing screen tab, got: %q", v)
	}
}

func TestNavBarShowsValidateBadgeWhenErrors(t *testing.T) {
	m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
	state := &materialize.State{Issues: map[string]*materialize.Issue{
		"T1": {ID: "T1", Status: "open"},
	}}
	m = m.WithState(state).WithValidateErrors(3)
	nav := m.NavBar()
	if !strings.Contains(nav, "⚠3") {
		t.Errorf("nav bar missing error badge, got: %q", nav)
	}
}

func TestQuitKey(t *testing.T) {
	m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Error("q should return a quit command")
	}
}
