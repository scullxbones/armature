package promotion

import (
	"testing"

	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func TestRemediationsForIssue_MixedTree_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// E merged epic; LF done leaf; R done story with RC1 done, RC2 open,
	// RC3 merged; RS done story with RSC done.
	g := Graph{
		"E":   {Status: ops.StatusMerged, Children: []string{"LF", "R"}},
		"LF":  {Status: ops.StatusDone},
		"R":   {Status: ops.StatusDone, Children: []string{"RC1", "RC2", "RC3", "RS"}},
		"RC1": {Status: ops.StatusDone},
		"RC2": {Status: ops.StatusOpen},
		"RC3": {Status: ops.StatusMerged},
		"RS":  {Status: ops.StatusDone, Children: []string{"RSC"}},
		"RSC": {Status: ops.StatusDone},
	}

	assert.Equal(t, []string{delivery.RecordArgv("LF")}, RemediationsForIssue("LF", g))
	assert.Equal(t, []string{
		delivery.RecordArgv("RC1"),
		"arm claim --issue RC2",
		delivery.RecordArgv("RSC"),
	}, RemediationsForIssue("R", g))
	assert.Nil(t, RemediationsForIssue("RC3", g), "merged leaf yields nothing")
	assert.Equal(t, []string{
		delivery.RecordArgv("LF"),
		delivery.RecordArgv("RC1"),
		"arm claim --issue RC2",
		delivery.RecordArgv("RSC"),
	}, RemediationsForIssue("E", g), "starting at a non-leaf walks pending descendants")

	// All children merged under a done non-leaf → empty (caller uses reopen).
	allMerged := Graph{
		"P": {Status: ops.StatusDone, Children: []string{"C"}},
		"C": {Status: ops.StatusMerged},
	}
	assert.Empty(t, RemediationsForIssue("P", allMerged))
	assert.Equal(t, "arm reopen --issue P", ChildrenMergedReopenArgv("P"))
}
