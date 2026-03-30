package workers

import (
	"strings"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
)

func TestWorkersViewContainsWorkerID(t *testing.T) {
	m := New()
	state := &materialize.State{
		Issues: map[string]*materialize.Issue{
			"TASK-1": {
				ID:        "TASK-1",
				Title:     "Task 1",
				ClaimedBy: "worker-1",
			},
		},
	}
	m.SetState(state)

	view := m.View()
	if !strings.Contains(view, "worker-1") {
		t.Errorf("expected view to contain worker-1, got:\n%s", view)
	}
	if !strings.Contains(view, "TASK-1") {
		t.Errorf("expected view to contain TASK-1, got:\n%s", view)
	}
}

func TestWorkersHelpBar(t *testing.T) {
	m := New()
	help := m.HelpBar()
	if !strings.Contains(help, "j/k move") {
		t.Errorf("expected help bar to contain j/k move, got: %s", help)
	}
}
