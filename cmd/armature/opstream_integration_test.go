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

func TestMaterializeCommand_ExcludesCrossWorkerOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Valid Task", "--type", "task", "--id", "valid-01")
	require.NoError(t, err)

	opsDir := filepath.Join(repo, ".armature", "ops")
	workerALogPath := filepath.Join(opsDir, "worker-a.log")

	if _, err := os.Stat(workerALogPath); err == nil {
		data, err := os.ReadFile(workerALogPath)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "valid-01", "valid-01 should not be in worker-a.log (should be in a UUID-named file)")
	}

	logPath := workerALogPath

	missingOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "invalid-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-b",
		Payload: ops.Payload{
			Title:    "Invalid Task",
			NodeType: "task",
		},
	}
	require.NoError(t, ops.AppendOp(logPath, missingOp))

	matOut, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	assert.Contains(t, matOut, "Materialized", "materialize should complete successfully")

	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err, "should be able to load index.json after materialize")

	assert.Len(t, index, 1, "should have exactly 1 task after exclude")
	assert.Contains(t, index, "valid-01", "valid-01 should be materialized")
	assert.NotContains(t, index, "invalid-01", "invalid-01 should be excluded due to worker ID mismatch")
}

func TestValidateCommand_ExcludesCrossWorkerOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Good Task", "--type", "task", "--id", "good-01",
		"--scope", "cmd/armature/good.go", "--dod", "The fixture task is complete and tested",
		"--acceptance", `[{"type":"test_passes"}]`)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "sources", "accept-citation", "--issue", "good-01",
		"--rationale", "test fixture has no external source", "--ci")
	require.NoError(t, err)

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

	validateOut, err := runTrls(t, repo, "validate", "--format", "json")
	require.Error(t, err, "default-strict validate must fail when mismatched ops were excluded: %s", validateOut)
	decoded := decodeContractEnvelope(t, validateOut, "findings")
	var findings []validateFindingRow
	require.NoError(t, json.Unmarshal(decoded["findings"], &findings))
	foundSnapshot := false
	for _, f := range findings {
		if f.Rule == snapshotFindingRule && strings.Contains(f.Message, "mismatch") {
			foundSnapshot = true
			break
		}
	}
	assert.True(t, foundSnapshot, "validate envelope must include the snapshot mismatch warning")

	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err, "should be able to load index.json after validate")

	assert.Len(t, index, 1, "should have exactly 1 task after validation")
	assert.Contains(t, index, "good-01", "good-01 should be in validated state")
	assert.NotContains(t, index, "bad-01", "bad-01 should be excluded due to worker ID mismatch")
}

func TestReadyCommand_ExcludesCrossWorkerOps(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Ready Task", "--type", "task", "--id", "ready-01",
		"--scope", "cmd/armature/ready.go", "--dod", "Ready task is complete and tested",
		"--acceptance", `[{"type":"test_passes"}]`)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "dag", "transition", "--issue", "ready-01", "--to", "verified")
	require.NoError(t, err)

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

	readyOut, err := runTrls(t, repo, "ready", "--format", "json")
	require.NoError(t, err)

	decoded := decodeReadyEnvelope(t, readyOut)
	var entries []readyIssueRow
	require.NoError(t, json.Unmarshal(decoded["issues"], &entries))

	readyIDs := make(map[string]bool)
	for _, entry := range entries {
		readyIDs[entry.ID] = true
	}

	assert.Len(t, readyIDs, 1, "should have exactly 1 ready task")
	assert.Contains(t, readyIDs, "ready-01", "ready-01 should be in ready queue")
	assert.NotContains(t, readyIDs, "excluded-01", "excluded-01 should not be in ready queue (cross-worker op)")
}

func TestMaterializeCommand_WarningsVisible(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Good Task", "--type", "task", "--id", "good-01")
	require.NoError(t, err)

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

	out, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	assert.Contains(t, out, "Materialized 1 issue", "materialize should complete successfully")

	index, err := materialize.LoadIndex(filepath.Join(getTestStateDir(t, repo), "index.json"))
	require.NoError(t, err)

	assert.NotContains(t, index, "mismatch-01", "mismatched op should be excluded")
	assert.NotContains(t, index, "mismatch-02", "mismatched op should be excluded")
}

func TestReadyCommand_UnknownOpWarningPrintedOnce(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	plantVerifiedTask(t, repo, "ready-01", "cmd/armature/ready.go")

	opsDir := filepath.Join(repo, ".armature", "ops")
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

	assert.Contains(t, stdout, "ready-01")
	assert.Contains(t, cmdStderr, "warning:", "command stderr should contain the warning")
	assert.Empty(t, string(rawStderr), "raw stderr should stay quiet for snapshot-backed warnings")
}

func TestMaterializeOffsetTracking(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

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

	out1, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	assert.Contains(t, out1, "Materialized 0 issues", "first run should have 0 issues")

	checkpointPath := filepath.Join(getTestStateDir(t, repo), "checkpoint.json")
	_, err = os.Stat(checkpointPath)
	require.NoError(t, err, "checkpoint.json should exist after first materialize")

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

	out2, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	assert.Contains(t, out2, "Materialized 0 issues", "second run should also have 0 issues (new op is also mismatched)")

	loaded, matErr := ops.LoadFromDirValidated(opsDir)
	require.NoError(t, matErr)
	offsets, warnings := loaded.PhysicalEOF, loaded.Warnings

	assert.Contains(t, offsets, "all-mismatch.log", "offset should be recorded for all-mismatch.log")

	info, err := os.Stat(logPath)
	require.NoError(t, err)
	finalFileSize := info.Size()
	recordedOffset := offsets["all-mismatch.log"]
	assert.Greater(t, recordedOffset, int64(0), "offset should be greater than 0")
	assert.LessOrEqual(t, recordedOffset, finalFileSize, "offset should not exceed file size")

	assert.Len(t, warnings, 3, "should have 3 warnings (one for each mismatched op)")
}
