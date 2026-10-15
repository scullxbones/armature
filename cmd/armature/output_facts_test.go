package main

import (
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/attestation"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputIssueFactsCopiesBorrowedFields(t *testing.T) {
	t.Parallel()

	issue := &materialize.Issue{
		ID:                 "TASK-01",
		Title:              "Show me",
		Type:               "task",
		Status:             ops.StatusDone,
		Parent:             "STORY-01",
		Priority:           "high",
		DefinitionOfDone:   "done",
		Outcome:            "shipped",
		ClaimedBy:          "w1",
		AssignedWorker:     "w2",
		Branch:             "task/TASK-01",
		Base:               "aaa",
		Tip:                "bbb",
		PR:                 "42",
		IntegrationBranch:  "main",
		TargetSHA:          "ccc",
		CombinedPatchID:    "pid",
		MatchedCommit:      "ddd",
		RollupStatusBefore: "open",
		Scope:              []string{"a.go"},
		BlockedBy:          []string{"T0"},
		Blocks:             []string{"T2"},
		Acceptance:         json.RawMessage(`{"ok":true}`),
		Notes: []materialize.Note{
			{Msg: "keep", Deleted: false},
			{Msg: "drop", Deleted: true},
		},
		AssessmentAttestations: []attestation.AssessmentAttestation{{
			BundleID:       "sha256:abc",
			Rating:         attestation.Green,
			SatisfiedCount: 1,
		}},
	}
	facts := outputIssueFacts(issue)
	issue.Scope[0] = "mutated"
	issue.Notes[0].Msg = "mutated"
	issue.AssessmentAttestations[0].BundleID = "zzz"

	assert.Equal(t, "TASK-01", facts.ID)
	assert.Equal(t, "Show me", facts.Title)
	assert.Equal(t, []string{"a.go"}, facts.Scope)
	assert.Equal(t, []output.NoteFact{
		{Msg: "keep", Deleted: false},
		{Msg: "drop", Deleted: true},
	}, facts.Notes)
	assert.True(t, facts.Derived)
	require.NotEmpty(t, facts.AssessmentAttestations)
	var atts []attestation.AssessmentAttestation
	require.NoError(t, json.Unmarshal(facts.AssessmentAttestations, &atts))
	require.Len(t, atts, 1)
	assert.Equal(t, "sha256:abc", atts[0].BundleID)
}

func TestOutputIssueFactsNil(t *testing.T) {
	t.Parallel()
	assert.Equal(t, output.IssueFacts{}, outputIssueFacts(nil))
}

func TestOutputListFactsCopiesIndexRows(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"T1": {Type: "task", Status: ops.StatusOpen, Title: "One", Parent: "S1"},
	}
	facts := outputListFacts(index)
	index["T1"] = materialize.IndexEntry{Type: "bug", Status: ops.StatusDone, Title: "mutated"}
	assert.Equal(t, map[string]output.ListFacts{
		"T1": {Type: "task", Status: ops.StatusOpen, Title: "One"},
	}, facts)
}
