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

func TestHook_Evaluate_BlocksOutOfScopeRelativePathWithSubdirectoryCwd_P1_SECURITY(t *testing.T) {
	t.Parallel()
	// Security test: relative file_path with cwd in a subdirectory of the worktree.
	// When cwd=/workspace/docs and file_path=internal/x.go, binding resolution uses
	// the absolutized path /workspace/docs/internal/x.go. But the scope is defined
	// relative to the worktree root /workspace. The relative path from /workspace
	// is docs/internal/x.go, which should NOT match scope "internal/".
	//
	// Without the fix: relative path "internal/x.go" matches scope "internal/" (WRONG)
	// With the fix: absolute path /workspace/docs/internal/x.go relativized to /workspace
	//              becomes "docs/internal/x.go", which doesn't match scope (CORRECT)

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
			Scope: []string{"internal/"}, // Only allows files under the root's internal/
		},
	}

	hook := NewHook(resolver)
	result, err := hook.Evaluate(context.Background(), EvaluateInput{
		Input:    input,
		Binding:  "task-01",
		Platform: "claude",
		Root:     worktreeRoot, // Worktree root is /workspace, not /workspace/docs
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	// The file /workspace/docs/internal/x.go is at docs/internal/x.go relative to /workspace
	// This is outside the scope "internal/", so it should be blocked
	assert.Equal(t, DecisionBlock, result.Decision.Action,
		"relative path with subdirectory cwd should be evaluated against worktree root, not cwd")
	assert.Contains(t, result.Decision.Message, "outside task scope")
}

func TestHook_Evaluate_RelativePathNoCwdWithRoot_EvaluatedAgainstRoot(t *testing.T) {
	t.Parallel()
	// Regression test for the residual scope bypass: when the event carries no
	// cwd (so absolutizeFilePath can't resolve the relative tool path) but a
	// worktree Root was supplied, the relative path must still be evaluated
	// consistently against Root rather than silently skipping normalization.
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
