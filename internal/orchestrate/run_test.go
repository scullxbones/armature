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
	if materializeCalls != 1 || loadIssueCalls != 1 || loadIndexCalls != 1 || loadStateCalls != 1 {
		t.Fatalf("prep path counts = %d/%d/%d/%d, want 1/1/1/1", materializeCalls, loadIssueCalls, loadIndexCalls, loadStateCalls)
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
