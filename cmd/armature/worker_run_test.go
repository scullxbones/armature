package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
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
