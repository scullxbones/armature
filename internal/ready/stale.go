package ready

import (
	"sort"
)

// StaleClaims returns a sorted list of issue IDs that are claimed but whose
// TTL has expired as of the ClaimStale bit on Facts.
func StaleClaims(facts map[string]Facts) []string {
	var stale []string
	for id, f := range facts {
		if f.Status != statusClaimed {
			continue
		}
		if f.ClaimStale {
			stale = append(stale, id)
		}
	}
	sort.Strings(stale)
	return stale
}

// ExpiredClaimEntry describes an issue whose Claim TTL has lapsed without
// renewal, for distinct surfacing in `arm ready` output (rather than being
// silently omitted from the Ready Queue, or silently included in it).
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
// in-progress status whose Claim TTL has expired. Unlike StaleClaims
// (StatusClaimed only, issue IDs only), this also covers in-progress —
// per the recovery state machine (docs/design/recovery-state-machine.md), a
// worker that goes silent mid-work is exactly as much an orphaned-claim
// signal as one that never starts.
func ExpiredClaims(facts map[string]Facts) []ExpiredClaimEntry {
	var expired []ExpiredClaimEntry
	for id, f := range facts {
		if f.Status != statusClaimed && f.Status != statusInProgress {
			continue
		}
		if !f.ClaimStale {
			continue
		}
		expired = append(expired, ExpiredClaimEntry{
			Issue:                      id,
			Title:                      f.Title,
			Status:                     f.Status,
			ClaimedBy:                  f.ClaimedBy,
			ClaimedAt:                  f.ClaimedAt,
			LastHeartbeat:              f.LastHeartbeat,
			ClaimTTL:                   f.ClaimTTL,
			LastClaimingWorkerActivity: f.LastClaimingWorkerActivity,
		})
	}
	sort.Slice(expired, func(i, j int) bool { return expired[i].Issue < expired[j].Issue })
	return expired
}
