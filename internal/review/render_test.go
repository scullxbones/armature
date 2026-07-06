package review_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderExcludesRawOutput_REQ_EXECEV_T3 exercises the full activity pipeline end to end:
// a real activity log entry containing a distinctive raw-output sentinel is parsed via
// LoadActivityEntries/FormatActivityEntryDetails (as record.go does), then rendered through
// both RenderMarkdown and RenderHuman. The rendered reports must surface the entry ID, command
// line, and exit status, but must never leak the raw command output.
func TestRenderExcludesRawOutput_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	const sentinel = "SENTINEL_RAW_BUILD_OUTPUT_DO_NOT_RENDER_9f3c2a"

	logPath := filepath.Join(t.TempDir(), "armature-activity.log")
	logLine := `2026-01-15T10:30:45Z activity: command="make build" exit_code=0 head_sha=abc123 output="` + sentinel + ` compiling... done"` + "\n"
	require.NoError(t, os.WriteFile(logPath, []byte(logLine), 0o600))

	entries := review.LoadActivityEntries(logPath)
	require.Len(t, entries, 1)
	details := entries[0]
	require.Equal(t, "make build", details.Command)
	require.Equal(t, 0, details.ExitCode)

	formatted := review.FormatActivityEntryDetails(details)
	require.NotContains(t, formatted, sentinel, "FormatActivityEntryDetails must not leak raw output")

	assessment := &review.ConformanceAssessment{
		SchemaVersion:       1,
		BundleID:            "bundle-raw-output",
		ContractFingerprint: "contract-fp",
		DeliveryFingerprint: "delivery-fp",
		Results: []review.CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    review.Satisfied,
				Rationale: "Build succeeded",
				Citations: []review.Citation{
					{
						ActivityEntryID:      "0",
						ActivityEntryDetails: formatted,
					},
				},
			},
		},
	}

	markdown := review.RenderMarkdown(assessment)
	human := review.RenderHuman(assessment)

	for _, output := range []string{markdown, human} {
		assert.Contains(t, output, "entry 0")
		assert.Contains(t, output, "make build")
		assert.Contains(t, output, "exit_code=0")
		assert.NotContains(t, output, sentinel, "rendered report must not include raw activity log output")
	}
}

func TestRenderMarkdown(t *testing.T) {
	t.Parallel()

	t.Run("basic assessment with all statuses", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-001",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Requirement met",
					Citations: []review.Citation{
						{Path: "src/main.go", Line: 10},
					},
				},
				{
					ID:              "criterion_2",
					Status:          review.PartiallySatisfied,
					Rationale:       "Partially complete",
					MissingEvidence: "Test coverage incomplete",
				},
				{
					ID:              "criterion_3",
					Status:          review.NotSatisfied,
					Rationale:       "Not implemented",
					MissingEvidence: "No implementation found",
				},
				{
					ID:              "criterion_4",
					Status:          review.Indeterminate,
					Rationale:       "Unclear from evidence",
					MissingEvidence: "Ambiguous requirements",
				},
			},
		}

		output := review.RenderMarkdown(assessment)

		// Check for table header
		assert.Contains(t, output, "| Criterion ID")
		assert.Contains(t, output, "| Status")
		assert.Contains(t, output, "| Rating")

		// Check for criterion rows
		assert.Contains(t, output, "criterion_1")
		assert.Contains(t, output, "criterion_2")
		assert.Contains(t, output, "criterion_3")
		assert.Contains(t, output, "criterion_4")

		// Check for status values
		assert.Contains(t, output, "satisfied")
		assert.Contains(t, output, "partially_satisfied")
		assert.Contains(t, output, "not_satisfied")
		assert.Contains(t, output, "indeterminate")

		// Check for overall rating (Red because one not_satisfied)
		assert.Contains(t, output, "red")

		// Check for rationale section
		assert.Contains(t, output, "Requirement met")
		assert.Contains(t, output, "Partially complete")
		assert.Contains(t, output, "Not implemented")
	})

	t.Run("html escaping in rationale", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-002",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Code includes <script> tags & special characters",
				},
			},
		}

		output := review.RenderMarkdown(assessment)

		// HTML entities should be escaped
		assert.Contains(t, output, "&lt;")
		assert.Contains(t, output, "&gt;")
		assert.Contains(t, output, "&amp;")
		assert.NotContains(t, output, "<script>")
	})

	t.Run("table cell escaping for pipes", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-003",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion|with|pipes",
					Status:    review.Satisfied,
					Rationale: "Rationale | with | pipes",
				},
			},
		}

		output := review.RenderMarkdown(assessment)

		// Pipes in table cells must be escaped
		assert.Contains(t, output, "criterion\\|with\\|pipes")
		assert.Contains(t, output, "Rationale \\| with \\| pipes")
	})

	t.Run("table cell escaping for backticks", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-004",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion",
					Status:    review.Satisfied,
					Rationale: "Code with `backticks` in text",
				},
			},
		}

		output := review.RenderMarkdown(assessment)

		// Backticks in table cells must be escaped
		assert.Contains(t, output, "\\`backticks\\`")
	})

	t.Run("table cell newline normalization", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-005",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion",
					Status:    review.Satisfied,
					Rationale: "Rationale with\nnewline\rand\r\nCRLF",
				},
			},
		}

		output := review.RenderMarkdown(assessment)

		// Newlines should be replaced with spaces in table cells
		for line := range strings.SplitSeq(output, "\n") {
			// Lines that are part of the table should not have embedded newlines
			if strings.Contains(line, "Rationale") {
				assert.NotContains(t, line, "\n")
				assert.NotContains(t, line, "\r")
			}
		}
	})

	t.Run("citations included", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-006",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Requirement met",
					Citations: []review.Citation{
						{Path: "src/main.go", Line: 10},
						{Path: "src/util.go", Line: 25},
					},
				},
			},
		}

		output := review.RenderMarkdown(assessment)

		// Citations should be included
		assert.Contains(t, output, "src/main.go")
		assert.Contains(t, output, "src/util.go")
		assert.Contains(t, output, "10")
		assert.Contains(t, output, "25")
	})

	t.Run("all green rating", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-007",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Met",
				},
				{
					ID:        "criterion_2",
					Status:    review.Satisfied,
					Rationale: "Met",
				},
			},
		}

		output := review.RenderMarkdown(assessment)

		// Should have green rating when all satisfied
		assert.Contains(t, output, "green")
		assert.NotContains(t, output, "| red |")
	})

	t.Run("yellow rating", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-008",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Met",
				},
				{
					ID:              "criterion_2",
					Status:          review.PartiallySatisfied,
					Rationale:       "Partial",
					MissingEvidence: "Incomplete",
				},
			},
		}

		output := review.RenderMarkdown(assessment)

		// Should have yellow rating when some partial but none not satisfied
		assert.Contains(t, output, "yellow")
	})

	t.Run("activity entry citation rendered with details_REQ_EXECEV_T3", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-009",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "acceptance[0]",
					Status:    review.Satisfied,
					Rationale: "Test passed",
					Citations: []review.Citation{
						{
							ActivityEntryID:      "0",
							ActivityEntryDetails: `entry 0: command="make test" exit_code=0`,
						},
					},
				},
			},
		}

		output := review.RenderMarkdown(assessment)

		// Should include activity entry details with entry ID, command, and exit status
		assert.Contains(t, output, "Activity:")
		assert.Contains(t, output, "entry 0")
		assert.Contains(t, output, "make test")
		assert.Contains(t, output, "exit_code=0")
		// Should NOT include raw output
		assert.NotContains(t, output, "... [output truncated")
	})
}

func TestRenderHuman(t *testing.T) {
	t.Parallel()

	t.Run("basic assessment", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-001",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Requirement met",
					Citations: []review.Citation{
						{Path: "src/main.go", Line: 10},
					},
				},
				{
					ID:              "criterion_2",
					Status:          review.NotSatisfied,
					Rationale:       "Not implemented",
					MissingEvidence: "No code found",
				},
			},
		}

		output := review.RenderHuman(assessment)

		// Should contain readable text representations
		assert.Contains(t, output, "Bundle")
		assert.Contains(t, output, "bundle-001")
		assert.Contains(t, output, "criterion_1")
		assert.Contains(t, output, "criterion_2")
		assert.Contains(t, output, "satisfied")
		assert.Contains(t, output, "not_satisfied")
		assert.Contains(t, output, "Requirement met")
		assert.Contains(t, output, "Not implemented")
	})

	t.Run("no markdown table in human output", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-002",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Met",
				},
			},
		}

		output := review.RenderHuman(assessment)

		// Should not contain markdown table syntax
		assert.NotContains(t, output, "|---|")
		assert.NotContains(t, output, "| Criterion")
	})

	t.Run("citations in human output", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-003",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Requirement met",
					Citations: []review.Citation{
						{Path: "src/main.go", Line: 10},
					},
				},
			},
		}

		output := review.RenderHuman(assessment)

		// Citations should be readable
		assert.Contains(t, output, "src/main.go")
		assert.Contains(t, output, "10")
	})

	t.Run("activity entry citation in human output_REQ_EXECEV_T3", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-010",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "acceptance[0]",
					Status:    review.Satisfied,
					Rationale: "Test executed",
					Citations: []review.Citation{
						{
							ActivityEntryID:      "1",
							ActivityEntryDetails: `entry 1: command="make lint" exit_code=0`,
						},
					},
				},
			},
		}

		output := review.RenderHuman(assessment)

		// Activity entry details should be shown with entry ID, command, exit status
		assert.Contains(t, output, "Activity:")
		assert.Contains(t, output, "entry 1")
		assert.Contains(t, output, "make lint")
		assert.Contains(t, output, "exit_code=0")
	})
}

func TestRenderJSON(t *testing.T) {
	t.Parallel()

	t.Run("valid json output", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-001",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Requirement met",
					Citations: []review.Citation{
						{Path: "src/main.go", Line: 10},
					},
				},
			},
		}

		output, err := review.RenderJSON(assessment)
		require.NoError(t, err)

		// Should be valid JSON
		var decoded review.ConformanceAssessment
		err = json.Unmarshal([]byte(output), &decoded)
		require.NoError(t, err)

		// Should round-trip
		assert.Equal(t, assessment.BundleID, decoded.BundleID)
		assert.Equal(t, len(assessment.Results), len(decoded.Results))
		assert.Equal(t, assessment.Results[0].ID, decoded.Results[0].ID)
	})

	t.Run("json is indented", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-001",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "criterion_1",
					Status:    review.Satisfied,
					Rationale: "Requirement met",
				},
			},
		}

		output, err := review.RenderJSON(assessment)
		require.NoError(t, err)

		// Should be indented (multiple lines with leading spaces)
		assert.Contains(t, output, "\n")
		assert.True(t, strings.Contains(output, "  ") || strings.Contains(output, "\t"),
			"JSON should be indented")
	})

	t.Run("all statuses roundtrip", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-002",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results: []review.CriterionResult{
				{
					ID:        "satisfied",
					Status:    review.Satisfied,
					Rationale: "Met",
				},
				{
					ID:              "partial",
					Status:          review.PartiallySatisfied,
					Rationale:       "Partial",
					MissingEvidence: "Incomplete",
				},
				{
					ID:              "not_satisfied",
					Status:          review.NotSatisfied,
					Rationale:       "Not met",
					MissingEvidence: "Missing",
				},
				{
					ID:              "indeterminate",
					Status:          review.Indeterminate,
					Rationale:       "Unclear",
					MissingEvidence: "Ambiguous",
				},
			},
		}

		output, err := review.RenderJSON(assessment)
		require.NoError(t, err)

		var decoded review.ConformanceAssessment
		err = json.Unmarshal([]byte(output), &decoded)
		require.NoError(t, err)

		// All statuses should roundtrip correctly
		assert.Equal(t, review.Satisfied, decoded.Results[0].Status)
		assert.Equal(t, review.PartiallySatisfied, decoded.Results[1].Status)
		assert.Equal(t, review.NotSatisfied, decoded.Results[2].Status)
		assert.Equal(t, review.Indeterminate, decoded.Results[3].Status)
	})

	t.Run("empty results", func(t *testing.T) {
		t.Parallel()
		assessment := &review.ConformanceAssessment{
			SchemaVersion:       1,
			BundleID:            "bundle-003",
			ContractFingerprint: "contract-fp",
			DeliveryFingerprint: "delivery-fp",
			Results:             []review.CriterionResult{},
		}

		output, err := review.RenderJSON(assessment)
		require.NoError(t, err)

		var decoded review.ConformanceAssessment
		err = json.Unmarshal([]byte(output), &decoded)
		require.NoError(t, err)

		assert.Equal(t, 0, len(decoded.Results))
	})
}
