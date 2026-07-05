package harnesshook

import (
	"context"
	"fmt"
	"path/filepath"

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
//
// Binding resolution (ADR-0007) happens exactly once, in the caller (the
// harness-hook command), via ResolveBindingFromEvent. Hook.Evaluate accepts
// the already-resolved issue ID and does not re-derive it, so the policy
// enforced always matches the binding that was stale-checked and logged.
//
// Root is the worktree root directory (used for path normalization in scope checking).
// For path-resolved bindings, this should be the worktree directory where the binding
// was found. When empty, falls back to os.Getwd().
type EvaluateInput struct {
	Input    []byte // raw hook event JSON
	Binding  string // resolved issue ID from ResolveBindingFromEvent (caller-resolved)
	Platform string // platform identifier (claude, codex, devin); defaults to "claude"
	Root     string // worktree root for path normalization (optional; defaults to os.Getwd())
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
// 2. Decodes input to Event
// 3. Resolves task policy using the caller-resolved binding
// 4. Builds evaluator from policy
// 5. Evaluates event against policy (with absolutized paths for scope checking)
// 6. Encodes result to output
func (h *Hook) Evaluate(ctx context.Context, input EvaluateInput) (RunResult, error) {
	// Select adapter for platform
	adapter, err := NewAdapterForPlatform(input.Platform)
	if err != nil {
		return RunResult{}, err
	}

	// Decode hook input to Event
	event, err := adapter.Decode(input.Input)
	if err != nil {
		return RunResult{}, fmt.Errorf("decode hook input: %w", err)
	}

	// For scope checking, we need absolute paths. If the event has relative paths and a cwd,
	// absolutize them against the cwd (as was done during binding resolution).
	// This ensures that when cwd=/repo/docs and file_path=internal/x.go, the scope check
	// evaluates the absolute path /repo/docs/internal/x.go against the root /repo,
	// not the relative path against the cwd.
	//
	// When absolutization against cwd fails (cwd empty or itself relative) but a worktree
	// root was supplied, join the relative path against Root instead of leaving it as-is.
	// Falling through to the raw relative path would let it match scope entries textually
	// (e.g. a path "internal/x.go" matching scope "internal/" even though, relative to
	// Root, the file actually lives elsewhere) — a residual scope-check bypass.
	if event.Cwd != "" || input.Root != "" {
		absolutizedPaths := make([]string, 0, len(event.Paths))
		for _, path := range event.Paths {
			if absPath, ok := absolutizeFilePath(path, event.Cwd); ok {
				absolutizedPaths = append(absolutizedPaths, absPath)
			} else if input.Root != "" && !filepath.IsAbs(path) {
				absolutizedPaths = append(absolutizedPaths, filepath.Join(input.Root, path))
			} else {
				// If absolutization fails, use the original path
				absolutizedPaths = append(absolutizedPaths, path)
			}
		}
		event.Paths = absolutizedPaths
	}

	// Resolve policy for task using the already-resolved binding (ADR-0007: single resolution)
	policy, err := h.resolver.Resolve(input.Binding)
	if err != nil {
		return RunResult{}, fmt.Errorf("resolve policy: %w", err)
	}

	// Build evaluator from resolved policy, using the provided root (if any) for path normalization.
	// For path-resolved bindings, the root should be the worktree directory where the binding was found.
	// When root is empty, falls back to os.Getwd().
	service := harnesspolicy.NewVerificationService()
	var scopePolicy harnesspolicy.ScopePolicy
	if input.Root != "" {
		scopePolicy = harnesspolicy.NewScopePolicyWithRoot(policy.Scope, input.Root)
	} else {
		scopePolicy = harnesspolicy.NewScopePolicy(policy.Scope)
	}
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy:         scopePolicy,
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
