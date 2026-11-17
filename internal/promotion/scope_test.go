package promotion_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveScope_NoScopeExcludesBoundaryTips_REQ_480(t *testing.T) {
	t.Parallel()
	head, boundary := "head", "boundary"
	res, err := promotion.DeriveScope(promotion.ScopeInput{
		Head:           head,
		Boundary:       boundary,
		BoundaryRef:    "main",
		IntegrationRef: "main",
		Ancestor:       ancMap(map[string]bool{"boundary boundary": true, "boundary head": true, "head head": true}),
		Issues: []promotion.ScopeIssue{
			{ID: "on-base", Status: ops.StatusDone, Base: "older", Tip: boundary},
			{ID: "open", Status: ops.StatusOpen, Base: "older", Tip: head},
			{ID: "parent", Status: ops.StatusDone, Base: "older", Tip: head, Children: []string{"child"}},
			{ID: "no-tip", Status: ops.StatusDone, Base: "older"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, promotion.ScopeKindNone, res.Kind)
	assert.False(t, res.Stacked)
	assert.Empty(t, res.Deliveries)
	text := promotion.FormatScope(promotion.ScopeInput{
		Head: head, Boundary: boundary, BoundaryRef: "main", IntegrationRef: "main",
	}, res)
	assert.Contains(t, text, "kind=no-scope")
	assert.Contains(t, text, "not a bypass of R2/R3/R4")
}

func TestDeriveScope_StackedUsesRecordedBase_REQ_480(t *testing.T) {
	t.Parallel()
	head, boundary, parentTip := "stacked-head", "parent-tip", "parent-tip"
	res, err := promotion.DeriveScope(promotion.ScopeInput{
		Head:           head,
		Boundary:       boundary,
		BoundaryRef:    "feat/parent",
		IntegrationRef: "main",
		Ancestor: ancMap(map[string]bool{
			parentTip + " " + boundary: true,
			parentTip + " " + head:     true,
			head + " " + head:          true,
			head + " " + boundary:      false,
		}),
		Issues: []promotion.ScopeIssue{
			{ID: "parent-leaf", Status: ops.StatusDone, Base: "main-base", Tip: parentTip},
			{
				ID: "stacked-leaf", Status: ops.StatusDone, Base: "parent-base", Tip: head,
				Assessments: []promotion.AssessmentFact{{BaseSHA: "parent-base", HeadSHA: head}},
			},
			{
				ID: "stacked-sibling", Status: ops.StatusDone, Base: "parent-base", Tip: head,
			},
		},
	})
	require.NoError(t, err)
	assert.True(t, res.Stacked)
	require.Len(t, res.Deliveries, 1)
	assert.Equal(t, "parent-base", res.Deliveries[0].Base)
	assert.Equal(t, []string{"stacked-leaf", "stacked-sibling"}, res.Deliveries[0].IssueIDs)
	assert.True(t, res.Deliveries[0].Assessed)
	text := promotion.FormatScope(promotion.ScopeInput{
		Head: head, Boundary: boundary, BoundaryRef: "feat/parent", IntegrationRef: "main",
	}, res)
	assert.Contains(t, text, "stacked base ref feat/parent is not integration main")
	assert.Contains(t, text, "issues=stacked-leaf,stacked-sibling")
	assert.NotContains(t, text, "parent-leaf")
}

func TestDeriveScope_MissingRecordedBaseFails_REQ_480(t *testing.T) {
	t.Parallel()
	_, err := promotion.DeriveScope(promotion.ScopeInput{
		Head: "head", Boundary: "boundary", IntegrationRef: "main",
		Ancestor: ancMap(map[string]bool{"head head": true}),
		Issues:   []promotion.ScopeIssue{{ID: "task-01", Status: ops.StatusDone, Tip: "head"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recorded delivery base")
}

func TestDeriveScope_AncestorErrorFails_REQ_480(t *testing.T) {
	t.Parallel()
	_, err := promotion.DeriveScope(promotion.ScopeInput{
		Head: "head", Boundary: "boundary",
		Ancestor: func(string, string) (bool, error) {
			return false, assertErr("boom")
		},
		Issues: []promotion.ScopeIssue{{ID: "task-01", Status: ops.StatusDone, Base: "b", Tip: "t"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "place tip of task-01")
}

func TestCheckReady_ActionsRunIDBindsEvidence_REQ_480(t *testing.T) {
	t.Parallel()
	base, tip := "base1", "tip1"
	ev := ciFullEvidence(tip)
	assert.True(t, ev.PromotionReadyEvidence(tip), "run id is not part of PromotionReadyEvidence")
	in := promotion.ReadyInput{
		BaseSHA: base, HeadSHA: tip,
		Issues: map[string]promotion.ReadyIssue{
			"task-01": {
				DefinitionOfDone: "done",
				Acceptance:       []byte(`["ok"]`),
				Assessments:      []promotion.AssessmentFact{{BaseSHA: base, HeadSHA: tip}},
			},
		},
		IssueIDs:     []string{"task-01"},
		GateEvidence: []ops.GateEvidence{ev},
		ActionsRunID: "999",
	}
	err := promotion.CheckReady(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "actions run 999")

	ev.ActionsRunID = "999"
	in.GateEvidence = []ops.GateEvidence{ev}
	require.NoError(t, promotion.CheckReady(in))
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func ancMap(ok map[string]bool) promotion.AncestorFunc {
	return func(ancestor, descendant string) (bool, error) {
		return ok[ancestor+" "+descendant], nil
	}
}
