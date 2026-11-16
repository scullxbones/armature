package promotion_test

import (
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckReady_EmptyContractFails_REQ_480(t *testing.T) {
	t.Parallel()
	base, tip := "base1", "tip1"
	err := promotion.CheckReady(promotion.ReadyInput{
		BaseSHA: base,
		HeadSHA: tip,
		Issues: map[string]promotion.ReadyIssue{
			"task-empty": {
				DefinitionOfDone: "",
				Acceptance:       json.RawMessage(`["done"]`),
				Assessments:      []promotion.AssessmentFact{{BaseSHA: base, HeadSHA: tip}},
			},
		},
		IssueIDs:     []string{"task-empty"},
		GateEvidence: []ops.GateEvidence{ciFullEvidence(tip)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty contract")
}

func TestCheckReady_UnknownScopeFails_REQ_480(t *testing.T) {
	t.Parallel()
	err := promotion.CheckReady(promotion.ReadyInput{
		BaseSHA:      "base1",
		HeadSHA:      "tip1",
		Issues:       map[string]promotion.ReadyIssue{},
		IssueIDs:     []string{"task-missing"},
		GateEvidence: []ops.GateEvidence{ciFullEvidence("tip1")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown scope")
}

func TestCheckReady_MissingCIGateEvidenceFails_REQ_480(t *testing.T) {
	t.Parallel()
	base, tip := "base1", "tip1"
	err := promotion.CheckReady(promotion.ReadyInput{
		BaseSHA: base,
		HeadSHA: tip,
		Issues: map[string]promotion.ReadyIssue{
			"task-01": {
				DefinitionOfDone: "ship it",
				Acceptance:       json.RawMessage(`["ok"]`),
				Assessments:      []promotion.AssessmentFact{{BaseSHA: base, HeadSHA: tip}},
			},
		},
		IssueIDs: []string{"task-01"},
		GateEvidence: []ops.GateEvidence{{
			Profile: "full",
			HeadSHA: tip,
			Exit:    0,
			CI:      false, // local run must not count
		}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gate evidence")
}

func TestCheckReady_StaleHeadAssessmentFails_REQ_480(t *testing.T) {
	t.Parallel()
	base, oldTip, newTip := "base1", "tip-old", "tip-new"
	err := promotion.CheckReady(promotion.ReadyInput{
		BaseSHA: base,
		HeadSHA: newTip,
		Issues: map[string]promotion.ReadyIssue{
			"task-01": {
				DefinitionOfDone: "ship it",
				Acceptance:       json.RawMessage(`["ok"]`),
				Assessments:      []promotion.AssessmentFact{{BaseSHA: base, HeadSHA: oldTip}},
			},
		},
		IssueIDs:     []string{"task-01"},
		GateEvidence: []ops.GateEvidence{ciFullEvidence(newTip)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "assessment")
}

func TestCheckReady_FreshPass_REQ_480(t *testing.T) {
	t.Parallel()
	base, tip := "base1", "tip1"
	err := promotion.CheckReady(promotion.ReadyInput{
		BaseSHA: base,
		HeadSHA: tip,
		Issues: map[string]promotion.ReadyIssue{
			"task-01": {
				DefinitionOfDone: "ship it",
				Acceptance:       json.RawMessage(`["ok"]`),
				Assessments:      []promotion.AssessmentFact{{BaseSHA: base, HeadSHA: tip}},
			},
		},
		IssueIDs:     []string{"task-01"},
		GateEvidence: []ops.GateEvidence{ciFullEvidence(tip)},
	})
	require.NoError(t, err)
}

func TestCheckReady_SharedDiffOneAssessmentSeveralLeaves_REQ_480(t *testing.T) {
	t.Parallel()
	base, tip := "base-shared", "tip-shared"
	err := promotion.CheckReady(promotion.ReadyInput{
		BaseSHA: base,
		HeadSHA: tip,
		Issues: map[string]promotion.ReadyIssue{
			"leaf-a": {
				DefinitionOfDone: "a done",
				Acceptance:       json.RawMessage(`["a"]`),
				Assessments:      nil, // no per-leaf attestation
			},
			"leaf-b": {
				DefinitionOfDone: "b done",
				Acceptance:       json.RawMessage(`["b"]`),
				// one assessment keyed by delivery covers both leaves
				Assessments: []promotion.AssessmentFact{{BaseSHA: base, HeadSHA: tip}},
			},
		},
		IssueIDs:     []string{"leaf-a", "leaf-b"},
		GateEvidence: []ops.GateEvidence{ciFullEvidence(tip)},
	})
	require.NoError(t, err)
}

func ciFullEvidence(head string) ops.GateEvidence {
	return ops.GateEvidence{
		Profile: "full",
		HeadSHA: head,
		Exit:    0,
		CI:      true,
	}
}
