package validate

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/scopematch"
	"github.com/scullxbones/armature/internal/traceability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func globOverlaps(a, b string) bool {
	return scopematch.Overlaps(a, b)
}

func makeState(issues ...*materialize.Issue) *materialize.State {
	s := materialize.NewState()
	for _, issue := range issues {
		s.Issues[issue.ID] = issue
	}
	return s
}

func graphFromState(state *materialize.State) *dag.Graph {
	return materialize.GraphFromState(state)
}

func introducedFindings(before, after Result, targeted []string) []Finding {
	prior := make(map[string]struct{}, len(before.Findings))
	for _, f := range before.Findings {
		prior[f.identity()] = struct{}{}
	}
	return introducedOnTargets(before, after, prior, targeted)
}

func TestValidate_Clean(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{}, Children: []string{}},
		&materialize.Issue{ID: "B", BlockedBy: []string{}, Children: []string{}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, result.OK)
	assert.Nil(t, result.Errors)
}

func TestValidate_OrphanedChild(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "A", Parent: "nonexistent", BlockedBy: []string{}, Children: []string{}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, result.OK)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "unresolved parent")
}

func TestValidate_CircularDep(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{"B"}, Children: []string{}},
		&materialize.Issue{ID: "B", BlockedBy: []string{"A"}, Children: []string{}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, result.OK)

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "cycle detected") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected cycle detected error, got: %v", result.Errors)
}

func TestValidate_UnknownBlocker(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{"ghost"}, Children: []string{}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, result.OK)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "unresolved link target")
}

func containsWarning(r Result, substr string) bool {
	for _, w := range r.Warnings {
		if strings.Contains(strings.ToLower(w), strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

func containsError(r Result, substr string) bool {
	for _, e := range r.Errors {
		if strings.Contains(strings.ToLower(e), strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

func containsPhantomScopeInfo(r Result) bool {
	for _, i := range r.Infos {
		if strings.Contains(strings.ToLower(i), "phantom scope") {
			return true
		}
	}
	return false
}

func TestW1ScopeOverlap(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "scope overlap"))
}

func TestW1ScopeOverlap_SuppressedByBlockedBy(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, Blocks: []string{"TSK-B"}},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, BlockedBy: []string{"TSK-A"}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, containsWarning(result, "scope overlap"), "scope overlap should be suppressed when one sibling blocks the other")
}

func TestW1ScopeOverlap_SkipsTerminalTasks(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, Status: "done"},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, Status: "merged"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, containsWarning(result, "scope overlap"), "terminal sibling tasks should not trigger scope overlap warnings")
}

func TestW1ScopeOverlap_SkipsNonTaskIssues(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "STORY-A", Type: "story", Parent: "EPIC-1", Scope: []string{"internal/ops/*.go"}},
		&materialize.Issue{ID: "STORY-B", Type: "story", Parent: "EPIC-1", Scope: []string{"internal/ops/*.go"}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "scope overlap"), "sibling ready-eligible stories with overlapping scope must emit W1")
}

func TestW1ReportsOverlapBetweenBugAndTask_REQ_W1TYPE_1(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Scope: []string{"cmd/armature/*.go"}},
		&materialize.Issue{ID: "BUG-1", Type: "bug", Scope: []string{"cmd/armature/bootstrap.go", "cmd/armature/bootstrap_test.go"}},
	)
	result := Validate(state, graphFromState(state), Options{})
	require.True(t, containsWarning(result, "scope overlap"), "a bug overlapping a live task must emit W1")
	var cited []string
	for _, f := range result.Findings {
		if f.Rule == "W1" {
			cited = f.CitedIDs
			break
		}
	}
	assert.Contains(t, cited, "TSK-1")
	assert.Contains(t, cited, "BUG-1")
}

func TestW1SuppressesParentChildScopeUnion_REQ_W1TYPE_1(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:       "STORY-1",
			Type:     "story",
			Scope:    []string{"internal/ops/*.go"},
			Children: []string{"TSK-1"},
		},
		&materialize.Issue{
			ID:     "TSK-1",
			Type:   "task",
			Parent: "STORY-1",
			Scope:  []string{"internal/ops/*.go"},
		},
	)
	result := Validate(state, graphFromState(state), Options{})
	assert.False(t, containsWarning(result, "scope overlap"),
		"a parent story's scope is the union of its children's and must not emit W1")
}

func TestW1IgnoresTerminalAndEpicIssues_REQ_W1TYPE_1(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-LIVE", Type: "task", Scope: []string{"internal/ops/*.go"}},
		&materialize.Issue{ID: "BUG-DONE", Type: "bug", Status: "done", Scope: []string{"internal/ops/*.go"}},
		&materialize.Issue{ID: "FEAT-MERGED", Type: "feature", Status: "merged", Scope: []string{"internal/ops/*.go"}},
		&materialize.Issue{ID: "STORY-CANCELLED", Type: "story", Status: "cancelled", Scope: []string{"internal/ops/*.go"}},
		&materialize.Issue{ID: "EPIC-1", Type: "epic", Scope: []string{"internal/ops/*.go"}},
	)
	result := Validate(state, graphFromState(state), Options{})
	assert.False(t, containsWarning(result, "scope overlap"),
		"terminal issues and type=epic must not participate in W1 even when scopes overlap")
}

func TestW1ExcludesPassiveAggregateParents_REQ_W1TYPE_1(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{
			ID:       "STORY-ROLLUP",
			Type:     "story",
			Status:   "in_progress",
			Scope:    []string{"cmd/armature/claim.go"},
			Children: []string{"TSK-DONE"},
		},
		&materialize.Issue{
			ID:     "TSK-DONE",
			Type:   "task",
			Parent: "STORY-ROLLUP",
			Status: "done",
			Scope:  []string{"cmd/armature/claim.go"},
		},
		&materialize.Issue{
			ID:    "TSK-NEW",
			Type:  "task",
			Scope: []string{"cmd/armature/claim.go"},
		},
	)
	result := Validate(state, graphFromState(state), Options{})
	assert.False(t, containsWarning(result, "scope overlap"),
		"passive aggregate story must not W1 against an unrelated live task on rolled-up files")
}

const w1ClaimNow int64 = 1_700_000_000

func TestW1KeepsExplicitlyClaimedAggregateParent_REQ_W1TYPE_1(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{
			ID:            "STORY-CLAIMED",
			Type:          "story",
			Status:        ops.StatusClaimed,
			ClaimedBy:     "worker-a",
			ClaimedAt:     w1ClaimNow - 60,
			LastHeartbeat: w1ClaimNow - 60,
			ClaimTTL:      3600,
			Scope:         []string{"cmd/armature/claim.go"},
			Children:      []string{"TSK-DONE"},
		},
		&materialize.Issue{
			ID:     "TSK-DONE",
			Type:   "task",
			Parent: "STORY-CLAIMED",
			Status: "done",
			Scope:  []string{"cmd/armature/claim.go"},
		},
		&materialize.Issue{
			ID:    "TSK-NEW",
			Type:  "task",
			Scope: []string{"cmd/armature/claim.go"},
		},
	)
	result := Validate(state, graphFromState(state), Options{Now: w1ClaimNow})
	require.True(t, containsWarning(result, "scope overlap"),
		"an explicitly claimed story must still W1 against an unrelated live task")
	var cited []string
	for _, f := range result.Findings {
		if f.Rule == "W1" {
			cited = f.CitedIDs
			break
		}
	}
	assert.Contains(t, cited, "STORY-CLAIMED")
	assert.Contains(t, cited, "TSK-NEW")
}

func TestW1InProgressRollupParentWithoutClaimantStaysPassive_REQ_W1TYPE_1(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:       "STORY-PROMOTED",
			Type:     "story",
			Status:   ops.StatusInProgress,
			Scope:    []string{"cmd/armature/claim.go"},
			Children: []string{"TSK-DONE"},
		},
		&materialize.Issue{
			ID:     "TSK-DONE",
			Type:   "task",
			Parent: "STORY-PROMOTED",
			Status: "done",
			Scope:  []string{"cmd/armature/claim.go"},
		},
		&materialize.Issue{
			ID:    "TSK-NEW",
			Type:  "task",
			Scope: []string{"cmd/armature/claim.go"},
		},
	)
	result := Validate(state, graphFromState(state), Options{})
	assert.False(t, containsWarning(result, "scope overlap"),
		"a rollup parent promoted to in-progress without a claimant must stay out of W1")
}

func TestW1ExcludesExpiredAggregateClaim_REQ_W1TYPE_1(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:            "STORY-EXPIRED",
			Type:          "story",
			Status:        ops.StatusClaimed,
			ClaimedBy:     "worker-a",
			ClaimedAt:     w1ClaimNow - 7200,
			LastHeartbeat: w1ClaimNow - 7200,
			ClaimTTL:      60,
			Scope:         []string{"cmd/armature/claim.go"},
			Children:      []string{"TSK-DONE"},
		},
		&materialize.Issue{
			ID:     "TSK-DONE",
			Type:   "task",
			Parent: "STORY-EXPIRED",
			Status: "done",
			Scope:  []string{"cmd/armature/claim.go"},
		},
		&materialize.Issue{
			ID:    "TSK-NEW",
			Type:  "task",
			Scope: []string{"cmd/armature/claim.go"},
		},
	)
	result := Validate(state, graphFromState(state), Options{Now: w1ClaimNow})
	assert.False(t, containsWarning(result, "scope overlap"),
		"an aggregate parent whose claim outlived its TTL must fall back to passive")
}

func TestW1ActiveChildStillCompetesUnderAggregateParent_REQ_W1TYPE_1(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:       "STORY-1",
			Type:     "story",
			Scope:    []string{"internal/ops/*.go"},
			Children: []string{"TSK-LIVE"},
		},
		&materialize.Issue{
			ID:     "TSK-LIVE",
			Type:   "task",
			Parent: "STORY-1",
			Scope:  []string{"internal/ops/*.go"},
		},
		&materialize.Issue{
			ID:    "TSK-OTHER",
			Type:  "task",
			Scope: []string{"internal/ops/*.go"},
		},
	)
	result := Validate(state, graphFromState(state), Options{})
	require.True(t, containsWarning(result, "scope overlap"),
		"active child under an aggregate story must still W1 against an unrelated live task")
	var cited []string
	for _, f := range result.Findings {
		if f.Rule == "W1" {
			cited = f.CitedIDs
			break
		}
	}
	assert.Contains(t, cited, "TSK-LIVE")
	assert.Contains(t, cited, "TSK-OTHER")
	assert.NotContains(t, cited, "STORY-1")
}

func TestW2NoTestCriteria(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Acceptance: json.RawMessage(`[{"type":"review","text":"look at it"}]`),
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "no test criteria"))
}

func TestW2NoTestCriteria_ManualReviewSatisfies(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Acceptance: json.RawMessage(`[{"type":"manual_review","description":"docs reviewed"}]`),
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, containsWarning(result, "no test criteria"), "manual_review should satisfy test criteria requirement")
}

func TestW7VagueDoD(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", DefinitionOfDone: "Make it work properly and correctly"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "vague dod"))
}

func TestW8ConflictingDecisions(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Decisions: []materialize.Decision{
				{Topic: "storage", Choice: "postgres"},
				{Topic: "storage", Choice: "sqlite"},
			},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "conflicting decisions"))
}

func TestW8ConflictingDecisions_IgnoresDuplicateChoices(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Decisions: []materialize.Decision{
				{Topic: "storage", Choice: "postgres"},
				{Topic: "storage", Choice: "postgres"},
				{Topic: "storage", Choice: "postgres"},
			},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, containsWarning(result, "conflicting decisions"), "repeating the same choice should not trigger a conflict warning")
}

func TestW11VagueOutcome(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done", Outcome: "done"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "vague outcome"))
}

func TestE5TypeHierarchy(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TASK-1", Type: "task", Children: []string{"TASK-2"}},
		&materialize.Issue{ID: "TASK-2", Type: "task", Parent: "TASK-1"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsError(result, "invalid hierarchy"))
}

func TestE6RequiredFields(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, result.OK)
	assert.True(t, containsError(result, "missing required field"))
}

func TestE6RequiredFields_SkipsMergedTask(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "merged"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, result.OK)
	assert.False(t, containsError(result, "missing required field"))
}

func TestE6RequiredFields_SkipsDoneTask(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, result.OK)
	assert.False(t, containsError(result, "missing required field"))
}

func TestE6RequiredFields_SkipsCancelledTask(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "cancelled"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, result.OK)
	assert.False(t, containsError(result, "missing required field"))
}

func TestE5TypeHierarchy_EpicWithTaskIsValid(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "EPIC-1", Type: "epic", Children: []string{"TASK-2"}},
		&materialize.Issue{ID: "TASK-2", Type: "task", Parent: "EPIC-1"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, containsError(result, "invalid hierarchy"), "epic with task child should be valid")
}

func TestW1ScopeOverlap_SuppressedWhenBBlocksA(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, BlockedBy: []string{"TSK-B"}},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, Blocks: []string{"TSK-A"}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, containsWarning(result, "scope overlap"), "scope overlap should be suppressed when B blocks A")
}

func TestCheckW1ScopeOverlap_SuppressesTransitivelyOrderedPairs_REQ_TOPTIER_S17_T2(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{
			ID:     "TSK-A",
			Type:   "task",
			Parent: "STORY-1",
			Scope:  []string{"internal/ops/*.go"},
			Blocks: []string{"TSK-B"},
		},
		&materialize.Issue{
			ID:        "TSK-B",
			Type:      "task",
			Parent:    "STORY-1",
			Scope:     []string{"internal/other/*.go"},
			BlockedBy: []string{"TSK-A"},
			Blocks:    []string{"TSK-C"},
		},
		&materialize.Issue{
			ID:        "TSK-C",
			Type:      "task",
			Parent:    "STORY-1",
			Scope:     []string{"internal/ops/*.go"},
			BlockedBy: []string{"TSK-B"},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})

	assert.False(t, containsWarning(result, "scope overlap"),
		"scope overlap should be suppressed when tasks are transitively ordered via blocked_by chain")
}

func TestW3BudgetExceeded_WithLargeContext(t *testing.T) {
	t.Parallel()

	largeContext := make([]byte, 20000)
	for i := range largeContext {
		largeContext[i] = 'x'
	}
	jsonContext := append([]byte(`"`), append(largeContext, '"')...)
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Context: json.RawMessage(jsonContext)},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "budget advisory"))
}

func TestW6ComplexityMismatch_SmallWith6Files(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Scope:         []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"},
			EstComplexity: "small",
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "complexity mismatch"))
}

func TestW6ComplexityMismatch_LargeWith1File(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Scope:         []string{"a.go"},
			EstComplexity: "large",
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "complexity mismatch"))
}

func TestW11VagueOutcome_ExactVagueWord(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done", Outcome: "done"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "vague outcome"))
}

func TestW5MissingContextFiles_TerminalStatusesSkipped(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"merged", "done", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			state := makeState(&materialize.Issue{
				ID:     "ISSUE-1",
				Type:   "task",
				Status: status,
				Scope: []string{
					"pkg/a/foo.go",
					"pkg/b/bar.go",
					"pkg/c/baz.go",
				},
			})
			graph := graphFromState(state)
			result := Validate(state, graph, Options{})
			assert.False(t, containsWarning(result, "missing context_files"),
				"status=%q: terminal issues should not warn about missing context_files", status)
		})
	}
}

func TestW5MissingContextFiles_ActiveIssueStillWarns(t *testing.T) {
	t.Parallel()
	state := makeState(&materialize.Issue{
		ID:     "ISSUE-1",
		Type:   "task",
		Status: "open",
		Scope:  []string{"pkg/a/foo.go", "pkg/b/bar.go", "pkg/c/baz.go"},
	})
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "missing context_files"),
		"active issues spanning 3+ dirs without context_files should still warn")

	for _, w := range result.Warnings {
		if strings.Contains(w, "missing context_files") {
			assert.NotContains(t, w, "--context-files",
				"W5 warning must not reference non-existent --context-files flag")
			assert.True(t,
				strings.Contains(w, "arm amend") && strings.Contains(w, "--scope"),
				"W5 warning should direct user to arm amend --scope or split the task: %s", w)
		}
	}
}

func TestW5MissingContextFiles_SkipsNonTasks_REQ_LNGHZN_S10_T4(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"story", "epic", "feature"} {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			state := makeState(&materialize.Issue{
				ID:     "ISSUE-1",
				Type:   typ,
				Status: "open",
				Scope:  []string{"pkg/a/foo.go", "pkg/b/bar.go", "pkg/c/baz.go"},
			})
			graph := graphFromState(state)
			result := Validate(state, graph, Options{})
			assert.False(t, containsWarning(result, "missing context_files"),
				"type=%q: container types with multi-dir scope are intentional", typ)
		})
	}
}

func TestW5MissingContextFiles_NonContainerTypesStillWarn_REQ_LNGHZN_S10_T4(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"bug", "", "spike"} {
		t.Run("type="+typ, func(t *testing.T) {
			t.Parallel()
			state := makeState(&materialize.Issue{
				ID:     "ISSUE-1",
				Type:   typ,
				Status: "open",
				Scope:  []string{"pkg/a/foo.go", "pkg/b/bar.go", "pkg/c/baz.go"},
			})
			graph := graphFromState(state)
			result := Validate(state, graph, Options{})
			assert.True(t, containsWarning(result, "missing context_files"),
				"type=%q: non-container types with 3-dir scope must still fire W5", typ)
		})
	}
}

func TestW10PhantomScope_TerminalStatusesSkipped(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"merged", "done", "cancelled"} {
		state := makeState(
			&materialize.Issue{
				ID:     "TSK-1",
				Type:   "task",
				Status: status,
				Scope:  []string{"nonexistent/path/*.go"},
			},
		)

		graph := graphFromState(state)
		result := Validate(state, graph, Options{PreExpandedScopes: nil})
		assert.False(t, containsPhantomScopeInfo(result),
			"status=%s: phantom scope should be skipped for terminal status", status)
	}
}

func TestW10PhantomScope_BlockedStillChecked(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{
			ID:     "TSK-1",
			Type:   "task",
			Status: "blocked",
			Scope:  []string{"nonexistent/path/*.go"},
		},
	)

	preExpandedScopes := map[string][]string{
		"TSK-1": {},
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	assert.True(t, containsPhantomScopeInfo(result),
		"blocked status should still trigger phantom scope warning")
}

func TestW10PhantomScope_EpicsAndStoriesWithTerminalStatusSkipped(t *testing.T) {
	t.Parallel()

	for _, issueType := range []string{"epic", "story"} {
		state := makeState(
			&materialize.Issue{
				ID:     "ISSUE-1",
				Type:   issueType,
				Status: "done",
				Scope:  []string{"nonexistent/path/*.go"},
			},
		)

		graph := graphFromState(state)
		result := Validate(state, graph, Options{PreExpandedScopes: nil})
		assert.False(t, containsPhantomScopeInfo(result),
			"type=%s status=done: phantom scope should be skipped for terminal status", issueType)
	}
}

func TestW10PhantomScope_NewSuffixSkipped(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{
			ID:     "ISSUE-1",
			Type:   "task",
			Status: "open",
			Scope:  []string{"internal/adapters/files.go (new)", "internal/adapters/git.go (new)"},
		},
	)

	preExpandedScopes := map[string][]string{
		"ISSUE-1": {},
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	assert.False(t, containsPhantomScopeInfo(result),
		"scope entries with (new) suffix should not trigger phantom scope warnings")
}

func TestW10PhantomScope_NewSuffixMixedWithExisting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	realFile := filepath.Join(dir, "real.go")
	require.NoError(t, os.WriteFile(realFile, []byte("package x\n"), 0644))

	state := makeState(
		&materialize.Issue{
			ID:     "ISSUE-1",
			Type:   "task",
			Status: "open",
			Scope:  []string{"real.go", "planned.go (new)", "ghost.go"},
		},
	)

	preExpandedScopes := map[string][]string{
		"ISSUE-1": {"real.go"},
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})

	assert.True(t, containsPhantomScopeInfo(result),
		"nonexistent file without (new) suffix should still trigger phantom scope warning")

	var phantomInfos []string
	for _, info := range result.Infos {
		if strings.Contains(info, "phantom scope") {
			phantomInfos = append(phantomInfos, info)
		}
	}
	assert.Len(t, phantomInfos, 1)
	assert.Contains(t, phantomInfos[0], "ghost.go")
	assert.NotContains(t, phantomInfos[0], "planned.go")
}

func TestW10PhantomScope_CommaSeparatedLegacyEntry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.go")
	require.NoError(t, os.WriteFile(realFile, []byte("package x\n"), 0644))

	state := makeState(
		&materialize.Issue{
			ID:     "ISSUE-1",
			Type:   "task",
			Status: "open",

			Scope: []string{"planned.go (new), real.go, ghost.go"},
		},
	)
	preExpandedScopes := map[string][]string{
		"ISSUE-1": {"real.go"},
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	var phantomInfos []string
	for _, info := range result.Infos {
		if strings.Contains(info, "phantom scope") {
			phantomInfos = append(phantomInfos, info)
		}
	}

	assert.Len(t, phantomInfos, 1)
	assert.Contains(t, phantomInfos[0], "ghost.go")
	assert.NotContains(t, phantomInfos[0], "planned.go")
	assert.NotContains(t, phantomInfos[0], "real.go")
}

func TestValidateUsesCoverage(t *testing.T) {
	t.Parallel()

	coverage := &traceability.Coverage{
		CitedNodes:  1,
		TotalNodes:  1,
		CoveragePct: 100,
	}

	state := makeState(&materialize.Issue{ID: "A"})
	graph := graphFromState(state)
	result := Validate(state, graph, Options{Coverage: coverage})
	assert.NotNil(t, result.Coverage)
	assert.Equal(t, 1, result.Coverage.CitedNodes)
}

func TestE5TypeHierarchy_SkipsTerminalStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"cancelled", "done", "merged"} {
		t.Run("status="+status, func(t *testing.T) {
			t.Parallel()

			state := makeState(
				&materialize.Issue{ID: "TASK-1", Type: "task", Status: status, Children: []string{"TASK-2"}},
				&materialize.Issue{ID: "TASK-2", Type: "task", Parent: "TASK-1"},
			)
			graph := graphFromState(state)
			result := Validate(state, graph, Options{})
			assert.False(t, containsError(result, "invalid hierarchy"),
				"terminal parent (status=%s) must not trigger hierarchy error", status)
		})
	}
}

func TestE5TypeHierarchy_SkipsTerminalChildren(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"cancelled", "done", "merged"} {
		t.Run("status="+status, func(t *testing.T) {
			t.Parallel()

			state := makeState(
				&materialize.Issue{ID: "TASK-1", Type: "task", Children: []string{"BUG-1"}},
				&materialize.Issue{ID: "BUG-1", Type: "bug", Parent: "TASK-1", Status: status},
			)
			graph := graphFromState(state)
			result := Validate(state, graph, Options{})
			assert.False(t, containsError(result, "invalid hierarchy"),
				"terminal child (status=%s) must not trigger hierarchy error", status)
		})
	}
}

func TestE5TypeHierarchy_BugUnderStoryIsValid(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story", Children: []string{"BUG-1"}},
		&materialize.Issue{ID: "BUG-1", Type: "bug", Parent: "STORY-1"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, containsError(result, "invalid hierarchy"), "bug under story should be valid")
}

func TestE5TypeHierarchy_BugUnderEpicIsValid(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "EPIC-1", Type: "epic", Children: []string{"BUG-1"}},
		&materialize.Issue{ID: "BUG-1", Type: "bug", Parent: "EPIC-1"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, containsError(result, "invalid hierarchy"), "bug under epic should be valid")
}

func TestE5TypeHierarchy_BugUnderTaskIsInvalid(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TASK-1", Type: "task", Children: []string{"BUG-1"}},
		&materialize.Issue{ID: "BUG-1", Type: "bug", Parent: "TASK-1"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsError(result, "invalid hierarchy"), "bug under task should be invalid")
}

func TestCheckE4Cycles_CrossScopeBlockerCycle(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{
			ID:        "A",
			Type:      "task",
			Status:    "open",
			BlockedBy: []string{"B"},
		},
		&materialize.Issue{
			ID:        "B",
			Type:      "task",
			Status:    "open",
			BlockedBy: []string{"A"},
		},
	)

	graph := graphFromState(state)

	scope := map[string]bool{
		"A": true,
	}

	result := graph.ScopedHasCycle("A", scope)

	assert.True(t, result, "expected ScopedHasCycle to detect cross-scope blocker cycle")
}

func TestCheckE4Cycles_OutOfScopeCycleIsNotFalsePositive(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:        "A",
			Type:      "task",
			Status:    "open",
			BlockedBy: []string{"B"},
		},
		&materialize.Issue{
			ID:        "B",
			Type:      "task",
			Status:    "open",
			BlockedBy: []string{"C"},
		},
		&materialize.Issue{
			ID:        "C",
			Type:      "task",
			Status:    "open",
			BlockedBy: []string{"B"},
		},
	)

	graph := graphFromState(state)

	scope := map[string]bool{
		"A": true,
	}

	result := graph.ScopedHasCycle("A", scope)
	assert.False(t, result, "out-of-scope cycle B→C→B must not be reported as a cycle for scoped node A")
}

func TestE9DoDLength_Exceeds500Chars(t *testing.T) {
	t.Parallel()
	longDoD := string(make([]byte, 501))
	state := makeState(&materialize.Issue{
		ID:               "task-01",
		Type:             "task",
		Status:           "open",
		DefinitionOfDone: longDoD,
		BlockedBy:        []string{},
		Children:         []string{},
	})
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	hasErr := false
	for _, e := range result.Errors {
		if strings.Contains(e, "definition_of_done exceeds") {
			hasErr = true
			break
		}
	}
	assert.True(t, hasErr, "expected definition_of_done length error for 501-char DoD")
}

func TestE9DoDLength_ExactlyAtLimit_NoError(t *testing.T) {
	t.Parallel()
	doD := string(make([]byte, 500))
	state := makeState(&materialize.Issue{
		ID:               "task-01",
		Type:             "task",
		Status:           "open",
		DefinitionOfDone: doD,
		BlockedBy:        []string{},
		Children:         []string{},
	})
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	for _, e := range result.Errors {
		assert.NotContains(t, e, "definition_of_done exceeds")
	}
}

func TestE10ScopeGlobs_InvalidGlob_EmitsError(t *testing.T) {
	t.Parallel()
	state := makeState(&materialize.Issue{
		ID:        "task-01",
		Type:      "task",
		Status:    "open",
		Scope:     []string{"[invalid"},
		BlockedBy: []string{},
		Children:  []string{},
	})
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	hasErr := false
	for _, e := range result.Errors {
		if strings.Contains(e, "invalid glob") {
			hasErr = true
			break
		}
	}
	assert.True(t, hasErr, "expected invalid glob error")
}

func TestW4BroadScope_DoubleStarScope(t *testing.T) {
	t.Parallel()
	state := makeState(&materialize.Issue{
		ID:        "task-01",
		Type:      "task",
		Status:    "open",
		Scope:     []string{"**"},
		BlockedBy: []string{},
		Children:  []string{},
	})
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	hasWarn := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "broad scope") {
			hasWarn = true
			break
		}
	}
	assert.True(t, hasWarn, "expected broad scope warning for ** glob")
}

func TestW4BroadScope_DotScope(t *testing.T) {
	t.Parallel()
	state := makeState(&materialize.Issue{
		ID:        "task-01",
		Type:      "task",
		Status:    "open",
		Scope:     []string{"."},
		BlockedBy: []string{},
		Children:  []string{},
	})
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	hasWarn := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "broad scope") {
			hasWarn = true
			break
		}
	}
	assert.True(t, hasWarn, "expected broad scope warning for . glob")
}

func TestW4BroadScope_SkipsTerminalStatus(t *testing.T) {
	t.Parallel()
	state := makeState(&materialize.Issue{
		ID:        "task-01",
		Type:      "task",
		Status:    ops.StatusDone,
		Scope:     []string{"**/*"},
		BlockedBy: []string{},
		Children:  []string{},
	})
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	for _, w := range result.Warnings {
		assert.NotContains(t, w, "broad scope", "done tasks must not produce broad scope warning")
	}
}

func TestCheckW1ScopeOverlap_FlagsCrossStoryOverlap_REQ_TOPTIER_S17_T3(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story"},
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}},
		&materialize.Issue{ID: "STORY-2", Type: "story"},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-2", Scope: []string{"internal/ops/*.go"}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "scope overlap"),
		"cross-story tasks with overlapping scope and no ordering edge should produce scope overlap warning")
}

func TestCheckW1ScopeOverlap_SuppressesCrossStoryWhenOrdered_REQ_TOPTIER_S17_T3(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story"},
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, Blocks: []string{"TSK-B"}},
		&materialize.Issue{ID: "STORY-2", Type: "story"},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-2", Scope: []string{"internal/ops/*.go"}, BlockedBy: []string{"TSK-A"}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, containsWarning(result, "scope overlap"),
		"cross-story tasks with overlapping scope but with ordering edge should not warn")
}

func TestCheckW10PhantomScope_SuppressesForBlockerCreatedFiles_REQ_TOPTIER_S17_T4(t *testing.T) {
	t.Parallel()

	state := makeState(

		&materialize.Issue{
			ID:     "TSK-BLOCKER",
			Type:   "task",
			Status: "open",
			Scope:  []string{"internal/new_file.go (new)"},
			Blocks: []string{"TSK-DOWNSTREAM"},
		},

		&materialize.Issue{
			ID:        "TSK-DOWNSTREAM",
			Type:      "task",
			Status:    "open",
			Scope:     []string{"internal/new_file.go"},
			BlockedBy: []string{"TSK-BLOCKER"},
		},
	)

	preExpandedScopes := map[string][]string{
		"TSK-BLOCKER":    {},
		"TSK-DOWNSTREAM": {},
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})

	assert.False(t, containsPhantomScopeInfo(result),
		"phantom scope should be suppressed when a blocking task declares the file with (new) suffix")
}

func TestCheckW1ScopeOverlap_ScopedSubsetSuppressesTransitiveChainThroughOutOfScopeIssue(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{ID: "EPIC-1", Type: "epic", Children: []string{"STORY-AC", "STORY-B"}},
		&materialize.Issue{ID: "STORY-AC", Type: "story", Parent: "EPIC-1", Children: []string{"TSK-A", "TSK-C"}},
		&materialize.Issue{ID: "STORY-B", Type: "story", Parent: "EPIC-1", Children: []string{"TSK-B"}},
		&materialize.Issue{
			ID:     "TSK-A",
			Type:   "task",
			Parent: "STORY-AC",
			Scope:  []string{"internal/ops/*.go"},
			Blocks: []string{"TSK-B"},
		},
		&materialize.Issue{
			ID:        "TSK-B",
			Type:      "task",
			Parent:    "STORY-B",
			Scope:     []string{"internal/other/*.go"},
			BlockedBy: []string{"TSK-A"},
			Blocks:    []string{"TSK-C"},
		},
		&materialize.Issue{
			ID:        "TSK-C",
			Type:      "task",
			Parent:    "STORY-AC",
			Scope:     []string{"internal/ops/*.go"},
			BlockedBy: []string{"TSK-B"},
		},
	)

	scoped := map[string]*materialize.Issue{
		"STORY-AC": state.Issues["STORY-AC"],
		"TSK-A":    state.Issues["TSK-A"],
		"TSK-C":    state.Issues["TSK-C"],
	}
	require.NotContains(t, scoped, "TSK-B", "test setup: TSK-B must be outside the scoped subset")

	warns := checkW1ScopeOverlap(scoped, state, graphFromState(state), w1ClaimNow)
	for _, w := range warns {
		assert.NotContains(t, w, "scope overlap",
			"scope overlap should be suppressed when the transitive blocked_by chain passes through an out-of-scope issue: %s", w)
	}
}

func TestDirectBlocks_DoesNotMaterializeTransitiveClosure(t *testing.T) {
	t.Parallel()
	state := makeState(

		&materialize.Issue{ID: "TSK-A"},
		&materialize.Issue{ID: "TSK-B", BlockedBy: []string{"TSK-A"}, Blocks: []string{"TSK-C"}},
		&materialize.Issue{ID: "TSK-C"},
	)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("UNRELATED-%d", i)
		state.Issues[id] = &materialize.Issue{ID: id}
	}

	blocks := directBlocks(state.Issues)
	assert.Equal(t, []string{"TSK-B"}, blocks["TSK-A"])
	assert.Equal(t, []string{"TSK-C"}, blocks["TSK-B"])
	assert.NotContains(t, blocks["TSK-A"], "TSK-C", "index must contain only direct edges")
	assert.Len(t, blocks, 2, "unrelated issues must not allocate closure entries")
	assert.True(t, blocksReachable("TSK-A", "TSK-C", blocks), "candidate traversal must still find transitive ordering")
}

func TestCheckW10PhantomScope_SuppressesForTwoHopBlockerCreatedFiles(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{
			ID:     "TSK-BLOCKER",
			Type:   "task",
			Status: "open",
			Scope:  []string{"internal/new_file.go (new)"},
			Blocks: []string{"TSK-MID"},
		},
		&materialize.Issue{
			ID:        "TSK-MID",
			Type:      "task",
			Status:    "open",
			Scope:     []string{"internal/unrelated.go"},
			BlockedBy: []string{"TSK-BLOCKER"},
			Blocks:    []string{"TSK-DOWNSTREAM"},
		},
		&materialize.Issue{
			ID:        "TSK-DOWNSTREAM",
			Type:      "task",
			Status:    "open",
			Scope:     []string{"internal/new_file.go"},
			BlockedBy: []string{"TSK-MID"},
		},
	)
	preExpandedScopes := map[string][]string{
		"TSK-BLOCKER":    {},
		"TSK-MID":        {},
		"TSK-DOWNSTREAM": {},
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	for _, info := range result.Infos {
		assert.NotContains(t, info, "new_file.go",
			"phantom scope for new_file.go should be suppressed via the 2-hop blocked_by chain to TSK-BLOCKER: %s", info)
	}
}

func TestCheckW1ScopeOverlap_FlagsGlobAwareCrossStoryOverlap_PR79(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story"},
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"cmd/armature/*.go"}},
		&materialize.Issue{ID: "STORY-2", Type: "story"},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-2", Scope: []string{"cmd/armature/claim.go"}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "scope overlap"),
		"cross-story tasks whose scopes overlap only via glob-vs-literal matching should still produce a scope overlap warning")
}

func TestCheckW10PhantomScope_ConsultsFullStateForCrossSubtreeBlocker_PR79(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{ID: "EPIC-1", Type: "epic", Children: []string{"STORY-BLOCKER", "STORY-DOWNSTREAM"}},
		&materialize.Issue{ID: "STORY-BLOCKER", Type: "story", Parent: "EPIC-1", Children: []string{"TSK-BLOCKER"}},
		&materialize.Issue{ID: "STORY-DOWNSTREAM", Type: "story", Parent: "EPIC-1", Children: []string{"TSK-DOWNSTREAM"}},
		&materialize.Issue{
			ID:     "TSK-BLOCKER",
			Type:   "task",
			Parent: "STORY-BLOCKER",
			Status: "open",
			Scope:  []string{"internal/new_file.go (new)"},
			Blocks: []string{"TSK-DOWNSTREAM"},
		},
		&materialize.Issue{
			ID:        "TSK-DOWNSTREAM",
			Type:      "task",
			Parent:    "STORY-DOWNSTREAM",
			Status:    "open",
			Scope:     []string{"internal/new_file.go"},
			BlockedBy: []string{"TSK-BLOCKER"},
		},
	)

	targets := map[string]*materialize.Issue{
		"STORY-DOWNSTREAM": state.Issues["STORY-DOWNSTREAM"],
		"TSK-DOWNSTREAM":   state.Issues["TSK-DOWNSTREAM"],
	}
	require.NotContains(t, targets, "TSK-BLOCKER", "test setup: TSK-BLOCKER must be outside the scoped subset")

	preExpandedScopes := map[string][]string{
		"TSK-DOWNSTREAM": {},
	}
	warns := checkW10PhantomScope(targets, preExpandedScopes, state.Issues)
	for _, w := range warns {
		assert.NotContains(t, w, "new_file.go",
			"phantom scope should be suppressed when the blocker declaring (new) lives outside the scoped subset but exists in full state: %s", w)
	}
}

func TestGlobOverlaps_RespectsPathSegmentBoundaries_PR79(t *testing.T) {
	t.Parallel()

	assert.False(t, globOverlaps("internal/claimx/foo.go", "internal/claim/*.go"),
		"internal/claimx and internal/claim share a string prefix but are sibling directories, not nested — must not overlap")
	assert.False(t, globOverlaps("internal/claim/*.go", "internal/claimx/foo.go"),
		"overlap check must be symmetric")

	assert.False(t, globOverlaps("internal/claim/sub/*.go", "internal/claim/*.go"),
		"single-segment glob 'internal/claim/*.go' does not match the deeper literal directory 'sub/' — no longer treated as overlapping via directory ancestry")
	assert.False(t, globOverlaps("internal/claim/*.go", "internal/claim/sub/*.go"),
		"overlap check must be symmetric")

	assert.False(t, globOverlaps("internal/claim/a.go", "internal/claim/b.go"),
		"two distinct literal files that merely share a containing directory must not overlap")
}

func TestGlobOverlapsIgnoresSharedAncestorDirectory_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()

	assert.False(t, globOverlaps("docs/agents/quality-gates.md", "docs/use-cases.md"),
		"distinct files under an ancestor/descendant directory relationship must not overlap")
	assert.False(t, globOverlaps("docs/use-cases.md", "docs/agents/quality-gates.md"),
		"overlap check must be symmetric")

	assert.False(t, globOverlaps("internal/claim/overlap.go", "internal/claim/overlap_test.go"),
		"two distinct files in the same directory must not overlap merely by sharing that directory")
	assert.False(t, globOverlaps("internal/claim/overlap_test.go", "internal/claim/overlap.go"),
		"overlap check must be symmetric")
}

func TestGlobOverlapsStillMatchesIdenticalAndGlobScopes_REQ_LNGHZN_S10_T7(t *testing.T) {
	t.Parallel()

	assert.True(t, globOverlaps("README.md", "README.md"),
		"identical scope entries must still overlap")

	assert.True(t, globOverlaps("docs/agents/**", "docs/agents/quality-gates.md"),
		"an explicit directory glob must still overlap a file beneath it")
	assert.True(t, globOverlaps("docs/agents/quality-gates.md", "docs/agents/**"),
		"overlap check must be symmetric")
}

func TestFirstGlobOverlapPair_ReportsMatchedPatterns_PR79(t *testing.T) {
	t.Parallel()
	a, b, overlaps := firstGlobOverlapPair(
		[]string{"cmd/other/*.go", "cmd/armature/*.go"},
		[]string{"cmd/armature/claim.go", "cmd/unrelated/x.go"},
	)
	require.True(t, overlaps)
	assert.Equal(t, "cmd/armature/*.go", a)
	assert.Equal(t, "cmd/armature/claim.go", b)

	_, _, overlaps = firstGlobOverlapPair([]string{"a/*.go"}, []string{"b/*.go"})
	assert.False(t, overlaps)
}

func TestCheckW1ScopeOverlap_MessageReportsMatchedPatternPair_PR79(t *testing.T) {
	t.Parallel()

	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story"},
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"cmd/other/*.go", "cmd/armature/*.go"}},
		&materialize.Issue{ID: "STORY-2", Type: "story"},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-2", Scope: []string{"cmd/armature/claim.go", "cmd/unrelated/x.go"}},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})

	var msg string
	for _, w := range result.Warnings {
		if strings.Contains(w, "scope overlap") {
			msg = w
			break
		}
	}
	require.NotEmpty(t, msg, "expected a scope overlap warning")
	assert.Contains(t, msg, "cmd/armature/*.go")
	assert.Contains(t, msg, "cmd/armature/claim.go")
	assert.NotContains(t, msg, "cmd/other/*.go",
		"message should report only the matched pattern pair, not the full scope lists: %s", msg)
	assert.NotContains(t, msg, "cmd/unrelated/x.go",
		"message should report only the matched pattern pair, not the full scope lists: %s", msg)
}

func TestGlobOverlaps_ParityWithClaimPackage_PR79(t *testing.T) {
	t.Parallel()
	for _, c := range scopematch.OverlapParityCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.Want, globOverlaps(c.A, c.B), "globOverlaps(%q, %q)", c.A, c.B)
			assert.Equal(t, c.Want, globOverlaps(c.B, c.A), "globOverlaps(%q, %q) (symmetric)", c.B, c.A)
		})
	}
}

func TestAffectsValidityCensus_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	for _, typ := range materialize.RegisteredOpTypes() {
		_, classified := ops.ClassifiedValidity(typ)
		assert.True(t, classified, "unclassified op type %q: every RegisteredOpTypes() entry must be classified AffectsValidity", typ)
	}
}

var introductionWriterAllowlist = map[string]string{
	"cmd/armature/helpers.go":      "Introduction wrappers: refuseIntroduction then AppendAndCommit",
	"cmd/armature/harness_hook.go": "heartbeat only; OpHeartbeat is classified AffectsValidity=false",
	"internal/decompose/apply.go":  "CheckIntroduction on the plan batch, then one AppendOps write",
	"internal/decompose/revert.go": "exempt: recommended Introduction remedy; cancel withdraws drafts",
	"internal/doctor/fix.go":       "exempt: recovery compensating transitions must remain landable",
}

func TestIntroductionWritersAreAllowlisted_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	var unexpected []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "testdata", ".git", "bin", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(rel, "cmd"+string(filepath.Separator)) && !strings.HasPrefix(rel, "internal"+string(filepath.Separator)) {
			return nil
		}
		if !productionFileCallsOpsAppend(t, path) {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, ok := introductionWriterAllowlist[rel]; !ok {
			unexpected = append(unexpected, rel)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, unexpected, "production ops.Append* call site is not on the Introduction allowlist: %v", unexpected)
}

func productionFileCallsOpsAppend(t *testing.T, path string) bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err, path)
	opsAlias := ""
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != "github.com/scullxbones/armature/internal/ops" {
			continue
		}
		if imp.Name != nil {
			opsAlias = imp.Name.Name
		} else {
			opsAlias = "ops"
		}
	}
	if opsAlias == "" || opsAlias == "_" {
		return false
	}
	src, err := os.ReadFile(path)
	require.NoError(t, err, path)
	for _, name := range []string{"AppendOp", "AppendOps", "AppendAndCommit"} {
		if strings.Contains(string(src), opsAlias+"."+name) {
			return true
		}
	}
	return false
}

func wellFormedTask(id, scope string) *materialize.Issue {
	return &materialize.Issue{
		ID:               id,
		Type:             "task",
		Status:           ops.StatusOpen,
		Title:            id,
		Scope:            []string{scope},
		DefinitionOfDone: "Task " + id + " is complete and tested",
		Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
		BlockedBy:        []string{},
		Children:         []string{},
		Provenance:       materialize.Provenance{Confidence: "draft"},
	}
}

func TestIntroductionRefusesDirtyWrite_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	state := makeState(wellFormedTask("EXISTING", "internal/ops/*.go"))
	proposed := []ops.Op{{
		Type:     ops.OpCreate,
		TargetID: "NEW-DIRTY",
		Payload: ops.Payload{
			Title:            "Overlapping write",
			NodeType:         "task",
			Scope:            []string{"internal/ops/*.go"},
			DefinitionOfDone: "New overlapping task is complete and tested",
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			Confidence:       "draft",
		},
	}}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.Error(t, err, "a write that introduces a Graph Finding on a targeted issue must be refused")
	assert.Contains(t, err.Error(), "scope overlap")
	assert.Contains(t, err.Error(), "NEW-DIRTY")
	assert.NotContains(t, err.Error(), "override-release")
	assert.NotContains(t, err.Error(), "skip-validate-gate")
	assert.Regexp(t, `revert|cancel`, err.Error())
}

func TestIntroductionIgnoresForeignFindings_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	foreign := wellFormedTask("FOREIGN", "docs/foreign.md")
	foreign.DefinitionOfDone = "Do this properly so it works correctly"
	state := makeState(foreign)
	proposed := []ops.Op{{
		Type:     ops.OpCreate,
		TargetID: "CLEAN-NEW",
		Payload: ops.Payload{
			Title:            "Unrelated clean task",
			NodeType:         "task",
			Scope:            []string{"cmd/armature/clean.go"},
			DefinitionOfDone: "Clean new task is complete and tested",
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			Confidence:       "draft",
		},
	}}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.NoError(t, err, "pre-existing findings on foreign IDs must not block an unrelated write")
}

func TestIntroductionRefusesE6W8W11_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()

	t.Run("E6 missing required field on create", func(t *testing.T) {
		t.Parallel()
		state := makeState(wellFormedTask("EXISTING", "internal/foo.go"))
		proposed := []ops.Op{{
			Type:     ops.OpCreate,
			TargetID: "NEW-INCOMPLETE",
			Payload: ops.Payload{
				Title:      "Missing required fields",
				NodeType:   "task",
				Confidence: "draft",
			},
		}}
		err := CheckIntroduction(state, proposed, Options{Strict: true})
		require.Error(t, err, "Introduction must refuse a write that introduces E6 on a targeted issue")
		assert.Contains(t, err.Error(), "missing required field")
		assert.Contains(t, err.Error(), "NEW-INCOMPLETE")
	})

	t.Run("W8 conflicting decision", func(t *testing.T) {
		t.Parallel()
		existing := wellFormedTask("DECIDE", "internal/decide.go")
		existing.Decisions = []materialize.Decision{
			{Topic: "storage", Choice: "postgres"},
		}
		state := makeState(existing)
		proposed := []ops.Op{{
			Type:     ops.OpDecision,
			TargetID: "DECIDE",
			Payload: ops.Payload{
				Topic:  "storage",
				Choice: "sqlite",
			},
		}}
		err := CheckIntroduction(state, proposed, Options{Strict: true})
		require.Error(t, err, "Introduction must refuse a write that introduces W8 on a targeted issue")
		assert.Contains(t, err.Error(), "conflicting decisions")
		assert.Contains(t, err.Error(), "DECIDE")
	})

	t.Run("W11 vague outcome on transition", func(t *testing.T) {
		t.Parallel()
		state := makeState(wellFormedTask("SHIP", "internal/ship.go"))
		proposed := []ops.Op{{
			Type:     ops.OpTransition,
			TargetID: "SHIP",
			Payload: ops.Payload{
				To:      ops.StatusDone,
				Outcome: "done",
			},
		}}
		err := CheckIntroduction(state, proposed, Options{Strict: true})
		require.Error(t, err, "Introduction must refuse a write that introduces W11 on a targeted issue")
		assert.Contains(t, err.Error(), "vague outcome")
		assert.Contains(t, err.Error(), "SHIP")
	})
}

func TestIntroductionAllowsCiteAfterE7_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	existing := wellFormedTask("EXISTING", "internal/foo.go")
	existing.CitationAcceptances = []materialize.CitationAcceptance{{WorkerID: "w", Timestamp: 1}}
	state := makeState(existing)
	proposed := []ops.Op{{
		Type:     ops.OpCreate,
		TargetID: "NEW-UNCITED",
		Payload: ops.Payload{
			Title:            "Uncited draft",
			NodeType:         "task",
			Scope:            []string{"internal/new.go"},
			DefinitionOfDone: "New uncited task is complete and tested",
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			Confidence:       "draft",
		},
	}}
	manifest, err := json.Marshal(map[string]any{
		"entries": map[string]map[string]string{"src-1": {"id": "src-1"}},
	})
	require.NoError(t, err)
	err = CheckIntroduction(state, proposed, Options{Strict: true, ManifestData: manifest})
	require.NoError(t, err, "cite-after remains legal: E7/E8 must not refuse Introduction")
}

func TestIntroductionRefusesAliasedW8Finding(t *testing.T) {
	t.Parallel()
	existing := wellFormedTask("DECIDE", "internal/decide.go")
	existing.Decisions = []materialize.Decision{
		{Topic: "storage", Choice: "postgres"},
		{Topic: "storage", Choice: "sqlite"},
		{Topic: "cache", Choice: "redis"},
	}
	state := makeState(existing)
	proposed := []ops.Op{{
		Type:     ops.OpDecision,
		TargetID: "DECIDE",
		Payload: ops.Payload{
			Topic:  "cache",
			Choice: "memcached",
		},
	}}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.Error(t, err, "a new W8 on a different topic must not alias an existing W8 on the same issue")
	assert.Contains(t, err.Error(), "cache")
}

func TestIntroductionProjectsLinkBeforeSameTimestampCreate(t *testing.T) {
	t.Parallel()
	state := makeState(wellFormedTask("EXISTING", "internal/existing.go"))
	proposed := []ops.Op{
		{
			Type:      ops.OpLink,
			TargetID:  "NEW",
			Timestamp: 100,
			Payload: ops.Payload{
				Dep: "EXISTING",
				Rel: "blocked_by",
			},
		},
		{
			Type:      ops.OpCreate,
			TargetID:  "NEW",
			Timestamp: 100,
			Payload: ops.Payload{
				Title:            "Forward-referenced create",
				NodeType:         "task",
				Scope:            []string{"internal/new.go"},
				DefinitionOfDone: "Forward-referenced create is complete and tested",
				Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
				Confidence:       "draft",
			},
		},
	}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.NoError(t, err, "same-timestamp link-before-create must project via create-first sort, not fail ApplyOp")
}

func TestIntroductionRollsUpProposedOps(t *testing.T) {
	t.Parallel()
	proposed := []ops.Op{
		{
			Type:      ops.OpCreate,
			TargetID:  "STORY",
			Timestamp: 100,
			Payload: ops.Payload{
				Title:    "Rolled-up story",
				NodeType: "story",
				Scope:    []string{"**/*"},
			},
		},
		{
			Type:      ops.OpCreate,
			TargetID:  "CHILD",
			Timestamp: 100,
			Payload: ops.Payload{
				Title:            "Sole child",
				NodeType:         "task",
				Parent:           "STORY",
				Scope:            []string{"internal/child.go"},
				DefinitionOfDone: "Sole child is complete and tested",
				Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
				Confidence:       "draft",
			},
		},
		{
			Type:      ops.OpTransition,
			TargetID:  "CHILD",
			Timestamp: 101,
			Payload: ops.Payload{
				To:      ops.StatusMerged,
				Outcome: "Child delivered with tests and a review",
			},
		},
	}
	err := CheckIntroduction(materialize.NewState(), proposed, Options{Strict: true})
	require.NoError(t, err, "rollup must run on the projected state so a now-terminal story does not introduce W4")
}

func TestValidate_CircularDepNamesParticipants(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{"B"}, Children: []string{}},
		&materialize.Issue{ID: "B", BlockedBy: []string{"A"}, Children: []string{}},
	)
	result := Validate(state, graphFromState(state), Options{})
	require.False(t, result.OK)
	require.NotEmpty(t, result.Errors)
	assert.Contains(t, result.Errors[0], "cycle detected")
	assert.Contains(t, result.Errors[0], "A")
	assert.Contains(t, result.Errors[0], "B")
}

func TestIntroductionAllowsW1NarrowingResidualOverlap_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	x := wellFormedTask("X", "a.go")
	x.Scope = []string{"a.go", "b.go"}
	y := wellFormedTask("Y", "a.go")
	y.Scope = []string{"a.go", "b.go"}
	state := makeState(x, y)

	before := Validate(state, graphFromState(state), Options{})
	require.True(t, containsWarning(before, "scope overlap"))

	proposed := []ops.Op{{
		Type:     ops.OpAmend,
		TargetID: "X",
		Payload: ops.Payload{
			Scope: []string{"a.go"},
		},
	}}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.NoError(t, err, "narrowing a foreign overlap down (still overlapping on a.go) must not read as newly introduced")
}

func TestIntroductionAllowsCountOnlyMessageChange_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()

	before := Result{Findings: []Finding{
		{Severity: "warning", Rule: "W11", CitedIDs: []string{"SHIP"}, Message: "vague outcome: SHIP outcome is 5 chars"},
	}}
	after := Result{Findings: []Finding{
		{Severity: "warning", Rule: "W11", CitedIDs: []string{"SHIP"}, Message: "vague outcome: SHIP outcome is 8 chars"},
	}}
	introduced := introducedFindings(before, after, []string{"SHIP"})
	assert.Empty(t, introduced, "a finding whose message embeds a changing detail (not its Key) must not be re-introduced")
}

func TestIntroductionE4PartialCycleBreakNotIntroduced_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()

	a := &materialize.Issue{ID: "A", Type: "task", Status: ops.StatusOpen, BlockedBy: []string{"B"}}
	b := &materialize.Issue{ID: "B", Type: "task", Status: ops.StatusOpen, BlockedBy: []string{"A", "C"}}
	c := &materialize.Issue{ID: "C", Type: "task", Status: ops.StatusOpen, BlockedBy: []string{"B"}}
	state := makeState(a, b, c)

	before := Validate(state, graphFromState(state), Options{})
	require.True(t, containsError(before, "cycle detected"))

	proposed := []ops.Op{{
		Type:     ops.OpUnlink,
		TargetID: "C",
		Payload: ops.Payload{
			Dep: "B",
			Rel: "blocked_by",
		},
	}}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.NoError(t, err, "a cycle that shrinks (residual CitedIDs subset of a pre-existing cycle) must not be reported as introduced")
}

func TestIntroductionE4GrowingCycleStillIntroduced_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	a := &materialize.Issue{ID: "A", Type: "task", Status: ops.StatusOpen, BlockedBy: []string{"B"}}
	b := &materialize.Issue{ID: "B", Type: "task", Status: ops.StatusOpen, BlockedBy: []string{"A"}}
	c := &materialize.Issue{ID: "C", Type: "task", Status: ops.StatusOpen, BlockedBy: []string{}}
	state := makeState(a, b, c)

	before := Validate(state, graphFromState(state), Options{})
	require.True(t, containsError(before, "cycle detected"))

	proposed := []ops.Op{
		{
			Type:     ops.OpLink,
			TargetID: "B",
			Payload:  ops.Payload{Dep: "C", Rel: "blocked_by"},
		},
		{
			Type:     ops.OpLink,
			TargetID: "C",
			Payload:  ops.Payload{Dep: "B", Rel: "blocked_by"},
		},
	}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.Error(t, err, "a cycle that grows to include a new node must still be refused as introduced")
	assert.Contains(t, err.Error(), "cycle detected")
}

func TestIntroductionRefusesSecondE6OnDifferentField_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()

	before := Result{Findings: []Finding{
		{Severity: "error", Rule: "E6", CitedIDs: []string{"BARE2"}, Key: "scope", Message: "missing required field: scope on task BARE2"},
	}}
	after := Result{Findings: []Finding{
		{Severity: "error", Rule: "E6", CitedIDs: []string{"BARE2"}, Key: "definition_of_done", Message: "missing required field: definition_of_done on task BARE2"},
	}}
	introduced := introducedFindings(before, after, []string{"BARE2"})
	require.Len(t, introduced, 1, "a second E6 finding on the same issue for a different required field must still be introduced")
	assert.Contains(t, introduced[0].Message, "definition_of_done")
}

func TestIntroductionRefusesSecondE10OnDifferentGlob_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	existing := wellFormedTask("GLOB", "internal/glob.go")
	existing.Scope = []string{"[invalid"}
	state := makeState(existing)

	before := Validate(state, graphFromState(state), Options{})
	require.True(t, containsError(before, "invalid glob"))

	proposed := []ops.Op{{
		Type:     ops.OpAmend,
		TargetID: "GLOB",
		Payload: ops.Payload{
			Scope: []string{"[invalid", "[alsoinvalid"},
		},
	}}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.Error(t, err, "a second E10 finding on the same issue for a different glob must still be introduced")
	assert.Contains(t, err.Error(), "[alsoinvalid")
}

func TestIntroductionRefusesSecondW8OnDifferentTopic_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	existing := wellFormedTask("DECIDE2", "internal/decide2.go")
	existing.Decisions = []materialize.Decision{
		{Topic: "storage", Choice: "postgres"},
		{Topic: "storage", Choice: "sqlite"},
	}
	state := makeState(existing)

	before := Validate(state, graphFromState(state), Options{})
	require.True(t, containsWarning(before, "conflicting decisions"))

	proposed := []ops.Op{
		{
			Type:     ops.OpDecision,
			TargetID: "DECIDE2",
			Payload:  ops.Payload{Topic: "cache", Choice: "redis"},
		},
		{
			Type:     ops.OpDecision,
			TargetID: "DECIDE2",
			Payload:  ops.Payload{Topic: "cache", Choice: "memcached"},
		},
	}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.Error(t, err, "a second W8 finding on the same issue for a different topic must still be introduced")
	assert.Contains(t, err.Error(), "cache")
}

func TestIntroductionAllowsReopenOfLegacyDoneIssueMissingRequiredFields_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	legacy := &materialize.Issue{
		ID:         "LEGACY",
		Type:       "task",
		Status:     ops.StatusDone,
		Title:      "LEGACY",
		Outcome:    "Delivered with tests and a full review of the change",
		Provenance: materialize.Provenance{Confidence: "draft"},
	}
	state := makeState(legacy)

	proposed := []ops.Op{{
		Type:     ops.OpTransition,
		TargetID: "LEGACY",
		Payload: ops.Payload{
			To: ops.StatusOpen,
		},
	}}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.NoError(t, err, "reopening a legacy issue must not be blocked by E6 findings that only exist because terminal suppression was lifted")

	afterState, perr := projectState(state, proposed)
	require.NoError(t, perr)
	after := Validate(afterState, materialize.GraphFromState(afterState), Options{})
	assert.True(t, containsError(after, "missing required field"), "arm validate must still surface E6 on the reopened issue")
}

func TestIntroductionAllowsReopenReenteringW1Overlap_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	reopened := wellFormedTask("REENTER", "shared.go")
	reopened.Status = ops.StatusDone
	foreign := wellFormedTask("INFLIGHT", "shared.go")
	state := makeState(reopened, foreign)

	proposed := []ops.Op{{
		Type:     ops.OpTransition,
		TargetID: "REENTER",
		Payload: ops.Payload{
			To: ops.StatusOpen,
		},
	}}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.NoError(t, err, "reopening a task back into a scope overlap with a foreign in-flight task must not be blocked")
}

func TestIntroductionTransitionIntoTerminalDoesNotWidenBaseline_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	incomplete := &materialize.Issue{
		ID:         "INCOMPLETE",
		Type:       "task",
		Status:     ops.StatusOpen,
		Title:      "INCOMPLETE",
		Provenance: materialize.Provenance{Confidence: "draft"},
	}
	state := makeState(incomplete)

	proposed := []ops.Op{{
		Type:     ops.OpTransition,
		TargetID: "INCOMPLETE",
		Payload: ops.Payload{
			To:      ops.StatusDone,
			Outcome: "Delivered with tests and a full review of the change",
		},
	}}

	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.NoError(t, err)
}

func TestIntroductionRefusesReopenBatchWithGenuinelyNewFinding_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	legacy := wellFormedTask("BATCH", "internal/batch.go")
	legacy.Status = ops.StatusDone
	state := makeState(legacy)

	proposed := []ops.Op{
		{
			Type:     ops.OpTransition,
			TargetID: "BATCH",
			Payload:  ops.Payload{To: ops.StatusOpen},
		},
		{
			Type:     ops.OpAmend,
			TargetID: "BATCH",
			Payload:  ops.Payload{Scope: []string{"[invalid"}},
		},
	}
	err := CheckIntroduction(state, proposed, Options{Strict: true})
	require.Error(t, err, "a batch that reopens AND introduces a genuinely new finding on the target must still be refused")
	assert.Contains(t, err.Error(), "invalid glob")
}

func s7T2Fixture() *materialize.Issue {
	return &materialize.Issue{
		ID:               "LNGHZN-S7-T2",
		Type:             "task",
		Status:           ops.StatusOpen,
		Title:            "strict config decode + doctor config-health",
		DefinitionOfDone: "arm doctor gains check D9 so the config file can never silently lie again",
		Scope: []string{
			"internal/config/strict.go",
			"internal/config/strict_test.go",
			"internal/doctor/config_check.go",
			"internal/doctor/config_check_test.go",
		},
		Acceptance: json.RawMessage(`[
			{"type":"test_passes","cmd":"go test ./internal/config/ -run TestStrictDecodeRejectsUnknownField"},
			{"type":"test_passes","cmd":"go test ./internal/doctor/ -run TestDoctorConfigCheck"},
			{"type":"test_passes","cmd":"make check"}
		]`),
		BlockedBy:  []string{},
		Children:   []string{},
		Provenance: materialize.Provenance{Confidence: "draft"},
	}
}

func TestValidateDoDScopeMismatch_REQ_TOPTIER_S18_T2(t *testing.T) {
	t.Parallel()

	t.Run("s7_t2_fixture_emits_e14_wiring_and_unit_only_acceptance", func(t *testing.T) {
		t.Parallel()
		state := makeState(s7T2Fixture())
		result := Validate(state, graphFromState(state), Options{})
		assert.False(t, result.OK)

		var wiring, unitOnly *Finding
		for i := range result.Findings {
			f := &result.Findings[i]
			if f.Rule != "E14" {
				continue
			}
			switch f.Key {
			case "doctor.run_wiring":
				wiring = f
			case "unit_only_acceptance":
				unitOnly = f
			}
		}
		require.NotNil(t, wiring, "expected E14 doctor.run_wiring, findings=%v errors=%v", result.Findings, result.Errors)
		assert.Equal(t, "error", wiring.Severity)
		assert.Equal(t, []string{"LNGHZN-S7-T2"}, wiring.CitedIDs)
		assert.Contains(t, wiring.Message, "E14")
		assert.Contains(t, wiring.Message, "LNGHZN-S7-T2")
		assert.Contains(t, wiring.Message, "internal/doctor/doctor.go")
		assert.Contains(t, wiring.Message, "doctor.run_wiring")

		require.NotNil(t, unitOnly, "expected E14 unit_only_acceptance beside CLI DoD, findings=%v", result.Findings)
		assert.Equal(t, "error", unitOnly.Severity)
		assert.Equal(t, []string{"LNGHZN-S7-T2"}, unitOnly.CitedIDs)
		assert.Contains(t, unitOnly.Message, "E14")
		assert.Contains(t, unitOnly.Message, "unit-only")
		assert.Contains(t, unitOnly.Message, "LNGHZN-S7-T2")
	})

	t.Run("doctor_go_in_scope_still_refuses_unit_only_acceptance", func(t *testing.T) {
		t.Parallel()
		issue := s7T2Fixture()
		issue.ID = "WIRED-UNIT"
		issue.Scope = []string{"internal/doctor/doctor.go", "internal/doctor/doctor_test.go"}
		state := makeState(issue)
		result := Validate(state, graphFromState(state), Options{})
		assert.False(t, result.OK)
		var sawWiring, sawUnit bool
		for _, f := range result.Findings {
			if f.Rule != "E14" {
				continue
			}
			if f.Key == "doctor.run_wiring" {
				sawWiring = true
			}
			if f.Key == "unit_only_acceptance" {
				sawUnit = true
			}
		}
		assert.False(t, sawWiring, "wiring file in scope must not emit doctor.run_wiring")
		assert.True(t, sawUnit, "CLI DoD with unit-only Acceptance must still emit E14")
	})

	t.Run("plain_string_acceptance_is_unit_only", func(t *testing.T) {
		t.Parallel()
		issue := s7T2Fixture()
		issue.ID = "PLAIN-UNIT"
		issue.Scope = []string{"internal/doctor/doctor.go", "internal/doctor/doctor_test.go"}
		issue.Acceptance = json.RawMessage(`["go test ./internal/doctor", "make check"]`)
		state := makeState(issue)
		result := Validate(state, graphFromState(state), Options{})
		assert.False(t, result.OK)
		var sawWiring, sawUnit bool
		for _, f := range result.Findings {
			if f.Rule != "E14" {
				continue
			}
			if f.Key == "doctor.run_wiring" {
				sawWiring = true
			}
			if f.Key == "unit_only_acceptance" {
				sawUnit = true
			}
		}
		assert.False(t, sawWiring, "wiring file in scope must not emit doctor.run_wiring")
		assert.True(t, sawUnit, "plain-string Acceptance is unit-only beside a CLI DoD")
	})

	t.Run("plain_string_arm_doctor_acceptance_is_same_surface", func(t *testing.T) {
		t.Parallel()
		issue := s7T2Fixture()
		issue.ID = "PLAIN-CLI"
		issue.Scope = []string{"internal/doctor/doctor.go"}
		issue.Acceptance = json.RawMessage(`["arm doctor --format json", "make check"]`)
		state := makeState(issue)
		result := Validate(state, graphFromState(state), Options{})
		for _, f := range result.Findings {
			assert.NotEqual(t, "E14", f.Rule, "plain-string arm doctor Acceptance is same-surface, got %+v", f)
		}
	})

	t.Run("cli_acceptance_and_doctor_go_ok", func(t *testing.T) {
		t.Parallel()
		issue := s7T2Fixture()
		issue.ID = "WIRED-CLI"
		issue.Scope = []string{"internal/doctor/doctor.go"}
		issue.Acceptance = json.RawMessage(`[{"type":"test_passes","cmd":"arm doctor --format json"}]`)
		state := makeState(issue)
		result := Validate(state, graphFromState(state), Options{})
		for _, f := range result.Findings {
			assert.NotEqual(t, "E14", f.Rule, "well-formed CLI contract must not emit E14, got %+v", f)
		}
	})

	t.Run("s15_t2_readme_pointer_is_not_e14", func(t *testing.T) {
		t.Parallel()
		issue := &materialize.Issue{
			ID:     "TOPTIER-S15-T2",
			Type:   "task",
			Status: ops.StatusOpen,
			Title:  "Troubleshooting appendix in quickstart",
			DefinitionOfDone: "README quickstart gains an If something goes wrong appendix covering " +
				"gopls/LSP false positives, checked-out-branch Managed Worktree failures, and worktree " +
				"leak / wrong-checkout classes from docs/dogfood/findings/themes/git-worktree-friction/README.md, " +
				"plus a pointer to arm doctor --explain and D9 Unrecognized Managed Worktree for doctor-visible cases.",
			Scope:      []string{"README.md"},
			Acceptance: json.RawMessage(`[{"type":"test_passes","cmd":"arm validate"}]`),
			BlockedBy:  []string{},
			Children:   []string{},
		}
		state := makeState(issue)
		result := Validate(state, graphFromState(state), Options{})
		for _, f := range result.Findings {
			assert.NotEqual(t, "E14", f.Rule, "S15-T2 README pointer must not emit E14, got %+v", f)
		}
	})

	t.Run("s18_t2_meta_validate_dod_is_not_e14", func(t *testing.T) {
		t.Parallel()
		issue := &materialize.Issue{
			ID:     "TOPTIER-S18-T2",
			Type:   "task",
			Status: ops.StatusOpen,
			Title:  "arm validate errors when DoD is not implementable in scope",
			DefinitionOfDone: "arm validate Graph Finding E14 when task DoD claims arm doctor/gains check Dn " +
				"whose wiring file is absent from Scope; unit-only Acceptance cannot stand alone beside CLI DoD. " +
				"Tests cover S7-T2 fixture. No S14 dependency.",
			Scope:      []string{"internal/validate/validate.go", "internal/validate/validate_test.go"},
			Acceptance: json.RawMessage(`[{"type":"test_passes","cmd":"go test ./internal/validate/ -run TestValidateDoDScopeMismatch_REQ_TOPTIER_S18_T2"}]`),
			BlockedBy:  []string{},
			Children:   []string{},
		}
		state := makeState(issue)
		result := Validate(state, graphFromState(state), Options{})
		for _, f := range result.Findings {
			assert.NotEqual(t, "E14", f.Rule, "S18-T2 meta DoD must not self-hit E14, got %+v", f)
		}
	})

	t.Run("helper_only_dod_skips_e14", func(t *testing.T) {
		t.Parallel()
		issue := s7T2Fixture()
		issue.ID = "HELPER-T1"
		issue.DefinitionOfDone = "arm doctor gains check D9 as an exported helper, not wired into Run"
		state := makeState(issue)
		result := Validate(state, graphFromState(state), Options{})
		for _, f := range result.Findings {
			assert.NotEqual(t, "E14", f.Rule, "helper-only DoD must not emit E14, got %+v", f)
		}
	})
}

func TestWritePathDoesNotIntroduceFindings(t *testing.T) {
	t.Parallel()
	assert.Equal(t, writePathDoesNotIntroduceFinding("E7"), writePathDoesNotIntroduceE7)
	assert.Equal(t, writePathDoesNotIntroduceFinding("E8"), writePathDoesNotIntroduceE8)
	assert.Equal(t, writePathDoesNotIntroduceFinding("E13"), writePathDoesNotIntroduceE13)
	assert.True(t, writeDoesNotIntroduceRule("E7"))
	assert.True(t, writeDoesNotIntroduceRule("E8"))
	assert.True(t, writeDoesNotIntroduceRule("E13"))
	assert.False(t, writeDoesNotIntroduceRule("E4"))
	assert.False(t, writeDoesNotIntroduceRule("E14"))
	assert.False(t, writeDoesNotIntroduceRule("W1"))
	assert.False(t, writeDoesNotIntroduceRule(""))
}
