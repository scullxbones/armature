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
		Root:     tmpdir,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, DecisionAllow, result.Decision.Action, "absolute path within scope should be allowed when root is provided")
	assert.NotEmpty(t, result.Output)
}

func TestHook_Evaluate_BlocksOutOfScopeRelativePathWithSubdirectoryCwd_P1_SECURITY(t *testing.T) {
	t.Parallel()

	worktreeRoot := t.TempDir()
	cwdInSubdir := filepath.Join(worktreeRoot, "docs")

	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"Edit",
		"cwd":"` + cwdInSubdir + `",
		"tool_input":{"path":"internal/x.go"}
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
		Root:     worktreeRoot,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, DecisionBlock, result.Decision.Action,
		"relative path with subdirectory cwd should be evaluated against worktree root, not cwd")
	assert.Contains(t, result.Decision.Message, "outside task scope")
}

func TestHook_Evaluate_RelativePathNoCwdWithRoot_EvaluatedAgainstRoot(t *testing.T) {
	t.Parallel()
	worktreeRoot := t.TempDir()

	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"Edit",
		"tool_input":{"path":"docs/other/x.go"}
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
		Root:     worktreeRoot,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, DecisionBlock, result.Decision.Action,
		"path outside the declared scope should be blocked even with no cwd, when Root is set")
}

func TestHook_Evaluate_RelativePathNoCwdWithRoot_InScopeAllowed(t *testing.T) {
	t.Parallel()
	worktreeRoot := t.TempDir()

	input := []byte(`{
		"hook_event_name":"PreToolUse",
		"tool_name":"Edit",
		"tool_input":{"path":"internal/harnesshook/hook.go"}
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
		Root:     worktreeRoot,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, DecisionAllow, result.Decision.Action,
		"in-scope relative path with no cwd but Root set should still be allowed")
}
