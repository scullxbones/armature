// Package validate implements armature's DAG and citation integrity checks (arm validate / arm doctor), reporting overlap, cycle, and coverage errors.
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
	"github.com/scullxbones/armature/internal/scopematch"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/scullxbones/armature/internal/traceability"
)

type Options struct {
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
	Findings []Finding
	Coverage *traceability.Coverage
}

// Finding is a Graph Finding: a rule violation arm validate reports,
// identified by a rule and the issue IDs it cites.
type Finding struct {
	Severity string
	Rule     string
	Message  string
	CitedIDs []string
}

func (f Finding) identity() string {
	ids := append([]string(nil), f.CitedIDs...)
	slices.Sort(ids)
	return f.Rule + "\x00" + strings.Join(ids, "\x00") + "\x00" + f.Message
}

func Validate(state *materialize.State, graph *dag.Graph, opts Options) Result {
	var findings []Finding

	targets := state.Issues

	findings = append(findings, checkE2E3ParentLinks(targets, state)...)
	findings = append(findings, checkE4Cycles(targets, graph)...)
	findings = append(findings, checkE5TypeHierarchy(targets, state)...)
	findings = append(findings, checkE6RequiredFields(targets)...)
	findings = append(findings, checkE9DoDLength(targets)...)
	findings = append(findings, checkE10ScopeGlobs(targets)...)
	// TODO(E4-S3): E11 check not yet implemented — spec definition pending.

	if len(opts.ManifestData) > 0 {
		findings = append(findings, checkE7E8E12Citations(targets, opts.ManifestData)...)
	}

	findings = append(findings, checkW1ScopeOverlap(targets, state)...)
	findings = append(findings, checkW2NoTestCriteria(targets)...)
	findings = append(findings, checkW3BudgetExceeded(targets)...)
	findings = append(findings, checkW4BroadScope(targets)...)
	findings = append(findings, checkW5MissingContextFiles(targets)...)
	findings = append(findings, checkW6ComplexityMismatch(targets)...)
	findings = append(findings, checkW7VagueDoD(targets)...)
	findings = append(findings, checkW8ConflictingDecisions(targets)...)
	// TODO(E4-S3): W9 stale-heartbeat check not yet implemented —
	// should warn when a claimed issue's last heartbeat exceeds its ClaimTTL.
	findings = append(findings, checkW11VagueOutcomes(targets)...)

	if opts.PreExpandedScopes != nil {
		findings = append(findings, checkW10PhantomScope(targets, opts.PreExpandedScopes, state.Issues)...)
	}

	var errors, warnings, infos []string
	for _, f := range findings {
		switch f.Severity {
		case "error":
			errors = append(errors, f.Message)
		case "warning":
			warnings = append(warnings, f.Message)
		default:
			infos = append(infos, f.Message)
		}
	}

	ok := len(errors) == 0
	if opts.Strict {
		ok = ok && len(warnings) == 0
	}

	return Result{OK: ok, Errors: errors, Warnings: warnings, Infos: infos, Findings: findings, Coverage: opts.Coverage}
}

// CheckIntroduction refuses a proposed write that introduces a Graph Finding
// on an issue the write created or targeted. Pre-existing findings on
// foreign IDs do not block.
func CheckIntroduction(current *materialize.State, proposed []ops.Op, opts Options) error {
	if current == nil {
		current = materialize.NewState()
	}
	before := Validate(current, materialize.GraphFromState(current), opts)
	afterState, err := projectState(current, proposed)
	if err != nil {
		return err
	}
	after := Validate(afterState, materialize.GraphFromState(afterState), opts)
	introduced := IntroducedOnTargets(before, after, targetedIDs(proposed))
	if len(introduced) == 0 {
		return nil
	}
	return formatIntroductionError(introduced)
}

// IntroducedOnTargets returns after-findings that were not present before
// and that cite at least one targeted issue ID. Infos never block.
func IntroducedOnTargets(before, after Result, targeted []string) []Finding {
	prior := make(map[string]struct{}, len(before.Findings))
	for _, f := range before.Findings {
		prior[f.identity()] = struct{}{}
	}
	targetSet := make(map[string]struct{}, len(targeted))
	for _, id := range targeted {
		if id != "" {
			targetSet[id] = struct{}{}
		}
	}
	var out []Finding
	for _, f := range after.Findings {
		if f.Severity == "info" {
			continue
		}
		// Cite-after remains legal on create (Plan Release / Integration).
		switch f.Rule {
		case "E7", "E8":
			continue
		}
		if _, ok := prior[f.identity()]; ok {
			continue
		}
		if citesAny(f.CitedIDs, targetSet) {
			out = append(out, f)
		}
	}
	return out
}

func projectState(current *materialize.State, proposed []ops.Op) (*materialize.State, error) {
	after, err := cloneState(current)
	if err != nil {
		return nil, err
	}
	if err := materialize.ApplyOpsSorted(after, proposed); err != nil {
		return nil, fmt.Errorf("project proposed: %w", err)
	}
	return after, nil
}

func cloneState(src *materialize.State) (*materialize.State, error) {
	if src == nil {
		return materialize.NewState(), nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("clone state: %w", err)
	}
	dst := materialize.NewState()
	if err := json.Unmarshal(data, dst); err != nil {
		return nil, fmt.Errorf("clone state: %w", err)
	}
	if dst.Issues == nil {
		dst.Issues = make(map[string]*materialize.Issue)
	}
	return dst, nil
}

func targetedIDs(proposed []ops.Op) []string {
	seen := make(map[string]struct{})
	var ids []string
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, op := range proposed {
		add(op.TargetID)
		add(op.Payload.Dep)
		switch op.Type {
		case ops.OpCreate, ops.OpReparent:
			add(op.Payload.Parent)
		}
	}
	return ids
}

func citesAny(cited []string, targets map[string]struct{}) bool {
	for _, id := range cited {
		if _, ok := targets[id]; ok {
			return true
		}
	}
	return false
}

func formatIntroductionError(findings []Finding) error {
	f := findings[0]
	var b strings.Builder
	b.WriteString("cannot introduce Graph Finding")
	if len(f.CitedIDs) > 0 {
		fmt.Fprintf(&b, " on %s", strings.Join(f.CitedIDs, ", "))
	}
	fmt.Fprintf(&b, ": %s", f.Message)
	if len(findings) > 1 {
		fmt.Fprintf(&b, " (and %d more)", len(findings)-1)
	}
	b.WriteString("\nFix the finding (narrow --scope, add context_files, or arm link), or withdraw the draft (arm dag revert / arm transition --to cancelled).")
	return fmt.Errorf("%s", b.String())
}

func checkE2E3ParentLinks(issues map[string]*materialize.Issue, state *materialize.State) []Finding {
	var findings []Finding
	for id, issue := range issues {
		if issue.Parent != "" {
			if _, ok := state.Issues[issue.Parent]; !ok {
				findings = append(findings, Finding{
					Severity: "error", Rule: "E2",
					Message:  fmt.Sprintf("unresolved parent: %s for node %s", issue.Parent, id),
					CitedIDs: []string{id, issue.Parent},
				})
			}
		}
		for _, blockerID := range issue.BlockedBy {
			if _, ok := state.Issues[blockerID]; !ok {
				findings = append(findings, Finding{
					Severity: "error", Rule: "E3",
					Message:  fmt.Sprintf("unresolved link target: %s from %s", blockerID, id),
					CitedIDs: []string{id, blockerID},
				})
			}
		}
	}
	return findings
}

func checkE4Cycles(issues map[string]*materialize.Issue, graph *dag.Graph) []Finding {
	scope := make(map[string]bool)
	for id := range issues {
		scope[id] = true
	}

	// Cite every node that participates in a scoped cycle so a new edge that
	// closes a loop naming an old node still counts as introduced.
	var cyclic []string
	for id := range issues {
		if graph.ScopedHasCycle(id, scope) {
			cyclic = append(cyclic, id)
		}
	}
	if len(cyclic) == 0 {
		return nil
	}
	slices.Sort(cyclic)
	return []Finding{{
		Severity: "error", Rule: "E4",
		Message:  fmt.Sprintf("cycle detected: %s", strings.Join(cyclic, ", ")),
		CitedIDs: cyclic,
	}}
}

func checkE5TypeHierarchy(issues map[string]*materialize.Issue, state *materialize.State) []Finding {
	var findings []Finding
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
				findings = append(findings, Finding{
					Severity: "error", Rule: "E5",
					Message: fmt.Sprintf("invalid hierarchy: %s %s cannot parent %s %s",
						issue.Type, id, child.Type, childID),
					CitedIDs: []string{id, childID},
				})
			}
		}
	}
	return findings
}

// checkE6RequiredFields checks that each issue has the required fields for its type.
func checkE6RequiredFields(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
	for id, issue := range issues {
		// Terminal-status issues have already been delivered; skip required-field checks.
		if issue.Status == ops.StatusMerged || issue.Status == ops.StatusDone || issue.Status == ops.StatusCancelled {
			continue
		}
		for _, field := range issuetype.RequiredFields(issue.Type) {
			switch field {
			case "scope":
				if len(issue.Scope) == 0 {
					findings = append(findings, Finding{
						Severity: "error", Rule: "E6",
						Message:  fmt.Sprintf("missing required field: scope on %s %s", issue.Type, id),
						CitedIDs: []string{id},
					})
				}
			case "acceptance":
				if len(issue.Acceptance) == 0 || string(issue.Acceptance) == "null" {
					findings = append(findings, Finding{
						Severity: "error", Rule: "E6",
						Message:  fmt.Sprintf("missing required field: acceptance on %s %s", issue.Type, id),
						CitedIDs: []string{id},
					})
				}
			case "definition_of_done":
				if issue.DefinitionOfDone == "" {
					findings = append(findings, Finding{
						Severity: "error", Rule: "E6",
						Message:  fmt.Sprintf("missing required field: definition_of_done on %s %s", issue.Type, id),
						CitedIDs: []string{id},
					})
				}
			}
		}
	}
	return findings
}

func checkE7E8E12Citations(issues map[string]*materialize.Issue, manifestData []byte) []Finding {
	var findings []Finding

	manifest, err := parseManifestData(manifestData)
	if err != nil {
		findings = append(findings, Finding{
			Severity: "error", Rule: "E7",
			Message: fmt.Sprintf("citation check skipped: cannot parse source manifest: %v", err),
		})
		return findings
	}

	for id, issue := range issues {
		if len(issue.SourceLinks) == 0 && len(issue.CitationAcceptances) == 0 {
			findings = append(findings, Finding{
				Severity: "error", Rule: "E7",
				Message:  fmt.Sprintf("uncited node: %s", id),
				CitedIDs: []string{id},
			})
			continue
		}
		for _, link := range issue.SourceLinks {
			if _, ok := manifest[link.SourceEntryID]; !ok {
				findings = append(findings, Finding{
					Severity: "error", Rule: "E8",
					Message:  fmt.Sprintf("unknown source: %s in citation for %s", link.SourceEntryID, id),
					CitedIDs: []string{id},
				})
			}
		}
	}
	return findings
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

func checkE9DoDLength(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
	for id, issue := range issues {
		if len(issue.DefinitionOfDone) > maxDoDLength {
			findings = append(findings, Finding{
				Severity: "error", Rule: "E9",
				Message:  fmt.Sprintf("definition_of_done exceeds %d chars on %s", maxDoDLength, id),
				CitedIDs: []string{id},
			})
		}
	}
	return findings
}

func checkE10ScopeGlobs(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
	for id, issue := range issues {
		for _, glob := range issue.Scope {
			if _, err := filepath.Match(glob, "test"); err != nil {
				findings = append(findings, Finding{
					Severity: "error", Rule: "E10",
					Message:  fmt.Sprintf("invalid glob: %s on %s", glob, id),
					CitedIDs: []string{id},
				})
			}
		}
	}
	return findings
}

func checkW1ScopeOverlap(issues map[string]*materialize.Issue, state *materialize.State) []Finding {
	var findings []Finding

	// Collect all active (non-terminal) tasks across all stories
	var tasks []*materialize.Issue
	for _, issue := range issues {
		if issue.Type != "task" || isTerminalStatus(issue.Status) {
			continue
		}
		tasks = append(tasks, issue)
	}

	// Index direct ordering edges from the full issue set (not the possibly
	// scope-narrowed `issues` subset). Reachability is then traversed only for
	// overlapping task pairs, so chains through an out-of-scope issue still
	// suppress a warning without materializing an all-pairs transitive closure.
	blocks := directBlocks(state.Issues)

	// Compare all pairs of tasks (including cross-story pairs).
	// Use i < j to avoid duplicate reporting of the same pair.
	for i, task1 := range tasks {
		for j := i + 1; j < len(tasks); j++ {
			task2 := tasks[j]
			matchedA, matchedB, overlaps := firstGlobOverlapPair(task1.Scope, task2.Scope)
			if !overlaps {
				continue
			}
			if hasSerialDependency(task1, task2, blocks) {
				continue
			}
			overlap := scopeIntersection(task1.Scope, task2.Scope)
			var detail string
			if len(overlap) > 0 {
				detail = strings.Join(overlap, ", ")
			} else {
				detail = fmt.Sprintf("%s <-> %s", matchedA, matchedB)
			}
			findings = append(findings, Finding{
				Severity: "warning", Rule: "W1",
				Message: fmt.Sprintf("scope overlap: %s and %s both modify %s",
					task1.ID, task2.ID, detail),
				CitedIDs: []string{task1.ID, task2.ID},
			})
		}
	}
	return findings
}

// directBlocks indexes the direct blocks relationship. It accepts both Blocks
// and BlockedBy because legacy materialized state can contain only one side of
// an otherwise equivalent ordering edge.
func directBlocks(issues map[string]*materialize.Issue) map[string][]string {
	blocks := make(map[string][]string)
	for id, issue := range issues {
		for _, blockedID := range issue.Blocks {
			if _, ok := issues[blockedID]; ok {
				blocks[id] = append(blocks[id], blockedID)
			}
		}
		for _, blockerID := range issue.BlockedBy {
			if _, ok := issues[blockerID]; ok {
				blocks[blockerID] = append(blocks[blockerID], id)
			}
		}
	}
	return blocks
}

// hasSerialDependency returns true if a blocks b or b blocks a, directly or
// transitively. It traverses only the candidate pair's reachable subgraph.
func hasSerialDependency(a, b *materialize.Issue, blocks map[string][]string) bool {
	return blocksReachable(a.ID, b.ID, blocks) || blocksReachable(b.ID, a.ID, blocks)
}

func blocksReachable(start, target string, blocks map[string][]string) bool {
	visited := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, blockedID := range blocks[current] {
			if blockedID == target {
				return true
			}
			if !visited[blockedID] {
				visited[blockedID] = true
				queue = append(queue, blockedID)
			}
		}
	}
	return false
}

// firstGlobOverlapPair returns the first pair of patterns (one from a, one from
// b) found to overlap via scopematch.Overlaps, matching claim-time semantics
// (claim.ScopesOverlap) rather than exact string equality: a glob like
// "cmd/armature/*.go" and a literal "cmd/armature/claim.go" must be recognized
// as overlapping so validate can't pass a claim that would later be rejected
// by claim.ScopesOverlap. Overlap matching is delegated to
// internal/scopematch — the single canonical implementation shared with
// internal/claim — rather than duplicated locally, so the two layers cannot
// diverge again as they once did. internal/scopematch is a leaf package with
// no dependency on the orchestration-layer internal/claim package, so it is
// safe under the validate-boundary depguard rule.
//
// firstGlobOverlapPair also reports whether any overlap was found at all.
// Used so warning messages can report the specific pattern pair that
// matched, rather than dumping both full scope lists when the overlap was
// only detected via glob matching (scopeIntersection's exact-string
// comparison found nothing).
func firstGlobOverlapPair(a, b []string) (patternA, patternB string, overlaps bool) {
	for _, x := range a {
		for _, y := range b {
			if scopematch.Overlaps(x, y) {
				return x, y, true
			}
		}
	}
	return "", "", false
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

func checkW2NoTestCriteria(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
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
			findings = append(findings, Finding{
				Severity: "warning", Rule: "W2",
				Message:  fmt.Sprintf("no test criteria on %s", id),
				CitedIDs: []string{id},
			})
		}
	}
	return findings
}

const defaultTokenBudget = 4000

func checkW3BudgetExceeded(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
	for id, issue := range issues {
		estimated := (len(issue.DefinitionOfDone) + len(issue.Title)) / 4
		if issue.Context != nil {
			estimated += len(issue.Context) / 4
		}
		if estimated > defaultTokenBudget {
			findings = append(findings, Finding{
				Severity: "warning", Rule: "W3",
				Message: fmt.Sprintf("budget advisory: %s est. %d tokens > %d",
					id, estimated, defaultTokenBudget),
				CitedIDs: []string{id},
			})
		}
	}
	return findings
}

func checkW4BroadScope(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
	for id, issue := range issues {
		if issue.Status == ops.StatusMerged || issue.Status == ops.StatusDone || issue.Status == ops.StatusCancelled {
			continue
		}
		for _, glob := range issue.Scope {
			if glob == "**/*" || glob == "**" || glob == "." {
				findings = append(findings, Finding{
					Severity: "warning", Rule: "W4",
					Message:  fmt.Sprintf("broad scope: %s scope covers entire tree", id),
					CitedIDs: []string{id},
				})
				break
			}
		}
	}
	return findings
}

func checkW5MissingContextFiles(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
	for id, issue := range issues {
		// Container types span many directories by design. Allowlist so a
		// future or typo'd type still gets the check.
		if isW5ContainerType(issue.Type) {
			continue
		}
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
			findings = append(findings, Finding{
				Severity: "warning", Rule: "W5",
				Message: fmt.Sprintf(
					"missing context_files on %s with broad scope — split the task into smaller pieces or narrow scope via: arm amend %s --scope <glob>",
					id, id),
				CitedIDs: []string{id},
			})
		}
	}
	return findings
}

func isW5ContainerType(typ string) bool {
	return typ == "story" || typ == "epic" || typ == "feature"
}

func checkW6ComplexityMismatch(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
	for id, issue := range issues {
		n := len(issue.Scope)
		switch issue.EstComplexity {
		case "small":
			if n > 5 {
				findings = append(findings, Finding{
					Severity: "warning", Rule: "W6",
					Message:  fmt.Sprintf("complexity mismatch: %s has %d files but marked small", id, n),
					CitedIDs: []string{id},
				})
			}
		case "large":
			if n < 2 {
				findings = append(findings, Finding{
					Severity: "warning", Rule: "W6",
					Message:  fmt.Sprintf("complexity mismatch: %s has %d files but marked large", id, n),
					CitedIDs: []string{id},
				})
			}
		}
	}
	return findings
}

var vagueWords = []string{"properly", "correctly", "good", "well", "appropriate", "suitable"}

func checkW7VagueDoD(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
	for id, issue := range issues {
		if issue.DefinitionOfDone == "" {
			continue
		}
		lower := strings.ToLower(issue.DefinitionOfDone)
		for _, word := range vagueWords {
			if strings.Contains(lower, word) {
				findings = append(findings, Finding{
					Severity: "warning", Rule: "W7",
					Message:  fmt.Sprintf(`vague DoD: %s contains "%s"`, id, word),
					CitedIDs: []string{id},
				})
				break
			}
		}
	}
	return findings
}

func checkW8ConflictingDecisions(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
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
				findings = append(findings, Finding{
					Severity: "warning", Rule: "W8",
					Message: fmt.Sprintf(`conflicting decisions: topic "%s" has %d choices: %s on %s`,
						topic, len(choices), strings.Join(choices, ", "), id),
					CitedIDs: []string{id},
				})
			}
		}
	}
	return findings
}

func isTerminalStatus(status string) bool {
	return status == ops.StatusMerged || status == ops.StatusDone || status == ops.StatusCancelled
}

func checkW10PhantomScope(issues map[string]*materialize.Issue, preExpandedScopes map[string][]string, allIssues map[string]*materialize.Issue) []Finding {
	var findings []Finding
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

		// Collect all "(new)" files declared by blocking tasks. Traverse from the
		// full issue set so a legitimate blocker is found even when the caller
		// passed a narrowed target map (library tests).
		blockerNewFiles := collectBlockerNewFiles(issue, allIssues)

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
					// Check if a blocker declares this file with "(new)" suffix
					// If yes, suppress the phantom scope warning since the file is legitimately
					// pending upstream creation
					if !blockerNewFiles[path] {
						findings = append(findings, Finding{
							Severity: "info", Rule: "W10",
							Message:  fmt.Sprintf("phantom scope: %s on %s does not match any file", path, id),
							CitedIDs: []string{id},
						})
					}
				}
			}
		}
	}
	return findings
}

// collectBlockerNewFiles gathers all "(new)"-annotated files declared by an issue's blocking tasks.
// It returns a map where the key is the filename (without " (new)") and the value indicates it was found.
func collectBlockerNewFiles(issue *materialize.Issue, issues map[string]*materialize.Issue) map[string]bool {
	result := make(map[string]bool)
	// Use a simple queue-based approach to handle transitive blockers
	toVisit := make([]string, len(issue.BlockedBy))
	copy(toVisit, issue.BlockedBy)
	visited := make(map[string]bool)

	for len(toVisit) > 0 {
		blockerID := toVisit[0]
		toVisit = toVisit[1:]

		// Skip if already visited (avoid cycles)
		if visited[blockerID] {
			continue
		}
		visited[blockerID] = true

		blocker, ok := issues[blockerID]
		if !ok {
			// Blocker not in scope; skip
			continue
		}

		// Collect all "(new)"-suffixed files from this blocker's scope
		for _, entry := range blocker.Scope {
			for _, path := range splitSeq(entry, ", ") {
				path = strings.TrimSpace(path)
				if hasNewSuffix(path) {
					// Remove the " (new)" suffix to get the base filename
					basePath := strings.TrimSuffix(path, " (new)")
					result[basePath] = true
				}
			}
		}

		// Transitively visit this blocker's blockers
		toVisit = append(toVisit, blocker.BlockedBy...)
	}

	return result
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

func checkW11VagueOutcomes(issues map[string]*materialize.Issue) []Finding {
	var findings []Finding
	for id, issue := range issues {
		if issue.Outcome == "" {
			continue
		}
		lower := strings.TrimSpace(strings.ToLower(issue.Outcome))
		if len(lower) < minOutcomeLength {
			findings = append(findings, Finding{
				Severity: "warning", Rule: "W11",
				Message:  fmt.Sprintf("vague outcome: %s outcome is %d chars", id, len(lower)),
				CitedIDs: []string{id},
			})
			continue
		}
		if slices.Contains(vagueOutcomes, lower) {
			findings = append(findings, Finding{
				Severity: "warning", Rule: "W11",
				Message:  fmt.Sprintf("vague outcome: %s outcome is %d chars", id, len(lower)),
				CitedIDs: []string{id},
			})
		}
	}
	return findings
}
