package workerruntime

import (
	"context"
	"errors"
	"fmt"
)

// ReadyProvider returns the next ready issue ID.
type ReadyProvider interface {
	NextReady(ctx context.Context) (issueID string, ok bool, err error)
}

// Claimer attempts to claim a task for this worker.
type Claimer interface {
	Claim(ctx context.Context, issueID string) (won bool, err error)
}

// Orchestrator executes one claimed issue.
type Orchestrator interface {
	Run(ctx context.Context, issueID string) error
}

// TraceSink receives ephemeral runtime narration.
type TraceSink interface {
	Trace(event string)
}

// Runtime coordinates ready -> claim -> orchestrate until stop conditions.
type Runtime struct {
	Ready ReadyProvider
	Claim Claimer
	Exec  Orchestrator
	Trace TraceSink
}

// Run drains ready work until queue empty, max-tasks reached, or cancellation.
func (r *Runtime) Run(ctx context.Context, opts RuntimeOptions) (RunResult, error) {
	var result RunResult
	if r.Ready == nil || r.Claim == nil || r.Exec == nil {
		return RunResult{FinalState: StateStopped}, nil
	}
	runCtx := ctx
	if opts.MaxRuntime > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, opts.MaxRuntime)
		defer cancel()
	}
	for {
		if err := runCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && opts.MaxRuntime > 0 {
				timeoutErr := fmt.Errorf("runtime timeout after %s\nAction: rerun with a larger --max-runtime or inspect blocked work with `arm ready --explain --format json`", opts.MaxRuntime)
				result.FinalState = StateEscalated
				result.Err = err
				return result, timeoutErr
			}
			result.FinalState = StateStopped
			result.Err = err
			return result, err
		}
		if opts.MaxTasks > 0 && result.TasksCompleted >= opts.MaxTasks {
			result.FinalState = StateStopped
			return result, nil
		}
		issueID, ok, err := r.Ready.NextReady(runCtx)
		if err != nil {
			result.FinalState = StateEscalated
			result.Err = err
			return result, err
		}
		if !ok {
			if r.Trace != nil {
				r.Trace.Trace(EventNoReadyWork)
			}
			result.FinalState = StateIdle
			return result, nil
		}
		won, err := r.Claim.Claim(runCtx, issueID)
		if err != nil {
			result.FinalState = StateEscalated
			result.Err = err
			return result, err
		}
		if !won {
			if r.Trace != nil {
				r.Trace.Trace(EventClaimLost)
			}
			continue
		}
		if err := r.Exec.Run(runCtx, issueID); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && runCtx.Err() == context.DeadlineExceeded && opts.MaxRuntime > 0 {
				timeoutErr := fmt.Errorf("runtime timeout after %s\nAction: rerun with a larger --max-runtime or inspect blocked work with `arm ready --explain --format json`", opts.MaxRuntime)
				result.FinalState = StateEscalated
				result.Err = err
				return result, timeoutErr
			}
			if r.Trace != nil {
				r.Trace.Trace(EventExecutionFailed)
			}
			result.FinalState = StateEscalated
			result.Err = err
			return result, err
		}
		result.TasksCompleted++
		if r.Trace != nil {
			r.Trace.Trace(EventExecutionCompleted)
		}
	}
}
