package ready

import (
	"sort"
	"time"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
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
	} else {
		currentTime = time.Now().Unix()
	}
	var ready []ReadyEntry

	for id, entry := range index {
		if entry.Type != "task" && entry.Type != "feature" && entry.Type != "story" {
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
		if issue != nil && issue.ClaimedBy != "" {
			if !isClaimStale(issue.ClaimedAt, issue.LastHeartbeat, issue.ClaimTTL, currentTime) {
				continue
			}
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

	sortReady(ready, index, workerID)
	return ready
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

func isClaimStale(claimedAt, lastHeartbeat int64, ttlMinutes int, now int64) bool {
	if ttlMinutes <= 0 {
		return false
	}
	lastActivity := claimedAt
	if lastHeartbeat > lastActivity {
		lastActivity = lastHeartbeat
	}
	ttlSeconds := int64(ttlMinutes) * 60
	return now > lastActivity+ttlSeconds
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

func sortReady(entries []ReadyEntry, index materialize.Index, workerID string) {
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
		di := depth(entries[i].Issue, index)
		dj := depth(entries[j].Issue, index)
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

func depth(id string, index materialize.Index) int {
	d := 0
	for {
		entry, ok := index[id]
		if !ok || entry.Parent == "" {
			return d
		}
		id = entry.Parent
		d++
		if d > 20 {
			return d
		}
	}
}
