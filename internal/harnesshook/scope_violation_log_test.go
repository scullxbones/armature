package harnesshook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScopeViolationLogging_REQ_TOPTIER_S5_T2 verifies that out-of-scope
// operations are logged with a "violation:" marker even when the hook's
// ultimate action is a pass-through (enforcement skipped), not just on block.
// This exercises LogPassThroughScopeViolation directly against a scope policy
// that declares a narrow scope and an event touching an out-of-scope path,
// simulating the stale-binding pass-through scenario documented in
// docs/harness-hook.md's "Scope Violation Visibility" section.
func TestScopeViolationLogging_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	scopePolicy := harnesspolicy.NewScopePolicyWithRoot([]string{"internal/"}, tmpDir)

	// Simulate a pass-through scenario (e.g. a stale binding skips
	// enforcement) where the event still touches an out-of-scope path.
	result, err := LogPassThroughScopeViolation(gitDir, scopePolicy, []string{"cmd/main.go"}, "stale binding")
	require.NoError(t, err)
	assert.False(t, result.Allowed, "expected the out-of-scope path to be flagged")
	require.NotEmpty(t, result.Violations)
	assert.Equal(t, "cmd/main.go", result.Violations[0].Path)

	logPath := filepath.Join(gitDir, "armature-hook.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err, "expected armature-hook.log to be written")

	logContent := string(data)
	assert.Contains(t, logContent, "violation:", "expected a violation: marker even though the hook passed through")
	assert.Contains(t, logContent, "cmd/main.go", "expected the out-of-scope path to be recorded")
	assert.Contains(t, logContent, "stale binding", "expected the pass-through reason to be recorded")

	// Sanity check: an in-scope path should not produce a violation line.
	tmpDir2 := t.TempDir()
	gitDir2 := filepath.Join(tmpDir2, ".git")
	require.NoError(t, os.MkdirAll(gitDir2, 0o755))
	inScopePolicy := harnesspolicy.NewScopePolicyWithRoot([]string{"internal/"}, tmpDir2)
	result2, err := LogPassThroughScopeViolation(gitDir2, inScopePolicy, []string{"internal/foo.go"}, "stale binding")
	require.NoError(t, err)
	assert.True(t, result2.Allowed)
	_, statErr := os.Stat(filepath.Join(gitDir2, "armature-hook.log"))
	assert.True(t, os.IsNotExist(statErr), "expected no log file to be written when the path is in scope")
}
