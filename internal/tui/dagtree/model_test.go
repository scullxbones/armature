package dagtree_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/dagtree"
)

func makeState(issues ...*materialize.Issue) *materialize.State {
	s := materialize.NewState()
	for _, i := range issues {
		issueCopy := *i
		s.Issues[i.ID] = &issueCopy
	}
	return s
}

func TestDAGTreeInit(t *testing.T) {
	m := dagtree.New()
	if cmd := m.Init(); cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestDAGTreeNilStateView(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	v := m.View()
	if !strings.Contains(v, "No state available") {
		t.Errorf("expected nil-state message, got: %s", v)
	}
}

func TestDAGTreeGlyphInProgress(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	m.SetState(makeState(&materialize.Issue{ID: "T1", Status: "in-progress", Title: "Active"}))
	v := m.View()
	if !strings.Contains(v, "▶") {
		t.Errorf("in-progress node should show ▶, got:\n%s", v)
	}
}

func TestDAGTreeGlyphCancelled(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	m.SetState(makeState(&materialize.Issue{ID: "T1", Status: "cancelled", Title: "Cancelled"}))
	v := m.View()
	if !strings.Contains(v, "—") {
		t.Errorf("cancelled node should show —, got:\n%s", v)
	}
}

func TestDAGTreeGlyphDefault(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	m.SetState(makeState(&materialize.Issue{ID: "T1", Status: "open", Title: "Open task"}))
	v := m.View()
	if !strings.Contains(v, "○") {
		t.Errorf("open node should show ○, got:\n%s", v)
	}
}

func TestDAGTreeUpdate_CursorNavigation(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	m.SetState(makeState(
		&materialize.Issue{ID: "E1", Type: "epic", Status: "open", Title: "Epic"},
		&materialize.Issue{ID: "E2", Type: "epic", Status: "open", Title: "Epic 2"},
	))
	// Move down
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	// Move up
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	// Boundary: cannot go above 0
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	// Expand/collapse
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
}

func TestDAGTreeUpdate_OpenDetail(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	m.SetState(makeState(&materialize.Issue{ID: "E1", Status: "open", Title: "Epic"}))
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func TestViewContainsIssueIDs(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	m.SetState(makeState(
		&materialize.Issue{ID: "E1", Type: "epic", Status: "open", Title: "Epic One"},
		&materialize.Issue{ID: "E1-S1", Type: "story", Status: "open", Title: "Story One", Parent: "E1"},
	))
	v := m.View()
	if !strings.Contains(v, "E1") {
		t.Errorf("View missing epic ID, got:\n%s", v)
	}
	if !strings.Contains(v, "E1-S1") {
		t.Errorf("View missing story ID, got:\n%s", v)
	}
}

func TestMergedNodeShowsCheckGlyph(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	m.SetState(makeState(&materialize.Issue{ID: "T1", Type: "task", Status: "merged", Title: "Done task"}))
	v := m.View()
	if !strings.Contains(v, "✓") {
		t.Errorf("merged node should show ✓ glyph, got:\n%s", v)
	}
}

func TestBlockedNodeShowsXGlyph(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	m.SetState(makeState(&materialize.Issue{ID: "T1", Type: "task", Status: "blocked", Title: "Blocked"}))
	v := m.View()
	if !strings.Contains(v, "✗") {
		t.Errorf("blocked node should show ✗ glyph, got:\n%s", v)
	}
}

func TestFilterHidesNonMatchingNodes(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	m.SetState(makeState(
		&materialize.Issue{ID: "E1", Type: "epic", Status: "open", Title: "Epic"},
		&materialize.Issue{ID: "E1-S1", Type: "story", Status: "open", Title: "visible", Parent: "E1"},
		&materialize.Issue{ID: "E1-S2", Type: "story", Status: "open", Title: "hidden", Parent: "E1"},
	))
	m = m.WithFilter("visible")
	v := m.View()
	if strings.Contains(v, "hidden") {
		t.Errorf("filter should hide non-matching nodes; got:\n%s", v)
	}
	if !strings.Contains(v, "E1") {
		t.Errorf("filter should keep ancestors visible; got:\n%s", v)
	}
}

func TestHelpBarContainsKeyHints(t *testing.T) {
	m := dagtree.New()
	m.SetSize(120, 40)
	h := m.HelpBar()
	if !strings.Contains(h, "j/k") {
		t.Errorf("help bar missing j/k hint, got: %q", h)
	}
}
