package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// TestPlanCouplingExemptsSelfCoLocatedSiblings asserts the shape E13 exists to
// reward: two siblings that each carry their own code AND their own census/doc
// lines are vertical slices, not a horizontal split, and must not be flagged.
func TestPlanCouplingExemptsSelfCoLocatedSiblings_REQ_LNGHZN_S10_T5(t *testing.T) {
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

	assert.False(t, containsError(result, "E13"),
		"self-co-located siblings are vertical slices, not coupling, got: %v", result.Errors)
}

// TestPlanCouplingIgnoresRepoWideScope asserts a repo-wide task (a lint sweep, a
// dependency bump) is not read as a phantom cmd/** code task. scopeTouchesSurface
// asks whether the entry definitely lands inside the surface, not whether the two
// globs could conceivably intersect.
func TestPlanCouplingIgnoresRepoWideScope_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{".", "**", "internal/**"} {
		state := makeState(
			&materialize.Issue{
				ID:     "TSK-BROAD",
				Type:   "task",
				Parent: "STORY-1",
				Scope:  []string{scope},
			},
			&materialize.Issue{
				ID:     "TSK-DOCS",
				Type:   "task",
				Parent: "STORY-1",
				Scope:  []string{"docs/commands.md"},
			},
		)
		result := Validate(state, graphFromState(state), Options{})
		assert.False(t, containsError(result, "E13"),
			"scope %q does not definitely land inside cmd/**, got: %v", scope, result.Errors)
	}
}

// TestPlanCouplingReportsOncePerCodeTask asserts findings scale with the number of
// offending tasks, not with the code x doc cross product. Per I4 the agent reading
// this output is the primary user: one finding per offending task, citing every
// implicated sibling, says the same thing once.
func TestPlanCouplingReportsOncePerCodeTask_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-C1", Type: "task", Parent: "STORY-1", Scope: []string{"cmd/armature/c1.go (new)"}},
		&materialize.Issue{ID: "TSK-C2", Type: "task", Parent: "STORY-1", Scope: []string{"cmd/armature/c2.go (new)"}},
		&materialize.Issue{ID: "TSK-C3", Type: "task", Parent: "STORY-1", Scope: []string{"cmd/armature/c3.go (new)"}},
		&materialize.Issue{ID: "TSK-D1", Type: "task", Parent: "STORY-1", Scope: []string{"docs/commands.md"}},
		&materialize.Issue{ID: "TSK-D2", Type: "task", Parent: "STORY-1", Scope: []string{"docs/design/surface-census.md"}},
	)
	result := Validate(state, graphFromState(state), Options{})

	var e13 []string
	for _, err := range result.Errors {
		if strings.Contains(err, "E13") {
			e13 = append(e13, err)
		}
	}
	assert.Len(t, e13, 3, "one finding per offending code task, not the 3x2 cross product, got: %v", e13)
	for _, msg := range e13 {
		assert.Contains(t, msg, "TSK-D1", "finding should cite every implicated sibling, got: %s", msg)
		assert.Contains(t, msg, "TSK-D2", "finding should cite every implicated sibling, got: %s", msg)
	}
}

// TestCensusedSurfacesMatchesCensusDoc is the drift gate for E13's own census
// copy. docs/design/surface-census.md is authoritative; censusedSurfaces restates
// it. Without this test, adding a surface to the census silently stops E13 from
// covering it -- a false negative, the failure mode a gate never announces.
func TestCensusedSurfacesMatchesCensusDoc_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "design", "surface-census.md"))
	require.NoError(t, err)

	fromDoc := parseCensusedSurfaceTable(t, string(doc))
	require.NotEmpty(t, fromDoc, "no Censused Surfaces table found in docs/design/surface-census.md")
	assert.Equal(t, fromDoc, censusedSurfaces,
		"censusedSurfaces has drifted from the Censused Surfaces table in docs/design/surface-census.md")
}

// parseCensusedSurfaceTable reads the "## Censused Surfaces" markdown table,
// whose rows are | `<surface glob>` | `<doc file>`, `<doc file>` | <notes> |.
func parseCensusedSurfaceTable(t *testing.T, doc string) map[string][]string {
	t.Helper()
	out := make(map[string][]string)
	inSection := false
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = trimmed == "## Censused Surfaces"
			continue
		}
		if !inSection || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		if len(cells) < 2 {
			continue
		}
		surface := strings.Trim(strings.TrimSpace(cells[0]), "`")
		if surface == "" || surface == "Surface" || strings.HasPrefix(surface, "---") {
			continue
		}
		var docFiles []string
		for _, f := range strings.Split(cells[1], ",") {
			if cleaned := strings.Trim(strings.TrimSpace(f), "`"); cleaned != "" {
				docFiles = append(docFiles, cleaned)
			}
		}
		out[surface] = docFiles
	}
	return out
}

// TestCheckIntroductionDoesNotBlockOnE13 asserts E13 is a plan-release gate, not a
// write-time refusal. A planner decomposes a story one task at a time and the graph
// is transiently ill-shaped between writes; refusing the create forbids ever
// reaching the intermediate state. E13 still fails Validate for arm validate and
// arm dag transition --to verified.
func TestCheckIntroductionDoesNotBlockOnE13_REQ_LNGHZN_S10_T5(t *testing.T) {
	t.Parallel()
	current := makeState(
		&materialize.Issue{
			ID:               "STORY-1",
			Type:             "story",
			DefinitionOfDone: "Deliver the vertical slice end to end, covering implementation and documentation.",
		},
		&materialize.Issue{
			ID:               "TSK-CODE",
			Type:             "task",
			Parent:           "STORY-1",
			Scope:            []string{"cmd/armature/flag.go (new)"},
			DefinitionOfDone: "The new flag is implemented and wired up.",
			Acceptance:       json.RawMessage(`["flag works as documented"]`),
		},
	)
	proposed := []ops.Op{{
		Type:     ops.OpCreate,
		TargetID: "TSK-DOCS",
		Payload: ops.Payload{
			NodeType:         "task",
			Parent:           "STORY-1",
			Scope:            []string{"docs/commands.md"},
			DefinitionOfDone: "docs/commands.md documents the new flag.",
			Acceptance:       json.RawMessage(`["docs/commands.md mentions the new flag"]`),
		},
	}}

	err := CheckIntroduction(current, proposed, Options{Strict: true})
	assert.NoError(t, err, "E13 is a plan-release gate, not a write-time refusal")
}
