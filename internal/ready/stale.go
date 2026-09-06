package ready

import (
	"sort"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// StaleClaims returns a sorted list of issue IDs that are claimed but whose
// TTL has expired as of now.
func StaleClaims(issues map[string]*materialize.Issue, now time.Time) []string {
	nowUnix := now.Unix()
	var stale []string
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		if issue.Status != ops.StatusClaimed {
			continue
		}
		if issue.ClaimStale(nowUnix) {
			stale = append(stale, id)
		}
	}
	sort.Strings(stale)
	return stale
}

// ExpiredClaimEntry describes an issue whose claim TTL has lapsed without
// renewal, for distinct surfacing in `arm ready` output (rather than being
// silently omitted from the ready queue, or silently included in it).
type ExpiredClaimEntry struct {
	Issue                      string `json:"issue"`
	Title                      string `json:"title"`
	Status                     string `json:"status"`
	ClaimedBy                  string `json:"claimed_by"`
	ClaimedAt                  int64  `json:"claimed_at"`
	LastHeartbeat              int64  `json:"last_heartbeat"`
	ClaimTTL                   int    `json:"claim_ttl"`
	LastClaimingWorkerActivity int64  `json:"last_claiming_worker_activity,omitempty"`
}

// ExpiredClaims returns a sorted (by issue ID) list of issues in claimed or
// in-progress status whose claim TTL has expired as of now. Unlike StaleClaims
// (StatusClaimed only, issue IDs only), this also covers StatusInProgress —
// per the recovery state machine (docs/design/recovery-state-machine.md), a
// worker that goes silent mid-work is exactly as much an orphaned-claim
// signal as one that never starts.
func ExpiredClaims(issues map[string]*materialize.Issue, now time.Time) []ExpiredClaimEntry {
	nowUnix := now.Unix()
	var expired []ExpiredClaimEntry
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		if issue.Status != ops.StatusClaimed && issue.Status != ops.StatusInProgress {
			continue
		}
		if !issue.ClaimStale(nowUnix) {
			continue
		}
		expired = append(expired, ExpiredClaimEntry{
			Issue:                      id,
			Title:                      issue.Title,
			Status:                     issue.Status,
			ClaimedBy:                  issue.ClaimedBy,
			ClaimedAt:                  issue.ClaimedAt,
			LastHeartbeat:              issue.LastHeartbeat,
			ClaimTTL:                   issue.ClaimTTL,
			LastClaimingWorkerActivity: issue.LastClaimingWorkerActivity,
		})
	}
	sort.Slice(expired, func(i, j int) bool { return expired[i].Issue < expired[j].Issue })
	return expired
}
