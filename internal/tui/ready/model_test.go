package ready_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	internalready "github.com/scullxbones/armature/internal/ready"
	readytui "github.com/scullxbones/armature/internal/tui/ready"
	"github.com/stretchr/testify/assert"
)

func makeEntries() []internalready.ReadyEntry {
	return []internalready.ReadyEntry{
		{Issue: "E5-S2-T3", Title: "trls ready interactive TUI", Priority: "medium"},
		{Issue: "E5-S3-T1", Title: "Git client forensics methods", Priority: "medium"},
		{Issue: "E5-S3-T2", Title: "Another task", Priority: "low"},
	}
}

func TestNew_InitialState(t *testing.T) {
	entries := makeEntries()
	m := readytui.New(entries)
	assert.Equal(t, 0, m.Cursor())
	assert.Equal(t, "", m.Selected())
	assert.False(t, m.Quit())
}

func TestNew_EmptyEntries(t *testing.T) {
	m := readytui.New(nil)
	assert.Equal(t, 0, m.Cursor())
	assert.Equal(t, "", m.Selected())
}

func TestInit_ReturnsNil(t *testing.T) {
	m := readytui.New(makeEntries())
	cmd := m.Init()
	assert.Nil(t, cmd)
}

func TestUpdate_MoveDown(t *testing.T) {
	m := readytui.New(makeEntries())
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := m2.(readytui.Model)
	assert.Equal(t, 1, updated.Cursor())
}

func TestUpdate_MoveDownKey(t *testing.T) {
	m := readytui.New(makeEntries())
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated := m2.(readytui.Model)
	assert.Equal(t, 1, updated.Cursor())
}

func TestUpdate_MoveUp(t *testing.T) {
	m := readytui.New(makeEntries())
	// Move down first
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	updated := m3.(readytui.Model)
	assert.Equal(t, 0, updated.Cursor())
}

func TestUpdate_MoveUpKey(t *testing.T) {
	m := readytui.New(makeEntries())
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated := m3.(readytui.Model)
	assert.Equal(t, 0, updated.Cursor())
}

func TestUpdate_MoveUpBounded(t *testing.T) {
	m := readytui.New(makeEntries())
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	updated := m2.(readytui.Model)
	assert.Equal(t, 0, updated.Cursor())
}

func TestUpdate_MoveDownBounded(t *testing.T) {
	entries := makeEntries()
	m := readytui.New(entries)
	// Move to last item
	for i := 0; i < len(entries)+5; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = next.(readytui.Model)
	}
	assert.Equal(t, len(entries)-1, m.Cursor())
}

func TestUpdate_EnterSelectsCurrentItem(t *testing.T) {
	m := readytui.New(makeEntries())
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(readytui.Model)
	assert.Equal(t, "E5-S2-T3", updated.Selected())
	assert.NotNil(t, cmd)
}

func TestUpdate_EnterSelectsAfterNavigation(t *testing.T) {
	m := readytui.New(makeEntries())
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m3.(readytui.Model)
	assert.Equal(t, "E5-S3-T1", updated.Selected())
	assert.NotNil(t, cmd)
}

func TestUpdate_QuitWithQ(t *testing.T) {
	m := readytui.New(makeEntries())
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	updated := m2.(readytui.Model)
	assert.True(t, updated.Quit())
	assert.Equal(t, "", updated.Selected())
	assert.NotNil(t, cmd)
}

func TestUpdate_QuitWithCtrlC(t *testing.T) {
	m := readytui.New(makeEntries())
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := m2.(readytui.Model)
	assert.True(t, updated.Quit())
	assert.NotNil(t, cmd)
}

func TestUpdate_EnterOnEmpty(t *testing.T) {
	m := readytui.New(nil)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(readytui.Model)
	assert.Equal(t, "", updated.Selected())
}

func TestView_ShowsHeader(t *testing.T) {
	m := readytui.New(makeEntries())
	view := m.View()
	assert.Contains(t, view, "Select a task to claim")
	assert.Contains(t, view, "j/k=move")
	assert.Contains(t, view, "enter=claim")
	assert.Contains(t, view, "q=quit")
}

func TestView_ShowsAllEntries(t *testing.T) {
	m := readytui.New(makeEntries())
	view := m.View()
	assert.Contains(t, view, "E5-S2-T3")
	assert.Contains(t, view, "E5-S3-T1")
	assert.Contains(t, view, "E5-S3-T2")
}

func TestView_ShowsPriority(t *testing.T) {
	m := readytui.New(makeEntries())
	view := m.View()
	assert.Contains(t, view, "medium")
	assert.Contains(t, view, "low")
}

func TestView_EmptyEntries(t *testing.T) {
	m := readytui.New(nil)
	view := m.View()
	assert.Contains(t, view, "No tasks ready")
}

func TestView_SelectedItemHasCursor(t *testing.T) {
	m := readytui.New(makeEntries())
	view := m.View()
	// The first item should have a ">" cursor indicator
	lines := strings.Split(view, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "E5-S2-T3") && strings.Contains(line, ">") {
			found = true
			break
		}
	}
	assert.True(t, found, "selected item should have > cursor")
}

func TestClaimMsg(t *testing.T) {
	msg := readytui.ClaimMsg{IssueID: "E5-S2-T3"}
	assert.Equal(t, "E5-S2-T3", msg.IssueID)
}
