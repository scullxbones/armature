package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeAdapterWritesConfigCallingArmHarnessHook(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewClaudeAdapter()

	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
	assert.NotContains(t, string(data), "sandbox")
}

func TestCodexAdapterWritesConfigCallingArmHarnessHook(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewCodexAdapter()

	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, "codex.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
	assert.NotContains(t, string(data), "writable_roots")
}

func TestDevinAdapterWritesConfigCallingArmHarnessHook(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewDevinAdapter()

	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".devin", "hooks.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm")
	assert.Contains(t, string(data), "harness-hook")
	assert.NotContains(t, string(data), "permissions")
}

func TestClaudeAdapterEncodesPreToolUseBlockWithPermissionDenial(t *testing.T) {
	t.Parallel()
	adapter := NewClaudeAdapter()
	event := Event{Kind: EventPreToolUse}

	out, code, err := adapter.Encode(event, Decision{Action: DecisionBlock, Message: "blocked"})

	require.NoError(t, err)
	require.Equal(t, 0, code)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	hookOut, ok := parsed["hookSpecificOutput"].(map[string]any)
	require.True(t, ok, "expected hookSpecificOutput key")
	assert.Equal(t, "deny", hookOut["permissionDecision"])
	assert.Contains(t, hookOut["permissionDecisionReason"], "blocked")
}

func TestClaudeAdapterEncodesStopBlockWithDecisionBlock(t *testing.T) {
	t.Parallel()
	adapter := NewClaudeAdapter()
	event := Event{Kind: EventStop}

	out, code, err := adapter.Encode(event, Decision{Action: DecisionBlock, Message: "stop blocked"})

	require.NoError(t, err)
	require.Equal(t, 0, code)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Equal(t, "block", parsed["decision"])
	assert.Contains(t, parsed["reason"], "stop blocked")
	assert.NotContains(t, string(out), "hookSpecificOutput")
}

func TestCodexAdapterEncodesBlockDecisionWithBlockNotDeny(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	event := Event{Kind: EventPreToolUse}

	out, _, err := adapter.Encode(event, Decision{Action: DecisionBlock, Message: "out of scope"})

	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Equal(t, "block", parsed["decision"], "codex requires 'block', not 'deny'")
}

func TestClaudeAdapterWriteConfigPreservesExistingSettings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	existing := `{"permissions":{"allow":["Bash(git status)"]},"hooks":{}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0o644))

	adapter := NewClaudeAdapter()
	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm harness-hook")
	assert.Contains(t, string(data), `"permissions"`, "existing permissions must be preserved")
}

func TestAdaptersExposeCapabilities(t *testing.T) {
	t.Parallel()
	adapters := []PlatformAdapter{NewClaudeAdapter(), NewCodexAdapter(), NewDevinAdapter()}
	for _, adapter := range adapters {
		caps := adapter.Capabilities()
		assert.True(t, caps.PreToolUse, adapter.Name())
		assert.True(t, caps.Stop, adapter.Name())
		assert.NotEmpty(t, caps.SupportedEditTools, adapter.Name())
	}
}

func TestCodexAdapterDecodesApplyPatchPath(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	input := map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "apply_patch",
		"tool_input": map[string]any{
			"changes": []any{map[string]any{"path": "internal/harnesshook/evaluator.go"}},
		},
	}
	data, err := json.Marshal(input)
	require.NoError(t, err)

	event, err := adapter.Decode(data)

	require.NoError(t, err)
	assert.Equal(t, EventPreToolUse, event.Kind)
	assert.Equal(t, []string{"internal/harnesshook/evaluator.go"}, event.Paths)
}

func TestAdapterRegistryReturnsClaudeAdapterForEmptyPlatform(t *testing.T) {
	t.Parallel()
	adapter, err := NewAdapterForPlatform("")
	require.NoError(t, err)
	assert.Equal(t, "claude", adapter.Name())
	assert.IsType(t, (*ClaudeAdapter)(nil), adapter)
}

func TestAdapterRegistryReturnsClaudeAdapterForClaudePlatform(t *testing.T) {
	t.Parallel()
	adapter, err := NewAdapterForPlatform("claude")
	require.NoError(t, err)
	assert.Equal(t, "claude", adapter.Name())
	assert.IsType(t, (*ClaudeAdapter)(nil), adapter)
}

func TestAdapterRegistryReturnsCodexAdapterForCodexPlatform(t *testing.T) {
	t.Parallel()
	adapter, err := NewAdapterForPlatform("codex")
	require.NoError(t, err)
	assert.Equal(t, "codex", adapter.Name())
	assert.IsType(t, (*CodexAdapter)(nil), adapter)
}

func TestAdapterRegistryReturnsDevinAdapterForDevinPlatform(t *testing.T) {
	t.Parallel()
	adapter, err := NewAdapterForPlatform("devin")
	require.NoError(t, err)
	assert.Equal(t, "devin", adapter.Name())
	assert.IsType(t, (*DevinAdapter)(nil), adapter)
}

func TestAdapterRegistryErrorsOnUnknownPlatform(t *testing.T) {
	t.Parallel()
	adapter, err := NewAdapterForPlatform("unknown-platform")
	require.Error(t, err)
	assert.Nil(t, adapter)
	assert.Contains(t, err.Error(), "unknown harness hook platform")
	assert.Contains(t, err.Error(), "unknown-platform")
}
