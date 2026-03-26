// Package doctor implements repo health checks for the trls doctor command.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/ready"
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
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Items    []string `json:"items,omitempty"`
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
func RunChecks(index materialize.Index, allIssues map[string]*materialize.Issue, opsTargetIDs []string, repoPath string) Report {
	var checks []Finding

	checks = append(checks, checkD1GitDivergence(repoPath, index))
	checks = append(checks, checkD2StaleClaims(allIssues))
	checks = append(checks, checkD3OrphanedOpsFromList(index, opsTargetIDs))
	checks = append(checks, checkD4BrokenParentRefs(index))
	checks = append(checks, checkD5DependencyCycles(index))
	checks = append(checks, checkD6UncitedIssues(allIssues))

	return Report{Checks: checks}
}

// Run executes all health checks and returns a Report.
func Run(issuesDir string, repoPath string) (Report, error) {
	singleBranch := true // single-branch is the default for doctor

	if _, err := materialize.Materialize(issuesDir, singleBranch); err != nil {
		return Report{}, fmt.Errorf("materialize: %w", err)
	}

	index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
	if err != nil {
		return Report{}, fmt.Errorf("load index: %w", err)
	}

	// Load all issues for detailed checks.
	allIssues, err := loadAllIssues(issuesDir, index)
	if err != nil {
		return Report{}, fmt.Errorf("load issues: %w", err)
	}

	var checks []Finding

	checks = append(checks, checkD1GitDivergence(repoPath, index))
	checks = append(checks, checkD2StaleClaims(allIssues))
	checks = append(checks, checkD3OrphanedOps(issuesDir, index))
	checks = append(checks, checkD4BrokenParentRefs(index))
	checks = append(checks, checkD5DependencyCycles(index))
	checks = append(checks, checkD6UncitedIssues(allIssues))

	return Report{Checks: checks}, nil
}

func loadAllIssues(issuesDir string, index materialize.Index) (map[string]*materialize.Issue, error) {
	result := make(map[string]*materialize.Issue, len(index))
	for id := range index {
		path := filepath.Join(issuesDir, "state", "issues", id+".json")
		issue, err := materialize.LoadIssue(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("load issue %s: %w", id, err)
		}
		issueCopy := issue
		result[id] = &issueCopy
	}
	return result, nil
}

// D1: git/trellis divergence — scan git log for issue IDs referenced in commits
// that are not in done/merged state.
func checkD1GitDivergence(repoPath string, index materialize.Index) Finding {
	f := Finding{Check: "D1", Severity: SeverityOK, Message: "No git/trellis divergence detected"}

	cmd := exec.Command("git", "-C", repoPath, "log", "--oneline", "--no-merges", "--pretty=%s")
	out, err := cmd.Output()
	if err != nil {
		// Not a git repo or no commits — skip
		return f
	}

	lines := strings.Split(string(out), "\n")
	seen := make(map[string]bool)
	var diverged []string

	for _, line := range lines {
		matches := issueIDPattern.FindAllString(line, -1)
		for _, id := range matches {
			if seen[id] {
				continue
			}
			seen[id] = true
			entry, ok := index[id]
			if !ok {
				continue
			}
			if entry.Status != "done" && entry.Status != "merged" {
				diverged = append(diverged, fmt.Sprintf("%s (%s)", id, entry.Status))
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
func checkD2StaleClaims(allIssues map[string]*materialize.Issue) Finding {
	f := Finding{Check: "D2", Severity: SeverityOK, Message: "No stale claims"}

	stale := ready.StaleClaims(allIssues, time.Now())
	if len(stale) > 0 {
		f.Severity = SeverityWarning
		f.Message = "Claimed issues with expired TTL"
		f.Items = stale
	}
	return f
}

// D3: orphaned ops — op files referencing issue IDs not in the graph.
func checkD3OrphanedOps(issuesDir string, index materialize.Index) Finding {
	opsDir := filepath.Join(issuesDir, "ops")
	entries, err := os.ReadDir(opsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Finding{Check: "D3", Severity: SeverityOK, Message: "No orphaned ops"}
		}
		return Finding{
			Check:    "D3",
			Severity: SeverityError,
			Message:  fmt.Sprintf("Cannot read ops directory: %v", err),
		}
	}

	var targetIDs []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		logPath := filepath.Join(opsDir, entry.Name())
		logOps, err := ops.ReadLog(logPath)
		if err != nil {
			continue
		}
		for _, op := range logOps {
			if op.TargetID != "" {
				targetIDs = append(targetIDs, op.TargetID)
			}
		}
	}
	return checkD3OrphanedOpsFromList(index, targetIDs)
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

// D5: dependency cycles — blocked_by chains that form a cycle.
func checkD5DependencyCycles(index materialize.Index) Finding {
	f := Finding{Check: "D5", Severity: SeverityOK, Message: "No dependency cycles"}

	// Build adjacency list from blocked_by.
	adj := make(map[string][]string)
	for id, entry := range index {
		adj[id] = entry.BlockedBy
	}

	// DFS cycle detection.
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
