package harnesshook

import (
	"context"
	"fmt"

	"github.com/scullxbones/armature/internal/harnesspolicy"
)

// PolicyResolver is the interface for resolving issue policies.
type PolicyResolver interface {
	Resolve(taskID string) (harnesspolicy.IssuePolicy, error)
}

// RunResult contains the output and exit code from a hook run.
type RunResult struct {
	Output   []byte
	Decision Decision
	ExitCode int
}

// EvaluateInput captures all inputs needed to evaluate a hook event.
type EvaluateInput struct {
	Input    []byte // raw hook event JSON
	TaskID   string
	Platform string // platform identifier (claude, codex, devin); defaults to "claude"
}

// Hook orchestrates hook evaluation: adapter selection, policy resolution,
// evaluator construction, event decoding/evaluation/encoding.
type Hook struct {
	resolver PolicyResolver
}

// NewHook creates a new Hook with the given policy resolver.
func NewHook(resolver PolicyResolver) *Hook {
	return &Hook{resolver: resolver}
}

// Evaluate executes the full hook evaluation pipeline:
// 1. Selects adapter for the platform
// 2. Resolves task policy
// 3. Builds evaluator from policy
// 4. Decodes input to Event
// 5. Evaluates event against policy
// 6. Encodes result to output
func (h *Hook) Evaluate(ctx context.Context, input EvaluateInput) (RunResult, error) {
	// Select adapter for platform
	adapter, err := NewAdapterForPlatform(input.Platform)
	if err != nil {
		return RunResult{}, err
	}

	// Resolve policy for task
	policy, err := h.resolver.Resolve(input.TaskID)
	if err != nil {
		return RunResult{}, fmt.Errorf("resolve policy: %w", err)
	}

	// Build evaluator from resolved policy
	service := harnesspolicy.NewVerificationService()
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy:         harnesspolicy.NewScopePolicy(policy.Scope),
		VerificationService: &service,
		VerificationInput: harnesspolicy.VerificationRequest{
			Acceptance: policy.Acceptance,
			Citations:  policy.Citations,
		},
	})

	// Decode hook input to Event
	event, err := adapter.Decode(input.Input)
	if err != nil {
		return RunResult{}, fmt.Errorf("decode hook input: %w", err)
	}

	// Evaluate the event against the policy
	decision, err := evaluator.Evaluate(ctx, event)
	if err != nil {
		return RunResult{}, fmt.Errorf("evaluate hook: %w", err)
	}

	// Encode the result
	output, exitCode, err := adapter.Encode(event, decision)
	if err != nil {
		return RunResult{}, fmt.Errorf("encode hook output: %w", err)
	}

	return RunResult{
		Output:   output,
		Decision: decision,
		ExitCode: exitCode,
	}, nil
}
