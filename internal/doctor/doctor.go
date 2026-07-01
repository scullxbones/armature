// Package doctor implements repo health checks for the trls doctor command.
package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/ready"
)

// Severity of a check finding.
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
	SeverityOK      Severity = "ok"
)

// Finding is a single finding from a health check.
type Finding struct {
	Check        string   `json:"check"`
	Severity     Severity `json:"severity"`
	Message      string   `json:"message"`
	Items        []string `json:"items,omitempty"`
	VerboseItems []string `json:"verbose_items,omitempty"`
}

// Report is the result of running all doctor checks.
type Report struct {
	Checks []Finding `json:"checks"`
}

// HasErrors returns true if any finding has error severity.
func (r Report) HasErrors() bool {
	for _, f := range r.Checks {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// HasWarnings returns true if any finding has warning severity.
func (r Report) HasWarnings() bool {
	for _, f := range r.Checks {
		if f.Severity == SeverityWarning {
			return true
		}
	}
	return false
}

// issueIDPattern matches issue-ID-like tokens in git commit messages.
// Matches uppercase/lowercase letters and digits separated by hyphens, e.g. E5-S1-T9, task-01.
var issueIDPattern = regexp.MustCompile(`\b([A-Za-z][A-Za-z0-9]*(?:-[A-Za-z0-9]+)+)\b`)

// RunChecks executes the subset of checks that don't require filesystem ops (D3)
// or git (D1). It accepts pre-loaded data, making it testable without I/O.
// Pass nil for allIssues and opsLog to skip those checks.
// repoPath is used for D1; pass "" to skip D1.
// now is used for D2 stale claim detection.
func RunChecks(index materialize.Index, allIssues map[string]*materialize.Issue, opsTargetIDs []string, repoPath string, now time.Time) Report {
	var checks []Finding

	checks = append(checks, checkD1GitDivergence(repoPath, index))
	checks = append(checks, checkD2StaleClaims(allIssues, now))
	checks = append(checks, checkD3OrphanedOpsFromList(index, opsTargetIDs))
	checks = append(checks, checkD4BrokenParentRefs(index))
	checks = append(checks, checkD5DependencyCycles(index))
	checks = append(checks, checkD6UncitedIssues(allIssues))

	return Report{Checks: checks}
}

// Run executes all health checks and returns a Report.
// verbose=true adds file path and line context to D3 violations via VerboseItems.
// now is used for D2 stale claim detection.
func Run(issuesDir string, stateDir string, repoPath string, verbose bool, now time.Time) (Report, error) {
	// Read ops from the ops directory using validated stream (excludes worker-ID mismatches)
	opsDir := filepath.Join(issuesDir, "ops")
	opItems, _, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return Report{}, fmt.Errorf("read ops: %w", err)
	}

	// Extract ops from OpItems
	allOps := ops.ExtractOps(opItems)

	if _, err := materialize.Materialize(stateDir, allOps, nil); err != nil {
		return Report{}, fmt.Errorf("materialize: %w", err)
	}

	index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
	if err != nil {
		return Report{}, fmt.Errorf("load index: %w", err)
	}

	// Load all issues for detailed checks.
	allIssues, err := loadAllIssues(stateDir, index)
	if err != nil {
		return Report{}, fmt.Errorf("load issues: %w", err)
	}

	// Extract target IDs from ops for D3 check
	opsTargetIDs := make([]string, 0, len(allOps))
	for _, op := range allOps {
		if op.Type != ops.OpSourceFingerprint && op.TargetID != "" {
			opsTargetIDs = append(opsTargetIDs, op.TargetID)
		}
	}

	// Build verbose context from OpItems metadata
	var verboseD3Context map[string][]opLocation
	if verbose {
		verboseD3Context = buildLocationMapFromOpItems(opItems)
	} else {
		verboseD3Context = make(map[string][]opLocation)
	}

	var checks []Finding

	checks = append(checks, checkD1GitDivergence(repoPath, index))
	checks = append(checks, checkD2StaleClaims(allIssues, now))
	checks = append(checks, checkD3OrphanedOpsFromListWithContext(index, opsTargetIDs, verboseD3Context))
	checks = append(checks, checkD4BrokenParentRefs(index))
	checks = append(checks, checkD5DependencyCycles(index))
	checks = append(checks, checkD6UncitedIssues(allIssues))
	checks = append(checks, checkD7WorkerIDMismatches(filterMismatchWarnings(warnings)))

	return Report{Checks: checks}, nil
}

func loadAllIssues(stateDir string, index materialize.Index) (map[string]*materialize.Issue, error) {
	result := make(map[string]*materialize.Issue, len(index))
	for id := range index {
		path := filepath.Join(stateDir, "issues", id+".json")
		issue, err := materialize.LoadIssue(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("load issue %s: %w", id, err)
		}
		issueCopy := issue
		result[id] = &issueCopy
	}
	return result, nil
}

// D1: git/armature divergence — scan git log for issue IDs referenced in commits
// that are not in done/merged state.
func checkD1GitDivergence(repoPath string, index materialize.Index) Finding {
	out, err := adapters.GitLog(repoPath, "--oneline", "--no-merges", "--pretty=%s")
	if err != nil {
		return Finding{Check: "D1", Severity: SeverityOK, Message: "No git/armature divergence detected"}
	}

	lines := strings.Split(out, "\n")
	statuses := make(map[string]string, len(index))
	for id, entry := range index {
		statuses[id] = entry.Status
	}
	return EvaluateD1GitDivergence(lines, statuses)
}

// EvaluateD1GitDivergence evaluates already-collected git subjects and issue statuses.
func EvaluateD1GitDivergence(commitSubjects []string, statuses map[string]string) Finding {
	f := Finding{Check: "D1", Severity: SeverityOK, Message: "No git/armature divergence detected"}
	seen := make(map[string]bool)
	var diverged []string

	for _, line := range commitSubjects {
		matches := issueIDPattern.FindAllString(line, -1)
		for _, id := range matches {
			if seen[id] {
				continue
			}
			seen[id] = true
			status, ok := statuses[id]
			if !ok {
				continue
			}
			if status != "done" && status != "merged" {
				diverged = append(diverged, fmt.Sprintf("%s (%s)", id, status))
			}
		}
	}

	if len(diverged) > 0 {
		sort.Strings(diverged)
		f.Severity = SeverityWarning
		f.Message = "Git commits reference issues not in done/merged state"
		f.Items = diverged
	}
	return f
}

// D2: stale claims — issues in claimed state with expired TTL.
func checkD2StaleClaims(allIssues map[string]*materialize.Issue, now time.Time) Finding {
	f := Finding{Check: "D2", Severity: SeverityOK, Message: "No stale claims"}

	stale := ready.StaleClaims(allIssues, now)
	if len(stale) > 0 {
		f.Severity = SeverityWarning
		f.Message = "Claimed issues with expired TTL"
		f.Items = stale
	}
	return f
}

// opLocation records where in an op log a target ID was found.
type opLocation struct {
	file string
	line int
}

// buildLocationMapFromOpItems builds a location map from OpItem metadata.
// Each OpItem contains the physical line number in its source log file.
// We use this directly for D3 verbose output.
func buildLocationMapFromOpItems(items []ops.OpItem) map[string][]opLocation {
	result := make(map[string][]opLocation)

	// Group by target ID
	targetToLocs := make(map[string][]opLocation)
	for _, item := range items {
		targetID := item.Op.TargetID
		logName := filepath.Base(item.LogFilename)
		loc := opLocation{file: logName, line: item.LineNumber}
		targetToLocs[targetID] = append(targetToLocs[targetID], loc)
	}

	// Copy to result, removing duplicates and sorting
	for targetID, locs := range targetToLocs {
		if len(locs) > 0 {
			result[targetID] = locs
		}
	}
	return result
}

// checkD3OrphanedOpsFromListWithContext checks for orphaned ops given a flat list of target IDs
// and optional verbose context (file:line locations).
func checkD3OrphanedOpsFromListWithContext(index materialize.Index, targetIDs []string, locations map[string][]opLocation) Finding {
	f := checkD3OrphanedOpsFromList(index, targetIDs)

	if f.Severity == SeverityError && len(f.Items) > 0 && len(locations) > 0 {
		orphanedSet := make(map[string]bool, len(f.Items))
		for _, id := range f.Items {
			orphanedSet[id] = true
		}
		var verboseItems []string
		for id, locs := range locations {
			if !orphanedSet[id] {
				continue
			}
			var locStrs []string
			for _, loc := range locs {
				locStrs = append(locStrs, fmt.Sprintf("%s:%d", loc.file, loc.line))
			}
			sort.Strings(locStrs)
			verboseItems = append(verboseItems, fmt.Sprintf("%s (%s)", id, strings.Join(locStrs, ", ")))
		}
		sort.Strings(verboseItems)
		f.VerboseItems = verboseItems
	}

	return f
}

// checkD3OrphanedOpsFromList checks for orphaned ops given a flat list of target IDs.
func checkD3OrphanedOpsFromList(index materialize.Index, targetIDs []string) Finding {
	f := Finding{Check: "D3", Severity: SeverityOK, Message: "No orphaned ops"}
	if targetIDs == nil {
		return f
	}

	orphaned := make(map[string]bool)
	for _, id := range targetIDs {
		if _, ok := index[id]; !ok {
			orphaned[id] = true
		}
	}

	if len(orphaned) > 0 {
		var items []string
		for id := range orphaned {
			items = append(items, id)
		}
		sort.Strings(items)
		f.Severity = SeverityError
		f.Message = "Op files reference issue IDs not in the graph"
		f.Items = items
	}
	return f
}

// D4: broken parent refs — issues whose parent points to a non-existent ID.
func checkD4BrokenParentRefs(index materialize.Index) Finding {
	f := Finding{Check: "D4", Severity: SeverityOK, Message: "No broken parent refs"}

	var broken []string
	for id, entry := range index {
		if entry.Parent == "" {
			continue
		}
		if _, ok := index[entry.Parent]; !ok {
			broken = append(broken, fmt.Sprintf("%s -> %s", id, entry.Parent))
		}
	}

	if len(broken) > 0 {
		sort.Strings(broken)
		f.Severity = SeverityError
		f.Message = "Issues with broken parent references"
		f.Items = broken
	}
	return f
}

// indexToDagNodes converts a materialize.Index to a map of dag.Node pointers.
// Only blocked_by edges are converted; parent-child hierarchy is preserved.
// Children is set to nil to ensure HasCycle() only traverses blocked_by edges.
func indexToDagNodes(index materialize.Index) map[string]*dag.Node {
	nodes := make(map[string]*dag.Node)
	for id, entry := range index {
		// Defensive copy of BlockedBy slice
		blockedBy := make([]string, len(entry.BlockedBy))
		copy(blockedBy, entry.BlockedBy)
		nodes[id] = &dag.Node{
			ID:        id,
			Title:     entry.Title,
			Type:      entry.Type,
			Parent:    entry.Parent,
			Children:  nil,
			BlockedBy: blockedBy,
			Blocks:    entry.Blocks,
		}
	}
	return nodes
}

// D5: dependency cycles — blocked_by chains that form a cycle.
func checkD5DependencyCycles(index materialize.Index) Finding {
	f := Finding{Check: "D5", Severity: SeverityOK, Message: "No dependency cycles"}

	// Use dag.Graph.HasCycle() for fast cycle detection.
	dagNodes := indexToDagNodes(index)
	graphIndex := dag.FromIndex(dagNodes)
	if !graphIndex.HasCycle() {
		return f
	}

	// Cycle detected. Collect cycle edges using simplified blocked_by-only DFS.
	adj := make(map[string][]string)
	for id, entry := range index {
		adj[id] = entry.BlockedBy
	}

	const (
		colorWhite = 0
		colorGray  = 1
		colorBlack = 2
	)
	color := make(map[string]int)

	var cycleNodes []string
	var dfs func(id string) bool
	dfs = func(id string) bool {
		color[id] = colorGray
		for _, dep := range adj[id] {
			if color[dep] == colorGray {
				cycleNodes = append(cycleNodes, fmt.Sprintf("%s -> %s", id, dep))
				return true
			}
			if color[dep] == colorWhite {
				if dfs(dep) {
					return true
				}
			}
		}
		color[id] = colorBlack
		return false
	}

	for id := range index {
		if color[id] == colorWhite {
			dfs(id)
		}
	}

	if len(cycleNodes) > 0 {
		sort.Strings(cycleNodes)
		f.Severity = SeverityError
		f.Message = "Dependency cycles detected in blocked_by chains"
		f.Items = cycleNodes
	}
	return f
}

// D6: uncited issues — issues without source-link or accept-citation.
func checkD6UncitedIssues(allIssues map[string]*materialize.Issue) Finding {
	f := Finding{Check: "D6", Severity: SeverityOK, Message: "All issues cited"}

	var uncited []string
	for id, issue := range allIssues {
		if issue == nil {
			continue
		}
		if len(issue.SourceLinks) == 0 && len(issue.CitationAcceptances) == 0 {
			uncited = append(uncited, id)
		}
	}

	if len(uncited) > 0 {
		sort.Strings(uncited)
		f.Severity = SeverityWarning
		f.Message = "Issues without source-link or accept-citation"
		f.Items = uncited
	}
	return f
}

func filterMismatchWarnings(warnings []string) []string {
	var out []string
	for _, w := range warnings {
		if strings.HasPrefix(w, "worker ID mismatch") {
			out = append(out, w)
		}
	}
	return out
}

// D7: worker-ID mismatches — ops that were excluded from the validated stream due to worker-ID mismatches.
func checkD7WorkerIDMismatches(warnings []string) Finding {
	f := Finding{Check: "D7", Severity: SeverityOK, Message: "No worker-ID mismatches detected"}

	if len(warnings) > 0 {
		sort.Strings(warnings)
		f.Severity = SeverityWarning
		f.Message = "Worker-ID mismatched ops detected"
		f.Items = warnings
	}
	return f
}
