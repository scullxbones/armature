package main

import (
	"testing"

	"github.com/scullxbones/armature/internal/attestation"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"

	"github.com/stretchr/testify/assert"
)

func TestPromotionIssueFactsCopiesBorrowedFields(t *testing.T) {
	t.Parallel()

	issue := materialize.Issue{
		ID:                "task-1",
		Status:            ops.StatusDone,
		Base:              "aaa",
		Tip:               "bbb",
		IntegrationBranch: "main",
		Children:          []string{"c1"},
		AssessmentAttestations: []attestation.AssessmentAttestation{{
			BaseSHA: "aaa",
			HeadSHA: "bbb",
			Rating:  attestation.Green,
		}},
	}
	facts := promotionIssueFacts(issue)
	issue.Children[0] = "mutated"
	issue.AssessmentAttestations[0].BaseSHA = "zzz"

	assert.Equal(t, "task-1", facts.ID)
	assert.Equal(t, ops.StatusDone, facts.Status)
	assert.Equal(t, "aaa", facts.Base)
	assert.Equal(t, "bbb", facts.Tip)
	assert.Equal(t, "main", facts.IntegrationBranch)
	assert.Equal(t, []string{"c1"}, facts.Children)
	assert.Equal(t, []promotion.AssessmentFact{{BaseSHA: "aaa", HeadSHA: "bbb"}}, facts.AssessmentAttestations)
}
