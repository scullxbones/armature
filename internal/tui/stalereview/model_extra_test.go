package stalereview_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
	stalereview "github.com/scullxbones/trellis/internal/tui/stalereview"
	"github.com/stretchr/testify/assert"
)

func makeItems() []stalereview.ReviewItem {
	return []stalereview.ReviewItem{
		{
			SourceID:      "prd",
			ChangeSummary: "section updated",
			CitedIssues:   []*materialize.Issue{{ID: "TSK-1", Title: "Task"}},
		},
		{
			SourceID:      "arch",
			ChangeSummary: "diagram updated",
			CitedIssues:   []*materialize.Issue{{ID: "TSK-2", Title: "Other"}},
		},
	}
}

func TestDecisions_InitiallyAllPending(t *testing.T) {
	m := stalereview.New(makeItems(), "w1")
	for i, d := range m.Decisions() {
		if d != 0 {
			t.Errorf("decision[%d] expected 0 (pending), got %d", i, d)
		}
	}
}

func TestItems_ReturnsAll(t *testing.T) {
	items := makeItems()
	m := stalereview.New(items, "w1")
	assert.Equal(t, len(items), len(m.Items()))
	assert.Equal(t, items[0].SourceID, m.Items()[0].SourceID)
}

func TestInit_ReturnsNil(t *testing.T) {
	m := stalereview.New(makeItems(), "w1")
	assert.Nil(t, m.Init())
}

func TestUpdate_FlagMsg(t *testing.T) {
	m := stalereview.New(makeItems(), "w1")
	m2, _ := m.Update(stalereview.FlagMsg{})
	updated := m2.(stalereview.Model)
	assert.Equal(t, 0, updated.ConfirmedCount())
}

func TestUpdate_SkipMsg(t *testing.T) {
	m := stalereview.New(makeItems(), "w1")
	m2, _ := m.Update(stalereview.SkipMsg{})
	updated := m2.(stalereview.Model)
	assert.Equal(t, 0, updated.ConfirmedCount())
}

func TestUpdate_QuitKey(t *testing.T) {
	m := stalereview.New(makeItems(), "w1")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assert.NotNil(t, cmd)
}

func TestUpdate_FlagKey(t *testing.T) {
	m := stalereview.New(makeItems(), "w1")
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
}

func TestUpdate_SkipKey(t *testing.T) {
	m := stalereview.New(makeItems(), "w1")
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
}

func TestView_ContainsSourceID(t *testing.T) {
	m := stalereview.New(makeItems(), "w1")
	view := m.View()
	assert.Contains(t, view, "prd")
}

func TestView_Complete_WhenNoItems(t *testing.T) {
	m := stalereview.New([]stalereview.ReviewItem{}, "w1")
	view := m.View()
	assert.Contains(t, view, "complete")
}

func TestAllDecided_Quits(t *testing.T) {
	items := []stalereview.ReviewItem{
		{SourceID: "s1", CitedIssues: []*materialize.Issue{{ID: "T1"}}},
	}
	m := stalereview.New(items, "w1")
	_, cmd := m.Update(stalereview.ConfirmMsg{})
	assert.NotNil(t, cmd)
}
