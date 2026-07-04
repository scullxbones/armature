package harnesshook

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockResolver struct {
	policy harnesspolicy.IssuePolicy
	err    error
}

func (m *mockResolver) Resolve(_ string) (harnesspolicy.IssuePolicy, error) {
	return m.policy, m.err
}

func TestHook_Evaluate_AllowsInScopeEdit_REQ_ARCHIMP_S17_T1(t *testing.T) {
	t.Parallel()
	// Test that Hook.Evaluate allows an in-scope edit event.
	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"Edit",
		"tool_input":{"path":"internal/harnesshook/hook.go"}
	}`)

	resolver := &mockResolver{
		policy: harnesspolicy.IssuePolicy{
			ID:    "task-01",
			Title: "Test hook task",
			Scope: []string{"internal/harnesshook/"},
		},
	}

	hook := NewHook(resolver)
	result, err := hook.Evaluate(context.Background(), EvaluateInput{
		Input:    input,
		Binding:  "task-01",
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
		Binding:  "task-01",
		Platform: "claude",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve policy")
	assert.ErrorIs(t, err, resolverErr)
}

func TestHook_Evaluate_AllowsAbsolutePathWithWorktreeRoot_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	// Test that Hook.Evaluate correctly evaluates absolute paths when a worktree root is provided.
	// This tests the fix for path-resolved events targeting a different worktree:
	// when we have an absolute path like /tmp/task-wt/internal/foo.go and the worktree
	// root is /tmp/task-wt, the scope policy should normalize it relative to /tmp/task-wt,
	// not relative to os.Getwd() (which would be the invoking repo).

	tmpdir := t.TempDir()
	absolutePath := filepath.Join(tmpdir, "internal", "hook.go")

	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"Edit",
		"tool_input":{"path":"` + absolutePath + `"}
	}`)

	resolver := &mockResolver{
		policy: harnesspolicy.IssuePolicy{
			ID:    "task-01",
			Title: "Test hook task",
			Scope: []string{"internal/"},
		},
	}

	hook := NewHook(resolver)
	result, err := hook.Evaluate(context.Background(), EvaluateInput{
		Input:    input,
		Binding:  "task-01",
		Platform: "claude",
		Root:     tmpdir, // Pass the worktree root so absolute path is normalized relative to it
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, DecisionAllow, result.Decision.Action, "absolute path within scope should be allowed when root is provided")
	assert.NotEmpty(t, result.Output)
}
