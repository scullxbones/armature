package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/traceability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	// At least one circular dependency error should be present
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
	assert.False(t, containsWarning(result, "scope overlap"), "story-level aggregate scopes should not trigger worker collision warnings")
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
		&materialize.Issue{ID: "TSK-1", Type: "task"}, // missing scope, acceptance, dod
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.False(t, result.OK)
	assert.True(t, containsError(result, "missing required field"))
}

func TestE6RequiredFields_SkipsMergedTask(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "merged"}, // merged — required fields not enforced
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, result.OK)
	assert.False(t, containsError(result, "missing required field"))
}

func TestE6RequiredFields_SkipsDoneTask(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done"}, // done — required fields not enforced
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, result.OK)
	assert.False(t, containsError(result, "missing required field"))
}

func TestE6RequiredFields_SkipsCancelledTask(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "cancelled"}, // cancelled — required fields not enforced
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
	// B.Blocks contains A (B was created first and blocks A) — should suppress overlap warning
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
	// Test transitive closure: A blocks B blocks C
	// Therefore A and C are transitively ordered (A ultimately blocks C)
	// and should NOT produce a scope-overlap warning.
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
			Scope:     []string{"internal/ops/*.go"}, // overlaps with TSK-A
			BlockedBy: []string{"TSK-B"},
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	// TSK-A and TSK-C should NOT trigger scope-overlap warning
	// because TSK-A transitively blocks TSK-C through TSK-B
	assert.False(t, containsWarning(result, "scope overlap"),
		"scope overlap should be suppressed when tasks are transitively ordered via blocked_by chain")
}

func TestW3BudgetExceeded_WithLargeContext(t *testing.T) {
	t.Parallel()
	// Context field pushes estimated token count over the 4000-token budget
	largeContext := make([]byte, 20000) // 20k bytes / 4 = 5000 est tokens
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
	// Outcome is exactly one of the vague words (exact match check at validate.go:491)
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done", Outcome: "done"},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{})
	assert.True(t, containsWarning(result, "vague outcome"))
}

func TestW5MissingContextFiles_TerminalStatusesSkipped(t *testing.T) {
	t.Parallel()
	// Merged/done/cancelled issues should not trigger the missing context_files warning —
	// the work is complete and the guidance is no longer actionable.
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
				// no ContextFiles — spans 3 dirs, would trigger W5 for active issues
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
	// The warning must not reference the non-existent --context-files flag.
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

func TestW10PhantomScope_TerminalStatusesSkipped(t *testing.T) {
	t.Parallel()
	// Issues with merged, done, or cancelled status should not trigger phantom scope warnings
	// even if their scope globs match no files.
	for _, status := range []string{"merged", "done", "cancelled"} {
		state := makeState(
			&materialize.Issue{
				ID:     "TSK-1",
				Type:   "task",
				Status: status,
				Scope:  []string{"nonexistent/path/*.go"},
			},
		)
		// For terminal statuses, W10 check is skipped anyway
		graph := graphFromState(state)
		result := Validate(state, graph, Options{PreExpandedScopes: nil})
		assert.False(t, containsPhantomScopeInfo(result),
			"status=%s: phantom scope should be skipped for terminal status", status)
	}
}

func TestW10PhantomScope_BlockedStillChecked(t *testing.T) {
	t.Parallel()
	// Blocked issues are not terminal — their scope should still be validated.
	state := makeState(
		&materialize.Issue{
			ID:     "TSK-1",
			Type:   "task",
			Status: "blocked",
			Scope:  []string{"nonexistent/path/*.go"},
		},
	)
	// Provide pre-expanded scopes showing no files match
	preExpandedScopes := map[string][]string{
		"TSK-1": {}, // empty list means globs matched no files
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	assert.True(t, containsPhantomScopeInfo(result),
		"blocked status should still trigger phantom scope warning")
}

func TestW10PhantomScope_EpicsAndStoriesWithTerminalStatusSkipped(t *testing.T) {
	t.Parallel()
	// Terminal status applies across all issue types, not just tasks.
	for _, issueType := range []string{"epic", "story"} {
		state := makeState(
			&materialize.Issue{
				ID:     "ISSUE-1",
				Type:   issueType,
				Status: "done",
				Scope:  []string{"nonexistent/path/*.go"},
			},
		)
		// Terminal status skips W10 check anyway
		graph := graphFromState(state)
		result := Validate(state, graph, Options{PreExpandedScopes: nil})
		assert.False(t, containsPhantomScopeInfo(result),
			"type=%s status=done: phantom scope should be skipped for terminal status", issueType)
	}
}

func TestW10PhantomScope_NewSuffixSkipped(t *testing.T) {
	t.Parallel()
	// Scope entries ending with " (new)" mark files not yet created; they should not
	// trigger phantom scope warnings because the file is intentionally planned, not missing.
	state := makeState(
		&materialize.Issue{
			ID:     "ISSUE-1",
			Type:   "task",
			Status: "open",
			Scope:  []string{"internal/adapters/files.go (new)", "internal/adapters/git.go (new)"},
		},
	)
	// "(new)" entries don't trigger phantom scope checks
	preExpandedScopes := map[string][]string{
		"ISSUE-1": {}, // empty list means no files matched
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	assert.False(t, containsPhantomScopeInfo(result),
		"scope entries with (new) suffix should not trigger phantom scope warnings")
}

func TestW10PhantomScope_NewSuffixMixedWithExisting(t *testing.T) {
	t.Parallel()
	// When a scope has both (new) and regular entries, only the regular nonexistent one triggers.
	dir := t.TempDir()
	// Create one real file
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
	// Provide pre-expanded scopes showing real.go exists but ghost.go doesn't
	preExpandedScopes := map[string][]string{
		"ISSUE-1": {"real.go"}, // ghost.go and planned.go (new) don't appear
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	// ghost.go is phantom (no (new) suffix, doesn't exist)
	assert.True(t, containsPhantomScopeInfo(result),
		"nonexistent file without (new) suffix should still trigger phantom scope warning")
	// Confirm only ghost.go is mentioned, not planned.go (new)
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
	// Legacy ops store scope as a single comma-joined string. The W10 check must split
	// and evaluate each path individually, skipping "(new)" entries within the list.
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.go")
	require.NoError(t, os.WriteFile(realFile, []byte("package x\n"), 0644))

	state := makeState(
		&materialize.Issue{
			ID:     "ISSUE-1",
			Type:   "task",
			Status: "open",
			// Legacy single-string entry with mixed (new), existing, and phantom paths.
			Scope: []string{"planned.go (new), real.go, ghost.go"},
		},
	)
	preExpandedScopes := map[string][]string{
		"ISSUE-1": {"real.go"}, // only real.go exists; planned.go (new) and ghost.go don't
	}
	graph := graphFromState(state)
	result := Validate(state, graph, Options{PreExpandedScopes: preExpandedScopes})
	var phantomInfos []string
	for _, info := range result.Infos {
		if strings.Contains(info, "phantom scope") {
			phantomInfos = append(phantomInfos, info)
		}
	}
	// Only ghost.go should be phantom; planned.go (new) is skipped, real.go exists.
	assert.Len(t, phantomInfos, 1)
	assert.Contains(t, phantomInfos[0], "ghost.go")
	assert.NotContains(t, phantomInfos[0], "planned.go")
	assert.NotContains(t, phantomInfos[0], "real.go")
}

func TestValidateUsesCoverage(t *testing.T) {
	t.Parallel()
	// Pass coverage data directly
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

// TestE5TypeHierarchy_SkipsTerminalStatus verifies that cancelled, done, and merged
// issues are not flagged for hierarchy violations — they have already been delivered.
func TestE5TypeHierarchy_SkipsTerminalStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"cancelled", "done", "merged"} {
		t.Run("status="+status, func(t *testing.T) {
			t.Parallel()
			// task parenting another task is normally invalid, but terminal tasks are exempt
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

// TestE5TypeHierarchy_SkipsTerminalChildren verifies that cancelled, done, and merged
// children are not flagged for hierarchy violations even if the parent/child combo would
// otherwise be invalid (e.g. bug under task).
func TestE5TypeHierarchy_SkipsTerminalChildren(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"cancelled", "done", "merged"} {
		t.Run("status="+status, func(t *testing.T) {
			t.Parallel()
			// bug under task is normally invalid, but terminal children are exempt
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

// TestE5TypeHierarchy_BugUnderStoryIsValid verifies that bug is a valid child of story.
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

// TestE5TypeHierarchy_BugUnderEpicIsValid verifies that bug is a valid child of epic.
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

// TestE5TypeHierarchy_BugUnderTaskIsInvalid verifies that bug cannot be parented under a task.
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

// TestParentFilterAllIssues validates that with empty ParentID, all issues are validated.
func TestParentFilter_NoFilterReturnsAllIssues(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story", Children: []string{"TASK-A", "TASK-B"}, BlockedBy: []string{}},
		&materialize.Issue{
			ID:               "TASK-A",
			Type:             "task",
			Parent:           "STORY-1",
			BlockedBy:        []string{},
			Scope:            []string{"a.go"},
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			DefinitionOfDone: "Complete the task",
		},
		&materialize.Issue{
			ID:               "TASK-B",
			Type:             "task",
			Parent:           "STORY-1",
			BlockedBy:        []string{},
			Scope:            []string{"b.go"},
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			DefinitionOfDone: "Complete the task",
		},
	)
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ParentID: ""})
	// No errors expected in clean state
	assert.True(t, result.OK)
}

// TestParentFilterDirectChildrenOnly validates that --parent restricts to direct children.
func TestParentFilter_RestrictsToDirectChildren(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "EPIC-1", Type: "epic", Children: []string{"STORY-1"}, BlockedBy: []string{}},
		&materialize.Issue{ID: "STORY-1", Type: "story", Parent: "EPIC-1", Children: []string{"TASK-A", "TASK-B"}, BlockedBy: []string{}},
		&materialize.Issue{
			ID:               "TASK-A",
			Type:             "task",
			Parent:           "STORY-1",
			BlockedBy:        []string{},
			Scope:            []string{"a.go"},
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			DefinitionOfDone: "Complete the task",
		},
		&materialize.Issue{
			ID:               "TASK-B",
			Type:             "task",
			Parent:           "STORY-1",
			BlockedBy:        []string{},
			Scope:            []string{"b.go"},
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			DefinitionOfDone: "Complete the task",
		},
	)
	// When filtering by STORY-1, only TASK-A and TASK-B should be validated (direct children)
	// EPIC-1 and STORY-1 itself should not appear
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ParentID: "STORY-1"})
	// Validate that the result is OK (no errors from children)
	assert.True(t, result.OK)
}

// TestParentFilterExcludesNonChildren validates that non-child issues are excluded.
func TestParentFilter_ExcludesNonChildren(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story", Children: []string{"TASK-A"}, BlockedBy: []string{}},
		&materialize.Issue{ID: "STORY-2", Type: "story", Children: []string{"TASK-B"}, BlockedBy: []string{}},
		&materialize.Issue{
			ID:               "TASK-A",
			Type:             "task",
			Parent:           "STORY-1",
			BlockedBy:        []string{},
			Scope:            []string{"a.go"},
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			DefinitionOfDone: "Complete the task",
		},
		&materialize.Issue{
			ID:               "TASK-B",
			Type:             "task",
			Parent:           "STORY-2",
			BlockedBy:        []string{},
			Scope:            []string{"b.go"},
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			DefinitionOfDone: "Complete the task",
		},
	)
	// Filter by STORY-1; only TASK-A should be validated
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ParentID: "STORY-1"})
	assert.True(t, result.OK)
}

// TestParentFilterValidatesChildrenOnly validates that validation only applies to children.
func TestParentFilter_ValidatesChildrenWithErrors(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story", Children: []string{"TASK-A", "TASK-B"}},
		&materialize.Issue{
			ID:     "TASK-A",
			Type:   "task",
			Parent: "STORY-1",
			// Missing required fields: scope, acceptance, definition_of_done
		},
		&materialize.Issue{ID: "TASK-B", Type: "task", Parent: "STORY-1"},
	)
	// When filtering by STORY-1, only direct children (TASK-A, TASK-B) are validated
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ParentID: "STORY-1"})
	// TASK-A is missing required fields; should have errors
	assert.False(t, result.OK)
	assert.True(t, containsError(result, "missing required field"))
}

// TestParentFilterEmptyParentScope validates parent filter with non-existent parent ID.
func TestParentFilter_NonexistentParentID(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story", Children: []string{"TASK-A"}},
		&materialize.Issue{ID: "TASK-A", Type: "task", Parent: "STORY-1"},
	)
	// When filtering by a non-existent parent ID, no children exist
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ParentID: "NONEXISTENT"})
	// No children to validate, so OK should be true (no errors)
	assert.True(t, result.OK)
}

// TestParentFilterParentNotValidated validates that the parent node itself is not validated.
func TestParentFilter_ParentNodeNotValidated(t *testing.T) {
	t.Parallel()
	state := makeState(
		&materialize.Issue{
			ID:        "TASK-PARENT",
			Type:      "task",
			Parent:    "STORY-1",
			Children:  []string{"TASK-CHILD"},
			BlockedBy: []string{},
			// Missing required fields — but this issue should not be validated
			// since it's not a direct child of itself
		},
		&materialize.Issue{
			ID:               "TASK-CHILD",
			Type:             "task",
			Parent:           "TASK-PARENT",
			BlockedBy:        []string{},
			Scope:            []string{"c.go"},
			Acceptance:       json.RawMessage(`[{"type":"test_passes"}]`),
			DefinitionOfDone: "Complete the task",
		},
	)
	// When filtering by TASK-PARENT, only TASK-CHILD should be validated
	// TASK-PARENT itself should not be in the validation set
	graph := graphFromState(state)
	result := Validate(state, graph, Options{ParentID: "TASK-PARENT"})
	// TASK-CHILD has no errors; the parent's missing fields should not be reported
	assert.True(t, result.OK)
}

// TestCheckE4Cycles_CrossScopeBlockerCycle verifies that graph.ScopedHasCycle detects
// a cycle where the blocker is outside the scope but creates a cycle with a scoped issue.
// Example: Task A (in scope) is blocked by Task B (out of scope). Task B is blocked by Task A.
// This is a real cycle that prevents A from ever becoming ready, and should be detected.
func TestCheckE4Cycles_CrossScopeBlockerCycle(t *testing.T) {
	t.Parallel()
	// Create a state with two issues: A and B, where A blocks B and B blocks A.
	// When validating scope={A}, graph.ScopedHasCycle("A", ...) should detect the cycle.
	state := makeState(
		&materialize.Issue{
			ID:        "A",
			Type:      "task",
			Status:    "open",
			BlockedBy: []string{"B"}, // A is blocked by B
		},
		&materialize.Issue{
			ID:        "B",
			Type:      "task",
			Status:    "open",
			BlockedBy: []string{"A"}, // B is blocked by A — cycle!
		},
	)

	// Build the graph
	graph := graphFromState(state)

	// Define scope containing only A (B is out of scope)
	scope := map[string]bool{
		"A": true,
	}

	// Call ScopedHasCycle with A in scope, B out of scope
	result := graph.ScopedHasCycle("A", scope)

	// Should detect the cycle even though B is out of scope
	assert.True(t, result, "expected ScopedHasCycle to detect cross-scope blocker cycle")
}

// TestCheckE4Cycles_OutOfScopeCycleIsNotFalsePositive verifies that graph.ScopedHasCycle does NOT
// report a cycle when the cycle exists entirely outside the scope.
// Example: A (in scope) is blocked by B (out of scope). B and C form a cycle B→C→B.
// A is not part of any cycle, so graph.ScopedHasCycle("A") must return false.
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
			BlockedBy: []string{"C"}, // B→C→B cycle, entirely out of scope
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
