package ready

import (
	"sort"
	"time"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
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
		if isClaimStale(issue.ClaimedAt, issue.LastHeartbeat, issue.ClaimTTL, nowUnix) {
			stale = append(stale, id)
		}
	}
	sort.Strings(stale)
	return stale
}
