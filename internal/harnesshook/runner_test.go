package harnesshook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockResolver struct {
	policy harnesspolicy.TaskPolicy
	err    error
}

func (m *mockResolver) Resolve(taskID string) (harnesspolicy.TaskPolicy, error) {
	return m.policy, m.err
}

type mockEvaluator struct {
	decision Decision
	err      error
}

func (m *mockEvaluator) Evaluate(ctx context.Context, event Event) (Decision, error) {
	return m.decision, m.err
}

func TestRunner_DecodeAndRun(t *testing.T) {
	// Test that Runner successfully decodes JSON input and runs evaluation.
	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"apply_patch",
		"tool_input":{"changes":[{"path":"internal/harnesshook/runner.go"}]}
	}`)

	policy := harnesspolicy.TaskPolicy{
		ID:    "task-01",
		Title: "Test task",
		Scope: []string{"internal/harnesshook/"},
	}

	resolver := &mockResolver{policy: policy}
	evaluator := &mockEvaluator{
		decision: Decision{Action: DecisionAllow, Message: "test allow"},
	}

	adapter := NewClaudeAdapter()
	runner := NewRunner(&RunnerConfig{
		Adapter:   adapter,
		Resolver:  resolver,
		Evaluator: evaluator,
		TaskID:    "task-01",
	})

	result, err := runner.Run(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, result.Decision.Action)
	assert.Equal(t, "test allow", result.Decision.Message)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunner_DecodeError(t *testing.T) {
	// Test that Runner returns an error when JSON is invalid.
	input := []byte(`invalid json`)

	resolver := &mockResolver{}
	evaluator := &mockEvaluator{}
	adapter := NewClaudeAdapter()

	runner := NewRunner(&RunnerConfig{
		Adapter:   adapter,
		Resolver:  resolver,
		Evaluator: evaluator,
		TaskID:    "task-01",
	})

	_, err := runner.Run(context.Background(), input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode hook input")
}

func TestRunner_EvaluatorError(t *testing.T) {
	// Test that Runner returns an error when evaluation fails.
	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"apply_patch",
		"tool_input":{"changes":[{"path":"internal/harnesshook/runner.go"}]}
	}`)

	policy := harnesspolicy.TaskPolicy{
		ID:    "task-01",
		Title: "Test task",
		Scope: []string{"internal/harnesshook/"},
	}

	resolver := &mockResolver{policy: policy}
	evaluator := &mockEvaluator{err: assert.AnError}
	adapter := NewClaudeAdapter()

	runner := NewRunner(&RunnerConfig{
		Adapter:   adapter,
		Resolver:  resolver,
		Evaluator: evaluator,
		TaskID:    "task-01",
	})

	_, err := runner.Run(context.Background(), input)

	require.Error(t, err)
}

func TestRunner_BlockDecision(t *testing.T) {
	// Test that Block decision results in non-zero exit code.
	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"apply_patch",
		"tool_input":{"changes":[{"path":"cmd/armature/main.go"}]}
	}`)

	policy := harnesspolicy.TaskPolicy{
		ID:    "task-01",
		Title: "Test task",
		Scope: []string{"internal/harnesshook/"},
	}

	resolver := &mockResolver{policy: policy}
	evaluator := &mockEvaluator{
		decision: Decision{Action: DecisionBlock, Message: "outside task scope"},
	}

	adapter := NewClaudeAdapter()
	runner := NewRunner(&RunnerConfig{
		Adapter:   adapter,
		Resolver:  resolver,
		Evaluator: evaluator,
		TaskID:    "task-01",
	})

	result, err := runner.Run(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, DecisionBlock, result.Decision.Action)
	assert.Equal(t, 1, result.ExitCode)
}

func TestRunner_EncodeOutput(t *testing.T) {
	// Test that Runner successfully encodes the output from the adapter.
	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"apply_patch",
		"tool_input":{"changes":[{"path":"internal/harnesshook/runner.go"}]}
	}`)

	policy := harnesspolicy.TaskPolicy{
		ID:    "task-01",
		Title: "Test task",
		Scope: []string{"internal/harnesshook/"},
	}

	resolver := &mockResolver{policy: policy}
	evaluator := &mockEvaluator{
		decision: Decision{Action: DecisionAllow, Message: "test allow"},
	}

	adapter := NewClaudeAdapter()
	runner := NewRunner(&RunnerConfig{
		Adapter:   adapter,
		Resolver:  resolver,
		Evaluator: evaluator,
		TaskID:    "task-01",
	})

	result, err := runner.Run(context.Background(), input)

	require.NoError(t, err)
	assert.NotEmpty(t, result.Output)

	// Verify the output is valid JSON
	var output map[string]any
	err = json.Unmarshal(result.Output, &output)
	require.NoError(t, err)
	// For Allow decision, Claude adapter returns "continue" and "suppressOutput"
	assert.Equal(t, true, output["continue"])
}

func TestRunner_AllowDecisionZeroExitCode(t *testing.T) {
	// Test that Allow decision results in zero exit code.
	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"apply_patch",
		"tool_input":{"changes":[{"path":"internal/harnesshook/runner.go"}]}
	}`)

	policy := harnesspolicy.TaskPolicy{
		ID:    "task-01",
		Title: "Test task",
		Scope: []string{"internal/harnesshook/"},
	}

	resolver := &mockResolver{policy: policy}
	evaluator := &mockEvaluator{
		decision: Decision{Action: DecisionAllow, Message: "approved"},
	}

	adapter := NewClaudeAdapter()
	runner := NewRunner(&RunnerConfig{
		Adapter:   adapter,
		Resolver:  resolver,
		Evaluator: evaluator,
		TaskID:    "task-01",
	})

	result, err := runner.Run(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, result.Decision.Action)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunner_DecisionNoneZeroExitCode(t *testing.T) {
	// Test that None decision results in zero exit code.
	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"apply_patch",
		"tool_input":{"changes":[{"path":"internal/harnesshook/runner.go"}]}
	}`)

	policy := harnesspolicy.TaskPolicy{
		ID:    "task-01",
		Title: "Test task",
		Scope: []string{"internal/harnesshook/"},
	}

	resolver := &mockResolver{policy: policy}
	evaluator := &mockEvaluator{
		decision: Decision{Action: DecisionNone, Message: "event ignored"},
	}

	adapter := NewClaudeAdapter()
	runner := NewRunner(&RunnerConfig{
		Adapter:   adapter,
		Resolver:  resolver,
		Evaluator: evaluator,
		TaskID:    "task-01",
	})

	result, err := runner.Run(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, DecisionNone, result.Decision.Action)
	assert.Equal(t, 0, result.ExitCode)
}
