package validate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanCouplingDetection_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:               "STORY-1",
			Type:             "story",
			DefinitionOfDone: "Deliver the vertical slice end to end, covering both implementation and documentation.",
		},
		&materialize.Issue{
			ID:               "TSK-CODE",
			Type:             "task",
			Parent:           "STORY-1",
			Scope:            []string{"cmd/armature/flag.go (new)"},
			DefinitionOfDone: "The new flag is implemented and wired up.",
			Acceptance:       json.RawMessage(`["flag works as documented"]`),
		},
		&materialize.Issue{
			ID:               "TSK-DOCS",
			Type:             "task",
			Parent:           "STORY-1",
			Scope:            []string{"docs/commands.md"},
			DefinitionOfDone: "docs/commands.md documents the new flag.",
			Acceptance:       json.RawMessage(`["docs/commands.md mentions the new flag"]`),
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})

	assert.False(t, result.OK)
	e13 := findErrorContaining(result, "E13:")
	require.NotEmpty(t, e13, "expected an E13 error, got: %v", result.Errors)
	assert.Contains(t, e13, "TSK-CODE", "E13 message should name the code task, got: %s", e13)
	assert.Contains(t, e13, "TSK-DOCS", "E13 message should name the docs task, got: %s", e13)
	assert.Contains(t, e13, "docs/commands.md", "E13 message should name the coupled file, got: %s", e13)
	assert.Contains(t, e13, "co-locate", "E13 message should state the remedy, got: %s", e13)
}

func findErrorContaining(r Result, substr string) string {
	for _, e := range r.Errors {
		if strings.Contains(e, substr) {
			return e
		}
	}
	return ""
}

func TestPlanCouplingCoLocatedScopePasses_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:     "TSK-VERTICAL",
			Type:   "task",
			Parent: "STORY-1",
			Scope:  []string{"cmd/armature/flag.go (new)", "docs/commands.md"},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})

	assert.False(t, containsError(result, "E13"), "co-located scope should not trigger coupling error, got: %v", result.Errors)
}

func TestPlanCouplingIgnoresOtherStories_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:     "TSK-CODE",
			Type:   "task",
			Parent: "STORY-1",
			Scope:  []string{"cmd/armature/flag.go (new)"},
		},
		&materialize.Issue{
			ID:     "TSK-DOCS",
			Type:   "task",
			Parent: "STORY-2",
			Scope:  []string{"docs/commands.md"},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})

	assert.False(t, containsError(result, "E13"), "coupling across different stories should not trigger, got: %v", result.Errors)
}

func TestPlanCouplingNoDocOwner_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:     "TSK-CODE",
			Type:   "task",
			Parent: "STORY-1",
			Scope:  []string{"cmd/armature/flag.go (new)"},
		},
		&materialize.Issue{
			ID:     "TSK-OTHER",
			Type:   "task",
			Parent: "STORY-1",
			Scope:  []string{"internal/other/pkg.go"},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})

	assert.False(t, containsError(result, "E13"), "no coupling when no sibling owns the census/doc files, got: %v", result.Errors)
}

func TestPlanCouplingSkipsTerminalStatusTasks_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:     "TSK-CODE-DONE",
			Type:   "task",
			Parent: "STORY-1",
			Status: ops.StatusMerged,
			Scope:  []string{"cmd/armature/flag.go (new)"},
		},
		&materialize.Issue{
			ID:     "TSK-DOCS-DONE",
			Type:   "task",
			Parent: "STORY-1",
			Status: ops.StatusMerged,
			Scope:  []string{"docs/commands.md"},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})

	assert.False(t, containsError(result, "E13"), "merged sibling tasks should not trigger coupling error, got: %v", result.Errors)
}

func TestPlanCouplingDeduplicatesPairAcrossDirections_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:     "TSK-A",
			Type:   "task",
			Parent: "STORY-1",
			Scope:  []string{"cmd/armature/a.go (new)", "docs/design/surface-census.md"},
		},
		&materialize.Issue{
			ID:     "TSK-B",
			Type:   "task",
			Parent: "STORY-1",
			Scope:  []string{"cmd/armature/b.go (new)", "docs/commands.md"},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})

	e13Count := 0
	for _, err := range result.Errors {
		if strings.Contains(err, "E13") {
			e13Count++
		}
	}
	assert.Equal(t, 1, e13Count, "coupled pair should be reported once, not once per direction, got: %v", result.Errors)
}
