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
	Input          []byte // raw hook event JSON
	TaskID         string // DEPRECATED: resolved binding from harness-hook cmd; kept for backward compat
	Platform       string // platform identifier (claude, codex, devin); defaults to "claude"
	SessionBinding string // binding from hook process cwd or env; used as fallback after path resolution
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
// 2. Decodes input to Event (before binding resolution per ADR-0007)
// 3. Resolves binding from decoded event and session binding
// 4. Resolves task policy using resolved binding
// 5. Builds evaluator from policy
// 6. Evaluates event against policy
// 7. Encodes result to output
func (h *Hook) Evaluate(ctx context.Context, input EvaluateInput) (RunResult, error) {
	// Select adapter for platform
	adapter, err := NewAdapterForPlatform(input.Platform)
	if err != nil {
		return RunResult{}, err
	}

	// Decode hook input to Event (before binding resolution)
	event, err := adapter.Decode(input.Input)
	if err != nil {
		return RunResult{}, fmt.Errorf("decode hook input: %w", err)
	}

	// Resolve binding from decoded event and session binding (ADR-0007)
	filePath := extractFilePathFromToolInput(event.ToolInput)
	eventInfo := &DecodedEventInfo{
		Kind:     event.Kind,
		FilePath: filePath,
	}
	binding, err := ResolveBindingFromEvent(eventInfo, input.SessionBinding)
	if err != nil {
		return RunResult{}, fmt.Errorf("resolve binding from event: %w", err)
	}

	// Use resolved binding, fall back to TaskID for backward compat
	if binding == "" {
		binding = input.TaskID
	}

	// Resolve policy for task using resolved binding
	policy, err := h.resolver.Resolve(binding)
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

// extractFilePathFromToolInput extracts the file path from the raw tool_input map.
// It checks for common file path keys in the order they're likely to be used.
func extractFilePathFromToolInput(toolInput map[string]any) string {
	if toolInput == nil {
		return ""
	}

	// Check for direct file_path or path keys
	for _, key := range []string{"file_path", "path"} {
		if value, ok := toolInput[key].(string); ok && value != "" {
			return value
		}
	}

	// Check for changes array (common in Edit/Write events)
	if changes, ok := toolInput["changes"].([]any); ok && len(changes) > 0 {
		if change, ok := changes[0].(map[string]any); ok {
			if path, ok := change["path"].(string); ok && path != "" {
				return path
			}
		}
	}

	return ""
}
