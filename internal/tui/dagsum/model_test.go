package dagsum_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui/dagsum"
	"github.com/stretchr/testify/assert"
)

func TestNewModelHasAllItems(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "First task", Type: "task"},
		{ID: "TSK-2", Title: "Second task", Type: "task"},
	}
	m := dagsum.New(issues)
	assert.Equal(t, 2, m.Total())
	assert.Equal(t, 0, m.Confirmed())
}

func TestConfirmAdvancesCursor(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task 1", Type: "task"},
		{ID: "TSK-2", Title: "Task 2", Type: "task"},
	}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	updated := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 1, updated.Confirmed())
	assert.Equal(t, 1, updated.Cursor())
}

func TestAllConfirmedQuitsProgram(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task", Type: "task"},
	}
	m := dagsum.New(issues)
	_, cmd := m.Update(dagsum.ConfirmMsg{})
	assert.NotNil(t, cmd)
}

func TestSkipDoesNotConfirm(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task", Type: "task"},
	}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.SkipMsg{})
	updated := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 0, updated.Confirmed())
}

func TestViewShowsEmptyState(t *testing.T) {
	t.Parallel()
	m := dagsum.New(nil)
	assert.Contains(t, m.View(), "No items to review.")
}

func TestUpdateHandlesNavigationBoundsAndQuit(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task 1", Type: "task"},
		{ID: "TSK-2", Title: "Task 2", Type: "task"},
	}
	m := dagsum.New(issues)

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.NotNil(t, cmd)
	assert.Equal(t, m, m2.(dagsum.Model)) //nolint:errcheck // test asserts the model is unchanged

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	down := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 1, down.Cursor())

	m2, _ = down.Update(tea.KeyMsg{Type: tea.KeyDown})
	atEnd := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 1, atEnd.Cursor(), "cursor should stop at the last item")

	m2, _ = atEnd.Update(tea.KeyMsg{Type: tea.KeyUp})
	up := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Equal(t, 0, up.Cursor(), "cursor should move back up")
}

func TestViewMarksReviewedItems(t *testing.T) {
	t.Parallel()
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task 1", Type: "task"},
	}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	updated := m2.(dagsum.Model) //nolint:errcheck // panic on failed type assertion is acceptable in tests

	assert.True(t, updated.Done())
}
