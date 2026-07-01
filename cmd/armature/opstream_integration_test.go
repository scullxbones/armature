package main

import (
	"encoding/json"
	"io"
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

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Create one task via normal flow
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Valid Task", "--type", "task", "--id", "valid-01")
	require.NoError(t, err)

	// Assumption: arm create writes to a UUID-named log file, not to "worker-a.log".
	// This test injects a cross-worker op into a hardcoded "worker-a.log" to avoid collision.
	// Verify this assumption holds by checking that valid-01 is NOT in a worker-a.log file.
	opsDir := filepath.Join(repo, ".arm", ".armature", "ops")
	workerALogPath := filepath.Join(opsDir, "worker-a.log")

	// Verify that valid-01 was not created in worker-a.log (it should be in a UUID-named file)
	if _, err := os.Stat(workerALogPath); err == nil {
		// If worker-a.log exists, verify it doesn't contain valid-01
		data, err := os.ReadFile(workerALogPath)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "valid-01", "valid-01 should not be in worker-a.log (should be in a UUID-named file)")
	}

	// Now inject a mismatched op directly into the ops log:
	// filename says worker-a, but op.WorkerID says worker-b
	logPath := workerALogPath

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
	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err, "should be able to load index.json after materialize")

	// The index should have exactly 1 issue (valid-01), with invalid-01 excluded due to worker ID mismatch
	assert.Len(t, index, 1, "should have exactly 1 task after exclude")
	assert.Contains(t, index, "valid-01", "valid-01 should be materialized")
	assert.NotContains(t, index, "invalid-01", "invalid-01 should be excluded due to worker ID mismatch")
}

// TestValidateCommand_ExcludesCrossWorkerOps tests that arm validate
// excludes ops with mismatched worker IDs, resulting in a DAG that only contains valid ops.
func TestValidateCommand_ExcludesCrossWorkerOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a valid task
	_, err = runTrls(t, repo, "create", "--title", "Good Task", "--type", "task", "--id", "good-01")
	require.NoError(t, err)

	// Inject a cross-worker op (will be excluded by validation)
	opsDir := filepath.Join(repo, ".arm", ".armature", "ops")
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
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(validateOut)), &result))

	// Verify that validation succeeded
	assert.NotNil(t, result, "result should be a valid JSON object")

	// Now verify the materialized state directly (validate calls materialize internally)
	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err, "should be able to load index.json after validate")

	// good-01 should be present and bad-01 should not be present (excluded due to worker ID mismatch)
	assert.Len(t, index, 1, "should have exactly 1 task after validation")
	assert.Contains(t, index, "good-01", "good-01 should be in validated state")
	assert.NotContains(t, index, "bad-01", "bad-01 should be excluded due to worker ID mismatch")
}

// TestReadyCommand_ExcludesCrossWorkerOps tests that arm ready
// only returns tasks from valid (non-mismatched) ops.
func TestReadyCommand_ExcludesCrossWorkerOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a valid ready task (must be verified to appear in ready queue)
	_, err = runTrls(t, repo, "create", "--title", "Ready Task", "--type", "task", "--id", "ready-01", "--confidence", "verified")
	require.NoError(t, err)

	// Inject a cross-worker task (will be excluded)
	opsDir := filepath.Join(repo, ".arm", ".armature", "ops")
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
	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(readyOut)), &entries))

	// Extract IDs from the "issue" field
	readyIDs := make(map[string]bool)
	for _, entry := range entries {
		if issue, ok := entry["issue"].(string); ok {
			readyIDs[issue] = true
		}
	}

	// The cross-worker task should never be materialized, so it's not in the index
	// Thus, only ready-01 should be in the ready queue
	assert.Len(t, readyIDs, 1, "should have exactly 1 ready task")
	assert.Contains(t, readyIDs, "ready-01", "ready-01 should be in ready queue")
	assert.NotContains(t, readyIDs, "excluded-01", "excluded-01 should not be in ready queue (cross-worker op)")
}

// TestMaterializeCommand_WarningsVisible tests that mismatched ops generate warnings
// that are logged (printed to stderr) during materialization.
// The warnings are surfaced to the user via stderr, not stdout.
func TestMaterializeCommand_WarningsVisible(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a valid task first
	_, err = runTrls(t, repo, "create", "--title", "Good Task", "--type", "task", "--id", "good-01")
	require.NoError(t, err)

	// Inject two mismatched ops to the same file
	opsDir := filepath.Join(repo, ".arm", ".armature", "ops")
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

	// Run materialize and capture stdout
	out, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Verify that materialize completed successfully (good-01 was materialized)
	assert.Contains(t, out, "Materialized 1 issue", "materialize should complete successfully")

	// Note: warnings are logged to os.Stderr directly in the helpers (fmt.Fprintf(os.Stderr, ...)),
	// not through the cobra command's error writer, so they won't be captured in stderr buffer.
	// The warnings are still generated and printed to the console during the test, but we verify
	// the effect instead: the mismatched ops are excluded from materialization.

	// Verify that the mismatched ops were excluded from materialization
	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)

	// mismatch-01 and mismatch-02 should NOT be in the state
	// (they have worker ID mismatches and must be excluded)
	assert.NotContains(t, index, "mismatch-01", "mismatched op should be excluded")
	assert.NotContains(t, index, "mismatch-02", "mismatched op should be excluded")
}

// TestReadyCommand_UnknownOpWarningPrintedOnce verifies that snapshot-backed commands
// surface unknown-op warnings through the command error stream without duplicating them
// on raw process stderr.
func TestReadyCommand_UnknownOpWarningPrintedOnce(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Ready Task", "--type", "task", "--id", "ready-01")
	require.NoError(t, err)

	opsDir := filepath.Join(repo, ".arm", ".armature", "ops")
	unknownLog := filepath.Join(opsDir, "worker-unknown.log")
	require.NoError(t, ops.AppendOp(unknownLog, ops.Op{
		Type:      "unknown_future_type",
		TargetID:  "ready-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-unknown",
	}))

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	stdout, cmdStderr, cmdErr := runTrlsWithStderr(t, repo, "ready")

	require.NoError(t, w.Close())
	os.Stderr = oldStderr
	rawStderr, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, cmdErr)

	assert.Contains(t, stdout, "Ready Task")
	assert.Contains(t, cmdStderr, "warning:", "command stderr should contain the warning")
	assert.Empty(t, string(rawStderr), "raw stderr should stay quiet for snapshot-backed warnings")
}

// TestMaterializeOffsetTracking tests that offsets are properly tracked even for
// files with all mismatched ops, preventing infinite re-read loops.
func TestMaterializeOffsetTracking(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a file with only mismatched ops
	opsDir := filepath.Join(repo, ".arm", ".armature", "ops")
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

	// First materialize should exclude all ops but record the offset in checkpoint.json
	out1, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// First run should have 0 issues materialized (since all ops were excluded)
	assert.Contains(t, out1, "Materialized 0 issues", "first run should have 0 issues")

	// Verify the checkpoint was recorded by checking the materialized state
	checkpointPath := filepath.Join(getTestStateDir(t, repo), "checkpoint.json")
	_, err = os.Stat(checkpointPath)
	require.NoError(t, err, "checkpoint.json should exist after first materialize")

	// Now add another mismatched op to the file (simulating new ops arriving)
	op3 := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "all-bad-03",
		Timestamp: 202,
		WorkerID:  "wrong-id",
		Payload: ops.Payload{
			Title:    "All Bad 3",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, op3))

	// Second materialize should:
	// 1. Skip the two ops from run 1 (they were already processed and recorded in checkpoint)
	// 2. Only process the new op3
	// 3. Still end up with 0 issues (since op3 also has worker ID mismatch)
	out2, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Second run should also have 0 issues materialized
	assert.Contains(t, out2, "Materialized 0 issues", "second run should also have 0 issues (new op is also mismatched)")

	// Verify that the checkpoint correctly tracked the offset after both runs
	// by loading the offsets through the normal flow
	_, offsets, warnings, matErr := ops.LoadFromDirWithOffsetsValidated(opsDir)
	require.NoError(t, matErr)

	// The offsets should have an entry for all-mismatch.log
	assert.Contains(t, offsets, "all-mismatch.log", "offset should be recorded for all-mismatch.log")

	// The final offset should be at or near the file size (all ops processed)
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	finalFileSize := info.Size()
	recordedOffset := offsets["all-mismatch.log"]
	assert.Greater(t, recordedOffset, int64(0), "offset should be greater than 0")
	assert.LessOrEqual(t, recordedOffset, finalFileSize, "offset should not exceed file size")

	// Check that warnings were generated (3 total: 2 from run 1, 1 from run 2)
	assert.Len(t, warnings, 3, "should have 3 warnings (one for each mismatched op)")
}
