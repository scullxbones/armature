package main

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
	res, err := deriveScope(scopeInput{
		Head:           head,
		Boundary:       boundary,
		BoundaryRef:    "main",
		IntegrationRef: "main",
		Ancestor:       ancMap(map[string]bool{"boundary boundary": true, "boundary head": true, "head head": true}),
		Issues: []scopeIssue{
			{ID: "on-base", Status: ops.StatusDone, Base: "older", Tip: boundary},
			{ID: "open", Status: ops.StatusOpen, Base: "older", Tip: head},
			{ID: "parent", Status: ops.StatusDone, Base: "older", Tip: head, Children: []string{"child"}},
			{ID: "no-tip", Status: ops.StatusDone, Base: "older"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, scopeKindNone, res.Kind)
	assert.False(t, res.Stacked)
	assert.Empty(t, res.Deliveries)
	text := formatScope(scopeInput{
		Head: head, Boundary: boundary, BoundaryRef: "main", IntegrationRef: "main",
	}, res)
	assert.Contains(t, text, "kind=no-scope")
	assert.Contains(t, text, "not a bypass of R2/R3/R4")
}

func TestDeriveScope_StackedUsesRecordedBase_REQ_480(t *testing.T) {
	t.Parallel()
	head, boundary, parentTip := "stacked-head", "parent-tip", "parent-tip"
	res, err := deriveScope(scopeInput{
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
		Issues: []scopeIssue{
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
	text := formatScope(scopeInput{
		Head: head, Boundary: boundary, BoundaryRef: "feat/parent", IntegrationRef: "main",
	}, res)
	assert.Contains(t, text, "stacked base ref feat/parent is not integration main")
	assert.Contains(t, text, "issues=stacked-leaf,stacked-sibling")
	assert.NotContains(t, text, "parent-leaf")
}

func TestDeriveScope_MissingRecordedBaseFails_REQ_480(t *testing.T) {
	t.Parallel()
	_, err := deriveScope(scopeInput{
		Head: "head", Boundary: "boundary", IntegrationRef: "main",
		Ancestor: ancMap(map[string]bool{"head head": true}),
		Issues:   []scopeIssue{{ID: "task-01", Status: ops.StatusDone, Tip: "head"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recorded delivery base")
}

func TestDeriveScope_AncestorErrorFails_REQ_480(t *testing.T) {
	t.Parallel()
	_, err := deriveScope(scopeInput{
		Head: "head", Boundary: "boundary",
		Ancestor: func(string, string) (bool, error) {
			return false, assertErr("boom")
		},
		Issues: []scopeIssue{{ID: "task-01", Status: ops.StatusDone, Base: "b", Tip: "t"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "place tip of task-01")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func ancMap(ok map[string]bool) scopeAncestorFunc {
	return func(ancestor, descendant string) (bool, error) {
		return ok[ancestor+" "+descendant], nil
	}
}
