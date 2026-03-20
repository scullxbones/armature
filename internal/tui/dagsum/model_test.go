package dagsum_test

import (
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/dagsum"
	"github.com/stretchr/testify/assert"
)

func TestNewModelHasAllItems(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "First task", Type: "task"},
		{ID: "TSK-2", Title: "Second task", Type: "task"},
	}
	m := dagsum.New(issues)
	assert.Equal(t, 2, m.Total())
	assert.Equal(t, 0, m.Confirmed())
}

func TestConfirmAdvancesCursor(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task 1", Type: "task"},
		{ID: "TSK-2", Title: "Task 2", Type: "task"},
	}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	updated := m2.(dagsum.Model)
	assert.Equal(t, 1, updated.Confirmed())
	assert.Equal(t, 1, updated.Cursor())
}

func TestAllConfirmedQuitsProgram(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task", Type: "task"},
	}
	m := dagsum.New(issues)
	_, cmd := m.Update(dagsum.ConfirmMsg{})
	assert.NotNil(t, cmd)
}

func TestSkipDoesNotConfirm(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task", Type: "task"},
	}
	m := dagsum.New(issues)
	m2, _ := m.Update(dagsum.SkipMsg{})
	updated := m2.(dagsum.Model)
	assert.Equal(t, 0, updated.Confirmed())
}
