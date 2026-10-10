package ready

import "github.com/scullxbones/armature/internal/materialize"

func queueFacts(index materialize.Index, issues map[string]*materialize.Issue, now int64) map[string]Facts {
	out := make(map[string]Facts, len(index))
	for id, e := range index {
		f := Facts{
			Type:           e.Type,
			Status:         e.Status,
			Parent:         e.Parent,
			Title:          e.Title,
			AssignedWorker: e.AssignedWorker,
			Children:       e.Children,
			BlockedBy:      e.BlockedBy,
			Blocks:         e.Blocks,
		}
		if issue := issues[id]; issue != nil {
			f.Priority = issue.Priority
			f.Scope = issue.Scope
			f.EstComplexity = issue.EstComplexity
			f.Confidence = issue.Provenance.Confidence
			f.ClaimedBy = issue.ClaimedBy
			f.ClaimedAt = issue.ClaimedAt
			f.LastHeartbeat = issue.LastHeartbeat
			f.ClaimTTL = issue.ClaimTTL
			f.LastClaimingWorkerActivity = issue.LastClaimingWorkerActivity
			f.ClaimStale = issue.ClaimStale(now)
		}
		out[id] = f
	}
	return out
}

func claimFactsFromIssues(issues map[string]*materialize.Issue, now int64) map[string]Facts {
	out := make(map[string]Facts, len(issues))
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		out[id] = Facts{
			Status:                     issue.Status,
			Title:                      issue.Title,
			ClaimedBy:                  issue.ClaimedBy,
			ClaimedAt:                  issue.ClaimedAt,
			LastHeartbeat:              issue.LastHeartbeat,
			ClaimTTL:                   issue.ClaimTTL,
			LastClaimingWorkerActivity: issue.LastClaimingWorkerActivity,
			ClaimStale:                 issue.ClaimStale(now),
		}
	}
	return out
}
