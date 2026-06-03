package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/scullxbones/armature/internal/workerruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedReady struct {
	ids []string
	i   int
}

func (r *fixedReady) NextReady(_ context.Context) (string, bool, error) {
	if r.i >= len(r.ids) {
		return "", false, nil
	}
	id := r.ids[r.i]
	r.i++
	return id, true, nil
}

type fixedClaim struct{}

func (fixedClaim) Claim(_ context.Context, _ string) (bool, error) { return true, nil }

type fixedExec struct{}

func (fixedExec) Run(_ context.Context, _ string) error { return nil }

type captureExec struct {
	seen []string
}

func (c *captureExec) Run(_ context.Context, issueID string) error {
	c.seen = append(c.seen, issueID)
	return nil
}

var workerRuntimeFactoryMu sync.Mutex

func TestWorkerRunMaxTasksOneExecutesSingleTask(t *testing.T) {
	workerRuntimeFactoryMu.Lock()
	defer workerRuntimeFactoryMu.Unlock()

	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	prev := newWorkerRuntime
	newWorkerRuntime = func(*workerRuntimeDeps) *workerruntime.Runtime {
		return &workerruntime.Runtime{
			Ready: &fixedReady{ids: []string{"T1", "T2"}},
			Claim: fixedClaim{},
			Exec:  fixedExec{},
		}
	}
	t.Cleanup(func() { newWorkerRuntime = prev })

	root := newRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"worker", "run", "--repo", repo, "--max-tasks", "1", "--format", "json"})
	err = root.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "\"tasks_completed\":1")
}

func TestWorkerRunCommandSupportsSingleTaskDogfood(t *testing.T) {
	workerRuntimeFactoryMu.Lock()
	defer workerRuntimeFactoryMu.Unlock()

	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	prev := newWorkerRuntime
	newWorkerRuntime = func(*workerRuntimeDeps) *workerruntime.Runtime {
		return &workerruntime.Runtime{
			Ready: &fixedReady{ids: []string{"T1"}},
			Claim: fixedClaim{},
			Exec:  fixedExec{},
		}
	}
	t.Cleanup(func() { newWorkerRuntime = prev })

	root := newRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"worker", "run", "--repo", repo, "--max-tasks", "1", "--dry-run", "--format", "json"})
	err = root.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "\"tasks_completed\":1")
	assert.Contains(t, out.String(), "\"dry_run\":true")
}

func TestWorkerRunUsesRealAdaptersByDefault(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "story-1", "--type", "story", "--title", "Story")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-1", "--type", "task", "--title", "Task", "--parent", "story-1")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "story-1", "--to", "in-progress")
	require.NoError(t, err)

	prev := newWorkerRuntime
	t.Cleanup(func() { newWorkerRuntime = prev })

	out, err := runTrls(t, repo, "worker", "run", "--max-tasks", "1", "--dry-run", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, "\"tasks_completed\":1")
	assert.NotContains(t, out, "\"final_state\":\"idle\"")
}

func TestWorkerRunDryRunDoesNotPersistClaim(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "story-1", "--type", "story", "--title", "Story")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-1", "--type", "task", "--title", "Task", "--parent", "story-1")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "story-1", "--to", "in-progress")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "worker", "run", "--max-tasks", "1", "--dry-run", "--format", "json")
	require.NoError(t, err)

	stateDir := getTestStateDir(t, repo)
	data, err := os.ReadFile(filepath.Join(stateDir, "issues", "task-1.json"))
	require.NoError(t, err)
	var issue map[string]any
	require.NoError(t, json.Unmarshal(data, &issue))
	assert.Empty(t, issue["claimed_by"])
}

func TestWorkerRunSkipsInferredReadyEntries(t *testing.T) {
	originalLoader := runtimeIssueStateLoader
	runtimeIssueStateLoader = func(*config.Context) (materialize.Index, map[string]*materialize.Issue, error) {
		return materialize.Index{
				"task-1": {Type: "task", Status: "open", Title: "Inferred task"},
			}, map[string]*materialize.Issue{
				"task-1": {ID: "task-1", Provenance: materialize.Provenance{Confidence: "inferred"}},
			}, nil
	}
	t.Cleanup(func() { runtimeIssueStateLoader = originalLoader })

	r := &repoReadyProvider{logicalWorkerID: "worker-a"}
	issueID, ok, err := r.NextReady(context.Background())
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, issueID)
	assert.Equal(t, "requires_confirmation", r.IdleDiagnostics()["reason"])
}

func TestWorkerRunSortsAssignmentsByBaseWorkerIDWhenSlotted(t *testing.T) {
	workerRuntimeFactoryMu.Lock()
	defer workerRuntimeFactoryMu.Unlock()

	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	seen := &captureExec{}
	prev := newWorkerRuntime
	newWorkerRuntime = func(deps *workerRuntimeDeps) *workerruntime.Runtime {
		return &workerruntime.Runtime{
			Ready: &repoReadyProvider{
				ctx:             deps.state.ctx,
				workerID:        deps.workerID,
				logicalWorkerID: baseWorkerIdentity(deps.workerID),
			},
			Claim: fixedClaim{},
			Exec:  seen,
		}
	}
	t.Cleanup(func() { newWorkerRuntime = prev })

	_, err = runTrls(t, repo, "create", "--id", "story-1", "--type", "story", "--title", "Story")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-assigned", "--type", "task", "--title", "Assigned", "--parent", "story-1")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-unassigned", "--type", "task", "--title", "Unassigned", "--parent", "story-1")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "story-1", "--to", "in-progress")
	require.NoError(t, err)
	baseWorkerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	t.Setenv("ARM_LOG_SLOT", "s1")
	_, err = runTrls(t, repo, "assign", "--issue", "task-assigned", "--worker", baseWorkerID)
	require.NoError(t, err)
	seen.seen = nil

	_, err = runTrls(t, repo, "worker", "run", "--max-tasks", "1", "--dry-run")
	require.NoError(t, err)
	require.NotEmpty(t, seen.seen)
	assert.Equal(t, "task-assigned", seen.seen[0])
}

func TestWorkerRunSkipsReadyTaskWhenScopeConflicts(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-1", "--type", "task", "--title", "Task1", "--scope", "src/foo/*")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-2", "--type", "task", "--title", "Task2", "--scope", "src/foo/bar.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "task-1")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "worker", "run", "--max-tasks", "1", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, "\"tasks_completed\":0")
	assert.Contains(t, out, "\"reason\":\"scope_conflict\"")
}

func TestWorkerRunDryRunWithoutMaxTasksStopsAfterSingleSimulation(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "story-1", "--type", "story", "--title", "Story")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-1", "--type", "task", "--title", "Task", "--parent", "story-1")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "story-1", "--to", "in-progress")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "worker", "run", "--dry-run", "--max-runtime", "1s", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, "\"tasks_completed\":1")
	assert.Contains(t, out, "\"final_state\":\"stopped\"")
}

func TestHasActiveScopeOverlap_IgnoresStaleClaims(t *testing.T) {
	now := int64(1000)
	index := materialize.Index{
		"task-a": {Status: ops.StatusClaimed, Scope: []string{"cmd/armature/*"}},
	}
	issues := map[string]*materialize.Issue{
		"task-a": {
			ID:            "task-a",
			ClaimedAt:     1,
			LastHeartbeat: 1,
			ClaimTTL:      1,
		},
	}
	assert.False(t, hasActiveScopeOverlap("task-b", []string{"cmd/armature/main.go"}, index, issues, now))
}

func TestHasActiveScopeOverlap_IgnoresAncestorContainers(t *testing.T) {
	now := int64(1000)
	index := materialize.Index{
		"story-1": {Status: ops.StatusInProgress, Scope: []string{"cmd/armature/*"}},
	}
	issues := map[string]*materialize.Issue{
		"story-1": {ID: "story-1", ClaimedAt: 900, LastHeartbeat: 950, ClaimTTL: 60},
		"task-1":  {ID: "task-1", Parent: "story-1"},
	}
	assert.False(t, hasActiveScopeOverlap("task-1", []string{"cmd/armature/main.go"}, index, issues, now))
}

func TestRuntimeActiveScopes_FiltersAncestorAndStaleClaims(t *testing.T) {
	now := int64(1000)
	index := materialize.Index{
		"task-1":  {Status: ops.StatusClaimed, Scope: []string{"cmd/armature/*"}},
		"story-1": {Status: ops.StatusInProgress, Scope: []string{"cmd/armature/*"}},
		"task-2":  {Status: ops.StatusClaimed, Scope: []string{"internal/*"}},
		"task-3":  {Status: ops.StatusClaimed, Scope: []string{"pkg/*"}},
	}
	issues := map[string]*materialize.Issue{
		"story-1": {ID: "story-1"},
		"task-1":  {ID: "task-1", Parent: "story-1", ClaimedAt: 900, LastHeartbeat: 950, ClaimTTL: 60},
		"task-2":  {ID: "task-2", ClaimedAt: 10, LastHeartbeat: 10, ClaimTTL: 1},
		"task-3":  {ID: "task-3", ClaimedAt: 900, LastHeartbeat: 950, ClaimTTL: 60},
	}

	active := runtimeActiveScopes("task-1", index, issues, now)
	assert.NotContains(t, active, "story-1")
	assert.NotContains(t, active, "task-2")
	assert.Contains(t, active, "task-3")
}

// fakeBlockedRunner returns a RunResult with StatusBlocked lifecycle outcome.
type fakeBlockedRunner struct{}

func (fakeBlockedRunner) Run(_ context.Context, _ orchestrate.RunRequest) (orchestrate.RunResult, error) {
	return orchestrate.RunResult{
		Phase: "complete",
		LifecycleOutcome: orchestrate.LifecycleOutcome{
			Status:  ops.StatusBlocked,
			Outcome: "task blocked by dependency",
		},
	}, nil
}

// blockedExec wraps the blocked runner path through repoOrchestrator for
// testing via the workerruntime.Orchestrator interface.
type blockedExec struct {
	prevNewRepoRunner func(*config.Context, string) orchestrateRunner
}

func (b *blockedExec) install() {
	b.prevNewRepoRunner = newRepoRunner
	newRepoRunner = func(_ *config.Context, _ string) orchestrateRunner {
		return fakeBlockedRunner{}
	}
}

func (b *blockedExec) restore() { newRepoRunner = b.prevNewRepoRunner }

func TestRepoOrchestratorRunReturnsErrorOnBlockedLifecycleOutcome(t *testing.T) {
	be := &blockedExec{}
	be.install()
	defer be.restore()

	o := &repoOrchestrator{
		ctx:      &config.Context{StateDir: t.TempDir()},
		workerID: "w1",
		dryRun:   false,
	}

	// Create a minimal issue file so LoadIssue doesn't fail.
	issuesDir := filepath.Join(o.ctx.StateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	issueData := `{"id":"task-blocked","type":"task","title":"Blocked task","status":"in-progress"}`
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "task-blocked.json"), []byte(issueData), 0o644))

	err := o.Run(context.Background(), "task-blocked")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lifecycle blocked")
}

// blockedLifecycleExec simulates an Orchestrator whose task completes with a
// blocked lifecycle outcome (i.e. repoOrchestrator.Run returned an error).
type blockedLifecycleExec struct{}

func (blockedLifecycleExec) Run(_ context.Context, issueID string) error {
	return fmt.Errorf("task %s lifecycle blocked: task blocked by dependency", issueID)
}

func TestWorkerRuntimeDoesNotIncrementTasksCompletedForBlockedOutcome(t *testing.T) {
	workerRuntimeFactoryMu.Lock()
	defer workerRuntimeFactoryMu.Unlock()

	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	prev := newWorkerRuntime
	newWorkerRuntime = func(*workerRuntimeDeps) *workerruntime.Runtime {
		return &workerruntime.Runtime{
			Ready: &fixedReady{ids: []string{"task-blocked"}},
			Claim: fixedClaim{},
			Exec:  blockedLifecycleExec{},
		}
	}
	t.Cleanup(func() { newWorkerRuntime = prev })

	root := newRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"worker", "run", "--repo", repo, "--max-tasks", "1", "--format", "json"})
	// The blocked error escalates the runtime — no JSON output on error path.
	// The key invariant: tasks_completed is never incremented.
	execErr := root.Execute()
	require.Error(t, execErr)
	// No JSON output on the error path, but tasks_completed must not be 1.
	assert.NotContains(t, out.String(), "\"tasks_completed\":1")
}

func TestWorkerRunClaimDefaultsTTLWhenConfigMissing(t *testing.T) {
	workerRuntimeFactoryMu.Lock()
	defer workerRuntimeFactoryMu.Unlock()

	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Zero out default_ttl to simulate hand-written/legacy config.
	cfgPath := filepath.Join(repo, ".armature", "config.json")
	cfgData, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(cfgData, &cfg))
	cfg["default_ttl"] = float64(0)
	cfgOut, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, cfgOut, 0o644))

	_, err = runTrls(t, repo, "create", "--id", "task-1", "--type", "task", "--title", "Task1", "--scope", "src/foo.go")
	require.NoError(t, err)

	prev := newWorkerRuntime
	newWorkerRuntime = func(deps *workerRuntimeDeps) *workerruntime.Runtime {
		return &workerruntime.Runtime{
			Ready: &fixedReady{ids: []string{"task-1"}},
			Claim: &repoClaimer{state: deps.state, workerID: deps.workerID, logPath: deps.logPath},
			Exec:  fixedExec{},
		}
	}
	t.Cleanup(func() { newWorkerRuntime = prev })

	_, err = runTrls(t, repo, "worker", "run", "--max-tasks", "1", "--format", "json")
	require.NoError(t, err)

	stateDir := getTestStateDir(t, repo)
	issue, err := materialize.LoadIssue(filepath.Join(stateDir, "issues", "task-1.json"))
	require.NoError(t, err)
	assert.Equal(t, 60, issue.ClaimTTL)
}
