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
	f.HostRefs = hostRefFacts(issue.HostRefs)
	if len(issue.AssessmentAttestations) > 0 {
		if data, err := json.Marshal(issue.AssessmentAttestations); err == nil {
			f.AssessmentAttestations = data
		}
	}
	return f
}

func hostRefFacts(refs []materialize.HostRef) []output.HostRefFact {
	live := materialize.LiveHostRefs(refs)
	if len(live) == 0 {
		return nil
	}
	currentURL := live[len(live)-1].URL
	out := make([]output.HostRefFact, 0, len(live))
	for _, ref := range live {
		out = append(out, output.HostRefFact{
			URL:     ref.URL,
			Role:    ref.Role,
			Current: ref.URL == currentURL,
		})
	}
	return out
}

func formatHostRefLines(refs []materialize.HostRef) []string {
	facts := hostRefFacts(refs)
	if len(facts) == 0 {
		return nil
	}
	lines := make([]string, 0, len(facts))
	for _, ref := range facts {
		line := ref.URL
		if ref.Role != "" {
			line += " role=" + ref.Role
		}
		if ref.Current {
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
