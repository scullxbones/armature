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
	// RC3 merged; RS done feature with RSC done. Parents wired so open RC2
	// under merged E is repair, never claim.
	g := Graph{
		"E":   {Status: ops.StatusMerged, Children: []string{"LF", "R", "AM"}},
		"LF":  {Status: ops.StatusDone, Parent: "E"},
		"R":   {Status: ops.StatusDone, Parent: "E", Children: []string{"RC1", "RC2", "RC3", "RS"}},
		"RC1": {Status: ops.StatusDone, Parent: "R"},
		"RC2": {Status: ops.StatusOpen, Parent: "R"},
		"RC3": {Status: ops.StatusMerged, Parent: "R"},
		"RS":  {Status: ops.StatusDone, Parent: "R", Children: []string{"RSC"}},
		"RSC": {Status: ops.StatusDone, Parent: "RS"},
		// AM/AMC: after rollup AM is merged when AMC is merged — empty remediations.
		"AM":  {Status: ops.StatusMerged, Parent: "E", Children: []string{"AMC"}},
		"AMC": {Status: ops.StatusMerged, Parent: "AM"},
	}

	assert.Equal(t, []string{delivery.RecordArgv("LF")}, RemediationsForIssue("LF", g))
	assert.Equal(t, []string{
		delivery.RecordArgv("RC1"),
		"arm reopen --issue E",
		"arm transition --issue RC2 --to cancelled --force",
		delivery.RecordArgv("RSC"),
	}, RemediationsForIssue("R", g))
	assert.Nil(t, RemediationsForIssue("RC3", g), "merged leaf yields nothing")
	assert.Empty(t, RemediationsForIssue("AM", g), "rollup-merged parent: no dead reopen branch")
	assert.Equal(t, []string{
		delivery.RecordArgv("LF"),
		delivery.RecordArgv("RC1"),
		"arm reopen --issue E",
		"arm transition --issue RC2 --to cancelled --force",
		delivery.RecordArgv("RSC"),
	}, RemediationsForIssue("E", g), "starting at a non-leaf walks pending descendants")

	// Open leaf with no merged ancestor → claim --worktree (healthy tree).
	healthy := Graph{"T": {Status: ops.StatusOpen}}
	assert.Equal(t, []string{"arm claim --issue T --worktree"}, RemediationsForIssue("T", healthy))
}

func TestRemediationsForIssue_FilledDeliverySHAs_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	g := Graph{
		"L": {Status: ops.StatusDone, Base: "aaa111", Tip: "bbb222"},
	}
	got := RemediationsForIssue("L", g)
	require.Equal(t, []string{delivery.RecordArgvFilled("L", "aaa111", "bbb222")}, got)
	assert.False(t, IsDeliveryRecordTemplate(got[0]))
	assert.True(t, IsDeliveryRecordTemplate(delivery.RecordArgv("L")))
}

func TestRemediationsForIssue_AllMergedChildrenEmpty_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// Pre-rollup shape is unreachable after materialize RunRollup (AM/AMC).
	allMerged := Graph{
		"P": {Status: ops.StatusDone, Children: []string{"C"}},
		"C": {Status: ops.StatusMerged, Parent: "P"},
	}
	assert.Empty(t, RemediationsForIssue("P", allMerged))
	assert.Equal(t, "arm doctor", FirstRemediation(RemediationsForIssue("P", allMerged)))
}

func TestIsRunnableArmCommandAndTemplate(t *testing.T) {
	t.Parallel()
	assert.True(t, IsRunnableArmCommand("arm claim --issue T --worktree"))
	assert.False(t, IsRunnableArmCommand(FallbackGuidance))
	assert.False(t, IsRunnableArmCommand(TreeRepairGuidance))
}
