package contextreport

import (
	"bytes"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasureListMatchesWriteListEnvelope_REQ_NXTTN_S3_T5(t *testing.T) {
	t.Parallel()

	_, index, err := replayFixtureState()
	require.NoError(t, err)

	got, err := measureList(index)
	require.NoError(t, err)

	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var want bytes.Buffer
	require.NoError(t, output.WriteListEnvelope(&want, output.ListRows(index, ids), nil, false, false))
	assert.Equal(t, want.Bytes(), got, "list meter must price writeListEnvelope bytes, not a pretty 7-field array")

	raw := bytes.TrimSpace(got)
	require.True(t, json.Valid(raw))
	require.True(t, bytes.HasPrefix(raw, []byte("{")), "agent list is a compact envelope object")
	assert.False(t, bytes.Contains(got, []byte("\n  ")), "envelope is compact, not MarshalIndent")

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Contains(t, decoded, "count")
	require.Contains(t, decoded, "issues")
	require.Contains(t, decoded, "help")
	require.NotContains(t, decoded, "payload")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(decoded["issues"], &rows))
	require.NotEmpty(t, rows)
	for _, row := range rows {
		require.Len(t, row, 4, "list rows are id/type/status/title only")
		_, hasParent := row["parent"]
		_, hasClaim := row["claimed_by"]
		_, hasOutcome := row["outcome"]
		assert.False(t, hasParent)
		assert.False(t, hasClaim)
		assert.False(t, hasOutcome)
	}
}

func TestMeasureReadyMatchesWriteReadyEnvelope_REQ_NXTTN_S3_T5(t *testing.T) {
	t.Parallel()

	state, index, err := replayFixtureState()
	require.NoError(t, err)
	now := time.Unix(fixtureReadyNow, 0)

	got, err := measureReady(index, state, now)
	require.NoError(t, err)

	entries := ready.ComputeReady(index, state.Issues, "", now.Unix())
	expired := ready.ExpiredClaims(state.Issues, now)
	require.NotEmpty(t, expired, "fixture in-progress claim must be TTL-expired at the frozen clock")

	var want bytes.Buffer
	require.NoError(t, output.WriteReadyEnvelope(&want, entries, nil, false, expired, "", ""))
	assert.Equal(t, want.Bytes(), got, "ready meter must price writeReadyEnvelope bytes, including expired_claims")

	raw := bytes.TrimSpace(got)
	require.True(t, json.Valid(raw))
	require.True(t, bytes.HasPrefix(raw, []byte("{")), "agent ready is a compact envelope object")
	assert.False(t, bytes.Contains(got, []byte("\n  ")), "envelope is compact, not pretty RenderReady")

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Contains(t, decoded, "count")
	require.Contains(t, decoded, "issues")
	require.Contains(t, decoded, "expired_claims")
	require.Contains(t, decoded, "help")

	var claims []output.ExpiredClaim
	require.NoError(t, json.Unmarshal(decoded["expired_claims"], &claims))
	require.NotEmpty(t, claims)
	assert.Equal(t, FixtureShowIssue, claims[0].ID)
}
