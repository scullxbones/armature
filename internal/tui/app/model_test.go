package app_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui/app"
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

func TestInitIncludesInitialRefresh(t *testing.T) {
	m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil cmd")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() should return tea.BatchMsg, got %T", msg)
	}

	found := false
	for _, c := range batch {
		if c == nil {
			continue
		}
		ch := make(chan tea.Msg, 1)
		go func(cmd tea.Cmd) { ch <- cmd() }(c)
		select {
		case result := <-ch:
			if _, ok := result.(app.RefreshMsg); ok {
				found = true
			}
		case <-time.After(200 * time.Millisecond):
			// Blocking cmd (watcher setup or scheduler) — skip
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("Init() should include a command that sends RefreshMsg for initial state load")
	}
}

func TestLiveModeRefreshMsgRestartsListener(t *testing.T) {
	m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skip("fsnotify not available:", err)
	}
	defer func() { _ = w.Close() }()

	// Put model in live mode by sending WatcherReadyMsg
	updated, _ := m.Update(app.WatcherReadyMsg{Watcher: w})
	m = updated.(app.Model)

	// RefreshMsg in live mode must return a batch: doRefresh + restarted listener
	_, cmd := m.Update(app.RefreshMsg{})
	if cmd == nil {
		t.Fatal("Update(RefreshMsg{}) returned nil cmd in live mode")
	}
	msg := cmd()
	if _, ok := msg.(tea.BatchMsg); !ok {
		t.Errorf("RefreshMsg in live mode should return tea.BatchMsg (doRefresh + listener restart), got %T", msg)
	}
}
