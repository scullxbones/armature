package promotion

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCLIDecide uses real claim.ForeignLiveLeaseBlocks (AcceptAt's predicate).
func testCLIDecide(viewer string, now int64) func(string, Graph) error {
	if now == 0 {
		now = time.Now().Unix()
	}
	return func(argv string, g Graph) error {
		argv = strings.TrimSpace(argv)
		if !strings.HasPrefix(argv, "arm ") {
			return nil
		}
		issue := IssueFlag(argv)
		if issue == "" {
			return nil
		}
		node := g[issue]
		switch {
		case strings.Contains(argv, "arm claim "):
			if WouldViolateTreeConsistency(issue, g) {
				return fmt.Errorf("tree consistency")
			}
			held := claim.LeaseFromClocks(node.Status, node.ClaimedBy, node.ClaimToken,
				node.ClaimedAt, node.LastActivity, 0, node.TTLMinutes, "")
			if claim.ForeignLiveLeaseBlocks(held, viewer, now) {
				return fmt.Errorf("foreign live lease")
			}
		case strings.Contains(argv, "--to cancelled"):
			switch node.Status {
			case ops.StatusClaimed, ops.StatusInProgress:
				held := claim.LeaseFromClocks(node.Status, node.ClaimedBy, node.ClaimToken,
					node.ClaimedAt, node.LastActivity, 0, node.TTLMinutes, "")
				if claim.ForeignLiveLeaseBlocks(held, viewer, now) {
					return claim.ErrNotClaimOwner
				}
				if held.Holder != "" && viewer != "" && held.Holder != viewer {
					return claim.ErrNotClaimOwner
				}
			}
		}
		return nil
	}
}

func TestRemediationsPlan_MixedTreeOptions_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	g := Graph{
		"E":   {Status: ops.StatusMerged, Children: []string{"LF", "R", "AM", "AC"}},
		"LF":  {Status: ops.StatusDone, Parent: "E"},
		"R":   {Status: ops.StatusDone, Parent: "E", Children: []string{"RC1", "RC2", "RC3", "RS"}},
		"RC1": {Status: ops.StatusDone, Parent: "R"},
		"RC2": {Status: ops.StatusOpen, Parent: "R"},
		"RC3": {Status: ops.StatusMerged, Parent: "R"},
		"RS":  {Status: ops.StatusDone, Parent: "R", Children: []string{"RSC"}},
		"RSC": {Status: ops.StatusDone, Parent: "RS"},
		"AM":  {Status: ops.StatusMerged, Parent: "E", Children: []string{"AMC"}},
		"AMC": {Status: ops.StatusMerged, Parent: "AM"},
		"AC":  {Status: ops.StatusDone, Parent: "E", Children: []string{"ACC"}},
		"ACC": {Status: ops.StatusCancelled, Parent: "AC"},
	}

	lfPlan := RemediationsPlan("LF", g)
	require.Len(t, lfPlan.Alternatives, 2)
	assert.Equal(t, []string{"arm reopen --issue E"}, lfPlan.Alternatives[0].RunnableCommands())
	assert.Equal(t, []string{"arm transition --issue LF --to cancelled --force"}, lfPlan.Alternatives[1].RunnableCommands())
	lf := lfPlan.RenderNextActions()
	assertOptionOwnLines(t, lf, 'A', "arm reopen --issue E")
	assertOptionOwnLines(t, lf, 'B', "arm transition --issue LF --to cancelled --force")
	assert.Contains(t, lf, MoreStepsFollowGuidance)
	assert.Contains(t, lf, LegacyTemplateGuidance)
	assert.Contains(t, lf, delivery.RecordArgv("LF"))
	assert.Equal(t, "arm reopen --issue E", FirstRemediation(lf))

	rPlan := RemediationsPlan("R", g)
	require.Len(t, rPlan.Alternatives, 2)
	assert.Equal(t, []string{"arm reopen --issue E"}, rPlan.Alternatives[0].RunnableCommands())
	assert.Equal(t, []string{
		"arm transition --issue RC2 --to cancelled --force",
		"arm transition --issue R --to cancelled --force",
	}, rPlan.Alternatives[1].RunnableCommands(),
		"Option B must cancel open descendants then self")
	r := rPlan.RenderNextActions()
	assertOptionOwnLines(t, r, 'A', "arm reopen --issue E")
	assertOptionOwnLines(t, r, 'B',
		"arm transition --issue RC2 --to cancelled --force",
		"arm transition --issue R --to cancelled --force")
	assert.Contains(t, r, "arm reopen --issue E", "Option label is own line; argv is paste-runnable")
	assert.Contains(t, r, MoreStepsFollowGuidance)
	assert.Contains(t, r, "arm claim --issue RC2 --worktree",
		"more-steps after reopen treat RC2 as healthy open leaf")
	assert.NotContains(t, r, "arm claim --issue R")
	assert.Equal(t, "arm reopen --issue E", FirstRemediation(r))

	assert.Empty(t, RemediationsPlan("RC3", g).RenderNextActions())
	assert.Empty(t, RemediationsPlan("AM", g).RenderNextActions())

	acPlan := RemediationsPlan("AC", g)
	require.Len(t, acPlan.Alternatives, 2)
	ac := acPlan.RenderNextActions()
	assertOptionOwnLines(t, ac, 'A', "arm reopen --issue E")
	assertOptionOwnLines(t, ac, 'B', "arm transition --issue AC --to cancelled --force")
	assert.NotContains(t, ac, "arm doctor")

	healthy := Graph{"T": {Status: ops.StatusOpen}}
	assert.Equal(t, []string{"arm claim --issue T --worktree"}, RemediationsForIssue("T", healthy))
}

func TestRemediationsPlan_NestedMergedAncestorChain_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// M1 → M2 → D: nearest-only reopen of M2 leaves M2 open under merged M1.
	g := Graph{
		"M1": {Status: ops.StatusMerged, Children: []string{"M2"}},
		"M2": {Status: ops.StatusMerged, Parent: "M1", Children: []string{"D"}},
		"D":  {Status: ops.StatusDone, Parent: "M2"},
	}
	assert.Equal(t, []string{"M1", "M2"}, mergedAncestorChain("D", g))

	plan := RemediationsPlan("D", g)
	require.Len(t, plan.Alternatives, 2)
	assert.Equal(t, []string{
		"arm reopen --issue M1",
		"arm reopen --issue M2",
	}, plan.Alternatives[0].RunnableCommands(),
		"Option A must reopen FULL merged-ancestor chain rootward-first")
	assert.Equal(t, []string{"arm transition --issue D --to cancelled --force"},
		plan.Alternatives[1].RunnableCommands())

	// CmdGate + construction: full chain is offered; nearest-only is not a product Option.
	assert.Equal(t, []string{"M1", "M2"}, mergedAncestorChain("D", g))
	nearestOnly := Alternative{Steps: []Step{{Argv: "arm reopen --issue M2", Kind: StepCommand}}}
	afterNearest := applyReopenCancelEffects(g, nearestOnly)
	viol := TreeConsistencyViolations(afterNearest)
	require.NotEmpty(t, viol, "nearest-only reopen must leave M2 open under M1")
	assert.Contains(t, strings.Join(viol, ";"), "M2 under merged M1")

	rendered := plan.RenderNextActions()
	assertOptionOwnLines(t, rendered, 'A', "arm reopen --issue M1", "arm reopen --issue M2")
}

func TestRemediationsPlan_ForeignLeaseNoInventedTakeover_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	opts := PlanOpts{Viewer: "me", Now: now, Decide: testCLIDecide("me", now)}

	g := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"R"}},
		"R": {Status: ops.StatusDone, Parent: "E", Children: []string{"C"}},
		"C": {
			Status: ops.StatusClaimed, Parent: "R", ClaimedBy: "worker-other",
			ClaimToken: "tok", ClaimedAt: now - 30, LastActivity: now - 30, TTLMinutes: 60,
		},
	}
	plan := RemediationsPlanOpts("R", g, opts)
	require.Len(t, plan.Alternatives, 1, "foreign live lease must drop cancel-other-worker Option B")
	assert.Equal(t, []string{"arm reopen --issue E"}, plan.Alternatives[0].RunnableCommands())
	rendered := plan.RenderNextActions()
	assert.NotContains(t, rendered, "Option B:")
	for _, line := range rendered {
		if cmd := RunnableArgv(line); cmd != "" {
			assert.NotEqual(t, "arm transition --issue C --to cancelled --force", cmd)
			assert.NotEqual(t, "arm claim --issue C --worktree", cmd,
				"MoreSteps must not emit gate-refused claim on foreign live lease")
		}
	}
	joined := strings.Join(plan.Guidance, "\n")
	assert.Contains(t, joined, "live claimed lease on C held by worker-other")
	assert.Contains(t, joined, "arm claim --force only overrides scope overlap")
	assert.Contains(t, joined, ".worktrees/C")
	assert.Contains(t, joined, fmt.Sprintf("unix %d", now-30+60*60), "real TTL expiry in guidance")

	leaf := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"C"}},
		"C": {
			Status: ops.StatusClaimed, Parent: "E", ClaimedBy: "worker-other",
			ClaimToken: "tok", ClaimedAt: now - 30, LastActivity: now - 30, TTLMinutes: 60,
		},
	}
	leafPlan := RemediationsPlanOpts("C", leaf, opts)
	require.Len(t, leafPlan.Alternatives, 1)
	assert.Empty(t, leafPlan.MoreSteps, "foreign leaf must not print arm claim in MoreSteps")

	// Expired foreign: Option B re-enabled for cancel targets Decide allows (R done);
	// C cancel still refused (not owner) so omitted from Option B steps.
	expired := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"R"}},
		"R": {Status: ops.StatusDone, Parent: "E", Children: []string{"C"}},
		"C": {
			Status: ops.StatusClaimed, Parent: "R", ClaimedBy: "worker-other",
			ClaimToken: "tok", ClaimedAt: now - 4000, LastActivity: now - 4000, TTLMinutes: 1,
		},
	}
	expPlan := RemediationsPlanOpts("R", expired, opts)
	require.GreaterOrEqual(t, len(expPlan.Alternatives), 2, "expired foreign must re-enable Option B")
	for _, step := range expPlan.Alternatives[1].Steps {
		assert.NotEqual(t, "arm transition --issue C --to cancelled --force", step.Argv,
			"expired foreign cancel still NOT-CLAIM-OWNER — omit from Option B")
	}
	assert.Contains(t, expPlan.Alternatives[1].RunnableCommands(), "arm transition --issue R --to cancelled --force")

	// Owned claim: viewer matches holder → Option B cancel offered.
	owned := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"R"}},
		"R": {Status: ops.StatusDone, Parent: "E", Children: []string{"C"}},
		"C": {
			Status: ops.StatusClaimed, Parent: "R", ClaimedBy: "me",
			ClaimToken: "tok", ClaimedAt: now - 30, LastActivity: now - 30, TTLMinutes: 60,
		},
	}
	ownedPlan := RemediationsPlanOpts("R", owned, opts)
	require.Len(t, ownedPlan.Alternatives, 2)
	assert.Equal(t, []string{
		"arm transition --issue C --to cancelled --force",
		"arm transition --issue R --to cancelled --force",
	}, ownedPlan.Alternatives[1].RunnableCommands())

	open := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"O"}},
		"O": {Status: ops.StatusOpen, Parent: "E"},
	}
	openPlan := RemediationsPlanOpts("O", open, opts)
	assert.Equal(t, []string{"arm transition --issue O --to cancelled --force"},
		openPlan.Alternatives[1].RunnableCommands())
}

func TestDecide_MatchesClaimForeignLiveLeaseBlocks_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	now := int64(1_700_000_000)
	d := testCLIDecide("me", now)
	g := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"C", "O"}},
		"C": {
			Status: ops.StatusClaimed, Parent: "E", ClaimedBy: "other",
			ClaimToken: "tok", ClaimedAt: now - 10, LastActivity: now - 10, TTLMinutes: 60,
		},
		"O": {Status: ops.StatusOpen, Parent: "E"},
	}
	assert.Error(t, d("arm claim --issue O --worktree", g))
	assert.Error(t, d("arm claim --issue C --force --worktree", g))
	assert.Error(t, d("arm transition --issue C --to cancelled --force", g))
	assert.NoError(t, testCLIDecide("other", now)("arm transition --issue C --to cancelled --force", g))
	assert.NoError(t, d("arm reopen --issue E", g))
}

func TestRemediationsPlan_FilledDeliveryRequiredInput_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	g := Graph{
		"L": {Status: ops.StatusDone, Base: "aaa111", Tip: "bbb222"},
	}
	plan := RemediationsPlan("L", g)
	require.Len(t, plan.MoreSteps, 2)
	assert.Equal(t, StepCommand, plan.MoreSteps[0].Kind)
	assert.Equal(t, delivery.RecordArgvFilled("L", "aaa111", "bbb222"), plan.MoreSteps[0].Argv)
	assert.Equal(t, StepRequiredInput, plan.MoreSteps[1].Kind)
	assert.Equal(t, "arm review record --issue L --assessment <assessment.json> --bundle <bundle.json>", plan.MoreSteps[1].Argv)
	assert.Contains(t, plan.MoreSteps[1].Help, "produce assessment.json:")
	assert.Contains(t, plan.MoreSteps[1].Help, "arm review prepare --issue L --base aaa111 --head bbb222")
	assert.Contains(t, plan.MoreSteps[1].Help, "arm review schema --output conformance-assessment.schema.json")
	assert.Contains(t, plan.MoreSteps[1].Help, "--bundle bundle.json")
	assert.NotContains(t, plan.MoreSteps[1].Help, "docs/schemas/conformance-assessment.schema.json")
	assert.NotContains(t, plan.MoreSteps[1].Help, "legacy")

	got := plan.RenderNextActions()
	require.GreaterOrEqual(t, len(got), 2)
	assert.Equal(t, delivery.RecordArgvFilled("L", "aaa111", "bbb222"), got[0])
	assert.True(t, strings.HasPrefix(got[1], "produce assessment.json:"))
	assert.Equal(t, "arm review record --issue L --assessment <assessment.json> --bundle <bundle.json>", got[2])
	assert.False(t, IsLegacyTemplateRemediation(got[2]))
	assert.True(t, IsRequiredInputRemediation(got[2]))
}

func TestMissingAssessmentActions_AssessmentHowToOnce_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	acts := missingAssessmentActions("FL", "base1", "tip1")
	joined := strings.Join(acts, "\n")
	assert.Equal(t, 1, strings.Count(joined, "arm review prepare --issue FL"),
		"prepare/schema/validate how-to must appear once, not doubled")
	assert.Equal(t, 1, strings.Count(joined, "arm review schema --output"))
	assert.Equal(t, 1, strings.Count(joined, "arm review validate --assessment"))
	assert.Contains(t, acts, "arm review record --issue FL --assessment <assessment.json> --bundle <bundle.json>")
}

func TestRemediationsPlan_AllMergedChildrenEmpty_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	allMerged := Graph{
		"P": {Status: ops.StatusDone, Children: []string{"C"}},
		"C": {Status: ops.StatusMerged, Parent: "P"},
	}
	assert.Empty(t, RemediationsForIssue("P", allMerged))
	assert.Equal(t, "arm doctor", FirstRemediation(RemediationsForIssue("P", allMerged)))

	mixed := Graph{
		"P": {Status: ops.StatusDone, Children: []string{"M", "X"}},
		"M": {Status: ops.StatusMerged, Parent: "P"},
		"X": {Status: ops.StatusCancelled, Parent: "P"},
	}
	assert.Empty(t, RemediationsForIssue("P", mixed))
}

func TestRemediationsPlan_AllCancelledDescendants_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	g := Graph{
		"G2": {Status: ops.StatusMerged, Children: []string{"P2"}},
		"P2": {Status: ops.StatusDone, Parent: "G2", Children: []string{"OS"}},
		"OS": {Status: ops.StatusCancelled, Parent: "P2"},
	}
	plan := RemediationsPlan("P2", g)
	require.Len(t, plan.Alternatives, 2)
	got := plan.RenderNextActions()
	assertOptionOwnLines(t, got, 'A', "arm reopen --issue G2")
	assertOptionOwnLines(t, got, 'B', "arm transition --issue P2 --to cancelled --force")
	assert.Equal(t, "arm reopen --issue G2", FirstRemediation(got))

	plain := Graph{
		"P":  {Status: ops.StatusDone, Children: []string{"C1", "C2"}},
		"C1": {Status: ops.StatusCancelled, Parent: "P"},
		"C2": {Status: ops.StatusCancelled, Parent: "P"},
	}
	plainPlan := RemediationsPlan("P", plain)
	require.Len(t, plainPlan.Alternatives, 2)
	plainGot := plainPlan.RenderNextActions()
	assertOptionOwnLines(t, plainGot, 'A', "arm transition --issue P --to cancelled --force")
	assertOptionOwnLines(t, plainGot, 'B', "arm reopen --issue P")
}

func TestTreeConsistencyInvariant_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	bad := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"R"}},
		"R": {Status: ops.StatusClaimed, Parent: "E"},
	}
	viol := TreeConsistencyViolations(bad)
	require.NotEmpty(t, viol)
	assert.Contains(t, viol[0], "R under merged E")
	assert.True(t, WouldViolateTreeConsistency("R", bad))

	good := Graph{
		"E": {Status: ops.StatusDone, Children: []string{"R"}},
		"R": {Status: ops.StatusOpen, Parent: "E"},
	}
	assert.Empty(t, TreeConsistencyViolations(good))
	assert.False(t, WouldViolateTreeConsistency("R", good))

	before := Graph{
		"E":   {Status: ops.StatusMerged},
		"R":   {Status: ops.StatusDone},
		"RC1": {Status: ops.StatusDone},
		"RC3": {Status: ops.StatusMerged},
	}
	afterFlat := Graph{
		"E":   {Status: ops.StatusOpen},
		"R":   {Status: ops.StatusCancelled},
		"RC1": {Status: ops.StatusCancelled},
		"RC3": {Status: ops.StatusOpen},
	}
	destroyed := FinishedWorkDestroyed(before, afterFlat, map[string]bool{"E": true})
	assert.Contains(t, strings.Join(destroyed, ";"), "R finished work destroyed")
	assert.Contains(t, strings.Join(destroyed, ";"), "RC1 finished work destroyed")
	assert.Contains(t, strings.Join(destroyed, ";"), "RC3 un-merged")

	afterReopen := Graph{
		"E":   {Status: ops.StatusOpen},
		"R":   {Status: ops.StatusDone},
		"RC1": {Status: ops.StatusDone},
		"RC3": {Status: ops.StatusMerged},
	}
	assert.Empty(t, FinishedWorkDestroyed(before, afterReopen, map[string]bool{"E": true}))
}

func TestRenderOrderNotAlphaSorted_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	g := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"Z", "A"}},
		"Z": {Status: ops.StatusDone, Parent: "E"},
		"A": {Status: ops.StatusOpen, Parent: "E"},
	}
	plan := RemediationsPlan("Z", g)
	got := plan.RenderNextActions()
	require.GreaterOrEqual(t, len(got), 2)
	assert.True(t, IsOptionLabel(got[0]), got[0])
	assert.Equal(t, "Option A:", got[0])
	assert.Equal(t, "arm reopen --issue E", got[1])
	assert.Equal(t, "Option B:", got[2])
	eKids := healthyDescendantOrLeafSteps("E", g["E"], g)
	require.GreaterOrEqual(t, len(eKids), 2)
	joined := ""
	for _, s := range eKids {
		joined += s.Argv + "\n"
	}
	zi := strings.Index(joined, "Z")
	ai := strings.Index(joined, "A")
	require.GreaterOrEqual(t, zi, 0)
	require.GreaterOrEqual(t, ai, 0)
	assert.Less(t, zi, ai, "model order follows children slice, not alpha")
}

func TestDropEitherAlternativeMutatesProducer_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	g := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"R"}},
		"R": {Status: ops.StatusDone, Parent: "E"},
	}
	opts := PlanOpts{
		Producer: func(id string, graph Graph, o PlanOpts) Plan {
			p := buildRemediationsPlan(id, graph, o)
			if len(p.Alternatives) >= 2 {
				p.Alternatives = p.Alternatives[:1]
			}
			return p
		},
	}
	plan := RemediationsPlanOpts("R", g, opts)
	assert.Less(t, len(plan.Alternatives), 2,
		"Producer pass-in must drop Option B on the product path")
	assert.NotContains(t, plan.RenderNextActions(), "Option B:")
}

func assertOptionOwnLines(t *testing.T, got []string, option rune, cmds ...string) {
	t.Helper()
	label := optionLabel(int(option - 'A'))
	idx := -1
	for i, s := range got {
		if s == label {
			idx = i
			break
		}
	}
	require.GreaterOrEqual(t, idx, 0, "missing %s in %v", label, got)
	require.True(t, IsOptionLabel(got[idx]), "%q must be a standalone Option label", got[idx])
	for i, cmd := range cmds {
		require.Less(t, idx+1+i, len(got), "missing step after %s", label)
		assert.Equal(t, cmd, got[idx+1+i], "step %d after %s", i, label)
		assert.False(t, strings.HasPrefix(got[idx+1+i], "Option "),
			"argv must not carry Option prefix: %q", got[idx+1+i])
	}
}
