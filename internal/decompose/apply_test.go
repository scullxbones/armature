package decompose

import (
	"path/filepath"
	"testing"

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
