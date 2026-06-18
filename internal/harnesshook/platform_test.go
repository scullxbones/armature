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

// TestClaudeAdapterWriteConfigPreservesUserManagedHooks verifies that user-managed hooks
// in PreToolUse and Stop are preserved when WriteConfig is called.
// This test verifies that Armature hooks are merged with user hooks, not replacing them.
func TestClaudeAdapterWriteConfigPreservesUserManagedHooks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))

	// Create settings with existing user hooks in the PreToolUse array
	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Bash(git status)"},
		},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "UserTool",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "user-custom-hook",
						},
					},
				},
			},
			"Stop": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "user-stop-hook",
						},
					},
				},
			},
		},
	}
	existingBytes, err := json.Marshal(existing)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), existingBytes, 0o644))

	adapter := NewClaudeAdapter()
	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))

	// The new Armature hooks should be there
	assert.Contains(t, string(data), "arm harness-hook")

	// User-managed hooks should also be preserved
	hooksRaw, ok := result["hooks"].(map[string]any)
	require.True(t, ok, "hooks should be a map")
	hooks := hooksRaw

	preToolUseRaw, ok := hooks["PreToolUse"].([]any)
	require.True(t, ok, "PreToolUse should be an array")
	preToolUseHooks := preToolUseRaw

	stopRaw, ok := hooks["Stop"].([]any)
	require.True(t, ok, "Stop should be an array")
	stopHooks := stopRaw

	// We should have BOTH the user hook (UserTool) and the Armature hook (Edit|Write|MultiEdit|Bash)
	assert.GreaterOrEqual(t, len(preToolUseHooks), 2, "user-managed PreToolUse hook should be preserved along with Armature hook")
	assert.GreaterOrEqual(t, len(stopHooks), 2, "user-managed Stop hook should be preserved along with Armature hook")

	// Check that user's custom hook is still there
	foundUserToolHook := false
	for _, h := range preToolUseHooks {
		if matcher, ok := h.(map[string]any)["matcher"].(string); ok {
			if matcher == "UserTool" {
				foundUserToolHook = true
				break
			}
		}
	}
	assert.True(t, foundUserToolHook, "user's UserTool hook should be preserved")

	// Check that user's stop hook is still there
	foundUserStopHook := false
	for _, h := range stopHooks {
		if hooks, ok := h.(map[string]any)["hooks"].([]any); ok {
			for _, hook := range hooks {
				if cmd, ok := hook.(map[string]any)["command"].(string); ok {
					if cmd == "user-stop-hook" {
						foundUserStopHook = true
						break
					}
				}
			}
		}
	}
	assert.True(t, foundUserStopHook, "user's Stop hook should be preserved")
}

// TestClaudeAdapterWriteConfigDeduplicates verifies that calling WriteConfig twice
// does not result in duplicate arm harness-hook entries.
func TestClaudeAdapterWriteConfigDeduplicates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewClaudeAdapter()

	// First call to WriteConfig
	require.NoError(t, adapter.WriteConfig(dir))

	// Second call to WriteConfig (simulating bootstrap being run twice)
	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))

	hooksRaw, ok := result["hooks"].(map[string]any)
	require.True(t, ok, "hooks should be a map")

	preToolUseRaw, ok := hooksRaw["PreToolUse"].([]any)
	require.True(t, ok, "PreToolUse should be an array")

	stopRaw, ok := hooksRaw["Stop"].([]any)
	require.True(t, ok, "Stop should be an array")

	// Count arm harness-hook entries in PreToolUse
	armHarnessHookCountPreToolUse := 0
	for _, hookEntry := range preToolUseRaw {
		hookMap, ok := hookEntry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := hookMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hooksList {
			h, ok := hook.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := h["command"].(string); ok && cmd == "arm harness-hook" {
				armHarnessHookCountPreToolUse++
			}
		}
	}

	// Count arm harness-hook entries in Stop
	armHarnessHookCountStop := 0
	for _, hookEntry := range stopRaw {
		hookMap, ok := hookEntry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := hookMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hooksList {
			h, ok := hook.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := h["command"].(string); ok && cmd == "arm harness-hook" {
				armHarnessHookCountStop++
			}
		}
	}

	assert.Equal(t, 1, armHarnessHookCountPreToolUse, "should have exactly 1 arm harness-hook in PreToolUse, not duplicates")
	assert.Equal(t, 1, armHarnessHookCountStop, "should have exactly 1 arm harness-hook in Stop, not duplicates")
}

// TestClaudeAdapterWriteConfigDeduplicatesWithUserHooks verifies that calling WriteConfig twice
// with existing user-managed hooks does not result in duplicate arm harness-hook entries.
func TestClaudeAdapterWriteConfigDeduplicatesWithUserHooks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))

	// Create settings with existing user hooks
	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Bash(git status)"},
		},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "UserTool",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "user-custom-hook",
						},
					},
				},
			},
			"Stop": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "user-stop-hook",
						},
					},
				},
			},
		},
	}
	existingBytes, err := json.Marshal(existing)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), existingBytes, 0o644))

	adapter := NewClaudeAdapter()

	// First call to WriteConfig
	require.NoError(t, adapter.WriteConfig(dir))

	// Second call to WriteConfig (simulating bootstrap being run twice)
	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))

	hooksRaw, ok := result["hooks"].(map[string]any)
	require.True(t, ok, "hooks should be a map")

	preToolUseRaw, ok := hooksRaw["PreToolUse"].([]any)
	require.True(t, ok, "PreToolUse should be an array")

	stopRaw, ok := hooksRaw["Stop"].([]any)
	require.True(t, ok, "Stop should be an array")

	// Count arm harness-hook entries in PreToolUse
	armHarnessHookCountPreToolUse := 0
	for _, hookEntry := range preToolUseRaw {
		hookMap, ok := hookEntry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := hookMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hooksList {
			h, ok := hook.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := h["command"].(string); ok && cmd == "arm harness-hook" {
				armHarnessHookCountPreToolUse++
			}
		}
	}

	// Count arm harness-hook entries in Stop
	armHarnessHookCountStop := 0
	for _, hookEntry := range stopRaw {
		hookMap, ok := hookEntry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := hookMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hooksList {
			h, ok := hook.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := h["command"].(string); ok && cmd == "arm harness-hook" {
				armHarnessHookCountStop++
			}
		}
	}

	assert.Equal(t, 1, armHarnessHookCountPreToolUse, "should have exactly 1 arm harness-hook in PreToolUse after second call, not duplicates")
	assert.Equal(t, 1, armHarnessHookCountStop, "should have exactly 1 arm harness-hook in Stop after second call, not duplicates")
}

func TestClaudeAdapterOwnsConfigAlwaysTrue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewClaudeAdapter()

	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.True(t, owns, "Claude should always own config")
}

func TestCodexAdapterOwnsConfigWhenMarkerPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "# armature:managed\n[hooks]\npre_tool_use = \"arm harness-hook\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(content), 0o600))

	adapter := NewCodexAdapter()
	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.True(t, owns, "Codex should own config when marker is present")
}

func TestCodexAdapterOwnsConfigWhenMarkerAbsent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A config with arm harness-hook but no marker is a legacy config from before the marker was introduced
	content := "[hooks]\npre_tool_use = \"arm harness-hook\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(content), 0o600))

	adapter := NewCodexAdapter()
	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.True(t, owns, "Codex should own legacy config with arm harness-hook for migration")
}

func TestCodexAdapterOwnsConfigWhenUserManaged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A config without the marker and without "arm harness-hook" is user-managed
	content := "[hooks]\npre_tool_use = \"some-other-hook\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(content), 0o600))

	adapter := NewCodexAdapter()
	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.False(t, owns, "Codex should not own user-managed config without arm harness-hook")
}

func TestCodexAdapterOwnsConfigWhenFileDoesNotExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewCodexAdapter()

	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.True(t, owns, "Codex may create config when file does not exist")
}

func TestDevinAdapterOwnsConfigWhenMarkerPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	devinDir := filepath.Join(dir, ".devin")
	require.NoError(t, os.MkdirAll(devinDir, 0o750))
	content := `{"_armature:managed": true, "hooks": {}}`
	require.NoError(t, os.WriteFile(filepath.Join(devinDir, "hooks.json"), []byte(content), 0o600))

	adapter := NewDevinAdapter()
	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.True(t, owns, "Devin should own config when marker is present")
}

func TestDevinAdapterOwnsConfigWhenMarkerAbsent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	devinDir := filepath.Join(dir, ".devin")
	require.NoError(t, os.MkdirAll(devinDir, 0o750))
	content := `{"hooks": {}}`
	require.NoError(t, os.WriteFile(filepath.Join(devinDir, "hooks.json"), []byte(content), 0o600))

	adapter := NewDevinAdapter()
	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.False(t, owns, "Devin should not own config when marker is absent")
}

func TestDevinAdapterOwnsConfigMigratesLegacyConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	devinDir := filepath.Join(dir, ".devin")
	require.NoError(t, os.MkdirAll(devinDir, 0o750))
	// Legacy config written before the _armature:managed marker was introduced.
	content := `{"hooks": {"PreToolUse": [{"matcher": "edit|exec", "command": "arm harness-hook"}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(devinDir, "hooks.json"), []byte(content), 0o600))

	adapter := NewDevinAdapter()
	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.True(t, owns, "Devin should own legacy config containing arm harness-hook for migration")
}

func TestDevinAdapterOwnsConfigWhenFileDoesNotExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewDevinAdapter()

	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.True(t, owns, "Devin may create config when file does not exist")
}

func TestCodexAdapterWriteConfigIncludesMarker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewCodexAdapter()

	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, "codex.toml"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "# armature:managed", "WriteConfig should include marker")
	assert.True(t, len(content) > 0 && content[0] == '#', "marker should be first line")
}

func TestDevinAdapterWriteConfigIncludesMarker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewDevinAdapter()

	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".devin", "hooks.json"))
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	v, ok := parsed["_armature:managed"].(bool)
	assert.True(t, ok && v, "WriteConfig should include managed marker")
}
