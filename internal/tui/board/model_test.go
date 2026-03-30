package board

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
)

func makeIssues() []*materialize.Issue {
	return []*materialize.Issue{
		{ID: "OPEN-1", Title: "Open issue 1", Status: "open", Priority: "high", Type: "task"},
		{ID: "OPEN-2", Title: "Open issue 2", Status: "open", Priority: "low", Type: "task"},
		{ID: "ACTIVE-1", Title: "Active issue 1", Status: "in-progress", Priority: "high", Type: "story"},
		{ID: "DONE-1", Title: "Done issue 1", Status: "done", Priority: "medium", Type: "task"},
		{ID: "DONE-2", Title: "Done issue 2", Status: "done", Priority: "low", Type: "bug"},
	}
}

func TestNew(t *testing.T) {
	issues := makeIssues()
	m := New(issues, 120, 40)

	if len(m.columns[ColOpen]) != 2 {
		t.Errorf("expected 2 open issues, got %d", len(m.columns[ColOpen]))
	}
	if len(m.columns[ColActive]) != 1 {
		t.Errorf("expected 1 active issue, got %d", len(m.columns[ColActive]))
	}
	if len(m.columns[ColDone]) != 2 {
		t.Errorf("expected 2 done issues, got %d", len(m.columns[ColDone]))
	}
	if m.ActiveCol() != 0 {
		t.Errorf("expected initial activeCol=0, got %d", m.ActiveCol())
	}
	if m.Cursor() != 0 {
		t.Errorf("expected initial cursor=0, got %d", m.Cursor())
	}
	if m.ShowDetail() {
		t.Error("expected showDetail=false initially")
	}
}

func TestNavRight(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	// l key moves right
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	result := updated.(Model)
	if result.ActiveCol() != 1 {
		t.Errorf("expected activeCol=1 after l, got %d", result.ActiveCol())
	}
}

func TestNavRightArrow(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	result := updated.(Model)
	if result.ActiveCol() != 1 {
		t.Errorf("expected activeCol=1 after right arrow, got %d", result.ActiveCol())
	}
}

func TestNavLeft(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	// Move to col 1 first
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(Model)
	// Now move left
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	result := updated.(Model)
	if result.ActiveCol() != 0 {
		t.Errorf("expected activeCol=0 after h, got %d", result.ActiveCol())
	}
}

func TestNavLeftClamped(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	// Already at col 0, h should clamp
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	result := updated.(Model)
	if result.ActiveCol() != 0 {
		t.Errorf("expected activeCol=0 (clamped), got %d", result.ActiveCol())
	}
}

func TestNavRightClamped(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	// Move to last column
	m.activeCol = 2
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	result := updated.(Model)
	if result.ActiveCol() != 2 {
		t.Errorf("expected activeCol=2 (clamped), got %d", result.ActiveCol())
	}
}

func TestNavDown(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	// j key moves cursor down in active column
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	result := updated.(Model)
	if result.Cursor() != 1 {
		t.Errorf("expected cursor=1 after j, got %d", result.Cursor())
	}
}

func TestNavUp(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	// Move down first
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	// Now move up
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	result := updated.(Model)
	if result.Cursor() != 0 {
		t.Errorf("expected cursor=0 after k, got %d", result.Cursor())
	}
}

func TestNavUpClamped(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	// Already at top, k should clamp at 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	result := updated.(Model)
	if result.Cursor() != 0 {
		t.Errorf("expected cursor=0 (clamped), got %d", result.Cursor())
	}
}

func TestNavDownClamped(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	// Move to bottom of open column (2 items, so max index 1)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	result := updated.(Model)
	if result.Cursor() != 1 {
		t.Errorf("expected cursor=1 (clamped at last), got %d", result.Cursor())
	}
}

func TestEnterOpensDetail(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(Model)
	if !result.ShowDetail() {
		t.Error("expected showDetail=true after enter")
	}
}

func TestNavRightClosesDetail(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	// Open detail
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !m.ShowDetail() {
		t.Fatal("expected showDetail=true")
	}
	// Navigate right should close detail
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	result := updated.(Model)
	if result.ShowDetail() {
		t.Error("expected showDetail=false after column nav")
	}
}

func TestNavLeftClosesDetail(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	m.activeCol = 1
	// Open detail
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	// Navigate left should close detail
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	result := updated.(Model)
	if result.ShowDetail() {
		t.Error("expected showDetail=false after left column nav")
	}
}

func TestQuit(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Error("expected non-nil cmd for quit")
	}
	// Verify it's a quit command
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestQuitCtrlC(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("expected non-nil cmd for ctrl+c")
	}
}

func TestEmptyColumn(t *testing.T) {
	// Only open issues, active and done are empty
	issues := []*materialize.Issue{
		{ID: "OPEN-1", Title: "Open issue 1", Status: "open"},
	}
	m := New(issues, 120, 40)

	// Navigate to active (empty) column
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(Model)
	if m.ActiveCol() != 1 {
		t.Errorf("expected activeCol=1, got %d", m.ActiveCol())
	}

	// j should not panic on empty column
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	result := updated.(Model)
	if result.Cursor() != 0 {
		t.Errorf("expected cursor=0 in empty column, got %d", result.Cursor())
	}

	// enter should not panic on empty column and should not open detail
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result = updated.(Model)
	if result.ShowDetail() {
		t.Error("expected showDetail=false when entering on empty column")
	}
}

func TestInit(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	cmd := m.Init()
	if cmd != nil {
		t.Error("expected nil cmd from Init")
	}
}

func TestView(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestViewWithDetail(t *testing.T) {
	m := New(makeIssues(), 120, 40)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view with detail")
	}
}

func TestNewEmptyIssues(t *testing.T) {
	m := New([]*materialize.Issue{}, 120, 40)
	if m.ActiveCol() != 0 {
		t.Errorf("expected activeCol=0, got %d", m.ActiveCol())
	}
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view even with empty issues")
	}
}

func TestRefreshMsg_UpdatesColumns(t *testing.T) {
	// Start with 2 open issues
	initial := []*materialize.Issue{
		{ID: "OPEN-1", Title: "Open issue 1", Status: "open"},
		{ID: "OPEN-2", Title: "Open issue 2", Status: "open"},
	}
	m := New(initial, 120, 40)
	if len(m.columns[ColOpen]) != 2 {
		t.Fatalf("expected 2 open issues initially, got %d", len(m.columns[ColOpen]))
	}

	// Refresh with different issues
	newIssues := []*materialize.Issue{
		{ID: "OPEN-1", Title: "Open issue 1", Status: "open"},
		{ID: "ACT-1", Title: "Active issue 1", Status: "in-progress"},
		{ID: "DONE-1", Title: "Done issue 1", Status: "done"},
	}
	updated, _ := m.Update(RefreshMsg{Issues: newIssues})
	result := updated.(Model)

	if len(result.columns[ColOpen]) != 1 {
		t.Errorf("expected 1 open issue after refresh, got %d", len(result.columns[ColOpen]))
	}
	if len(result.columns[ColActive]) != 1 {
		t.Errorf("expected 1 active issue after refresh, got %d", len(result.columns[ColActive]))
	}
	if len(result.columns[ColDone]) != 1 {
		t.Errorf("expected 1 done issue after refresh, got %d", len(result.columns[ColDone]))
	}
}

func TestRefreshMsg_ClampsCursorPosition(t *testing.T) {
	// Start with 3 open issues, cursor at 2
	initial := []*materialize.Issue{
		{ID: "OPEN-1", Title: "Open issue 1", Status: "open"},
		{ID: "OPEN-2", Title: "Open issue 2", Status: "open"},
		{ID: "OPEN-3", Title: "Open issue 3", Status: "open"},
	}
	m := New(initial, 120, 40)
	// Move cursor to bottom
	m.cursors[ColOpen] = 2

	// Refresh with only 1 open issue — cursor must clamp to 0
	newIssues := []*materialize.Issue{
		{ID: "OPEN-1", Title: "Open issue 1", Status: "open"},
	}
	updated, _ := m.Update(RefreshMsg{Issues: newIssues})
	result := updated.(Model)

	if result.cursors[ColOpen] != 0 {
		t.Errorf("expected cursor clamped to 0, got %d", result.cursors[ColOpen])
	}
}

func TestRefreshMsg_PreservesCursorWhenValid(t *testing.T) {
	// Start with 3 open issues, cursor at 1
	initial := []*materialize.Issue{
		{ID: "OPEN-1", Title: "Open issue 1", Status: "open"},
		{ID: "OPEN-2", Title: "Open issue 2", Status: "open"},
		{ID: "OPEN-3", Title: "Open issue 3", Status: "open"},
	}
	m := New(initial, 120, 40)
	m.cursors[ColOpen] = 1

	// Refresh with 3 open issues still — cursor stays at 1
	newIssues := []*materialize.Issue{
		{ID: "OPEN-A", Title: "Open issue A", Status: "open"},
		{ID: "OPEN-B", Title: "Open issue B", Status: "open"},
		{ID: "OPEN-C", Title: "Open issue C", Status: "open"},
	}
	updated, _ := m.Update(RefreshMsg{Issues: newIssues})
	result := updated.(Model)

	if result.cursors[ColOpen] != 1 {
		t.Errorf("expected cursor preserved at 1, got %d", result.cursors[ColOpen])
	}
}

func TestNewWithRefresh(t *testing.T) {
	issues := makeIssues()
	m := NewWithRefresh(issues, 120, 40, "/some/dir", "/some/state/dir")
	if m.issuesDir != "/some/dir" {
		t.Errorf("expected issuesDir='/some/dir', got %q", m.issuesDir)
	}
}

func TestInitWithRefresh_ReturnsCmd(t *testing.T) {
	issues := makeIssues()
	m := NewWithRefresh(issues, 120, 40, "/some/dir", "/some/state/dir")
	cmd := m.Init()
	if cmd == nil {
		t.Error("expected non-nil cmd from Init when issuesDir is set")
	}
}

func TestInit_NoRefresh_ReturnsNil(t *testing.T) {
	// Model created via New (no issuesDir) returns nil from Init
	m := New(makeIssues(), 120, 40)
	cmd := m.Init()
	if cmd != nil {
		t.Error("expected nil cmd from Init when issuesDir is empty")
	}
}
