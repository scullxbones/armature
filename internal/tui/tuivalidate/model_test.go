package tuivalidate_test

import (
	"strings"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/tuivalidate"
)

func TestValidateScreenRendersIssues(t *testing.T) {
	m := tuivalidate.New()
	state := materialize.NewState()
	// No issues -> should show OK.
	m.SetState(state)
	v := m.View()
	if !strings.Contains(v, "No issues found") {
		t.Errorf("expected OK, got:\n%s", v)
	}

	// Add an issue that causes an error.
	state.Issues["T1"] = &materialize.Issue{ID: "T1", Type: "task", Parent: "E1"} // E1 missing
	m.SetState(state)
	v = m.View()
	if !strings.Contains(v, "ERROR:") {
		t.Errorf("expected ERROR, got:\n%s", v)
	}
}
