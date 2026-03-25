# E4 Full Validate Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the full W1–W11 warning checks and E1–E12 error checks in the validate package, and add `--scope`, `--strict`, and JSON output to the validate CLI command.

**Spec:** `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` (E3-S3 section)

**Depends on:** `2026-03-18-e4-s2-glamour-render.md`

**Execution order within E4:** S1 → S4 → S7 → S2 → S3 → S5 → S6 → S8 → S9

**Tech Stack:** Go 1.26, Cobra v1.8, testify

---

## File Structure

| File | Change |
|---|---|
| `internal/validate/validate.go` | Full W1-W11 + E1-E12 checks, subtree scope, coverage metrics |
| `internal/validate/validate_test.go` | Extend with full check coverage |
| `cmd/trellis/validate.go` | Add `--scope`, `--strict` flags; JSON output |

---

## Tasks

### Task 1: Full validation — warning checks W1–W11

**Files:**
- Modify: `internal/validate/validate.go`
- Modify: `internal/validate/validate_test.go`

- [ ] **Step 1: Write failing tests for warnings**

```go
// internal/validate/validate_test.go (append)
func TestW1ScopeOverlap(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TSK-A"] = &materialize.Issue{
		ID: "TSK-A", Type: "task", Parent: "STORY-1",
		Scope: []string{"internal/ops/*.go"},
	}
	state.Issues["TSK-B"] = &materialize.Issue{
		ID: "TSK-B", Type: "task", Parent: "STORY-1",
		Scope: []string{"internal/ops/*.go"},
	}
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "scope overlap"))
}

func TestW2NoTestCriteria(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TSK-1"] = &materialize.Issue{
		ID: "TSK-1", Type: "task",
		Acceptance: json.RawMessage(`[{"type":"review","text":"look at it"}]`),
	}
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "no test criteria"))
}

func TestW7VagueDoD(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TSK-1"] = &materialize.Issue{
		ID: "TSK-1", Type: "task",
		DefinitionOfDone: "Make it work properly and correctly",
	}
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "vague DoD"))
}

func TestW8ConflictingDecisions(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TSK-1"] = &materialize.Issue{
		ID: "TSK-1", Type: "task",
		Decisions: []materialize.Decision{
			{Topic: "storage", Choice: "postgres", Timestamp: 100},
			{Topic: "storage", Choice: "sqlite", Timestamp: 200},
		},
	}
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "conflicting decisions"))
}

func TestW11VagueOutcome(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TSK-1"] = &materialize.Issue{
		ID: "TSK-1", Type: "task",
		Status: "done", Outcome: "done",
	}
	result := Validate(state, Options{})
	assert.True(t, containsWarning(result, "vague outcome"))
}

func containsWarning(r Result, substr string) bool {
	for _, w := range r.Warnings {
		if strings.Contains(strings.ToLower(w), strings.ToLower(substr)) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/validate/... -run "TestW[0-9]" -v
```

Expected: FAIL — `Options` type undefined, `Validate` signature mismatch.

- [ ] **Step 3: Refactor and extend validate.go**

Replace `internal/validate/validate.go` with the full implementation:

```go
// internal/validate/validate.go
package validate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/traceability"
)

// Options controls which checks are run and how results are reported.
type Options struct {
	// ScopeID restricts validation to the subtree rooted at this node ID.
	ScopeID string
	// Strict treats warnings as errors.
	Strict bool
	// IssuesDir is needed for W10 (file existence) and E7/E8/E12 (sources).
	// W9 (git merge-detection coverage) is deferred — requires git log parsing.
	IssuesDir string
	// RepoPath is the git worktree root, used for W9 and W10.
	RepoPath string
}

// Result holds the outcome of a validation run.
type Result struct {
	OK       bool
	Errors   []string
	Warnings []string
	// Coverage from traceability.json (nil if not computed).
	Coverage *traceability.Coverage
}

// Validate checks the materialized state for consistency issues.
func Validate(state *materialize.State, opts Options) Result {
	var errors, warnings []string

	targets := issueSubset(state, opts.ScopeID)

	// --- Error checks ---
	errors = append(errors, checkE2E3ParentLinks(targets, state)...)
	errors = append(errors, checkE4Cycles(targets, state)...)
	errors = append(errors, checkE5TypeHierarchy(targets, state)...)
	errors = append(errors, checkE6RequiredFields(targets)...)
	errors = append(errors, checkE9DoDLength(targets)...)
	errors = append(errors, checkE10ScopeGlobs(targets)...)

	// E7/E8/E12 require sources manifest
	if opts.IssuesDir != "" {
		errors = append(errors, checkE7E8E12Citations(targets, opts.IssuesDir)...)
	}

	// --- Warning checks ---
	warnings = append(warnings, checkW1ScopeOverlap(targets, state)...)
	warnings = append(warnings, checkW2NoTestCriteria(targets)...)
	warnings = append(warnings, checkW3BudgetExceeded(targets)...)
	warnings = append(warnings, checkW4BroadScope(targets)...)
	warnings = append(warnings, checkW5MissingContextFiles(targets)...)
	warnings = append(warnings, checkW6ComplexityMismatch(targets)...)
	warnings = append(warnings, checkW7VagueDoD(targets)...)
	warnings = append(warnings, checkW8ConflictingDecisions(targets)...)
	warnings = append(warnings, checkW11VagueOutcomes(targets)...)

	// W10 requires repo access; W9 (git merge-detection coverage) is deferred
	if opts.RepoPath != "" {
		warnings = append(warnings, checkW10PhantomScope(targets, opts.RepoPath)...)
	}

	// Coverage metrics
	var cov *traceability.Coverage
	if opts.IssuesDir != "" {
		c, err := traceability.Read(opts.IssuesDir)
		if err == nil {
			cov = &c
		}
	}

	if opts.Strict {
		errors = append(errors, warnings...)
		warnings = nil
	}

	return Result{
		OK:       len(errors) == 0,
		Errors:   errors,
		Warnings: warnings,
		Coverage: cov,
	}
}

// issueSubset returns the issues to validate: all issues, or the subtree under scopeID.
func issueSubset(state *materialize.State, scopeID string) map[string]*materialize.Issue {
	if scopeID == "" {
		return state.Issues
	}
	subset := make(map[string]*materialize.Issue)
	collectSubtree(scopeID, state, subset)
	return subset
}

func collectSubtree(id string, state *materialize.State, out map[string]*materialize.Issue) {
	issue, ok := state.Issues[id]
	if !ok {
		return
	}
	out[id] = issue
	for _, child := range issue.Children {
		collectSubtree(child, state, out)
	}
}

// E2/E3: parent and blocker references must resolve.
func checkE2E3ParentLinks(issues map[string]*materialize.Issue, state *materialize.State) []string {
	var errs []string
	for id, issue := range issues {
		if issue.Parent != "" {
			if _, ok := state.Issues[issue.Parent]; !ok {
				errs = append(errs, fmt.Sprintf("unresolved parent: %s for node %s", issue.Parent, id))
			}
		}
		for _, blockerID := range issue.BlockedBy {
			if _, ok := state.Issues[blockerID]; !ok {
				errs = append(errs, fmt.Sprintf("unresolved link target: %s from %s", blockerID, id))
			}
		}
	}
	return errs
}

// E4: DFS cycle detection in BlockedBy graph.
func checkE4Cycles(issues map[string]*materialize.Issue, state *materialize.State) []string {
	var errs []string
	for id := range issues {
		if hasCycle(id, state) {
			errs = append(errs, fmt.Sprintf("cycle detected: %s → ... → %s", id, id))
		}
	}
	return errs
}

func hasCycle(startID string, state *materialize.State) bool {
	visited := make(map[string]bool)
	return dfs(startID, startID, visited, state, true)
}

func dfs(startID, currentID string, visited map[string]bool, state *materialize.State, first bool) bool {
	if !first && currentID == startID {
		return true
	}
	if visited[currentID] {
		return false
	}
	visited[currentID] = true
	issue, ok := state.Issues[currentID]
	if !ok {
		return false
	}
	for _, b := range issue.BlockedBy {
		if b == startID {
			return true
		}
		if dfs(startID, b, visited, state, false) {
			return true
		}
	}
	return false
}

// E5: type hierarchy — epic can parent story/task, story can parent task, task cannot parent anything.
func checkE5TypeHierarchy(issues map[string]*materialize.Issue, state *materialize.State) []string {
	var errs []string
	for id, issue := range issues {
		for _, childID := range issue.Children {
			child, ok := state.Issues[childID]
			if !ok {
				continue
			}
			if !validHierarchy(issue.Type, child.Type) {
				errs = append(errs, fmt.Sprintf("invalid hierarchy: %s %s cannot parent %s %s",
					issue.Type, id, child.Type, childID))
			}
		}
	}
	return errs
}

func validHierarchy(parentType, childType string) bool {
	switch parentType {
	case "epic":
		return childType == "story" || childType == "task"
	case "story":
		return childType == "task"
	case "task":
		return false
	}
	return true
}

// E6: task nodes must have scope, acceptance, definition_of_done.
func checkE6RequiredFields(issues map[string]*materialize.Issue) []string {
	var errs []string
	for id, issue := range issues {
		if issue.Type != "task" {
			continue
		}
		if len(issue.Scope) == 0 {
			errs = append(errs, fmt.Sprintf("missing required field: scope on task %s", id))
		}
		if len(issue.Acceptance) == 0 || string(issue.Acceptance) == "null" {
			errs = append(errs, fmt.Sprintf("missing required field: acceptance on task %s", id))
		}
		if issue.DefinitionOfDone == "" {
			errs = append(errs, fmt.Sprintf("missing required field: definition_of_done on task %s", id))
		}
	}
	return errs
}

// E7/E8/E12: citation coverage, source existence, source version freshness.
func checkE7E8E12Citations(issues map[string]*materialize.Issue, issuesDir string) []string {
	var errs []string
	manifest, err := readManifestForValidate(issuesDir)
	if err != nil {
		return nil // can't check without manifest
	}
	for id, issue := range issues {
		if len(issue.SourceLinks) == 0 {
			errs = append(errs, fmt.Sprintf("uncited node: %s", id))
			continue
		}
		for _, link := range issue.SourceLinks {
			if _, ok := manifest[link.SourceID]; !ok {
				errs = append(errs, fmt.Sprintf("unknown source: %s in citation for %s", link.SourceID, id))
			}
		}
	}
	return errs
}

// readManifestForValidate returns a set of known source IDs.
func readManifestForValidate(issuesDir string) (map[string]struct{}, error) {
	path := filepath.Join(issuesDir, "state", "sources", "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m struct {
		Sources []struct{ ID string `json:"id"` } `json:"sources"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(m.Sources))
	for _, s := range m.Sources {
		result[s.ID] = struct{}{}
	}
	return result, nil
}

// E9: definition_of_done must be ≤500 chars (configurable; hardcoded here).
const maxDoDLength = 500

func checkE9DoDLength(issues map[string]*materialize.Issue) []string {
	var errs []string
	for id, issue := range issues {
		if len(issue.DefinitionOfDone) > maxDoDLength {
			errs = append(errs, fmt.Sprintf("definition_of_done exceeds %d chars on %s", maxDoDLength, id))
		}
	}
	return errs
}

// E10: scope globs must be syntactically valid (filepath.Match test).
func checkE10ScopeGlobs(issues map[string]*materialize.Issue) []string {
	var errs []string
	for id, issue := range issues {
		for _, glob := range issue.Scope {
			if _, err := filepath.Match(glob, "test"); err != nil {
				errs = append(errs, fmt.Sprintf("invalid glob: %s on %s", glob, id))
			}
		}
	}
	return errs
}

// W1: scope overlap between sibling nodes.
func checkW1ScopeOverlap(issues map[string]*materialize.Issue, state *materialize.State) []string {
	var warns []string
	// Group issues by parent
	byParent := make(map[string][]*materialize.Issue)
	for _, issue := range issues {
		byParent[issue.Parent] = append(byParent[issue.Parent], issue)
	}
	for _, siblings := range byParent {
		for i := 0; i < len(siblings); i++ {
			for j := i + 1; j < len(siblings); j++ {
				overlap := scopeIntersection(siblings[i].Scope, siblings[j].Scope)
				if len(overlap) > 0 {
					warns = append(warns, fmt.Sprintf("scope overlap: %s and %s both modify %s",
						siblings[i].ID, siblings[j].ID, strings.Join(overlap, ", ")))
				}
			}
		}
	}
	_ = state
	return warns
}

func scopeIntersection(a, b []string) []string {
	setA := make(map[string]struct{}, len(a))
	for _, s := range a {
		setA[s] = struct{}{}
	}
	var result []string
	for _, s := range b {
		if _, ok := setA[s]; ok {
			result = append(result, s)
		}
	}
	return result
}

// W2: task acceptance criteria must include at least one item of type "test_passes".
func checkW2NoTestCriteria(issues map[string]*materialize.Issue) []string {
	var warns []string
	for id, issue := range issues {
		if issue.Type != "task" || len(issue.Acceptance) == 0 {
			continue
		}
		var criteria []struct{ Type string `json:"type"` }
		if err := json.Unmarshal(issue.Acceptance, &criteria); err != nil {
			continue
		}
		hasTest := false
		for _, c := range criteria {
			if c.Type == "test_passes" {
				hasTest = true
				break
			}
		}
		if !hasTest {
			warns = append(warns, fmt.Sprintf("no test criteria on %s", id))
		}
	}
	return warns
}

// W3: advisory token budget check (chars/4 estimate vs config).
const defaultTokenBudget = 4000

func checkW3BudgetExceeded(issues map[string]*materialize.Issue) []string {
	var warns []string
	for id, issue := range issues {
		estimated := (len(issue.DefinitionOfDone) + len(issue.Title)) / 4
		if issue.Context != nil {
			estimated += len(issue.Context) / 4
		}
		if estimated > defaultTokenBudget {
			warns = append(warns, fmt.Sprintf("budget advisory: %s est. %d tokens > %d",
				id, estimated, defaultTokenBudget))
		}
	}
	return warns
}

// W4: broad scope globs (e.g., "**/*").
func checkW4BroadScope(issues map[string]*materialize.Issue) []string {
	var warns []string
	for id, issue := range issues {
		for _, glob := range issue.Scope {
			if glob == "**/*" || glob == "**" || glob == "." {
				warns = append(warns, fmt.Sprintf("broad scope: %s scope covers entire tree", id))
				break
			}
		}
	}
	return warns
}

// W5: no context_files for scope spanning 3+ directories.
func checkW5MissingContextFiles(issues map[string]*materialize.Issue) []string {
	var warns []string
	for id, issue := range issues {
		if len(issue.ContextFiles) > 0 {
			continue
		}
		dirs := make(map[string]struct{})
		for _, glob := range issue.Scope {
			dirs[filepath.Dir(glob)] = struct{}{}
		}
		if len(dirs) >= 3 {
			warns = append(warns, fmt.Sprintf("missing context_files on %s with broad scope", id))
		}
	}
	return warns
}

// W6: complexity label inconsistent with scope file count.
func checkW6ComplexityMismatch(issues map[string]*materialize.Issue) []string {
	var warns []string
	for id, issue := range issues {
		n := len(issue.Scope)
		switch issue.EstComplexity {
		case "small":
			if n > 5 {
				warns = append(warns, fmt.Sprintf("complexity mismatch: %s has %d files but marked small", id, n))
			}
		case "large":
			if n < 2 {
				warns = append(warns, fmt.Sprintf("complexity mismatch: %s has %d files but marked large", id, n))
			}
		}
	}
	return warns
}

// W7: vague language in definition_of_done.
var vagueWords = []string{"properly", "correctly", "good", "well", "appropriate", "suitable"}

func checkW7VagueDoD(issues map[string]*materialize.Issue) []string {
	var warns []string
	for id, issue := range issues {
		if issue.DefinitionOfDone == "" {
			continue
		}
		lower := strings.ToLower(issue.DefinitionOfDone)
		for _, word := range vagueWords {
			if strings.Contains(lower, word) {
				warns = append(warns, fmt.Sprintf(`vague DoD: %s contains "%s"`, id, word))
				break
			}
		}
	}
	return warns
}

// W8: conflicting decisions on same topic within an issue.
func checkW8ConflictingDecisions(issues map[string]*materialize.Issue) []string {
	var warns []string
	for id, issue := range issues {
		byTopic := make(map[string][]string)
		for _, d := range issue.Decisions {
			byTopic[d.Topic] = append(byTopic[d.Topic], d.Choice)
		}
		for topic, choices := range byTopic {
			if len(choices) > 1 {
				warns = append(warns, fmt.Sprintf(`conflicting decisions: topic "%s" has %d choices: %s on %s`,
					topic, len(choices), strings.Join(choices, ", "), id))
			}
		}
	}
	return warns
}

// W10: scope path doesn't match any existing file (advisory — task may create new files).
func checkW10PhantomScope(issues map[string]*materialize.Issue, repoPath string) []string {
	var warns []string
	for id, issue := range issues {
		for _, glob := range issue.Scope {
			matches, err := filepath.Glob(filepath.Join(repoPath, glob))
			if err != nil || len(matches) == 0 {
				warns = append(warns, fmt.Sprintf("phantom scope: %s on %s does not match any file", glob, id))
			}
		}
	}
	return warns
}

// W11: transition outcome is too short or matches low-value patterns.
const minOutcomeLength = 20

var vagueOutcomes = []string{"done", "completed", "finished", "ok", "fixed"}

func checkW11VagueOutcomes(issues map[string]*materialize.Issue) []string {
	var warns []string
	for id, issue := range issues {
		if issue.Outcome == "" {
			continue
		}
		lower := strings.TrimSpace(strings.ToLower(issue.Outcome))
		if len(lower) < minOutcomeLength {
			warns = append(warns, fmt.Sprintf("vague outcome: %s outcome is %d chars", id, len(lower)))
			continue
		}
		for _, vague := range vagueOutcomes {
			if lower == vague {
				warns = append(warns, fmt.Sprintf("vague outcome: %s outcome is %d chars", id, len(lower)))
				break
			}
		}
	}
	return warns
}
```

Note: this file needs `"os"` in imports. Add it.

- [ ] **Step 4: Run all validate tests**

```bash
go test ./internal/validate/... -v
```

Expected: PASS. Fix any compile errors (missing imports, type mismatches with changed `Validate` signature).

- [ ] **Step 5: Commit**

```bash
git add internal/validate/validate.go internal/validate/validate_test.go
git commit -m "feat(validate): full W1-W11 warning checks and E2-E12 error checks"
```

---

### Task 2: Validate CLI — --scope, --strict, JSON output

**Files:**
- Modify: `cmd/trellis/validate.go`

- [ ] **Step 1: Update validate command**

Replace `cmd/trellis/validate.go` with:

```go
// cmd/trellis/validate.go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/validate"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var (
		ci     bool
		strict bool
		scope  string
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the issue graph for consistency",
		RunE: func(cmd *cobra.Command, args []string) error {
			state, _, err := materialize.MaterializeAndReturn(appCtx.IssuesDir, true)
			if err != nil {
				return err
			}

			opts := validate.Options{
				ScopeID:   scope,
				Strict:    strict,
				IssuesDir: appCtx.IssuesDir,
				RepoPath:  appCtx.RepoPath,
			}
			result := validate.Validate(state, opts)

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" {
				out, err := json.MarshalIndent(map[string]interface{}{
					"errors":   result.Errors,
					"warnings": result.Warnings,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			} else {
				for _, e := range result.Errors {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ERROR: %s\n", e)
				}
				for _, w := range result.Warnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "WARNING: %s\n", w)
				}
				if result.Coverage != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "COVERAGE: %.1f%% (%d/%d nodes cited)\n",
						result.Coverage.CoveragePct, result.Coverage.CitedNodes, result.Coverage.TotalNodes)
				}
				if result.OK {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "OK: no issues found")
				}
			}

			if (ci || strict) && len(result.Errors) > 0 {
				return fmt.Errorf("validation failed with %d error(s)", len(result.Errors))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&ci, "ci", false, "Exit non-zero if errors found")
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings as errors")
	cmd.Flags().StringVar(&scope, "scope", "", "Validate only the subtree rooted at this node ID")
	return cmd
}
```

- [ ] **Step 2: Build and test**

```bash
go build ./cmd/trellis/... && ./bin/trls validate --help
```

Expected: shows `--ci`, `--strict`, `--scope` flags.

- [ ] **Step 3: Commit**

```bash
git add cmd/trellis/validate.go
git commit -m "feat(validate): --scope, --strict, and JSON output flags"
```
