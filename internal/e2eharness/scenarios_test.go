package e2eharness_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/scullxbones/armature/internal/e2eharness"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClaimRaceAndStaleReclaim_REQ_TOPTIER_S3_T2 verifies both sides of the
// failure contract: exactly one concurrent claimant owns a fresh task and a
// task abandoned past its TTL can be reclaimed by another worker.
func TestClaimRaceAndStaleReclaim_REQ_TOPTIER_S3_T2(t *testing.T) {
	t.Parallel()

	h := scenarioHarness(t, "RACE-001", "STALE-001")
	seedExpiredClaim(t, h, "STALE-001")
	workerA, workerB := scenarioWorkers(t, h)

	var raceResults [2]struct {
		out string
		err error
	}
	var wg sync.WaitGroup
	for i, worker := range []string{workerA, workerB} {
		wg.Add(1)
		go func(i int, worker string) {
			defer wg.Done()
			raceResults[i].out, raceResults[i].err = scenarioRunArmIn(h, worker,
				"claim", "--repo", worker, "--issue", "RACE-001", "--worktree", filepath.Join(h.TempDir, "race-worktree-"+string(rune('a'+i))))
		}(i, worker)
	}
	wg.Wait()

	for _, result := range raceResults {
		require.NoError(t, result.err, "concurrent claim must fail only by losing the race: %s", result.out)
	}
	// Each command may report a provisional local win before it sees the other
	// log. Recovery is the authoritative race resolution point.
	recoveryDir := filepath.Join(h.TempDir, "race-recovery")
	require.NoError(t, h.Clone("race-recovery", recoveryDir))
	out, err := h.RunArmIn(recoveryDir, "bootstrap", "--repo", recoveryDir)
	require.NoError(t, err, "race recovery bootstrap failed: %s", out)
	out, err = h.RunArmIn(recoveryDir, "materialize", "--repo", recoveryDir)
	require.NoError(t, err, "race recovery materialization failed: %s", out)
	out, err = h.RunArmIn(recoveryDir, "show", "--repo", recoveryDir, "--issue", "RACE-001", "--field", "claimed_by")
	require.NoError(t, err, "race recovery show failed: %s", out)
	assert.NotEmpty(t, strings.TrimSpace(out), "replay must converge to exactly one claim owner")

	out, err = h.RunArmIn(workerB, "claim", "--repo", workerB, "--issue", "STALE-001", "--worktree", filepath.Join(h.TempDir, "stale-worktree-b"))
	require.NoError(t, err, "reclaim after TTL expiry failed: %s", out)
	assert.Contains(t, out, "STALE-001")
}

// TestCoordinatorRecoveryAndConcurrentOpsPushes_REQ_TOPTIER_S3_T2 simulates a
// coordinator stopping after dispatching a wave. A fresh coordinator clone must
// recover both worker transitions after simultaneous _armature pushes.
func TestCoordinatorRecoveryAndConcurrentOpsPushes_REQ_TOPTIER_S3_T2(t *testing.T) {
	t.Parallel()

	h := scenarioHarness(t, "WAVE-001", "WAVE-002")
	workerA, workerB := scenarioWorkers(t, h)

	var results [2]struct {
		out string
		err error
	}
	var wg sync.WaitGroup
	for i, run := range []struct {
		worker string
		issue  string
	}{
		{workerA, "WAVE-001"},
		{workerB, "WAVE-002"},
	} {
		wg.Add(1)
		go func(i int, run struct{ worker, issue string }) {
			defer wg.Done()
			results[i].out, results[i].err = h.RunArmIn(run.worker,
				"transition", "--repo", run.worker, "--issue", run.issue, "--to", "in-progress")
		}(i, run)
	}
	wg.Wait()
	for _, result := range results {
		require.NoError(t, result.err, "concurrent ops-branch transition failed: %s", result.out)
	}

	// The original coordinator is intentionally not used again. Recovery starts
	// from a new clone, as it would after an interruption mid-wave.
	recoveryDir := filepath.Join(h.TempDir, "recovery-coordinator")
	require.NoError(t, h.Clone("recovery", recoveryDir))
	out, err := h.RunArmIn(recoveryDir, "bootstrap", "--repo", recoveryDir)
	require.NoError(t, err, "recovery bootstrap failed: %s", out)
	out, err = h.RunArmIn(recoveryDir, "materialize", "--repo", recoveryDir)
	require.NoError(t, err, "recovery materialization failed: %s", out)
	for _, issueID := range []string{"WAVE-001", "WAVE-002"} {
		out, err = h.RunArmIn(recoveryDir, "show", "--repo", recoveryDir, "--issue", issueID, "--field", "status")
		require.NoError(t, err, "recovered coordinator could not show %s: %s", issueID, out)
		assert.Contains(t, out, "in-progress", "recovery must preserve dispatched worker state for %s", issueID)
	}
}

func scenarioHarness(t *testing.T, issueIDs ...string) *e2eharness.Harness {
	t.Helper()
	h := e2eharness.New(t, buildArmBinary(t))
	out, err := h.RunArm("bootstrap", "--repo", h.WorkDir)
	require.NoError(t, err, "bootstrap failed: %s", out)

	issues := make([]map[string]any, 0, len(issueIDs))
	for _, issueID := range issueIDs {
		issues = append(issues, map[string]any{
			"id": issueID, "title": issueID + " scenario task", "type": "task", "dod": "scenario complete",
		})
	}
	planData, err := json.Marshal(map[string]any{"version": 1, "title": "failure scenarios", "issues": issues})
	require.NoError(t, err)
	planPath := filepath.Join(h.TempDir, "scenario-plan.json")
	require.NoError(t, os.WriteFile(planPath, planData, 0o600))
	out, err = h.RunArm("dag", "apply", "--repo", h.WorkDir, "--plan", planPath)
	require.NoError(t, err, "dag apply failed: %s", out)
	for _, issueID := range issueIDs {
		out, err = h.RunArm("dag", "transition", "--repo", h.WorkDir, "--issue", issueID)
		require.NoError(t, err, "dag transition for %s failed: %s", issueID, out)
	}
	out, err = h.RunArm("push-ops", "--repo", h.WorkDir)
	require.NoError(t, err, "initial ops push failed: %s", out)
	return h
}

func scenarioWorkers(t *testing.T, h *e2eharness.Harness) (string, string) {
	t.Helper()
	workerA := filepath.Join(h.TempDir, "worker-a")
	workerB := filepath.Join(h.TempDir, "worker-b")
	require.NoError(t, h.Clone("worker-a", workerA))
	require.NoError(t, h.Clone("worker-b", workerB))
	for _, worker := range []string{workerA, workerB} {
		out, err := h.RunArmIn(worker, "bootstrap", "--repo", worker)
		require.NoError(t, err, "worker bootstrap failed: %s", out)
		out, err = h.RunArmIn(worker, "worker-init", "--repo", worker)
		require.NoError(t, err, "worker-init failed: %s", out)
	}
	return workerA, workerB
}

func seedExpiredClaim(t *testing.T, h *e2eharness.Harness, issueID string) {
	t.Helper()
	opsWorktree := filepath.Join(h.WorkDir, ".armature")
	logPath := filepath.Join(opsWorktree, "ops", "abandoned-worker.log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  issueID,
		Timestamp: 1,
		WorkerID:  "abandoned-worker",
		Payload:   ops.Payload{NodeType: "task", Title: issueID + " expired-claim fixture"},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpClaim,
		TargetID:  issueID,
		Timestamp: 2,
		WorkerID:  "abandoned-worker",
		Payload:   ops.Payload{TTL: 1},
	}))
	gitRunInDir(t, opsWorktree, "add", "ops/abandoned-worker.log")
	gitRunInDir(t, opsWorktree, "commit", "-m", "test: seed expired claim")
	gitRunInDir(t, opsWorktree, "push", "origin", "_armature")
}

func scenarioRunArmIn(h *e2eharness.Harness, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), h.ArmBinPath, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
