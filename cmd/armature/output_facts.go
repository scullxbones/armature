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

// hostRefFacts projects live host names in CurrentHostRef order. Withdrawn
// events drop out; the latest live name is marked current. No body is copied.
func hostRefFacts(refs []materialize.HostRef) []output.HostRefFact {
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
	out := make([]output.HostRefFact, 0, len(order))
	for _, url := range order {
		ref := live[url]
		out = append(out, output.HostRefFact{
			URL:     ref.URL,
			Role:    ref.Role,
			Current: url == currentURL,
		})
	}
	return out
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

// formatHostRefLines renders hostRefFacts as arm-show token lines for list.
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
