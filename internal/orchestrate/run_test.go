package orchestrate

import (
	"context"
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
	// loadState is called twice on a live run: once to build the issue map for
	// scope-conflict filtering (stale-claim and ancestor checks) and once to
	// assemble the task context for the harness.
	if materializeCalls != 1 || loadIssueCalls != 1 || loadIndexCalls != 1 || loadStateCalls != 2 {
		t.Fatalf("prep path counts = %d/%d/%d/%d, want 1/1/1/2", materializeCalls, loadIssueCalls, loadIndexCalls, loadStateCalls)
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
