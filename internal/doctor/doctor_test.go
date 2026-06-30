package doctor_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_CleanRepo(t *testing.T) {
	t.Parallel()
	// Run creates a temp issues dir so we need a helper.
	// We test the internal checks directly.
	t.Run("D4_NoBrokenParents", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01":  {Status: "open", Type: "task", Parent: "story-01"},
			"story-01": {Status: "open", Type: "story"},
		}
		report := doctor.RunChecks(index, nil, nil, "", time.Now())
		d4 := findCheck(t, report, "D4")
		assert.Equal(t, doctor.SeverityOK, d4.Severity)
	})

	t.Run("D4_BrokenParent", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01": {Status: "open", Type: "task", Parent: "nonexistent"},
		}
		report := doctor.RunChecks(index, nil, nil, "", time.Now())
		d4 := findCheck(t, report, "D4")
		assert.Equal(t, doctor.SeverityError, d4.Severity)
		assert.Contains(t, d4.Items[0], "task-01")
	})

	t.Run("D5_NoCycle", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01": {Status: "open", BlockedBy: []string{"task-02"}},
			"task-02": {Status: "open"},
		}
		report := doctor.RunChecks(index, nil, nil, "", time.Now())
		d5 := findCheck(t, report, "D5")
		assert.Equal(t, doctor.SeverityOK, d5.Severity)
	})

	t.Run("D5_Cycle", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01": {Status: "open", BlockedBy: []string{"task-02"}},
			"task-02": {Status: "open", BlockedBy: []string{"task-01"}},
		}
		report := doctor.RunChecks(index, nil, nil, "", time.Now())
		d5 := findCheck(t, report, "D5")
		assert.Equal(t, doctor.SeverityError, d5.Severity)
	})

	t.Run("D6_UncitedIssues", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01": {Status: "open"},
		}
		allIssues := map[string]*materialize.Issue{
			"task-01": {ID: "task-01", Status: "open"},
		}
		report := doctor.RunChecks(index, allIssues, nil, "", time.Now())
		d6 := findCheck(t, report, "D6")
		assert.Equal(t, doctor.SeverityWarning, d6.Severity)
		assert.Contains(t, d6.Items, "task-01")
	})

	t.Run("D6_CitedIssue_SourceLink", func(t *testing.T) {
		t.Parallel()
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
		report := doctor.RunChecks(index, allIssues, nil, "", time.Now())
		d6 := findCheck(t, report, "D6")
		assert.Equal(t, doctor.SeverityOK, d6.Severity)
	})
}

func TestReport_HasErrors(t *testing.T) {
	t.Parallel()
	r := doctor.Report{
		Checks: []doctor.Finding{
			{Check: "D4", Severity: doctor.SeverityError, Message: "broken"},
		},
	}
	assert.True(t, r.HasErrors())
}

func TestReport_HasWarnings(t *testing.T) {
	t.Parallel()
	r := doctor.Report{
		Checks: []doctor.Finding{
			{Check: "D6", Severity: doctor.SeverityWarning, Message: "uncited"},
		},
	}
	assert.True(t, r.HasWarnings())
	assert.False(t, r.HasErrors())
}

func TestRunChecks_D2_StaleClaims_InjectedTime(t *testing.T) {
	t.Parallel()
	// Test that injected time is used for stale claim detection
	// Create an issue with a claim that would be fresh at time.Now() but stale at far-future time
	claimedAt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	ttl := 3600 // 1 hour in seconds

	index := materialize.Index{
		"claimed-task": {Status: "open", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"claimed-task": {
			ID:            "claimed-task",
			Status:        "claimed",
			ClaimedAt:     claimedAt,
			LastHeartbeat: claimedAt,
			ClaimTTL:      ttl,
		},
	}

	// At far-future time, claim should be stale
	farFuture := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	report := doctor.RunChecks(index, allIssues, nil, "", farFuture)
	d2 := findCheck(t, report, "D2")
	assert.Equal(t, doctor.SeverityWarning, d2.Severity)
	assert.Contains(t, d2.Items, "claimed-task")
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

// initIssuesDir sets up a minimal .armature directory for integration tests.
func initIssuesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".armature")
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
	t.Parallel()
	issuesDir := initIssuesDir(t)

	// Write an empty ops log so materialize can run
	workerLog := filepath.Join(issuesDir, "ops", "test-worker.log")
	require.NoError(t, os.WriteFile(workerLog, []byte(""), 0644))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", false, time.Now())
	require.NoError(t, err)
	// All checks should be OK on an empty repo.
	for _, f := range report.Checks {
		assert.NotEqual(t, doctor.SeverityError, f.Severity, "check %s should not error on empty repo", f.Check)
	}
}

func TestRun_Integration_D3_OrphanedOps(t *testing.T) {
	t.Parallel()
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

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", false, time.Now())
	require.NoError(t, err)

	// D3 should be an error since ghost-issue-01 is not in the graph.
	d3 := findCheck(t, report, "D3")
	assert.Equal(t, doctor.SeverityError, d3.Severity)
	assert.Contains(t, d3.Items, "ghost-issue-01")
}

func TestRun_ValidatedOpsExcludesMismatches(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	// Create a valid issue first (no mismatch)
	validWorkerLog := filepath.Join(issuesDir, "ops", "worker-valid.log")
	createOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "valid-issue-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-valid",
		Payload:   ops.Payload{Title: "Valid issue", NodeType: "task"},
	}
	require.NoError(t, ops.AppendOp(validWorkerLog, createOp))

	// Create a log file with a mismatched worker ID op (GHOST-99).
	// The op claims to be from "worker-other" but the filename says "worker-mismatched".
	mismatchWorkerLog := filepath.Join(issuesDir, "ops", "worker-mismatched.log")
	mismatchOp := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "mismatched-issue-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-other", // Mismatch! Filename says worker-mismatched
		Payload:   ops.Payload{Msg: "This op has a worker ID mismatch"},
	}
	require.NoError(t, ops.AppendOp(mismatchWorkerLog, mismatchOp))

	// Run doctor
	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", false, time.Now())
	require.NoError(t, err)

	// D3 should report mismatched-issue-01 as orphaned (it has no create op)
	// BUT the key point is that mismatched-issue-01 should appear in D3
	// because the mismatch should cause it to be excluded from the ops list.
	// However, let's verify the logic more carefully: if the op is excluded due to
	// mismatch, it won't be in the opsTargetIDs, so the D3 check should pass.
	// Therefore, D3 should be OK (no orphaned ops).
	d3 := findCheck(t, report, "D3")
	assert.Equal(t, doctor.SeverityOK, d3.Severity,
		"D3 should be OK because mismatched ops are excluded from D3 check")
	assert.NotContains(t, d3.Items, "mismatched-issue-01",
		"Worker-ID mismatched ops should not appear in D3 orphaned list")
	// valid-issue-01 should have a create op, so it's not orphaned
	assert.NotContains(t, d3.Items, "valid-issue-01")

	// D7 should report the worker-ID mismatch warning
	d7 := findCheck(t, report, "D7")
	assert.Equal(t, doctor.SeverityWarning, d7.Severity, "D7 should warn about worker-ID mismatches")
	assert.Len(t, d7.Items, 1, "should report one mismatch warning")
}

func TestRun_CorruptLineDoesNotTriggerD7(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	// Write a log file with a corrupt (non-JSON) line — no worker-ID mismatch.
	logPath := filepath.Join(issuesDir, "ops", "worker-clean.log")
	require.NoError(t, os.WriteFile(logPath, []byte("this is not valid json\n"), 0o644))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", false, time.Now())
	require.NoError(t, err)

	d7 := findCheck(t, report, "D7")
	assert.Equal(t, doctor.SeverityOK, d7.Severity,
		"corrupt-line warnings must not cause D7 to fire as a worker-ID mismatch")
}

func TestRun_Integration_D2_StaleClaims(t *testing.T) {
	t.Parallel()
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

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", false, time.Now())
	require.NoError(t, err)

	d2 := findCheck(t, report, "D2")
	assert.Equal(t, doctor.SeverityWarning, d2.Severity)
	assert.Contains(t, d2.Items, "stale-01")
}

func TestRun_Integration_D3_Verbose_ShowsFileAndLine(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	logPath := filepath.Join(issuesDir, "ops", "worker-verbose.log")
	op := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "ghost-verbose-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-verbose",
		Payload:   ops.Payload{Msg: "orphaned note"},
	}
	require.NoError(t, ops.AppendOp(logPath, op))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", true, time.Now())
	require.NoError(t, err)

	d3 := findCheck(t, report, "D3")
	assert.Equal(t, doctor.SeverityError, d3.Severity)
	// Regular items unchanged — just the orphaned ID
	assert.Contains(t, d3.Items, "ghost-verbose-01")
	// VerboseItems should include file name and line number
	require.NotEmpty(t, d3.VerboseItems)
	assert.Contains(t, d3.VerboseItems[0], "worker-verbose.log")
	assert.Contains(t, d3.VerboseItems[0], "ghost-verbose-01")
}

func TestRun_Integration_Verbose_CleanRepo_NoExtraOutput(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	workerLog := filepath.Join(issuesDir, "ops", "worker-clean.log")
	require.NoError(t, os.WriteFile(workerLog, []byte(""), 0644))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", true, time.Now())
	require.NoError(t, err)

	for _, f := range report.Checks {
		assert.Empty(t, f.VerboseItems, "no verbose items on clean repo for check %s", f.Check)
	}
}

func TestDoctorRunUsesStateDir(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	stateDir := filepath.Join(t.TempDir(), "specific-state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0755))

	// Write an empty ops log so materialize can run
	workerLog := filepath.Join(issuesDir, "ops", "test-worker.log")
	require.NoError(t, os.WriteFile(workerLog, []byte(""), 0644))

	// Write a mock index.json to the specific stateDir
	index := materialize.Index{
		"T-001": {Status: "open", Type: "task"},
	}
	indexPath := filepath.Join(stateDir, "index.json")
	require.NoError(t, materialize.WriteIndex(indexPath, index))

	// doctor.Run should load the index from stateDir.
	// We pass an empty repoPath to skip D1 git divergence.
	report, err := doctor.Run(issuesDir, stateDir, "", false, time.Now())
	require.NoError(t, err)

	// D4 checks broken parent refs. If it saw T-001, it means it loaded the index.
	// Since T-001 has no parent, D4 should be OK.
	d4 := findCheck(t, report, "D4")
	assert.Equal(t, doctor.SeverityOK, d4.Severity)
}

func TestRunChecks_D1_GitDivergence(t *testing.T) {
	t.Parallel()
	// Create a temp git repo with a commit referencing an issue not in done/merged state (doctor.go:159)
	repoDir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	run("git", "config", "commit.gpgsign", "false")
	run("git", "commit", "--allow-empty", "-m", "feat(task-open-1): implement feature")

	index := materialize.Index{
		"task-open-1": {Status: "in-progress", Type: "task"},
	}
	report := doctor.RunChecks(index, nil, nil, repoDir, time.Now())
	d1 := findCheck(t, report, "D1")
	assert.Equal(t, doctor.SeverityWarning, d1.Severity, "D1 should warn when commit references non-done issue")
	assert.Contains(t, d1.Items[0], "task-open-1")
}

func TestRunChecks_D1_DoneIssue_NoWarning(t *testing.T) {
	t.Parallel()
	// Done issues referenced in commits should not trigger D1 warning (covers 159:46 — "merged" branch)
	repoDir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	run("git", "config", "commit.gpgsign", "false")
	run("git", "commit", "--allow-empty", "-m", "feat(task-done-1): implement feature")

	index := materialize.Index{
		"task-done-1": {Status: "done", Type: "task"},
	}
	report := doctor.RunChecks(index, nil, nil, repoDir, time.Now())
	d1 := findCheck(t, report, "D1")
	assert.Equal(t, doctor.SeverityOK, d1.Severity, "D1 should be OK when commit references done issue")
}

func TestRun_PhysicalLineUsedForD3Verbose(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	logPath := filepath.Join(issuesDir, "ops", "worker-physical.log")

	// Write three lines:
	// Line 1: accepted op (orphaned - note op without create)
	acceptedOp1 := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "orphan-1",
		Timestamp: 100,
		WorkerID:  "worker-physical",
		Payload:   ops.Payload{Msg: "First"},
	}
	require.NoError(t, ops.AppendOp(logPath, acceptedOp1))

	// Line 2: mismatched op (rejected, not in items)
	mismatchOp := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "orphan-2",
		Timestamp: 101,
		WorkerID:  "worker-wrong",
		Payload:   ops.Payload{Msg: "Mismatch"},
	}
	require.NoError(t, ops.AppendOp(logPath, mismatchOp))

	// Line 3: accepted op (orphaned, needs D3 verbose output)
	acceptedOp3 := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "orphan-3",
		Timestamp: 102,
		WorkerID:  "worker-physical",
		Payload:   ops.Payload{Msg: "Third"},
	}
	require.NoError(t, ops.AppendOp(logPath, acceptedOp3))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", true, time.Now())
	require.NoError(t, err)

	d3 := findCheck(t, report, "D3")
	assert.Equal(t, doctor.SeverityError, d3.Severity, "should have orphaned ops")
	assert.Contains(t, d3.Items, "orphan-1", "orphan-1 should be in items")
	assert.Contains(t, d3.Items, "orphan-3", "orphan-3 should be in items")

	// Check verbose items use physical line numbers, not ordinal position
	require.NotEmpty(t, d3.VerboseItems, "verbose items should be present")

	// Find the verbose items for orphan-1 and orphan-3
	verbose1Found := false
	verbose3Found := false
	for _, vi := range d3.VerboseItems {
		if strings.Contains(vi, "orphan-1") {
			verbose1Found = true
			// orphan-1 should be at physical line 1
			assert.Contains(t, vi, "worker-physical.log:1", "orphan-1 should be reported at physical line 1")
		}
		if strings.Contains(vi, "orphan-3") {
			verbose3Found = true
			// orphan-3 should be at physical line 3 (not line 2, which was the mismatch)
			assert.Contains(t, vi, "worker-physical.log:3", "orphan-3 should be reported at physical line 3")
		}
	}
	assert.True(t, verbose1Found, "verbose item for orphan-1 should be present")
	assert.True(t, verbose3Found, "verbose item for orphan-3 should be present")
}
