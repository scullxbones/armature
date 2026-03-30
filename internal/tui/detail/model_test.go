package detail_test

import (
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/detail"
	"github.com/stretchr/testify/assert"
)

func TestDetailViewRendersTitle(t *testing.T) {
	issue := &materialize.Issue{
		ID:    "TASK-14",
		Title: "Build detail overlay model",
	}
	m := detail.New(issue, 80, 24)
	view := m.View()
	assert.Contains(t, view, "TASK-14")
	assert.Contains(t, view, "Build detail overlay model")
}

func TestDetailViewHidden(t *testing.T) {
	// Case 1: Issue is nil
	m := detail.New(nil, 80, 24)
	assert.Empty(t, m.View())

	// Case 2: Hidden is true
	issue := &materialize.Issue{ID: "TASK-14"}
	m2 := detail.New(issue, 80, 24)
	m2.Hidden = true
	assert.Empty(t, m2.View())
}
