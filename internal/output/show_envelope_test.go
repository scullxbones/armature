package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteShowEnvelopeCompactAndTruncates(t *testing.T) {
	t.Parallel()

	row := IssueJSON{
		ID:               "T1",
		Title:            "One",
		Type:             "task",
		Status:           "open",
		DefinitionOfDone: strings.Repeat("done. ", 200),
	}
	require.Greater(t, len(row.DefinitionOfDone), ShowLargeFieldLimit)
	trunc := TruncateShowIssue(&row)
	require.NotEmpty(t, trunc)

	var buf bytes.Buffer
	require.NoError(t, WriteShowEnvelope(&buf, []string{"T1"}, []IssueJSON{row}, trunc))

	raw := bytes.TrimSpace(buf.Bytes())
	require.True(t, json.Valid(raw))
	assert.False(t, bytes.Contains(buf.Bytes(), []byte("\n  ")))

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Contains(t, decoded, "count")
	require.Contains(t, decoded, "issues")
	require.Contains(t, decoded, "help")
	require.Contains(t, decoded, "truncated")
	require.NotContains(t, decoded, "payload")

	var full bytes.Buffer
	fullRow := IssueJSON{ID: "T1", Title: "One", Type: "task", Status: "open"}
	require.NoError(t, WriteShowEnvelope(&full, []string{"T1"}, []IssueJSON{fullRow}, nil))
	var fullDecoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(full.Bytes()), &fullDecoded))
	_, hasTrunc := fullDecoded["truncated"]
	assert.False(t, hasTrunc)
}

func TestTruncateShowTextUTF8Safe(t *testing.T) {
	t.Parallel()
	shown, total, truncated := TruncateShowText("abc", 10)
	assert.Equal(t, "abc", shown)
	assert.Equal(t, 3, total)
	assert.False(t, truncated)

	s := "éééé"
	shown, total, truncated = TruncateShowText(s, 3)
	assert.True(t, truncated)
	assert.Equal(t, len(s), total)
	assert.True(t, json.Valid([]byte(`"`+shown+`"`)))
	assert.LessOrEqual(t, len(shown), 3)
}
