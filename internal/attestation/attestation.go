// Package attestation holds the Conformance Rating vocabulary and the
// Assessment Attestation record shared by review, materialize, and output
// so those packages do not import each other for this durable projection.
package attestation

type AssessmentAttestation struct {
	SchemaVersion           int     `json:"schema_version"`
	BundleID                string  `json:"bundle_id"`
	ContractFingerprint     string  `json:"contract_fingerprint"`
	DeliveryFingerprint     string  `json:"delivery_fingerprint"`
	ActivityDigest          string  `json:"activity_digest,omitempty"`
	BaseSHA                 string  `json:"base_sha"`
	HeadSHA                 string  `json:"head_sha"`
	SkillVersion            string  `json:"skill_version,omitempty"`
	ModelIdentity           string  `json:"model_identity,omitempty"`
	InputTokens             int     `json:"input_tokens,omitempty"`
	OutputTokens            int     `json:"output_tokens,omitempty"`
	Rating                  Rating  `json:"rating"`
	EffectiveRating         Rating  `json:"effective_rating,omitempty"`
	IsDisagreement          bool    `json:"is_disagreement,omitempty"`
	ConflictsWithBundleID   string  `json:"conflicts_with_bundle_id,omitempty"`
	ConflictsWithRating     *Rating `json:"conflicts_with_rating,omitempty"`
	ResultFingerprint       string  `json:"result_fingerprint"`
	SatisfiedCount          int     `json:"satisfied_count"`
	PartiallySatisfiedCount int     `json:"partially_satisfied_count"`
	NotSatisfiedCount       int     `json:"not_satisfied_count"`
	IndeterminateCount      int     `json:"indeterminate_count"`
}
