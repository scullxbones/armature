package dagtree

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
)

func TestDetailIsOpen(t *testing.T) {
	t.Parallel()
	var m issueDetail
	if m.isOpen() {
		t.Error("new model should not be open")
	}
	issue := &materialize.Issue{ID: "T1", Title: "Task"}
	m = m.openIssue(issue)
	if !m.isOpen() {
		t.Error("model should be open after openIssue()")
	}
	m = m.close()
	if m.isOpen() {
		t.Error("model should be closed after close()")
	}
}

func TestDetailUpdateCloseOnEsc(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{ID: "T1"}
	m := issueDetail{}.setSize(80, 24).openIssue(issue)
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.isOpen() {
		t.Error("esc should close the overlay")
	}
}

func TestDetailUpdateCloseOnQ(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{ID: "T1"}
	m := issueDetail{}.setSize(80, 24).openIssue(issue)
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.isOpen() {
		t.Error("q should close the overlay")
	}
}

func TestDetailUpdateScrolling(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{ID: "T1", Title: strings.Repeat("line\n", 20)}
	m := issueDetail{}.setSize(80, 24).openIssue(issue)
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, _ = m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
}

func TestDetailUpdateCopiesIssueID(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{ID: "T1"}
	m := issueDetail{}.setSize(80, 24).openIssue(issue)
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if !m.isOpen() {
		t.Error("c should not close the overlay")
	}
}

func TestDetailSetSizeWhenOpen(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{ID: "T1"}
	m := issueDetail{}.openIssue(issue)
	_ = m.setSize(100, 30)
}

func TestDetailBuildContentWithDefinitionOfDone(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:               "T1",
		Title:            "Task",
		DefinitionOfDone: "All tests pass",
	}
	m := issueDetail{}.setSize(80, 24).openIssue(issue)
	v := m.view()
	if !strings.Contains(v, "All tests pass") {
		t.Errorf("expected DefinitionOfDone in view, got: %s", v)
	}
}

func TestDetailBuildContentWithOutcome(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:      "T1",
		Title:   "Task",
		Outcome: "Delivered successfully",
	}
	m := issueDetail{}.setSize(80, 24).openIssue(issue)
	v := m.view()
	if !strings.Contains(v, "Delivered successfully") {
		t.Errorf("expected Outcome in view, got: %s", v)
	}
}

func TestDetailUpdateWhenClosed(t *testing.T) {
	t.Parallel()
	var m issueDetail
	m2, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m2.isOpen() {
		t.Error("closed model should stay closed")
	}
	if cmd != nil {
		t.Error("closed model update should return nil cmd")
	}
}

func TestDetailViewRendersTitle(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:    "TASK-14",
		Title: "Build detail overlay model",
	}
	m := issueDetail{}.setSize(80, 24).openIssue(issue)
	view := m.view()
	if !strings.Contains(view, "TASK-14") {
		t.Errorf("expected TASK-14, got:\n%s", view)
	}
	if !strings.Contains(view, "Build detail overlay model") {
		t.Errorf("expected title, got:\n%s", view)
	}
}

func TestDetailViewHidden(t *testing.T) {
	t.Parallel()
	m := issueDetail{}.openIssue(nil)
	if m.view() != "" {
		t.Errorf("expected empty view for nil issue")
	}

	issue := &materialize.Issue{ID: "TASK-14"}
	m2 := issueDetail{}.openIssue(issue).close()
	if m2.view() != "" {
		t.Errorf("expected empty view when closed")
	}
}
