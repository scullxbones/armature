package e2eharness_test

import (
	"encoding/json"
	"os"
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
	workers := []string{workerA, workerB}

	var raceResults [2]struct {
		out string
		err error
	}
	var wg sync.WaitGroup
	for i, worker := range workers {
		wg.Add(1)
		go func(i int, worker string) {
			defer wg.Done()
			raceResults[i].out, raceResults[i].err = h.RunArmIn(worker,
				"claim", "--repo", worker, "--issue", "RACE-001", "--worktree")
		}(i, worker)
	}
	wg.Wait()

	for _, result := range raceResults {
		require.NoError(t, result.err, "concurrent claim must fail only by losing the race: %s", result.out)
	}
	workerIDs := []string{
		gitConfigValue(t, workerA, "armature.worker-id"),
		gitConfigValue(t, workerB, "armature.worker-id"),
	}
	require.NotEqual(t, workerIDs[0], workerIDs[1], "the race needs two distinct workers")

	// Concurrent commands may each make a provisional local claim. The durable
	// contract is the recovered materialized state: exactly one of those two
	// workers is authoritative, and independent replays choose the same owner.
	recoveryDir := filepath.Join(h.TempDir, "race-recovery")
	require.NoError(t, h.Clone("race-recovery", recoveryDir))
	out, err := h.RunArmIn(recoveryDir, "bootstrap", "--repo", recoveryDir)
	require.NoError(t, err, "race recovery bootstrap failed: %s", out)
	out, err = h.RunArmIn(recoveryDir, "materialize", "--repo", recoveryDir)
	require.NoError(t, err, "race recovery materialization failed: %s", out)
	out, err = h.RunArmIn(recoveryDir, "show", "--repo", recoveryDir, "--issue", "RACE-001", "--field", "claimed_by")
	require.NoError(t, err, "race recovery show failed: %s", out)
	authoritativeOwner := strings.TrimSpace(out)
	require.Contains(t, workerIDs, authoritativeOwner, "replay must choose exactly one race participant as owner")

	secondRecoveryDir := filepath.Join(h.TempDir, "race-recovery-second")
	require.NoError(t, h.Clone("race-recovery-second", secondRecoveryDir))
	out, err = h.RunArmIn(secondRecoveryDir, "bootstrap", "--repo", secondRecoveryDir)
	require.NoError(t, err, "second race recovery bootstrap failed: %s", out)
	out, err = h.RunArmIn(secondRecoveryDir, "materialize", "--repo", secondRecoveryDir)
	require.NoError(t, err, "second race recovery materialization failed: %s", out)
	out, err = h.RunArmIn(secondRecoveryDir, "show", "--repo", secondRecoveryDir, "--issue", "RACE-001", "--field", "claimed_by")
	require.NoError(t, err, "second race recovery show failed: %s", out)
	assert.Equal(t, authoritativeOwner, strings.TrimSpace(out), "independent replays must retain one authoritative winner")

	out, err = h.RunArmIn(workerB, "claim", "--repo", workerB, "--issue", "STALE-001", "--worktree")
	require.NoError(t, err, "reclaim after TTL expiry failed: %s", out)
	assert.Contains(t, out, "STALE-001")
}

// TestCoordinatorRecoveryResumesPartialWave_REQ_TOPTIER_S3_T2 simulates a
// coordinator crash after dispatching only the first item in a wave. A fresh
// coordinator must observe that partial state and dispatch only the remaining
// item instead of recreating the entire wave.
func TestCoordinatorRecoveryResumesPartialWave_REQ_TOPTIER_S3_T2(t *testing.T) {
	t.Parallel()

	h := scenarioHarness(t, "WAVE-001", "WAVE-002")
	out, err := h.RunArm("worker-init", "--repo", h.WorkDir)
	require.NoError(t, err, "initial coordinator worker-init failed: %s", out)

	// The first coordinator gets only the first work item out before it crashes.
	out, err = h.RunArm("claim", "--repo", h.WorkDir, "--issue", "WAVE-001", "--worktree")
	require.NoError(t, err, "initial wave dispatch failed: %s", out)
	assertScenarioStatus(t, h, h.WorkDir, "WAVE-001", "claimed")
	assertScenarioStatus(t, h, h.WorkDir, "WAVE-002", "open")

	// Do not reuse h.WorkDir: this is the interruption boundary. The new clone
	// must observe the partial wave and dispatch only the remaining item.
	recoveryDir := filepath.Join(h.TempDir, "recovery-coordinator")
	require.NoError(t, h.Clone("recovery", recoveryDir))
	out, err = h.RunArmIn(recoveryDir, "bootstrap", "--repo", recoveryDir)
	require.NoError(t, err, "recovery bootstrap failed: %s", out)
	out, err = h.RunArmIn(recoveryDir, "worker-init", "--repo", recoveryDir)
	require.NoError(t, err, "recovery worker-init failed: %s", out)
	assertScenarioStatus(t, h, recoveryDir, "WAVE-001", "claimed")
	assertScenarioStatus(t, h, recoveryDir, "WAVE-002", "open")

	out, err = h.RunArmIn(recoveryDir, "claim", "--repo", recoveryDir, "--issue", "WAVE-002", "--worktree")
	require.NoError(t, err, "recovered coordinator must dispatch remaining wave item: %s", out)
	assertScenarioStatus(t, h, recoveryDir, "WAVE-002", "claimed")
	assertScenarioStatus(t, h, recoveryDir, "WAVE-001", "claimed")
}

func assertScenarioStatus(t *testing.T, h *e2eharness.Harness, repo, issueID, want string) {
	t.Helper()
	out, err := h.RunArmIn(repo, "materialize", "--repo", repo)
	require.NoError(t, err, "materialize %s failed: %s", issueID, out)
	out, err = h.RunArmIn(repo, "show", "--repo", repo, "--issue", issueID, "--field", "status")
	require.NoError(t, err, "show status for %s failed: %s", issueID, out)
	assert.Equal(t, want, strings.TrimSpace(out), "materialized status for %s", issueID)
}

func scenarioHarness(t *testing.T, issueIDs ...string) *e2eharness.Harness {
	t.Helper()
	h := e2eharness.New(t, buildArmBinary(t))
	out, err := h.RunArm("bootstrap", "--repo", h.WorkDir)
	require.NoError(t, err, "bootstrap failed: %s", out)

	issues := make([]map[string]any, 0, len(issueIDs))
	for _, issueID := range issueIDs {
		issues = append(issues, map[string]any{
			"id":         issueID,
			"title":      issueID + " scenario task",
			"type":       "task",
			"source":     "src-e2e",
			"dod":        "scenario complete",
			"scope":      "cmd/armature/" + issueID + ".go",
			"acceptance": []map[string]any{{"type": "test_passes"}},
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
