package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const documentedSyncEnvelope = `{` +
	`"count":1,` +
	`"issues":[{` +
	`"id":"TASK-001",` +
	`"type":"task",` +
	`"status":"done",` +
	`"title":"Landed without assessment",` +
	`"kind":"missing-assessment",` +
	`"next_action":"arm review record --issue TASK-001 --assessment <assessment.json>"` +
	`}],` +
	`"help":["arm review record --issue TASK-001 --assessment <assessment.json>"]` +
	`}`

func TestSyncEnvelopeJSONRoundTrip_REQ_LNGHZN_S11_T3(t *testing.T) {
	t.Parallel()

	var decoded struct {
		Count  int         `json:"count"`
		Issues []SyncIssue `json:"issues"`
		Help   []string    `json:"help"`
	}
	require.NoError(t, json.Unmarshal([]byte(documentedSyncEnvelope), &decoded))
	require.Equal(t, 1, decoded.Count)
	require.Len(t, decoded.Issues, 1)
	assert.Equal(t, "TASK-001", decoded.Issues[0].ID)
	assert.Equal(t, "missing-assessment", decoded.Issues[0].Kind)
	assert.Equal(t, "arm review record --issue TASK-001 --assessment <assessment.json>", decoded.Issues[0].NextAction)
	require.Equal(t, []string{"arm review record --issue TASK-001 --assessment <assessment.json>"}, decoded.Help)

	var buf bytes.Buffer
	require.NoError(t, WriteSyncEnvelope(&buf, decoded.Issues))
	assert.JSONEq(t, documentedSyncEnvelope, buf.String())
}

func TestSyncHelpNamesConcreteArmCommand_REQ_LNGHZN_S11_T3(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"arm list --status done"}, SyncHelp(nil))
	assert.Equal(t, []string{"arm show T1"}, SyncHelp([]SyncIssue{{
		ID: "T1", Kind: "promote", Status: "merged",
	}}))
	assert.Equal(t, []string{"arm review record --issue T2 --assessment <assessment.json>"}, SyncHelp([]SyncIssue{
		{ID: "T1", Kind: "legacy", NextAction: "arm delivery record --issue T1 --base <sha> --tip <sha>"},
		{ID: "T2", Kind: "missing-assessment", NextAction: "arm review record --issue T2 --assessment <assessment.json>"},
	}))
}
