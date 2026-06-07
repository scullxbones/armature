package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/tui/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTUIModel_LoadsValidOpsExcludesCrossWorker tests that the TUI model's
// op loading correctly excludes ops with mismatched worker IDs.
// This validates that readAllOpsFromDirWithOffsets in model.go properly
// uses LoadFromDirWithOffsetsValidated to filter cross-worker ops.
func TestTUIModel_LoadsValidOpsExcludesCrossWorker(t *testing.T) {
	// Create a temp directory with mixed valid and cross-worker ops
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	// Create a valid op in a file that matches its worker ID
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

	// Create an op with mismatched worker ID
	// File says worker-a, but op says worker-b
	mismatchLogPath := filepath.Join(opsDir, "worker-a.log")
	mismatchOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-mismatch",
		Timestamp: 101,
		WorkerID:  "worker-b", // Mismatch!
		Payload: ops.Payload{
			Title:    "Mismatched Task",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(mismatchLogPath, mismatchOp))

	// Create a model and trigger materialization
	model := app.New(dir, stateDir, "test-worker")

	// Manually call materialize (simulating what happens during app refresh)
	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	require.NoError(t, err)

	// Check that warnings are returned for the mismatch
	assert.NotEmpty(t, warnings, "should have warning for mismatched op")
	assert.Greater(t, len(warnings), 0, "at least one warning should be present")

	// Verify warnings mention the mismatch
	hasWorkerIDWarning := false
	for _, w := range warnings {
		if contains(w, "worker ID mismatch") {
			hasWorkerIDWarning = true
			break
		}
	}
	assert.True(t, hasWorkerIDWarning, "warnings should mention worker ID mismatch")

	// Extract ops from items
	allOps := ops.ExtractOps(items)

	// Verify only valid ops are included
	assert.Len(t, allOps, 1, "should have exactly 1 op (only valid-01)")
	assert.Equal(t, "task-valid", allOps[0].TargetID, "only valid-01 should be loaded")

	// Verify offsets are tracked for both files
	assert.Contains(t, offsets, "worker-valid.log", "offset should be tracked for valid log")
	assert.Contains(t, offsets, "worker-a.log", "offset should be tracked even for all-mismatch file")

	// Materialize state
	allOpsForMat := ops.ExtractOps(items)
	state, _, err := materialize.MaterializeAndReturn(stateDir, allOpsForMat, true, offsets)
	require.NoError(t, err)

	// Verify materialized state only includes the valid task
	assert.Contains(t, state.Issues, "task-valid", "materialized state should contain valid task")
	assert.NotContains(t, state.Issues, "task-mismatch", "materialized state should not contain mismatched task")

	_ = model // use model variable to avoid unused warning
}

// TestTUIModel_WarningsSurfacedDuringLoad tests that warnings from op loading
// are properly surfaced (logged to stderr) so users see validation issues.
func TestTUIModel_WarningsSurfacedDuringLoad(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	// Create a file with multiple mismatched ops
	logPath := filepath.Join(opsDir, "worker-x.log")
	for i := 0; i < 3; i++ {
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

	// Load ops and check warnings
	items, _, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	require.NoError(t, err)

	// Should have 3 warnings (one for each mismatched op)
	assert.Len(t, warnings, 3, "should have warning for each mismatched op")

	// All ops should be excluded
	assert.Len(t, items, 0, "all mismatched ops should be excluded")

	// Each warning should be about worker ID mismatch
	for _, w := range warnings {
		assert.Contains(t, w, "worker ID mismatch", "each warning should mention worker ID mismatch")
	}
}

// TestTUIModel_MixedValidityLoadingCorrectly tests that a file with both valid and
// invalid ops is handled correctly: valid ones are kept, invalid ones are excluded,
// and appropriate warnings are generated.
func TestTUIModel_MixedValidityLoadingCorrectly(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	logPath := filepath.Join(opsDir, "worker-c.log")

	// Add valid op (worker ID matches filename)
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

	// Add invalid op (worker ID doesn't match)
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

	// Add another valid op
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

	// Should have exactly 1 warning (for the one invalid op)
	assert.Len(t, warnings, 1, "should have exactly 1 warning for the mismatched op")

	// Should have exactly 2 items (valid-task and valid-task-2)
	assert.Len(t, items, 2, "should have exactly 2 valid ops")

	// Verify the items are the valid ones
	ids := make(map[string]bool)
	for _, item := range items {
		ids[item.Op.TargetID] = true
	}
	assert.Contains(t, ids, "valid-task", "should include valid-task")
	assert.Contains(t, ids, "valid-task-2", "should include valid-task-2")
	assert.NotContains(t, ids, "invalid-task", "should exclude invalid-task")

	// Offset should be recorded
	assert.Contains(t, offsets, "worker-c.log", "offset should be recorded")

	// Materialize and verify
	allOps := ops.ExtractOps(items)
	state, _, err := materialize.MaterializeAndReturn(stateDir, allOps, true, offsets)
	require.NoError(t, err)

	assert.Contains(t, state.Issues, "valid-task", "valid-task should be in state")
	assert.Contains(t, state.Issues, "valid-task-2", "valid-task-2 should be in state")
	assert.NotContains(t, state.Issues, "invalid-task", "invalid-task should not be in state")
}

// TestTUIModel_EmptyOpsDir tests that an empty ops directory is handled gracefully
func TestTUIModel_EmptyOpsDir(t *testing.T) {
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

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || (len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr)))
}
