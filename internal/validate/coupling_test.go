package validate

import (
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestPlanCouplingDetection_REQ_LNGHZN_S10_T5(t *testing.T) {
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
			Parent: "STORY-1",
			Scope:  []string{"docs/commands.md"},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})

	assert.False(t, result.OK)
	assert.True(t, containsError(result, "TSK-CODE"), "error should name the code task, got: %v", result.Errors)
	assert.True(t, containsError(result, "TSK-DOCS"), "error should name the docs task, got: %v", result.Errors)
	assert.True(t, containsError(result, "docs/commands.md"), "error should name the coupled file, got: %v", result.Errors)
	assert.True(t, containsError(result, "co-locate"), "error should state the remedy, got: %v", result.Errors)
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
