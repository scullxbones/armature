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
