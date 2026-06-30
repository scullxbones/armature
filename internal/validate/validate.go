package validate

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/issuetype"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/scullxbones/armature/internal/traceability"
)

type Options struct {
	ScopeID      string
	ParentID     string
	Strict       bool
	ManifestData []byte                 // Pre-read manifest bytes (may be nil/empty if citations not available)
	Coverage     *traceability.Coverage // Pre-loaded traceability coverage
	// PreExpandedScopes maps issue ID to pre-expanded file paths (for W10 phantom scope check)
	// If nil, W10 phantom scope checks are skipped
	PreExpandedScopes map[string][]string
}

type Result struct {
	OK       bool
	Errors   []string
	Warnings []string
	Infos    []string
	Coverage *traceability.Coverage
}

func Validate(state *materialize.State, graph *dag.Graph, opts Options) Result {
	var errors, warnings, infos []string

	targets := issueSubset(state, opts.ScopeID, graph)
	if opts.ParentID != "" {
		targets = parentFilter(targets, opts.ParentID)
	}

	errors = append(errors, checkE2E3ParentLinks(targets, state)...)
	errors = append(errors, checkE4Cycles(targets, graph)...)
	errors = append(errors, checkE5TypeHierarchy(targets, state)...)
	errors = append(errors, checkE6RequiredFields(targets)...)
	errors = append(errors, checkE9DoDLength(targets)...)
	errors = append(errors, checkE10ScopeGlobs(targets)...)
	// TODO(E4-S3): E11 check not yet implemented — spec definition pending.

	if len(opts.ManifestData) > 0 {
		errors = append(errors, checkE7E8E12Citations(targets, opts.ManifestData)...)
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

	if opts.PreExpandedScopes != nil {
		infos = append(infos, checkW10PhantomScope(targets, opts.PreExpandedScopes)...)
	}

	if opts.Strict {
		errors = append(errors, warnings...)
		warnings = nil
	}

	return Result{OK: len(errors) == 0, Errors: errors, Warnings: warnings, Infos: infos, Coverage: opts.Coverage}
}

func issueSubset(state *materialize.State, scopeID string, graph *dag.Graph) map[string]*materialize.Issue {
	if scopeID == "" {
		return state.Issues
	}
	subset := make(map[string]*materialize.Issue)
	// Include the root issue and all its descendants
	if issue, ok := state.Issues[scopeID]; ok {
		subset[scopeID] = issue
	}
	descendants := graph.Descendants(scopeID)
	for _, descID := range descendants {
		if desc, ok := state.Issues[descID]; ok {
			subset[descID] = desc
		}
	}
	return subset
}

func parentFilter(issues map[string]*materialize.Issue, parentID string) map[string]*materialize.Issue {
	subset := make(map[string]*materialize.Issue)
	for id, issue := range issues {
		if issue.Parent == parentID {
			subset[id] = issue
		}
	}
	return subset
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

func checkE4Cycles(issues map[string]*materialize.Issue, graph *dag.Graph) []string {
	var errs []string

	// Convert issues map to scope map for graph.ScopedHasCycle
	scope := make(map[string]bool)
	for id := range issues {
		scope[id] = true
	}

	// Check for cycles restricted to the scoped issue set (not the entire DAG).
	// This prevents false positives when a cycle exists elsewhere in the graph.
	for id := range issues {
		if graph.ScopedHasCycle(id, scope) {
			errs = append(errs, fmt.Sprintf("cycle detected: %s", id))
			break // Report only once to avoid redundant messages
		}
	}

	return errs
}

func checkE5TypeHierarchy(issues map[string]*materialize.Issue, state *materialize.State) []string {
	var errs []string
	for id, issue := range issues {
		// Terminal issues have already been delivered; skip hierarchy checks for them.
		if isTerminalStatus(issue.Status) {
			continue
		}
		for _, childID := range issue.Children {
			child, ok := state.Issues[childID]
			if !ok {
				continue
			}
			if isTerminalStatus(child.Status) {
				continue
			}
			if !issuetype.IsLegalHierarchy(issue.Type, child.Type) {
				errs = append(errs, fmt.Sprintf("invalid hierarchy: %s %s cannot parent %s %s",
					issue.Type, id, child.Type, childID))
			}
		}
	}
	return errs
}

// checkE6RequiredFields checks that each issue has the required fields for its type.
func checkE6RequiredFields(issues map[string]*materialize.Issue) []string {
	var errs []string
	for id, issue := range issues {
		// Terminal-status issues have already been delivered; skip required-field checks.
		if issue.Status == ops.StatusMerged || issue.Status == ops.StatusDone || issue.Status == ops.StatusCancelled {
			continue
		}
		for _, field := range issuetype.RequiredFields(issue.Type) {
			switch field {
			case "scope":
				if len(issue.Scope) == 0 {
					errs = append(errs, fmt.Sprintf("missing required field: scope on %s %s", issue.Type, id))
				}
			case "acceptance":
				if len(issue.Acceptance) == 0 || string(issue.Acceptance) == "null" {
					errs = append(errs, fmt.Sprintf("missing required field: acceptance on %s %s", issue.Type, id))
				}
			case "definition_of_done":
				if issue.DefinitionOfDone == "" {
					errs = append(errs, fmt.Sprintf("missing required field: definition_of_done on %s %s", issue.Type, id))
				}
			}
		}
	}
	return errs
}

func checkE7E8E12Citations(issues map[string]*materialize.Issue, manifestData []byte) []string {
	var errs []string

	manifest, err := parseManifestData(manifestData)
	if err != nil {
		errs = append(errs, fmt.Sprintf("citation check skipped: cannot parse source manifest: %v", err))
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

func parseManifestData(manifestData []byte) (map[string]struct{}, error) {
	m := sources.Manifest{}
	if err := json.Unmarshal(manifestData, &m); err != nil {
		return nil, err
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
		if issue.Type != "task" || isTerminalStatus(issue.Status) {
			continue
		}
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
	return slices.Contains(a.Blocks, b.ID) || slices.Contains(b.Blocks, a.ID)
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
		if issue.Status == ops.StatusMerged || issue.Status == ops.StatusDone || issue.Status == ops.StatusCancelled {
			continue
		}
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
		if issue.Status == ops.StatusMerged || issue.Status == ops.StatusDone || issue.Status == ops.StatusCancelled {
			continue
		}
		if len(issue.ContextFiles) > 0 {
			continue
		}
		dirs := make(map[string]struct{})
		for _, glob := range issue.Scope {
			dirs[filepath.Dir(glob)] = struct{}{}
		}
		if len(dirs) >= 3 {
			warns = append(warns, fmt.Sprintf(
				"missing context_files on %s with broad scope — split the task into smaller pieces or narrow scope via: arm amend %s --scope <glob>",
				id, id))
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
		seenByTopic := make(map[string]map[string]struct{})
		for _, d := range issue.Decisions {
			topic := strings.TrimSpace(d.Topic)
			choice := strings.TrimSpace(d.Choice)
			if topic == "" || choice == "" {
				continue
			}
			if seenByTopic[topic] == nil {
				seenByTopic[topic] = make(map[string]struct{})
			}
			if _, seen := seenByTopic[topic][choice]; seen {
				continue
			}
			seenByTopic[topic][choice] = struct{}{}
			byTopic[topic] = append(byTopic[topic], choice)
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

func isTerminalStatus(status string) bool {
	return status == ops.StatusMerged || status == ops.StatusDone || status == ops.StatusCancelled
}

func checkW10PhantomScope(issues map[string]*materialize.Issue, preExpandedScopes map[string][]string) []string {
	var warns []string
	for id, issue := range issues {
		// Terminal-status issues have already been delivered; their scope no longer needs to exist.
		if issue.Status == ops.StatusMerged || issue.Status == ops.StatusDone || issue.Status == ops.StatusCancelled {
			continue
		}
		expandedFiles, ok := preExpandedScopes[id]
		if !ok {
			// No pre-expanded data for this issue; skip check
			continue
		}

		// If expandedFiles is not empty, at least some globs matched files
		// If it's empty, no globs matched any files
		hasMatches := len(expandedFiles) > 0

		// Check each scope entry against the expanded files
		for _, entry := range issue.Scope {
			// Legacy ops may store multiple comma-separated paths as one entry; check each individually.
			for _, path := range splitSeq(entry, ", ") {
				path = strings.TrimSpace(path) // trim whitespace
				// "(new)" entries are planned files not yet created; skip them.
				if hasNewSuffix(path) {
					continue
				}
				// Determine if this path is phantom
				isPhantom := false
				if !hasMatches {
					// No files matched any globs — this entry is phantom
					isPhantom = true
				} else if !isGlobPattern(path) && !slices.Contains(expandedFiles, path) {
					// This is a literal path and it doesn't appear in the expanded files
					isPhantom = true
				}
				// If it's a glob pattern and we have matches, assume it matched (can't validate further without the glob)

				if isPhantom {
					warns = append(warns, fmt.Sprintf("phantom scope: %s on %s does not match any file", path, id))
				}
			}
		}
	}
	return warns
}

// isGlobPattern checks if a string contains glob characters
func isGlobPattern(s string) bool {
	return strings.ContainsAny(s, "*?[]")
}

// splitSeq is a helper that splits a string on a separator using strings.Split.
func splitSeq(s, sep string) []string {
	return strings.Split(s, sep)
}

// hasNewSuffix checks if a string ends with " (new)".
func hasNewSuffix(s string) bool {
	const newSuffix = " (new)"
	return len(s) >= len(newSuffix) && s[len(s)-len(newSuffix):] == newSuffix
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
		if slices.Contains(vagueOutcomes, lower) {
			warns = append(warns, fmt.Sprintf("vague outcome: %s outcome is %d chars", id, len(lower)))
		}
	}
	return warns
}
