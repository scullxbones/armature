package main

import (
	"github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/materialize"
)

func contextFacts(state *materialize.State) map[string]context.IssueFacts {
	if state == nil {
		return nil
	}
	out := make(map[string]context.IssueFacts, len(state.Issues))
	for id, issue := range state.Issues {
		if issue == nil {
			continue
		}
		out[id] = contextIssueFacts(issue)
	}
	return out
}

func contextIssueFacts(issue *materialize.Issue) context.IssueFacts {
	f := context.IssueFacts{
		ID:               issue.ID,
		Title:            issue.Title,
		Type:             issue.Type,
		Status:           issue.Status,
		Parent:           issue.Parent,
		Priority:         issue.Priority,
		DefinitionOfDone: issue.DefinitionOfDone,
		Outcome:          issue.Outcome,
		Scope:            append([]string(nil), issue.Scope...),
		ContextFiles:     append([]string(nil), issue.ContextFiles...),
		Children:         append([]string(nil), issue.Children...),
		BlockedBy:        append([]string(nil), issue.BlockedBy...),
		Blocks:           append([]string(nil), issue.Blocks...),
		Context:          append([]byte(nil), issue.Context...),
	}
	for _, n := range issue.Notes {
		f.Notes = append(f.Notes, context.NoteFact{Timestamp: n.Timestamp, Msg: n.Msg, Deleted: n.Deleted})
	}
	for _, d := range issue.Decisions {
		f.Decisions = append(f.Decisions, context.DecisionFact{Topic: d.Topic, Choice: d.Choice, Rationale: d.Rationale})
	}
	return f
}
