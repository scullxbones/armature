// Package ready computes the set of unblocked, unclaimed issues (the Ready Queue) and detects stale in-progress claims.
package ready

import (
	"fmt"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/issuetype"
)

const (
	statusOpen       = "open"
	statusClaimed    = "claimed"
	statusInProgress = "in-progress"
	statusMerged     = "merged"
	statusDone       = "done"
)

// ReadyEntry represents a task in the Ready Queue.
type ReadyEntry struct {
	Issue                string   `json:"issue"`
	Type                 string   `json:"type"`
	Parent               string   `json:"parent,omitempty"`
	Title                string   `json:"title"`
	Priority             string   `json:"priority,omitempty"`
	Scope                []string `json:"scope,omitempty"`
	EstComplexity        string   `json:"estimated_complexity,omitempty"`
	RequiresConfirmation bool     `json:"requires_confirmation,omitempty"`
	AssignedWorker       string   `json:"assigned_worker,omitempty"`
}

// ComputeReady applies the 4-rule gate and returns a priority-sorted Ready Queue.
// workerID is used for assignment-aware sorting: assigned-to-me first, unassigned next,
// other-assigned last. Pass "" to disable assignment-aware sorting.
func ComputeReady(facts map[string]Facts, workerID string) []ReadyEntry {
	graph := graphFromFacts(facts)

	var ready []ReadyEntry
	for id, f := range facts {
		if skipReadyScan(f) {
			continue
		}
		if !allBlockersMerged(f.BlockedBy, facts) {
			continue
		}
		if !parentIsActive(f, facts) {
			continue
		}

		re := ReadyEntry{
			Issue:                id,
			Type:                 f.Type,
			Parent:               f.Parent,
			Title:                f.Title,
			AssignedWorker:       f.AssignedWorker,
			Priority:             f.Priority,
			Scope:                f.Scope,
			EstComplexity:        f.EstComplexity,
			RequiresConfirmation: f.Confidence == "inferred",
		}
		ready = append(ready, re)
	}

	sortReady(ready, facts, graph, workerID)
	return ready
}

// ExplainNotReady returns a map of issue ID to exclusion reason for every open
// unclaimed task that is NOT in the Ready Queue. Keys are sorted deterministically.
// The reason string identifies which gate excluded the issue.
func ExplainNotReady(facts map[string]Facts) map[string]string {
	result := make(map[string]string)
	for id, f := range facts {
		if skipReadyScan(f) {
			continue
		}
		if !allBlockersMerged(f.BlockedBy, facts) {
			result[id] = fmt.Sprintf("blocker(s) not merged: %s", unmergedBlockerHints(f.BlockedBy, facts))
			continue
		}
		if status, ok := inactiveParentStatus(f, facts); !ok {
			result[id] = fmt.Sprintf("parent %s is not active (status: %s)", f.Parent, status)
		}
	}
	return result
}

func skipReadyScan(f Facts) bool {
	if !issuetype.IsReadyEligible(f.Type) || f.Status != statusOpen {
		return true
	}
	if f.Confidence == "draft" {
		return true
	}
	return f.ClaimedBy != "" && !f.ClaimStale
}

func parentIsActive(f Facts, facts map[string]Facts) bool {
	_, ok := inactiveParentStatus(f, facts)
	return ok
}

func inactiveParentStatus(f Facts, facts map[string]Facts) (status string, active bool) {
	if f.Parent == "" {
		return "", true
	}
	parent, ok := facts[f.Parent]
	if !ok {
		return "missing", false
	}
	switch parent.Status {
	case statusInProgress, statusClaimed, statusOpen:
		return parent.Status, true
	default:
		return parent.Status, false
	}
}

func unmergedBlockerHints(blockers []string, facts map[string]Facts) string {
	var unmerged []string
	for _, bid := range blockers {
		e, ok := facts[bid]
		if !ok || e.Status != statusMerged {
			hint := ""
			if ok && e.Status == statusDone {
				hint = fmt.Sprintf(" — run: arm merged --issue %s", bid)
			}
			unmerged = append(unmerged, bid+hint)
		}
	}
	return strings.Join(unmerged, ", ")
}

// FilterByAssignedTo returns entries whose AssignedWorker matches workerID.
// If workerID is empty, all entries are returned unchanged.
func FilterByAssignedTo(entries []ReadyEntry, workerID string) []ReadyEntry {
	if workerID == "" {
		return entries
	}
	filtered := entries[:0:0]
	for _, e := range entries {
		if e.AssignedWorker == workerID {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func allBlockersMerged(blockers []string, facts map[string]Facts) bool {
	for _, bid := range blockers {
		entry, ok := facts[bid]
		if !ok || entry.Status != statusMerged {
			return false
		}
	}
	return true
}

var priorityOrder = map[string]int{
	"critical": 0,
	"high":     1,
	"medium":   2,
	"low":      3,
	"":         4,
}

func assignmentTier(issueID, workerID string, facts map[string]Facts) int {
	if workerID == "" {
		return 1
	}
	entry := facts[issueID]
	if entry.AssignedWorker == "" {
		return 1
	}
	if entry.AssignedWorker == workerID {
		return 0
	}
	return 2
}

func sortReady(entries []ReadyEntry, facts map[string]Facts, graph *dag.Graph, workerID string) {
	sort.SliceStable(entries, func(i, j int) bool {
		ai := assignmentTier(entries[i].Issue, workerID, facts)
		aj := assignmentTier(entries[j].Issue, workerID, facts)
		if ai != aj {
			return ai < aj
		}
		pi := priorityOrder[entries[i].Priority]
		pj := priorityOrder[entries[j].Priority]
		if pi != pj {
			return pi < pj
		}
		di := graph.Depth(entries[i].Issue)
		dj := graph.Depth(entries[j].Issue)
		if di != dj {
			return di > dj
		}
		bi := len(facts[entries[i].Issue].Blocks)
		bj := len(facts[entries[j].Issue].Blocks)
		if bi != bj {
			return bi > bj
		}
		return entries[i].Issue < entries[j].Issue
	})
}

// CollectDescendants returns the set of all descendant IDs of root (not including root itself).
func CollectDescendants(root string, facts map[string]Facts) map[string]bool {
	descendants := graphFromFacts(facts).Descendants(root)

	result := make(map[string]bool)
	for _, id := range descendants {
		result[id] = true
	}
	return result
}
