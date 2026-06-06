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
	adapter := NewCodexAdapter()
	event := Event{Kind: EventPreToolUse}

	out, _, err := adapter.Encode(event, Decision{Action: DecisionBlock, Message: "out of scope"})

	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Equal(t, "block", parsed["decision"], "codex requires 'block', not 'deny'")
}

func TestAdaptersExposeCapabilities(t *testing.T) {
	adapters := []PlatformAdapter{NewClaudeAdapter(), NewCodexAdapter(), NewDevinAdapter()}
	for _, adapter := range adapters {
		caps := adapter.Capabilities()
		assert.True(t, caps.PreToolUse, adapter.Name())
		assert.True(t, caps.Stop, adapter.Name())
		assert.NotEmpty(t, caps.SupportedEditTools, adapter.Name())
	}
}

func TestCodexAdapterDecodesApplyPatchPath(t *testing.T) {
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
