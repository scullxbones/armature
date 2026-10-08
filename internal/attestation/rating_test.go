package attestation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRatingJSONRoundTrip(t *testing.T) {
	t.Parallel()
	ratings := []Rating{Green, Yellow, Red}
	for _, rating := range ratings {
		t.Run(rating.String(), func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(rating)
			require.NoError(t, err)
			assert.Equal(t, `"`+rating.String()+`"`, string(data))
			var decoded Rating
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, rating, decoded)
		})
	}
}

func TestRatingUnmarshalSkillOutput(t *testing.T) {
	t.Parallel()
	var result struct {
		Rating Rating `json:"rating"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"rating":"green"}`), &result))
	assert.Equal(t, Green, result.Rating)
}

func TestRatingString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "green", Green.String())
	assert.Equal(t, "yellow", Yellow.String())
	assert.Equal(t, "red", Red.String())
	assert.Equal(t, "unknown", Unspecified.String())
	assert.Equal(t, "unknown", Rating(99).String())
}

func TestParseRating(t *testing.T) {
	t.Parallel()
	got, err := ParseRating("green")
	require.NoError(t, err)
	assert.Equal(t, Green, got)
	got, err = ParseRating("YELLOW")
	require.NoError(t, err)
	assert.Equal(t, Yellow, got)
	_, err = ParseRating("invalid")
	assert.Error(t, err)
	_, err = ParseRating("")
	assert.Error(t, err)
}

func TestMaxRatingSeverityOrder_REQ_TOPTIER_S13_T1(t *testing.T) {
	t.Parallel()
	empty, ok := MaxRating()
	assert.False(t, ok)
	assert.NotEqual(t, Green, empty)
	assert.Equal(t, "unknown", empty.String())
	raw, err := json.Marshal(empty)
	require.NoError(t, err)
	assert.Equal(t, `"unknown"`, string(raw))

	got, ok := MaxRating(Green)
	assert.True(t, ok)
	assert.Equal(t, Green, got)
	got, ok = MaxRating(Green, Yellow)
	assert.True(t, ok)
	assert.Equal(t, Yellow, got)
	got, ok = MaxRating(Green, Yellow, Red)
	assert.True(t, ok)
	assert.Equal(t, Red, got)
	got, ok = MaxRating(Red, Green)
	assert.True(t, ok)
	assert.Equal(t, Red, got)
	got, ok = MaxRating(Yellow, Yellow)
	assert.True(t, ok)
	assert.Equal(t, Yellow, got)
	got, ok = MaxRating(Rating(99), Green)
	assert.True(t, ok)
	assert.Equal(t, Green, got)
}

func TestAtLeastAsSevere(t *testing.T) {
	t.Parallel()
	assert.True(t, AtLeastAsSevere(Red, Green))
	assert.True(t, AtLeastAsSevere(Yellow, Yellow))
	assert.False(t, AtLeastAsSevere(Green, Red))
}
