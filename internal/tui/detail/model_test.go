package detail_test

import (
	"strings"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/detail"
)

func TestDetailViewRendersTitle(t *testing.T) {
	issue := &materialize.Issue{
		ID:    "TASK-14",
		Title: "Build detail overlay model",
	}
	m := detail.New()
	m = m.SetSize(80, 24)
	m = m.Open(issue)
	view := m.View()
	if !strings.Contains(view, "TASK-14") {
		t.Errorf("expected TASK-14, got:\n%s", view)
	}
	if !strings.Contains(view, "Build detail overlay model") {
		t.Errorf("expected title, got:\n%s", view)
	}
}

func TestDetailViewHidden(t *testing.T) {
	// Case 1: Issue is nil
	m := detail.New()
	m = m.Open(nil)
	if m.View() != "" {
		t.Errorf("expected empty view for nil issue")
	}

	// Case 2: Closed
	issue := &materialize.Issue{ID: "TASK-14"}
	m2 := detail.New()
	m2 = m2.Open(issue)
	m2 = m2.Close()
	if m2.View() != "" {
		t.Errorf("expected empty view when closed")
	}
}
