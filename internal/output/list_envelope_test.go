package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteListEnvelopeCompactFourFieldRows(t *testing.T) {
	t.Parallel()

	index := materialize.Index{
		"T1": {Type: "task", Status: ops.StatusOpen, Title: "One", Parent: "S1"},
		"T2": {Type: "task", Status: ops.StatusDone, Title: "Two"},
	}
	ids := []string{"T1", "T2"}
	var buf bytes.Buffer
	require.NoError(t, WriteListEnvelope(&buf, ListRows(index, ids), nil, false, false))

	raw := bytes.TrimSpace(buf.Bytes())
	require.True(t, json.Valid(raw))
	assert.False(t, bytes.Contains(buf.Bytes(), []byte("\n  ")))

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(decoded["issues"], &rows))
	require.Len(t, rows, 2)
	assert.Equal(t, map[string]any{"id": "T1", "type": "task", "status": "open", "title": "One"}, rows[0])
	groups := ListGroupsByStatus(index, ids)
	var grouped bytes.Buffer
	require.NoError(t, WriteListEnvelope(&grouped, ListRows(index, ids), groups, true, false))
	assert.Contains(t, grouped.String(), `"groups"`)
	assert.Equal(t, []ListGroup{
		{Status: ops.StatusDone, IDs: []string{"T2"}},
		{Status: ops.StatusOpen, IDs: []string{"T1"}},
	}, groups)
}

func TestWriteReadyEnvelopeIncludesExpiredClaims(t *testing.T) {
	t.Parallel()

	entries := []ready.ReadyEntry{{
		Issue: "R1", Type: "task", Title: "Ready", Parent: "S1", Scope: []string{"a.go"},
	}}
	expired := []ready.ExpiredClaimEntry{{
		Issue: "X1", Title: "Stale", Status: ops.StatusInProgress, ClaimedBy: "w", ClaimTTL: 60,
	}}
	var buf bytes.Buffer
	require.NoError(t, WriteReadyEnvelope(&buf, entries, nil, false, expired, "", ""))

	raw := bytes.TrimSpace(buf.Bytes())
	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Contains(t, decoded, "expired_claims")
	var claims []ExpiredClaim
	require.NoError(t, json.Unmarshal(decoded["expired_claims"], &claims))
	require.Equal(t, []ExpiredClaim{{
		ID: "X1", Title: "Stale", Status: ops.StatusInProgress, ClaimedBy: "w", ClaimTTL: 60,
	}}, claims)

	waves := [][]ready.ReadyEntry{entries}
	assert.Equal(t, [][]string{{"R1"}}, ReadyWaveIDs(waves))
	var withWaves bytes.Buffer
	require.NoError(t, WriteReadyEnvelope(&withWaves, entries, waves, true, nil, "", ""))
	assert.Contains(t, withWaves.String(), `"waves"`)
}
