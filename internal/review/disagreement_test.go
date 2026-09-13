package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewRecord_TracksDisagreementEvents_REQ_TOPTIER_S13_T2 proves G3.3:
// RecordWithDuplicateCheck writes IsDisagreement / ConflictsWith* on the new
// Assessment Attestation, and CollectDisagreementStats treats those stored
// fields as the queryable disagreement facts.
func TestReviewRecord_TracksDisagreementEvents_REQ_TOPTIER_S13_T2(t *testing.T) {
	t.Parallel()

	prior := recordDisagreementAttestation(t, "bundle-green", "green review", Satisfied)
	input := RecordInput{
		Assessment: disagreementAssessment("bundle-red", "red review", NotSatisfied),
		IssueID:    "task-01",
	}
	result, err := RecordWithDuplicateCheck(input, []AssessmentAttestation{prior})
	require.NoError(t, err)
	require.NotNil(t, result.Attestation)
	assert.False(t, result.IsDuplicate)
	assert.True(t, result.Attestation.IsDisagreement)
	assert.Equal(t, "bundle-green", result.Attestation.ConflictsWithBundleID)
	require.NotNil(t, result.Attestation.ConflictsWithRating)
	assert.Equal(t, Green, *result.Attestation.ConflictsWithRating)
	assert.False(t, prior.IsDisagreement, "T1 writes disagreement only on the new record")

	history := []AssessmentAttestation{prior, *result.Attestation}
	stats := CollectDisagreementStats(history)
	assert.Equal(t, 1, stats.Count)
	require.Len(t, stats.Events, 1)
	assert.Equal(t, stats.Count, len(stats.Events))
	event := stats.Events[0]
	assert.Equal(t, "bundle-red", event.BundleID)
	assert.Equal(t, disagreementDeliveryFP, event.DeliveryFingerprint)
	assert.Equal(t, Red, event.Rating)
	assert.Equal(t, Red, event.EffectiveRating)
	assert.Equal(t, "bundle-green", event.ConflictsWithBundleID)
	require.NotNil(t, event.ConflictsWithRating)
	assert.Equal(t, Green, *event.ConflictsWithRating)

	assert.Equal(t, 0, CollectDisagreementStats([]AssessmentAttestation{prior}).Count,
		"the unre-written prior is not a disagreement fact")
}

// TestDisagreementStats_IgnoresNonDisagreementRows_REQ_TOPTIER_S13_T2 asserts
// the aggregator keys off stored IsDisagreement and does not re-derive events
// from raw Conformance Rating pairs or leftover ConflictsWith* fields.
func TestDisagreementStats_IgnoresNonDisagreementRows_REQ_TOPTIER_S13_T2(t *testing.T) {
	t.Parallel()

	red := Red
	green := Green
	rows := []AssessmentAttestation{
		{
			BundleID:            "legacy-red",
			DeliveryFingerprint: "fp-same",
			Rating:              Red,
		},
		{
			BundleID:            "legacy-green",
			DeliveryFingerprint: "fp-same",
			Rating:              Green,
		},
		{
			BundleID:              "conflicts-without-flag",
			DeliveryFingerprint:   "fp-same",
			Rating:                Green,
			ConflictsWithBundleID: "legacy-red",
			ConflictsWithRating:   &red,
		},
		{
			BundleID:              "stored-disagreement",
			DeliveryFingerprint:   "fp-same",
			Rating:                Green,
			EffectiveRating:       Red,
			IsDisagreement:        true,
			ConflictsWithBundleID: "legacy-red",
			ConflictsWithRating:   &red,
		},
		{
			BundleID:              "second-disagreement",
			DeliveryFingerprint:   "fp-other",
			Rating:                Yellow,
			EffectiveRating:       Yellow,
			IsDisagreement:        true,
			ConflictsWithBundleID: "bundle-green-prior",
			ConflictsWithRating:   &green,
		},
	}

	stats := CollectDisagreementStats(rows)
	assert.Equal(t, 2, stats.Count)
	require.Len(t, stats.Events, 2)
	assert.Equal(t, "stored-disagreement", stats.Events[0].BundleID)
	assert.Equal(t, "legacy-red", stats.Events[0].ConflictsWithBundleID)
	require.NotNil(t, stats.Events[0].ConflictsWithRating)
	assert.Equal(t, Red, *stats.Events[0].ConflictsWithRating)
	assert.Equal(t, "second-disagreement", stats.Events[1].BundleID)
	assert.Equal(t, "bundle-green-prior", stats.Events[1].ConflictsWithBundleID)
	require.NotNil(t, stats.Events[1].ConflictsWithRating)
	assert.Equal(t, Green, *stats.Events[1].ConflictsWithRating)

	empty := CollectDisagreementStats(nil)
	assert.Equal(t, 0, empty.Count)
	assert.Empty(t, empty.Events)

	red = Yellow
	require.NotNil(t, stats.Events[0].ConflictsWithRating)
	assert.Equal(t, Red, *stats.Events[0].ConflictsWithRating,
		"aggregator must clone ConflictsWithRating so callers cannot mutate source rows through Events")
}

type disagreementEvent struct {
	BundleID              string
	DeliveryFingerprint   string
	Rating                Rating
	EffectiveRating       Rating
	ConflictsWithBundleID string
	ConflictsWithRating   *Rating
}

type disagreementStats struct {
	Count  int
	Events []disagreementEvent
}

func CollectDisagreementStats(atts []AssessmentAttestation) disagreementStats {
	var stats disagreementStats
	for i := range atts {
		att := atts[i]
		if !att.IsDisagreement {
			continue
		}
		stats.Events = append(stats.Events, disagreementEvent{
			BundleID:              att.BundleID,
			DeliveryFingerprint:   att.DeliveryFingerprint,
			Rating:                att.Rating,
			EffectiveRating:       att.EffectiveRating,
			ConflictsWithBundleID: att.ConflictsWithBundleID,
			ConflictsWithRating:   cloneRating(att.ConflictsWithRating),
		})
	}
	stats.Count = len(stats.Events)
	return stats
}

func cloneRating(r *Rating) *Rating {
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}
