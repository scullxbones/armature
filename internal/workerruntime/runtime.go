package workerruntime

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrTaskRetrying signals recoverable unfinished orchestration work.
var ErrTaskRetrying = errors.New("task orchestration retrying")

// ReadyProvider returns the next ready issue ID.
type ReadyProvider interface {
	NextReady(ctx context.Context) (issueID string, ok bool, err error)
}

// Claimer attempts to claim a task for this worker.
type Claimer interface {
	Claim(ctx context.Context, issueID string) (won bool, err error)
}

// RetryClaimVerifier checks whether the worker still owns a previously claimed issue.
type RetryClaimVerifier interface {
	StillClaimed(ctx context.Context, issueID string) (bool, error)
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
	var retryIssueID string
	retryAttempts := 0
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
		issueID := retryIssueID
		if issueID == "" {
			var ok bool
			var err error
			issueID, ok, err = r.Ready.NextReady(runCtx)
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
			retryAttempts = 0
		} else {
			if verifier, ok := r.Claim.(RetryClaimVerifier); ok {
				stillClaimed, err := verifier.StillClaimed(runCtx, issueID)
				if err != nil {
					result.FinalState = StateEscalated
					result.Err = err
					return result, err
				}
				if !stillClaimed {
					retryIssueID = ""
					retryAttempts = 0
					continue
				}
			}
		}
		if err := r.Exec.Run(runCtx, issueID); err != nil {
			if errors.Is(err, ErrTaskRetrying) {
				retryAttempts++
				if retryAttempts > 20 {
					err = fmt.Errorf("task %s exceeded retry loop guard after %d attempts", issueID, retryAttempts)
					result.FinalState = StateEscalated
					result.Err = err
					return result, err
				}
				retryIssueID = issueID
				if r.Trace != nil {
					r.Trace.Trace(EventExecutionFailed)
				}
				time.Sleep(50 * time.Millisecond)
				continue
			}
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
		retryIssueID = ""
		retryAttempts = 0
		result.TasksCompleted++
		if r.Trace != nil {
			r.Trace.Trace(EventExecutionCompleted)
		}
	}
}
