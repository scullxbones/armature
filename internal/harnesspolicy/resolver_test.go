package harnesspolicy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyFromFactsMarksAcceptedKnownSources(t *testing.T) {
	t.Parallel()

	acceptance := json.RawMessage(`["go test ./... passes"]`)
	policy := PolicyFromFacts(IssueFacts{
		ID:         "TASK-1",
		Title:      "Implement hook",
		Scope:      []string{"internal/harnesshook/"},
		Acceptance: acceptance,
		SourceLinks: []SourceLinkFact{
			{SourceEntryID: "SRC-1"},
		},
		CitationAcceptances: []CitationAcceptanceFact{
			{SourceEntryID: "SRC-1"},
		},
	}, map[string]struct{}{"SRC-1": {}})

	assert.Equal(t, "TASK-1", policy.ID)
	assert.Equal(t, "Implement hook", policy.Title)
	assert.Equal(t, []string{"internal/harnesshook/"}, policy.Scope)
	assert.JSONEq(t, string(acceptance), string(policy.Acceptance))
	require.Len(t, policy.Citations, 1)
	assert.Equal(t, "SRC-1", policy.Citations[0].SourceEntryID)
	assert.True(t, policy.Citations[0].Accepted)
}

func TestPolicyFromFactsMarksUnacceptedKnownSources(t *testing.T) {
	t.Parallel()

	policy := PolicyFromFacts(IssueFacts{
		ID:    "TASK-2",
		Title: "Need citation",
		Scope: []string{"internal/harnesspolicy/"},
		SourceLinks: []SourceLinkFact{
			{SourceEntryID: "SRC-2"},
		},
	}, map[string]struct{}{"SRC-2": {}})

	require.Len(t, policy.Citations, 1)
	assert.Equal(t, "SRC-2", policy.Citations[0].SourceEntryID)
	assert.False(t, policy.Citations[0].Accepted)
}

func TestPolicyFromFactsTreatsGlobalAcceptanceAsCitingAllSources(t *testing.T) {
	t.Parallel()

	policy := PolicyFromFacts(IssueFacts{
		ID:    "TASK-3",
		Title: "Linked source task",
		SourceLinks: []SourceLinkFact{
			{SourceEntryID: "SRC-3"},
		},
		CitationAcceptances: []CitationAcceptanceFact{
			{SourceEntryID: ""},
		},
	}, map[string]struct{}{"SRC-3": {}})

	require.Len(t, policy.Citations, 1)
	assert.Equal(t, "SRC-3", policy.Citations[0].SourceEntryID)
	assert.True(t, policy.Citations[0].Accepted, "global accept-citation should cover all linked sources")
}

func TestPolicyFromFactsUnknownSourceUsesOnlyGlobalAcceptance(t *testing.T) {
	t.Parallel()

	withoutGlobal := PolicyFromFacts(IssueFacts{
		SourceLinks: []SourceLinkFact{{SourceEntryID: "MISSING"}},
		CitationAcceptances: []CitationAcceptanceFact{
			{SourceEntryID: "SRC-1"},
		},
	}, map[string]struct{}{"SRC-1": {}})
	require.Len(t, withoutGlobal.Citations, 1)
	assert.False(t, withoutGlobal.Citations[0].Accepted)

	withGlobal := PolicyFromFacts(IssueFacts{
		SourceLinks: []SourceLinkFact{{SourceEntryID: "MISSING"}},
		CitationAcceptances: []CitationAcceptanceFact{
			{SourceEntryID: ""},
		},
	}, nil)
	require.Len(t, withGlobal.Citations, 1)
	assert.True(t, withGlobal.Citations[0].Accepted)
}

func TestPolicyFromFactsSkipsEmptySourceLinkIDs(t *testing.T) {
	t.Parallel()

	policy := PolicyFromFacts(IssueFacts{
		SourceLinks: []SourceLinkFact{
			{SourceEntryID: ""},
			{SourceEntryID: "SRC-1"},
		},
	}, map[string]struct{}{"SRC-1": {}})

	require.Len(t, policy.Citations, 1)
	assert.Equal(t, "SRC-1", policy.Citations[0].SourceEntryID)
}

func TestPolicyFromFactsEmptySourceLinksYieldNilCitations(t *testing.T) {
	t.Parallel()

	policy := PolicyFromFacts(IssueFacts{ID: "TASK-4"}, nil)
	assert.Nil(t, policy.Citations)
}
