package main

import (
	"encoding/json"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/output"
)

func outputIssueFacts(issue *materialize.Issue) output.IssueFacts {
	if issue == nil {
		return output.IssueFacts{}
	}
	f := output.IssueFacts{
		ID:                issue.ID,
		Title:             issue.Title,
		Type:              issue.Type,
		Status:            issue.Status,
		Parent:            issue.Parent,
		Priority:          issue.Priority,
		DefinitionOfDone:  issue.DefinitionOfDone,
		Outcome:           issue.Outcome,
		ClaimedBy:         issue.ClaimedBy,
		AssignedWorker:    issue.AssignedWorker,
		Branch:            issue.Branch,
		Base:              issue.Base,
		Tip:               issue.Tip,
		PR:                issue.PR,
		IntegrationBranch: issue.IntegrationBranch,
		TargetSHA:         issue.TargetSHA,
		CombinedPatchID:   issue.CombinedPatchID,
		MatchedCommit:     issue.MatchedCommit,
		Derived:           issue.RollupStatusBefore != "",
		Scope:             append([]string(nil), issue.Scope...),
		BlockedBy:         append([]string(nil), issue.BlockedBy...),
		Blocks:            append([]string(nil), issue.Blocks...),
		Acceptance:        append(json.RawMessage(nil), issue.Acceptance...),
	}
	for _, n := range issue.Notes {
		f.Notes = append(f.Notes, output.NoteFact{Msg: n.Msg, Deleted: n.Deleted})
	}
	if len(issue.AssessmentAttestations) > 0 {
		if data, err := json.Marshal(issue.AssessmentAttestations); err == nil {
			f.AssessmentAttestations = data
		}
	}
	return f
}

func dropHostURL(order []string, url string) []string {
	next := make([]string, 0, len(order))
	for _, item := range order {
		if item != url {
			next = append(next, item)
		}
	}
	return next
}

// formatHostRefLines prints live names in the same order/current rule as
// hostRefFacts (#449): withdrawn drop out; latest live is current. No body.
// Line shape matches arm show's "URL role=… current" tokens (list indents).
func formatHostRefLines(refs []materialize.HostRef) []string {
	live := map[string]materialize.HostRef{}
	order := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Withdrawn {
			delete(live, ref.URL)
			order = dropHostURL(order, ref.URL)
			continue
		}
		order = dropHostURL(order, ref.URL)
		order = append(order, ref.URL)
		live[ref.URL] = ref
	}
	if len(order) == 0 {
		return nil
	}
	currentURL := order[len(order)-1]
	lines := make([]string, 0, len(order))
	for _, url := range order {
		ref := live[url]
		line := url
		if ref.Role != "" {
			line += " role=" + ref.Role
		}
		if url == currentURL {
			line += " current"
		}
		lines = append(lines, line)
	}
	return lines
}

func outputListFacts(index materialize.Index) map[string]output.ListFacts {
	out := make(map[string]output.ListFacts, len(index))
	for id, e := range index {
		out[id] = output.ListFacts{Type: e.Type, Status: e.Status, Title: e.Title}
	}
	return out
}
