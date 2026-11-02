package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMutation_DropDecideFromPrintPath_Fails_REQ_TOPTIER_S18_T4 proves argv
// filtering requires Decide: nil Decide + DefaultDecideOnlyTree leaks foreign
// live claim on a healthy tree (no merged ancestor). Gate kill proofs for
// RequireOwner / ForeignLiveLeaseBlocks / tree consistency live in
// remediation_gates_test.go (real path; no injected Decide).
func TestMutation_DropDecideFromPrintPath_Fails_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	g := promotion.Graph{
		"P": {Status: ops.StatusDone, Children: []string{"C"}},
		"C": {
			Status: ops.StatusClaimed, Parent: "P", ClaimedBy: "other",
			ClaimToken: "tok", ClaimedAt: now - 30, LastActivity: now - 30, TTLMinutes: 60,
		},
	}
	opts := promotion.PlanOpts{Viewer: "me", Now: now} // Decide nil
	plan := promotion.RemediationsPlanOpts("P", g, opts)
	leaked := false
	for _, line := range plan.RenderNextActions() {
		if promotion.RunnableArgv(line) == "arm claim --issue C --worktree" {
			leaked = true
		}
	}
	assert.True(t, leaked,
		"mutation drop Decide from print path must leak claim C (RED proof)")
}

// TestPrintedArgvExecuteProgress_OwnedForeignLiveExpired_REQ_TOPTIER_S18_T4
// runs every printed Option/CLAIM-1 next_actions argv for owned, foreign-live,
// and foreign-expired fixtures IN ORDER via the binary → D14 clear; no exit-0 no-op.
func TestPrintedArgvExecuteProgress_OwnedForeignLiveExpired_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo, workerID := setupOwnedForeignMergedFixture(t)
	ctx := getTestContext(t, repo)
	opts := planOptsForRemediation(ctx, workerID)
	g := loadPromotionGraph(t, repo)

	collectArgv := func(plan promotion.Plan) []string {
		var out []string
		for _, line := range plan.RenderNextActions() {
			if cmd := promotion.RunnableArgv(line); cmd != "" {
				out = append(out, cmd)
			}
		}
		return out
	}

	osPlan := promotion.RemediationsPlanOpts("OS", g, opts)
	require.Len(t, osPlan.Alternatives, 1, "live foreign hides Option B")
	for _, cmd := range collectArgv(osPlan) {
		assert.NotEqual(t, "arm claim --issue OS --worktree", cmd)
		assert.NotContains(t, cmd, "arm claim --issue OS --force")
	}

	// CLAIM-1 next_actions for OS under merged (live).
	out := new(bytes.Buffer)
	code := executeThenHandleRootError(t, out, new(bytes.Buffer),
		"claim", "--issue", "OS", "--force", "--worktree", "--repo", repo, "--format", "agent")
	require.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, out.String())
	for _, a := range cf.NextActions {
		cmd := promotion.RunnableArgv(a)
		if cmd == "" {
			continue
		}
		assert.NotEqual(t, "arm claim --issue OS --worktree", cmd)
		assert.NotEqual(t, "arm claim --issue OS --force --worktree", cmd)
	}

	ownPlan := promotion.RemediationsPlanOpts("OWN", g, opts)
	for i := range ownPlan.Alternatives {
		fresh, fw := setupOwnedForeignMergedFixture(t)
		before := loadPromotionGraph(t, fresh)
		plan := promotion.RemediationsPlanOpts("OWN", before,
			planOptsForRemediation(getTestContext(t, fresh), fw))
		require.NoError(t, applyAlternative(t, fresh, plan.Alternatives[i]), "option %c", 'A'+i)
		after := loadPromotionGraph(t, fresh)
		assert.Empty(t, filterViolations(promotion.TreeConsistencyViolations(after), subjectSubtree("OWN", before)),
			"option %c must clear OWN under-merged (progress, not a no-op)", 'A'+i)
	}

	// Live guidance once across OS+P2 doctor findings.
	d14 := doctorD14Explain(t, repo)
	count := 0
	for _, s := range d14.suggested {
		if strings.Contains(s, "live claimed lease on OS held by ") {
			count++
		}
	}
	assert.Equal(t, 1, count, "live-lease guidance must appear once per doctor run")

	// Foreign-expired: claim OS --ttl 1, wait past expiry. Done P2 is reopen-only;
	// Option B (doctor --fix + cancel) lives on open OS's plan.
	expBase, expWorker := setupForeignExpiredMergedFixture(t)
	expOpts := planOptsForRemediation(getTestContext(t, expBase), expWorker)
	expG := loadPromotionGraph(t, expBase)
	require.Equal(t, ops.StatusClaimed, expG["OS"].Status)

	// CLAIM-1 after expiry while G2 still merged: must NOT print claim OS.
	claimOut := new(bytes.Buffer)
	claimCode := executeThenHandleRootError(t, claimOut, new(bytes.Buffer),
		"claim", "--issue", "OS", "--force", "--worktree", "--repo", expBase, "--format", "agent")
	require.Equal(t, 1, claimCode)
	claimCF := agentFailureFromStdout(t, claimOut.String())
	for _, a := range claimCF.NextActions {
		cmd := promotion.RunnableArgv(a)
		assert.NotEqual(t, "arm claim --issue OS --worktree", cmd,
			"CLAIM-1 after expiry under merged must not print claim OS")
	}
	assert.Contains(t, claimCF.NextActions, "arm reopen --issue G2")

	p2Plan := promotion.RemediationsPlanOpts("P2", expG, expOpts)
	require.Len(t, p2Plan.Alternatives, 1, "done P2: reopen only — never cancel done")
	assert.Equal(t, []string{"arm reopen --issue G2"}, p2Plan.Alternatives[0].RunnableCommands())

	osPlan = promotion.RemediationsPlanOpts("OS", expG, expOpts)
	require.GreaterOrEqual(t, len(osPlan.Alternatives), 2, "expired foreign open OS must offer Option B")
	require.NotEmpty(t, osPlan.Alternatives[1].RunnableCommands())
	assert.Equal(t, "arm doctor --fix", osPlan.Alternatives[1].RunnableCommands()[0])
	assert.Contains(t, osPlan.Alternatives[1].RunnableCommands(),
		"arm transition --issue OS --to cancelled --force")

	// Fresh expired fixtures per Option on OS (open subject).
	for i := range osPlan.Alternatives {
		fresh, fw := setupForeignExpiredMergedFixture(t)
		before := loadPromotionGraph(t, fresh)
		plan := promotion.RemediationsPlanOpts("OS", before,
			planOptsForRemediation(getTestContext(t, fresh), fw))
		require.GreaterOrEqual(t, len(plan.Alternatives), 2)
		require.NoError(t, applyAlternative(t, fresh, plan.Alternatives[i]), "expired option %c", 'A'+i)
		after := loadPromotionGraph(t, fresh)
		assert.Empty(t, filterViolations(promotion.TreeConsistencyViolations(after), subjectSubtree("OS", before)),
			"expired option %c must clear OS under-merged", 'A'+i)
	}
}

// setupForeignExpiredMergedFixture builds G2(merged)→P2(done)→OS foreign claimed
// with --ttl 1, then waits past expiry so doctor --fix can release it.
func setupForeignExpiredMergedFixture(t *testing.T) (repo, workerID string) {
	t.Helper()
	repo = gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)
	ctx := getTestContext(t, repo)
	var logPath string
	var err error
	workerID, logPath, err = resolveWorkerAndLog(ctx)
	require.NoError(t, err)

	for _, c := range []struct {
		id, title, typ, parent string
	}{
		{"G2", "Epic G2", "epic", ""},
		{"P2", "Story P2", "story", "G2"},
		{"OS", "Foreign OS", "task", "P2"},
	} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpCreate, TargetID: c.id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{
				Title: c.title, NodeType: c.typ, Parent: c.parent,
				DefinitionOfDone: c.title + " is complete and tested",
				Acceptance:       json.RawMessage(testAcceptance),
				Confidence:       "verified",
				Scope:            []string{"cmd/armature/" + strings.ToLower(c.id) + ".go"},
			},
		}))
	}
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "P2", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{To: ops.StatusDone, Outcome: "fixture done parent under merged for expired foreign"},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	other := "other-worker-foreign"
	_, err = runTrls(t, repo, "claim", "--issue", "OS", "--worktree", "--ttl", "1", "--worker-id", other)
	require.NoError(t, err)

	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "G2", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{To: ops.StatusMerged},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// TTL minutes=1 → 60s window; wait past expiry for doctor --fix / Option B.
	time.Sleep(65 * time.Second)
	return repo, workerID
}
