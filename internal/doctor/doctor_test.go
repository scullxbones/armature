package doctor_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/trellis/internal/doctor"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_CleanRepo(t *testing.T) {
	// Run creates a temp issues dir so we need a helper.
	// We test the internal checks directly.
	t.Run("D4_NoBrokenParents", func(t *testing.T) {
		index := materialize.Index{
			"task-01":  {Status: "open", Type: "task", Parent: "story-01"},
			"story-01": {Status: "open", Type: "story"},
		}
		report := doctor.RunChecks(index, nil, nil, "")
		d4 := findCheck(t, report, "D4")
		assert.Equal(t, doctor.SeverityOK, d4.Severity)
	})

	t.Run("D4_BrokenParent", func(t *testing.T) {
		index := materialize.Index{
			"task-01": {Status: "open", Type: "task", Parent: "nonexistent"},
		}
		report := doctor.RunChecks(index, nil, nil, "")
		d4 := findCheck(t, report, "D4")
		assert.Equal(t, doctor.SeverityError, d4.Severity)
		assert.Contains(t, d4.Items[0], "task-01")
	})

	t.Run("D5_NoCycle", func(t *testing.T) {
		index := materialize.Index{
			"task-01": {Status: "open", BlockedBy: []string{"task-02"}},
			"task-02": {Status: "open"},
		}
		report := doctor.RunChecks(index, nil, nil, "")
		d5 := findCheck(t, report, "D5")
		assert.Equal(t, doctor.SeverityOK, d5.Severity)
	})

	t.Run("D5_Cycle", func(t *testing.T) {
		index := materialize.Index{
			"task-01": {Status: "open", BlockedBy: []string{"task-02"}},
			"task-02": {Status: "open", BlockedBy: []string{"task-01"}},
		}
		report := doctor.RunChecks(index, nil, nil, "")
		d5 := findCheck(t, report, "D5")
		assert.Equal(t, doctor.SeverityError, d5.Severity)
	})

	t.Run("D6_UncitedIssues", func(t *testing.T) {
		index := materialize.Index{
			"task-01": {Status: "open"},
		}
		allIssues := map[string]*materialize.Issue{
			"task-01": {ID: "task-01", Status: "open"},
		}
		report := doctor.RunChecks(index, allIssues, nil, "")
		d6 := findCheck(t, report, "D6")
		assert.Equal(t, doctor.SeverityWarning, d6.Severity)
		assert.Contains(t, d6.Items, "task-01")
	})

	t.Run("D6_CitedIssue_SourceLink", func(t *testing.T) {
		index := materialize.Index{
			"task-01": {Status: "open"},
		}
		allIssues := map[string]*materialize.Issue{
			"task-01": {
				ID:          "task-01",
				Status:      "open",
				SourceLinks: []materialize.SourceLink{{SourceEntryID: "src-1"}},
			},
		}
		report := doctor.RunChecks(index, allIssues, nil, "")
		d6 := findCheck(t, report, "D6")
		assert.Equal(t, doctor.SeverityOK, d6.Severity)
	})
}

func TestReport_HasErrors(t *testing.T) {
	r := doctor.Report{
		Checks: []doctor.Finding{
			{Check: "D4", Severity: doctor.SeverityError, Message: "broken"},
		},
	}
	assert.True(t, r.HasErrors())
}

func TestReport_HasWarnings(t *testing.T) {
	r := doctor.Report{
		Checks: []doctor.Finding{
			{Check: "D6", Severity: doctor.SeverityWarning, Message: "uncited"},
		},
	}
	assert.True(t, r.HasWarnings())
	assert.False(t, r.HasErrors())
}

func findCheck(t *testing.T, report doctor.Report, checkID string) doctor.Finding {
	t.Helper()
	for _, f := range report.Checks {
		if f.Check == checkID {
			return f
		}
	}
	require.Fail(t, "check not found", checkID)
	return doctor.Finding{}
}

// initIssuesDir sets up a minimal .issues directory for integration tests.
func initIssuesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".issues")
	require.NoError(t, os.MkdirAll(filepath.Join(issuesDir, "ops"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(issuesDir, "state", "issues"), 0755))
	// Write a minimal config.json
	require.NoError(t, os.WriteFile(
		filepath.Join(issuesDir, "config.json"),
		[]byte(`{"mode":"single-branch"}`),
		0644,
	))
	return issuesDir
}

func TestRun_Integration_EmptyRepo(t *testing.T) {
	issuesDir := initIssuesDir(t)

	// Write an empty ops log so materialize can run
	workerLog := filepath.Join(issuesDir, "ops", "test-worker.log")
	require.NoError(t, os.WriteFile(workerLog, []byte(""), 0644))

	report, err := doctor.Run(issuesDir, "")
	require.NoError(t, err)
	// All checks should be OK on an empty repo.
	for _, f := range report.Checks {
		assert.NotEqual(t, doctor.SeverityError, f.Severity, "check %s should not error on empty repo", f.Check)
	}
}

func TestRun_Integration_D3_OrphanedOps(t *testing.T) {
	issuesDir := initIssuesDir(t)

	// Write a note op that references an issue that was never created (no create op).
	// This simulates a corrupt op log referencing a deleted/nonexistent issue.
	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	op := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "ghost-issue-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-01",
		Payload:   ops.Payload{Msg: "progress note"},
	}
	require.NoError(t, ops.AppendOp(logPath, op))

	report, err := doctor.Run(issuesDir, "")
	require.NoError(t, err)

	// D3 should be an error since ghost-issue-01 is not in the graph.
	d3 := findCheck(t, report, "D3")
	assert.Equal(t, doctor.SeverityError, d3.Severity)
	assert.Contains(t, d3.Items, "ghost-issue-01")
}

func TestRun_Integration_D2_StaleClaims(t *testing.T) {
	issuesDir := initIssuesDir(t)

	// Create an issue and claim it with TTL=1 (already expired)
	logPath := filepath.Join(issuesDir, "ops", "worker-02.log")
	createOp := ops.Op{
		Type: ops.OpCreate, TargetID: "stale-01",
		Timestamp: 1, WorkerID: "worker-02",
		Payload: ops.Payload{Title: "Stale task", NodeType: "task"},
	}
	claimOp := ops.Op{
		Type: ops.OpClaim, TargetID: "stale-01",
		Timestamp: 1, WorkerID: "worker-02",
		Payload: ops.Payload{TTL: 1},
	}
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{createOp, claimOp}))

	report, err := doctor.Run(issuesDir, "")
	require.NoError(t, err)

	d2 := findCheck(t, report, "D2")
	assert.Equal(t, doctor.SeverityWarning, d2.Severity)
	assert.Contains(t, d2.Items, "stale-01")
}
