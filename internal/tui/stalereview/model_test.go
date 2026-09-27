package stalereview_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	stalereview "github.com/scullxbones/armature/internal/tui/stalereview"
	"github.com/stretchr/testify/assert"
)

func TestNewModelHasItems(t *testing.T) {
	t.Parallel()
	items := []stalereview.ReviewItem{
		{SourceID: "prd", ChangeSummary: "Section 3 updated",
			CitedIssues: []*materialize.Issue{{ID: "TSK-1", Title: "Task 1"}}},
	}
	m := stalereview.New(items, "worker-1")
	assert.Equal(t, 1, m.Total())
}

func TestConfirmRecordsDecision(t *testing.T) {
	t.Parallel()
	items := []stalereview.ReviewItem{
		{SourceID: "prd", ChangeSummary: "Updated",
			CitedIssues: []*materialize.Issue{{ID: "TSK-1"}}},
	}
	m := stalereview.New(items, "worker-1")
	m2, _ := m.Update(stalereview.ConfirmMsg{})
	updated, ok := m2.(stalereview.Model)
	if !ok {
		t.Fatalf("Update returned %T, want stalereview.Model", m2)
	}
	assert.Equal(t, 1, updated.ConfirmedCount())
}
