package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_CleanRepo(t *testing.T) {
	t.Parallel()

	t.Run("D4_NoBrokenParents", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01":  {Status: "open", Type: "task", Parent: "story-01"},
			"story-01": {Status: "open", Type: "story"},
		}
		report := runChecks(index, nil, "", time.Now())
		d4 := findCheck(t, report, "D4")
		assert.Equal(t, SeverityOK, d4.Severity)
	})

	t.Run("D4_BrokenParent", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01": {Status: "open", Type: "task", Parent: "nonexistent"},
		}
		report := runChecks(index, nil, "", time.Now())
		d4 := findCheck(t, report, "D4")
		assert.Equal(t, SeverityError, d4.Severity)
		assert.Contains(t, d4.Items[0], "task-01")
	})

	t.Run("D4_SelfParent", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"SF": {Status: "done", Type: "task", Parent: "SF"},
		}
		report := runChecks(index, nil, "", time.Now())
		d4 := findCheck(t, report, "D4")
		assert.Equal(t, SeverityError, d4.Severity, "Parent==id must be a D4 broken parent ref")
		require.Len(t, d4.Items, 1)
		assert.Equal(t, "SF -> SF", d4.Items[0])
		d5 := findCheck(t, report, "D5")
		assert.Equal(t, SeverityOK, d5.Severity, "D5 only walks blocked_by; self-parent is D4")
	})

	t.Run("D5_NoCycle", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01": {Status: "open", BlockedBy: []string{"task-02"}},
			"task-02": {Status: "open"},
		}
		report := runChecks(index, nil, "", time.Now())
		d5 := findCheck(t, report, "D5")
		assert.Equal(t, SeverityOK, d5.Severity)
	})

	t.Run("D5_Cycle", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01": {Status: "open", BlockedBy: []string{"task-02"}},
			"task-02": {Status: "open", BlockedBy: []string{"task-01"}},
		}
		report := runChecks(index, nil, "", time.Now())
		d5 := findCheck(t, report, "D5")
		assert.Equal(t, SeverityError, d5.Severity)
	})

	t.Run("D6_UncitedIssues", func(t *testing.T) {
		t.Parallel()
		index := materialize.Index{
			"task-01": {Status: "open"},
		}
		allIssues := map[string]*materialize.Issue{
			"task-01": {ID: "task-01", Status: "open"},
		}
		report := runChecks(index, allIssues, "", time.Now())
		d6 := findCheck(t, report, "D6")
		assert.Equal(t, SeverityWarning, d6.Severity)
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
		report := runChecks(index, allIssues, "", time.Now())
		d6 := findCheck(t, report, "D6")
		assert.Equal(t, SeverityOK, d6.Severity)
	})
}

func TestReport_HasErrors(t *testing.T) {
	t.Parallel()
	r := Report{
		Checks: []Finding{
			{Check: "D4", Severity: SeverityError, Message: "broken"},
		},
	}
	assert.True(t, r.HasErrors())
}

func TestReport_HasWarnings(t *testing.T) {
	t.Parallel()
	r := Report{
		Checks: []Finding{
			{Check: "D6", Severity: SeverityWarning, Message: "uncited"},
		},
	}
	assert.True(t, r.HasWarnings())
	assert.False(t, r.HasErrors())
}

func TestRunChecks_D2_StaleClaims_InjectedTime(t *testing.T) {
	t.Parallel()

	claimedAt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	ttl := 3600

	index := materialize.Index{
		"claimed-task": {Status: "open", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"claimed-task": {
			ID:            "claimed-task",
			Status:        "claimed",
			ClaimedBy:     "worker-a",
			ClaimedAt:     claimedAt,
			LastHeartbeat: claimedAt,
			ClaimTTL:      ttl,
		},
	}

	farFuture := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	report := runChecks(index, allIssues, "", farFuture)
	d2 := findCheck(t, report, "D2")
	assert.Equal(t, SeverityWarning, d2.Severity)
	assert.Contains(t, d2.Items, "claimed-task")
}

func TestEvaluateD9UnrecognizedWorktrees_REQ_LNGHZN_S5_T8(t *testing.T) {
	t.Parallel()

	t.Run("clean_when_none", func(t *testing.T) {
		t.Parallel()
		f := evaluateD9UnrecognizedWorktrees(nil)
		assert.Equal(t, "D9", f.Check)
		assert.Equal(t, SeverityOK, f.Severity)
		assert.Empty(t, f.Items)
	})

	t.Run("warns_and_lists_paths", func(t *testing.T) {
		t.Parallel()
		f := evaluateD9UnrecognizedWorktrees([]string{"/repo/.worktrees/b", "/repo/.worktrees/a"})
		assert.Equal(t, "D9", f.Check)
		assert.Equal(t, SeverityWarning, f.Severity)

		assert.Equal(t, []string{"/repo/.worktrees/a", "/repo/.worktrees/b"}, f.Items)
	})
}

func TestRunChecks_D9_SkippedWhenIssueDetailsOmitted_REQ_LNGHZN_S5_T8(t *testing.T) {
	t.Parallel()

	repoDir := gittest.InitRepo(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}
	run("git", "commit", "--allow-empty", "-m", "chore: initial commit")

	boundPath := filepath.Join(repoDir, ".worktrees", "T-001")
	run("git", "worktree", "add", "-b", "bound-branch", boundPath)
	bindingPath := filepath.Join(repoDir, ".git", "worktrees", "T-001", "armature-issue-id")
	require.NoError(t, os.WriteFile(bindingPath, []byte("T-001\n"), 0o644))

	index := materialize.Index{
		"T-001": {Status: "in-progress", Type: "task"},
	}
	report := runChecks(index, nil, repoDir, time.Now())

	d9 := findCheck(t, report, "D9")
	assert.Equal(t, SeverityOK, d9.Severity, "D9 must be skipped when issue details were omitted")
	assert.Empty(t, d9.Items)
}

func TestRun_Integration_D9_UnrecognizedManagedWorktree_REQ_LNGHZN_S5_T8(t *testing.T) {
	t.Parallel()

	repoDir := gittest.InitRepo(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}
	run("git", "commit", "--allow-empty", "-m", "chore: initial commit")

	strayPath := filepath.Join(repoDir, ".worktrees", "stray")
	run("git", "worktree", "add", "-b", "stray-branch", strayPath)

	issuesDir := filepath.Join(repoDir, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(issuesDir, "ops"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(issuesDir, "state", "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "config.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "worker-01.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), repoDir, "", false, time.Now())
	require.NoError(t, err)

	d9 := findCheck(t, report, "D9")
	assert.Equal(t, SeverityWarning, d9.Severity, "unrecognized managed worktree must be a health problem")
	require.NotEmpty(t, d9.Items)
	assert.Contains(t, strings.Join(d9.Items, "\n"), "stray")

	assert.True(t, report.HasWarnings(), "--strict must fail on the unrecognized worktree")
	assert.False(t, report.HasErrors(), "plain doctor must not error on the unrecognized worktree")
}

func TestRun_IncludesConfigHealth_REQ_LNGHZN_S7_T2(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(issuesDir, "config.json"),
		[]byte(`{"project_type":"go","token_budegt":1600}`),
		0o644,
	))
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	d10 := findCheck(t, report, "D10")
	assert.Equal(t, SeverityError, d10.Severity)
	require.NotEmpty(t, d10.Items)
	assert.Contains(t, strings.Join(d10.Items, "\n"), "token_budegt")
}

func TestRunChecks_IncludesConfigHealth_REQ_LNGHZN_S7_T2(t *testing.T) {
	t.Parallel()
	repoDir := t.TempDir()
	armatureDir := filepath.Join(repoDir, ".armature")
	require.NoError(t, os.MkdirAll(armatureDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(armatureDir, "config.json"),
		[]byte(`{"project_type":"go","token_budegt":1600}`),
		0o644,
	))

	report := runChecks(nil, nil, repoDir, time.Now())
	d10 := findCheck(t, report, "D10")
	assert.Equal(t, SeverityError, d10.Severity)
	require.NotEmpty(t, d10.Items)
	assert.Contains(t, strings.Join(d10.Items, "\n"), "token_budegt")
}

func findCheck(t *testing.T, report Report, checkID string) Finding {
	t.Helper()
	for _, f := range report.Checks {
		if f.Check == checkID {
			return f
		}
	}
	require.Fail(t, "check not found", checkID)
	return Finding{}
}

func initIssuesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".armature")
	require.NoError(t, os.MkdirAll(filepath.Join(issuesDir, "ops"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(issuesDir, "state", "issues"), 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(issuesDir, "config.json"),
		[]byte(`{}`),
		0644,
	))
	return issuesDir
}

func TestRun_Integration_EmptyRepo(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	workerLog := filepath.Join(issuesDir, "ops", "test-worker.log")
	require.NoError(t, os.WriteFile(workerLog, []byte(""), 0644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	for _, f := range report.Checks {
		assert.NotEqual(t, SeverityError, f.Severity, "check %s should not error on empty repo", f.Check)
	}
}

func TestLiveCheckIDsMatchesRun_REQ_TOPTIER_S18_T0(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	runIDs := make([]string, 0, len(report.Checks))
	for _, f := range report.Checks {
		runIDs = append(runIDs, f.Check)
	}
	assert.Equal(t, LiveCheckIDs(), runIDs, "LiveCheckIDs must match the live Run path in order")
	assert.Equal(t, []string{"D1", "D2", "D3", "D4", "D5", "D6", "D7", "D8", "D9", "D10", "D11", "D12", "D14"}, LiveCheckIDs())
}

func TestDoctorD14_WarnsOnDoneUnderMerged_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()

	index := materialize.Index{
		"story-01": {Status: "merged", Type: "story"},
		"task-01":  {Status: "done", Type: "task", Parent: "story-01"},
		"task-02":  {Status: "merged", Type: "task", Parent: "story-01"},
	}
	f := checkD14DoneUnderMerged(index)
	assert.Equal(t, "D14", f.Check)
	assert.Equal(t, SeverityWarning, f.Severity)
	assert.Equal(t, "Done or open/claimed work under a merged ancestor", f.Message)
	require.Len(t, f.Items, 1)
	assert.Equal(t, "task-01 -> story-01", f.Items[0])
}

func TestDoctorD14_WarnsOnDoneUnderMergedAncestor_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// E merged → S done → T done: T's direct parent is not merged, but ancestor E is.
	index := materialize.Index{
		"E": {Status: "merged", Type: "epic"},
		"S": {Status: "done", Type: "story", Parent: "E"},
		"T": {Status: "done", Type: "task", Parent: "S"},
	}
	f := checkD14DoneUnderMerged(index)
	require.Equal(t, SeverityWarning, f.Severity)
	assert.Contains(t, f.Items, "S -> E")
	assert.Contains(t, f.Items, "T -> E", "D14 must walk all ancestors, not only the direct parent")
}

func TestDoctorD14_SkipsMergedUnderCancelled_ORCRUN_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()

	// Include a done child under cancelled: if D14 were broadened to warn on
	// cancelled parents (exclusion removed), this fixture must fail the test.
	index := materialize.Index{
		"ORCRUN":        {Status: "cancelled", Type: "epic"},
		"ORCRUN-S1":     {Status: "merged", Type: "story", Parent: "ORCRUN"},
		"ORCRUN-T-done": {Status: "done", Type: "task", Parent: "ORCRUN"},
		"healthy":       {Status: "merged", Type: "story"},
		"child":         {Status: "merged", Type: "task", Parent: "healthy"},
	}
	f := checkD14DoneUnderMerged(index)
	assert.Equal(t, SeverityOK, f.Severity, "done/merged under cancelled must not become a D14 finding")
	assert.Empty(t, f.Items)
	assert.Equal(t, "No done issues under a merged parent", f.Message)
}

func TestDoctorD14_OKWhenDoneParentNotMerged_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()

	index := materialize.Index{
		"story-01": {Status: "in-progress", Type: "story"},
		"task-01":  {Status: "done", Type: "task", Parent: "story-01"},
		"story-02": {Status: "cancelled", Type: "story"},
		"task-02":  {Status: "done", Type: "task", Parent: "story-02"},
	}
	f := checkD14DoneUnderMerged(index)
	assert.Equal(t, SeverityOK, f.Severity)
	assert.Empty(t, f.Items)
}

func TestDoctorD14_SkipsMissingParentAndSelfParent_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()

	// D14 skips; D4 owns missing and self-parent. Keep D14 one index pass, no git.
	index := materialize.Index{
		"ghost-child": {Status: "done", Type: "task", Parent: "missing-parent"},
		"loop-child":  {Status: "done", Type: "task", Parent: "loop-child"},
		"story-ok":    {Status: "merged", Type: "story"},
		"task-ok":     {Status: "merged", Type: "task", Parent: "story-ok"},
	}
	f := checkD14DoneUnderMerged(index)
	assert.Equal(t, SeverityOK, f.Severity)
	assert.Empty(t, f.Items)

	d4 := checkD4BrokenParentRefs(index)
	assert.Equal(t, SeverityError, d4.Severity)
	assert.Contains(t, d4.Items, "ghost-child -> missing-parent")
	assert.Contains(t, d4.Items, "loop-child -> loop-child")
}

func TestDoctorD4_FlagsSelfParent_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()

	// Repro class: Parent==id is present in the index (so "missing parent" D4
	// would miss it) and blocked_by is empty (so D5 stays OK). Removing the
	// D4 self-parent branch must fail this test.
	index := materialize.Index{
		"SF": {Status: "done", Type: "task", Parent: "SF"},
	}
	d4 := checkD4BrokenParentRefs(index)
	require.Equal(t, SeverityError, d4.Severity)
	require.Equal(t, []string{"SF -> SF"}, d4.Items)

	d5 := checkD5DependencyCycles(index)
	assert.Equal(t, SeverityOK, d5.Severity)

	d14 := checkD14DoneUnderMerged(index)
	assert.Equal(t, SeverityOK, d14.Severity, "D14 must not warn; D4 owns self-parent")
}

func TestDoctorD14_EmittedByRun_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)
	d14 := findCheck(t, report, "D14")
	assert.Equal(t, SeverityOK, d14.Severity, "empty repo must not warn D14")
}

func TestDoctorD14_RunFailClosedOnIndexWriteBlock_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0644))
	stateDir := filepath.Join(issuesDir, "state")
	_, err := Run(issuesDir, stateDir, "", "", false, time.Now())
	require.NoError(t, err)
	// Block rematerialize from rewriting index.json so the next Run cannot load
	// a usable index — D14 must not soft-skip; Run fails closed.
	indexPath := filepath.Join(stateDir, "index.json")
	require.NoError(t, os.Remove(indexPath))
	require.NoError(t, os.Mkdir(indexPath, 0755))

	_, err = Run(issuesDir, stateDir, "", "", false, time.Now())
	require.Error(t, err, "index-load/write errors must fail closed before D14 runs")
}

func TestRun_Integration_D3_GateEvidenceIsNotAnOrphan_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	require.NoError(t, ops.AppendGateEvidence(logPath, "worker-01", ops.GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Start:   100,
		End:     101,
		Exit:    0,
	}))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	d3 := findCheck(t, report, "D3")
	assert.Equal(t, SeverityOK, d3.Severity,
		"gate-evidence targeting a profile name must not be reported as an orphaned issue")
	assert.NotContains(t, d3.Items, "full")
}

func TestRun_Integration_D3_OrphanedOps(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	op := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "ghost-issue-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-01",
		Payload:   ops.Payload{Msg: "progress note"},
	}
	require.NoError(t, ops.AppendOp(logPath, op))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	d3 := findCheck(t, report, "D3")
	assert.Equal(t, SeverityError, d3.Severity)
	assert.Contains(t, d3.Items, "ghost-issue-01")
}

func TestRun_Integration_D3_SkipsDeletedNoteOnlyIssues(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	noteOp := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "list",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-01",
		Payload:   ops.Payload{Msg: "ARCHIMP-S19-T2", NoteID: "note-1"},
	}
	deleteOp := ops.Op{
		Type:      ops.OpNoteDelete,
		TargetID:  "list",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-01",
		Payload:   ops.Payload{NoteID: "note-1"},
	}
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{noteOp, deleteOp}))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	d3 := findCheck(t, report, "D3")
	assert.Equal(t, SeverityOK, d3.Severity,
		"a note that was created and fully deleted against a bogus target should not orphan D3")
	assert.NotContains(t, d3.Items, "list")
}

func TestRun_Integration_D3_UndeletedNoteStillOrphans(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	logPath := filepath.Join(issuesDir, "ops", "worker-01.log")
	noteOp := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "ghost-issue-02",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-01",
		Payload:   ops.Payload{Msg: "still here", NoteID: "note-2"},
	}
	require.NoError(t, ops.AppendOp(logPath, noteOp))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	d3 := findCheck(t, report, "D3")
	assert.Equal(t, SeverityError, d3.Severity)
	assert.Contains(t, d3.Items, "ghost-issue-02")
}

func TestRun_ValidatedOpsExcludesMismatches(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	validWorkerLog := filepath.Join(issuesDir, "ops", "worker-valid.log")
	createOp := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "valid-issue-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-valid",
		Payload:   ops.Payload{Title: "Valid issue", NodeType: "task"},
	}
	require.NoError(t, ops.AppendOp(validWorkerLog, createOp))

	mismatchWorkerLog := filepath.Join(issuesDir, "ops", "worker-mismatched.log")
	mismatchOp := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "mismatched-issue-01",
		Timestamp: time.Now().Unix(),
		WorkerID:  "worker-other",
		Payload:   ops.Payload{Msg: "This op has a worker ID mismatch"},
	}
	require.NoError(t, ops.AppendOp(mismatchWorkerLog, mismatchOp))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	d3 := findCheck(t, report, "D3")
	assert.Equal(t, SeverityOK, d3.Severity,
		"D3 should be OK because mismatched ops are excluded from D3 check")
	assert.NotContains(t, d3.Items, "mismatched-issue-01",
		"Worker-ID mismatched ops should not appear in D3 orphaned list")

	assert.NotContains(t, d3.Items, "valid-issue-01")

	d7 := findCheck(t, report, "D7")
	assert.Equal(t, SeverityWarning, d7.Severity, "D7 should warn about worker-ID mismatches")
	assert.Len(t, d7.Items, 1, "should report one mismatch warning")
}

func TestRun_CorruptLineDoesNotTriggerD7(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	logPath := filepath.Join(issuesDir, "ops", "worker-clean.log")
	require.NoError(t, os.WriteFile(logPath, []byte("this is not valid json\n"), 0o644))

	_, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.Error(t, err, "corrupt log must fail the load instead of skipping into D-checks")
	assert.Contains(t, err.Error(), "invalid JSONL line")
	var located *ops.ParseLoadError
	require.ErrorAs(t, err, &located)
	assert.Equal(t, 1, located.Line)
	assert.Contains(t, located.LogPath, "worker-clean.log")
	assert.Equal(t, "arm doctor quarantine --log worker-clean.log --line 1", located.RepairCommand())
}

func TestRun_CorruptLineReportsFileAndLine_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	logPath := filepath.Join(issuesDir, "ops", "worker-clean.log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 1, WorkerID: "worker-clean",
		Payload: ops.Payload{Title: "ok", NodeType: "task"},
	}))
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("not json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker-clean.log:2:")
	var located *ops.ParseLoadError
	require.ErrorAs(t, err, &located)
	assert.Equal(t, 2, located.Line)
	assert.Equal(t, "arm doctor quarantine --log worker-clean.log --line 2", located.RepairCommand())
}

func TestRun_Integration_D2_StaleClaims(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

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

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	d2 := findCheck(t, report, "D2")
	assert.Equal(t, SeverityWarning, d2.Severity)
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

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", true, time.Now())
	require.NoError(t, err)

	d3 := findCheck(t, report, "D3")
	assert.Equal(t, SeverityError, d3.Severity)

	assert.Contains(t, d3.Items, "ghost-verbose-01")

	require.NotEmpty(t, d3.VerboseItems)
	assert.Contains(t, d3.VerboseItems[0], "worker-verbose.log")
	assert.Contains(t, d3.VerboseItems[0], "ghost-verbose-01")
}

func TestRun_Integration_Verbose_CleanRepo_NoExtraOutput(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	workerLog := filepath.Join(issuesDir, "ops", "worker-clean.log")
	require.NoError(t, os.WriteFile(workerLog, []byte(""), 0644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", true, time.Now())
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

	workerLog := filepath.Join(issuesDir, "ops", "test-worker.log")
	require.NoError(t, os.WriteFile(workerLog, []byte(""), 0644))

	index := materialize.Index{
		"T-001": {Status: "open", Type: "task"},
	}
	indexPath := filepath.Join(stateDir, "index.json")
	require.NoError(t, materialize.WriteIndex(indexPath, index))

	report, err := Run(issuesDir, stateDir, "", "", false, time.Now())
	require.NoError(t, err)

	d4 := findCheck(t, report, "D4")
	assert.Equal(t, SeverityOK, d4.Severity)
}

func TestRunChecks_D1_GitDivergence(t *testing.T) {
	t.Parallel()

	repoDir := gittest.InitRepo(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}
	run("git", "commit", "--allow-empty", "-m", "feat(task-open-1): implement feature")

	index := materialize.Index{
		"task-open-1": {Status: "in-progress", Type: "task"},
	}
	report := runChecks(index, nil, repoDir, time.Now())
	d1 := findCheck(t, report, "D1")
	assert.Equal(t, SeverityWarning, d1.Severity, "D1 should warn when commit references non-done issue")
	assert.Contains(t, d1.Items[0], "task-open-1")
}

func TestRunChecks_D1_DoneIssue_NoWarning(t *testing.T) {
	t.Parallel()

	repoDir := gittest.InitRepo(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}
	run("git", "commit", "--allow-empty", "-m", "feat(task-done-1): implement feature")

	index := materialize.Index{
		"task-done-1": {Status: "done", Type: "task"},
	}
	report := runChecks(index, nil, repoDir, time.Now())
	d1 := findCheck(t, report, "D1")
	assert.Equal(t, SeverityOK, d1.Severity, "D1 should be OK when commit references done issue")
}

func TestRun_PhysicalLineUsedForD3Verbose(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)

	logPath := filepath.Join(issuesDir, "ops", "worker-physical.log")

	acceptedOp1 := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "orphan-1",
		Timestamp: 100,
		WorkerID:  "worker-physical",
		Payload:   ops.Payload{Msg: "First"},
	}
	require.NoError(t, ops.AppendOp(logPath, acceptedOp1))

	mismatchOp := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "orphan-2",
		Timestamp: 101,
		WorkerID:  "worker-wrong",
		Payload:   ops.Payload{Msg: "Mismatch"},
	}
	require.NoError(t, ops.AppendOp(logPath, mismatchOp))

	acceptedOp3 := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "orphan-3",
		Timestamp: 102,
		WorkerID:  "worker-physical",
		Payload:   ops.Payload{Msg: "Third"},
	}
	require.NoError(t, ops.AppendOp(logPath, acceptedOp3))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", true, time.Now())
	require.NoError(t, err)

	d3 := findCheck(t, report, "D3")
	assert.Equal(t, SeverityError, d3.Severity, "should have orphaned ops")
	assert.Contains(t, d3.Items, "orphan-1", "orphan-1 should be in items")
	assert.Contains(t, d3.Items, "orphan-3", "orphan-3 should be in items")

	require.NotEmpty(t, d3.VerboseItems, "verbose items should be present")

	verbose1Found := false
	verbose3Found := false
	for _, vi := range d3.VerboseItems {
		if strings.Contains(vi, "orphan-1") {
			verbose1Found = true

			assert.Contains(t, vi, "worker-physical.log:1", "orphan-1 should be reported at physical line 1")
		}
		if strings.Contains(vi, "orphan-3") {
			verbose3Found = true

			assert.Contains(t, vi, "worker-physical.log:3", "orphan-3 should be reported at physical line 3")
		}
	}
	assert.True(t, verbose1Found, "verbose item for orphan-1 should be present")
	assert.True(t, verbose3Found, "verbose item for orphan-3 should be present")
}
