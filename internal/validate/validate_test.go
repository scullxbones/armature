package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
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

func TestValidate_Clean(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{}, Children: []string{}},
		&materialize.Issue{ID: "B", BlockedBy: []string{}, Children: []string{}},
	)
	result := Validate(state, Options{})
	assert.True(t, result.OK)
	assert.Nil(t, result.Errors)
}

func TestValidate_OrphanedChild(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "A", Parent: "nonexistent", BlockedBy: []string{}, Children: []string{}},
	)
	result := Validate(state, Options{})
	assert.False(t, result.OK)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "unresolved parent")
}

func TestValidate_CircularDep(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{"B"}, Children: []string{}},
		&materialize.Issue{ID: "B", BlockedBy: []string{"A"}, Children: []string{}},
	)
	result := Validate(state, Options{})
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
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{"ghost"}, Children: []string{}},
	)
	result := Validate(state, Options{})
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

func TestW1ScopeOverlap(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}},
	)
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "scope overlap"))
}

func TestW1ScopeOverlap_SuppressedByBlockedBy(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, Blocks: []string{"TSK-B"}},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, BlockedBy: []string{"TSK-A"}},
	)
	result := Validate(state, Options{})
	assert.False(t, containsWarning(result, "scope overlap"), "scope overlap should be suppressed when one sibling blocks the other")
}

func TestW1ScopeOverlap_SkipsTerminalTasks(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, Status: "done"},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, Status: "merged"},
	)
	result := Validate(state, Options{})
	assert.False(t, containsWarning(result, "scope overlap"), "terminal sibling tasks should not trigger scope overlap warnings")
}

func TestW1ScopeOverlap_SkipsNonTaskIssues(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "STORY-A", Type: "story", Parent: "EPIC-1", Scope: []string{"internal/ops/*.go"}},
		&materialize.Issue{ID: "STORY-B", Type: "story", Parent: "EPIC-1", Scope: []string{"internal/ops/*.go"}},
	)
	result := Validate(state, Options{})
	assert.False(t, containsWarning(result, "scope overlap"), "story-level aggregate scopes should not trigger worker collision warnings")
}

func TestW2NoTestCriteria(t *testing.T) {
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Acceptance: json.RawMessage(`[{"type":"review","text":"look at it"}]`),
		},
	)
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "no test criteria"))
}

func TestW2NoTestCriteria_ManualReviewSatisfies(t *testing.T) {
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Acceptance: json.RawMessage(`[{"type":"manual_review","description":"docs reviewed"}]`),
		},
	)
	result := Validate(state, Options{})
	assert.False(t, containsWarning(result, "no test criteria"), "manual_review should satisfy test criteria requirement")
}

func TestW7VagueDoD(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", DefinitionOfDone: "Make it work properly and correctly"},
	)
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "vague dod"))
}

func TestW8ConflictingDecisions(t *testing.T) {
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Decisions: []materialize.Decision{
				{Topic: "storage", Choice: "postgres"},
				{Topic: "storage", Choice: "sqlite"},
			},
		},
	)
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "conflicting decisions"))
}

func TestW8ConflictingDecisions_IgnoresDuplicateChoices(t *testing.T) {
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
	result := Validate(state, Options{})
	assert.False(t, containsWarning(result, "conflicting decisions"), "repeating the same choice should not trigger a conflict warning")
}

func TestW11VagueOutcome(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done", Outcome: "done"},
	)
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "vague outcome"))
}

func TestE5TypeHierarchy(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TASK-1", Type: "task", Children: []string{"TASK-2"}},
		&materialize.Issue{ID: "TASK-2", Type: "task", Parent: "TASK-1"},
	)
	result := Validate(state, Options{})
	assert.True(t, containsError(result, "invalid hierarchy"))
}

func TestE6RequiredFields(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task"}, // missing scope, acceptance, dod
	)
	result := Validate(state, Options{})
	assert.False(t, result.OK)
	assert.True(t, containsError(result, "missing required field"))
}

func TestE6RequiredFields_SkipsMergedTask(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "merged"}, // merged — required fields not enforced
	)
	result := Validate(state, Options{})
	assert.True(t, result.OK)
	assert.False(t, containsError(result, "missing required field"))
}

func TestE6RequiredFields_SkipsDoneTask(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done"}, // done — required fields not enforced
	)
	result := Validate(state, Options{})
	assert.True(t, result.OK)
	assert.False(t, containsError(result, "missing required field"))
}

func TestE6RequiredFields_SkipsCancelledTask(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "cancelled"}, // cancelled — required fields not enforced
	)
	result := Validate(state, Options{})
	assert.True(t, result.OK)
	assert.False(t, containsError(result, "missing required field"))
}

func TestE5TypeHierarchy_EpicWithTaskIsValid(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "EPIC-1", Type: "epic", Children: []string{"TASK-2"}},
		&materialize.Issue{ID: "TASK-2", Type: "task", Parent: "EPIC-1"},
	)
	result := Validate(state, Options{})
	assert.False(t, containsError(result, "invalid hierarchy"), "epic with task child should be valid")
}

func TestW1ScopeOverlap_SuppressedWhenBBlocksA(t *testing.T) {
	// B.Blocks contains A (B was created first and blocks A) — should suppress overlap warning
	state := makeState(
		&materialize.Issue{ID: "TSK-A", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, BlockedBy: []string{"TSK-B"}},
		&materialize.Issue{ID: "TSK-B", Type: "task", Parent: "STORY-1", Scope: []string{"internal/ops/*.go"}, Blocks: []string{"TSK-A"}},
	)
	result := Validate(state, Options{})
	assert.False(t, containsWarning(result, "scope overlap"), "scope overlap should be suppressed when B blocks A")
}

func TestW3BudgetExceeded_WithLargeContext(t *testing.T) {
	// Context field pushes estimated token count over the 4000-token budget
	largeContext := make([]byte, 20000) // 20k bytes / 4 = 5000 est tokens
	for i := range largeContext {
		largeContext[i] = 'x'
	}
	jsonContext := append([]byte(`"`), append(largeContext, '"')...)
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Context: json.RawMessage(jsonContext)},
	)
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "budget advisory"))
}

func TestW6ComplexityMismatch_SmallWith6Files(t *testing.T) {
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Scope:         []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"},
			EstComplexity: "small",
		},
	)
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "complexity mismatch"))
}

func TestW6ComplexityMismatch_LargeWith1File(t *testing.T) {
	state := makeState(
		&materialize.Issue{
			ID: "TSK-1", Type: "task",
			Scope:         []string{"a.go"},
			EstComplexity: "large",
		},
	)
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "complexity mismatch"))
}

func TestW11VagueOutcome_ExactVagueWord(t *testing.T) {
	// Outcome is exactly one of the vague words (exact match check at validate.go:491)
	state := makeState(
		&materialize.Issue{ID: "TSK-1", Type: "task", Status: "done", Outcome: "done"},
	)
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "vague outcome"))
}

func TestW5MissingContextFiles_TerminalStatusesSkipped(t *testing.T) {
	// Merged/done/cancelled issues should not trigger the missing context_files warning —
	// the work is complete and the guidance is no longer actionable.
	for _, status := range []string{"merged", "done", "cancelled"} {
		t.Run(status, func(t *testing.T) {
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
			result := Validate(state, Options{})
			assert.False(t, containsWarning(result, "missing context_files"),
				"status=%q: terminal issues should not warn about missing context_files", status)
		})
	}
}

func TestW5MissingContextFiles_ActiveIssueStillWarns(t *testing.T) {
	state := makeState(&materialize.Issue{
		ID:     "ISSUE-1",
		Type:   "task",
		Status: "open",
		Scope:  []string{"pkg/a/foo.go", "pkg/b/bar.go", "pkg/c/baz.go"},
	})
	result := Validate(state, Options{})
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
		result := Validate(state, Options{PreExpandedScopes: nil})
		assert.False(t, containsInfo(result, "phantom scope"),
			"status=%s: phantom scope should be skipped for terminal status", status)
	}
}

func TestW10PhantomScope_BlockedStillChecked(t *testing.T) {
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
	result := Validate(state, Options{PreExpandedScopes: preExpandedScopes})
	assert.True(t, containsInfo(result, "phantom scope"),
		"blocked status should still trigger phantom scope warning")
}

func TestW10PhantomScope_EpicsAndStoriesWithTerminalStatusSkipped(t *testing.T) {
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
		result := Validate(state, Options{PreExpandedScopes: nil})
		assert.False(t, containsInfo(result, "phantom scope"),
			"type=%s status=done: phantom scope should be skipped for terminal status", issueType)
	}
}

func TestW10PhantomScope_NewSuffixSkipped(t *testing.T) {
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
	result := Validate(state, Options{PreExpandedScopes: preExpandedScopes})
	assert.False(t, containsInfo(result, "phantom scope"),
		"scope entries with (new) suffix should not trigger phantom scope warnings")
}

func TestW10PhantomScope_NewSuffixMixedWithExisting(t *testing.T) {
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
	result := Validate(state, Options{PreExpandedScopes: preExpandedScopes})
	// ghost.go is phantom (no (new) suffix, doesn't exist)
	assert.True(t, containsInfo(result, "phantom scope"),
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
	result := Validate(state, Options{PreExpandedScopes: preExpandedScopes})
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
	// Pass coverage data directly
	coverage := &traceability.Coverage{
		CitedNodes:  1,
		TotalNodes:  1,
		CoveragePct: 100,
	}

	state := makeState(&materialize.Issue{ID: "A"})
	result := Validate(state, Options{Coverage: coverage})
	assert.NotNil(t, result.Coverage)
	assert.Equal(t, 1, result.Coverage.CitedNodes)
}

// TestE5TypeHierarchy_SkipsTerminalStatus verifies that cancelled, done, and merged
// issues are not flagged for hierarchy violations — they have already been delivered.
func TestE5TypeHierarchy_SkipsTerminalStatus(t *testing.T) {
	for _, status := range []string{"cancelled", "done", "merged"} {
		t.Run("status="+status, func(t *testing.T) {
			// task parenting another task is normally invalid, but terminal tasks are exempt
			state := makeState(
				&materialize.Issue{ID: "TASK-1", Type: "task", Status: status, Children: []string{"TASK-2"}},
				&materialize.Issue{ID: "TASK-2", Type: "task", Parent: "TASK-1"},
			)
			result := Validate(state, Options{})
			assert.False(t, containsError(result, "invalid hierarchy"),
				"terminal parent (status=%s) must not trigger hierarchy error", status)
		})
	}
}

// TestE5TypeHierarchy_BugUnderStoryIsValid verifies that bug is a valid child of story.
func TestE5TypeHierarchy_BugUnderStoryIsValid(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "STORY-1", Type: "story", Children: []string{"BUG-1"}},
		&materialize.Issue{ID: "BUG-1", Type: "bug", Parent: "STORY-1"},
	)
	result := Validate(state, Options{})
	assert.False(t, containsError(result, "invalid hierarchy"), "bug under story should be valid")
}

// TestE5TypeHierarchy_BugUnderEpicIsValid verifies that bug is a valid child of epic.
func TestE5TypeHierarchy_BugUnderEpicIsValid(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "EPIC-1", Type: "epic", Children: []string{"BUG-1"}},
		&materialize.Issue{ID: "BUG-1", Type: "bug", Parent: "EPIC-1"},
	)
	result := Validate(state, Options{})
	assert.False(t, containsError(result, "invalid hierarchy"), "bug under epic should be valid")
}

// TestE5TypeHierarchy_BugUnderTaskIsInvalid verifies that bug cannot be parented under a task.
func TestE5TypeHierarchy_BugUnderTaskIsInvalid(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "TASK-1", Type: "task", Children: []string{"BUG-1"}},
		&materialize.Issue{ID: "BUG-1", Type: "bug", Parent: "TASK-1"},
	)
	result := Validate(state, Options{})
	assert.True(t, containsError(result, "invalid hierarchy"), "bug under task should be invalid")
}
