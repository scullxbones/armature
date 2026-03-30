package detail_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/detail"
)

func TestDetailIsOpen(t *testing.T) {
	m := detail.New()
	if m.IsOpen() {
		t.Error("new model should not be open")
	}
	issue := &materialize.Issue{ID: "T1", Title: "Task"}
	m = m.Open(issue)
	if !m.IsOpen() {
		t.Error("model should be open after Open()")
	}
	m = m.Close()
	if m.IsOpen() {
		t.Error("model should be closed after Close()")
	}
}

func TestDetailUpdateCloseOnEsc(t *testing.T) {
	issue := &materialize.Issue{ID: "T1"}
	m := detail.New()
	m = m.SetSize(80, 24)
	m = m.Open(issue)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.IsOpen() {
		t.Error("esc should close the overlay")
	}
}

func TestDetailUpdateCloseOnQ(t *testing.T) {
	issue := &materialize.Issue{ID: "T1"}
	m := detail.New()
	m = m.SetSize(80, 24)
	m = m.Open(issue)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.IsOpen() {
		t.Error("q should close the overlay")
	}
}

func TestDetailUpdateScrolling(t *testing.T) {
	issue := &materialize.Issue{ID: "T1", Title: strings.Repeat("line\n", 20)}
	m := detail.New()
	m = m.SetSize(80, 24)
	m = m.Open(issue)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
}

func TestDetailSetSizeWhenOpen(t *testing.T) {
	issue := &materialize.Issue{ID: "T1"}
	m := detail.New()
	m = m.Open(issue)
	_ = m.SetSize(100, 30) // must not panic when open
}

func TestDetailBuildContentWithDefinitionOfDone(t *testing.T) {
	issue := &materialize.Issue{
		ID:               "T1",
		Title:            "Task",
		DefinitionOfDone: "All tests pass",
	}
	m := detail.New()
	m = m.SetSize(80, 24)
	m = m.Open(issue)
	v := m.View()
	if !strings.Contains(v, "All tests pass") {
		t.Errorf("expected DefinitionOfDone in view, got: %s", v)
	}
}

func TestDetailBuildContentWithOutcome(t *testing.T) {
	issue := &materialize.Issue{
		ID:      "T1",
		Title:   "Task",
		Outcome: "Delivered successfully",
	}
	m := detail.New()
	m = m.SetSize(80, 24)
	m = m.Open(issue)
	v := m.View()
	if !strings.Contains(v, "Delivered successfully") {
		t.Errorf("expected Outcome in view, got: %s", v)
	}
}

func TestDetailUpdateWhenClosed(t *testing.T) {
	m := detail.New()
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m2.IsOpen() {
		t.Error("closed model should stay closed")
	}
	if cmd != nil {
		t.Error("closed model update should return nil cmd")
	}
}

func TestDetailViewRendersTitle(t *testing.T) {
	issue := &materialize.Issue{
		ID:    "TASK-14",
		Title: "Build detail overlay model",
	}
	m := detail.New()
	m = m.SetSize(80, 24)
	m = m.Open(issue)
	view := m.View()
	if !strings.Contains(view, "TASK-14") {
		t.Errorf("expected TASK-14, got:\n%s", view)
	}
	if !strings.Contains(view, "Build detail overlay model") {
		t.Errorf("expected title, got:\n%s", view)
	}
}

func TestDetailViewHidden(t *testing.T) {
	// Case 1: Issue is nil
	m := detail.New()
	m = m.Open(nil)
	if m.View() != "" {
		t.Errorf("expected empty view for nil issue")
	}

	// Case 2: Closed
	issue := &materialize.Issue{ID: "TASK-14"}
	m2 := detail.New()
	m2 = m2.Open(issue)
	m2 = m2.Close()
	if m2.View() != "" {
		t.Errorf("expected empty view when closed")
	}
}
