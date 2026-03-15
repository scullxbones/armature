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
}

// ComputeReady applies the 4-rule gate and returns a priority-sorted ready queue.
func ComputeReady(index materialize.Index, issues map[string]*materialize.Issue, now ...int64) []ReadyEntry {
	var currentTime int64
	if len(now) > 0 {
		currentTime = now[0]
	} else {
		currentTime = time.Now().Unix()
	}
	var ready []ReadyEntry

	for id, entry := range index {
		if entry.Type != "task" && entry.Type != "feature" {
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
			if !ok || parentEntry.Status != ops.StatusInProgress {
				continue
			}
		}
		issue := issues[id]
		if issue != nil && issue.ClaimedBy != "" {
			if !isClaimStale(issue.ClaimedAt, issue.LastHeartbeat, issue.ClaimTTL, currentTime) {
				continue
			}
		}

		re := ReadyEntry{
			Issue:  id,
			Type:   entry.Type,
			Parent: entry.Parent,
			Title:  entry.Title,
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

	sortReady(ready, index)
	return ready
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

func sortReady(entries []ReadyEntry, index materialize.Index) {
	sort.SliceStable(entries, func(i, j int) bool {
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
