package workers

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
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

func makeWorkersState(workerIDs ...string) *materialize.State {
	issues := make(map[string]*materialize.Issue)
	for i, id := range workerIDs {
		issueID := fmt.Sprintf("T%d", i+1)
		issues[issueID] = &materialize.Issue{ID: issueID, Title: "Task", ClaimedBy: id}
	}
	return &materialize.State{Issues: issues}
}

func TestWorkersViewClipsToHeight(t *testing.T) {
	m := New()
	m.SetSize(120, 2)
	// Each worker renders at least 2 lines (worker row + issue row), 4 workers → 8+ lines
	m.SetState(makeWorkersState("worker-a", "worker-b", "worker-c", "worker-d"))
	v := m.View()
	lines := strings.Split(strings.TrimRight(v, "\n"), "\n")
	if len(lines) > 2 {
		t.Errorf("expected at most 2 lines for height=2, got %d:\n%s", len(lines), v)
	}
}

func TestWorkersViewScrollsToKeepCursorVisible(t *testing.T) {
	m := New()
	m.SetSize(120, 3)
	// Create 5 workers, each with one issue: each worker row = 1 line (worker) + 1 line (issue) + 1 blank = 3 lines
	m.SetState(makeWorkersState("worker-a", "worker-b", "worker-c", "worker-d", "worker-e"))
	// Move cursor down past height
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	v := m.View()
	if strings.Contains(v, "worker-a") {
		t.Errorf("worker-a should have scrolled off when cursor moves down, got:\n%s", v)
	}
}
