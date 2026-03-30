package workers

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
)

func TestWorkersInit(t *testing.T) {
	m := New()
	if cmd := m.Init(); cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestWorkersSetSize(t *testing.T) {
	m := New()
	m.SetSize(80, 24) // must not panic
}

func TestWorkersNilStateView(t *testing.T) {
	m := New()
	v := m.View()
	if v != "No state available." {
		t.Errorf("expected nil-state message, got: %s", v)
	}
}

func TestWorkersNoWorkersView(t *testing.T) {
	m := New()
	m.SetState(&materialize.State{Issues: map[string]*materialize.Issue{}})
	v := m.View()
	if v != "No active workers." {
		t.Errorf("expected no-workers message, got: %s", v)
	}
}

func TestWorkersSetStateNil(t *testing.T) {
	m := New()
	m.SetState(nil) // must not panic
}

func TestWorkersCursorMovement(t *testing.T) {
	m := New()
	m.SetState(&materialize.State{Issues: map[string]*materialize.Issue{
		"T1": {ID: "T1", ClaimedBy: "worker-a"},
		"T2": {ID: "T2", ClaimedBy: "worker-b"},
	}})

	// cursor starts at 0; move down
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	// move back up
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	// boundary: cannot go below 0
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	// boundary: cannot go above len(workers)-1
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
}

func TestWorkersViewContainsWorkerID(t *testing.T) {
	m := New()
	state := &materialize.State{
		Issues: map[string]*materialize.Issue{
			"TASK-1": {
				ID:        "TASK-1",
				Title:     "Task 1",
				ClaimedBy: "worker-1",
			},
		},
	}
	m.SetState(state)

	view := m.View()
	if !strings.Contains(view, "worker-1") {
		t.Errorf("expected view to contain worker-1, got:\n%s", view)
	}
	if !strings.Contains(view, "TASK-1") {
		t.Errorf("expected view to contain TASK-1, got:\n%s", view)
	}
}

func TestWorkersHelpBar(t *testing.T) {
	m := New()
	help := m.HelpBar()
	if !strings.Contains(help, "j/k move") {
		t.Errorf("expected help bar to contain j/k move, got: %s", help)
	}
}
