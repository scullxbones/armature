package orchestrate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHarnessAdapterInvalidName verifies that an unknown adapter name returns an error.
func TestNewHarnessAdapterInvalidName(t *testing.T) {
	_, err := NewHarnessAdapter(HarnessConfig{Adapter: "unknown"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown harness")
}

// TestNewHarnessAdapterClaude verifies that "claude" returns an adapter named "claude".
func TestNewHarnessAdapterClaude(t *testing.T) {
	a, err := NewHarnessAdapter(HarnessConfig{Adapter: "claude", Model: "claude-haiku-4-5", Timeout: 600})
	require.NoError(t, err)
	assert.Equal(t, "claude", a.Name())
}

// TestNewHarnessAdapterCodex verifies that "codex" returns an adapter named "codex".
func TestNewHarnessAdapterCodex(t *testing.T) {
	a, err := NewHarnessAdapter(HarnessConfig{Adapter: "codex", Model: "o4-mini"})
	require.NoError(t, err)
	assert.Equal(t, "codex", a.Name())
}

// TestNewHarnessAdapterDevin verifies that "devin" returns an adapter named "devin".
func TestNewHarnessAdapterDevin(t *testing.T) {
	a, err := NewHarnessAdapter(HarnessConfig{Adapter: "devin"})
	require.NoError(t, err)
	assert.Equal(t, "devin", a.Name())
}

// TestSandboxAvailable checks sandbox probing — skips if neither bwrap nor sandbox-exec is present.
func TestSandboxAvailable(t *testing.T) {
	ok, _ := SandboxAvailable()
	if !ok {
		t.Skip("sandbox not available on this host")
	}
	assert.True(t, ok)
}

// TestWriteClaudeSettings verifies settings.json is written correctly.
// Crucially, it must NOT contain dangerouslySkipPermissions.
func TestWriteClaudeSettings(t *testing.T) {
	dir := t.TempDir()
	scope := []string{"internal/dag/", "internal/validate/"}
	err := writeClaudeSettings(dir, scope)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)

	// Must NOT contain dangerouslySkipPermissions.
	assert.NotContains(t, string(data), "dangerouslySkipPermissions")

	// Must contain the scope paths.
	assert.Contains(t, string(data), "internal/dag/")
	assert.Contains(t, string(data), "internal/validate/")

	// Must be valid JSON.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
}

// TestWriteClaudeSettingsEmptyScope verifies that an empty scope returns an error.
func TestWriteClaudeSettingsEmptyScope(t *testing.T) {
	dir := t.TempDir()
	err := writeClaudeSettings(dir, nil)
	assert.Error(t, err)
}

// TestWriteCodexConfig verifies codex config.toml is written.
func TestWriteCodexConfig(t *testing.T) {
	dir := t.TempDir()
	scope := []string{"internal/dag/", "internal/validate/"}
	err := writeCodexConfig(dir, scope)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "codex.toml"))
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "internal/dag/")
	assert.Contains(t, content, "internal/validate/")
}

// TestWriteDevinConfig verifies devin config.json is written.
func TestWriteDevinConfig(t *testing.T) {
	dir := t.TempDir()
	scope := []string{"internal/dag/"}
	err := writeDevinConfig(dir, scope)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".devin", "config.json"))
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	content := string(data)
	assert.Contains(t, content, "internal/dag/")
}

// TestWithIssueScope verifies scope injection and retrieval via context.
func TestWithIssueScope(t *testing.T) {
	scope := []string{"internal/foo/", "internal/bar/"}
	ctx := WithIssueScope(context.Background(), scope)
	got := issueFromCtx(ctx)
	assert.Equal(t, scope, got.Scope)
}

// TestIssueFromCtxEmpty verifies issueFromCtx returns empty scope for a plain context.
func TestIssueFromCtxEmpty(t *testing.T) {
	got := issueFromCtx(context.Background())
	assert.Empty(t, got.Scope)
}

// TestValidateIssueScope verifies that an empty scope returns an error.
func TestValidateIssueScope(t *testing.T) {
	assert.Error(t, validateIssueScope(nil))
	assert.Error(t, validateIssueScope([]string{}))
	assert.NoError(t, validateIssueScope([]string{"internal/foo/"}))
}

func TestBuildClaudeLaunchArgs_NonInteractive(t *testing.T) {
	args := buildClaudeLaunchArgs("claude-sonnet-4-6", "do the task")
	assert.Equal(t, "claude", args[0])
	assert.Contains(t, args, "--print")
	assert.Contains(t, args, "--output-format")
	assert.Contains(t, args, "text")
	assert.Contains(t, args, "--model")
	assert.Equal(t, "do the task", args[len(args)-1])
}

func TestBuildCodexLaunchArgs_NonInteractive(t *testing.T) {
	args := buildCodexLaunchArgs("gpt-5", "do the task")
	assert.Equal(t, "codex", args[0])
	assert.Equal(t, "exec", args[1])
	assert.Contains(t, args, "--color")
	assert.Contains(t, args, "never")
	assert.Contains(t, args, "--model")
	assert.Equal(t, "do the task", args[len(args)-1])
}

func TestBuildHarnessPrompt_IncludesScope(t *testing.T) {
	prompt := buildHarnessPrompt([]string{"internal/orchestrate/", "cmd/armature/"})
	assert.Contains(t, prompt, defaultHarnessPrompt)
	assert.Contains(t, prompt, "internal/orchestrate/")
	assert.Contains(t, prompt, "cmd/armature/")
}

func TestCodexAdapterRunDryRun(t *testing.T) {
	dir := t.TempDir()
	a, err := NewHarnessAdapter(HarnessConfig{Adapter: "codex"})
	require.NoError(t, err)

	ctx := WithIssueScope(context.Background(), []string{"internal/foo/"})
	result, err := a.Run(ctx, HarnessConfig{WorkDir: dir}, RunOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, "codex", result.Name)
	assert.True(t, result.Passed)
}

// TestBuildSandboxCmd verifies that buildSandboxCmd returns a non-empty command list.
func TestBuildSandboxCmd(t *testing.T) {
	result := buildSandboxCmd("/tmp/worktree", []string{"echo", "hello"})
	assert.NotEmpty(t, result)
	// Must contain the original command somewhere in the result.
	found := false
	for _, arg := range result {
		if arg == "echo" {
			found = true
			break
		}
	}
	assert.True(t, found, "buildSandboxCmd should include the original command args")
}

// TestClaudeAdapterRunDryRun verifies that Run returns a result without panicking
// when DryRun is true (no actual process spawned).
func TestClaudeAdapterRunDryRun(t *testing.T) {
	dir := t.TempDir()
	a, err := NewHarnessAdapter(HarnessConfig{Adapter: "claude"})
	require.NoError(t, err)

	ctx := WithIssueScope(context.Background(), []string{"internal/foo/"})
	result, err := a.Run(ctx, HarnessConfig{WorkDir: dir}, RunOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, "claude", result.Name)
	assert.True(t, result.Passed)
}

// TestCodexAdapterRunDryRun verifies that Run returns a result without panicking
// when DryRun is true.
// TestDevinAdapterRunDryRun verifies that Run returns a result without panicking
// when DryRun is true.
func TestDevinAdapterRunDryRun(t *testing.T) {
	dir := t.TempDir()
	a, err := NewHarnessAdapter(HarnessConfig{Adapter: "devin"})
	require.NoError(t, err)

	ctx := WithIssueScope(context.Background(), []string{"internal/foo/"})
	result, err := a.Run(ctx, HarnessConfig{WorkDir: dir}, RunOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, "devin", result.Name)
	assert.True(t, result.Passed)
}

// TestInvokeProcessDryRun verifies that dry-run returns ExitSuccess with no process spawned.
func TestInvokeProcessDryRun(t *testing.T) {
	result, err := invokeProcess(context.Background(), t.TempDir(), []string{"false"}, true)
	require.NoError(t, err)
	assert.Equal(t, ExitSuccess, result.Status)
}

// TestInvokeProcessSuccess verifies that a successful command yields ExitSuccess.
func TestInvokeProcessSuccess(t *testing.T) {
	result, err := invokeProcess(context.Background(), t.TempDir(), []string{"echo", "hello"}, false)
	require.NoError(t, err)
	assert.Equal(t, ExitSuccess, result.Status)
	assert.Equal(t, 0, result.ExitCode)
}

// TestInvokeProcessFailure verifies that a failing command yields ExitFailure.
func TestInvokeProcessFailure(t *testing.T) {
	result, _ := invokeProcess(context.Background(), t.TempDir(), []string{"false"}, false)
	assert.Equal(t, ExitFailure, result.Status)
}

// TestInvokeProcessTimeout verifies that context cancellation yields ExitTimeout.
func TestInvokeProcessTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	result, _ := invokeProcess(ctx, t.TempDir(), []string{"sleep", "10"}, false)
	assert.Equal(t, ExitTimeout, result.Status)
}

// TestErrAsMissingInterface verifies errAs returns false when err lacks ExitCode().
func TestErrAsMissingInterface(t *testing.T) {
	var target interface{ ExitCode() int }
	err := assert.AnError
	ok := errAs(err, &target)
	assert.False(t, ok)
}

// TestClaudeAdapterRunEmptyScope verifies Run returns error when scope is empty.
func TestClaudeAdapterRunEmptyScope(t *testing.T) {
	dir := t.TempDir()
	a, err := NewHarnessAdapter(HarnessConfig{Adapter: "claude"})
	require.NoError(t, err)

	ctx := context.Background() // no scope injected
	result, err := a.Run(ctx, HarnessConfig{WorkDir: dir}, RunOptions{DryRun: false})
	assert.Error(t, err)
	assert.False(t, result.Passed)
}

// TestCodexAdapterRunEmptyScope verifies Run returns error when scope is empty.
func TestCodexAdapterRunEmptyScope(t *testing.T) {
	dir := t.TempDir()
	a, err := NewHarnessAdapter(HarnessConfig{Adapter: "codex"})
	require.NoError(t, err)

	ctx := context.Background()
	result, err := a.Run(ctx, HarnessConfig{WorkDir: dir}, RunOptions{DryRun: false})
	assert.Error(t, err)
	assert.False(t, result.Passed)
}

// TestDevinAdapterRunEmptyScope verifies Run returns error when scope is empty.
func TestDevinAdapterRunEmptyScope(t *testing.T) {
	dir := t.TempDir()
	a, err := NewHarnessAdapter(HarnessConfig{Adapter: "devin"})
	require.NoError(t, err)

	ctx := context.Background()
	result, err := a.Run(ctx, HarnessConfig{WorkDir: dir}, RunOptions{DryRun: false})
	assert.Error(t, err)
	assert.False(t, result.Passed)
}
