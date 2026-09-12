package review

// DisagreementEvent is one stored reviewer-disagreement fact (G3.3).
// Values are copied from Assessment Attestation fields written at record
// time; they are not recomputed from Conformance Rating pairs.
type DisagreementEvent struct {
	BundleID              string
	DeliveryFingerprint   string
	Rating                Rating
	EffectiveRating       Rating
	ConflictsWithBundleID string
	ConflictsWithRating   *Rating
}

// DisagreementStats counts and lists stored disagreement facts for Dogfood
// Finding / analytics use. Count always equals len(Events).
type DisagreementStats struct {
	Count  int
	Events []DisagreementEvent
}

// CollectDisagreementStats aggregates attestations whose stored
// IsDisagreement flag is true. Non-disagreement rows are ignored even when
// ConflictsWith* is set or Conformance Ratings would conflict if compared.
func CollectDisagreementStats(atts []AssessmentAttestation) DisagreementStats {
	var stats DisagreementStats
	for i := range atts {
		att := atts[i]
		if !att.IsDisagreement {
			continue
		}
		stats.Events = append(stats.Events, DisagreementEvent{
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
