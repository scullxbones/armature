package doctor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/gitoid"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorD13WarnsUnmatchedMerged_REQ_LNGHZN_S11_T4(t *testing.T) {
	t.Parallel()

	allIssues := map[string]*materialize.Issue{
		"legacy-b":  {ID: "legacy-b", Status: ops.StatusMerged},
		"legacy-a":  {ID: "legacy-a", Status: ops.StatusMerged, TargetSHA: gitoid.PadSHA1("a")},
		"promoted":  {ID: "promoted", Status: ops.StatusMerged, MatchedCommit: gitoid.PadSHA1("c")},
		"rollup":    {ID: "rollup", Status: ops.StatusMerged, Children: []string{"promoted"}},
		"done-leaf": {ID: "done-leaf", Status: ops.StatusDone},
		"nil-entry": nil,
	}
	f := checkD13MergedWithoutDeliveryMatch(allIssues)
	assert.Equal(t, "D13", f.Check)
	assert.Equal(t, SeverityWarning, f.Severity)
	assert.Equal(t, "Merged issues with no stored delivery match", f.Message)
	assert.Equal(t, []string{"legacy-a", "legacy-b"}, f.Items,
		"only merged leaves without matched_commit; a target SHA alone is not a match")
}

func TestDoctorD13OKWhenEveryMergedLeafMatched_REQ_LNGHZN_S11_T4(t *testing.T) {
	t.Parallel()

	allIssues := map[string]*materialize.Issue{
		"promoted":  {ID: "promoted", Status: ops.StatusMerged, MatchedCommit: gitoid.PadSHA1("c")},
		"rollup":    {ID: "rollup", Status: ops.StatusMerged, Children: []string{"promoted"}},
		"done-leaf": {ID: "done-leaf", Status: ops.StatusDone},
	}
	f := checkD13MergedWithoutDeliveryMatch(allIssues)
	assert.Equal(t, "D13", f.Check)
	assert.Equal(t, SeverityOK, f.Severity)
	assert.Equal(t, "No merged issues without a stored delivery match", f.Message)
	assert.Empty(t, f.Items)
}

func TestDoctorD13NeverDemotesMerged_REQ_LNGHZN_S11_T4(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(issuesDir, "state")
	opsDir := filepath.Join(issuesDir, "ops")
	logPath := filepath.Join(opsDir, "worker-d13.log")
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "LEGACY", Timestamp: 1, WorkerID: "worker-d13",
			Payload: ops.Payload{Title: "merged before ADR 0022", NodeType: "task"}},
		{Type: ops.OpTransition, TargetID: "LEGACY", Timestamp: 2, WorkerID: "worker-d13",
			Payload: ops.Payload{To: ops.StatusMerged}},
		{Type: ops.OpCreate, TargetID: "PROMOTED", Timestamp: 3, WorkerID: "worker-d13",
			Payload: ops.Payload{Title: "promoted by arm sync", NodeType: "task"}},
		{Type: ops.OpTransition, TargetID: "PROMOTED", Timestamp: 4, WorkerID: "worker-d13",
			Payload: ops.Payload{
				To:              ops.StatusMerged,
				TargetSHA:       gitoid.PadSHA1("a"),
				CombinedPatchID: gitoid.PadSHA1("b"),
				MatchedCommit:   gitoid.PadSHA1("c"),
			}},
	}))
	logBefore, err := os.ReadFile(logPath)
	require.NoError(t, err)
	entriesBefore, err := os.ReadDir(opsDir)
	require.NoError(t, err)

	for range 2 {
		report, runErr := Run(issuesDir, stateDir, "", "", false, time.Now())
		require.NoError(t, runErr)
		d13 := findCheck(t, report, "D13")
		assert.Equal(t, SeverityWarning, d13.Severity)
		assert.Equal(t, []string{"LEGACY"}, d13.Items)
	}

	logAfter, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, logBefore, logAfter, "D13 must not append ops")
	entriesAfter, err := os.ReadDir(opsDir)
	require.NoError(t, err)
	assert.Equal(t, entriesBefore, entriesAfter, "D13 must not create ops logs")

	index, issues, err := LoadState(issuesDir, stateDir)
	require.NoError(t, err)
	assert.Equal(t, ops.StatusMerged, index["LEGACY"].Status, "D13 must not demote merged")
	require.Contains(t, issues, "LEGACY")
	assert.Equal(t, ops.StatusMerged, issues["LEGACY"].Status, "D13 must not demote merged")
	assert.Empty(t, PlanFixes(issues, "worker-d13", time.Now(), ""),
		"doctor --fix must not plan a demotion for an unmatched merged issue")
}
