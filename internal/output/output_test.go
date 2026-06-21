package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ready"
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
			Type:       "task",
			Priority:   "high",
			Status:     "open",
			AssignedTo: "worker-1",
		},
		{
			Issue:      "TASK-02",
			Title:      "Second Task",
			Type:       "task",
			Priority:   "low",
			Status:     "done",
			AssignedTo: "",
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
	// Verify that ListEntry struct can be created with expected fields
	entry := ListEntry{
		Issue:      "TASK-01",
		Title:      "Test",
		Type:       "task",
		Priority:   "high",
		Status:     "open",
		Parent:     "STORY-01",
		AssignedTo: "worker-1",
		Scope:      []string{"file.go"},
	}
	assert.Equal(t, "TASK-01", entry.Issue)
	assert.Equal(t, "Test", entry.Title)
	assert.Equal(t, "task", entry.Type)
	assert.Equal(t, "high", entry.Priority)
	assert.Equal(t, "open", entry.Status)
	assert.Equal(t, "STORY-01", entry.Parent)
	assert.Equal(t, "worker-1", entry.AssignedTo)
	assert.Len(t, entry.Scope, 1)
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
		{Issue: "T1", Title: "Task 1", Type: "task", Status: "open", Priority: "high"},
		{Issue: "T2", Title: "Task 2", Type: "task", Status: "in-progress", Priority: "low", AssignedTo: "worker-1"},
		{Issue: "T3", Title: "Task 3", Type: "feature", Status: "done", Priority: ""},
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
