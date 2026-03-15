package decompose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Task 25: ParsePlan tests ---

func TestParsePlan_Valid(t *testing.T) {
	plan := Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "First issue", Type: "task"},
			{ID: "PLAN-002", Title: "Second issue", Type: "task"},
		},
	}
	data, err := json.Marshal(plan)
	require.NoError(t, err)

	tmpFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	parsed, err := ParsePlan(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	assert.Len(t, parsed.Issues, 2)
	assert.Equal(t, "PLAN-001", parsed.Issues[0].ID)
	assert.Equal(t, "PLAN-002", parsed.Issues[1].ID)
}

func TestParsePlan_InvalidVersion(t *testing.T) {
	plan := Plan{
		Version: 2,
		Title:   "Bad Plan",
		Issues:  []PlanIssue{},
	}
	data, err := json.Marshal(plan)
	require.NoError(t, err)

	tmpFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	_, err = ParsePlan(tmpFile)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unsupported plan version"), "expected error to contain 'unsupported plan version', got: %s", err.Error())
}

func TestParsePlan_MissingFile(t *testing.T) {
	_, err := ParsePlan("/nonexistent/path/plan.json")
	require.Error(t, err)
}

// --- Task 26: ApplyPlan tests ---

func TestApplyPlan_CreatesOps(t *testing.T) {
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "First issue", Type: "task"},
			{ID: "PLAN-002", Title: "Second issue", Type: "task"},
		},
	}

	state := materialize.NewState()

	count, err := ApplyPlan(plan, dir, workerID, state)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	assert.Len(t, readOps, 2)
}

func TestApplyPlan_SkipsExisting(t *testing.T) {
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "First issue", Type: "task"},
			{ID: "PLAN-002", Title: "Second issue", Type: "task"},
		},
	}

	state := materialize.NewState()
	state.Issues["PLAN-001"] = &materialize.Issue{ID: "PLAN-001", Status: "open"}

	count, err := ApplyPlan(plan, dir, workerID, state)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// --- Task 27: RevertPlan tests ---

func TestRevertPlan_CancelsOpen(t *testing.T) {
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "First issue", Type: "task"},
			{ID: "PLAN-002", Title: "Second issue", Type: "task"},
		},
	}

	state := materialize.NewState()
	state.Issues["PLAN-001"] = &materialize.Issue{ID: "PLAN-001", Status: "open"}
	state.Issues["PLAN-002"] = &materialize.Issue{ID: "PLAN-002", Status: "open"}

	count, err := RevertPlan(plan, dir, workerID, state)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestRevertPlan_SkipsNonOpen(t *testing.T) {
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "First issue", Type: "task"},
			{ID: "PLAN-002", Title: "Second issue", Type: "task"},
		},
	}

	state := materialize.NewState()
	state.Issues["PLAN-001"] = &materialize.Issue{ID: "PLAN-001", Status: "open"}
	state.Issues["PLAN-002"] = &materialize.Issue{ID: "PLAN-002", Status: "done"}

	count, err := RevertPlan(plan, dir, workerID, state)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// --- Task 27: PlanContext tests ---

func TestPlanContext(t *testing.T) {
	plan := &Plan{
		Version: 1,
		Title:   "My Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001"},
			{ID: "PLAN-002"},
			{ID: "PLAN-003"},
		},
	}

	result := PlanContext(plan)
	assert.Contains(t, result, "My Plan")
	assert.Contains(t, result, "3")
}
