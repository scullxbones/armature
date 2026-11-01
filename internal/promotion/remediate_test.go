package promotion

import (
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	require.NoError(t, validateAlternatives(g, plan))

	nearestOnly := Plan{Alternatives: []Alternative{{
		Steps: []Step{{Argv: "arm reopen --issue M2", Kind: StepCommand}},
	}}}
	err := validateAlternatives(g, nearestOnly)
	require.Error(t, err, "nearest-only reopen must leave M2 open under M1")
	assert.Contains(t, err.Error(), "inconsistency")

	rendered := plan.RenderNextActions()
	assertOptionOwnLines(t, rendered, 'A', "arm reopen --issue M1", "arm reopen --issue M2")
}

func TestRemediationsPlan_OwnerAwareCancel_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	g := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"R"}},
		"R": {Status: ops.StatusDone, Parent: "E", Children: []string{"C"}},
		"C": {Status: ops.StatusClaimed, Parent: "R", ClaimedBy: "worker-other"},
	}
	plan := RemediationsPlan("R", g)
	require.Len(t, plan.Alternatives, 2)
	assert.Equal(t, []string{
		"arm claim --issue C --force --worktree",
		"arm transition --issue C --to cancelled --force",
		"arm transition --issue R --to cancelled --force",
	}, plan.Alternatives[1].RunnableCommands(),
		"Option B must steal claim before cancel; --force on transition does not skip owner")

	rendered := plan.RenderNextActions()
	assert.Contains(t, rendered, "steal claim from worker-other before cancel (transition --force does not skip owner gate)")
	assert.Contains(t, rendered, "arm claim --issue C --force --worktree")

	// Open (unowned) keeps plain cancel.
	open := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"O"}},
		"O": {Status: ops.StatusOpen, Parent: "E"},
	}
	openPlan := RemediationsPlan("O", open)
	assert.Equal(t, []string{"arm transition --issue O --to cancelled --force"},
		openPlan.Alternatives[1].RunnableCommands())
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

func TestDropEitherAlternativeMutatesProducer_REQ_TOPTIER_S18_T4(t *testing.T) { //nolint:paralleltest // mutates package-level planProducer
	g := Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"R"}},
		"R": {Status: ops.StatusDone, Parent: "E"},
	}
	prev := planProducer
	planProducer = func(id string, graph Graph) Plan {
		p := buildRemediationsPlan(id, graph)
		if len(p.Alternatives) >= 2 {
			p.Alternatives = p.Alternatives[:1]
		}
		return p
	}
	t.Cleanup(func() { planProducer = prev })

	plan := RemediationsPlan("R", g)
	assert.Less(t, len(plan.Alternatives), 2,
		"mutating real planProducer must drop Option B on the product path")
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
