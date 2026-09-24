// Package review implements semantic conformance review: preparing task-scoped delivery
// bundles, recording reviewer assessments, and rating them against acceptance criteria.
package review

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
)

const SchemaVersion = 1

type CriterionStatus int

const (
	Satisfied CriterionStatus = iota
	PartiallySatisfied
	NotSatisfied
	Indeterminate
)

func (cs CriterionStatus) String() string {
	switch cs {
	case Satisfied:
		return "satisfied"
	case PartiallySatisfied:
		return "partially_satisfied"
	case NotSatisfied:
		return "not_satisfied"
	case Indeterminate:
		return "indeterminate"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes CriterionStatus as its string name so JSON output produced
// by this package uses human-readable values ("satisfied", "not_satisfied", …).
func (cs CriterionStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(cs.String())
}

// UnmarshalJSON decodes CriterionStatus from its string name, matching the values
// that the armature-reviewer skill emits ("satisfied", "partially_satisfied", …).
func (cs *CriterionStatus) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := ParseCriterionStatus(s)
	if err != nil {
		return err
	}
	*cs = v
	return nil
}

func ParseCriterionStatus(s string) (CriterionStatus, error) {
	switch strings.ToLower(s) {
	case "satisfied":
		return Satisfied, nil
	case "partially_satisfied":
		return PartiallySatisfied, nil
	case "not_satisfied":
		return NotSatisfied, nil
	case "indeterminate":
		return Indeterminate, nil
	default:
		return 0, fmt.Errorf("invalid criterion status: %s", s)
	}
}

type Rating int

const (
	Green Rating = iota
	Yellow
	Red
)

func (r Rating) String() string {
	switch r {
	case Green:
		return "green"
	case Yellow:
		return "yellow"
	case Red:
		return "red"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes Rating as its string name so JSON output uses "green",
// "yellow", or "red" rather than opaque integers.
func (r Rating) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

// UnmarshalJSON decodes Rating from its string name, accepting "green", "yellow",
// or "red" as emitted by the armature-reviewer skill.
func (r *Rating) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := ParseRating(s)
	if err != nil {
		return err
	}
	*r = v
	return nil
}

func ParseRating(s string) (Rating, error) {
	switch strings.ToLower(s) {
	case "green":
		return Green, nil
	case "yellow":
		return Yellow, nil
	case "red":
		return Red, nil
	default:
		return 0, fmt.Errorf("invalid rating: %s", s)
	}
}

type Citation struct {
	Path                 string `json:"path,omitempty"`
	Line                 int    `json:"line,omitempty"`
	Column               int    `json:"column,omitempty"`
	ActivityEntryID      string `json:"activity_entry_id,omitempty"`
	ActivityEntryDetails string `json:"activity_entry_details,omitempty"`
}

// UnmarshalJSON rejects schema-invalid citation coordinates. JSON null unmarshals
// into int as a no-op (leaving 0), which ValidateResult treats as an omitted
// path-level coordinate; the published assessment schema allows an integer or
// omission, not null. An explicit column below 1 is also rejected. Omitted
// line/column remain valid.
func (c *Citation) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if line, ok := raw["line"]; ok {
		if err := decodeCitationCoordinate("line", line, 0); err != nil {
			return err
		}
	}
	if col, ok := raw["column"]; ok {
		if err := decodeCitationCoordinate("column", col, 1); err != nil {
			return err
		}
	}
	type alias Citation
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*c = Citation(a)
	return nil
}

func decodeCitationCoordinate(field string, raw json.RawMessage, min int) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("citation: %s must be an integer, not null", field)
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("citation: invalid %s: %w", field, err)
	}
	if n < min {
		return fmt.Errorf("citation: %s must be >= %d, got %d", field, min, n)
	}
	return nil
}

type CriterionResult struct {
	ID              string          `json:"id"`
	Status          CriterionStatus `json:"status"`
	Citations       []Citation      `json:"citations,omitempty"`
	Rationale       string          `json:"rationale"`
	MissingEvidence string          `json:"missing_evidence,omitempty"`
}

// UnmarshalJSON detects a missing "status" key. Unknown fields are
// intentionally allowed here: the published conformance-assessment schema
// (docs/schemas/conformance-assessment.schema.json) does not set
// additionalProperties: false on results[] entries or citations, so a
// schema-valid reviewer payload may legitimately carry extension/metadata
// fields. This mirrors the policy strictjson.Decode documents and applies
// package-wide — rejecting unknown fields here would fail artifacts the
// published contract accepts.
func (cr *CriterionResult) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, ok := raw["status"]; !ok {
		return fmt.Errorf("criterion result: missing required field \"status\"")
	}
	if rawCitations, ok := raw["citations"]; ok {
		if bytes.Equal(bytes.TrimSpace(rawCitations), []byte("null")) {
			return fmt.Errorf("criterion result: citations must be an array")
		}
	}

	type Alias CriterionResult
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*cr = CriterionResult(alias)
	return nil
}

func (cr CriterionResult) Valid() error {
	if cr.ID == "" {
		return fmt.Errorf("criterion result: missing ID")
	}
	if cr.Rationale == "" {
		return fmt.Errorf("criterion result: missing rationale")
	}
	// Satisfied always needs at least one citation. missing_evidence can
	// rescue the other three statuses, but it cannot manufacture a Green:
	// dropping the last citation cannot leave status=satisfied.
	if cr.Status == Satisfied && len(cr.Citations) == 0 {
		return fmt.Errorf("criterion result %s: citations required for status %s", cr.ID, cr.Status)
	}
	if len(cr.Citations) == 0 && cr.MissingEvidence == "" {
		return fmt.Errorf("criterion result %s: citations or missing_evidence required for status %s", cr.ID, cr.Status)
	}
	for i, citation := range cr.Citations {
		if citation.ActivityEntryID != "" && citation.Path != "" {
			return fmt.Errorf("criterion result %s: citation %d has both path %q and activity_entry_id %q; these are mutually exclusive",
				cr.ID, i, citation.Path, citation.ActivityEntryID)
		}
	}
	return nil
}

type IssueInfo struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Outcome string `json:"outcome"`
}

type Contract struct {
	DefinitionOfDone string   `json:"definition_of_done"`
	Scope            []string `json:"scope,omitempty"`
	Acceptance       []string `json:"acceptance"`
}

type Delivery struct {
	BaseSHA      string   `json:"base_sha"`
	HeadSHA      string   `json:"head_sha"`
	ChangedFiles []string `json:"changed_files"`
	Diff         string   `json:"diff,omitempty"`
}

type Fingerprints struct {
	Contract string `json:"contract"`
	Delivery string `json:"delivery"`
}

type Activity struct {
	Digest            string `json:"digest"`
	EntryCount        int    `json:"entry_count"`
	DeliveryHeadCount int    `json:"delivery_head_count"`
	EarlierCount      int    `json:"earlier_count"`
	LogPath           string `json:"log_path"`
}

type ReviewBundle struct {
	SchemaVersion int                `json:"schema_version"`
	BundleID      string             `json:"bundle_id"`
	Issue         IssueInfo          `json:"issue"`
	Contract      Contract           `json:"contract"`
	Delivery      Delivery           `json:"delivery"`
	Fingerprints  Fingerprints       `json:"fingerprints"`
	Activity      *Activity          `json:"activity,omitempty"`
	GateEvidence  []ops.GateEvidence `json:"gate_evidence,omitempty"`
}

func (rb ReviewBundle) Valid() error {
	if rb.SchemaVersion != SchemaVersion {
		return fmt.Errorf("review bundle: unsupported schema version %d", rb.SchemaVersion)
	}
	if rb.BundleID == "" {
		return fmt.Errorf("review bundle: missing bundle ID")
	}
	if rb.Issue.ID == "" {
		return fmt.Errorf("review bundle: missing issue ID")
	}
	if rb.Issue.Type == "" {
		return fmt.Errorf("review bundle: missing issue type")
	}
	if rb.Issue.Title == "" {
		return fmt.Errorf("review bundle: missing issue title")
	}
	if rb.Fingerprints.Contract == "" {
		return fmt.Errorf("review bundle: missing contract fingerprint")
	}
	if rb.Fingerprints.Delivery == "" {
		return fmt.Errorf("review bundle: missing delivery fingerprint")
	}
	return nil
}

type ConformanceAssessment struct {
	SchemaVersion       int               `json:"schema_version"`
	BundleID            string            `json:"bundle_id"`
	Results             []CriterionResult `json:"results"`
	ContractFingerprint string            `json:"contract_fingerprint"`
	DeliveryFingerprint string            `json:"delivery_fingerprint"`
}

func (ca ConformanceAssessment) Valid() error {
	if ca.SchemaVersion != SchemaVersion {
		return fmt.Errorf("conformance assessment: unsupported schema version %d", ca.SchemaVersion)
	}
	if ca.BundleID == "" {
		return fmt.Errorf("conformance assessment: missing bundle ID")
	}
	if len(ca.Results) == 0 {
		return fmt.Errorf("conformance assessment: no results provided")
	}
	if ca.ContractFingerprint == "" {
		return fmt.Errorf("conformance assessment: missing contract fingerprint")
	}
	if ca.DeliveryFingerprint == "" {
		return fmt.Errorf("conformance assessment: missing delivery fingerprint")
	}
	for _, result := range ca.Results {
		if err := result.Valid(); err != nil {
			return err
		}
	}
	return nil
}

type AssessmentAttestation struct {
	SchemaVersion       int    `json:"schema_version"`
	BundleID            string `json:"bundle_id"`
	ContractFingerprint string `json:"contract_fingerprint"`
	DeliveryFingerprint string `json:"delivery_fingerprint"`
	ActivityDigest      string `json:"activity_digest,omitempty"`
	BaseSHA             string `json:"base_sha"`
	HeadSHA             string `json:"head_sha"`
	SkillVersion        string `json:"skill_version,omitempty"`
	ModelIdentity       string `json:"model_identity,omitempty"`
	// InputTokens and OutputTokens record reviewer LLM usage on the same
	// assessment-attested op (G1.1). They are optional: omitempty keeps every
	// legacy attestation valid. Absent fields decode as 0. No new op type.
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Rating       Rating `json:"rating"`
	// EffectiveRating is the severity-max of this Conformance Rating and
	// qualifying same-DeliveryFingerprint priors (Green < Yellow < Red).
	// It is advisory only: it does not overwrite Rating and confers no merge
	// authority (Constitution I5/N4). Populated at RecordWithDuplicateCheck
	// for newly accepted (non-duplicate) attestations.
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
