package materialize

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func materializeAndReturn(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: true})
}

func materializeQuiet(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true})
}

func materializeExcludeWorker(allOps []ops.Op, excludeWorkerID string) (*State, Result, error) {
	return Run("", allOps, nil, Options{ExcludeWorkerID: excludeWorkerID, EmitWarnings: true})
}

func TestToTraceabilityRefs_CarriesConfidence_REQ_CITEGATE_T2(t *testing.T) {
	t.Parallel()
	issues := map[string]*Issue{
		"task-draft":    {ID: "task-draft", Provenance: Provenance{Confidence: "draft"}},
		"task-verified": {ID: "task-verified", Provenance: Provenance{Confidence: "verified"}},
	}

	got := make(map[string]string, len(issues))
	for _, ref := range toTraceabilityRefs(issues) {
		got[ref.ID] = ref.Confidence
	}

	assert.Equal(t, "draft", got["task-draft"])
	assert.Equal(t, "verified", got["task-verified"])
}

func TestMaterialize_IncrementalReplayNormalizesLoadedIssues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".armature", "state", "worker-1")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	legacyIssue := []byte(`{
		"id": "task-01",
		"type": "task",
		"status": "open",
		"title": "Fix auth",
		"scope": ["src/auth/**", ""],
		"context_files": ["docs/design.md", ""]
	}`)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "task-01.json"), legacyIssue, 0644))
	require.NoError(t, WriteCheckpoint(filepath.Join(stateDir, "checkpoint.json"), Checkpoint{
		ByteOffsets: map[string]int64{"worker-1.log": 123},
	}))

	_, _, err := materializeAndReturn(stateDir, nil, nil)
	require.NoError(t, err)

	loaded, err := LoadIssue(filepath.Join(issuesDir, "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, []string{"src/auth/**"}, loaded.Scope)
	assert.Equal(t, []string{"docs/design.md"}, loaded.ContextFiles)
}

func TestMaterialize_MkdirAllErrorPropagated(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root; permission restrictions do not apply")
	}
	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(readOnlyDir, 0555))
	t.Cleanup(func() {
		if err := os.Chmod(readOnlyDir, 0755); err != nil {
			t.Fatal(err)
		}
	})

	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	stateDir := filepath.Join(readOnlyDir, "state")

	_, _, err := materializeAndReturn(stateDir, []ops.Op{}, nil)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails, got nil")
	}
}

func TestMaterializeAndReturn_MkdirAllErrorPropagated(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("running as root; permission restrictions do not apply")
	}
	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(readOnlyDir, 0555))
	t.Cleanup(func() {
		if err := os.Chmod(readOnlyDir, 0755); err != nil {
			t.Fatal(err)
		}
	})

	issuesDir := filepath.Join(dir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	stateDir := filepath.Join(readOnlyDir, "state")

	_, _, err := materializeAndReturn(stateDir, []ops.Op{}, nil)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails, got nil")
	}
}

func TestMaterialize_SlottedLogsIncluded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "state", "issues"), 0755))
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	workerID := "worker-x"

	plainLog := filepath.Join(opsDir, workerID+".log")
	require.NoError(t, ops.AppendOp(plainLog, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: workerID,
		Payload: ops.Payload{Title: "My task", NodeType: "task"},
	}))

	slottedLog := filepath.Join(opsDir, workerID+"~slot-a.log")
	require.NoError(t, ops.AppendOp(slottedLog, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200, WorkerID: workerID,
		Payload: ops.Payload{TTL: 60},
	}))
	require.NoError(t, ops.AppendOp(slottedLog, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300, WorkerID: workerID,
		Payload: ops.Payload{To: "done", Outcome: "finished"},
	}))

	allOps, err := ops.ReadLog(plainLog)
	require.NoError(t, err)
	slottedOps, err := ops.ReadLog(slottedLog)
	require.NoError(t, err)
	allOps = append(allOps, slottedOps...)

	_, result, err := materializeAndReturn(filepath.Join(dir, "state"), allOps, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount)
	assert.Equal(t, 3, result.OpsProcessed)
}

func TestMaterializeExcludeWorker_AlsoExcludesSlottedLogs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	workerA := "worker-a"
	workerB := "worker-b"

	logA := filepath.Join(opsDir, workerA+".log")
	require.NoError(t, ops.AppendOp(logA, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: workerA,
		Payload: ops.Payload{Title: "Task one", NodeType: "task"},
	}))
	logASlot := filepath.Join(opsDir, workerA+"~s1.log")
	require.NoError(t, ops.AppendOp(logASlot, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: 200, WorkerID: workerA,
		Payload: ops.Payload{To: "done"},
	}))

	logB := filepath.Join(opsDir, workerB+".log")
	require.NoError(t, ops.AppendOp(logB, ops.Op{
		Type: ops.OpCreate, TargetID: "task-02", Timestamp: 300, WorkerID: workerB,
		Payload: ops.Payload{Title: "Task two", NodeType: "task"},
	}))

	opsA, err := ops.ReadLog(logA)
	require.NoError(t, err)
	opsASlot, err := ops.ReadLog(logASlot)
	require.NoError(t, err)
	opsB, err := ops.ReadLog(logB)
	require.NoError(t, err)
	allOps := append(append(opsA, opsASlot...), opsB...)

	state, result, err := materializeExcludeWorker(allOps, workerA)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount, "only worker-b's issue should be present")
	_, hasTaskOne := state.Issues["task-01"]
	assert.False(t, hasTaskOne, "task-01 created by excluded worker must not appear")
}

func TestMaterializeExcludeWorker_ToleratesMissingTargetsFromExcludedCreates(t *testing.T) {
	t.Parallel()

	workerA := "worker-a"
	workerB := "worker-b"

	validCreate := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-02",
		Timestamp: 100,
		WorkerID:  workerB,
		Payload:   ops.Payload{Title: "Task two", NodeType: "task"},
	}
	missingTargetClaim := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: 200,
		WorkerID:  workerB,
		Payload:   ops.Payload{TTL: 60},
	}
	excludedCreate := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 300,
		WorkerID:  workerA,
		Payload:   ops.Payload{Title: "Task one", NodeType: "task"},
	}

	allOps := []ops.Op{validCreate, missingTargetClaim, excludedCreate}

	state, result, err := materializeExcludeWorker(allOps, workerA)
	require.NoError(t, err, "exclude-worker replay should tolerate missing targets from filtered creates")
	assert.Equal(t, 1, result.IssueCount)
	assert.Equal(t, 2, result.OpsProcessed)
	_, hasTaskTwo := state.Issues["task-02"]
	assert.True(t, hasTaskTwo, "task-02 should still be materialized")
	_, hasTaskOne := state.Issues["task-01"]
	assert.False(t, hasTaskOne, "excluded task-01 should not be materialized")
}

func TestMaterializeExcludeWorker_DoesNotSuppressUnrelatedMissingTargets(t *testing.T) {
	t.Parallel()

	workerA := "worker-a"
	workerB := "worker-b"

	validCreate := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-02",
		Timestamp: 100,
		WorkerID:  workerB,
		Payload:   ops.Payload{Title: "Task two", NodeType: "task"},
	}
	missingTargetClaim := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-01",
		Timestamp: 200,
		WorkerID:  workerB,
		Payload:   ops.Payload{TTL: 60},
	}
	unrelatedExcludedCreate := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-99",
		Timestamp: 300,
		WorkerID:  workerA,
		Payload:   ops.Payload{Title: "Task ninety-nine", NodeType: "task"},
	}

	_, _, err := materializeExcludeWorker([]ops.Op{validCreate, missingTargetClaim, unrelatedExcludedCreate}, workerA)
	require.Error(t, err, "unrelated missing-target replay errors should still surface")
	assert.Contains(t, err.Error(), "task-01")
}

func TestMaterialize_UnknownOpTypeErrorSurfaced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "unknown_future_op_type",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
		Payload:   ops.Payload{},
	}

	allOps := []ops.Op{validOp, unknownOp}

	_, result, err := materializeAndReturn(stateDir, allOps, nil)
	require.NoError(t, err, "Materialize should not error, but should capture unknown ops")

	assert.Greater(t, len(result.UnhandledOps), 0, "unknown op type should be captured in UnhandledOps")
	assert.Equal(t, 1, len(result.UnhandledOps), "should have exactly one unhandled op")
	assert.Equal(t, "unknown_future_op_type", result.UnhandledOps[0].Type, "unhandled op should be the unknown type")
}

func TestMaterializeAndReturn_UnknownOpTypeErrorSurfaced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "another_unknown_type",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
	}

	allOps := []ops.Op{validOp, unknownOp}

	_, result, err := materializeAndReturn(stateDir, allOps, nil)
	require.NoError(t, err, "MaterializeAndReturn should not error, but should capture unknown ops")

	assert.Greater(t, len(result.UnhandledOps), 0, "unknown op type error should be captured in UnhandledOps")
	assert.Equal(t, 1, len(result.UnhandledOps), "should have exactly one unhandled op")
	assert.Equal(t, "another_unknown_type", result.UnhandledOps[0].Type, "unhandled op should be the unknown type")
}

func TestMaterializeAndReturn_HandlerErrorSurfaced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	handlerErrOp := ops.Op{
		Type:      ops.OpClaim,
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
		Payload:   ops.Payload{TTL: 60},
	}

	_, _, err := materializeAndReturn(stateDir, []ops.Op{validOp, handlerErrOp}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claim: issue task-02 not found")

	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	_, statErr := os.Stat(checkpointPath)
	assert.True(t, os.IsNotExist(statErr), "checkpoint should not be written after a handler error")
}

func TestMaterializeExcludeWorker_UnknownOpTypeErrorSurfaced(t *testing.T) {
	t.Parallel()
	workerA := "worker-a"
	workerB := "worker-b"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerA,
		Payload:   ops.Payload{Title: "Task one", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "yet_another_unknown",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerB,
	}

	allOps := []ops.Op{validOp, unknownOp}

	_, result, err := materializeExcludeWorker(allOps, "worker-c")
	require.NoError(t, err, "MaterializeExcludeWorker should not error, but should capture unknown ops")

	assert.Greater(t, len(result.UnhandledOps), 0, "unknown op type error should be captured in UnhandledOps")
}

func TestMaterialize_UnhandledOpsWarningEmitted(t *testing.T) { //nolint:paralleltest
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	unknownOp1 := ops.Op{
		Type:      "unknown_type_1",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
	}

	unknownOp2 := ops.Op{
		Type:      "unknown_type_2",
		TargetID:  "task-03",
		Timestamp: 300,
		WorkerID:  workerID,
	}

	allOps := []ops.Op{validOp, unknownOp1, unknownOp2}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	_, result, materializeErr := materializeAndReturn(stateDir, allOps, nil)

	require.NoError(t, w.Close())
	os.Stderr = oldStderr
	stderrOutput, err := io.ReadAll(r)
	require.NoError(t, err)

	require.NoError(t, materializeErr, "Materialize should not error")
	assert.Equal(t, 2, len(result.UnhandledOps), "should have two unhandled ops")

	stderrStr := string(stderrOutput)
	assert.Contains(t, stderrStr, "warning:", "stderr should contain warning prefix")
	assert.Contains(t, stderrStr, "2", "stderr should mention count of unhandled ops")
	assert.Contains(t, stderrStr, "op(s) with unknown types skipped", "stderr should describe the issue")
}

func TestMaterializeAndReturn_UnhandledOpsWarningEmitted(t *testing.T) { //nolint:paralleltest
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "unknown_op_type",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
	}

	allOps := []ops.Op{validOp, unknownOp}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	_, result, funcErr := materializeAndReturn(stateDir, allOps, nil)

	os.Stderr = oldStderr
	require.NoError(t, w.Close())

	stderrOutput, readErr := io.ReadAll(r)
	require.NoError(t, readErr)

	require.NoError(t, funcErr, "MaterializeAndReturn should not error")
	assert.Equal(t, 1, len(result.UnhandledOps), "should have one unhandled op")

	stderrStr := string(stderrOutput)
	assert.Contains(t, stderrStr, "warning:", "stderr should contain warning prefix")
	assert.Contains(t, stderrStr, "op(s) with unknown types skipped", "stderr should describe the issue")
}

func TestMaterializeExcludeWorker_UnhandledOpsWarningEmitted(t *testing.T) { //nolint:paralleltest
	workerA := "worker-a"
	workerB := "worker-b"

	validOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerA,
		Payload:   ops.Payload{Title: "Task one", NodeType: "task"},
	}

	unknownOp := ops.Op{
		Type:      "unknown_type_3",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerB,
	}

	allOps := []ops.Op{validOp, unknownOp}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	_, result, funcErr := materializeExcludeWorker(allOps, "worker-c")

	os.Stderr = oldStderr
	require.NoError(t, w.Close())
	stderrOutput, readErr := io.ReadAll(r)
	require.NoError(t, readErr)

	require.NoError(t, funcErr, "MaterializeExcludeWorker should not error")
	assert.Greater(t, len(result.UnhandledOps), 0, "should have at least one unhandled op")

	stderrStr := string(stderrOutput)
	assert.Contains(t, stderrStr, "warning:", "stderr should contain warning prefix")
	assert.Contains(t, stderrStr, "op(s) with unknown types skipped", "stderr should describe the issue")
}

func TestIncremental_MatchesFullReplay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	workerID := "worker-x"
	logPath := filepath.Join(opsDir, workerID+".log")

	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: workerID,
		Payload: ops.Payload{Title: "Task one", NodeType: "task"},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-02", Timestamp: 200, WorkerID: workerID,
		Payload: ops.Payload{Title: "Task two", NodeType: "task"},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpNote, TargetID: "task-01", Timestamp: 210, WorkerID: workerID,
		Payload: ops.Payload{Msg: "first note"},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpDecision, TargetID: "task-01", Timestamp: 220, WorkerID: workerID,
		Payload: ops.Payload{Topic: "delivery", Choice: "ship", Rationale: "ready", Affects: []string{"task-01"}},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpSourceLink, TargetID: "task-01", Timestamp: 230, WorkerID: workerID,
		Payload: ops.Payload{SourceID: "src-1", SourceURL: "https://example.com/spec", Title: "Spec"},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCitationAccepted, TargetID: "task-01", Timestamp: 240, WorkerID: workerID,
		Payload: ops.Payload{SourceEntryID: "src-1", ConfirmedNoninteractively: true},
	}))

	opsInitial, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	offsets := map[string]int64{filepath.Base(logPath): info.Size()}
	baselineState, baselineResult, err := materializeAndReturn(stateDir, opsInitial, offsets)
	require.NoError(t, err)
	assert.Equal(t, 2, len(baselineState.Issues))
	assert.Equal(t, 6, baselineResult.OpsProcessed)
	assert.True(t, baselineResult.FullReplay, "baseline should be a full replay")

	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	cp, err := LoadCheckpoint(checkpointPath)
	require.NoError(t, err)
	assert.Greater(t, len(cp.ByteOffsets), 0, "checkpoint should have saved byte offsets")

	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 300, WorkerID: workerID,
		Payload: ops.Payload{TTL: 60},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: 400, WorkerID: workerID,
		Payload: ops.Payload{To: "done", Outcome: "completed"},
	}))

	opsAll, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	info2, err := os.Stat(logPath)
	require.NoError(t, err)
	offsets2 := map[string]int64{filepath.Base(logPath): info2.Size()}
	incrementalState, incrementalResult, err := materializeAndReturn(stateDir, opsAll, offsets2)
	require.NoError(t, err)
	assert.Equal(t, 2, len(incrementalState.Issues))
	assert.Equal(t, 8, incrementalResult.OpsProcessed, "should have processed all 8 ops")
	assert.False(t, incrementalResult.FullReplay, "incremental replay should set FullReplay=false")

	dir2 := t.TempDir()
	opsDir2 := filepath.Join(dir2, "ops")
	stateDir2 := filepath.Join(dir2, "state")
	require.NoError(t, os.MkdirAll(opsDir2, 0755))

	logPath2 := filepath.Join(opsDir2, workerID+".log")
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: workerID,
		Payload: ops.Payload{Title: "Task one", NodeType: "task"},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpCreate, TargetID: "task-02", Timestamp: 200, WorkerID: workerID,
		Payload: ops.Payload{Title: "Task two", NodeType: "task"},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpNote, TargetID: "task-01", Timestamp: 210, WorkerID: workerID,
		Payload: ops.Payload{Msg: "first note"},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpDecision, TargetID: "task-01", Timestamp: 220, WorkerID: workerID,
		Payload: ops.Payload{Topic: "delivery", Choice: "ship", Rationale: "ready", Affects: []string{"task-01"}},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpSourceLink, TargetID: "task-01", Timestamp: 230, WorkerID: workerID,
		Payload: ops.Payload{SourceID: "src-1", SourceURL: "https://example.com/spec", Title: "Spec"},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpCitationAccepted, TargetID: "task-01", Timestamp: 240, WorkerID: workerID,
		Payload: ops.Payload{SourceEntryID: "src-1", ConfirmedNoninteractively: true},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: 300, WorkerID: workerID,
		Payload: ops.Payload{TTL: 60},
	}))
	require.NoError(t, ops.AppendOp(logPath2, ops.Op{
		Type: ops.OpTransition, TargetID: "task-01", Timestamp: 400, WorkerID: workerID,
		Payload: ops.Payload{To: "done", Outcome: "completed"},
	}))

	opsAll2, err := ops.ReadLog(logPath2)
	require.NoError(t, err)
	fullReplayState, fullReplayResult, err := materializeAndReturn(stateDir2, opsAll2, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, len(fullReplayState.Issues))
	assert.Equal(t, 8, fullReplayResult.OpsProcessed)
	assert.True(t, fullReplayResult.FullReplay)

	assert.Equal(t, len(fullReplayState.Issues), len(incrementalState.Issues), "issue count must match")
	for issueID, fullIssue := range fullReplayState.Issues {
		incrementalIssue, ok := incrementalState.Issues[issueID]
		assert.True(t, ok, "issue %s must exist in incremental state", issueID)
		assert.Equal(t, fullIssue.ID, incrementalIssue.ID, "issue ID must match")
		assert.Equal(t, fullIssue.Title, incrementalIssue.Title, "title must match")
		assert.Equal(t, fullIssue.Status, incrementalIssue.Status, "status must match: %v vs %v", fullIssue.Status, incrementalIssue.Status)
		assert.Equal(t, fullIssue.ClaimedBy, incrementalIssue.ClaimedBy, "claimed_by must match")
		assert.Equal(t, fullIssue.Outcome, incrementalIssue.Outcome, "outcome must match")
		assert.Equal(t, fullIssue.Notes, incrementalIssue.Notes, "notes must match")
		assert.Equal(t, fullIssue.Decisions, incrementalIssue.Decisions, "decisions must match")
		assert.Equal(t, fullIssue.SourceLinks, incrementalIssue.SourceLinks, "source links must match")
		assert.Equal(t, fullIssue.CitationAcceptances, incrementalIssue.CitationAcceptances, "citation acceptances must match")
	}
}

func TestIncremental_ReplayRepairsStaleIssueState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	staleIssue := Issue{
		ID:           "task-01",
		Type:         "task",
		Status:       "open",
		Title:        "Repair stale state",
		Scope:        []string{"", "src/task.go", ""},
		ContextFiles: []string{"", "docs/spec.md", ""},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}
	require.NoError(t, WriteIssue(issuesDir, staleIssue))

	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	require.NoError(t, WriteCheckpoint(checkpointPath, Checkpoint{
		ByteOffsets: map[string]int64{"worker-a.log": 128},
	}))

	state, result, err := materializeAndReturn(stateDir, nil, map[string]int64{"worker-a.log": 128})
	require.NoError(t, err)
	assert.False(t, result.FullReplay, "checkpointed replay should take the incremental path")

	loadedIssue, err := LoadIssue(filepath.Join(issuesDir, "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, []string{"src/task.go"}, loadedIssue.Scope)
	assert.Equal(t, []string{"docs/spec.md"}, loadedIssue.ContextFiles)

	currentIssue, ok := state.Issues["task-01"]
	require.True(t, ok)
	assert.Equal(t, []string{"src/task.go"}, currentIssue.Scope)
	assert.Equal(t, []string{"docs/spec.md"}, currentIssue.ContextFiles)

	rawIssuePath := filepath.Join(issuesDir, "task-01.json")
	rawData, err := os.ReadFile(rawIssuePath)
	require.NoError(t, err)
	var rawIssue Issue
	require.NoError(t, json.Unmarshal(rawData, &rawIssue))
	assert.Equal(t, []string{"src/task.go"}, rawIssue.Scope)
	assert.Equal(t, []string{"docs/spec.md"}, rawIssue.ContextFiles)
}

func TestMaterializeAndReturnQuiet_BasicRoundTrip(t *testing.T) {
	t.Parallel()
	stateDir := t.TempDir()

	state, result, err := materializeQuiet(stateDir, []ops.Op{}, nil)
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, 0, result.OpsProcessed)
}

func TestRun_WriteStateFilesControlsDiskWrites_REQ_ARCHIMP_S13(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	workerID := "worker-x"

	createOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}

	allOps := []ops.Op{createOp}

	state, result, err := Run(stateDir, allOps, nil, Options{WriteStateFiles: false})
	require.NoError(t, err, "Run should succeed with WriteStateFiles=false")

	assert.Equal(t, 1, result.IssueCount, "should have one issue in memory")
	assert.Equal(t, 1, result.OpsProcessed)
	assert.NotNil(t, state)
	_, hasTask := state.Issues["task-01"]
	assert.True(t, hasTask, "issue should be materialized in state")

	indexPath := filepath.Join(stateDir, "index.json")
	_, err = os.Stat(indexPath)
	assert.True(t, os.IsNotExist(err), "index.json should not exist when WriteStateFiles=false")

	issuesDir := filepath.Join(stateDir, "issues")
	_, err = os.Stat(issuesDir)
	assert.True(t, os.IsNotExist(err), "issues directory should not exist when WriteStateFiles=false")

	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	_, err = os.Stat(checkpointPath)
	assert.True(t, os.IsNotExist(err), "checkpoint.json should not exist when WriteStateFiles=false")

	traceabilityPath := filepath.Join(stateDir, "traceability.json")
	_, err = os.Stat(traceabilityPath)
	assert.True(t, os.IsNotExist(err), "traceability.json should not exist when WriteStateFiles=false")

	readyPath := filepath.Join(stateDir, "ready.json")
	_, err = os.Stat(readyPath)
	assert.True(t, os.IsNotExist(err), "ready.json should not exist when WriteStateFiles=false")

	require.NoError(t, os.MkdirAll(stateDir, 0755))
	state2, result2, err := Run(stateDir, allOps, nil, Options{WriteStateFiles: true})
	require.NoError(t, err, "Run should succeed with WriteStateFiles=true")

	_, err = os.Stat(indexPath)
	assert.NoError(t, err, "index.json should exist when WriteStateFiles=true")

	_, err = os.Stat(checkpointPath)
	assert.NoError(t, err, "checkpoint.json should exist when WriteStateFiles=true")

	_, err = os.Stat(traceabilityPath)
	assert.NoError(t, err, "traceability.json should exist when WriteStateFiles=true")

	_, err = os.Stat(readyPath)
	assert.NoError(t, err, "ready.json should exist when WriteStateFiles=true")

	assert.Equal(t, result.IssueCount, result2.IssueCount)
	assert.Equal(t, result.OpsProcessed, result2.OpsProcessed)
	_, hasTask2 := state2.Issues["task-01"]
	assert.True(t, hasTask2)
}

func TestRun_ExcludeWorkerFiltersOpsAndSkipsWrites_REQ_ARCHIMP_S13(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	workerA := "worker-a"
	workerB := "worker-b"

	opFromA := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerA,
		Payload:   ops.Payload{Title: "Task from A", NodeType: "task"},
	}

	opFromB := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerB,
		Payload:   ops.Payload{Title: "Task from B", NodeType: "task"},
	}

	allOps := []ops.Op{opFromA, opFromB}

	state, result, err := Run(stateDir, allOps, nil, Options{ExcludeWorkerID: workerA})
	require.NoError(t, err, "Run should succeed with ExcludeWorkerID set")

	assert.Equal(t, 1, result.IssueCount, "should have one issue (only from worker-b)")
	assert.Equal(t, 1, result.OpsProcessed, "should have processed only one op")
	_, hasTaskOne := state.Issues["task-01"]
	assert.False(t, hasTaskOne, "task-01 from excluded worker-a should not exist")
	_, hasTaskTwo := state.Issues["task-02"]
	assert.True(t, hasTaskTwo, "task-02 from worker-b should exist")

	indexPath := filepath.Join(stateDir, "index.json")
	_, err = os.Stat(indexPath)
	assert.True(t, os.IsNotExist(err), "index.json should not exist in diagnostic mode")

	checkpointPath := filepath.Join(stateDir, "checkpoint.json")
	_, err = os.Stat(checkpointPath)
	assert.True(t, os.IsNotExist(err), "checkpoint.json should not exist in diagnostic mode")

	traceabilityPath := filepath.Join(stateDir, "traceability.json")
	_, err = os.Stat(traceabilityPath)
	assert.True(t, os.IsNotExist(err), "traceability.json should not exist in diagnostic mode")

	readyPath := filepath.Join(stateDir, "ready.json")
	_, err = os.Stat(readyPath)
	assert.True(t, os.IsNotExist(err), "ready.json should not exist in diagnostic mode")

	require.NoError(t, os.MkdirAll(stateDir, 0755))
	state2, result2, err := Run(stateDir, allOps, nil, Options{WriteStateFiles: true})
	require.NoError(t, err, "normal Run should succeed")

	_, err = os.Stat(indexPath)
	assert.NoError(t, err, "index.json should exist in normal mode")

	_, err = os.Stat(traceabilityPath)
	assert.NoError(t, err, "traceability.json should exist in normal mode")

	_, err = os.Stat(readyPath)
	assert.NoError(t, err, "ready.json should exist in normal mode")

	assert.Equal(t, 2, result2.IssueCount, "should have both issues in normal mode")
	_, hasTaskOne2 := state2.Issues["task-01"]
	assert.True(t, hasTaskOne2, "task-01 should exist in normal mode")
	_, hasTaskTwo2 := state2.Issues["task-02"]
	assert.True(t, hasTaskTwo2, "task-02 should exist in normal mode")
}

func TestRun_EmitWarningsFalse_SuppressesStderr_REQ_ARCHIMP_S13(t *testing.T) { //nolint:paralleltest
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	workerID := "worker-x"

	createOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "My task", NodeType: "task"},
	}
	unknownOp := ops.Op{
		Type:      "unknown_emit_test_type",
		TargetID:  "task-02",
		Timestamp: 200,
		WorkerID:  workerID,
	}

	allOps := []ops.Op{createOp, unknownOp}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	_, result, runErr := Run(stateDir, allOps, nil, Options{WriteStateFiles: true, EmitWarnings: false})

	os.Stderr = oldStderr
	require.NoError(t, w.Close())
	stderrOutput, readErr := io.ReadAll(r)
	require.NoError(t, readErr)

	require.NoError(t, runErr, "Run should not error")
	assert.Equal(t, 1, len(result.UnhandledOps), "unhandled op should be captured in Result")
	assert.Equal(t, "unknown_emit_test_type", result.UnhandledOps[0].Type)

	assert.Empty(t, string(stderrOutput), "stderr should be empty when EmitWarnings=false")
}

func TestMaterialize_AssessmentAttestedOp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	workerID := "worker-x"

	createOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "task-01",
		Timestamp: 100,
		WorkerID:  workerID,
		Payload:   ops.Payload{Title: "Test task", NodeType: "task"},
	}

	att := review.AssessmentAttestation{
		SchemaVersion:           1,
		BundleID:                "bundle-test-01",
		ContractFingerprint:     "cf-abc123",
		DeliveryFingerprint:     "df-def456",
		BaseSHA:                 "base-sha-123",
		HeadSHA:                 "head-sha-456",
		SkillVersion:            "1.0.0",
		ModelIdentity:           "claude-opus",
		Rating:                  review.Green,
		ResultFingerprint:       "rf-ghi789",
		SatisfiedCount:          2,
		PartiallySatisfiedCount: 0,
		NotSatisfiedCount:       0,
		IndeterminateCount:      0,
	}
	assessmentJSON, err := json.Marshal(att)
	require.NoError(t, err)

	assessmentOp := ops.Op{
		Type:      ops.OpAssessmentAttested,
		TargetID:  "task-01",
		Timestamp: 200,
		WorkerID:  workerID,
		Payload:   ops.Payload{Assessment: assessmentJSON},
	}

	allOps := []ops.Op{createOp, assessmentOp}

	state, result, err := materializeAndReturn(stateDir, allOps, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, result.IssueCount)
	assert.Equal(t, 2, result.OpsProcessed)

	issue, ok := state.Issues["task-01"]
	require.True(t, ok, "issue task-01 should exist")
	require.NotNil(t, issue.AssessmentAttestations)
	require.Len(t, issue.AssessmentAttestations, 1)

	attestation := issue.AssessmentAttestations[0]
	assert.Equal(t, "bundle-test-01", attestation.BundleID)
	assert.Equal(t, "cf-abc123", attestation.ContractFingerprint)
	assert.Equal(t, "df-def456", attestation.DeliveryFingerprint)
	assert.Equal(t, "base-sha-123", attestation.BaseSHA)
	assert.Equal(t, "head-sha-456", attestation.HeadSHA)
	assert.Equal(t, "1.0.0", attestation.SkillVersion)
	assert.Equal(t, "claude-opus", attestation.ModelIdentity)
	assert.Equal(t, review.Green, attestation.Rating)
	assert.Equal(t, "rf-ghi789", attestation.ResultFingerprint)
	assert.Equal(t, 2, attestation.SatisfiedCount)
	assert.Equal(t, 0, attestation.PartiallySatisfiedCount)
	assert.Equal(t, 0, attestation.NotSatisfiedCount)
	assert.Equal(t, 0, attestation.IndeterminateCount)
}

func TestIncremental_RetractsCachedPromotionBeforeReplay_REQ_TOPTIER_B1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logPath := filepath.Join(opsDir, "w1.log")

	seed := []ops.Op{
		{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100, WorkerID: "w1",
			Payload: ops.Payload{Title: "Story", NodeType: "story"}},
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101, WorkerID: "w1",
			Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}},
		{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 102, WorkerID: "w1",
			Payload: ops.Payload{To: ops.StatusMerged}},
	}
	for _, op := range seed {
		require.NoError(t, ops.AppendOp(logPath, op))
	}

	seedOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	seedState, _, err := materializeAndReturn(stateDir, seedOps,
		map[string]int64{filepath.Base(logPath): info.Size()})
	require.NoError(t, err)
	require.Equal(t, ops.StatusMerged, seedState.Issues["story-01"].Status,
		"precondition: story promoted by rollup and cached as merged")

	for _, op := range []ops.Op{
		{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 103, WorkerID: "w1",
			Payload: ops.Payload{To: ops.StatusOpen}},
		{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 104, WorkerID: "w1",
			Payload: ops.Payload{TTL: 60}},
	} {
		require.NoError(t, ops.AppendOp(logPath, op))
	}

	allOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	info2, err := os.Stat(logPath)
	require.NoError(t, err)
	incremental, incResult, err := materializeAndReturn(stateDir, allOps,
		map[string]int64{filepath.Base(logPath): info2.Size()})
	require.NoError(t, err)
	require.False(t, incResult.FullReplay, "precondition: this run must be incremental")

	coldState, _, err := materializeAndReturn(filepath.Join(dir, "state2"), allOps, nil)
	require.NoError(t, err)
	require.Equal(t, ops.StatusInProgress, coldState.Issues["story-01"].Status,
		"precondition: a cold replay promotes the story when its child is reclaimed")

	assert.Equal(t, coldState.Issues["story-01"].Status, incremental.Issues["story-01"].Status,
		"incremental materialization must agree with a cold replay of the same log")
}

func TestMaterialize_PreVersionCheckpointForcesFullReplay_REQ_TOPTIER_B1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	issuesStateDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesStateDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(issuesStateDir, "story-01.json"), []byte(
		`{"id":"story-01","type":"story","status":"merged","title":"Story","children":["task-01"]}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(issuesStateDir, "task-01.json"), []byte(
		`{"id":"task-01","type":"task","status":"merged","title":"Task A","parent":"story-01"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "checkpoint.json"), []byte(
		`{"last_materialized_commit":"","byte_offsets":{"w1.log":128}}`), 0644))

	allOps := []ops.Op{
		{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100, WorkerID: "w1",
			Payload: ops.Payload{Title: "Story", NodeType: "story"}},
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101, WorkerID: "w1",
			Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}},
		{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 102, WorkerID: "w1",
			Payload: ops.Payload{To: ops.StatusMerged}},
		{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 103, WorkerID: "w1",
			Payload: ops.Payload{To: ops.StatusOpen}},
	}
	state, result, err := materializeAndReturn(stateDir, allOps, map[string]int64{"w1.log": 256})
	require.NoError(t, err)

	assert.True(t, result.FullReplay, "a pre-version checkpoint must force a cold replay")
	assert.Equal(t, ops.StatusOpen, state.Issues["story-01"].Status,
		"the stale derived promotion must not survive the upgrade")

	cp, err := LoadCheckpoint(filepath.Join(stateDir, "checkpoint.json"))
	require.NoError(t, err)
	assert.Equal(t, CurrentStateVersion, cp.StateVersion,
		"the rewritten checkpoint must carry the current state version")
}

func TestMaterialize_NewerCheckpointForcesFullReplay_REQ_TOPTIER_B1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	issuesStateDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesStateDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(issuesStateDir, "story-01.json"), []byte(
		`{"id":"story-01","type":"story","status":"merged","title":"Story","children":["task-01"]}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(issuesStateDir, "task-01.json"), []byte(
		`{"id":"task-01","type":"task","status":"merged","title":"Task A","parent":"story-01"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "checkpoint.json"), []byte(
		fmt.Sprintf(`{"last_materialized_commit":"","state_version":%d,"byte_offsets":{"w1.log":128}}`,
			CurrentStateVersion+1)), 0644))

	allOps := []ops.Op{
		{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100, WorkerID: "w1",
			Payload: ops.Payload{Title: "Story", NodeType: "story"}},
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101, WorkerID: "w1",
			Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}},
		{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 102, WorkerID: "w1",
			Payload: ops.Payload{To: ops.StatusMerged}},
		{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 103, WorkerID: "w1",
			Payload: ops.Payload{To: ops.StatusOpen}},
	}
	state, result, err := materializeAndReturn(stateDir, allOps, map[string]int64{"w1.log": 256})
	require.NoError(t, err)

	assert.True(t, result.FullReplay, "a checkpoint from a newer state version must force a cold replay")
	assert.Equal(t, ops.StatusOpen, state.Issues["story-01"].Status,
		"snapshots this build cannot fully decode must not be trusted")
}

func TestMaterialize_FullReplayPurgesOrphanedSnapshots_REQ_TOPTIER_B1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	issuesStateDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesStateDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(issuesStateDir, "task-01.json"), []byte(
		`{"id":"task-01","type":"task","status":"open","title":"Task A"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(issuesStateDir, "task-99.json"), []byte(
		`{"id":"task-99","type":"task","status":"open","title":"Ghost"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "checkpoint.json"), []byte(
		fmt.Sprintf(`{"last_materialized_commit":"","state_version":%d,"byte_offsets":{"w1.log":128}}`,
			CurrentStateVersion+1)), 0644))

	allOps := []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
			Payload: ops.Payload{Title: "Task A", NodeType: "task"}},
	}
	state, result, err := materializeAndReturn(stateDir, allOps, map[string]int64{"w1.log": 256})
	require.NoError(t, err)
	require.True(t, result.FullReplay, "precondition: the version mismatch forces a cold replay")
	require.NotContains(t, state.Issues, "task-99", "precondition: the replay does not yield the orphan")

	assert.NoFileExists(t, filepath.Join(issuesStateDir, "task-99.json"),
		"a snapshot the replay did not produce must not survive on disk")

	next, nextResult, err := materializeAndReturn(stateDir, allOps, map[string]int64{"w1.log": 256})
	require.NoError(t, err)
	require.False(t, nextResult.FullReplay, "precondition: the rewritten checkpoint enables incremental replay")
	assert.NotContains(t, next.Issues, "task-99",
		"incremental replay must not load an issue the log does not produce")
}
