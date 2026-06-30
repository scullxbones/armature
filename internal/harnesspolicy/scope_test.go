package harnesspolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopePolicyAllowsExactFile(t *testing.T) {
	t.Parallel()
	policy := NewScopePolicy([]string{"cmd/armature/main.go"})

	result := policy.CheckPaths([]string{"cmd/armature/main.go"})

	require.True(t, result.Allowed)
	assert.Empty(t, result.Violations)
}

func TestScopePolicyAllowsDirectoryScope(t *testing.T) {
	t.Parallel()
	policy := NewScopePolicy([]string{"internal/orchestrate/"})

	result := policy.CheckPaths([]string{"internal/orchestrate/engine.go"})

	require.True(t, result.Allowed)
}

func TestScopePolicyAllowsGlobScope(t *testing.T) {
	t.Parallel()
	policy := NewScopePolicy([]string{"internal/orchestrate/*.go"})

	result := policy.CheckPaths([]string{"internal/orchestrate/engine.go"})

	require.True(t, result.Allowed)
}

func TestScopePolicyRejectsOutOfScopePath(t *testing.T) {
	t.Parallel()
	policy := NewScopePolicy([]string{"internal/orchestrate/"})

	result := policy.CheckPaths([]string{"cmd/armature/main.go"})

	require.False(t, result.Allowed)
	require.Len(t, result.Violations, 1)
	assert.Equal(t, "cmd/armature/main.go", result.Violations[0].Path)
	assert.Contains(t, result.Message(), "outside task scope")
	assert.Contains(t, result.Message(), "allowed scope")
	assert.Contains(t, result.Message(), "internal/orchestrate/")
}

func TestScopePolicyCleansTraversal(t *testing.T) {
	t.Parallel()
	policy := NewScopePolicy([]string{"internal/orchestrate/"})

	result := policy.CheckPaths([]string{"internal/orchestrate/../config/config.go"})

	require.False(t, result.Allowed)
	assert.Equal(t, "internal/config/config.go", result.Violations[0].Path)
}

func TestScopePolicyRejectsEmptyScope(t *testing.T) {
	t.Parallel()
	policy := NewScopePolicy(nil)

	result := policy.CheckPaths([]string{"internal/orchestrate/engine.go"})

	require.False(t, result.Allowed)
	assert.Contains(t, result.Message(), "task has no declared scope")
	assert.Contains(t, result.Message(), "declare scope")
}

func TestScopePolicyAllowsAbsolutePathWithinScope(t *testing.T) {
	t.Parallel()
	policy := newScopePolicyWithRoot([]string{"internal/harnesshook/"}, "/workspace/armature")

	result := policy.CheckPaths([]string{"/workspace/armature/internal/harnesshook/evaluator.go"})

	require.True(t, result.Allowed)
	assert.Empty(t, result.Violations)
}

func TestScopePolicyRejectsAbsolutePathOutsideScope(t *testing.T) {
	t.Parallel()
	policy := newScopePolicyWithRoot([]string{"internal/harnesshook/"}, "/workspace/armature")

	result := policy.CheckPaths([]string{"/workspace/armature/cmd/armature/main.go"})

	require.False(t, result.Allowed)
	require.Len(t, result.Violations, 1)
	assert.Equal(t, "cmd/armature/main.go", result.Violations[0].Path)
}

func TestScopePolicyAllowsDoubleStarGlobForNestedPaths(t *testing.T) {
	t.Parallel()
	policy := NewScopePolicy([]string{"src/**"})

	result := policy.CheckPaths([]string{"src/auth/login.go"})

	require.True(t, result.Allowed)
}

func TestScopePolicyRejectsPathOutsideDoubleStarScope(t *testing.T) {
	t.Parallel()
	policy := NewScopePolicy([]string{"src/**"})

	result := policy.CheckPaths([]string{"cmd/main.go"})

	require.False(t, result.Allowed)
}
