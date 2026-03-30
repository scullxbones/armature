package tuivalidate_test

import (
	"strings"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/tuivalidate"
)

func TestValidateInit(t *testing.T) {
	m := tuivalidate.New()
	if cmd := m.Init(); cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestValidateSetSize(t *testing.T) {
	m := tuivalidate.New()
	m.SetSize(80, 24) // must not panic
}

func TestValidateHelpBar(t *testing.T) {
	m := tuivalidate.New()
	h := m.HelpBar()
	if !strings.Contains(h, "q quit") {
		t.Errorf("help bar missing q quit, got: %s", h)
	}
}

func TestValidateUpdate(t *testing.T) {
	m := tuivalidate.New()
	screen, cmd := m.Update(nil)
	if screen == nil {
		t.Error("Update should return the model")
	}
	if cmd != nil {
		t.Error("Update should return nil cmd")
	}
}

func TestValidateNilStateView(t *testing.T) {
	m := tuivalidate.New()
	v := m.View()
	if !strings.Contains(v, "No state available") {
		t.Errorf("expected nil-state message, got: %s", v)
	}
}

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
