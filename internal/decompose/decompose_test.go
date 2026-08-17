package decompose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/clock"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func taskPlanIssue(id, title string) PlanIssue {
	return PlanIssue{
		ID:         id,
		Title:      title,
		Type:       "task",
		Scope:      "internal/" + id + ".go",
		DoD:        title + " is complete and tested",
		Acceptance: json.RawMessage(`[{"type":"test_passes"}]`),
		Source:     "src-test",
	}
}

// --- Task 26: ApplyPlan tests ---

func TestApplyPlan_CreatesOps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			taskPlanIssue("PLAN-001", "First issue"),
			taskPlanIssue("PLAN-002", "Second issue"),
		},
	}

	state := materialize.NewState()

	count, err := ApplyPlan(plan, dir, workerID, state)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	assert.Len(t, readOps, 4)
}

func TestApplyPlan_EmitsDraftConfidence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			taskPlanIssue("PLAN-001", "First issue"),
		},
	}

	state := materialize.NewState()

	_, err := ApplyPlan(plan, dir, workerID, state)
	require.NoError(t, err)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(readOps), 1)
	assert.Equal(t, "draft", readOps[0].Payload.Confidence, "decompose-apply must emit confidence=draft on all created nodes")
}

func TestApplyPlan_PreservesContextFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"

	issue := taskPlanIssue("PLAN-001", "First issue")
	issue.ContextFiles = []string{"docs/adr.md", "docs/design.md"}
	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues:  []PlanIssue{issue},
	}

	state := materialize.NewState()

	_, err := ApplyPlan(plan, dir, workerID, state)
	require.NoError(t, err)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(readOps), 1)
	assert.Equal(t, []string{"docs/adr.md", "docs/design.md"}, readOps[0].Payload.ContextFiles)
}

func TestApplyPlan_SkipsExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			taskPlanIssue("PLAN-001", "First issue"),
			taskPlanIssue("PLAN-002", "Second issue"),
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
	t.Parallel()
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
	t.Parallel()
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

// --- QLTYCNTRL-S2-T3: Clock injection for RevertPlan ---

func TestRevertPlanWithOptions_InjectsClockTimestamp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"
	fixedTimestamp := int64(1234567890)

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "Issue with injected clock", Type: "task"},
		},
	}

	state := materialize.NewState()
	state.Issues["PLAN-001"] = &materialize.Issue{ID: "PLAN-001", Status: "open"}
	fixedClock := clock.Fixed(fixedTimestamp)

	count, err := RevertPlanWithOptions(plan, dir, workerID, state, fixedClock)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.Len(t, readOps, 1)
	assert.Equal(t, fixedTimestamp, readOps[0].Timestamp,
		"injected clock timestamp should appear in written op")
}

// --- E6-S3-T3: DryRunRevertPlan tests ---

func TestDryRunRevertPlan_ReturnsWouldCancel(t *testing.T) {
	t.Parallel()
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

	result, err := DryRunRevertPlan(plan, state)
	require.NoError(t, err)
	assert.Len(t, result.WouldCancel, 2)
	ids := []string{result.WouldCancel[0].ID, result.WouldCancel[1].ID}
	assert.Contains(t, ids, "PLAN-001")
	assert.Contains(t, ids, "PLAN-002")
}

func TestDryRunRevertPlan_SkipsNonOpen(t *testing.T) {
	t.Parallel()
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

	result, err := DryRunRevertPlan(plan, state)
	require.NoError(t, err)
	assert.Len(t, result.WouldCancel, 1)
	assert.Equal(t, "PLAN-001", result.WouldCancel[0].ID)
}

func TestDryRunRevertPlan_DoesNotWriteOps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues:  []PlanIssue{{ID: "PLAN-001", Title: "Issue", Type: "task"}},
	}

	state := materialize.NewState()
	state.Issues["PLAN-001"] = &materialize.Issue{ID: "PLAN-001", Status: "open"}

	_, err := DryRunRevertPlan(plan, state)
	require.NoError(t, err)

	// Verify no files were written
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// --- Task 27: PlanContext tests ---

func TestPlanContext(t *testing.T) {
	t.Parallel()
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

func TestDecomposeContextNoSources(t *testing.T) {
	t.Parallel()
	plan := &Plan{Title: "Plan", Issues: []PlanIssue{}}
	ctx, err := BuildContext(ContextParams{Plan: plan})
	require.NoError(t, err)
	assert.Empty(t, ctx.Sources)
	assert.NotEmpty(t, ctx.PlanSchema)
}

func TestDecomposeContextWithSources(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sourcesDir := filepath.Join(dir, "sources")
	content := []byte("# PRD\n\nProduct requirements.")
	require.NoError(t, sources.WriteCache(sourcesDir, "prd", content))
	m := sources.Manifest{}
	m.Upsert(sources.SourceEntry{ID: "prd", ProviderType: "filesystem"})
	require.NoError(t, sources.WriteManifest(sourcesDir, m))

	plan := &Plan{Title: "My Plan", Issues: []PlanIssue{{ID: "TSK-1", Title: "Task one", Type: "task"}}}
	ctx, err := BuildContext(ContextParams{
		IssuesDir: dir,
		Plan:      plan,
		SourceIDs: []string{"prd"},
		Template:  "Sources: {{SOURCES}}",
	})
	require.NoError(t, err)
	assert.Contains(t, ctx.PromptTemplate, "PRD")
	assert.Len(t, ctx.Sources, 1)
	assert.Equal(t, "prd", ctx.Sources[0].ID)
}

// --- E6-S6-T1: acceptance field tests ---

func TestApplyPlan_ImportsAcceptanceFromPlan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"

	acceptance := json.RawMessage(`[{"type":"test_passes","cmd":"make check"}]`)
	issue := taskPlanIssue("PLAN-001", "First issue")
	issue.Acceptance = acceptance
	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues:  []PlanIssue{issue},
	}

	state := materialize.NewState()

	count, err := ApplyPlan(plan, dir, workerID, state)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(readOps), 1)
	assert.Equal(t, string(acceptance), string(readOps[0].Payload.Acceptance), "acceptance field should be imported from plan")
}

func TestApplyPlan_HandlesEmptyAcceptance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"

	issue := taskPlanIssue("PLAN-001", "First issue")
	issue.Acceptance = nil
	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues:  []PlanIssue{issue},
	}

	state := materialize.NewState()

	count, err := ApplyPlan(plan, dir, workerID, state)
	require.Error(t, err, "Introduction must refuse a task create that introduces E6")
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "missing required field")
}
