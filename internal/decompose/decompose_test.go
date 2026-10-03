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

	created, err := ApplyPlan(plan, dir, workerID, state, ApplyOptions{}, clock.System)
	require.NoError(t, err)
	assert.Len(t, created, 2)

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

	_, err := ApplyPlan(plan, dir, workerID, state, ApplyOptions{}, clock.System)
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

	_, err := ApplyPlan(plan, dir, workerID, state, ApplyOptions{}, clock.System)
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

	created, err := ApplyPlan(plan, dir, workerID, state, ApplyOptions{}, clock.System)
	require.NoError(t, err)
	assert.Len(t, created, 1)
}

func TestCheckForeignChildren_RefusesAttachedChild(t *testing.T) {
	t.Parallel()
	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues:  []PlanIssue{{ID: "PLAN-001", Title: "Parent", Type: "story"}},
	}
	state := materialize.NewState()
	state.Issues["PLAN-001"] = &materialize.Issue{ID: "PLAN-001", Status: "open", Children: []string{"FOREIGN-1"}}
	state.Issues["FOREIGN-1"] = &materialize.Issue{ID: "FOREIGN-1", Status: "open", Parent: "PLAN-001"}

	err := CheckForeignChildren(plan, state)
	require.Error(t, err)
	var foreign *ForeignChildError
	require.ErrorAs(t, err, &foreign)
	assert.Equal(t, "FOREIGN-1", foreign.Child)
	assert.Equal(t, "PLAN-001", foreign.Parent)
	assert.Contains(t, err.Error(), "FOREIGN-1")
	assert.Contains(t, err.Error(), "PLAN-001")
}

func TestCancelOps_RefusesForeignChild(t *testing.T) {
	t.Parallel()
	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues:  []PlanIssue{{ID: "PLAN-001", Title: "Parent", Type: "story"}},
	}
	state := materialize.NewState()
	state.Issues["PLAN-001"] = &materialize.Issue{ID: "PLAN-001", Status: "open", Children: []string{"FOREIGN-1"}}
	state.Issues["FOREIGN-1"] = &materialize.Issue{ID: "FOREIGN-1", Status: "open", Parent: "PLAN-001"}

	proposed, err := CancelOps(plan, "worker-test", state, clock.System)
	require.Error(t, err)
	assert.Nil(t, proposed)
	assert.Contains(t, err.Error(), "FOREIGN-1")
	assert.Contains(t, err.Error(), "PLAN-001")
}

func TestDryRunRevertPlan_RefusesForeignChild(t *testing.T) {
	t.Parallel()
	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues:  []PlanIssue{{ID: "PLAN-001", Title: "Parent", Type: "story"}},
	}
	state := materialize.NewState()
	state.Issues["PLAN-001"] = &materialize.Issue{ID: "PLAN-001", Status: "open", Children: []string{"FOREIGN-1"}}
	state.Issues["FOREIGN-1"] = &materialize.Issue{ID: "FOREIGN-1", Status: "open", Parent: "PLAN-001"}

	result, err := DryRunRevertPlan(plan, state)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "FOREIGN-1")
	assert.Contains(t, err.Error(), "PLAN-001")
}

func TestCancelOps_AllowsChildrenInPlan(t *testing.T) {
	t.Parallel()
	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "Parent", Type: "story"},
			{ID: "PLAN-002", Title: "Child", Type: "task"},
		},
	}
	state := materialize.NewState()
	state.Issues["PLAN-001"] = &materialize.Issue{ID: "PLAN-001", Status: "open", Children: []string{"PLAN-002"}}
	state.Issues["PLAN-002"] = &materialize.Issue{ID: "PLAN-002", Status: "open", Parent: "PLAN-001"}

	proposed, err := CancelOps(plan, "worker-test", state, clock.System)
	require.NoError(t, err)
	require.Len(t, proposed, 2)
}

func TestCancelOps_CancelsOpen(t *testing.T) {
	t.Parallel()
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

	proposed, err := CancelOps(plan, workerID, state, clock.System)
	require.NoError(t, err)
	assert.Len(t, proposed, 2)
}

func TestCancelOps_SkipsNonOpen(t *testing.T) {
	t.Parallel()
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

	proposed, err := CancelOps(plan, workerID, state, clock.System)
	require.NoError(t, err)
	assert.Len(t, proposed, 1)
	assert.Equal(t, "PLAN-001", proposed[0].TargetID)
}

func TestCancelOps_InjectsClockTimestamp(t *testing.T) {
	t.Parallel()
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
	fixedClock := func() int64 { return fixedTimestamp }

	proposed, err := CancelOps(plan, workerID, state, fixedClock)
	require.NoError(t, err)
	require.Len(t, proposed, 1)
	assert.Equal(t, fixedTimestamp, proposed[0].Timestamp,
		"injected clock timestamp should appear in written op")
}

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

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
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

	created, err := ApplyPlan(plan, dir, workerID, state, ApplyOptions{}, clock.System)
	require.NoError(t, err)
	assert.Len(t, created, 1)

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

	created, err := ApplyPlan(plan, dir, workerID, state, ApplyOptions{}, clock.System)
	require.Error(t, err, "Introduction must refuse a task create that introduces E6")
	assert.Empty(t, created)
	assert.Contains(t, err.Error(), "missing required field")
}
