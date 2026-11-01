package main

import (
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/scullxbones/armature/internal/snapshot"
)

func promotionIssueFacts(issue materialize.Issue) promotion.IssueFacts {
	f := promotion.IssueFacts{
		ID:                issue.ID,
		Status:            issue.Status,
		Base:              issue.Base,
		Tip:               issue.Tip,
		IntegrationBranch: issue.IntegrationBranch,
		Children:          append([]string(nil), issue.Children...),
	}
	for _, att := range issue.AssessmentAttestations {
		f.AssessmentAttestations = append(f.AssessmentAttestations, promotion.AssessmentFact{
			BaseSHA: att.BaseSHA,
			HeadSHA: att.HeadSHA,
		})
	}
	return f
}

func promotionGraphFromIndex(index materialize.Index) promotion.Graph {
	g := make(promotion.Graph, len(index))
	for id, entry := range index {
		g[id] = promotion.Node{
			Status:   entry.Status,
			Parent:   entry.Parent,
			Children: append([]string(nil), entry.Children...),
		}
	}
	return g
}

// promotionGraphFromIssues includes delivery Base/Tip when known so remediation
// can emit filled arm delivery record argv instead of <sha> templates.
func promotionGraphFromIssues(issues map[string]*materialize.Issue) promotion.Graph {
	g := make(promotion.Graph, len(issues))
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		g[id] = promotion.Node{
			Status:   issue.Status,
			Parent:   issue.Parent,
			Children: append([]string(nil), issue.Children...),
			Base:     issue.Base,
			Tip:      issue.Tip,
		}
	}
	return g
}

// promotionGraphFromStore enriches the index graph with Base/Tip via ReadIssue
// only — no rematerialize / checkpoint write (safe for arm merged).
func promotionGraphFromStore(store *snapshot.Store, index materialize.Index) promotion.Graph {
	g := promotionGraphFromIndex(index)
	if store == nil {
		return g
	}
	for id, node := range g {
		issue, err := store.ReadIssue(id)
		if err != nil || issue == nil {
			continue
		}
		node.Base = issue.Base
		node.Tip = issue.Tip
		if node.Parent == "" {
			node.Parent = issue.Parent
		}
		g[id] = node
	}
	return g
}
