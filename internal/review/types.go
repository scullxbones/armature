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

// SchemaVersion is the current semantic review protocol version.
const SchemaVersion = 1

// CriterionStatus represents the reviewer's assessment of a single criterion.
type CriterionStatus int

const (
	// Satisfied indicates complete criterion fulfillment.
	Satisfied CriterionStatus = iota
	// PartiallySatisfied indicates partial criterion fulfillment.
	PartiallySatisfied
	// NotSatisfied indicates criterion not met.
	NotSatisfied
	// Indeterminate indicates insufficient evidence or ambiguity.
	Indeterminate
)

// String returns the canonical string representation of the status.
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

// ParseCriterionStatus parses a string into a CriterionStatus.
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

// Rating represents the summary conformance rating derived from criterion results.
type Rating int

const (
	// Green indicates all criteria are satisfied.
	Green Rating = iota
	// Yellow indicates some criteria are partially satisfied or indeterminate, none not satisfied.
	Yellow
	// Red indicates at least one criterion is not satisfied.
	Red
)

// String returns the canonical string representation of the rating.
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

// ParseRating parses a string into a Rating.
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

// Citation provides evidence for a criterion result by referencing a specific location in the delivery.
// It can cite either a diff location (Path/Line/Column) or an activity log entry (ActivityEntryID).
// These two forms are mutually exclusive.
type Citation struct {
	// Path is the file path within the delivery range (for diff citations).
	Path string `json:"path,omitempty"`
	// Line is the line number (optional, for precision, for diff citations).
	Line int `json:"line,omitempty"`
	// Column is the column number (optional, for precision, for diff citations).
	Column int `json:"column,omitempty"`
	// ActivityEntryID references a raw entry ID from the activity log (for activity citations).
	// This is mutually exclusive with Path/Line/Column.
	ActivityEntryID string `json:"activity_entry_id,omitempty"`
	// ActivityEntryDetails contains pre-rendered activity entry information (entry ID, command, exit status).
	// This is populated during record time for activity citations.
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

// CriterionResult records the reviewer's assessment for a single criterion.
type CriterionResult struct {
	// ID uniquely identifies the criterion (e.g., "definition_of_done", "acceptance[0]").
	ID string `json:"id"`
	// Status is the reviewer's assessment.
	Status CriterionStatus `json:"status"`
	// Citations provide structured evidence from the delivery.
	Citations []Citation `json:"citations,omitempty"`
	// Rationale explains the assessment concisely.
	Rationale string `json:"rationale"`
	// MissingEvidence explicitly states missing evidence when citations are absent.
	MissingEvidence string `json:"missing_evidence,omitempty"`
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
	// detect absent key via raw map
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

	// use type alias to avoid infinite recursion
	type Alias CriterionResult
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*cr = CriterionResult(alias)
	return nil
}

// Valid validates that the CriterionResult is well-formed.
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
	// Path and ActivityEntryID are mutually exclusive citation forms; a citation
	// with both set would silently skip diff-index validation (since activity
	// citations are validated separately) while still counting as a diff
	// citation for upgrade-only-rule purposes, letting a fabricated Path escape
	// verification against the diff.
	for i, citation := range cr.Citations {
		if citation.ActivityEntryID != "" && citation.Path != "" {
			return fmt.Errorf("criterion result %s: citation %d has both path %q and activity_entry_id %q; these are mutually exclusive",
				cr.ID, i, citation.Path, citation.ActivityEntryID)
		}
	}
	return nil
}

// IssueInfo captures the issue being reviewed.
type IssueInfo struct {
	// ID is the issue identifier.
	ID string `json:"id"`
	// Type is the issue type (e.g., "task", "story").
	Type string `json:"type"`
	// Title is the issue title.
	Title string `json:"title"`
	// Outcome is the recorded delivery outcome.
	Outcome string `json:"outcome"`
}

// Contract captures the contractual requirements for the delivery.
type Contract struct {
	// DefinitionOfDone is the primary criterion.
	DefinitionOfDone string `json:"definition_of_done"`
	// Scope is the list of files or areas in scope for the delivery.
	Scope []string `json:"scope,omitempty"`
	// Acceptance is an ordered list of acceptance criteria.
	Acceptance []string `json:"acceptance"`
}

// Delivery captures metadata about the completed delivery.
type Delivery struct {
	// BaseSHA is the parent commit SHA.
	BaseSHA string `json:"base_sha"`
	// HeadSHA is the delivery commit SHA.
	HeadSHA string `json:"head_sha"`
	// ChangedFiles lists the files modified in the delivery range.
	ChangedFiles []string `json:"changed_files"`
	// Diff is the unified diff for the delivery range.
	Diff string `json:"diff,omitempty"`
}

// Fingerprints captures canonical fingerprints for reproducibility.
type Fingerprints struct {
	// Contract is the SHA-256 fingerprint of the contract.
	Contract string `json:"contract"`
	// Delivery is the SHA-256 fingerprint of the delivery metadata.
	Delivery string `json:"delivery"`
}

// Activity captures execution evidence for a worktree.
type Activity struct {
	// Digest is the SHA-256 fingerprint of the activity log file content.
	Digest string `json:"digest"`
	// EntryCount is the total number of entries in the activity log.
	EntryCount int `json:"entry_count"`
	// DeliveryHeadCount is the number of entries at the delivery HEAD commit.
	DeliveryHeadCount int `json:"delivery_head_count"`
	// EarlierCount is the number of entries at commits before the delivery HEAD.
	EarlierCount int `json:"earlier_count"`
	// LogPath is the absolute path to the activity log file (see prepare.go's
	// attachActivitySection, which deliberately resolves it to absolute so record
	// time re-reads the same file regardless of working directory).
	LogPath string `json:"log_path"`
}

// ReviewBundle is the canonical input package for a reviewer.
type ReviewBundle struct {
	// SchemaVersion is the protocol version.
	SchemaVersion int `json:"schema_version"`
	// BundleID is a canonical identifier for this bundle.
	BundleID string `json:"bundle_id"`
	// Issue captures the reviewed issue.
	Issue IssueInfo `json:"issue"`
	// Contract captures the contractual requirements.
	Contract Contract `json:"contract"`
	// Delivery captures the delivery metadata.
	Delivery Delivery `json:"delivery"`
	// Fingerprints captures canonical fingerprints.
	Fingerprints Fingerprints `json:"fingerprints"`
	// Activity optionally captures execution evidence when a worktree activity log exists.
	Activity *Activity `json:"activity,omitempty"`
	// GateEvidence is wrapper-recorded gate runs from worker logs so reviewers can cite them.
	GateEvidence []ops.GateEvidence `json:"gate_evidence,omitempty"`
}

// Valid validates that the ReviewBundle is well-formed.
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

// ConformanceAssessment is the structured result returned by a reviewer.
type ConformanceAssessment struct {
	// SchemaVersion is the protocol version.
	SchemaVersion int `json:"schema_version"`
	// BundleID must match the prepared bundle.
	BundleID string `json:"bundle_id"`
	// Results contains one result per criterion.
	Results []CriterionResult `json:"results"`
	// ContractFingerprint must match the prepared contract fingerprint.
	ContractFingerprint string `json:"contract_fingerprint"`
	// DeliveryFingerprint must match the prepared delivery fingerprint.
	DeliveryFingerprint string `json:"delivery_fingerprint"`
}

// Valid validates that the ConformanceAssessment is well-formed.
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
	// Validate each result.
	for _, result := range ca.Results {
		if err := result.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// AssessmentAttestation is the compact durable record of a review.
type AssessmentAttestation struct {
	// SchemaVersion is the protocol version.
	SchemaVersion int `json:"schema_version"`
	// BundleID from the prepared bundle.
	BundleID string `json:"bundle_id"`
	// ContractFingerprint from the bundle.
	ContractFingerprint string `json:"contract_fingerprint"`
	// DeliveryFingerprint from the bundle.
	DeliveryFingerprint string `json:"delivery_fingerprint"`
	// ActivityDigest from the bundle (optional, present if activity log exists).
	ActivityDigest string `json:"activity_digest,omitempty"`
	// BaseSHA from the delivery range.
	BaseSHA string `json:"base_sha"`
	// HeadSHA from the delivery range.
	HeadSHA string `json:"head_sha"`
	// SkillVersion identifies the reviewer skill that produced this assessment.
	SkillVersion string `json:"skill_version,omitempty"`
	// ModelIdentity identifies the model/provider when exposed by the harness.
	ModelIdentity string `json:"model_identity,omitempty"`
	// Rating is the derived conformance rating.
	Rating Rating `json:"rating"`
	// ResultFingerprint is the SHA-256 of the detailed result for idempotence detection.
	ResultFingerprint string `json:"result_fingerprint"`
	// SatisfiedCount is the number of satisfied criteria.
	SatisfiedCount int `json:"satisfied_count"`
	// PartiallySatisfiedCount is the number of partially satisfied criteria.
	PartiallySatisfiedCount int `json:"partially_satisfied_count"`
	// NotSatisfiedCount is the number of not satisfied criteria.
	NotSatisfiedCount int `json:"not_satisfied_count"`
	// IndeterminateCount is the number of indeterminate criteria.
	IndeterminateCount int `json:"indeterminate_count"`
}
