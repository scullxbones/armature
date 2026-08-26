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
	_, err = NewEnvelope("issues", []byte("not-an-array"), help)
	require.Error(t, err, "[]byte must not be accepted: encoding/json marshals it as a string")
	_, err = NewEnvelope("issues", json.RawMessage(`[{"id":"x"}]`), help)
	require.Error(t, err, "json.RawMessage must not be accepted: it is a []byte alias")
	_, err = NewEnvelope("issues", [4]byte{1, 2, 3, 4}, help)
	require.Error(t, err, "byte array must not be accepted: encoding/json marshals it as a string")
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

func TestEnvelopeMemberOrder_REQ_AOC_S1_T2(t *testing.T) {
	t.Parallel()

	// Payload keys chosen to straddle "count" and "help" lexicographically:
	// map-backed marshaling sorts "artifacts" ahead of "count" and "help"
	// ahead of "issues", so only ordered marshaling passes for all three.
	for _, payloadKey := range []string{"issues", "workers", "artifacts"} {
		t.Run(payloadKey, func(t *testing.T) {
			t.Parallel()

			env, err := NewEnvelope(payloadKey, []contractListRow{{ID: "AOC-S1-T2"}},
				[]string{"arm show <id> for detail"})
			require.NoError(t, err)

			marshaled, err := env.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, []string{"count", payloadKey, "help"}, topLevelKeyOrder(t, marshaled),
				"help must trail the payload (N5.1)")

			var stdout bytes.Buffer
			require.NoError(t, WriteEnvelope(&stdout, env))
			require.Equal(t, []string{"count", payloadKey, "help"}, topLevelKeyOrder(t, stdout.Bytes()),
				"stdout bytes must carry the same member order")
		})
	}
}

func TestEnvelopeDoesNotEscapeContractText_REQ_AOC_S1_T2(t *testing.T) {
	t.Parallel()

	env, err := NewEnvelope("issues",
		[]contractListRow{{ID: "AOC-S1-T2", Title: "<hold> & wait"}},
		[]string{"arm show <id> for outcome, scope, and acceptance"})
	require.NoError(t, err)

	var stdout bytes.Buffer
	require.NoError(t, WriteEnvelope(&stdout, env))
	raw := stdout.String()

	require.NotContains(t, raw, "\\u003c", "< must survive verbatim, not as an escape")
	require.NotContains(t, raw, "\\u003e", "> must survive verbatim, not as an escape")
	require.NotContains(t, raw, "\\u0026", "& must survive verbatim, not as an escape")
	require.Contains(t, raw, "arm show <id> for outcome, scope, and acceptance")
	require.Contains(t, raw, "<hold> & wait")

	// Escaping is a serialization detail, never a semantic one: the bytes
	// must still decode back to exactly the strings that went in.
	var decoded struct {
		Issues []contractListRow `json:"issues"`
		Help   []string          `json:"help"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &decoded))
	require.Equal(t, "<hold> & wait", decoded.Issues[0].Title)
	require.Equal(t, []string{"arm show <id> for outcome, scope, and acceptance"}, decoded.Help)
}

func TestEnvelopeEmptyStateRequiresHelp_REQ_AOC_S1_T2(t *testing.T) {
	t.Parallel()

	_, err := NewEnvelope("issues", []contractListRow{}, nil)
	require.Error(t, err, "N6: zero items with nil help must be rejected")
	_, err = NewEnvelope("issues", []contractListRow{}, []string{})
	require.Error(t, err, "N6: zero items with empty help must be rejected")

	var nilSlice []contractListRow
	_, err = NewEnvelope("issues", nilSlice, nil)
	require.Error(t, err, "N6: a nil payload slice is an empty state too")

	env, err := NewEnvelope("issues", []contractListRow{}, []string{"no issues match the filter"})
	require.NoError(t, err, "zero items plus a reason is the conforming empty state")
	got, err := env.MarshalJSON()
	require.NoError(t, err)
	assertJSONEqual(t, []byte(`{"count":0,"issues":[],"help":["no issues match the filter"]}`), got)

	// The helper enforces N6 only. A non-empty result may carry empty help;
	// N5.3's "point at arm show" rule is per-command and not decidable here.
	_, err = NewEnvelope("issues", []contractListRow{{ID: "AOC-S1-T2"}}, nil)
	require.NoError(t, err, "help is mandatory only on the empty state")
}

// topLevelKeyOrder returns the envelope's member names in serialized order.
// Unmarshaling into a map would discard exactly the property under test.
func topLevelKeyOrder(t *testing.T, raw []byte) []string {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), tok, "envelope must be a JSON object")

	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		require.NoError(t, err)
		key, ok := tok.(string)
		require.True(t, ok, "object member name must be a string")
		keys = append(keys, key)

		var discard json.RawMessage
		require.NoError(t, dec.Decode(&discard))
	}
	return keys
}

func assertJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var wantV, gotV any
	require.NoError(t, json.Unmarshal(want, &wantV))
	require.NoError(t, json.Unmarshal(got, &gotV))
	require.Equal(t, wantV, gotV)
}
