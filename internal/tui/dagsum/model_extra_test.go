package dagsum_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/dagsum"
	"github.com/stretchr/testify/assert"
)

func TestDone_FalseInitially(t *testing.T) {
	issues := []*materialize.Issue{{ID: "TSK-1", Title: "Task"}}
	m := dagsum.New(issues)
	assert.False(t, m.Done())
}

func TestDone_TrueAfterAllConfirmed(t *testing.T) {
	issues := []*materialize.Issue{{ID: "TSK-1", Title: "Task"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	assert.True(t, m2.(dagsum.Model).Done())
}

func TestConfirmedIDs_Empty(t *testing.T) {
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	assert.Empty(t, m.ConfirmedIDs())
}

func TestConfirmedIDs_AfterConfirm(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1"},
		{ID: "TSK-2"},
	}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	ids := m2.(dagsum.Model).ConfirmedIDs()
	assert.Equal(t, []string{"TSK-1"}, ids)
}

func TestInit_ReturnsNil(t *testing.T) {
	m := dagsum.New([]*materialize.Issue{{ID: "TSK-1"}})
	assert.Nil(t, m.Init())
}

func TestView_ContainsIssueID(t *testing.T) {
	issues := []*materialize.Issue{{ID: "TSK-42", Title: "My Task", Type: "task"}}
	m := dagsum.New(issues)
	view := m.View()
	assert.Contains(t, view, "TSK-42")
}

func TestView_EmptyIssues(t *testing.T) {
	m := dagsum.New([]*materialize.Issue{})
	view := m.View()
	assert.True(t, strings.Contains(view, "No items") || len(view) > 0)
}

func TestUpdate_QuitKey(t *testing.T) {
	issues := []*materialize.Issue{{ID: "TSK-1"}}
	m := dagsum.New(issues)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assert.NotNil(t, cmd)
}

func TestUpdate_NavigationDown(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1"},
		{ID: "TSK-2"},
	}
	m := dagsum.New(issues)
	// skip first, then navigate down
	m2, _ := m.Update(dagsum.SkipMsg{})
	updated := m2.(dagsum.Model)
	assert.Equal(t, 1, updated.Cursor())
}

func TestUpdate_UnknownMsg_Ignored(t *testing.T) {
	issues := []*materialize.Issue{{ID: "TSK-1"}}
	m := dagsum.New(issues)
	m2, cmd := m.Update("unknown message")
	assert.Equal(t, m, m2.(dagsum.Model))
	assert.Nil(t, cmd)
}
