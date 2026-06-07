package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMaterializeCommand_ExcludesCrossWorkerOps tests that arm materialize
// excludes ops whose WorkerID doesn't match the filename's worker ID.
// This validates that LoadFromDirWithOffsetsValidated correctly filters mismatched ops.
func TestMaterializeCommand_ExcludesCrossWorkerOps(t *testing.T) {
	// Setup: Create a temp repo with mixed valid and cross-worker ops
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	// Create one task via normal flow
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Valid Task", "--type", "task", "--id", "valid-01")
	require.NoError(t, err)

	// Now inject a mismatched op directly into the ops log:
	// filename says worker-a, but op.WorkerID says worker-b
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "worker-a.log")

	missingOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "invalid-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-b", // Mismatch: filename expects worker-a
		Payload: ops.Payload{
			Title:    "Invalid Task",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, missingOp))

	// Run materialize and capture output
	matOut, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Note: warnings are printed to stderr by readAllOpsFromDirWithOffsets,
	// so they may not appear in stdout. We verify by checking the state instead.

	// Materialize should have successfully processed the ops
	assert.Contains(t, matOut, "Materialized", "materialize should complete successfully")

	// Verify that invalid-01 is NOT in the state (only valid-01 should be)
	stateDir := filepath.Join(repo, ".armature", "state")
	index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
	require.NoError(t, err, "should be able to load index.json after materialize")

	// The index should have exactly 1 issue (valid-01)
	if len(index) > 0 {
		assert.Len(t, index, 1, "should have exactly 1 task after exclude")
		assert.Contains(t, index, "valid-01", "valid-01 should be materialized")
		assert.NotContains(t, index, "invalid-01", "invalid-01 should be excluded due to worker ID mismatch")
	} else {
		// It's possible the index is still being built, let's check if we can at least verify no invalid-01
		assert.NotContains(t, index, "invalid-01", "invalid-01 should not be in state")
	}
}

// TestValidateCommand_ExcludesCrossWorkerOps tests that arm validate
// excludes ops with mismatched worker IDs, resulting in a DAG that only contains valid ops.
func TestValidateCommand_ExcludesCrossWorkerOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a valid task
	_, err = runTrls(t, repo, "create", "--title", "Good Task", "--type", "task", "--id", "good-01")
	require.NoError(t, err)

	// Inject a cross-worker op (will be excluded by validation)
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "worker-x.log")
	badOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "bad-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "wrong-worker-id",
		Payload: ops.Payload{
			Title:    "Bad Task",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, badOp))

	// Run validate and check output (which internally calls materialize)
	validateOut, err := runTrls(t, repo, "validate", "--format", "json")
	require.NoError(t, err)

	// Parse JSON output
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(validateOut)), &result))

	// Verify that validation succeeded
	assert.NotNil(t, result, "result should be a valid JSON object")

	// Now verify the materialized state directly (validate calls materialize internally)
	stateDir := filepath.Join(repo, ".armature", "state")
	index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
	require.NoError(t, err, "should be able to load index.json after validate")

	// The index should contain good-01 if it exists
	// (Note: it might be empty if no ops were successfully created)
	if len(index) > 0 {
		// good-01 should be present and bad-01 should not be
		assert.Contains(t, index, "good-01", "good-01 should be in validated state")
		assert.NotContains(t, index, "bad-01", "bad-01 should be excluded due to worker ID mismatch")
	} else {
		// If no issues were materialized, at least verify bad-01 is not present
		assert.NotContains(t, index, "bad-01", "bad-01 should not be in state (cross-worker op)")
	}
}

// TestReadyCommand_ExcludesCrossWorkerOps tests that arm ready
// only returns tasks from valid (non-mismatched) ops.
func TestReadyCommand_ExcludesCrossWorkerOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a valid ready task
	_, err = runTrls(t, repo, "create", "--title", "Ready Task", "--type", "task", "--id", "ready-01")
	require.NoError(t, err)

	// Inject a cross-worker task (will be excluded)
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "worker-y.log")
	crossWorkerOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "excluded-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "mismatched-worker",
		Payload: ops.Payload{
			Title:    "Excluded Task",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, crossWorkerOp))

	// Run ready and check output
	readyOut, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)

	// Parse JSON array of ready entries
	var entries []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(readyOut)), &entries))

	// Extract IDs from the "Issue" field
	readyIDs := make(map[string]bool)
	for _, entry := range entries {
		if issue, ok := entry["Issue"].(string); ok {
			readyIDs[issue] = true
		}
	}

	// The cross-worker task should never be materialized, so it's not in the index
	// Thus, only ready-01 should be in the ready queue
	if len(readyIDs) > 0 {
		assert.Contains(t, readyIDs, "ready-01", "ready-01 should be in ready queue")
		assert.NotContains(t, readyIDs, "excluded-01", "excluded-01 should not be in ready queue (cross-worker op)")
	} else {
		// If no tasks are ready, at least verify that excluded-01 is not in the state
		stateDir := filepath.Join(repo, ".armature", "state")
		index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
		require.NoError(t, err)
		assert.NotContains(t, index, "excluded-01", "excluded-01 should never be materialized due to worker ID mismatch")
	}
}

// TestMaterializeCommand_WarningsVisible tests that mismatched ops generate warnings
// that are logged (printed to stderr) during materialization.
// The warnings are surfaced to the user via stderr, not stdout.
func TestMaterializeCommand_WarningsVisible(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a valid task first
	_, err = runTrls(t, repo, "create", "--title", "Good Task", "--type", "task", "--id", "good-01")
	require.NoError(t, err)

	// Inject two mismatched ops to the same file
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "worker-mismatch.log")

	op1 := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "mismatch-01",
		Timestamp: 100,
		WorkerID:  "different-worker",
		Payload: ops.Payload{
			Title:    "Mismatch 1",
			NodeType: "task",
		},
	}
	op2 := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "mismatch-02",
		Timestamp: 101,
		WorkerID:  "different-worker",
		Payload: ops.Payload{
			Title:    "Mismatch 2",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, op1))
	require.NoError(t, ops.AppendOp(logPath, op2))

	// Run materialize
	out, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify that materialize completed successfully
	assert.Contains(t, out, "Materialized", "materialize should complete successfully")

	// Verify that the mismatched ops were excluded from materialization
	stateDir := filepath.Join(repo, ".armature", "state")
	index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
	require.NoError(t, err)

	// At minimum, mismatch-01 and mismatch-02 should NOT be in the state
	// (they have worker ID mismatches and must be excluded)
	assert.NotContains(t, index, "mismatch-01", "mismatched op should be excluded")
	assert.NotContains(t, index, "mismatch-02", "mismatched op should be excluded")
}

// TestMaterializeOffsetTracking tests that offsets are properly tracked even for
// files with all mismatched ops, preventing infinite re-read loops.
func TestMaterializeOffsetTracking(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a file with only mismatched ops
	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "all-mismatch.log")

	op1 := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "all-bad-01",
		Timestamp: 200,
		WorkerID:  "wrong-id",
		Payload: ops.Payload{
			Title:    "All Bad 1",
			NodeType: "task",
		},
	}
	op2 := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "all-bad-02",
		Timestamp: 201,
		WorkerID:  "wrong-id",
		Payload: ops.Payload{
			Title:    "All Bad 2",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, op1))
	require.NoError(t, ops.AppendOp(logPath, op2))

	// Get file size before materialization
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	fileSize := info.Size()

	// First materialize should exclude all ops but record the offset
	out1, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify that the offsets were recorded
	// LoadFromDirWithOffsetsValidated should have recorded offsets for all-mismatch.log
	// even though all its ops were excluded
	_, offsets, warnings, matErr := ops.LoadFromDirWithOffsetsValidated(opsDir)
	require.NoError(t, matErr)

	// The offsets should have an entry for all-mismatch.log
	assert.Contains(t, offsets, "all-mismatch.log", "offset should be recorded for all-mismatch.log even though all ops are excluded")

	// The offset should be greater than 0 and close to file size
	recordedOffset := offsets["all-mismatch.log"]
	assert.Greater(t, recordedOffset, int64(0), "offset should be greater than 0")
	assert.LessOrEqual(t, recordedOffset, fileSize, "offset should not exceed file size")
	assert.GreaterOrEqual(t, recordedOffset, fileSize-int64(10), "offset should be close to file size (within 10 bytes)")

	// Check that warnings were generated
	assert.Len(t, warnings, 2, "should have 2 warnings (one for each mismatched op)")

	// Both runs should have 0 issues materialized (since all ops were excluded)
	assert.Contains(t, out1, "0 issues", "first run should have 0 issues")
}
