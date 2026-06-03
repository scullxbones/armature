package orchestrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	armcontext "github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// RunRequest is the caller-facing input contract for a single orchestration run.
// It stays small on purpose; repo-backed preparation resolves the rest.
type RunRequest struct {
	TaskID        string
	WorkerID      string
	Harness       string
	ModelOverride string
	RetryBudget   int
	Timeout       time.Duration
	DryRun        bool
	Progress      func(ProgressEvent)
}

// ScopeConflict reports another active task whose scope blocks dispatch.
type ScopeConflict struct {
	TaskID string
	Paths  []string
}

// LifecycleOutcome reports the task status decision made by a completed run.
type LifecycleOutcome struct {
	Status  string
	Outcome string
}

// RunDiagnostics packages caller-relevant run details without exposing
// op-replay state as the command/runtime surface.
type RunDiagnostics struct {
	Checks  []CheckResult
	Timeout TimeoutDiagnostics
}

// RunResult is the caller-facing result contract for a single orchestration run.
type RunResult struct {
	TaskID            string
	Phase             string
	Run               int
	DryRun            bool
	WouldClaim        bool
	ClaimHeld         bool
	ClaimOwner        string
	WouldDispatch     bool
	BlockedReason     string
	ScopeConflicts    []ScopeConflict
	Harness           string
	Model             string
	AuthSource        string
	LifecycleOutcome  LifecycleOutcome
	CompletionMessage string
	Diagnostics       RunDiagnostics
}

// RepoRunner executes orchestration runs from repo-backed state instead of
// requiring command code to assemble materialized issue truth first.
type RepoRunner struct {
	appCtx   *config.Context
	workerID string
	deps     repoRunnerDeps
}

type repoRunnerDeps struct {
	readAllOpsWithOffsets func(string) ([]ops.Op, map[string]int64, error)
	readLog               func(string) ([]ops.Op, error)
	appendAndCommit       func(string, string, ops.Op, ops.GitCommitter) error
	materialize           func(string, []ops.Op, bool, map[string]int64) (materialize.Result, error)
	loadIssue             func(string) (materialize.Issue, error)
	loadIndex             func(string) (materialize.Index, error)
	loadState             func(string) (*materialize.State, error)
	resolveAuthPlan       func(string, AuthConfig) (AuthPlan, error)
	newHarnessAdapter     func(HarnessConfig) (HarnessAdapter, error)
	newGitClient          func(string) GitClient
	execute               func(context.Context, ServiceConfig, RunInput) (OrchestrateState, error)
	nowUnix               func() int64
}

type preparedRun struct {
	serviceCfg ServiceConfig
	runInput   RunInput
	result     RunResult
}

func NewRepoRunner(appCtx *config.Context, workerID string) *RepoRunner {
	return &RepoRunner{
		appCtx:   appCtx,
		workerID: workerID,
		deps: repoRunnerDeps{
			readAllOpsWithOffsets: readAllOpsFromDirWithOffsets,
			readLog:               ops.ReadLog,
			appendAndCommit:       ops.AppendAndCommit,
			materialize:           materialize.Materialize,
			loadIssue:             materialize.LoadIssue,
			loadIndex:             materialize.LoadIndex,
			loadState:             loadStateFromStateDir,
			resolveAuthPlan:       ResolveAuthPlan,
			newHarnessAdapter:     NewHarnessAdapter,
			newGitClient: func(repoPath string) GitClient {
				return adapters.New(repoPath)
			},
			execute: func(ctx context.Context, svcCfg ServiceConfig, input RunInput) (OrchestrateState, error) {
				return NewService(svcCfg).Run(ctx, input)
			},
			nowUnix: func() int64 { return time.Now().Unix() },
		},
	}
}

func (r *RepoRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	prepared, err := r.prepare(ctx, req)
	if err != nil {
		return RunResult{}, err
	}
	if prepared.result.BlockedReason != "" || req.DryRun {
		return prepared.result, nil
	}

	state, err := r.deps.execute(ctx, prepared.serviceCfg, prepared.runInput)
	if err != nil {
		if runErr, ok := err.(*RunError); ok {
			prepared.result.Diagnostics.Timeout = runErr.Diagnostics
		}
		return mergeRunResult(prepared.result, state), err
	}
	return mergeRunResult(prepared.result, state), nil
}

func (r *RepoRunner) prepare(ctx context.Context, req RunRequest) (preparedRun, error) {
	if r.appCtx == nil {
		return preparedRun{}, fmt.Errorf("repo runner requires config context")
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return preparedRun{}, fmt.Errorf("task id is required")
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		workerID = r.workerID
	}
	if workerID == "" {
		return preparedRun{}, fmt.Errorf("worker id is required")
	}
	harness := strings.TrimSpace(req.Harness)
	if harness == "" {
		harness = "claude"
	}

	logPath := filepath.Join(r.appCtx.IssuesDir, "ops", workerID+".log")
	allOps, offsets, err := r.deps.readAllOpsWithOffsets(filepath.Join(r.appCtx.IssuesDir, "ops"))
	if err != nil {
		return preparedRun{}, fmt.Errorf("read ops: %w", err)
	}
	stateDir := r.appCtx.StateDir
	if stateDir == "" {
		stateDir = filepath.Join(r.appCtx.IssuesDir, "state", workerID)
	}
	if _, err := r.deps.materialize(stateDir, allOps, r.appCtx.Mode == "single-branch", offsets); err != nil {
		return preparedRun{}, fmt.Errorf("materialize: %w", err)
	}
	issue, err := r.deps.loadIssue(filepath.Join(stateDir, "issues", req.TaskID+".json"))
	if err != nil {
		return preparedRun{}, fmt.Errorf("issue %s not found: %w", req.TaskID, err)
	}
	index, err := r.deps.loadIndex(filepath.Join(stateDir, "index.json"))
	if err != nil {
		return preparedRun{}, fmt.Errorf("load index: %w", err)
	}

	model := resolveRunModel(req.ModelOverride, issue.PreferredModel, r.appCtx.Config.Orchestrator.DefaultModel)
	result := RunResult{
		TaskID:        req.TaskID,
		Phase:         "pending",
		DryRun:        req.DryRun,
		Harness:       harness,
		Model:         model,
		WouldDispatch: true,
	}

	if issue.ClaimedBy != "" && issue.ClaimedBy != workerID {
		result.ClaimOwner = issue.ClaimedBy
		result.BlockedReason = fmt.Sprintf("task claimed by %s", issue.ClaimedBy)
		result.WouldDispatch = false
		return preparedRun{result: result}, nil
	}

	if issue.ClaimedBy == workerID {
		result.ClaimHeld = true
		result.ClaimOwner = workerID
	} else {
		result.WouldClaim = true
		if !req.DryRun {
			ttl := r.appCtx.Config.DefaultTTL
			if ttl <= 0 {
				ttl = 60
			}
			claimOp := ops.Op{
				Type:      ops.OpClaim,
				TargetID:  req.TaskID,
				Timestamp: r.deps.nowUnix(),
				WorkerID:  workerID,
				Payload:   ops.Payload{TTL: ttl},
			}
			if err := r.deps.appendAndCommit(logPath, r.appCtx.WorktreePath, claimOp, gitCommitterForContext(r.appCtx)); err != nil {
				return preparedRun{}, fmt.Errorf("append claim op: %w", err)
			}
			allOps, offsets, err = r.deps.readAllOpsWithOffsets(filepath.Join(r.appCtx.IssuesDir, "ops"))
			if err != nil {
				return preparedRun{}, fmt.Errorf("read ops after claim: %w", err)
			}
			if _, err := r.deps.materialize(stateDir, allOps, r.appCtx.Mode == "single-branch", offsets); err != nil {
				return preparedRun{}, fmt.Errorf("materialize after claim: %w", err)
			}
			issue, err = r.deps.loadIssue(filepath.Join(stateDir, "issues", req.TaskID+".json"))
			if err != nil {
				return preparedRun{}, fmt.Errorf("reload issue %s: %w", req.TaskID, err)
			}
			index, err = r.deps.loadIndex(filepath.Join(stateDir, "index.json"))
			if err != nil {
				return preparedRun{}, fmt.Errorf("reload index: %w", err)
			}
			if issue.ClaimedBy != workerID {
				result.ClaimOwner = issue.ClaimedBy
				result.BlockedReason = fmt.Sprintf("task claimed by %s", issue.ClaimedBy)
				result.WouldDispatch = false
				return preparedRun{result: result}, nil
			}
			result.ClaimHeld = true
			result.ClaimOwner = workerID
		}
	}

	activeScopes := make(map[string][]string)
	for id, entry := range index {
		if id == req.TaskID {
			continue
		}
		if entry.Status == ops.StatusClaimed || entry.Status == ops.StatusInProgress {
			activeScopes[id] = entry.Scope
		}
	}
	for otherID, scope := range activeScopes {
		if scopesOverlap(issue.Scope, scope) {
			result.ScopeConflicts = append(result.ScopeConflicts, ScopeConflict{TaskID: otherID, Paths: append([]string(nil), scope...)})
		}
	}
	if len(result.ScopeConflicts) > 0 {
		result.BlockedReason = "scope conflict"
		result.WouldDispatch = false
		return preparedRun{result: result}, nil
	}

	if req.DryRun {
		return preparedRun{result: result}, nil
	}

	renderedContext, err := r.buildTaskContext(ctx, stateDir, req.TaskID)
	if err != nil {
		return preparedRun{}, fmt.Errorf("build task context: %w", err)
	}

	authPlan, err := r.deps.resolveAuthPlan(harness, AuthConfig{
		Mode:    r.appCtx.Config.Orchestrator.Auth.Mode,
		EnvFile: r.appCtx.Config.Orchestrator.Auth.EnvFile,
	})
	if err != nil {
		return preparedRun{}, fmt.Errorf("orchestrate preflight auth: %w", err)
	}
	result.AuthSource = authPlan.Source

	harnessCfg := HarnessConfig{
		Adapter:        harness,
		Model:          model,
		Timeout:        int(req.Timeout / time.Second),
		BuildCmd:       r.appCtx.Config.Orchestrator.Adapters.Build,
		LintCmd:        r.appCtx.Config.Orchestrator.Adapters.Lint,
		TestCmd:        r.appCtx.Config.Orchestrator.Adapters.Test,
		CoverageCmd:    r.appCtx.Config.Orchestrator.Adapters.Coverage,
		MutateCmd:      r.appCtx.Config.Orchestrator.Adapters.Mutate,
		WorkDir:        r.appCtx.RepoPath,
		TimeoutSeconds: int(req.Timeout / time.Second),
		Env:            authPlan.Env,
		AuthSource:     authPlan.Source,
	}
	harnessAdapter, err := r.deps.newHarnessAdapter(harnessCfg)
	if err != nil {
		return preparedRun{}, fmt.Errorf("create harness: %w", err)
	}
	serviceCfg := ServiceConfig{
		Git:     r.deps.newGitClient(r.appCtx.RepoPath),
		OpLog:   &repoOpLog{deps: r.deps, logPath: logPath, worktreePath: r.appCtx.WorktreePath},
		Harness: harnessAdapter,
	}
	runInput := RunInput{
		TaskID:       req.TaskID,
		TaskTitle:    issue.Title,
		TaskContract: string(issue.Acceptance),
		BuildTaskContext: func(context.Context, string) (string, error) {
			return renderedContext, nil
		},
		WorkerID:     workerID,
		RetryBudget:  req.RetryBudget,
		Scope:        issue.Scope,
		ActiveScopes: activeScopes,
		HarnessCfg:   harnessCfg,
		Opts: RunOptions{
			DryRun:            req.DryRun,
			WorkDir:           r.appCtx.RepoPath,
			HeartbeatInterval: 5 * time.Second,
			Progress:          req.Progress,
		},
	}
	return preparedRun{
		serviceCfg: serviceCfg,
		runInput:   runInput,
		result:     result,
	}, nil
}

func (r *RepoRunner) buildTaskContext(ctx context.Context, stateDir, issueID string) (string, error) {
	state, err := r.deps.loadState(stateDir)
	if err != nil {
		return "", fmt.Errorf("load state: %w", err)
	}
	assembled, err := armcontext.Assemble(issueID, stateDir, state)
	if err != nil {
		return "", fmt.Errorf("assemble context: %w", err)
	}
	if r.appCtx.Config.TokenBudget > 0 {
		assembled = armcontext.Truncate(assembled, r.appCtx.Config.TokenBudget)
	}
	rendered, err := armcontext.RenderAgent(assembled)
	if err != nil {
		return "", fmt.Errorf("render context: %w", err)
	}
	return rendered, nil
}

func mergeRunResult(base RunResult, state OrchestrateState) RunResult {
	base.Phase = state.Phase
	base.Run = state.Run
	base.CompletionMessage = state.CompletionMessage
	base.Diagnostics.Checks = append([]CheckResult(nil), state.Checks...)
	switch {
	case state.Phase == "complete" && state.CompletionMessage == "":
		base.LifecycleOutcome = LifecycleOutcome{Status: ops.StatusDone, Outcome: "orchestrate completed with committed changes"}
	case state.Phase == "complete" && state.CompletionMessage != "":
		base.LifecycleOutcome = LifecycleOutcome{Status: ops.StatusBlocked, Outcome: state.CompletionMessage}
	case state.Phase == "escalated":
		base.LifecycleOutcome = LifecycleOutcome{Status: ops.StatusBlocked, Outcome: "orchestration escalated"}
	}
	return base
}

type repoOpLog struct {
	deps         repoRunnerDeps
	logPath      string
	worktreePath string
}

func (r *repoOpLog) ReadAll() ([]ops.Op, error) {
	return r.deps.readLog(r.logPath)
}

func (r *repoOpLog) Append(op ops.Op) error {
	return r.deps.appendAndCommit(r.logPath, r.worktreePath, op, gitCommitterForWorktree(r.worktreePath))
}

func gitCommitterForContext(appCtx *config.Context) ops.GitCommitter {
	if appCtx == nil || appCtx.WorktreePath == "" {
		return nil
	}
	return adapters.New(appCtx.WorktreePath)
}

func gitCommitterForWorktree(worktreePath string) ops.GitCommitter {
	if worktreePath == "" {
		return nil
	}
	return adapters.New(worktreePath)
}

func readAllOpsFromDirWithOffsets(opsDir string) ([]ops.Op, map[string]int64, error) {
	logFiles, err := adapters.ListLogFiles(opsDir)
	if err != nil {
		if logFiles == nil {
			return []ops.Op{}, map[string]int64{}, nil
		}
		return nil, nil, err
	}
	var allOps []ops.Op
	offsets := make(map[string]int64)
	for _, logPath := range logFiles {
		logOps, err := ops.ReadLog(logPath)
		if err != nil {
			continue
		}
		if info, err := os.Stat(logPath); err == nil {
			offsets[filepath.Base(logPath)] = info.Size()
		}
		allOps = append(allOps, logOps...)
	}
	return allOps, offsets, nil
}

func loadStateFromStateDir(stateDir string) (*materialize.State, error) {
	stateIssuesDir := filepath.Join(stateDir, "issues")
	state := materialize.NewState()
	entries, err := os.ReadDir(stateIssuesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		issue, err := materialize.LoadIssue(filepath.Join(stateIssuesDir, entry.Name()))
		if err != nil {
			continue
		}
		issueCopy := issue
		state.Issues[issue.ID] = &issueCopy
	}
	return state, nil
}

func scopesOverlap(left, right []string) bool {
	for _, l := range left {
		ln := normalizeScopePath(l)
		for _, r := range right {
			rn := normalizeScopePath(r)
			if ln == rn || strings.HasPrefix(ln, rn) || strings.HasPrefix(rn, ln) {
				return true
			}
		}
	}
	return false
}

func normalizeScopePath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimSuffix(path, "*")
	return path
}

func resolveRunModel(flagModel, taskModel, configDefault string) string {
	if flagModel != "" {
		return flagModel
	}
	if taskModel != "" {
		return taskModel
	}
	return configDefault
}
