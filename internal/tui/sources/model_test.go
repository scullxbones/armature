package sources_test

import (
	"strings"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui/sources"
)

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
