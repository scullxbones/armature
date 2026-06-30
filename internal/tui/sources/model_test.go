package sources_test

import (
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui/sources"
)

func TestSourcesInit(t *testing.T) {
	m := sources.New()
	if cmd := m.Init(); cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestSourcesSetSize(t *testing.T) {
	m := sources.New()
	m.SetSize(80, 24) // must not panic
}

func TestSourcesHelpBar(t *testing.T) {
	m := sources.New()
	h := m.HelpBar()
	if !strings.Contains(h, "q quit") {
		t.Errorf("help bar missing q quit, got: %s", h)
	}
}

func TestSourcesUpdate(t *testing.T) {
	m := sources.New()
	screen, cmd := m.Update(nil)
	if screen == nil {
		t.Error("Update should return the model")
	}
	if cmd != nil {
		t.Error("Update should return nil cmd")
	}
}

func TestSourcesNilStateView(t *testing.T) {
	m := sources.New()
	v := m.View()
	if !strings.Contains(v, "No state available") {
		t.Errorf("expected nil-state message, got: %s", v)
	}
}

func TestSourcesEmptySourcesView(t *testing.T) {
	m := sources.New()
	m.SetState(materialize.NewState())
	v := m.View()
	if !strings.Contains(v, "No sources cited") {
		t.Errorf("expected empty-sources message, got: %s", v)
	}
}

func TestSourcesScreen(t *testing.T) {
	m := sources.New()
	state := materialize.NewState()
	state.Issues["T1"] = &materialize.Issue{ID: "T1", Type: "task", SourceLinks: []materialize.SourceLink{{SourceEntryID: "SRC-1"}}}
	state.Issues["T2"] = &materialize.Issue{ID: "T2", Type: "task", SourceLinks: []materialize.SourceLink{{SourceEntryID: "SRC-1"}}}
	state.Issues["T3"] = &materialize.Issue{ID: "T3", Type: "task", SourceLinks: []materialize.SourceLink{{SourceEntryID: "SRC-2"}}}

	m.SetState(state)
	v := m.View()

	if !strings.Contains(v, "SRC-1: T1, T2") {
		t.Errorf("expected SRC-1 aggregation, got:\n%s", v)
	}
	if !strings.Contains(v, "SRC-2: T3") {
		t.Errorf("expected SRC-2 aggregation, got:\n%s", v)
	}
}
