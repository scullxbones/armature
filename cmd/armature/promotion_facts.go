package main

import (
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/promotion"
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
			Children: append([]string(nil), entry.Children...),
		}
	}
	return g
}

func promotionGraphFromIssues(issues map[string]*materialize.Issue) promotion.Graph {
	g := make(promotion.Graph, len(issues))
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		g[id] = promotion.Node{
			Status:   issue.Status,
			Children: append([]string(nil), issue.Children...),
		}
	}
	return g
}
