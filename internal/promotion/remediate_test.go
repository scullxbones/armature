package promotion

import (
	"testing"

	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemediationsForIssue_MixedTree_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// E merged epic; LF done leaf; R done story with RC1 done, RC2 open,
	// RC3 merged; RS done feature with RSC done; AC done with only-cancelled ACC.
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

	lf := RemediationsForIssue("LF", g)
	assert.Equal(t, []string{
		"arm reopen --issue E",
		"arm transition --issue LF --to cancelled --force",
		MoreStepsFollowGuidance,
		LegacyTemplateGuidance,
		delivery.RecordArgv("LF"),
	}, lf, "done leaf under merged: complete tree repair then legacy delivery")

	r := RemediationsForIssue("R", g)
	assert.Contains(t, r, "arm reopen --issue E")
	assert.Contains(t, r, "arm transition --issue R --to cancelled --force")
	assert.Contains(t, r, MoreStepsFollowGuidance)
	assert.Contains(t, r, "arm transition --issue RC2 --to cancelled --force")
	assert.NotContains(t, r, "arm claim --issue RC2")
	assert.NotContains(t, r, "arm merged --issue R")

	assert.Nil(t, RemediationsForIssue("RC3", g), "merged leaf yields nothing")
	assert.Empty(t, RemediationsForIssue("AM", g), "rollup-merged parent: no dead reopen branch")

	ac := RemediationsForIssue("AC", g)
	assert.Contains(t, ac, "arm reopen --issue E")
	assert.Contains(t, ac, "arm transition --issue AC --to cancelled --force",
		"all-cancelled under merged: cancel parent clears D14 (rollup will not)")
	assert.NotContains(t, ac, "arm doctor")

	// Open leaf with no merged ancestor → claim --worktree (healthy tree).
	healthy := Graph{"T": {Status: ops.StatusOpen}}
	assert.Equal(t, []string{"arm claim --issue T --worktree"}, RemediationsForIssue("T", healthy))
}

func TestRemediationsForIssue_FilledDeliveryIncludesReview_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	g := Graph{
		"L": {Status: ops.StatusDone, Base: "aaa111", Tip: "bbb222"},
	}
	got := RemediationsForIssue("L", g)
	require.Equal(t, []string{
		delivery.RecordArgvFilled("L", "aaa111", "bbb222"),
		"arm review record --issue L --assessment <assessment.json>",
	}, got)
	assert.False(t, IsDeliveryRecordTemplate(got[0]))
	assert.True(t, IsLegacyTemplateRemediation(got[1]), "assessment placeholder is legacy template")
	assert.True(t, IsDeliveryRecordTemplate(delivery.RecordArgv("L")))
}

func TestRemediationsForIssue_AllMergedChildrenEmpty_REQ_TOPTIER_S18_T4(t *testing.T) {
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

func TestRemediationsForIssue_AllCancelledDescendants_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// G2 merged, P2 done, OS cancelled — cancel OS alone left D14; stranded parent must cancel/reopen P2.
	g := Graph{
		"G2": {Status: ops.StatusMerged, Children: []string{"P2"}},
		"P2": {Status: ops.StatusDone, Parent: "G2", Children: []string{"OS"}},
		"OS": {Status: ops.StatusCancelled, Parent: "P2"},
	}
	got := RemediationsForIssue("P2", g)
	assert.Contains(t, got, "arm reopen --issue G2")
	assert.Contains(t, got, "arm transition --issue P2 --to cancelled --force")
	assert.NotContains(t, got, "arm doctor")
	assert.NotEqual(t, "arm doctor", FirstRemediation(got))

	// No merged ancestor: only stranded cancel/reopen.
	plain := Graph{
		"P":  {Status: ops.StatusDone, Children: []string{"C1", "C2"}},
		"C1": {Status: ops.StatusCancelled, Parent: "P"},
		"C2": {Status: ops.StatusCancelled, Parent: "P"},
	}
	assert.Equal(t, []string{
		"arm transition --issue P --to cancelled --force",
		"arm reopen --issue P",
	}, RemediationsForIssue("P", plain))
}

func TestIsRunnableArmCommandAndTemplate(t *testing.T) {
	t.Parallel()
	assert.True(t, IsRunnableArmCommand("arm claim --issue T --worktree"))
	assert.False(t, IsRunnableArmCommand(FallbackGuidance))
	assert.False(t, IsRunnableArmCommand(TreeRepairGuidance))
	assert.True(t, IsGuidanceRemediation(MoreStepsFollowGuidance))
	assert.True(t, IsLegacyTemplateRemediation(LegacyTemplateGuidance))
	assert.Equal(t, "arm reopen --issue E", FirstRemediation([]string{
		MoreStepsFollowGuidance, "arm reopen --issue E",
	}))
}
