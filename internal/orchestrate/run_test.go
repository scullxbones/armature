package orchestrate

import (
	"context"
	"fmt"
	"testing"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

type repoRunnerHarness struct{}

func (repoRunnerHarness) Name() string { return "test" }
func (repoRunnerHarness) Run(context.Context, HarnessConfig, RunOptions) (CheckResult, error) {
	return CheckResult{Name: "test", Passed: true, Severity: SeverityInfo}, nil
}

type repoRunnerGit struct{ head string }

func (g *repoRunnerGit) HeadSHA() (string, error)              { return g.head, nil }
func (g *repoRunnerGit) DiffFrom(string) (string, error)       { return "", nil }
func (g *repoRunnerGit) DiffNameOnly(string) ([]string, error) { return nil, nil }
func (g *repoRunnerGit) ResetHard(string) error                { return nil }
func (g *repoRunnerGit) ApplyPatch([]byte) error               { return nil }
func (g *repoRunnerGit) AddAll() error                         { return nil }
func (g *repoRunnerGit) AddPaths([]string) error               { return nil }
func (g *repoRunnerGit) CommitWithMessage(string) error        { return nil }
func (g *repoRunnerGit) RemoveWorktree(string) error           { return nil }

func TestRepoRunner_DryRunAndMutatingRunSharePreparationPath(t *testing.T) {
	var materializeCalls, loadIssueCalls, loadIndexCalls, loadStateCalls int
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL:  60,
				TokenBudget: 1600,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				materializeCalls++
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				loadIssueCalls++
				return materialize.Issue{
					ID:             "ORCRUN-T02",
					Title:          "task",
					ClaimedBy:      "worker-a",
					Scope:          []string{"internal/orchestrate/run.go"},
					Acceptance:     []byte(`["go test ./internal/orchestrate"]`),
					PreferredModel: "",
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				loadIndexCalls++
				return materialize.Index{"ORCRUN-T02": {Status: ops.StatusClaimed}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				loadStateCalls++
				state := materialize.NewState()
				state.Issues["ORCRUN-T02"] = &materialize.Issue{ID: "ORCRUN-T02", Title: "task", Scope: []string{"internal/orchestrate/run.go"}}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				return OrchestrateState{Phase: "complete", Run: 1}, nil
			},
			nowUnix: func() int64 { return 100 },
		},
	}

	dryResult, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T02", WorkerID: "worker-a", Harness: "codex", DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !dryResult.DryRun {
		t.Fatalf("dry result should report dry run")
	}

	materializeCalls = 0
	loadIssueCalls = 0
	loadIndexCalls = 0
	loadStateCalls = 0

	liveResult, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T02", WorkerID: "worker-a", Harness: "codex"})
	if err != nil {
		t.Fatalf("live run: %v", err)
	}
	if liveResult.Phase != "complete" {
		t.Fatalf("Phase = %q, want complete", liveResult.Phase)
	}
	// loadState is called three times on a live run (already-owned claim path):
	// once to build the issue map for scope-conflict filtering, once to assemble
	// the task context for the harness, and once for the post-prep scope refresh.
	if materializeCalls != 2 || loadIssueCalls != 1 || loadIndexCalls != 2 || loadStateCalls != 3 {
		t.Fatalf("prep path counts = %d/%d/%d/%d, want 2/1/2/3", materializeCalls, loadIssueCalls, loadIndexCalls, loadStateCalls)
	}
}

func TestRepoRunner_DryRunSkipsAuthAndHarnessResolution(t *testing.T) {
	authCalled := false
	harnessCalled := false
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL:  60,
				TokenBudget: 1600,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				return materialize.Issue{
					ID:         "ORCRUN-T02",
					Title:      "task",
					ClaimedBy:  "worker-a",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-T02": {Status: ops.StatusClaimed}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T02"] = &materialize.Issue{ID: "ORCRUN-T02", Title: "task", Scope: []string{"internal/orchestrate/run.go"}}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				authCalled = true
				return AuthPlan{}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				harnessCalled = true
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				t.Fatal("execute should not run for dry-run")
				return OrchestrateState{}, nil
			},
			nowUnix: func() int64 { return 100 },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T02", WorkerID: "worker-a", Harness: "claude", DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if authCalled {
		t.Fatal("dry-run should not resolve harness auth")
	}
	if harnessCalled {
		t.Fatal("dry-run should not construct harness adapter")
	}
	if result.Harness != "claude" {
		t.Fatalf("Harness = %q, want claude", result.Harness)
	}
}

func TestRepoRunner_ClaimsTaskWhenNeeded(t *testing.T) {
	claimed := false
	var appended []ops.Op
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog: func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(_ string, _ string, op ops.Op, _ ops.GitCommitter) error {
				appended = append(appended, op)
				claimed = true
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				issue := materialize.Issue{
					ID:         "ORCRUN-T02",
					Title:      "task",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}
				if claimed {
					issue.ClaimedBy = "worker-a"
				}
				return issue, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-T02": {Status: ops.StatusOpen}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T02"] = &materialize.Issue{ID: "ORCRUN-T02", Title: "task", Scope: []string{"internal/orchestrate/run.go"}}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				return OrchestrateState{Phase: "complete", Run: 1}, nil
			},
			nowUnix: func() int64 { return 100 },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T02", WorkerID: "worker-a", Harness: "codex"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(appended) != 1 || appended[0].Type != ops.OpClaim {
		t.Fatalf("expected claim op, got %+v", appended)
	}
	if !result.ClaimHeld || result.ClaimOwner != "worker-a" {
		t.Fatalf("claim state = held:%t owner:%q", result.ClaimHeld, result.ClaimOwner)
	}
}

func TestRepoRunner_StaleClaimAllowsTakeover(t *testing.T) {
	// A claim held by another worker that has exceeded its TTL is stale.
	// prepare() should allow this worker to take over rather than blocking forever.
	const (
		claimedAt     = int64(1000)
		ttlMinutes    = 1                                    // 60 seconds
		staleNow      = claimedAt + int64(ttlMinutes)*60 + 1 // one second past TTL
		lastHeartbeat = int64(0)                             // no heartbeat
	)

	var appended []ops.Op
	claimed := false
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: ttlMinutes,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-b",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog: func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(_ string, _ string, op ops.Op, _ ops.GitCommitter) error {
				appended = append(appended, op)
				if op.Type == ops.OpClaim {
					claimed = true
				}
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				issue := materialize.Issue{
					ID:         "ORCRUN-T03",
					Title:      "task",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
					// Stale claim held by another worker
					ClaimedBy:     "worker-a",
					ClaimedAt:     claimedAt,
					ClaimTTL:      ttlMinutes,
					LastHeartbeat: lastHeartbeat,
				}
				if claimed {
					issue.ClaimedBy = "worker-b"
				}
				return issue, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-T03": {Status: ops.StatusClaimed}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T03"] = &materialize.Issue{ID: "ORCRUN-T03", Title: "task", Scope: []string{"internal/orchestrate/run.go"}}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				return OrchestrateState{Phase: "complete", Run: 1}, nil
			},
			nowUnix: func() int64 { return staleNow },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T03", WorkerID: "worker-b", Harness: "codex"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason != "" {
		t.Fatalf("BlockedReason = %q, want empty (stale claim should allow takeover)", result.BlockedReason)
	}
	if !result.ClaimHeld || result.ClaimOwner != "worker-b" {
		t.Fatalf("claim state = held:%t owner:%q, want held:true owner:worker-b", result.ClaimHeld, result.ClaimOwner)
	}
	if len(appended) != 1 || appended[0].Type != ops.OpClaim {
		t.Fatalf("expected one claim op for takeover, got %+v", appended)
	}
	if result.Phase != "complete" {
		t.Fatalf("Phase = %q, want complete", result.Phase)
	}
}

func TestRepoRunner_FreshClaimByOtherWorkerStillBlocks(t *testing.T) {
	// A claim held by another worker that is still within TTL must block.
	const (
		claimedAt  = int64(1000)
		ttlMinutes = 1
		freshNow   = claimedAt + 10 // well within TTL
	)

	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: ttlMinutes,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-b",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				return materialize.Issue{
					ID:            "ORCRUN-T04",
					Title:         "task",
					Scope:         []string{"internal/orchestrate/run.go"},
					Acceptance:    []byte(`["go test ./internal/orchestrate"]`),
					ClaimedBy:     "worker-a",
					ClaimedAt:     claimedAt,
					ClaimTTL:      ttlMinutes,
					LastHeartbeat: int64(0),
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-T04": {Status: ops.StatusClaimed}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				return materialize.NewState(), nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) { return AuthPlan{}, nil },
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				t.Fatal("execute should not run when claim is fresh")
				return OrchestrateState{}, nil
			},
			nowUnix: func() int64 { return freshNow },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T04", WorkerID: "worker-b", Harness: "codex"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason == "" {
		t.Fatal("BlockedReason should be set when claim is fresh and held by another worker")
	}
	if result.WouldDispatch {
		t.Fatal("WouldDispatch should be false")
	}
}

func TestRepoRunner_ActiveScopesSkipsStaleClaimsFromOtherTasks(t *testing.T) {
	// A scope-conflicting task with a stale claim should NOT block dispatch.
	// If the activeScopes loop does not skip stale claims, this test will
	// incorrectly see a scope conflict and set BlockedReason = "scope conflict".
	const (
		otherClaimedAt = int64(1000)
		otherTTL       = 1 // 60 seconds
		staleNow       = otherClaimedAt + int64(otherTTL)*60 + 1
	)

	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL:  60,
				TokenBudget: 1600,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				return materialize.Issue{
					ID:         "ORCRUN-T05",
					Title:      "task",
					ClaimedBy:  "worker-a",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{
					"ORCRUN-T05": {Status: ops.StatusClaimed, Scope: []string{"internal/orchestrate/run.go"}},
					// OTHER has overlapping scope but a stale claim — should be skipped
					"OTHER-STALE": {Status: ops.StatusClaimed, Scope: []string{"internal/orchestrate/run.go"}},
				}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T05"] = &materialize.Issue{
					ID:    "ORCRUN-T05",
					Title: "task",
					Scope: []string{"internal/orchestrate/run.go"},
				}
				state.Issues["OTHER-STALE"] = &materialize.Issue{
					ID:            "OTHER-STALE",
					Title:         "stale task",
					Scope:         []string{"internal/orchestrate/run.go"},
					ClaimedBy:     "worker-b",
					ClaimedAt:     otherClaimedAt,
					ClaimTTL:      otherTTL,
					LastHeartbeat: 0,
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				return OrchestrateState{Phase: "complete", Run: 1}, nil
			},
			nowUnix: func() int64 { return staleNow },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T05", WorkerID: "worker-a", Harness: "codex"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason != "" {
		t.Fatalf("BlockedReason = %q, want empty: stale-claimed conflicting task should not block dispatch", result.BlockedReason)
	}
	if len(result.ScopeConflicts) != 0 {
		t.Fatalf("ScopeConflicts = %+v, want none: stale claims should not appear as conflicts", result.ScopeConflicts)
	}
}

func TestRepoRunner_ActiveScopesSkipsAncestorIssues(t *testing.T) {
	// A scope-conflicting task that is an ancestor of the task being dispatched
	// should NOT block dispatch — parent tasks legitimately share child scope.
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL:  60,
				TokenBudget: 1600,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				return materialize.Issue{
					ID:         "ORCRUN-CHILD",
					Title:      "child task",
					ClaimedBy:  "worker-a",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
					Parent:     "ORCRUN-PARENT",
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{
					"ORCRUN-CHILD": {Status: ops.StatusClaimed, Scope: []string{"internal/orchestrate/run.go"}, Parent: "ORCRUN-PARENT"},
					// PARENT has overlapping scope and is in-progress, but is an ancestor — should be skipped
					"ORCRUN-PARENT": {Status: ops.StatusInProgress, Scope: []string{"internal/orchestrate/run.go"}, Parent: ""},
				}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-CHILD"] = &materialize.Issue{
					ID:     "ORCRUN-CHILD",
					Title:  "child task",
					Scope:  []string{"internal/orchestrate/run.go"},
					Parent: "ORCRUN-PARENT",
				}
				state.Issues["ORCRUN-PARENT"] = &materialize.Issue{
					ID:    "ORCRUN-PARENT",
					Title: "parent task",
					Scope: []string{"internal/orchestrate/run.go"},
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				return OrchestrateState{Phase: "complete", Run: 1}, nil
			},
			nowUnix: func() int64 { return 100 },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-CHILD", WorkerID: "worker-a", Harness: "codex"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason != "" {
		t.Fatalf("BlockedReason = %q, want empty: ancestor issue should not block dispatch", result.BlockedReason)
	}
	if len(result.ScopeConflicts) != 0 {
		t.Fatalf("ScopeConflicts = %+v, want none: ancestor issues should not appear as conflicts", result.ScopeConflicts)
	}
}

func TestRepoRunner_DualBranchPushesClaimOpAfterAppendAndCommit(t *testing.T) {
	// In dual-branch mode (WorktreePath set), prepare() must push the claim op
	// to _armature after AppendAndCommit so other workers observe it immediately.
	var appended []ops.Op
	pushCalled := false
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:     t.TempDir(),
			IssuesDir:    t.TempDir(),
			StateDir:     t.TempDir(),
			WorktreePath: t.TempDir(), // non-empty == dual-branch mode
			Mode:         "dual-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog: func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(_ string, _ string, op ops.Op, _ ops.GitCommitter) error {
				appended = append(appended, op)
				return nil
			},
			pushClaimOp: func(worktreePath string) error {
				pushCalled = true
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				issue := materialize.Issue{
					ID:         "ORCRUN-DUAL-T01",
					Title:      "dual-branch task",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}
				if len(appended) > 0 {
					issue.ClaimedBy = "worker-a"
				}
				return issue, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-DUAL-T01": {Status: ops.StatusOpen}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-DUAL-T01"] = &materialize.Issue{
					ID:    "ORCRUN-DUAL-T01",
					Title: "dual-branch task",
					Scope: []string{"internal/orchestrate/run.go"},
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				return OrchestrateState{Phase: "complete", Run: 1}, nil
			},
			nowUnix: func() int64 { return 100 },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-DUAL-T01", WorkerID: "worker-a", Harness: "codex"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(appended) != 1 || appended[0].Type != ops.OpClaim {
		t.Fatalf("expected claim op, got %+v", appended)
	}
	if !pushCalled {
		t.Fatal("pushClaimOp was not called in dual-branch mode after claim AppendAndCommit")
	}
	if !result.ClaimHeld || result.ClaimOwner != "worker-a" {
		t.Fatalf("claim state = held:%t owner:%q", result.ClaimHeld, result.ClaimOwner)
	}
}

func TestRepoRunner_SingleBranchDoesNotPushClaimOp(t *testing.T) {
	// In single-branch mode (WorktreePath empty), pushClaimOp must NOT be called.
	pushCalled := false
	claimed := false
	var appended []ops.Op
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			// WorktreePath intentionally empty
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog: func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(_ string, _ string, op ops.Op, _ ops.GitCommitter) error {
				appended = append(appended, op)
				claimed = true
				return nil
			},
			pushClaimOp: func(worktreePath string) error {
				pushCalled = true
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				issue := materialize.Issue{
					ID:         "ORCRUN-SB-T01",
					Title:      "single-branch task",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}
				if claimed {
					issue.ClaimedBy = "worker-a"
				}
				return issue, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-SB-T01": {Status: ops.StatusOpen}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-SB-T01"] = &materialize.Issue{
					ID:    "ORCRUN-SB-T01",
					Title: "single-branch task",
					Scope: []string{"internal/orchestrate/run.go"},
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				return OrchestrateState{Phase: "complete", Run: 1}, nil
			},
			nowUnix: func() int64 { return 100 },
		},
	}

	_, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-SB-T01", WorkerID: "worker-a", Harness: "codex"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if pushCalled {
		t.Fatal("pushClaimOp should not be called in single-branch mode")
	}
}

func TestRepoRunner_GlobScopeConflictBlocksDispatch(t *testing.T) {
	// Validates that glob patterns like "src/*.go" match concrete paths like
	// "src/foo.go" — the old prefix-matching scopesOverlap would not catch this.
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				// This task has a concrete path scope
				return materialize.Issue{
					ID:        "ORCRUN-GLOB-NEW",
					Title:     "new task",
					ClaimedBy: "worker-a",
					// Concrete file path — should match another task's glob pattern
					Scope:      []string{"src/foo.go"},
					Acceptance: []byte(`["go test ./..."]`),
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{
					"ORCRUN-GLOB-NEW": {Status: ops.StatusClaimed, Scope: []string{"src/foo.go"}},
					// OTHER has a glob-pattern scope that matches src/foo.go
					"ORCRUN-GLOB-OTHER": {Status: ops.StatusClaimed, Scope: []string{"src/*.go"}},
				}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-GLOB-NEW"] = &materialize.Issue{
					ID:    "ORCRUN-GLOB-NEW",
					Title: "new task",
					Scope: []string{"src/foo.go"},
				}
				state.Issues["ORCRUN-GLOB-OTHER"] = &materialize.Issue{
					ID:        "ORCRUN-GLOB-OTHER",
					Title:     "other task with glob",
					Scope:     []string{"src/*.go"},
					ClaimedBy: "worker-b",
					ClaimedAt: 100,
					ClaimTTL:  60,
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				t.Fatal("execute should not run when scope is blocked by glob match")
				return OrchestrateState{}, nil
			},
			nowUnix: func() int64 { return 110 }, // within TTL of the other task's claim
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{
		TaskID:   "ORCRUN-GLOB-NEW",
		WorkerID: "worker-a",
		Harness:  "codex",
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason != "scope conflict" {
		t.Fatalf("BlockedReason = %q, want 'scope conflict': glob pattern src/*.go should match concrete path src/foo.go", result.BlockedReason)
	}
	if result.WouldDispatch {
		t.Fatal("WouldDispatch should be false when scope is blocked by glob match")
	}
	if len(result.ScopeConflicts) != 1 || result.ScopeConflicts[0].TaskID != "ORCRUN-GLOB-OTHER" {
		t.Fatalf("ScopeConflicts = %+v, want one conflict with ORCRUN-GLOB-OTHER", result.ScopeConflicts)
	}
}

func TestRepoRunner_BlocksOnScopeConflictAfterClaimVerification(t *testing.T) {
	claimed := false
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog: func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(_ string, _ string, op ops.Op, _ ops.GitCommitter) error {
				if op.Type == ops.OpClaim {
					claimed = true
				}
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				issue := materialize.Issue{
					ID:         "ORCRUN-T02",
					Title:      "task",
					Scope:      []string{"internal/orchestrate/"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}
				if claimed {
					issue.ClaimedBy = "worker-a"
				}
				return issue, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{
					"ORCRUN-T02": {Status: ops.StatusClaimed, Scope: []string{"internal/orchestrate/"}},
					"OTHER":      {Status: ops.StatusClaimed, Scope: []string{"internal/orchestrate/run.go"}},
				}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T02"] = &materialize.Issue{ID: "ORCRUN-T02", Title: "task", Scope: []string{"internal/orchestrate/"}}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				t.Fatal("execute should not run when scope is blocked")
				return OrchestrateState{}, nil
			},
			nowUnix: func() int64 { return 100 },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T02", WorkerID: "worker-a", Harness: "codex"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason != "scope conflict" {
		t.Fatalf("BlockedReason = %q, want scope conflict", result.BlockedReason)
	}
	if result.WouldDispatch {
		t.Fatal("WouldDispatch should be false when scope is blocked")
	}
	if len(result.ScopeConflicts) != 1 || result.ScopeConflicts[0].TaskID != "OTHER" {
		t.Fatalf("ScopeConflicts = %+v", result.ScopeConflicts)
	}
}

// Fix 4: TestRepoRunner_ZeroTTLClaimStaleAllowsTakeover verifies that when another
// worker holds a claim with ClaimTTL = 0, and current time is past the default TTL,
// the current worker can take over (should NOT block forever).
func TestRepoRunner_ZeroTTLClaimStaleAllowsTakeover(t *testing.T) {
	const (
		claimedAt     = int64(1000)
		defaultTTL    = 60                                   // minutes (default TTL when ClaimTTL == 0)
		staleNow      = claimedAt + int64(defaultTTL)*60 + 1 // one second past 60-minute default TTL
		lastHeartbeat = int64(0)                             // no heartbeat
	)

	var appended []ops.Op
	claimed := false
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: 0, // no config default — should fall back to 60
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-b",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog: func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(_ string, _ string, op ops.Op, _ ops.GitCommitter) error {
				appended = append(appended, op)
				if op.Type == ops.OpClaim {
					claimed = true
				}
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				issue := materialize.Issue{
					ID:         "ORCRUN-T04",
					Title:      "task",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
					// Claim held by another worker with ZERO TTL
					ClaimedBy:     "worker-a",
					ClaimedAt:     claimedAt,
					ClaimTTL:      0, // zero TTL — should use default
					LastHeartbeat: lastHeartbeat,
				}
				if claimed {
					issue.ClaimedBy = "worker-b"
				}
				return issue, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-T04": {Status: ops.StatusClaimed}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T04"] = &materialize.Issue{ID: "ORCRUN-T04", Title: "task", Scope: []string{"internal/orchestrate/run.go"}}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				return OrchestrateState{Phase: "complete", Run: 1}, nil
			},
			nowUnix: func() int64 { return staleNow },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T04", WorkerID: "worker-b", Harness: "codex"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Should allow takeover, not block
	if result.BlockedReason != "" {
		t.Fatalf("BlockedReason = %q, want empty (zero-TTL stale claim should allow takeover)", result.BlockedReason)
	}
	if !result.ClaimHeld || result.ClaimOwner != "worker-b" {
		t.Fatalf("claim state = held:%t owner:%q, want held:true owner:worker-b", result.ClaimHeld, result.ClaimOwner)
	}
	if len(appended) < 1 || appended[0].Type != ops.OpClaim {
		t.Fatalf("expected claim op for takeover, got %+v", appended)
	}
}

// Fix 5: TestRepoRunner_PrepFailureBeforeClaimDoesNotLeaveClaim verifies that when
// resolveAuthPlan returns an error (prep step), no OpClaim is appended to the log.
func TestRepoRunner_PrepFailureBeforeClaimDoesNotLeaveClaim(t *testing.T) {
	var appended []ops.Op
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog: func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(_ string, _ string, op ops.Op, _ ops.GitCommitter) error {
				appended = append(appended, op)
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				return materialize.Issue{
					ID:         "ORCRUN-T05",
					Title:      "task",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-T05": {Status: ops.StatusOpen}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T05"] = &materialize.Issue{ID: "ORCRUN-T05", Title: "task", Scope: []string{"internal/orchestrate/run.go"}}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{}, fmt.Errorf("auth plan failed: credentials not found")
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				t.Fatal("execute should not run when prep fails")
				return OrchestrateState{}, nil
			},
			nowUnix: func() int64 { return 100 },
		},
	}

	_, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T05", WorkerID: "worker-a", Harness: "codex"})
	if err == nil {
		t.Fatal("expected error when resolveAuthPlan fails")
	}
	// Critical: no OpClaim should have been written before the failure
	for _, op := range appended {
		if op.Type == ops.OpClaim {
			t.Errorf("OpClaim was appended before prep failure — claim leaked: %+v", op)
		}
	}
}

// P1-4: TestRepoRunner_UnclaimedInProgressIsNotTreatedAsStale verifies that an
// issue in in-progress status with no claim (ClaimedAt==0) is treated as always
// active and contributes to scope conflict detection.
func TestRepoRunner_UnclaimedInProgressIsNotTreatedAsStale(t *testing.T) {
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				return materialize.Issue{
					ID:         "ORCRUN-T06",
					Title:      "task",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{
					"ORCRUN-T06":      {Status: ops.StatusOpen, Scope: []string{"internal/orchestrate/run.go"}},
					"OTHER-UNCLAIMED": {Status: ops.StatusInProgress, Scope: []string{"internal/orchestrate/run.go"}},
				}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T06"] = &materialize.Issue{
					ID:    "ORCRUN-T06",
					Title: "task",
					Scope: []string{"internal/orchestrate/run.go"},
				}
				// Manually transitioned to in-progress: no claim owner, no timestamps.
				state.Issues["OTHER-UNCLAIMED"] = &materialize.Issue{
					ID:            "OTHER-UNCLAIMED",
					Title:         "manual in-progress task",
					Scope:         []string{"internal/orchestrate/run.go"},
					ClaimedBy:     "",
					ClaimedAt:     0,
					ClaimTTL:      0,
					LastHeartbeat: 0,
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				t.Fatal("execute should not run when scope is blocked")
				return OrchestrateState{}, nil
			},
			nowUnix: func() int64 { return 9_999_999_999 },
		},
	}

	// DryRun=true so the test only exercises the pre-claim scope check path.
	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T06", WorkerID: "worker-a", DryRun: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason != "scope conflict" {
		t.Fatalf("BlockedReason = %q, want scope conflict: unclaimed in-progress task should block dispatch", result.BlockedReason)
	}
	if len(result.ScopeConflicts) == 0 || result.ScopeConflicts[0].TaskID != "OTHER-UNCLAIMED" {
		t.Fatalf("ScopeConflicts = %+v, want conflict with OTHER-UNCLAIMED", result.ScopeConflicts)
	}
}

// P1-1: TestRepoRunner_LifecycleOutcomeRequiresTransitionWritten verifies that when
// execute returns phase=complete but TransitionWritten=false, no lifecycle outcome is set.
func TestRepoRunner_LifecycleOutcomeRequiresTransitionWritten(t *testing.T) {
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				return materialize.Issue{
					ID:         "ORCRUN-T07",
					Title:      "task",
					ClaimedBy:  "worker-a",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-T07": {Status: ops.StatusClaimed}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T07"] = &materialize.Issue{ID: "ORCRUN-T07", Title: "task", Scope: []string{"internal/orchestrate/run.go"}}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				// Phase complete but transition was NOT durably written.
				return OrchestrateState{Phase: "complete", Run: 1, TransitionWritten: false}, nil
			},
			nowUnix: func() int64 { return 100 },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T07", WorkerID: "worker-a"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.LifecycleOutcome.Status != "" {
		t.Fatalf("LifecycleOutcome.Status = %q, want empty when TransitionWritten=false", result.LifecycleOutcome.Status)
	}
}

// P1-2: TestRepoRunner_ScopeConflictDetectedAfterClaiming verifies that when a
// competing task acquires scope overlap during the claim window, the post-claim
// scope re-check detects the conflict and blocks dispatch.
func TestRepoRunner_ScopeConflictDetectedAfterClaiming(t *testing.T) {
	claimed := false
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog: func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(_ string, _ string, op ops.Op, _ ops.GitCommitter) error {
				if op.Type == ops.OpClaim {
					claimed = true
				}
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				issue := materialize.Issue{
					ID:         "ORCRUN-T08",
					Title:      "task",
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./internal/orchestrate"]`),
				}
				if claimed {
					issue.ClaimedBy = "worker-a"
				}
				return issue, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				if claimed {
					// Post-claim: competitor has appeared with overlapping scope.
					return materialize.Index{
						"ORCRUN-T08": {Status: ops.StatusOpen, Scope: []string{"internal/orchestrate/run.go"}},
						"COMPETITOR": {Status: ops.StatusClaimed, Scope: []string{"internal/orchestrate/run.go"}},
					}, nil
				}
				return materialize.Index{
					"ORCRUN-T08": {Status: ops.StatusOpen, Scope: []string{"internal/orchestrate/run.go"}},
				}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T08"] = &materialize.Issue{
					ID:    "ORCRUN-T08",
					Title: "task",
					Scope: []string{"internal/orchestrate/run.go"},
				}
				if claimed {
					state.Issues["COMPETITOR"] = &materialize.Issue{
						ID:        "COMPETITOR",
						Title:     "competitor task",
						Scope:     []string{"internal/orchestrate/run.go"},
						ClaimedBy: "worker-b",
						ClaimedAt: 1000,
						ClaimTTL:  60,
					}
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				t.Fatal("execute should not run when post-claim scope conflict is detected")
				return OrchestrateState{}, nil
			},
			// nowUnix between claimedAt(1000) and TTL expiry(1000+3600=4600).
			nowUnix: func() int64 { return 2000 },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T08", WorkerID: "worker-a"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason != "scope conflict" {
		t.Fatalf("BlockedReason = %q, want scope conflict after claiming", result.BlockedReason)
	}
	if len(result.ScopeConflicts) == 0 || result.ScopeConflicts[0].TaskID != "COMPETITOR" {
		t.Fatalf("ScopeConflicts = %+v, want conflict with COMPETITOR", result.ScopeConflicts)
	}
}

// P1-8: TestRepoRunner_StaleOwnClaimIsReacquired verifies that when this worker holds a
// claim whose TTL has expired, the runner reacquires it rather than proceeding with a
// possibly-contested stale claim.
func TestRepoRunner_StaleOwnClaimIsReacquired(t *testing.T) {
	const (
		claimedAt  = int64(1000)
		ttlMinutes = 1
		staleNow   = claimedAt + int64(ttlMinutes)*60 + 1
	)

	var appended []ops.Op
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: ttlMinutes,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog: func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(_ string, _ string, op ops.Op, _ ops.GitCommitter) error {
				appended = append(appended, op)
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				issue := materialize.Issue{
					ID:            "ORCRUN-STALE-OWN",
					Title:         "task",
					Scope:         []string{"internal/orchestrate/run.go"},
					Acceptance:    []byte(`["go test ./..."]`),
					ClaimedBy:     "worker-a",
					ClaimedAt:     claimedAt,
					ClaimTTL:      ttlMinutes,
					LastHeartbeat: 0,
				}
				return issue, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-STALE-OWN": {Status: ops.StatusClaimed}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-STALE-OWN"] = &materialize.Issue{
					ID: "ORCRUN-STALE-OWN", Title: "task", Scope: []string{"internal/orchestrate/run.go"},
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				return OrchestrateState{Phase: "complete", Run: 1}, nil
			},
			nowUnix: func() int64 { return staleNow },
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-STALE-OWN", WorkerID: "worker-a"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason != "" {
		t.Fatalf("BlockedReason = %q, want empty: stale own claim should be reacquired, not blocked", result.BlockedReason)
	}
	// A claim op must have been written to renew the expired claim.
	hasClaim := false
	for _, op := range appended {
		if op.Type == ops.OpClaim {
			hasClaim = true
		}
	}
	if !hasClaim {
		t.Fatal("expected OpClaim to reacquire stale own claim")
	}
}

// P1-7: TestRepoRunner_ScopeConflictRefreshedWhenClaimAlreadyOwned verifies that when
// a worker already owns the task claim, a scope conflict introduced by another worker
// during context/auth preparation is detected by the post-prep scope refresh.
func TestRepoRunner_ScopeConflictRefreshedWhenClaimAlreadyOwned(t *testing.T) {
	var loadIndexCalls int
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:  t.TempDir(),
			IssuesDir: t.TempDir(),
			StateDir:  t.TempDir(),
			Mode:      "single-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				return materialize.Issue{
					ID:         "ORCRUN-T09",
					Title:      "task",
					ClaimedBy:  "worker-a",
					ClaimedAt:  100,
					ClaimTTL:   60,
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./..."]`),
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				loadIndexCalls++
				if loadIndexCalls >= 2 {
					// Second call: competitor has claimed overlapping scope during prep.
					return materialize.Index{
						"ORCRUN-T09": {Status: ops.StatusClaimed, Scope: []string{"internal/orchestrate/run.go"}},
						"COMPETITOR": {Status: ops.StatusClaimed, Scope: []string{"internal/orchestrate/run.go"}},
					}, nil
				}
				// First call: no conflicts yet.
				return materialize.Index{
					"ORCRUN-T09": {Status: ops.StatusClaimed, Scope: []string{"internal/orchestrate/run.go"}},
				}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-T09"] = &materialize.Issue{
					ID: "ORCRUN-T09", Title: "task", Scope: []string{"internal/orchestrate/run.go"},
					ClaimedBy: "worker-a", ClaimedAt: 100, ClaimTTL: 60,
				}
				state.Issues["COMPETITOR"] = &materialize.Issue{
					ID: "COMPETITOR", Title: "competitor", Scope: []string{"internal/orchestrate/run.go"},
					ClaimedBy: "worker-b", ClaimedAt: 200, ClaimTTL: 60,
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error) {
				t.Fatal("execute should not run when post-prep scope conflict is detected")
				return OrchestrateState{}, nil
			},
			nowUnix: func() int64 { return 250 }, // within TTL of competitor (200+3600)
		},
	}

	result, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-T09", WorkerID: "worker-a"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.BlockedReason != "scope conflict" {
		t.Fatalf("BlockedReason = %q, want scope conflict: already-owned claim must re-check scopes after prep", result.BlockedReason)
	}
	if len(result.ScopeConflicts) == 0 || result.ScopeConflicts[0].TaskID != "COMPETITOR" {
		t.Fatalf("ScopeConflicts = %+v, want conflict with COMPETITOR", result.ScopeConflicts)
	}
}

// P1-6: TestRepoRunner_DualBranchPushesHeartbeatOpDuringExecution verifies that in
// dual-branch mode, each OpHeartbeat appended to the op log is also pushed to _armature
// so remote workers can observe the claim renewal.
func TestRepoRunner_DualBranchPushesHeartbeatOpDuringExecution(t *testing.T) {
	var pushCount int
	runner := &RepoRunner{
		appCtx: &config.Context{
			RepoPath:     t.TempDir(),
			IssuesDir:    t.TempDir(),
			StateDir:     t.TempDir(),
			WorktreePath: t.TempDir(), // non-empty == dual-branch mode
			Mode:         "dual-branch",
			Config: config.Config{
				DefaultTTL: 60,
				Orchestrator: config.OrchestratorConfig{
					DefaultModel: "gpt-test",
				},
			},
		},
		workerID: "worker-a",
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: func(string) ([]ops.Op, map[string]int64, error) {
				return nil, map[string]int64{}, nil
			},
			readLog:         func(string) ([]ops.Op, error) { return nil, nil },
			appendAndCommit: func(string, string, ops.Op, ops.GitCommitter) error { return nil },
			pushClaimOp: func(string) error {
				pushCount++
				return nil
			},
			materialize: func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error) {
				return materialize.Result{}, nil
			},
			loadIssue: func(string) (materialize.Issue, error) {
				return materialize.Issue{
					ID:         "ORCRUN-HB-T01",
					Title:      "task",
					ClaimedBy:  "worker-a",
					ClaimedAt:  100,
					ClaimTTL:   60,
					Scope:      []string{"internal/orchestrate/run.go"},
					Acceptance: []byte(`["go test ./..."]`),
				}, nil
			},
			loadIndex: func(string) (materialize.Index, error) {
				return materialize.Index{"ORCRUN-HB-T01": {Status: ops.StatusClaimed}}, nil
			},
			loadState: func(string) (*materialize.State, error) {
				state := materialize.NewState()
				state.Issues["ORCRUN-HB-T01"] = &materialize.Issue{
					ID: "ORCRUN-HB-T01", Scope: []string{"internal/orchestrate/run.go"},
				}
				return state, nil
			},
			resolveAuthPlan: func(string, AuthConfig) (AuthPlan, error) {
				return AuthPlan{Source: "oauth-session"}, nil
			},
			newHarnessAdapter: func(HarnessConfig) (HarnessAdapter, error) {
				return repoRunnerHarness{}, nil
			},
			newGitClient: func(string) GitClient { return &repoRunnerGit{head: "abc123"} },
			execute: func(_ context.Context, svcCfg ServiceConfig, _ RunInput) (OrchestrateState, error) {
				// Simulate the engine writing a heartbeat op during harness execution.
				_ = svcCfg.OpLog.Append(ops.Op{
					Type:     ops.OpHeartbeat,
					TargetID: "ORCRUN-HB-T01",
					WorkerID: "worker-a",
				})
				return OrchestrateState{Phase: "complete", Run: 1, TransitionWritten: true}, nil
			},
			nowUnix: func() int64 { return 150 }, // within TTL of own claim (100+3600)
		},
	}

	_, err := runner.Run(context.Background(), RunRequest{TaskID: "ORCRUN-HB-T01", WorkerID: "worker-a"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if pushCount == 0 {
		t.Fatal("pushClaimOp was not called for heartbeat op in dual-branch mode")
	}
}
