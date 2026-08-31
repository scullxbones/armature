package review

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// criterionIDPattern is the canonical criterion-ID format: definition_of_done or acceptance[N].
var criterionIDPattern = regexp.MustCompile(`^definition_of_done$|^acceptance\[\d+\]$`)

var criterionIDAcceptanceIndex = regexp.MustCompile(`(?i)acceptance[^\d]*(\d+)`)

// ValidateResult checks that a ConformanceAssessment is well-formed:
// - bundle ID is non-empty
// - all CriterionResults are valid (call result.Valid())
// - all Citations reference lines present in the DiffIndex
// Returns a slice of validation error strings (empty = valid).
func ValidateResult(assessment *ConformanceAssessment, idx *DiffIndex) []string {
	var errs []string

	// Check bundle ID is non-empty
	if assessment.BundleID == "" {
		errs = append(errs, "bundle ID is empty")
	}

	// Validate each criterion result
	for i, result := range assessment.Results {
		if err := result.Valid(); err != nil {
			errs = append(errs, fmt.Sprintf("criterion result %d: %v", i, err))
		}

		// Validate citations reference lines present in the diff
		for _, citation := range result.Citations {
			// Activity citations reference the activity log, not the diff; they are validated
			// separately by ValidateActivityCitations/ValidateActivityDigestAndLoadEntries and have no Path.
			if citation.ActivityEntryID != "" {
				continue
			}

			// If Line is omitted (0), validate that the file is in the diff
			if citation.Line == 0 {
				if !idx.ContainsFile(citation.Path) {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: citation references %s which is not in diff (suggestion: remove the citation or cite a path present in the delivery diff)",
						result.ID, citation.Path))
				}
			} else {
				// If Line is specified, validate the specific line
				if !idx.ContainsLine(citation.Path, citation.Line) {
					msg := fmt.Sprintf("criterion result %s: citation references %s:%d which is not in diff", result.ID, citation.Path, citation.Line)
					if idx.ContainsFile(citation.Path) {
						msg += fmt.Sprintf(" (suggestion: downgrade citation to path-level; omit line %d and cite %s only)", citation.Line, citation.Path)
					} else {
						msg += " (suggestion: remove the citation or cite a path present in the delivery diff)"
					}
					errs = append(errs, msg)
				}
			}
		}
	}

	return errs
}

// ValidateResultNoDiff checks a ConformanceAssessment for structural validity without
// performing diff-index citation coordinate checking. This variant is appropriate at
// record time when the diff is not available; coordinate checking is a prepare-time concern.
func ValidateResultNoDiff(assessment *ConformanceAssessment) []string {
	var errs []string

	if assessment.BundleID == "" {
		errs = append(errs, "bundle ID is empty")
	}

	for i, result := range assessment.Results {
		if err := result.Valid(); err != nil {
			errs = append(errs, fmt.Sprintf("criterion result %d: %v", i, err))
		}
		if result.ID != "" && !validCriterionID(result.ID) {
			errs = append(errs, fmt.Sprintf("criterion result %d: invalid criterion ID %q (suggestion: %s)", i, result.ID, suggestCriterionID(result.ID)))
		}
	}

	return errs
}

func validCriterionID(id string) bool {
	return criterionIDPattern.MatchString(id)
}

func suggestCriterionID(id string) string {
	compact := strings.ToLower(strings.NewReplacer("-", "_", " ", "", "[", "", "]", "").Replace(id))
	if compact == "definition_of_done" || compact == "definitionofdone" || compact == "def_of_done" || compact == "dod" {
		return `use criterion id "definition_of_done"`
	}
	if m := criterionIDAcceptanceIndex.FindStringSubmatch(id); m != nil {
		return fmt.Sprintf(`use criterion id "acceptance[%s]"`, m[1])
	}
	return `use "definition_of_done" or "acceptance[N]" (N is a 0-based integer)`
}

// ValidateAssessment runs the same checks Record performs. It does not persist
// an attestation; arm review validate uses this so record remains the sole
// enforcement gate that appends ops. Every failure line includes an auto-fix
// suggestion.
func ValidateAssessment(input RecordInput) error {
	_, err := Record(input)
	return AnnotateValidateError(err)
}

const suggestionMarker = " (suggestion: "

// AnnotateValidateError appends an auto-fix suggestion to each validation
// failure line that does not already include one.
func AnnotateValidateError(err error) error {
	if err == nil {
		return nil
	}
	lines := strings.Split(err.Error(), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		body := trimmed
		bullet := ""
		if strings.HasPrefix(trimmed, "- ") {
			bullet = "- "
			body = strings.TrimPrefix(trimmed, "- ")
		}
		if body == "" || strings.HasSuffix(body, ":") {
			continue
		}
		if strings.Contains(body, suggestionMarker) {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + bullet + body + suggestionMarker + SuggestValidateFix(body) + ")"
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

// ValidateFix is the reviewer-facing auto-fix for one validation failure.
// Fixable is true when rewriting the assessment JSON can apply the suggestion.
type ValidateFix struct {
	Suggestion string
	Fixable    bool
}

// SuggestValidateFix returns a reviewer-facing auto-fix for a validation
// failure message from Record, Valid, or JSON decode.
func SuggestValidateFix(message string) string {
	return ClassifyValidateFix(message).Suggestion
}

// ClassifyValidateFix returns the auto-fix suggestion and whether rewriting
// the assessment can apply it. Bundle and issue-state failures are not fixable.
func ClassifyValidateFix(message string) ValidateFix {
	msg := strings.ToLower(message)
	fix := func(suggestion string) ValidateFix {
		return ValidateFix{Suggestion: suggestion, Fixable: true}
	}
	setup := func(suggestion string) ValidateFix {
		return ValidateFix{Suggestion: suggestion, Fixable: false}
	}
	switch {
	case strings.Contains(msg, "review bundle:"):
		return setup("re-run arm review prepare --output <bundle.json> and pass that file as --bundle")
	case strings.Contains(msg, "unsupported schema version"):
		return fix(fmt.Sprintf("set schema_version to %d", SchemaVersion))
	case strings.Contains(msg, "unknown field"):
		return fix("remove the unknown field or rename it to a documented schema property")
	case strings.Contains(msg, "column must be"):
		return fix("omit column or set it to a 1-based column number (>= 1)")
	case strings.Contains(msg, "line must be"):
		return fix("omit line or set it to an integer; JSON null is not allowed")
	case strings.Contains(msg, "citations must be"):
		return fix("set citations to an array of evidence, or [] with missing_evidence")
	case strings.Contains(msg, "invalid criterion status"):
		return fix(`set status to one of "satisfied", "partially_satisfied", "not_satisfied", "indeterminate"`)
	case strings.Contains(msg, "missing required field"):
		return fix("add the required field on the criterion result")
	case strings.Contains(msg, "parse assessment json"),
		strings.Contains(msg, "decode conformance assessment"),
		strings.Contains(msg, "unexpected trailing json"):
		return fix(fmt.Sprintf("emit JSON matching docs/schemas/conformance-assessment.schema.json with schema_version %d", SchemaVersion))
	case strings.Contains(msg, "parse bundle json"),
		strings.Contains(msg, "decode review bundle"):
		return setup("re-run arm review prepare --output <bundle.json> and pass that file as --bundle")
	case strings.Contains(msg, "missing bundle id"), strings.Contains(msg, "bundle id is empty"):
		return fix("copy bundle_id from the prepared review bundle")
	case strings.Contains(msg, "no results provided"):
		return fix("add one results[] entry per contract criterion")
	case strings.Contains(msg, "missing contract fingerprint"):
		return fix("copy fingerprints.contract from the prepared review bundle")
	case strings.Contains(msg, "missing delivery fingerprint"):
		return fix("copy fingerprints.delivery from the prepared review bundle")
	case strings.Contains(msg, "missing id"):
		return fix(`set id to "definition_of_done" or "acceptance[N]"`)
	case strings.Contains(msg, "missing rationale"):
		return fix("add a rationale explaining the criterion status")
	case strings.Contains(msg, "citations required"):
		return fix("add at least one citation, or lower the status from satisfied and set missing_evidence")
	case strings.Contains(msg, "missing evidence"), strings.Contains(msg, "citations or missing_evidence"):
		return fix("set missing_evidence to describe what is absent, or add citations")
	case strings.Contains(msg, "mutually exclusive"):
		return fix("keep either path or activity_entry_id on the citation, not both")
	case strings.Contains(msg, "delivery_fingerprint") && strings.Contains(msg, "does not match"):
		return fix("copy fingerprints.delivery from the prepared review bundle")
	case strings.Contains(msg, "issue contract fingerprint"):
		return setup("re-run arm review prepare --output <bundle.json> and pass that file as --bundle")
	case strings.Contains(msg, "contract fingerprint") && strings.Contains(msg, "does not match"),
		strings.Contains(msg, "contract_fingerprint") && strings.Contains(msg, "does not match"):
		return fix("copy fingerprints.contract from the prepared review bundle")
	case strings.Contains(msg, "bundle integrity"):
		return setup("re-run arm review prepare --output <bundle.json>; do not edit the bundle file")
	case strings.Contains(msg, "bundle_id") && strings.Contains(msg, "does not match"):
		return fix("set bundle_id to the prepared bundle's bundle_id")
	case strings.Contains(msg, "bundle was prepared for issue"):
		return setup("validate against the bundle's issue or re-run arm review prepare for this issue")
	case strings.Contains(msg, "duplicate id"):
		return fix("keep a single result for this criterion id")
	case strings.Contains(msg, "unexpected criterion id"):
		return fix(`rename to "definition_of_done" or "acceptance[N]" from the contract, or remove it`)
	case strings.Contains(msg, "missing expected id"):
		if id := firstQuoted(message); id != "" {
			return fix(fmt.Sprintf("add a criterion result with id %q", id))
		}
		return fix(`add a criterion result with id "definition_of_done" or "acceptance[N]"`)
	case strings.Contains(msg, "no bundle activity section"), strings.Contains(msg, "cites activity log entries"):
		return fix("re-run arm review prepare so the bundle includes activity, or drop activity_entry_id citations")
	case strings.Contains(msg, "invalid activity entry id"):
		return fix("cite a numeric activity_entry_id from the bundle activity log")
	case strings.Contains(msg, "unknown activity entry"):
		return fix("cite an activity_entry_id present in the activity log")
	case strings.Contains(msg, "earlier commits"):
		return fix("cite an activity entry executed at the delivery head_sha")
	case strings.Contains(msg, "unknown exit code"):
		return fix("do not use this entry to support satisfied; lower the status or cite an entry with a known zero exit code")
	case strings.Contains(msg, "failed exit code"):
		return fix("do not use a failed command as satisfied evidence; lower the status or cite a passing entry")
	case strings.Contains(msg, "upgrade-only"):
		return fix("add a diff citation (path) for this implementation criterion")
	case strings.Contains(msg, "activity log digest mismatch"):
		return setup("re-run arm review prepare so activity.digest matches the on-disk log")
	case strings.Contains(msg, "activity log missing or unreadable"):
		return setup("restore the activity log or re-run arm review prepare")
	case strings.Contains(msg, "activity log validation"):
		return setup("re-run arm review prepare so activity.digest matches the on-disk log")
	case strings.Contains(msg, "gate evidence"):
		return setup("re-run arm review prepare after restoring original gate evidence logs")
	case strings.Contains(msg, "build diff index"):
		return setup("re-run arm review prepare so Delivery.Diff is a well-formed unified diff")
	case strings.Contains(msg, "acceptance criteria"):
		return setup("fix the issue acceptance JSON and re-run arm review prepare")
	case strings.Contains(msg, "not in diff"):
		return fix("remove the citation or cite a path present in the delivery diff")
	default:
		return setup("fix the assessment to satisfy this check, then re-run arm review validate")
	}
}

func firstQuoted(s string) string {
	start := strings.Index(s, `"`)
	if start < 0 {
		return ""
	}
	rest := s[start+1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// NewAttestation creates an AssessmentAttestation from a validated ConformanceAssessment and
// its corresponding Delivery. The delivery's BaseSHA and HeadSHA are recorded in the attestation
// so the durable record captures the exact commit range that was reviewed.
// activity is optional (nil when the bundle had no Activity section); when present, its digest
// is carried into the attestation per ADR-0008 ("the digest enters the attestation").
func NewAttestation(assessment *ConformanceAssessment, delivery Delivery, activity *Activity) *AssessmentAttestation {
	// Derive rating and counts from results
	rating := DeriveRating(assessment.Results)
	satisfied, partiallySatisfied, notSatisfied, indeterminate := CountCriteria(assessment.Results)

	// Compute fingerprint of the results for idempotence detection
	resultFingerprint := FingerprintResult(*assessment)

	att := &AssessmentAttestation{
		SchemaVersion:           SchemaVersion,
		BundleID:                assessment.BundleID,
		ContractFingerprint:     assessment.ContractFingerprint,
		DeliveryFingerprint:     assessment.DeliveryFingerprint,
		BaseSHA:                 delivery.BaseSHA,
		HeadSHA:                 delivery.HeadSHA,
		Rating:                  rating,
		ResultFingerprint:       resultFingerprint,
		SatisfiedCount:          satisfied,
		PartiallySatisfiedCount: partiallySatisfied,
		NotSatisfiedCount:       notSatisfied,
		IndeterminateCount:      indeterminate,
	}

	if activity != nil {
		att.ActivityDigest = activity.Digest
	}

	return att
}

// IsDuplicate returns true if two attestations have the same ResultFingerprint,
// indicating they represent identical review content. This allows the same bundle
// to be re-assessed with a corrected result while remaining idempotent for identical content.
func IsDuplicate(a, b *AssessmentAttestation) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ResultFingerprint == b.ResultFingerprint
}

// Applicable returns true if the attestation's BundleID matches the given bundle.
func Applicable(att *AssessmentAttestation, bundleID string) bool {
	if att == nil {
		return false
	}
	return att.BundleID == bundleID
}

// ValidateResultCoverage checks that a ConformanceAssessment covers all expected criterion IDs
// from the contract and contains no duplicates. Returns a slice of validation error strings (empty = valid).
func ValidateResultCoverage(assessment *ConformanceAssessment, contract Contract) []string {
	var errs []string

	// Build expected criterion IDs from contract
	expectedIDs := make(map[string]bool)
	if contract.DefinitionOfDone != "" {
		expectedIDs["definition_of_done"] = true
	}
	for i := range contract.Acceptance {
		expectedIDs[fmt.Sprintf("acceptance[%d]", i)] = true
	}

	// Track submitted IDs, check for duplicates, and flag unexpected IDs.
	submittedIDs := make(map[string]bool)
	for _, result := range assessment.Results {
		if submittedIDs[result.ID] {
			errs = append(errs, fmt.Sprintf(
				"criterion result: duplicate ID %q (suggestion: keep a single result for this criterion id)", result.ID))
		}
		submittedIDs[result.ID] = true

		// Flag IDs that are not in the expected set.
		if !expectedIDs[result.ID] {
			errs = append(errs, fmt.Sprintf(
				"unexpected criterion ID %s: not in contract (suggestion: rename to a contract id or remove it)", result.ID))
		}
	}

	// Check for missing expected IDs
	for id := range expectedIDs {
		if !submittedIDs[id] {
			errs = append(errs, fmt.Sprintf(
				"criterion result: missing expected ID %q (suggestion: add a criterion result with id %q)", id, id))
		}
	}

	return errs
}

// hasActivityCitations reports whether any citation in the assessment references an activity
// log entry (as opposed to a diff line).
func hasActivityCitations(assessment *ConformanceAssessment) bool {
	for _, result := range assessment.Results {
		for _, citation := range result.Citations {
			if citation.ActivityEntryID != "" {
				return true
			}
		}
	}
	return false
}

// ValidateActivityCitations validates activity citations in a ConformanceAssessment against
// parsed activity log entries. It checks:
// - All cited entry IDs exist in the log
// - No activity entry with an unknown exit code can support a Satisfied criterion status
// - Activity-citations-only cannot support Satisfied or PartiallySatisfied on implementation criteria
// - All cited entries match the delivery's HeadSHA (reject entries from earlier commits)
// Returns a slice of validation error strings (empty = valid).
// deliveryHeadSHA is the expected commit SHA for the delivery; entries whose HeadSHA does
// not match this value are rejected (they represent evidence from earlier commits).
func ValidateActivityCitations(assessment *ConformanceAssessment, activity *Activity, entries map[int]ActivityEntryDetails, deliveryHeadSHA string) []string {
	var errs []string

	if activity == nil {
		// No activity section to validate against
		return errs
	}

	// Track which criteria have activity citations only
	activityOnlyByID := make(map[string]bool)

	// Validate each activity citation in the assessment
	for _, result := range assessment.Results {
		hasActivityCitation := false
		hasDiffCitation := false

		for _, citation := range result.Citations {
			if citation.ActivityEntryID != "" {
				hasActivityCitation = true

				// Reject citations referencing the "activity index" (use of "index" terminology)
				// Index would be something like "index:0" or "activity_index"
				// We only accept raw entry IDs which are numeric strings
				entryID, err := strconv.Atoi(citation.ActivityEntryID)
				if err != nil {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: invalid activity entry ID %q (must be numeric) (suggestion: cite a numeric activity_entry_id from the bundle activity log)",
						result.ID, citation.ActivityEntryID))
					continue
				}

				entry, ok := entries[entryID]
				if !ok {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: unknown activity entry ID %d (not present in the activity log) (suggestion: cite an activity_entry_id present in the activity log)",
						result.ID, entryID))
					continue
				}

				supportsPositiveStatus := result.Status == Satisfied || result.Status == PartiallySatisfied

				// Reject entries from earlier commits when they are being used to support a
				// Satisfied/PartiallySatisfied status: the cited entry must have been executed
				// at the delivery's HEAD commit, not at an earlier commit, to serve as evidence
				// that the current delivery passes. Citing an earlier-commit entry to support a
				// NotSatisfied status ("this was already broken before this commit too") is a
				// legitimate use and is not blocked here.
				if supportsPositiveStatus && deliveryHeadSHA != "" && entry.HeadSHA != deliveryHeadSHA {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: activity entry %d was executed at head_sha=%q but delivery head_sha=%q; "+
							"entries from earlier commits cannot be used as evidence for the current delivery "+
							"(suggestion: cite an activity entry executed at the delivery head_sha)",
						result.ID, entryID, entry.HeadSHA, deliveryHeadSHA))
					continue
				}

				// An entry with no recorded exit code (harness omitted it) cannot be used
				// as evidence that a criterion is fully satisfied — "unknown" and "succeeded"
				// must remain distinguishable outcomes for verified behavioral evidence.
				if !entry.ExitCodeKnown && result.Status == Satisfied {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: activity entry %d has an unknown exit code and cannot support satisfied status "+
							"(suggestion: lower the status or cite an entry with a known zero exit code)",
						result.ID, entryID))
				}

				// An entry with a known but nonzero exit code represents a failed command
				// execution and cannot be used as evidence that a criterion passed.
				if entry.ExitCodeKnown && entry.ExitCode != 0 && result.Status == Satisfied {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: activity entry %d has a failed exit code (%d) and cannot support satisfied status "+
							"(suggestion: lower the status or cite a passing activity entry)",
						result.ID, entryID, entry.ExitCode))
				}
			}

			if citation.Path != "" {
				hasDiffCitation = true
			}
		}

		// Track if this criterion has only activity citations
		if hasActivityCitation && !hasDiffCitation {
			activityOnlyByID[result.ID] = true
		}
	}

	// Check upgrade-only rule: activity-citations-only cannot satisfy or partially
	// satisfy implementation criteria (ADR-0008 rule 1).
	for criterionID := range activityOnlyByID {
		// Find the corresponding result
		var result *CriterionResult
		for i := range assessment.Results {
			if assessment.Results[i].ID == criterionID {
				result = &assessment.Results[i]
				break
			}
		}

		if result == nil {
			continue
		}

		// Check if this is an implementation criterion
		// Implementation criteria are: definition_of_done
		// (Acceptance criteria and custom criteria are behavioral and can be satisfied by activity evidence)
		isImplementationCriterion := (criterionID == "definition_of_done")

		// If activity-citations-only is used on an implementation criterion with a
		// Satisfied or PartiallySatisfied status, reject it.
		if isImplementationCriterion && (result.Status == Satisfied || result.Status == PartiallySatisfied) {
			msg := fmt.Sprintf(
				"criterion result %s: activity citations alone cannot support %s on implementation criterion (upgrade-only rule) "+
					"(suggestion: add a diff citation (path) for this implementation criterion)",
				result.ID, result.Status,
			)
			errs = append(errs, msg)
		}
	}

	return errs
}
