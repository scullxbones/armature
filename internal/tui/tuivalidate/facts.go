package tuivalidate

import (
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/validate"
)

func validateFacts(issues map[string]*materialize.Issue, now int64) map[string]validate.IssueFacts {
	out := make(map[string]validate.IssueFacts, len(issues))
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		f := validate.IssueFacts{
			ID:                  issue.ID,
			Type:                issue.Type,
			Status:              issue.Status,
			Title:               issue.Title,
			Parent:              issue.Parent,
			DefinitionOfDone:    issue.DefinitionOfDone,
			EstComplexity:       issue.EstComplexity,
			Outcome:             issue.Outcome,
			ClaimedBy:           issue.ClaimedBy,
			ClaimStale:          issue.ClaimStale(now),
			CitationAcceptances: len(issue.CitationAcceptances),
			Scope:               append([]string(nil), issue.Scope...),
			ContextFiles:        append([]string(nil), issue.ContextFiles...),
			Children:            append([]string(nil), issue.Children...),
			BlockedBy:           append([]string(nil), issue.BlockedBy...),
			Blocks:              append([]string(nil), issue.Blocks...),
			Acceptance:          append([]byte(nil), issue.Acceptance...),
			Context:             append([]byte(nil), issue.Context...),
		}
		for _, link := range issue.SourceLinks {
			f.SourceEntryIDs = append(f.SourceEntryIDs, link.SourceEntryID)
		}
		for _, d := range issue.Decisions {
			f.Decisions = append(f.Decisions, validate.DecisionFact{Topic: d.Topic, Choice: d.Choice})
		}
		out[id] = f
	}
	return out
}
