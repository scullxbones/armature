package decompose

import (
	"errors"
	"os"
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
				ID:         "PLAN-001",
				Title:      "Multi-scope issue",
				Type:       "task",
				Scope:      "internal/foo/bar.go, internal/baz/qux.go",
				DoD:        "Multi-scope issue is complete and tested",
				Acceptance: []byte(`[{"type":"test_passes"}]`),
				Source:     "src-test",
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
	require.GreaterOrEqual(t, len(readOps), 1)
	assert.Equal(t, ops.OpCreate, readOps[0].Type)
	assert.Equal(t, []string{"internal/foo/bar.go", "internal/baz/qux.go"}, readOps[0].Payload.Scope,
		"comma-separated scope should be split into individual entries")
	assert.Equal(t, ops.OpSourceLink, readOps[1].Type)
	assert.Equal(t, "src-test", readOps[1].Payload.SourceID)
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
				ID:         "PLAN-001",
				Title:      "Single-scope issue",
				Type:       "task",
				Scope:      "internal/foo/bar.go",
				DoD:        "Single-scope issue is complete and tested",
				Acceptance: []byte(`[{"type":"test_passes"}]`),
				Source:     "src-test",
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
	require.GreaterOrEqual(t, len(readOps), 1)
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
			taskPlanIssue("PLAN-001", "Issue with injected clock"),
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
	require.GreaterOrEqual(t, len(readOps), 1)
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
			taskPlanIssue("PLAN-001", "Top-level task"),
		},
	}

	state := materialize.NewState()
	state.Issues["EPIC-001"] = &materialize.Issue{ID: "EPIC-001", Type: "epic", Status: ops.StatusOpen, Title: "Root epic"}
	count, err := ApplyPlanWithOptions(plan, dir, workerID, state, ApplyOptions{Root: "EPIC-001"}, clock.Fixed(42))
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	logPath := filepath.Join(dir, workerID+".log")
	readOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(readOps), 1)
	assert.Equal(t, "EPIC-001", readOps[0].Payload.Parent)
}

func TestDryRunApplyPlan_ReturnsWouldCreate(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Version: 1,
		Title:   "Dry Run Plan",
		Issues: []PlanIssue{
			taskPlanIssue("PLAN-001", "New issue"),
			taskPlanIssue("PLAN-002", "Another issue"),
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
			taskPlanIssue("PLAN-001", "New issue"),
			taskPlanIssue("PLAN-EXISTING", "Already exists"),
		},
	}
	state := materialize.NewState()
	state.Issues["PLAN-EXISTING"] = &materialize.Issue{ID: "PLAN-EXISTING"}

	result, err := DryRunApplyPlan(plan, state)
	require.NoError(t, err)
	assert.Len(t, result.WouldCreate, 1)
	assert.Equal(t, "PLAN-001", result.WouldCreate[0].ID)
}

func TestApplyRefusesUncitedPlan_REQ_LNGHZN_S10_T12(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plan := &Plan{
		Version: 1,
		Title:   "Uncited plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "Task without source", Type: "task", DoD: "done"},
		},
	}
	state := materialize.NewState()

	count, err := ApplyPlan(plan, dir, "worker-test", state)
	require.Error(t, err, "plans without per-issue source must fail apply")
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "source")
	assert.Contains(t, err.Error(), "PLAN-001")

	_, err = DryRunApplyPlan(plan, state)
	require.Error(t, err, "dry-run must also refuse an uncited plan")
	assert.Contains(t, err.Error(), "source")
}

func TestApplyPlan_RefusesUnknownSourceWhenManifestProvided(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plan := &Plan{
		Version: 1,
		Title:   "Fabricated source",
		Issues: []PlanIssue{{
			ID:         "PLAN-001",
			Title:      "Cited against a missing source",
			Type:       "task",
			Scope:      "internal/PLAN-001.go",
			DoD:        "Cited against a missing source is complete and tested",
			Acceptance: []byte(`[{"type":"test_passes"}]`),
			Source:     "00000000-0000-0000-0000-000000000001",
		}},
	}
	opts := ApplyOptions{ManifestData: []byte(`{"entries":{"src-real":{"id":"src-real"}}}`)}
	count, err := ApplyPlanWithOptions(plan, dir, "worker-test", materialize.NewState(), opts, clock.System)
	require.Error(t, err, "apply must refuse a source ID that is not in the manifest")
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "00000000-0000-0000-0000-000000000001")
	_, statErr := os.Stat(filepath.Join(dir, "worker-test.log"))
	assert.True(t, os.IsNotExist(statErr), "a refused apply must not land an uncited create")

	_, err = DryRunApplyPlanWithOptions(plan, materialize.NewState(), opts)
	require.Error(t, err, "dry-run must also refuse an unknown source")
	assert.Contains(t, err.Error(), "00000000-0000-0000-0000-000000000001")
}

func TestApplyPlan_WritesCreateAndSourceLinkAtomically(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var batches [][]string
	opts := ApplyOptions{appendOps: func(path string, proposed []ops.Op) error {
		types := make([]string, 0, len(proposed))
		for _, op := range proposed {
			types = append(types, op.Type)
		}
		batches = append(batches, types)
		return ops.AppendOps(path, proposed)
	}}

	plan := &Plan{
		Version: 1,
		Title:   "Atomic plan",
		Issues: []PlanIssue{
			taskPlanIssue("PLAN-001", "First"),
			taskPlanIssue("PLAN-002", "Second"),
		},
	}
	count, err := ApplyPlanWithOptions(plan, dir, "worker-test", materialize.NewState(), opts, clock.System)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	require.Len(t, batches, 1, "create + source_link (+links) must be one write, not one write per op")
	assert.Contains(t, batches[0], ops.OpCreate)
	assert.Contains(t, batches[0], ops.OpSourceLink)
}

func TestApplyPlan_UsesRealAppenderWhenUnset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plan := &Plan{
		Version: 1,
		Title:   "Default appender",
		Issues:  []PlanIssue{taskPlanIssue("PLAN-001", "Only")},
	}
	count, err := ApplyPlanWithOptions(plan, dir, "worker-test", materialize.NewState(), ApplyOptions{}, clock.System)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	data, readErr := os.ReadFile(filepath.Join(dir, "worker-test.log"))
	require.NoError(t, readErr, "an unset appendOps must fall back to ops.AppendOps")
	assert.Contains(t, string(data), ops.OpCreate)
}

func TestApplyPlan_FailedAppendLeavesNoPartialLog(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opts := ApplyOptions{appendOps: func(string, []ops.Op) error {
		return errors.New("disk full")
	}}

	plan := &Plan{
		Version: 1,
		Title:   "Failing plan",
		Issues:  []PlanIssue{taskPlanIssue("PLAN-001", "Only")},
	}
	count, err := ApplyPlanWithOptions(plan, dir, "worker-test", materialize.NewState(), opts, clock.System)
	require.Error(t, err)
	assert.Equal(t, 0, count)
	_, statErr := os.Stat(filepath.Join(dir, "worker-test.log"))
	assert.True(t, os.IsNotExist(statErr), "a failed atomic append must not leave an uncited create")
}

func TestValidatePlan_DoesNotWarnOnInvalidType_REQ_NXTTN_S2_T2(t *testing.T) {
	t.Parallel()

	// Invalid types are now always fatal (see validateTypes /
	// TestApplyPlan_InvalidType_AlwaysFatal below), not advisory warnings.
	plan := &Plan{
		Version: 1,
		Title:   "Plan with invalid type",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "Valid task", Type: "task", DoD: "definition"},
			{ID: "PLAN-002", Title: "Invalid behavior type", Type: "behavior", DoD: "definition"},
		},
	}

	warnings := ValidatePlan(plan)
	assert.Empty(t, warnings, "invalid type should not be reported as an advisory warning")
}

func TestApplyPlan_InvalidType_AlwaysFatal(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Version: 1,
		Title:   "Plan with invalid type",
		Issues: []PlanIssue{
			{ID: "PLAN-002", Title: "Invalid behavior type", Type: "behavior", DoD: "definition", Source: "src-test"},
		},
	}
	state := materialize.NewState()
	dir := t.TempDir()

	// Invalid type is always fatal, regardless of other plan checks.
	count, err := ApplyPlanWithOptions(plan, dir, "worker-1", state, ApplyOptions{}, clock.System)
	require.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "invalid type")
	assert.Contains(t, err.Error(), "PLAN-002")
}

func TestDryRunApplyPlan_InvalidType_AlwaysFatal(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Version: 1,
		Title:   "Plan with invalid type",
		Issues: []PlanIssue{
			{ID: "PLAN-002", Title: "Invalid behavior type", Type: "behavior", DoD: "definition"},
		},
	}
	state := materialize.NewState()

	_, err := DryRunApplyPlanWithOptions(plan, state, ApplyOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}
