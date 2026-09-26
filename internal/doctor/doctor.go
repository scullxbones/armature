// Package doctor implements repo health checks for the arm doctor command.
package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
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
	"github.com/scullxbones/armature/internal/worktree"
)

type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
	SeverityOK      Severity = "ok"
)

type Finding struct {
	Check        string   `json:"check"`
	Severity     Severity `json:"severity"`
	Message      string   `json:"message"`
	Items        []string `json:"items,omitempty"`
	VerboseItems []string `json:"verbose_items,omitempty"`
}

type Report struct {
	Checks []Finding `json:"checks"`
}

func (r Report) HasErrors() bool {
	for _, f := range r.Checks {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r Report) HasWarnings() bool {
	for _, f := range r.Checks {
		if f.Severity == SeverityWarning {
			return true
		}
	}
	return false
}

var issueIDPattern = regexp.MustCompile(`\b([A-Za-z][A-Za-z0-9]*(?:-[A-Za-z0-9]+)+)\b`)

func RunChecks(index materialize.Index, allIssues map[string]*materialize.Issue, opsTargetIDs []string, repoPath string, now time.Time) Report {
	configPath := ""
	if repoPath != "" {
		configPath = filepath.Join(repoPath, ".armature", "config.json")
	}
	return Report{Checks: indexedChecks(index, allIssues, opsTargetIDs, repoPath, now, nil, nil, configPath)}
}

var liveCheckIDs = []string{"D1", "D2", "D3", "D4", "D5", "D6", "D7", "D8", "D9", "D10", "D12"}

func LiveCheckIDs() []string {
	out := make([]string, len(liveCheckIDs))
	copy(out, liveCheckIDs)
	return out
}

func Run(issuesDir string, stateDir string, repoPath string, worktreePath string, verbose bool, now time.Time) (Report, error) {
	loaded, err := loadMaterializedState(issuesDir, stateDir)
	if err != nil {
		return Report{}, err
	}
	index := loaded.index
	allIssues := loaded.issues
	allOps := loaded.allOps
	opItems := loaded.opItems
	warnings := loaded.warnings

	noteOnlyOrphans := targetsWithOnlyFullyDeletedNotes(allOps)
	opsTargetIDs := make([]string, 0, len(allOps))
	for _, op := range allOps {
		if !ops.IsAuditOnly(op.Type) && op.TargetID != "" && !noteOnlyOrphans[op.TargetID] {
			opsTargetIDs = append(opsTargetIDs, op.TargetID)
		}
	}

	var verboseD3Context map[string][]opLocation
	if verbose {
		verboseD3Context = buildLocationMapFromOpItems(opItems)
	} else {
		verboseD3Context = make(map[string][]opLocation)
	}

	checks := indexedChecks(index, allIssues, opsTargetIDs, repoPath, now, verboseD3Context,
		[]Finding{checkD7WorkerIDMismatches(filterMismatchWarnings(warnings))},
		filepath.Join(issuesDir, "config.json"))
	checks = append(checks, checkD12OpsWorktreeLag(worktreePath))
	return Report{Checks: checks}, nil
}

func indexedChecks(
	index materialize.Index,
	allIssues map[string]*materialize.Issue,
	opsTargetIDs []string,
	repoPath string,
	now time.Time,
	d3ctx map[string][]opLocation,
	afterD6 []Finding,
	configPath string,
) []Finding {
	checks := []Finding{
		checkD1GitDivergence(repoPath, index),
		checkD2StaleClaims(allIssues, now),
		checkD3OrphanedOpsFromListWithContext(index, opsTargetIDs, d3ctx),
		checkD4BrokenParentRefs(index),
		checkD5DependencyCycles(index),
		checkD6UncitedIssues(allIssues),
	}
	checks = append(checks, afterD6...)
	return append(checks,
		CheckD8ScopeViolations(index, allIssues, repoPath, now),
		checkD9UnrecognizedWorktrees(repoPath, allIssues, now),
		CheckD10ConfigHealth(configPath),
	)
}

type materializedState struct {
	index    materialize.Index
	issues   map[string]*materialize.Issue
	opItems  []ops.OpItem
	warnings []string
	allOps   []ops.Op
}

func loadMaterializedState(issuesDir, stateDir string) (materializedState, error) {
	opsDir := filepath.Join(issuesDir, "ops")
	loaded, err := ops.LoadFromDirValidated(opsDir)
	if err != nil {
		return materializedState{}, fmt.Errorf("read ops: %w", err)
	}
	opItems := loaded.Items
	warnings := loaded.Warnings
	allOps := ops.ExtractOps(opItems)
	if _, _, err := materialize.Run(stateDir, allOps, nil, materialize.Options{WriteStateFiles: true}); err != nil {
		return materializedState{}, fmt.Errorf("materialize: %w", err)
	}
	index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
	if err != nil {
		return materializedState{}, fmt.Errorf("load index: %w", err)
	}
	allIssues, err := loadAllIssues(stateDir, index)
	if err != nil {
		return materializedState{}, fmt.Errorf("load issues: %w", err)
	}
	return materializedState{
		index:    index,
		issues:   allIssues,
		opItems:  opItems,
		warnings: warnings,
		allOps:   allOps,
	}, nil
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
	return evaluateD1GitDivergence(lines, statuses)
}

func evaluateD1GitDivergence(commitSubjects []string, statuses map[string]string) Finding {
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

type opLocation struct {
	file string
	line int
}

func buildLocationMapFromOpItems(items []ops.OpItem) map[string][]opLocation {
	result := make(map[string][]opLocation)
	for _, item := range items {
		targetID := item.Op.TargetID
		result[targetID] = append(result[targetID], opLocation{
			file: filepath.Base(item.LogFilename),
			line: item.LineNumber,
		})
	}
	return result
}

func checkD3OrphanedOpsFromListWithContext(index materialize.Index, targetIDs []string, locations map[string][]opLocation) Finding {
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

func targetsWithOnlyFullyDeletedNotes(allOps []ops.Op) map[string]bool {
	type noteState struct{ created, deleted bool }
	notes := make(map[string]map[string]*noteState)
	otherRefs := make(map[string]bool)

	for _, op := range allOps {
		if op.TargetID == "" {
			continue
		}
		switch op.Type {
		case ops.OpNote, ops.OpNoteDelete:
			if notes[op.TargetID] == nil {
				notes[op.TargetID] = make(map[string]*noteState)
			}
			ns := notes[op.TargetID][op.Payload.NoteID]
			if ns == nil {
				ns = &noteState{}
				notes[op.TargetID][op.Payload.NoteID] = ns
			}
			if op.Type == ops.OpNote {
				ns.created = true
			} else {
				ns.deleted = true
			}
		case ops.OpSourceFingerprint, ops.OpGateEvidence:
		default:
			otherRefs[op.TargetID] = true
		}
	}

	result := make(map[string]bool)
	for targetID, noteMap := range notes {
		if otherRefs[targetID] {
			continue
		}
		allDeleted := true
		for _, ns := range noteMap {
			if !ns.created || !ns.deleted {
				allDeleted = false
				break
			}
		}
		if allDeleted {
			result[targetID] = true
		}
	}
	return result
}

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

func blockedByOnlyDAGNodes(index materialize.Index) map[string]*dag.Node {
	nodes := make(map[string]*dag.Node)
	for id, entry := range index {
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

func checkD5DependencyCycles(index materialize.Index) Finding {
	f := Finding{Check: "D5", Severity: SeverityOK, Message: "No dependency cycles"}

	dagNodes := blockedByOnlyDAGNodes(index)
	graphIndex := dag.FromIndex(dagNodes)
	if !graphIndex.HasCycle() {
		return f
	}

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

func checkD9UnrecognizedWorktrees(repoPath string, allIssues map[string]*materialize.Issue, now time.Time) Finding {
	if repoPath == "" || allIssues == nil {
		return Finding{Check: "D9", Severity: SeverityOK, Message: "No unrecognized managed worktrees"}
	}
	worktrees, err := worktree.ListManaged(repoPath)
	if err != nil {
		return Finding{Check: "D9", Severity: SeverityOK, Message: "No unrecognized managed worktrees"}
	}
	result := worktree.Reconcile(worktrees, allIssues, now, worktree.CanonicalRoot(repoPath))
	return EvaluateD9UnrecognizedWorktrees(result.Unrecognized)
}

func EvaluateD9UnrecognizedWorktrees(unrecognized []string) Finding {
	f := Finding{Check: "D9", Severity: SeverityOK, Message: "No unrecognized managed worktrees"}
	if len(unrecognized) > 0 {
		items := append([]string(nil), unrecognized...)
		sort.Strings(items)
		f.Severity = SeverityWarning
		f.Message = "Managed worktrees with no issue binding"
		f.Items = items
	}
	return f
}

func checkD12OpsWorktreeLag(worktreePath string) Finding {
	skip := Finding{Check: "D12", Severity: SeverityOK, Message: "Ops worktree lag not checked"}
	if strings.TrimSpace(worktreePath) == "" {
		return skip
	}
	info, err := os.Stat(worktreePath)
	if err != nil || !info.IsDir() {
		return skip
	}
	gc := adapters.New(worktreePath)
	fetchErr := gc.FetchTrackingRefWithoutMovingHEAD("_armature")
	behind, err := gc.RevListCount("HEAD..origin/_armature")
	if err != nil {
		return skip
	}
	_ = fetchErr
	return EvaluateD12OpsWorktreeLag(behind)
}

func EvaluateD12OpsWorktreeLag(behind int) Finding {
	f := Finding{Check: "D12", Severity: SeverityOK, Message: "Ops worktree is not behind origin/_armature"}
	if behind > 0 {
		f.Severity = SeverityWarning
		f.Message = fmt.Sprintf("Ops worktree is %d commit(s) behind origin/_armature", behind)
		f.Items = []string{fmt.Sprintf("%d", behind)}
	}
	return f
}
