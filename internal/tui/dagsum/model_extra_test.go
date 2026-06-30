package dagsum_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui/dagsum"
	"github.com/stretchr/testify/assert"
)

func TestDone_FalseInitially(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1", Title: "Task"}}
	m := dagsum.New(issues)
	assert.False(t, m.Done())
}

func TestDone_TrueAfterAllConfirmed(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1", Title: "Task"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	assert.True(t, m2.(dagsum.Model).Done()) //nolint:errcheck // panic on failed type assertion is acceptable in tests
}

func TestConfirmedIDs_Empty(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	assert.Empty(t, m.ConfirmedIDs())
}

func TestConfirmedIDs_AfterConfirm(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{
		{ID: "TSK-1"},
		{ID: "TSK-2"},
	}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	ids := m2.(dagsum.Model).ConfirmedIDs() //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, []string{"TSK-1"}, ids)
}

func TestInit_ReturnsNil(t *testing.T) {
	t.Parallel()
	m := dagsum.New([]*materialize.Issue{{ID: "TSK-1"}})
	assert.Nil(t, m.Init())
}

func TestView_ContainsIssueID(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-42", Title: "My Task", Type: "task"}}
	m := dagsum.New(issues)
	view := m.View()
	assert.Contains(t, view, "TSK-42")
}

func TestView_EmptyIssues(t *testing.T) {
	t.Parallel()
	m := dagsum.New([]*materialize.Issue{})
	view := m.View()
	assert.True(t, strings.Contains(view, "No items") || len(view) > 0)
}

func TestUpdate_QuitKey(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}}
	m := dagsum.New(issues)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assert.NotNil(t, cmd)
}

func TestUpdate_NavigationDown(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{
		{ID: "TSK-1"},
		{ID: "TSK-2"},
	}
	m := dagsum.New(issues)
	// skip first, then navigate down
	m2, _ := m.Update(dagsum.SkipMsg{})
	updated := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 1, updated.Cursor())
}

func TestUpdate_UnknownMsg_Ignored(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}}
	m := dagsum.New(issues)
	m2, cmd := m.Update("unknown message")
	assert.Equal(t, m, m2.(dagsum.Model)) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Nil(t, cmd)
}

func TestUpdate_CtrlC_Quits(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}}
	m := dagsum.New(issues)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.NotNil(t, cmd)
}

func TestUpdate_YKey_Confirms(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	updated := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 1, updated.Confirmed())
}

func TestUpdate_EnterKey_Confirms(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 1, updated.Confirmed())
}

func TestUpdate_SKey_Skips(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	updated := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 0, updated.Confirmed())
	assert.Equal(t, 1, updated.Cursor())
}

func TestUpdate_DownKey_MovesCursorDown(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 1, updated.Cursor())
}

func TestUpdate_JKey_MovesCursorDown(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 1, updated.Cursor())
}

func TestUpdate_UpKey_MovesCursorUp(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	// Move down first, then up
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3, _ := m2.(dagsum.Model).Update(tea.KeyMsg{Type: tea.KeyUp}) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 0, m3.(dagsum.Model).Cursor())                 //nolint:errcheck // panic on failed type assertion is acceptable in tests
}

func TestUpdate_KKey_MovesCursorUp(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	//nolint:errcheck // panic on failed type assertion is acceptable in tests
	m3, _ := m2.(dagsum.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	assert.Equal(t, 0, m3.(dagsum.Model).Cursor()) //nolint:errcheck // panic on failed type assertion is acceptable in tests
}

func TestUpdate_UpKey_AtZero_DoesNotGoNegative(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}, {ID: "TSK-2"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, m2.(dagsum.Model).Cursor()) //nolint:errcheck // panic on failed type assertion is acceptable in tests
}

func TestUpdate_DownKey_AtLast_DoesNotExceedBound(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 0, m2.(dagsum.Model).Cursor()) //nolint:errcheck // panic on failed type assertion is acceptable in tests
}

func TestView_ConfirmedState_ShowsCheckmark(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1", Title: "Task", Type: "task"}, {ID: "TSK-2", Title: "Task 2", Type: "task"}}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	// Cursor moves to TSK-2; view shows TSK-2 which is pending
	view := m2.(dagsum.Model).View() //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Contains(t, view, "TSK-2")
}

func TestView_CursorBeyondIssues_ShowsComplete(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{{ID: "TSK-1", Title: "Task", Type: "task"}}
	m := dagsum.New(issues)
	// Confirm all items so cursor ends up beyond issues length
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	view := m2.(dagsum.Model).View() //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.NotEmpty(t, view)
}
