package review

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var criterionIDPattern = regexp.MustCompile(`^definition_of_done$|^acceptance\[\d+\]$`)

var criterionIDAcceptanceIndex = regexp.MustCompile(`(?i)acceptance[^\d]*(\d+)`)

func ValidateResult(assessment *ConformanceAssessment, idx *DiffIndex) []string {
	var errs []string

	if assessment.BundleID == "" {
		errs = append(errs, "bundle ID is empty")
	}

	for i, result := range assessment.Results {
		if err := result.Valid(); err != nil {
			errs = append(errs, fmt.Sprintf("criterion result %d: %v", i, err))
		}

		for _, citation := range result.Citations {
			if citation.ActivityEntryID() != "" {
				continue
			}

			if citation.Line() == 0 {
				if !idx.ContainsFile(citation.Path()) {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: citation references %s which is not in diff (suggestion: remove the citation or cite a path present in the delivery diff)",
						result.ID, citation.Path()))
				}
			} else {
				if !idx.ContainsLine(citation.Path(), citation.Line()) {
					msg := fmt.Sprintf("criterion result %s: citation references %s:%d which is not in diff", result.ID, citation.Path(), citation.Line())
					if idx.ContainsFile(citation.Path()) {
						msg += fmt.Sprintf(" (suggestion: downgrade citation to path-level; omit line %d and cite %s only)", citation.Line(), citation.Path())
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
		lines[i] = indent + bullet + body + suggestionMarker + ClassifyValidateFix(body).Suggestion + ")"
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

type ValidateFix struct {
	Suggestion string
	Fixable    bool
}

func ClassifyValidateFix(message string) ValidateFix {
	msg := strings.ToLower(message)
	for _, rule := range validateFixRules {
		if !rule.match(msg) {
			continue
		}
		text := rule.text
		if rule.schema {
			text = fmt.Sprintf(text, SchemaVersion)
		}
		if rule.quoted {
			if id := firstQuoted(message); id != "" {
				text = fmt.Sprintf("add a criterion result with id %q", id)
			}
		}
		return ValidateFix{Suggestion: text, Fixable: rule.fixable}
	}
	return ValidateFix{
		Suggestion: "fix the assessment to satisfy this check, then re-run arm review validate",
		Fixable:    false,
	}
}

const (
	fixPrepareBundle    = "re-run arm review prepare --output <bundle.json> and pass that file as --bundle"
	fixCopyDeliveryFP   = "copy fingerprints.delivery from the prepared review bundle"
	fixCopyContractFP   = "copy fingerprints.contract from the prepared review bundle"
	fixActivityDigest   = "re-run arm review prepare so activity.digest matches the on-disk log"
	fixDefaultCriterion = `add a criterion result with id "definition_of_done" or "acceptance[N]"`
	fixAssessmentJSON   = "emit JSON matching docs/schemas/conformance-assessment.schema.json with schema_version %d"
	fixDropActivityCite = "re-run arm review prepare so the bundle includes activity, or drop activity_entry_id citations"
	fixUnknownExit      = "do not use this entry to support satisfied; lower the status or cite an entry with a known zero exit code"
	fixFailedExit       = "do not use a failed command as satisfied evidence; lower the status or cite a passing entry"
	fixCitationsReq     = "add at least one citation, or lower the status from satisfied and set missing_evidence"
	fixInvalidStatus    = `set status to one of "satisfied", "partially_satisfied", "not_satisfied", "indeterminate"`
	fixUnexpectedID     = `rename to "definition_of_done" or "acceptance[N]" from the contract, or remove it`
	fixUnknownField     = "remove the unknown field or rename it to a documented schema property"
)

type validateFixRule struct {
	any     []string
	all     []string
	fixable bool
	text    string
	schema  bool
	quoted  bool
}

func (r validateFixRule) match(msg string) bool {
	for _, n := range r.all {
		if !strings.Contains(msg, n) {
			return false
		}
	}
	if len(r.any) == 0 {
		return true
	}
	for _, n := range r.any {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

var validateFixRules = []validateFixRule{
	{any: []string{"review bundle:"}, text: fixPrepareBundle},
	{any: []string{"unsupported schema version"}, fixable: true, text: "set schema_version to %d", schema: true},
	{any: []string{"unknown field"}, fixable: true, text: fixUnknownField},
	{any: []string{"column must be"}, fixable: true, text: "omit column or set it to a 1-based column number (>= 1)"},
	{any: []string{"line must be"}, fixable: true, text: "omit line or set it to an integer; JSON null is not allowed"},
	{any: []string{"citations must be"}, fixable: true, text: "set citations to an array of evidence, or [] with missing_evidence"},
	{any: []string{"invalid criterion status"}, fixable: true, text: fixInvalidStatus},
	{any: []string{"missing required field"}, fixable: true, text: "add the required field on the criterion result"},
	{
		any:     []string{"parse assessment json", "decode conformance assessment", "unexpected trailing json"},
		fixable: true, text: fixAssessmentJSON, schema: true,
	},
	{any: []string{"parse bundle json", "decode review bundle"}, text: fixPrepareBundle},
	{any: []string{"missing bundle id", "bundle id is empty"}, fixable: true, text: "copy bundle_id from the prepared review bundle"},
	{any: []string{"no results provided"}, fixable: true, text: "add one results[] entry per contract criterion"},
	{any: []string{"missing contract fingerprint"}, fixable: true, text: fixCopyContractFP},
	{any: []string{"missing delivery fingerprint"}, fixable: true, text: fixCopyDeliveryFP},
	{any: []string{"missing id"}, fixable: true, text: `set id to "definition_of_done" or "acceptance[N]"`},
	{any: []string{"missing rationale"}, fixable: true, text: "add a rationale explaining the criterion status"},
	{any: []string{"citations required"}, fixable: true, text: fixCitationsReq},
	{any: []string{"missing evidence", "citations or missing_evidence"}, fixable: true, text: "set missing_evidence to describe what is absent, or add citations"},
	{any: []string{"mutually exclusive"}, fixable: true, text: "keep either path or activity_entry_id on the citation, not both"},
	{all: []string{"delivery_fingerprint", "does not match"}, fixable: true, text: fixCopyDeliveryFP},
	{any: []string{"issue contract fingerprint"}, text: fixPrepareBundle},
	{all: []string{"contract fingerprint", "does not match"}, fixable: true, text: fixCopyContractFP},
	{all: []string{"contract_fingerprint", "does not match"}, fixable: true, text: fixCopyContractFP},
	{any: []string{"bundle integrity"}, text: "re-run arm review prepare --output <bundle.json>; do not edit the bundle file"},
	{all: []string{"bundle_id", "does not match"}, fixable: true, text: "set bundle_id to the prepared bundle's bundle_id"},
	{any: []string{"bundle was prepared for issue"}, text: "validate against the bundle's issue or re-run arm review prepare for this issue"},
	{any: []string{"duplicate id"}, fixable: true, text: "keep a single result for this criterion id"},
	{any: []string{"unexpected criterion id"}, fixable: true, text: fixUnexpectedID},
	{any: []string{"missing expected id"}, fixable: true, text: fixDefaultCriterion, quoted: true},
	{any: []string{"no bundle activity section", "cites activity log entries"}, fixable: true, text: fixDropActivityCite},
	{any: []string{"invalid activity entry id"}, fixable: true, text: "cite a numeric activity_entry_id from the bundle activity log"},
	{any: []string{"unknown activity entry"}, fixable: true, text: "cite an activity_entry_id present in the activity log"},
	{any: []string{"earlier commits"}, fixable: true, text: "cite an activity entry executed at the delivery head_sha"},
	{any: []string{"unknown exit code"}, fixable: true, text: fixUnknownExit},
	{any: []string{"failed exit code"}, fixable: true, text: fixFailedExit},
	{any: []string{"upgrade-only"}, fixable: true, text: "add a diff citation (path) for this implementation criterion"},
	{any: []string{"activity log digest mismatch"}, text: fixActivityDigest},
	{any: []string{"activity log missing or unreadable"}, text: "restore the activity log or re-run arm review prepare"},
	{any: []string{"activity log validation"}, text: fixActivityDigest},
	{any: []string{"gate evidence"}, text: "re-run arm review prepare after restoring original gate evidence logs"},
	{any: []string{"build diff index"}, text: "re-run arm review prepare so Delivery.Diff is a well-formed unified diff"},
	{any: []string{"acceptance criteria"}, text: "fix the issue acceptance JSON and re-run arm review prepare"},
	{any: []string{"not in diff"}, fixable: true, text: "remove the citation or cite a path present in the delivery diff"},
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

// NewAttestation records delivery BaseSHA/HeadSHA and, when activity is present,
// carries activity.Digest into the attestation per ADR-0008.
func NewAttestation(assessment *ConformanceAssessment, delivery Delivery, activity *Activity) *AssessmentAttestation {
	rating := DeriveRating(assessment.Results)
	satisfied, partiallySatisfied, notSatisfied, indeterminate := CountCriteria(assessment.Results)

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

func IsDuplicate(a, b *AssessmentAttestation) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ResultFingerprint == b.ResultFingerprint
}

func ValidateResultCoverage(assessment *ConformanceAssessment, contract Contract) []string {
	var errs []string

	expectedIDs := make(map[string]bool)
	if contract.DefinitionOfDone != "" {
		expectedIDs["definition_of_done"] = true
	}
	for i := range contract.Acceptance {
		expectedIDs[fmt.Sprintf("acceptance[%d]", i)] = true
	}

	submittedIDs := make(map[string]bool)
	for _, result := range assessment.Results {
		if submittedIDs[result.ID] {
			errs = append(errs, fmt.Sprintf(
				"criterion result: duplicate ID %q (suggestion: keep a single result for this criterion id)", result.ID))
		}
		submittedIDs[result.ID] = true

		if !expectedIDs[result.ID] {
			errs = append(errs, fmt.Sprintf(
				"unexpected criterion ID %s: not in contract (suggestion: rename to a contract id or remove it)", result.ID))
		}
	}

	for id := range expectedIDs {
		if !submittedIDs[id] {
			errs = append(errs, fmt.Sprintf(
				"criterion result: missing expected ID %q (suggestion: add a criterion result with id %q)", id, id))
		}
	}

	return errs
}

func hasActivityCitations(assessment *ConformanceAssessment) bool {
	for _, result := range assessment.Results {
		for _, citation := range result.Citations {
			if citation.ActivityEntryID() != "" {
				return true
			}
		}
	}
	return false
}

// ValidateActivityCitations enforces ADR-0008 on activity citations: upgrade-only
// (activity cannot satisfy implementation criteria alone), HEAD-anchored entries,
// and harness-recorded exit status. Empty return means valid.
func ValidateActivityCitations(assessment *ConformanceAssessment, activity *Activity, entries map[int]ActivityEntryDetails, deliveryHeadSHA string) []string {
	var errs []string

	if activity == nil {
		return errs
	}

	activityOnlyByID := make(map[string]bool)

	for _, result := range assessment.Results {
		hasActivityCitation := false
		hasDiffCitation := false

		for _, citation := range result.Citations {
			if citation.ActivityEntryID() != "" {
				hasActivityCitation = true

				entryID, err := strconv.Atoi(citation.ActivityEntryID())
				if err != nil {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: invalid activity entry ID %q (must be numeric) (suggestion: cite a numeric activity_entry_id from the bundle activity log)",
						result.ID, citation.ActivityEntryID()))
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

				if supportsPositiveStatus && deliveryHeadSHA != "" && entry.HeadSHA != deliveryHeadSHA {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: activity entry %d was executed at head_sha=%q but delivery head_sha=%q; "+
							"entries from earlier commits cannot be used as evidence for the current delivery "+
							"(suggestion: cite an activity entry executed at the delivery head_sha)",
						result.ID, entryID, entry.HeadSHA, deliveryHeadSHA))
					continue
				}

				if !entry.ExitCodeKnown && result.Status == Satisfied {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: activity entry %d has an unknown exit code and cannot support satisfied status "+
							"(suggestion: lower the status or cite an entry with a known zero exit code)",
						result.ID, entryID))
				}

				if entry.ExitCodeKnown && entry.ExitCode != 0 && result.Status == Satisfied {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: activity entry %d has a failed exit code (%d) and cannot support satisfied status "+
							"(suggestion: lower the status or cite a passing activity entry)",
						result.ID, entryID, entry.ExitCode))
				}
			}

			if citation.Path() != "" {
				hasDiffCitation = true
			}
		}

		if hasActivityCitation && !hasDiffCitation {
			activityOnlyByID[result.ID] = true
		}
	}

	for criterionID := range activityOnlyByID {
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

		isImplementationCriterion := (criterionID == "definition_of_done")

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
