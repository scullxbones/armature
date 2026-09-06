// Package ready computes the set of unblocked, unclaimed issues (the ready queue) and detects stale in-progress claims.
package ready

import (
	"fmt"
	"sort"
	"strings"

	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/issuetype"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// ReadyEntry represents a task in the ready queue.
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

// ComputeReady applies the 4-rule gate and returns a priority-sorted ready queue.
// workerID is used for assignment-aware sorting: assigned-to-me first, unassigned next,
// other-assigned last. Pass "" to disable assignment-aware sorting.
func ComputeReady(index materialize.Index, issues map[string]*materialize.Issue, workerID string, now ...int64) []ReadyEntry {
	var currentTime int64
	if len(now) > 0 {
		currentTime = now[0]
	}

	graph := graphFromIndex(index)

	var ready []ReadyEntry

	for id, entry := range index {
		if !issuetype.IsReadyEligible(entry.Type) {
			continue
		}
		if entry.Status != ops.StatusOpen {
			continue
		}
		if !allBlockersMerged(entry.BlockedBy, index) {
			continue
		}
		if entry.Parent != "" {
			parentEntry, ok := index[entry.Parent]
			if !ok || (parentEntry.Status != ops.StatusInProgress && parentEntry.Status != ops.StatusClaimed && parentEntry.Status != ops.StatusOpen) {
				continue
			}
		}
		issue := issues[id]
		if issue != nil && issue.Provenance.Confidence == "draft" {
			continue
		}
		if issue != nil && issue.ClaimedBy != "" && !issue.ClaimStale(currentTime) {
			continue
		}

		re := ReadyEntry{
			Issue:          id,
			Type:           entry.Type,
			Parent:         entry.Parent,
			Title:          entry.Title,
			AssignedWorker: entry.AssignedWorker,
		}
		if issue != nil {
			re.Priority = issue.Priority
			re.Scope = issue.Scope
			re.EstComplexity = issue.EstComplexity
			if issue.Provenance.Confidence == "inferred" {
				re.RequiresConfirmation = true
			}
		}

		ready = append(ready, re)
	}

	sortReady(ready, index, graph, workerID)
	return ready
}

// ExplainNotReady returns a map of issue ID to exclusion reason for every open
// unclaimed task that is NOT in the ready queue. Keys are sorted deterministically.
// The reason string identifies which gate excluded the issue.
// Pass variadic now parameter to inject deterministic time (for testing).
func ExplainNotReady(index materialize.Index, issues map[string]*materialize.Issue, now ...int64) map[string]string {
	var currentTime int64
	if len(now) > 0 {
		currentTime = now[0]
	}

	result := make(map[string]string)
	for id, entry := range index {
		if !issuetype.IsReadyEligible(entry.Type) {
			continue
		}
		if entry.Status != ops.StatusOpen {
			continue
		}
		issue := issues[id]
		// Skip draft issues (they have their own gate but are not "not ready" — they
		// require confirmation first and are intentionally excluded).
		if issue != nil && issue.Provenance.Confidence == "draft" {
			continue
		}
		// Skip issues that are actively claimed (not stale).
		if issue != nil && issue.ClaimedBy != "" && !issue.ClaimStale(currentTime) {
			continue
		}

		// Check each gate in order and record the first failing one.
		if !allBlockersMerged(entry.BlockedBy, index) {
			var unmerged []string
			for _, bid := range entry.BlockedBy {
				e, ok := index[bid]
				if !ok || e.Status != ops.StatusMerged {
					hint := ""
					if ok && e.Status == ops.StatusDone {
						hint = fmt.Sprintf(" — run: arm merged --issue %s", bid)
					}
					unmerged = append(unmerged, bid+hint)
				}
			}
			result[id] = fmt.Sprintf("blocker(s) not merged: %s", strings.Join(unmerged, ", "))
			continue
		}
		if entry.Parent != "" {
			parentEntry, ok := index[entry.Parent]
			if !ok || (parentEntry.Status != ops.StatusInProgress && parentEntry.Status != ops.StatusClaimed && parentEntry.Status != ops.StatusOpen) {
				result[id] = fmt.Sprintf("parent %s is not active (status: %s)", entry.Parent, func() string {
					if !ok {
						return "missing"
					}
					return parentEntry.Status
				}())
				continue
			}
		}
		// Issue passed all gates — it IS ready, do not include it.
	}
	return result
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

func allBlockersMerged(blockers []string, index materialize.Index) bool {
	for _, bid := range blockers {
		entry, ok := index[bid]
		if !ok || entry.Status != ops.StatusMerged {
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

// assignmentTier returns a sort tier for assignment-aware ordering:
// 0 = assigned to me, 1 = unassigned, 2 = assigned to someone else.
func assignmentTier(issueID, workerID string, index materialize.Index) int {
	if workerID == "" {
		return 1 // no worker context — treat all as unassigned tier
	}
	entry := index[issueID]
	if entry.AssignedWorker == "" {
		return 1
	}
	if entry.AssignedWorker == workerID {
		return 0
	}
	return 2
}

func sortReady(entries []ReadyEntry, index materialize.Index, graph *dag.Graph, workerID string) {
	sort.SliceStable(entries, func(i, j int) bool {
		// Assignment tier first
		ai := assignmentTier(entries[i].Issue, workerID, index)
		aj := assignmentTier(entries[j].Issue, workerID, index)
		if ai != aj {
			return ai < aj
		}
		pi := priorityOrder[entries[i].Priority]
		pj := priorityOrder[entries[j].Priority]
		if pi != pj {
			return pi < pj
		}
		// Use graph projection for depth calculation
		di := graph.Depth(entries[i].Issue)
		dj := graph.Depth(entries[j].Issue)
		if di != dj {
			return di > dj
		}
		bi := len(index[entries[i].Issue].Blocks)
		bj := len(index[entries[j].Issue].Blocks)
		if bi != bj {
			return bi > bj
		}
		return entries[i].Issue < entries[j].Issue
	})
}

// CollectDescendants returns the set of all descendant IDs of root (not including root itself).
func CollectDescendants(root string, index materialize.Index) map[string]bool {
	descendants := graphFromIndex(index).Descendants(root)

	result := make(map[string]bool)
	for _, id := range descendants {
		result[id] = true
	}
	return result
}

// graphFromIndex projects a materialize.Index into a dag.Graph. Slices are
// copied so callers can mutate index entries without corrupting the graph.
func graphFromIndex(index materialize.Index) *dag.Graph {
	nodeIndex := make(map[string]*dag.Node, len(index))
	for id, entry := range index {
		nodeIndex[id] = &dag.Node{
			ID:        id,
			Title:     entry.Title,
			Type:      entry.Type,
			Parent:    entry.Parent,
			Children:  append([]string(nil), entry.Children...),
			BlockedBy: append([]string(nil), entry.BlockedBy...),
			Blocks:    append([]string(nil), entry.Blocks...),
		}
	}
	return dag.FromIndex(nodeIndex)
}
