package harnesshook

import (
	"context"
	"fmt"

	"github.com/scullxbones/armature/internal/harnesspolicy"
)

// PolicyResolver is the interface for resolving task policies.
type PolicyResolver interface {
	Resolve(taskID string) (harnesspolicy.TaskPolicy, error)
}

// RunnerConfig holds the dependencies for the hook runner.
// It now includes fields for state loading and policy resolution,
// which the runner performs during Run().
type RunnerConfig struct {
	Adapter   PlatformAdapter
	Resolver  PolicyResolver
	Evaluator Evaluator // May be nil; runner will create it from resolved policy if needed
	TaskID    string
	IssuesDir string
	StateDir  string
}

// RunResult contains the output and exit code from a hook run.
type RunResult struct {
	Output   []byte
	Decision Decision
	ExitCode int
}

// Runner orchestrates the hook execution: decoding input, resolving policy,
// evaluating the hook event, encoding output, and mapping to exit code.
type Runner struct {
	adapter   PlatformAdapter
	resolver  PolicyResolver
	evaluator Evaluator
	taskID    string
	issuesDir string
	stateDir  string
}

// NewRunner creates a new hook runner.
func NewRunner(cfg *RunnerConfig) *Runner {
	return &Runner{
		adapter:   cfg.Adapter,
		resolver:  cfg.Resolver,
		evaluator: cfg.Evaluator,
		taskID:    cfg.TaskID,
		issuesDir: cfg.IssuesDir,
		stateDir:  cfg.StateDir,
	}
}

// Run executes the hook runner pipeline:
// 1. Loads state and resolves policy (unless evaluator already configured)
// 2. Decodes the JSON input to an Event
// 3. Evaluates the event against the policy
// 4. Encodes the result
// 5. Maps the decision to an exit code
func (r *Runner) Run(ctx context.Context, input []byte) (RunResult, error) {
	// Ensure evaluator is configured; if not, load state and resolve policy
	evaluator := r.evaluator
	if evaluator == nil && r.resolver != nil {
		// Resolve policy for the task
		task, err := r.resolver.Resolve(r.taskID)
		if err != nil {
			return RunResult{}, fmt.Errorf("resolve policy for task %s: %w", r.taskID, err)
		}

		// Create evaluator from resolved policy
		service := harnesspolicy.NewVerificationService()
		evaluator = NewEvaluator(EvaluatorConfig{
			ScopePolicy:         harnesspolicy.NewScopePolicy(task.Scope),
			VerificationService: &service,
			VerificationInput: harnesspolicy.VerificationRequest{
				Acceptance: task.Acceptance,
				Citations:  task.Citations,
			},
		})
	}

	if evaluator == nil {
		return RunResult{}, fmt.Errorf("no evaluator configured and policy resolver unavailable")
	}

	// Decode hook input to Event
	event, err := r.adapter.Decode(input)
	if err != nil {
		return RunResult{}, fmt.Errorf("decode hook input: %w", err)
	}

	// Evaluate the event against the policy
	decision, err := evaluator.Evaluate(ctx, event)
	if err != nil {
		return RunResult{}, fmt.Errorf("evaluate hook: %w", err)
	}

	// Encode the result
	output, exitCode, err := r.adapter.Encode(event, decision)
	if err != nil {
		return RunResult{}, fmt.Errorf("encode hook output: %w", err)
	}

	return RunResult{
		Output:   output,
		Decision: decision,
		ExitCode: exitCode,
	}, nil
}
