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
