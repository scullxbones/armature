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

func (m *mockResolver) Resolve(_ string) (harnesspolicy.TaskPolicy, error) {
	return m.policy, m.err
}

type mockEvaluator struct {
	decision Decision
	err      error
}

func (m *mockEvaluator) Evaluate(_ context.Context, event Event) (Decision, error) {
	return m.decision, m.err
}

type mockAdapter struct {
	encodeExitCode int
}

func (m *mockAdapter) Name() string {
	return "mock"
}

func (m *mockAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{}
}

func (m *mockAdapter) WriteConfig(_ string) error {
	return nil
}

func (m *mockAdapter) OwnsConfig(_ string) (bool, error) {
	return true, nil
}

func (m *mockAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

func (m *mockAdapter) Encode(_ Event, decision Decision) ([]byte, int, error) {
	data, err := json.Marshal(map[string]any{"decision": "allow"})
	return data, m.encodeExitCode, err
}

func TestRunner_DecodeAndRun(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestRunner_BlockDecision_PropagatesAdapterExitCode(t *testing.T) {
	t.Parallel()
	// Test that the runner passes through the exit code returned by the adapter.
	// Using a mockAdapter with encodeExitCode=2 to prove it's not hardcoded.
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

	adapter := &mockAdapter{encodeExitCode: 2}
	runner := NewRunner(&RunnerConfig{
		Adapter:   adapter,
		Resolver:  resolver,
		Evaluator: evaluator,
		TaskID:    "task-01",
	})

	result, err := runner.Run(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, DecisionBlock, result.Decision.Action)
	assert.Equal(t, 2, result.ExitCode)
}

func TestRunner_EncodeOutput(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestRunner_BlockDecision_UsesAdapterExitCode(t *testing.T) {
	t.Parallel()
	// Test that the runner uses the exit code returned by the adapter,
	// not a hardcoded mapping. ClaudeAdapter returns 0 for block decisions
	// (structured JSON output requires exit 0 for Claude to process it).
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
		decision: Decision{Action: DecisionBlock, Message: "access denied"},
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
	// ClaudeAdapter.Encode returns 0 for block decisions (structured JSON)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunner_StopBlockDecision(t *testing.T) {
	t.Parallel()
	// Test that Stop event + Block decision works correctly.
	// ClaudeAdapter has a special branch for Stop+Block that produces {"decision":"block","reason":"..."}.
	input := []byte(`{
		"hook_event_name":"Stop"
	}`)

	policy := harnesspolicy.TaskPolicy{
		ID:    "task-01",
		Title: "Test task",
		Scope: []string{"internal/harnesshook/"},
	}

	resolver := &mockResolver{policy: policy}
	evaluator := &mockEvaluator{
		decision: Decision{Action: DecisionBlock, Message: "task complete"},
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
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, DecisionBlock, result.Decision.Action)

	// Verify the output is valid JSON and contains the expected structure
	var output map[string]any
	err = json.Unmarshal(result.Output, &output)
	require.NoError(t, err)
	assert.Equal(t, "block", output["decision"])
}
