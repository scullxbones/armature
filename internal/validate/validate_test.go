package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
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
}

func TestW10PhantomScope_TerminalStatusesSkipped(t *testing.T) {
	// Issues with merged, done, or cancelled status should not trigger phantom scope warnings
	// even if their scope globs match no files.
	dir := t.TempDir() // empty dir — no files match any glob
	for _, status := range []string{"merged", "done", "cancelled"} {
		state := makeState(
			&materialize.Issue{
				ID:     "TSK-1",
				Type:   "task",
				Status: status,
				Scope:  []string{"nonexistent/path/*.go"},
			},
		)
		result := Validate(state, Options{RepoPath: dir})
		assert.False(t, containsInfo(result, "phantom scope"),
			"status=%s: phantom scope should be skipped for terminal status", status)
	}
}

func TestW10PhantomScope_BlockedStillChecked(t *testing.T) {
	// Blocked issues are not terminal — their scope should still be validated.
	dir := t.TempDir() // empty dir — no files match any glob
	state := makeState(
		&materialize.Issue{
			ID:     "TSK-1",
			Type:   "task",
			Status: "blocked",
			Scope:  []string{"nonexistent/path/*.go"},
		},
	)
	result := Validate(state, Options{RepoPath: dir})
	assert.True(t, containsInfo(result, "phantom scope"),
		"blocked status should still trigger phantom scope warning")
}

func TestW10PhantomScope_EpicsAndStoriesWithTerminalStatusSkipped(t *testing.T) {
	// Terminal status applies across all issue types, not just tasks.
	dir := t.TempDir() // empty dir — no files match any glob
	for _, issueType := range []string{"epic", "story"} {
		state := makeState(
			&materialize.Issue{
				ID:     "ISSUE-1",
				Type:   issueType,
				Status: "done",
				Scope:  []string{"nonexistent/path/*.go"},
			},
		)
		result := Validate(state, Options{RepoPath: dir})
		assert.False(t, containsInfo(result, "phantom scope"),
			"type=%s status=done: phantom scope should be skipped for terminal status", issueType)
	}
}

func TestW10PhantomScope_NewSuffixSkipped(t *testing.T) {
	// Scope entries ending with " (new)" mark files not yet created; they should not
	// trigger phantom scope warnings because the file is intentionally planned, not missing.
	dir := t.TempDir()
	state := makeState(
		&materialize.Issue{
			ID:     "ISSUE-1",
			Type:   "task",
			Status: "open",
			Scope:  []string{"internal/adapters/files.go (new)", "internal/adapters/git.go (new)"},
		},
	)
	result := Validate(state, Options{RepoPath: dir})
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
	result := Validate(state, Options{RepoPath: dir})
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
	result := Validate(state, Options{RepoPath: dir})
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

func TestValidateUsesStateDir(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	// Write a mock traceability.json
	cov := `{"cited_nodes": 1, "total_nodes": 1, "coverage_pct": 100}`
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "traceability.json"), []byte(cov), 0644))

	state := makeState(&materialize.Issue{ID: "A"})
	result := Validate(state, Options{StateDir: stateDir})
	assert.NotNil(t, result.Coverage)
	assert.Equal(t, 1, result.Coverage.CitedNodes)
}
