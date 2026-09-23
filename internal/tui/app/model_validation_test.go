package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromDirWithOffsetsValidated_ExcludesCrossWorkerOps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	validLogPath := filepath.Join(opsDir, "worker-valid.log")
	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-valid",
		Timestamp: 100,
		WorkerID:  "worker-valid",
		Payload: ops.Payload{
			Title:    "Valid Task",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(validLogPath, validOp))

	mismatchLogPath := filepath.Join(opsDir, "worker-a.log")
	mismatchOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-mismatch",
		Timestamp: 101,
		WorkerID:  "worker-b",
		Payload: ops.Payload{
			Title:    "Mismatched Task",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(mismatchLogPath, mismatchOp))

	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	require.NoError(t, err)

	assert.NotEmpty(t, warnings, "should have warning for mismatched op")

	hasWorkerIDWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "worker ID mismatch") {
			hasWorkerIDWarning = true
			break
		}
	}
	assert.True(t, hasWorkerIDWarning, "warnings should mention worker ID mismatch")

	allOps := ops.ExtractOps(items)

	assert.Len(t, allOps, 1, "should have exactly 1 op (only valid-01)")
	assert.Equal(t, "task-valid", allOps[0].TargetID, "only valid-01 should be loaded")

	assert.Contains(t, offsets, "worker-valid.log", "offset should be tracked for valid log")
	assert.Contains(t, offsets, "worker-a.log", "offset should be tracked even for all-mismatch file")

	allOpsForMat := ops.ExtractOps(items)
	state, _, err := materialize.Run(stateDir, allOpsForMat, offsets, materialize.Options{WriteStateFiles: true})
	require.NoError(t, err)

	assert.Contains(t, state.Issues, "task-valid", "materialized state should contain valid task")
	assert.NotContains(t, state.Issues, "task-mismatch", "materialized state should not contain mismatched task")
}

func TestLoadFromDirWithOffsetsValidated_ReturnsWarningsForMismatches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")

	require.NoError(t, os.MkdirAll(opsDir, 0755))

	logPath := filepath.Join(opsDir, "worker-x.log")
	for i := range 3 {
		op := ops.Op{
			Type:      ops.OpCreate,
			TargetID:  fmt.Sprintf("task-%d", i),
			Timestamp: int64(100 + i),
			WorkerID:  "wrong-worker",
			Payload: ops.Payload{
				Title:    fmt.Sprintf("Task %d", i),
				NodeType: "task",
			},
		}
		require.NoError(t, ops.AppendOp(logPath, op))
	}

	items, _, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	require.NoError(t, err)

	assert.Len(t, warnings, 3, "should have warning for each mismatched op")

	assert.Len(t, items, 0, "all mismatched ops should be excluded")

	for _, w := range warnings {
		assert.Contains(t, w, "worker ID mismatch", "each warning should mention worker ID mismatch")
	}
}

func TestTUIModel_MixedValidityLoadingCorrectly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	logPath := filepath.Join(opsDir, "worker-c.log")

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "valid-task",
		Timestamp: 100,
		WorkerID:  "worker-c",
		Payload: ops.Payload{
			Title:    "Valid",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, validOp))

	invalidOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "invalid-task",
		Timestamp: 101,
		WorkerID:  "worker-d",
		Payload: ops.Payload{
			Title:    "Invalid",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, invalidOp))

	validOp2 := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "valid-task-2",
		Timestamp: 102,
		WorkerID:  "worker-c",
		Payload: ops.Payload{
			Title:    "Valid 2",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, validOp2))

	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	require.NoError(t, err)

	assert.Len(t, warnings, 1, "should have exactly 1 warning for the mismatched op")

	assert.Len(t, items, 2, "should have exactly 2 valid ops")

	ids := make(map[string]bool)
	for _, item := range items {
		ids[item.Op.TargetID] = true
	}
	assert.Contains(t, ids, "valid-task", "should include valid-task")
	assert.Contains(t, ids, "valid-task-2", "should include valid-task-2")
	assert.NotContains(t, ids, "invalid-task", "should exclude invalid-task")

	assert.Contains(t, offsets, "worker-c.log", "offset should be recorded")

	allOps := ops.ExtractOps(items)
	state, _, err := materialize.Run(stateDir, allOps, offsets, materialize.Options{WriteStateFiles: true})
	require.NoError(t, err)

	assert.Contains(t, state.Issues, "valid-task", "valid-task should be in state")
	assert.Contains(t, state.Issues, "valid-task-2", "valid-task-2 should be in state")
	assert.NotContains(t, state.Issues, "invalid-task", "invalid-task should not be in state")
}

func TestTUIModel_EmptyOpsDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	require.NoError(t, err)

	assert.Empty(t, items, "empty ops dir should return no items")
	assert.Empty(t, warnings, "empty ops dir should return no warnings")
	assert.Empty(t, offsets, "empty ops dir should return no offsets")
}
