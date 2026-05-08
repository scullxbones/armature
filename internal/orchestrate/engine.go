package orchestrate

import (
	"context"
	"fmt"
	"time"

	claimPkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
)

// GitClient abstracts the git operations required by the engine.
// This interface is satisfied by *adapters.Client.
type GitClient interface {
	// HeadSHA returns the current HEAD commit SHA in the worktree.
	HeadSHA() (string, error)
	// DiffFrom returns the unified diff between baseSHA and HEAD.
	DiffFrom(baseSHA string) (string, error)
	// DiffNameOnly returns the list of files changed between baseSHA and HEAD.
	DiffNameOnly(baseSHA string) ([]string, error)
	// ResetHard resets the working tree and index to ref.
	ResetHard(ref string) error
	// ApplyPatch applies a unified diff patch to the working tree.
	ApplyPatch(patch []byte) error
	// AddAll stages all changes (git add -A).
	AddAll() error
	// CommitWithMessage creates a commit with the given message.
	CommitWithMessage(message string) error
	// RemoveWorktree removes a linked worktree at path.
	RemoveWorktree(path string) error
}

// OpLog abstracts reading and writing ops.
type OpLog interface {
	// ReadAll returns all ops in the log.
	ReadAll() ([]ops.Op, error)
	// Append writes a single op to the log.
	Append(op ops.Op) error
}

// EngineConfig holds all dependencies for the engine.
type EngineConfig struct {
	// TaskID is the issue ID being orchestrated.
	TaskID string
	// Git is the git adapter for the agent worktree.
	Git GitClient
	// OpLog is the op log adapter.
	OpLog OpLog
	// Harness is the verification adapter to run after dispatch.
	Harness HarnessAdapter
	// HarnessCfg is configuration passed to the harness.
	HarnessCfg HarnessConfig
	// Scope is the list of file paths the task is permitted to touch.
	Scope []string
	// ActiveScopes maps other task IDs to their scope lists for overlap checking.
	ActiveScopes map[string][]string
	// Opts controls dry-run, parallelism, etc.
	Opts RunOptions
	// RetryBudget is the number of retry attempts allowed.
	RetryBudget int
	// WorkerID is the ID of the running worker (used in op payloads).
	WorkerID string
}

// Engine implements the orchestration loop for a single task.
type Engine struct {
	cfg EngineConfig
}

// NewEngine creates an Engine from the given config.
func NewEngine(cfg EngineConfig) *Engine {
	return &Engine{cfg: cfg}
}

// Run executes the orchestration state machine for the configured task.
//
// State machine:
//
//	pending     → dispatched  (write OpOrchestrateDispatch)
//	dispatched  → verifying   (run harness)
//	verifying   → correcting  (harness passed: zero-trust commit sequence)
//	correcting  → done        (write OpOrchestrateComplete)
//	verifying   → retrying    (harness failed, budget > 0: write VerifyFail + Retry)
//	retrying    → dispatched  (re-dispatch next run)
//	retrying    → escalated   (budget == 0: write OpOrchestrateEscalate)
//
// Crash resume: DeriveState replays the op log so a re-entered Run resumes
// from the correct phase without re-writing already-written ops.
//
// Dry-run: exits before any dispatch op is written.
func (e *Engine) Run(ctx context.Context) (OrchestrateState, error) {
	// Honour cancellation before touching anything.
	if err := ctx.Err(); err != nil {
		return OrchestrateState{}, err
	}

	// --- 1. Crash-resume: replay op log ---
	allOps, err := e.cfg.OpLog.ReadAll()
	if err != nil {
		return OrchestrateState{}, fmt.Errorf("read op log: %w", err)
	}
	state := DeriveState(allOps, e.cfg.TaskID)

	// --- 2. Terminal phases are idempotent ---
	switch state.Phase {
	case "complete", "escalated":
		return state, nil
	}

	// --- 3. Dry-run exits here (before any dispatch op) ---
	if e.cfg.Opts.DryRun {
		return state, nil
	}

	// --- 4. Scope overlap check ---
	if err := e.claimTask(); err != nil {
		return state, fmt.Errorf("claim task: %w", err)
	}

	// --- 5. Dispatch (pending or retrying) ---
	if state.Phase == "pending" || state.Phase == "retrying" {
		state, err = e.dispatchPhase(ctx, state)
		if err != nil {
			return state, err
		}
	}

	// --- 6. Running: zero-trust commit + verify ---
	if state.Phase == "dispatched" || state.Phase == "running" {
		state, err = e.runningPhase(ctx, state)
		if err != nil {
			return state, err
		}
	}

	return state, nil
}

// claimTask checks scope overlap with other active tasks.
// Returns an error if the task scope overlaps with an already-active task.
func (e *Engine) claimTask() error {
	for otherID, otherScope := range e.cfg.ActiveScopes {
		if otherID == e.cfg.TaskID {
			continue
		}
		if claimPkg.ScopesOverlap(e.cfg.Scope, otherScope) {
			return fmt.Errorf("scope overlap between %s and %s", e.cfg.TaskID, otherID)
		}
	}
	return nil
}

// dispatchPhase records the pre-dispatch HEAD ref and writes the dispatch op.
func (e *Engine) dispatchPhase(ctx context.Context, state OrchestrateState) (OrchestrateState, error) {
	if err := ctx.Err(); err != nil {
		return state, err
	}

	ref, err := e.cfg.Git.HeadSHA()
	if err != nil {
		return state, fmt.Errorf("head sha for dispatch: %w", err)
	}

	dispatchOp := ops.Op{
		Type:      ops.OpOrchestrateDispatch,
		TargetID:  e.cfg.TaskID,
		Timestamp: time.Now().Unix(),
		WorkerID:  e.cfg.WorkerID,
		Payload: ops.Payload{
			PreDispatchRef: ref,
			WorktreePath:   e.cfg.HarnessCfg.WorkDir,
			RetryBudget:    e.cfg.RetryBudget,
		},
	}
	if err := e.cfg.OpLog.Append(dispatchOp); err != nil {
		return state, fmt.Errorf("append dispatch op: %w", err)
	}

	state.Phase = "dispatched"
	state.PreDispatchRef = ref
	state.Run++

	return state, nil
}

// runningPhase dispatches the harness, then performs the zero-trust commit
// sequence or records a verify failure and schedules retry or escalation.
func (e *Engine) runningPhase(ctx context.Context, state OrchestrateState) (OrchestrateState, error) {
	if err := ctx.Err(); err != nil {
		return state, err
	}

	// If we resumed into dispatched (not yet running), run the harness.
	if state.Phase == "dispatched" {
		// Record dispatch-complete transition.
		dcOp := ops.Op{
			Type:      ops.OpOrchestrateDispatchComplete,
			TargetID:  e.cfg.TaskID,
			Timestamp: time.Now().Unix(),
			WorkerID:  e.cfg.WorkerID,
		}
		if err := e.cfg.OpLog.Append(dcOp); err != nil {
			return state, fmt.Errorf("append dispatch-complete op: %w", err)
		}
		state.Phase = "running"
	}

	// Run the harness adapter.
	checkResult, err := e.cfg.Harness.Run(ctx, e.cfg.HarnessCfg, e.cfg.Opts)
	if err != nil {
		return state, fmt.Errorf("harness run: %w", err)
	}
	state.Checks = append(state.Checks, checkResult)

	// Record the check result op.
	crOp := ops.Op{
		Type:      ops.OpOrchestrateCheckResult,
		TargetID:  e.cfg.TaskID,
		Timestamp: time.Now().Unix(),
		WorkerID:  e.cfg.WorkerID,
		Payload:   ops.Payload{Msg: checkResult.Message},
	}
	if err := e.cfg.OpLog.Append(crOp); err != nil {
		return state, fmt.Errorf("append check-result op: %w", err)
	}

	if !checkResult.Passed && checkResult.Severity == SeverityError {
		return e.handleVerifyFailure(ctx, state)
	}

	// Harness passed — proceed with zero-trust commit sequence.
	return e.zeroTrustCommit(ctx, state)
}

// handleVerifyFailure records a verify-fail op and either schedules a retry
// or escalates if the retry budget is exhausted.
func (e *Engine) handleVerifyFailure(ctx context.Context, state OrchestrateState) (OrchestrateState, error) {
	if err := ctx.Err(); err != nil {
		return state, err
	}

	state.Failed = true

	// Write verify-fail op.
	vfOp := ops.Op{
		Type:      ops.OpOrchestrateVerifyFail,
		TargetID:  e.cfg.TaskID,
		Timestamp: time.Now().Unix(),
		WorkerID:  e.cfg.WorkerID,
	}
	if err := e.cfg.OpLog.Append(vfOp); err != nil {
		return state, fmt.Errorf("append verify-fail op: %w", err)
	}
	state.Phase = "verify-failed"

	budget := state.RetryBudget
	if budget <= 0 {
		budget = e.cfg.RetryBudget
	}

	if budget <= 0 {
		// Escalate.
		escOp := ops.Op{
			Type:      ops.OpOrchestrateEscalate,
			TargetID:  e.cfg.TaskID,
			Timestamp: time.Now().Unix(),
			WorkerID:  e.cfg.WorkerID,
			Payload:   ops.Payload{Msg: "retry budget exhausted"},
		}
		if err := e.cfg.OpLog.Append(escOp); err != nil {
			return state, fmt.Errorf("append escalate op: %w", err)
		}
		state.Phase = "escalated"
		return state, nil
	}

	// Decrement budget and retry.
	newBudget := budget - 1
	retryOp := ops.Op{
		Type:      ops.OpOrchestrateRetry,
		TargetID:  e.cfg.TaskID,
		Timestamp: time.Now().Unix(),
		WorkerID:  e.cfg.WorkerID,
		Payload:   ops.Payload{RetryBudget: newBudget},
	}
	if err := e.cfg.OpLog.Append(retryOp); err != nil {
		return state, fmt.Errorf("append retry op: %w", err)
	}
	state.Phase = "retrying"
	state.RetryBudget = newBudget

	return state, nil
}

// zeroTrustCommit implements the zero-trust commit sequence:
//
//	DiffFrom(preDispatchRef) → ResetHard(preDispatchRef) → ApplyPatch → RunPipeline → AddAll + CommitWithMessage
func (e *Engine) zeroTrustCommit(ctx context.Context, state OrchestrateState) (OrchestrateState, error) {
	if err := ctx.Err(); err != nil {
		return state, err
	}

	preRef := state.PreDispatchRef
	if preRef == "" {
		return state, fmt.Errorf("zero-trust commit: missing pre-dispatch ref")
	}

	// Step 1: capture diff since pre-dispatch ref.
	patch, err := e.cfg.Git.DiffFrom(preRef)
	if err != nil {
		return state, fmt.Errorf("zero-trust diff: %w", err)
	}

	// Step 2: reset to pre-dispatch state.
	if err := e.cfg.Git.ResetHard(preRef); err != nil {
		return state, fmt.Errorf("zero-trust reset: %w", err)
	}

	// Step 3: re-apply patch.
	if len(patch) > 0 {
		if err := e.cfg.Git.ApplyPatch([]byte(patch)); err != nil {
			return state, fmt.Errorf("zero-trust apply: %w", err)
		}
	}

	// Step 4: stage all changes and commit.
	if err := e.cfg.Git.AddAll(); err != nil {
		return state, fmt.Errorf("zero-trust add: %w", err)
	}

	commitMsg := fmt.Sprintf("feat(%s): automated implementation run %d", e.cfg.TaskID, state.Run)
	if err := e.cfg.Git.CommitWithMessage(commitMsg); err != nil {
		// Nothing to commit is acceptable (patch was empty); log as info.
		// Any other error is fatal.
		if err.Error() != "nothing to commit: index is clean" {
			return state, fmt.Errorf("zero-trust commit: %w", err)
		}
	}

	// Step 5: write complete op.
	completeOp := ops.Op{
		Type:      ops.OpOrchestrateComplete,
		TargetID:  e.cfg.TaskID,
		Timestamp: time.Now().Unix(),
		WorkerID:  e.cfg.WorkerID,
	}
	if err := e.cfg.OpLog.Append(completeOp); err != nil {
		return state, fmt.Errorf("append complete op: %w", err)
	}
	state.Phase = "complete"

	return state, nil
}
