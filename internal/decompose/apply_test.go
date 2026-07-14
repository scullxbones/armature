package decompose

import (
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/clock"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- SFT-S1-T13: multi-file scope splitting ---

func TestApplyPlan_SplitsCommaSeparatedScope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{
				ID:    "PLAN-001",
				Title: "Multi-scope issue",
				Type:  "task",
				Scope: "internal/foo/bar.go, internal/baz/qux.go",
			},
		},
	}

	state := materialize.NewState()

	count, err := ApplyPlan(plan, dir, workerID, state)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.Len(t, readOps, 1)
	assert.Equal(t, []string{"internal/foo/bar.go", "internal/baz/qux.go"}, readOps[0].Payload.Scope,
		"comma-separated scope should be split into individual entries")
}

func TestApplyPlan_SingleScopeUnchanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{
				ID:    "PLAN-001",
				Title: "Single-scope issue",
				Type:  "task",
				Scope: "internal/foo/bar.go",
			},
		},
	}

	state := materialize.NewState()

	count, err := ApplyPlan(plan, dir, workerID, state)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.Len(t, readOps, 1)
	assert.Equal(t, []string{"internal/foo/bar.go"}, readOps[0].Payload.Scope,
		"single scope entry should remain as a single-element slice")
}

// --- QLTYCNTRL-S2-T2: Clock injection ---

func TestApplyPlanWithOptions_InjectsClockTimestamp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"
	fixedTimestamp := int64(1234567890)

	plan := &Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{
				ID:    "PLAN-001",
				Title: "Issue with injected clock",
				Type:  "task",
			},
		},
	}

	state := materialize.NewState()
	fixedClock := clock.Fixed(fixedTimestamp)

	count, err := ApplyPlanWithOptions(plan, dir, workerID, state, ApplyOptions{}, fixedClock)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.Len(t, readOps, 1)
	assert.Equal(t, fixedTimestamp, readOps[0].Timestamp,
		"injected clock timestamp should appear in written op")
}

func TestApplyPlanWithOptions_AppliesRootToTopLevelIssues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerID := "worker-test"

	plan := &Plan{
		Version: 1,
		Title:   "Rooted plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "Top-level task", Type: "task"},
		},
	}

	state := materialize.NewState()
	count, err := ApplyPlanWithOptions(plan, dir, workerID, state, ApplyOptions{Root: "EPIC-001"}, clock.Fixed(42))
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.Len(t, readOps, 1)
	assert.Equal(t, "EPIC-001", readOps[0].Payload.Parent)
}

func TestDryRunApplyPlan_ReturnsWouldCreate(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Version: 1,
		Title:   "Dry Run Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "New issue", Type: "task"},
			{ID: "PLAN-002", Title: "Another issue", Type: "task"},
		},
	}
	state := materialize.NewState()

	result, err := DryRunApplyPlan(plan, state)
	require.NoError(t, err)
	assert.Len(t, result.WouldCreate, 2)
}

func TestDryRunApplyPlan_SkipsExisting(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Version: 1,
		Title:   "Plan with existing",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "New issue", Type: "task"},
			{ID: "PLAN-EXISTING", Title: "Already exists", Type: "task"},
		},
	}
	state := materialize.NewState()
	state.Issues["PLAN-EXISTING"] = &materialize.Issue{ID: "PLAN-EXISTING"}

	result, err := DryRunApplyPlan(plan, state)
	require.NoError(t, err)
	assert.Len(t, result.WouldCreate, 1)
	assert.Equal(t, "PLAN-001", result.WouldCreate[0].ID)
}

func TestDryRunApplyPlanWithOptions_StrictRejectsWarnings(t *testing.T) {
	t.Parallel()

	// Create a plan with a duplicate ID to trigger a warning.
	plan := &Plan{
		Version: 1,
		Title:   "Strict plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "Task one", Type: "task"},
			{ID: "PLAN-001", Title: "Duplicate ID", Type: "task"},
		},
	}
	state := materialize.NewState()

	_, err := DryRunApplyPlanWithOptions(plan, state, ApplyOptions{Strict: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "warning")
}

func TestValidatePlan_RejectsInvalidType_REQ_NXTTN_S2_T2(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Version: 1,
		Title:   "Plan with invalid type",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "Valid task", Type: "task", DoD: "definition"},
			{ID: "PLAN-002", Title: "Invalid behavior type", Type: "behavior", DoD: "definition"},
		},
	}

	warnings := ValidatePlan(plan)
	assert.Len(t, warnings, 1, "should have exactly one warning for invalid type")
	assert.Contains(t, warnings[0], "PLAN-002")
	assert.Contains(t, warnings[0], "invalid type")
	assert.Contains(t, warnings[0], "behavior")
}
