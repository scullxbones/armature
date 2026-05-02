package validate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/scullxbones/armature/internal/traceability"
)

type Options struct {
	ScopeID   string
	Strict    bool
	IssuesDir string
	StateDir  string
	RepoPath  string
}

type Result struct {
	OK       bool
	Errors   []string
	Warnings []string
	Infos    []string
	Coverage *traceability.Coverage
}

func Validate(state *materialize.State, opts Options) Result {
	var errors, warnings, infos []string

	targets := issueSubset(state, opts.ScopeID)

	errors = append(errors, checkE2E3ParentLinks(targets, state)...)
	errors = append(errors, checkE4Cycles(targets, state)...)
	errors = append(errors, checkE5TypeHierarchy(targets, state)...)
	errors = append(errors, checkE6RequiredFields(targets)...)
	errors = append(errors, checkE9DoDLength(targets)...)
	errors = append(errors, checkE10ScopeGlobs(targets)...)
	// TODO(E4-S3): E11 check not yet implemented — spec definition pending.

	if opts.IssuesDir != "" {
		errors = append(errors, checkE7E8E12Citations(targets, opts.IssuesDir)...)
	}

	warnings = append(warnings, checkW1ScopeOverlap(targets, state)...)
	warnings = append(warnings, checkW2NoTestCriteria(targets)...)
	warnings = append(warnings, checkW3BudgetExceeded(targets)...)
	warnings = append(warnings, checkW4BroadScope(targets)...)
	warnings = append(warnings, checkW5MissingContextFiles(targets)...)
	warnings = append(warnings, checkW6ComplexityMismatch(targets)...)
	warnings = append(warnings, checkW7VagueDoD(targets)...)
	warnings = append(warnings, checkW8ConflictingDecisions(targets)...)
	// TODO(E4-S3): W9 stale-heartbeat check not yet implemented —
	// should warn when a claimed issue's last heartbeat exceeds its ClaimTTL.
	warnings = append(warnings, checkW11VagueOutcomes(targets)...)

	if opts.RepoPath != "" {
		infos = append(infos, checkW10PhantomScope(targets, opts.RepoPath)...)
	}

	var cov *traceability.Coverage
	if opts.StateDir != "" {
		tracePath := filepath.Join(opts.StateDir, "traceability.json")
		c, err := traceability.Read(tracePath)
		if err == nil {
			cov = &c
		}
	}

	if opts.Strict {
		errors = append(errors, warnings...)
		warnings = nil
	}

	return Result{OK: len(errors) == 0, Errors: errors, Warnings: warnings, Infos: infos, Coverage: cov}
}

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

func checkE6RequiredFields(issues map[string]*materialize.Issue) []string {
	var errs []string
	for id, issue := range issues {
		if issue.Type != "task" {
			continue
		}
		// Terminal-status tasks have already been delivered; skip required-field checks.
		if issue.Status == ops.StatusMerged || issue.Status == ops.StatusDone || issue.Status == ops.StatusCancelled {
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

func checkE7E8E12Citations(issues map[string]*materialize.Issue, issuesDir string) []string {
	var errs []string
	manifest, err := readManifestForValidate(issuesDir)
	if err != nil {
		// Manifest absent or unreadable — citation checks skipped.
		// Run `trls sources add` to register sources before validating citations.
		if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Sprintf("citation check skipped: cannot read source manifest: %v", err))
		}
		return errs
	}
	for id, issue := range issues {
		if len(issue.SourceLinks) == 0 && len(issue.CitationAcceptances) == 0 {
			errs = append(errs, fmt.Sprintf("uncited node: %s", id))
			continue
		}
		for _, link := range issue.SourceLinks {
			if _, ok := manifest[link.SourceEntryID]; !ok {
				errs = append(errs, fmt.Sprintf("unknown source: %s in citation for %s", link.SourceEntryID, id))
			}
		}
	}
	return errs
}

func readManifestForValidate(issuesDir string) (map[string]struct{}, error) {
	sourcesPath := filepath.Join(issuesDir, "sources")
	m, err := sources.ReadManifest(sourcesPath)
	if err != nil {
		return nil, err
	}
	// ReadManifest returns empty Manifest (not error) when file is absent;
	// propagate a synthetic ErrNotExist so the caller skips citation checks.
	if len(m.Entries) == 0 {
		// Check whether the file actually exists to distinguish absent from empty.
		if _, statErr := os.Stat(filepath.Join(sourcesPath, "manifest.json")); os.IsNotExist(statErr) {
			return nil, os.ErrNotExist
		}
	}
	result := make(map[string]struct{}, len(m.Entries))
	for id := range m.Entries {
		result[id] = struct{}{}
	}
	return result, nil
}

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

func checkW1ScopeOverlap(issues map[string]*materialize.Issue, state *materialize.State) []string {
	var warns []string
	byParent := make(map[string][]*materialize.Issue)
	for _, issue := range issues {
		byParent[issue.Parent] = append(byParent[issue.Parent], issue)
	}
	for _, siblings := range byParent {
		for i, sib := range siblings {
			for _, other := range siblings[i+1:] {
				overlap := scopeIntersection(sib.Scope, other.Scope)
				if len(overlap) == 0 {
					continue
				}
				if hasSerialDependency(sib, other) {
					continue
				}
				warns = append(warns, fmt.Sprintf("scope overlap: %s and %s both modify %s",
					sib.ID, other.ID, strings.Join(overlap, ", ")))
			}
		}
	}
	_ = state
	return warns
}

// hasSerialDependency returns true if a blocks b or b blocks a,
// meaning the two tasks execute serially and a shared scope is intentional.
func hasSerialDependency(a, b *materialize.Issue) bool {
	for _, id := range a.Blocks {
		if id == b.ID {
			return true
		}
	}
	for _, id := range b.Blocks {
		if id == a.ID {
			return true
		}
	}
	return false
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

func checkW2NoTestCriteria(issues map[string]*materialize.Issue) []string {
	var warns []string
	for id, issue := range issues {
		if issue.Type != "task" || len(issue.Acceptance) == 0 {
			continue
		}
		var criteria []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(issue.Acceptance, &criteria); err != nil {
			continue
		}
		hasTest := false
		for _, c := range criteria {
			if c.Type == "test_passes" || c.Type == "manual_review" {
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

func checkW10PhantomScope(issues map[string]*materialize.Issue, repoPath string) []string {
	var warns []string
	for id, issue := range issues {
		// Terminal-status issues have already been delivered; their scope no longer needs to exist.
		if issue.Status == ops.StatusMerged || issue.Status == ops.StatusDone || issue.Status == ops.StatusCancelled {
			continue
		}
		for _, glob := range issue.Scope {
			matches, err := filepath.Glob(filepath.Join(repoPath, glob))
			if err != nil || len(matches) == 0 {
				warns = append(warns, fmt.Sprintf("phantom scope: %s on %s does not match any file", glob, id))
			}
		}
	}
	return warns
}

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
