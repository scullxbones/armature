package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/oporder"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMutation_ForeignLiveLeaseBlocksAlwaysFalse_Fails_REQ_TOPTIER_S18_T4
// proves print path calls claim.ForeignLiveLeaseBlocks: forcing it false must
// leak a gate-refused claim into MoreSteps (RED).
func TestMutation_ForeignLiveLeaseBlocksAlwaysFalse_Fails_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo, workerID := setupOwnedForeignMergedFixture(t)
	restore := claim.ReplaceForeignLiveLeaseBlocksForTest(func(claim.Lease, string, int64) bool {
		return false
	})
	t.Cleanup(restore)

	g := loadPromotionGraph(t, repo)
	opts := planOptsForRemediation(getTestContext(t, repo), workerID)
	plan := promotion.RemediationsPlanOpts("OS", g, opts)
	leaked := false
	for _, step := range plan.MoreSteps {
		if step.Argv == "arm claim --issue OS --worktree" {
			leaked = true
		}
	}
	for _, line := range plan.RenderNextActions() {
		if promotion.RunnableArgv(line) == "arm claim --issue OS --worktree" {
			leaked = true
		}
	}
	assert.True(t, leaked,
		"mutation ForeignLiveLeaseBlocks=false must leak claim OS into MoreSteps (RED proof)")
}

// TestMutation_RequireOwnerOff_Fails_REQ_TOPTIER_S18_T4 proves cancel Decide
// calls RequireOwner: forcing it off must offer cancel-other on a live foreign lease.
func TestMutation_RequireOwnerOff_Fails_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo, workerID := setupOwnedForeignMergedFixture(t)
	restore := oporder.ReplaceRequireOwnerForTest(func([]oporder.LocatedOp, string, string) error {
		return nil
	})
	t.Cleanup(restore)

	g := loadPromotionGraph(t, repo)
	opts := planOptsForRemediation(getTestContext(t, repo), workerID)
	plan := promotion.RemediationsPlanOpts("OS", g, opts)
	// With RequireOwner always OK, cancel OS is Decide-allowed → Option B offered
	// (live foreign still blocks claim via ForeignLiveLeaseBlocks, but cancel passes).
	joined := strings.Join(plan.RenderNextActions(), "\n")
	assert.Contains(t, joined, "arm transition --issue OS --to cancelled --force",
		"mutation RequireOwner=off must print cancel-other (RED proof)")
}

// TestMutation_DropDecideFromPrintPath_Fails_REQ_TOPTIER_S18_T4 proves argv
// filtering requires Decide: nil Decide + DefaultDecideOnlyTree leaks foreign claim.
func TestMutation_DropDecideFromPrintPath_Fails_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo, workerID := setupOwnedForeignMergedFixture(t)
	g := loadPromotionGraph(t, repo)
	opts := promotion.PlanOpts{Viewer: workerID, Now: time.Now().Unix()} // Decide nil
	plan := promotion.RemediationsPlanOpts("OS", g, opts)
	leaked := false
	for _, line := range plan.RenderNextActions() {
		if promotion.RunnableArgv(line) == "arm claim --issue OS --worktree" {
			leaked = true
		}
	}
	assert.True(t, leaked,
		"mutation drop Decide from print path must leak claim OS (RED proof)")
}

// TestPrintedArgvExecuteProgress_OwnedForeignLiveExpired_REQ_TOPTIER_S18_T4
// runs every printed Option/MoreSteps/CLAIM-1 next_actions argv for owned +
// foreign-live fixtures and asserts success with progress (no exit-0 no-ops).
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
	for _, cmd := range collectArgv(osPlan) {
		assert.NotEqual(t, "arm claim --issue OS --worktree", cmd)
		assert.NotContains(t, cmd, "arm claim --issue OS --force")
	}

	// CLAIM-1 next_actions for OS under merged.
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
}
