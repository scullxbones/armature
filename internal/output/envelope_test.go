package output

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// Documented list-row and envelope from docs/design/agent-output-contract.md
// (Normative spec N2/N4 and the successful-list worked example).
type contractListRow struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

const documentedListEnvelope = `{
  "count": 2,
  "issues": [
    {"id": "AOC-S1-T1", "type": "task", "status": "claimed", "title": "Contract definition: ADR 0017 and the normative output spec"},
    {"id": "AOC-S1-T2", "type": "task", "status": "open", "title": "Envelope and channel helpers, added alongside existing output"}
  ],
  "help": ["arm show <id> for outcome, scope, and acceptance"]
}`

func TestEnvelopeShape_REQ_AOC_S1_T2(t *testing.T) {
	t.Parallel()

	rows := []contractListRow{
		{
			ID:     "AOC-S1-T1",
			Type:   "task",
			Status: "claimed",
			Title:  "Contract definition: ADR 0017 and the normative output spec",
		},
		{
			ID:     "AOC-S1-T2",
			Type:   "task",
			Status: "open",
			Title:  "Envelope and channel helpers, added alongside existing output",
		},
	}
	env, err := NewEnvelope("issues", rows, []string{"arm show <id> for outcome, scope, and acceptance"})
	require.NoError(t, err)

	got, err := json.Marshal(env)
	require.NoError(t, err)

	assertJSONEqual(t, []byte(documentedListEnvelope), got)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got, &decoded))
	require.Contains(t, decoded, "count")
	require.Contains(t, decoded, "issues")
	require.Contains(t, decoded, "help")
	require.NotContains(t, decoded, "payload")

	var count json.Number
	require.NoError(t, json.Unmarshal(decoded["count"], &count))
	n, err := count.Int64()
	require.NoError(t, err)
	require.Equal(t, int64(2), n)

	var issues []contractListRow
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	require.Len(t, issues, 2)

	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.Equal(t, []string{"arm show <id> for outcome, scope, and acceptance"}, help)
}

func TestEnvelopeRejectsInvalidInput_REQ_AOC_S1_T2(t *testing.T) {
	t.Parallel()

	help := []string{"arm show <id> for detail"}
	_, err := NewEnvelope("", []contractListRow{}, help)
	require.Error(t, err)
	_, err = NewEnvelope("payload", []contractListRow{}, help)
	require.Error(t, err)
	_, err = NewEnvelope("count", []contractListRow{}, help)
	require.Error(t, err)
	_, err = NewEnvelope("help", []contractListRow{}, help)
	require.Error(t, err)
	_, err = NewEnvelope("issues", nil, help)
	require.Error(t, err)
	_, err = NewEnvelope("issues", "not-an-array", help)
	require.Error(t, err)
	_, err = NewEnvelope("issues", []contractListRow{}, []string{""})
	require.Error(t, err)
	require.Error(t, WriteEnvelope(&bytes.Buffer{}, nil))

	env, err := NewEnvelope("issues", []contractListRow{}, help)
	require.NoError(t, err)
	require.Error(t, WriteEnvelope(failWriter{}, env))
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestEnvelopeNilPayloadMarshalsEmptyArray_REQ_AOC_S1_T2(t *testing.T) {
	t.Parallel()

	var rows []contractListRow
	env, err := NewEnvelope("issues", rows, []string{"no issues match the filter"})
	require.NoError(t, err)
	got, err := json.Marshal(env)
	require.NoError(t, err)
	assertJSONEqual(t, []byte(`{"count":0,"issues":[],"help":["no issues match the filter"]}`), got)
}

func TestEnvelopeWritesToStdout_REQ_AOC_S1_T2(t *testing.T) {
	t.Parallel()

	env, err := NewEnvelope("issues", []contractListRow{}, []string{
		"no issues are ready to claim; blockers are unmerged or claims are active",
	})
	require.NoError(t, err)

	var stdout bytes.Buffer
	require.NoError(t, WriteEnvelope(&stdout, env))

	raw := stdout.Bytes()
	require.True(t, bytes.HasSuffix(raw, []byte("\n")), "stdout must terminate with one newline")
	require.Equal(t, 1, bytes.Count(raw, []byte("\n")), "stdout must contain exactly one JSON value")

	dec := json.NewDecoder(bytes.NewReader(raw))
	var decoded map[string]any
	require.NoError(t, dec.Decode(&decoded))
	require.False(t, dec.More(), "stdout must not contain a second JSON value")

	require.Equal(t, float64(0), decoded["count"])
	issues, ok := decoded["issues"].([]any)
	require.True(t, ok, "payload must be a JSON array, not null or omitted")
	require.Empty(t, issues)
	help, ok := decoded["help"].([]any)
	require.True(t, ok)
	require.Equal(t, []any{"no issues are ready to claim; blockers are unmerged or claims are active"}, help)
}

func assertJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var wantV, gotV any
	require.NoError(t, json.Unmarshal(want, &wantV))
	require.NoError(t, json.Unmarshal(got, &gotV))
	require.Equal(t, wantV, gotV)
}
