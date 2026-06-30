package review_test

import (
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAcceptanceCriteria_PlainStrings(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`["test 1", "test 2", "test 3"]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"test 1", "test 2", "test 3"}, criteria)
}

func TestParseAcceptanceCriteria_StructuredObjects(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`[
		{"type":"test_passes","description":"all tests pass"},
		{"type":"coverage","description":"test coverage > 80%"}
	]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"all tests pass", "test coverage > 80%"}, criteria)
}

func TestParseAcceptanceCriteria_StructuredWithTextField(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`[
		{"type":"requirement","text":"all requirements met"},
		{"type":"validation","text":"validation complete"}
	]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"all requirements met", "validation complete"}, criteria)
}

func TestParseAcceptanceCriteria_Empty(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`[]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{}, criteria)
}

func TestParseAcceptanceCriteria_Nil(t *testing.T) {
	t.Parallel()
	var input json.RawMessage
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{}, criteria)
}

func TestParseAcceptanceCriteria_InvalidJSON(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{invalid json}`)
	_, err := review.ParseAcceptanceCriteria(input)
	require.Error(t, err)
}

func TestParseAcceptanceCriteria_NotAnArray(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`"just a string"`)
	_, err := review.ParseAcceptanceCriteria(input)
	require.Error(t, err)
}

func TestParseAcceptanceCriteria_TypeOnly(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`[{"type":"test_passes"}]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"test_passes"}, criteria)
}

func TestParseAcceptanceCriteria_TypePatternObject(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`[{"type":"test_passes","pattern":"*.go"}]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"test_passes: *.go"}, criteria)
}

func TestParseAcceptanceCriteria_TypeCmdObject(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`[{"type":"test_passes","cmd":"make check"}]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"test_passes: make check"}, criteria)
}

func TestParseAcceptanceCriteria_TypePathObject(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`[{"type":"no_regression","path":"tests/auth/**"}]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"no_regression: tests/auth/**"}, criteria)
}

func TestParseAcceptanceCriteria_TypeOtherFields(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`[{"type":"function_exists","file":"src/auth/callback.ts","name":"handleCallback"}]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"function_exists: src/auth/callback.ts, handleCallback"}, criteria)
}

func TestParseAcceptanceCriteria_NoRecognizableFields(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`[{"foo":"bar"}]`)
	criteria, err := review.ParseAcceptanceCriteria(input)
	require.NoError(t, err)
	assert.Equal(t, []string{`{"foo":"bar"}`}, criteria)
}
