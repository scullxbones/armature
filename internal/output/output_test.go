package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/review"
	"github.com/scullxbones/armature/internal/traceability"
	"github.com/scullxbones/armature/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderIssue_HumanReadable(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:               "TASK-01",
		Type:             "task",
		Status:           "open",
		Title:            "Test Issue",
		Parent:           "STORY-01",
		Priority:         "high",
		DefinitionOfDone: "All tests pass",
		Scope:            []string{"file1.go", "file2.go"},
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "TASK-01")
	assert.Contains(t, output, "Test Issue")
	assert.Contains(t, output, "task")
	assert.Contains(t, output, "open")
	assert.Contains(t, output, "STORY-01")
	assert.Contains(t, output, "high")
}

func TestRenderIssue_JSON(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:     "TASK-01",
		Type:   "task",
		Status: "open",
		Title:  "Test Issue",
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, true)
	require.NoError(t, err)
	output := buf.String()

	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err)
	assert.Equal(t, "TASK-01", result["id"])
	assert.Equal(t, "Test Issue", result["title"])
	assert.Equal(t, "task", result["type"])
	assert.Equal(t, "open", result["status"])
}

func TestRenderList_Empty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	var entries []ListEntry
	err := RenderList(&buf, entries)
	require.NoError(t, err)
	output := buf.String()
	assert.Equal(t, "", output)
}

func TestRenderList_HumanReadable(t *testing.T) {
	t.Parallel()
	entries := []ListEntry{
		{
			Issue:      "TASK-01",
			Title:      "First Task",
			Status:     "open",
			AssignedTo: "worker-1",
		},
		{
			Issue:  "TASK-02",
			Title:  "Second Task",
			Status: "done",
		},
	}
	var buf bytes.Buffer
	err := RenderList(&buf, entries)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "TASK-01")
	assert.Contains(t, output, "First Task")
	assert.Contains(t, output, "TASK-02")
	assert.Contains(t, output, "Second Task")
}

func TestRenderReady_HumanReadable(t *testing.T) {
	t.Parallel()
	entries := []ready.ReadyEntry{
		{
			Issue:                "TASK-01",
			Type:                 "task",
			Title:                "Ready Task",
			Priority:             "high",
			RequiresConfirmation: false,
			AssignedWorker:       "worker-1",
		},
	}
	var buf bytes.Buffer
	err := RenderReady(&buf, entries, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "TASK-01")
	assert.Contains(t, output, "Ready Task")
	assert.Contains(t, output, "high")
}

func TestRenderExpiredClaims_HumanReadable_REQ_TOPTIER_S4_T3(t *testing.T) {
	t.Parallel()
	claims := []ready.ExpiredClaimEntry{
		{Issue: "TASK-01", Title: "Expired Task", Status: "claimed", ClaimedBy: "worker-1"},
	}
	var buf bytes.Buffer
	require.NoError(t, RenderExpiredClaims(&buf, claims, false))
	out := buf.String()
	assert.Contains(t, out, "Expired claims")
	assert.Contains(t, out, "TASK-01")
	assert.Contains(t, out, "Expired Task")
	assert.Contains(t, out, "worker-1")
}

func TestRenderExpiredClaims_HumanReadable_Empty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	require.NoError(t, RenderExpiredClaims(&buf, nil, false))
	assert.Empty(t, buf.String(), "nothing to surface when there are no expired claims")
}

func TestRenderExpiredClaims_JSON(t *testing.T) {
	t.Parallel()
	claims := []ready.ExpiredClaimEntry{
		{Issue: "TASK-01", Title: "Expired Task", Status: "in-progress", ClaimedBy: "worker-1"},
	}
	var buf bytes.Buffer
	require.NoError(t, RenderExpiredClaims(&buf, claims, true))
	var result []map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "TASK-01", result[0]["issue"])
	assert.Equal(t, "in-progress", result[0]["status"])
}

func TestRenderReady_JSON(t *testing.T) {
	t.Parallel()
	entries := []ready.ReadyEntry{
		{
			Issue:    "TASK-01",
			Type:     "task",
			Title:    "Ready Task",
			Priority: "high",
		},
	}
	var buf bytes.Buffer
	err := RenderReady(&buf, entries, true)
	require.NoError(t, err)
	output := buf.String()

	var result []map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "TASK-01", result[0]["issue"])
	assert.Equal(t, "Ready Task", result[0]["title"])
}

func TestRenderValidation_ErrorsOnly(t *testing.T) {
	t.Parallel()
	result := validate.Result{
		OK:       false,
		Errors:   []string{"error 1", "error 2"},
		Warnings: []string{},
		Infos:    []string{},
	}
	var buf bytes.Buffer
	err := RenderValidation(&buf, result, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "ERROR: error 1")
	assert.Contains(t, output, "ERROR: error 2")
}

func TestRenderValidation_WithWarnings(t *testing.T) {
	t.Parallel()
	result := validate.Result{
		OK:       true,
		Errors:   []string{},
		Warnings: []string{"warning 1"},
		Infos:    []string{},
	}
	var buf bytes.Buffer
	err := RenderValidation(&buf, result, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "WARNING: warning 1")
}

func TestRenderValidation_AllOK(t *testing.T) {
	t.Parallel()
	result := validate.Result{
		OK:       true,
		Errors:   []string{},
		Warnings: []string{},
		Infos:    []string{},
	}
	var buf bytes.Buffer
	err := RenderValidation(&buf, result, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "OK: no issues found")
}

func TestRenderValidation_QuietSuppressesInfo(t *testing.T) {
	t.Parallel()
	result := validate.Result{
		OK:     true,
		Errors: []string{},
		Infos:  []string{"info line"},
	}
	var buf bytes.Buffer
	err := RenderValidation(&buf, result, true)
	require.NoError(t, err)
	output := buf.String()
	assert.NotContains(t, output, "INFO:")
	assert.Contains(t, output, "OK: no issues found")
}

func TestListEntry_struct(t *testing.T) {
	t.Parallel()
	// Verify that ListEntry struct can be created with expected fields.
	// ListEntry only contains fields that are both populated by callers and rendered in output.
	entry := ListEntry{
		Issue:      "TASK-01",
		Title:      "Test",
		Status:     "open",
		AssignedTo: "worker-1",
	}
	assert.Equal(t, "TASK-01", entry.Issue)
	assert.Equal(t, "Test", entry.Title)
	assert.Equal(t, "open", entry.Status)
	assert.Equal(t, "worker-1", entry.AssignedTo)
}

func TestRenderIssue_WithAllFields(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:               "TASK-01",
		Type:             "task",
		Status:           "in-progress",
		Title:            "Complex Issue",
		Parent:           "STORY-01",
		Priority:         "critical",
		DefinitionOfDone: "Feature complete and tested",
		Scope:            []string{"cmd/main.go", "internal/logic.go", "internal/logic_test.go"},
		Outcome:          "Implemented feature X with 95% test coverage",
		ClaimedBy:        "worker-id-123",
		AssignedWorker:   "worker-id-456",
		BlockedBy:        []string{"TASK-00"},
		Blocks:           []string{"TASK-02"},
		Notes: []materialize.Note{
			{Msg: "Investigation complete", Deleted: false},
			{Msg: "Stale note", Deleted: true},
		},
		Acceptance: json.RawMessage(`{"scenario":"when task is done"}`),
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "TASK-01")
	assert.Contains(t, output, "Complex Issue")
	assert.Contains(t, output, "in-progress")
	assert.Contains(t, output, "STORY-01")
	assert.Contains(t, output, "critical")
	assert.Contains(t, output, "Feature complete and tested")
	assert.Contains(t, output, "cmd/main.go")
	assert.Contains(t, output, "Implemented feature X")
	assert.Contains(t, output, "worker-id-123")
	assert.Contains(t, output, "worker-id-456")
	assert.Contains(t, output, "TASK-00")
	assert.Contains(t, output, "TASK-02")
	assert.Contains(t, output, "Investigation complete")
	assert.NotContains(t, output, "Stale note")
}

func TestRenderReady_RequiresConfirmation(t *testing.T) {
	t.Parallel()
	entries := []ready.ReadyEntry{
		{
			Issue:                "TASK-01",
			Type:                 "task",
			Title:                "Inferred Task",
			Priority:             "medium",
			RequiresConfirmation: true,
		},
	}
	var buf bytes.Buffer
	err := RenderReady(&buf, entries, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "TASK-01")
	assert.Contains(t, output, "requires confirmation")
}

func TestRenderValidation_AllFields(t *testing.T) {
	t.Parallel()
	result := validate.Result{
		OK:       false,
		Errors:   []string{"error 1", "error 2"},
		Warnings: []string{"warning 1"},
		Infos:    []string{"info 1"},
	}
	var buf bytes.Buffer
	err := RenderValidation(&buf, result, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "ERROR: error 1")
	assert.Contains(t, output, "ERROR: error 2")
	assert.Contains(t, output, "WARNING: warning 1")
	assert.Contains(t, output, "INFO: info 1")
}

func TestRenderValidation_AcceptedRiskCoverage(t *testing.T) {
	t.Parallel()
	result := validate.Result{
		OK: true,
		Coverage: &traceability.Coverage{
			TotalNodes:        4,
			CitedNodes:        3,
			AcceptedRiskNodes: 1,
		},
	}

	var buf bytes.Buffer
	err := RenderValidation(&buf, result, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "COVERAGE: 4/4 cited")
	assert.Contains(t, output, "accepted-risk")
}

func TestCoverageLine_FormatsBothVariants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", CoverageLine(validate.Result{}))
	assert.Equal(t, "COVERAGE: 3/4 cited", CoverageLine(validate.Result{
		Coverage: &traceability.Coverage{TotalNodes: 4, CitedNodes: 3},
	}))
	assert.Equal(t, "COVERAGE: 4/4 cited (3 source-linked, 1 accepted-risk)", CoverageLine(validate.Result{
		Coverage: &traceability.Coverage{TotalNodes: 4, CitedNodes: 3, AcceptedRiskNodes: 1},
	}))
}

func TestRenderIssue_MinimalIssue(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:     "EPIC-01",
		Type:   "epic",
		Status: "open",
		Title:  "Major Initiative",
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "EPIC-01")
	assert.Contains(t, output, "Major Initiative")
	assert.Contains(t, output, "epic")
	assert.Contains(t, output, "open")
	// Should not contain empty optional fields
	assert.NotContains(t, output, "Parent:")
	assert.NotContains(t, output, "Priority:")
}

func TestRenderList_WithMultipleStatuses(t *testing.T) {
	t.Parallel()
	entries := []ListEntry{
		{Issue: "T1", Title: "Task 1", Status: "open"},
		{Issue: "T2", Title: "Task 2", Status: "in-progress", AssignedTo: "worker-1"},
		{Issue: "T3", Title: "Task 3", Status: "done"},
	}
	var buf bytes.Buffer
	err := RenderList(&buf, entries)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "T1")
	assert.Contains(t, output, "T2")
	assert.Contains(t, output, "T3")
	assert.Contains(t, output, "assigned to worker-1")
}

func TestRenderReady_Empty(t *testing.T) {
	t.Parallel()
	var entries []ready.ReadyEntry
	var buf bytes.Buffer
	err := RenderReady(&buf, entries, false)
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "No tasks ready.")
}

func TestRenderReady_EmptyJSON(t *testing.T) {
	t.Parallel()
	var entries []ready.ReadyEntry
	var buf bytes.Buffer
	err := RenderReady(&buf, entries, true)
	require.NoError(t, err)
	output := buf.String()

	var result []interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestRenderIssue_JSON_WithAllFields(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:               "TASK-01",
		Type:             "task",
		Status:           "in-progress",
		Title:            "Complex Task",
		Parent:           "STORY-01",
		Priority:         "high",
		DefinitionOfDone: "All tests pass",
		Scope:            []string{"file1.go"},
		Outcome:          "Implementation done",
		ClaimedBy:        "worker-1",
		AssignedWorker:   "worker-2",
		BlockedBy:        []string{"TASK-00"},
		Blocks:           []string{"TASK-02"},
		Acceptance:       json.RawMessage(`{}`),
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, true)
	require.NoError(t, err)
	output := buf.String()

	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err)
	assert.Equal(t, "TASK-01", result["id"])
	assert.Equal(t, "STORY-01", result["parent"])
	assert.Equal(t, "high", result["priority"])
	assert.Equal(t, "All tests pass", result["definition_of_done"])
	assert.Equal(t, "Implementation done", result["outcome"])
}

func TestRenderIssue_JSON_IncludesNotes(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:     "TASK-01",
		Type:   "task",
		Status: "open",
		Title:  "Task with notes",
		Notes: []materialize.Note{
			{Msg: "Active note", Deleted: false},
			{Msg: "Deleted note", Deleted: true},
		},
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, true)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	notes, ok := result["notes"]
	require.True(t, ok, "notes field must be present in JSON output")
	noteSlice, ok := notes.([]interface{})
	require.True(t, ok)
	assert.Len(t, noteSlice, 1, "only non-deleted notes should appear")
	assert.Equal(t, "Active note", noteSlice[0])
}

func TestRenderList_ColumnAlignment(t *testing.T) {
	t.Parallel()
	entries := []ListEntry{
		{Issue: "SHORT", Status: "open", Title: "First"},
		{Issue: "LONGER-ID", Status: "in-progress", Title: "Second"},
	}
	var buf bytes.Buffer
	err := RenderList(&buf, entries)
	require.NoError(t, err)
	lines := buf.String()

	// Each line must use aligned columns: %-12s for ID, %-14s for status.
	// Use TrimSpace-based checks to avoid brittle trailing-space assertions.
	for _, line := range strings.Split(strings.TrimRight(lines, "\n"), "\n") {
		if strings.Contains(line, "SHORT") {
			assert.True(t, strings.HasPrefix(strings.TrimLeft(line, " "), "SHORT"), "ID column must start with SHORT")
			// Verify the status column appears after sufficient padding
			assert.Contains(t, line, "open", "status column must contain open")
		}
		if strings.Contains(line, "LONGER-ID") {
			assert.Contains(t, line, "in-progress", "status column must contain in-progress")
		}
	}
}

func TestRenderBoard_Basic(t *testing.T) {
	t.Parallel()
	entries := []BoardEntry{
		{Issue: "TASK-01", Status: "open", Claimed: "worker-a", Outcome: "Short outcome", Title: "First task"},
		{Issue: "TASK-02", Status: "in-progress", Claimed: "", Outcome: strings.Repeat("x", 35), Title: "Long outcome task"},
	}
	var buf bytes.Buffer
	err := RenderBoard(&buf, entries)
	require.NoError(t, err)
	out := buf.String()

	// Header must be present
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "CLAIMED")
	assert.Contains(t, out, "OUTCOME")
	assert.Contains(t, out, "TITLE")

	// Data rows
	assert.Contains(t, out, "TASK-01")
	assert.Contains(t, out, "worker-a")
	assert.Contains(t, out, "Short outcome")

	// Long outcome must be truncated to 30 chars (27 + "...")
	assert.Contains(t, out, "...", "outcome longer than 30 chars must be truncated")
	assert.NotContains(t, out, strings.Repeat("x", 35), "full 35-char outcome must not appear verbatim")
}

func TestRenderBoard_Empty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := RenderBoard(&buf, []BoardEntry{})
	require.NoError(t, err)
	assert.Empty(t, buf.String(), "empty entries should produce no output")
}

func TestRenderIssue_WithAssessmentAttestations_Human(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:     "TASK-01",
		Type:   "task",
		Status: "done",
		Title:  "Task with Review",
		AssessmentAttestations: []review.AssessmentAttestation{
			{
				BundleID: "sha256:0123456789abcdef",
				Rating:   review.Yellow,
				HeadSHA:  "abc1234567890def",
			},
			{
				BundleID:           "sha256:fedcba9876543210",
				Rating:             review.Yellow,
				HeadSHA:            "def4567890123abc",
				SatisfiedCount:     1,
				IndeterminateCount: 1,
			},
		},
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, false)
	require.NoError(t, err)
	output := buf.String()

	// Should contain the issue basics
	assert.Contains(t, output, "TASK-01")
	assert.Contains(t, output, "Task with Review")

	// Should contain Review line with latest attestation (the second one)
	// Format: Review:    yellow (bundle sha256:fedcb...; 1 satisfied, 1 indeterminate)
	assert.Contains(t, output, "Review:")
	assert.Contains(t, output, "yellow")
	assert.Contains(t, output, "fedcb")
	assert.Contains(t, output, "1 satisfied")
	assert.Contains(t, output, "1 indeterminate")
}

func TestRenderIssue_WithAssessmentAttestations_JSON(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:     "TASK-01",
		Type:   "task",
		Status: "done",
		Title:  "Task with Review",
		AssessmentAttestations: []review.AssessmentAttestation{
			{
				BundleID:       "sha256:abc123def456",
				Rating:         review.Green,
				HeadSHA:        "abc1234567890def",
				SatisfiedCount: 2,
			},
		},
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, true)
	require.NoError(t, err)
	output := buf.String()

	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err)

	// Should have assessment_attestations field in JSON
	attestations, ok := result["assessment_attestations"]
	require.True(t, ok, "assessment_attestations field must be present in JSON output")
	attestationSlice, ok := attestations.([]interface{})
	require.True(t, ok)
	assert.Len(t, attestationSlice, 1)
}

func TestRenderIssue_NoAssessmentAttestations_Human(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:                     "TASK-01",
		Type:                   "task",
		Status:                 "open",
		Title:                  "Task without Review",
		AssessmentAttestations: []review.AssessmentAttestation{},
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, false)
	require.NoError(t, err)
	output := buf.String()

	// Should not contain Review line when no attestations
	assert.NotContains(t, output, "Review:")
}

func TestRenderIssue_LatestAttestationOnly(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		ID:     "TASK-01",
		Type:   "task",
		Status: "done",
		Title:  "Task with Multiple Reviews",
		AssessmentAttestations: []review.AssessmentAttestation{
			{
				BundleID:          "sha256:aaaaaabbbbbbccccccdddddd",
				Rating:            review.Red,
				SatisfiedCount:    0,
				NotSatisfiedCount: 1,
			},
			{
				BundleID:       "sha256:eeeeeeffffffffgggggghhhhh",
				Rating:         review.Green,
				SatisfiedCount: 2,
			},
		},
	}
	var buf bytes.Buffer
	err := RenderIssue(&buf, issue, false)
	require.NoError(t, err)
	output := buf.String()

	// Should only show the latest (second) attestation
	assert.Contains(t, output, "Review:")
	assert.Contains(t, output, "green")
	// The bundle ID should display the first 12 hex chars (sha256: prefix stripped).
	// "sha256:eeeeeeffffffffgggggghhhhh" → strip prefix → "eeeeeeffffffffgggggghhhhh" → first 12 → "eeeeeeffffff"
	assert.Contains(t, output, "eeeeeeffffff")
	// Should not show the first attestation's hash digits
	assert.NotContains(t, output, "aaaaaabb")
}
