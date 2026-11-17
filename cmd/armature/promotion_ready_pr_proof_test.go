package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

func TestPromotionReadyPRProof_REQ_480(t *testing.T) {
	t.Parallel()

	t.Run("empty-contract", func(t *testing.T) {
		t.Parallel()
		repo, base, tip := promotionReadyFixture(t)
		plantCIGateEvidence(t, repo, tip)
		plantEmptyContractTask(t, repo, "task-empty")
		_, err := runTrls(t, repo, "promotion-ready", "--issue", "task-empty", "--base", base, "--head", tip, "--format", "human")
		require.Error(t, err)
		t.Logf("PROOF empty-contract: %v", err)
		require.Contains(t, err.Error(), "PROMOTION-READY-1")
		require.Contains(t, err.Error(), "empty contract")
	})

	t.Run("missing-assessment", func(t *testing.T) {
		t.Parallel()
		repo, base, tip := promotionReadyFixture(t)
		appendDone(t, repo, "task-01", base, tip)
		plantCIGateEvidence(t, repo, tip)
		_, err := runTrls(t, repo, "promotion-ready", "--issue", "task-01", "--base", base, "--head", tip, "--format", "human")
		require.Error(t, err)
		t.Logf("PROOF missing-assessment: %v", err)
		require.Contains(t, err.Error(), "PROMOTION-READY-1")
		require.Contains(t, err.Error(), "no valid assessment")
	})

	t.Run("stale-head", func(t *testing.T) {
		t.Parallel()
		repo, base, tip := promotionReadyFixture(t)
		appendDone(t, repo, "task-01", base, tip)
		recordAssessmentForTask01(t, repo, base, tip)
		require.NoError(t, os.WriteFile(filepath.Join(repo, "extra.go"), []byte("package main\n"), 0o644))
		run(t, repo, "git", "add", "extra.go")
		run(t, repo, "git", "commit", "-m", "feat(task-01): push after assessment")
		newTip := gitRevParse(t, repo)
		plantCIGateEvidence(t, repo, newTip)
		_, err := runTrls(t, repo, "promotion-ready", "--issue", "task-01", "--base", base, "--head", newTip, "--format", "human")
		require.Error(t, err)
		t.Logf("PROOF stale-head: %v", err)
		require.Contains(t, err.Error(), "PROMOTION-READY-1")
		require.Contains(t, err.Error(), "assessment")
	})

	t.Run("missing-ci-evidence", func(t *testing.T) {
		t.Parallel()
		repo, base, tip := promotionReadyFixture(t)
		appendDone(t, repo, "task-01", base, tip)
		recordAssessmentForTask01(t, repo, base, tip)
		_, err := runTrls(t, repo, "promotion-ready", "--issue", "task-01", "--base", base, "--head", tip, "--format", "human")
		require.Error(t, err)
		t.Logf("PROOF missing-ci-evidence: %v", err)
		require.Contains(t, err.Error(), "PROMOTION-READY-1")
		require.Contains(t, err.Error(), "missing CI gate evidence")
	})

	t.Run("conforming", func(t *testing.T) {
		t.Parallel()
		repo, base, tip := promotionReadyFixture(t)
		appendDone(t, repo, "task-01", base, tip)
		recordAssessmentForTask01(t, repo, base, tip)
		plantCIGateEvidence(t, repo, tip)
		out, err := runTrls(t, repo, "promotion-ready", "--issue", "task-01", "--base", base, "--head", tip, "--format", "human")
		require.NoError(t, err)
		t.Logf("PROOF conforming: %s", out)
		require.Contains(t, out, "promotion-ready: ok")
	})

	t.Run("no-scope", func(t *testing.T) {
		t.Parallel()
		repo, base, tip := promotionReadyFixture(t)
		plantVerifiedTask(t, repo, "on-base", "on-base.go")
		appendDone(t, repo, "on-base", base, base)
		out, err := runTrls(t, repo, "promotion-ready", "scope",
			"--head", tip, "--boundary", base, "--boundary-ref", "main", "--format", "human")
		require.NoError(t, err)
		t.Logf("PROOF no-scope: %s", out)
		require.Contains(t, out, "kind=no-scope")
		require.Contains(t, out, "not a bypass of R2/R3/R4")
		require.NotContains(t, out, "delivery base=")
	})

	t.Run("stacked", func(t *testing.T) {
		t.Parallel()
		repo, _, parentTip := promotionReadyFixture(t)
		require.NoError(t, os.WriteFile(filepath.Join(repo, "stacked.go"), []byte("package main\n"), 0o644))
		run(t, repo, "git", "add", "stacked.go")
		run(t, repo, "git", "commit", "-m", "feat(task-02): stacked tip")
		stacked := gitRevParse(t, repo)

		appendDone(t, repo, "task-01", parentTip, parentTip)
		plantVerifiedTask(t, repo, "task-02", "stacked.go")
		_, err := runTrls(t, repo, "amend", "--issue", "task-02", "--acceptance", testAcceptance, "--dod", "task-02 done")
		require.NoError(t, err)
		appendDone(t, repo, "task-02", parentTip, stacked)

		out, err := runTrls(t, repo, "promotion-ready", "scope",
			"--head", stacked, "--boundary", parentTip, "--boundary-ref", "feat/parent", "--format", "human")
		require.NoError(t, err)
		t.Logf("PROOF stacked: %s", out)
		require.Contains(t, out, "kind=scoped")
		require.Contains(t, out, "stacked=true")
		require.Contains(t, out, "stacked base ref feat/parent is not integration main")
		require.Contains(t, out, "issues=task-02")
		require.NotContains(t, out, "task-01")
	})

	t.Run("ordering", func(t *testing.T) {
		t.Parallel()
		repo, base, tip := promotionReadyFixture(t)
		appendDone(t, repo, "task-01", base, tip)
		_, err := runTrls(t, repo, "promotion-ready", "--issue", "task-01", "--base", base, "--head", tip, "--format", "human")
		require.Error(t, err)
		require.Contains(t, err.Error(), "PROMOTION-READY-1")
		recordAssessmentForTask01(t, repo, base, tip)
		plantCIGateEvidence(t, repo, tip)
		out, err := runTrls(t, repo, "promotion-ready", "--issue", "task-01", "--base", base, "--head", tip, "--format", "human")
		require.NoError(t, err)
		t.Logf("PROOF ordering: assessment recorded for (%s, %s) after the head existed and before promotion-ready passed; %s", base, tip, out)
		require.Contains(t, out, "promotion-ready: ok")
	})
}

func appendDone(t *testing.T, repo, id, base, tip string) {
	t.Helper()
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpTransition,
		TargetID:  id,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			To:      ops.StatusDone,
			Outcome: "delivered",
			Base:    base,
			Tip:     tip,
		},
	}))
}
