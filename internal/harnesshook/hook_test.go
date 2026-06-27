package harnesshook

import (
	"context"
	"errors"
	"testing"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHook_Evaluate_AllowsInScopeEdit_REQ_ARCHIMP_S17_T1(t *testing.T) {
	t.Parallel()
	// Test that Hook.Evaluate allows an in-scope edit event.
	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"Edit",
		"tool_input":{"path":"internal/harnesshook/hook.go"}
	}`)

	resolver := &mockResolver{
		policy: harnesspolicy.TaskPolicy{
			ID:    "task-01",
			Title: "Test hook task",
			Scope: []string{"internal/harnesshook/"},
		},
	}

	hook := NewHook(resolver)
	result, err := hook.Evaluate(context.Background(), EvaluateInput{
		Input:    input,
		TaskID:   "task-01",
		Platform: "claude",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, DecisionAllow, result.Decision.Action)
	assert.NotEmpty(t, result.Output)
}

func TestHook_Evaluate_ResolverErrorPropagates_REQ_ARCHIMP_S17_T1(t *testing.T) {
	t.Parallel()
	// Test that Hook.Evaluate propagates resolver errors.
	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"Edit",
		"tool_input":{"path":"internal/harnesshook/hook.go"}
	}`)

	resolverErr := errors.New("policy not found")
	resolver := &mockResolver{
		err: resolverErr,
	}

	hook := NewHook(resolver)
	_, err := hook.Evaluate(context.Background(), EvaluateInput{
		Input:    input,
		TaskID:   "task-01",
		Platform: "claude",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve policy")
	assert.ErrorIs(t, err, resolverErr)
}
