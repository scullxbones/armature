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

	data, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
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
	codexDir := filepath.Join(dir, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0o755))
	content := "# armature:managed\n[[hooks.PreToolUse]]\n"
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(content), 0o600))

	adapter := NewCodexAdapter()
	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.True(t, owns, "Codex should own config when marker is present")
}

func TestCodexAdapterOwnsConfigWhenMarkerAbsent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A config at the old root location that is exactly the legacy body must be recognised as owned for migration
	content := "[hooks]\npre_tool_use = \"arm harness-hook\"\nstop = \"arm harness-hook\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(content), 0o600))

	adapter := NewCodexAdapter()
	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.True(t, owns, "Codex should own legacy config at root for migration")
}

func TestCodexAdapterDoesNotOwnUserConfigMentioningArmHarnessHook(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A user-authored file at the old root location that merely mentions "arm harness-hook" (e.g. in a comment) but
	// is NOT the exact legacy config body must NOT be treated as owned — otherwise WriteConfig
	// would silently truncate a user-managed file.
	content := "# my notes about arm harness-hook\n[hooks]\npre_tool_use = \"my-own-tool\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(content), 0o600))

	adapter := NewCodexAdapter()
	owns, err := adapter.OwnsConfig(dir)

	require.NoError(t, err)
	assert.False(t, owns, "Codex must not own a user file at root that merely mentions arm harness-hook")
}

func TestCodexAdapterOwnsConfigWhenUserManaged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A config at the new location without the marker and without "arm harness-hook" is user-managed
	codexDir := filepath.Join(dir, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0o755))
	content := "[hooks]\npre_tool_use = \"some-other-hook\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(content), 0o600))

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
	assert.True(t, owns, "Codex may create config when files do not exist")
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

	data, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
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

// TestCodexAdapterMigratesLegacyRootConfigToNewPath verifies the full migration path:
// a pre-marker legacy root codex.toml is recognised as owned, WriteConfig creates
// the new .codex/config.toml with the marker, and the root codex.toml is removed.
func TestCodexAdapterMigratesLegacyRootConfigToNewPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewCodexAdapter()

	// 1. Write the legacy body (no marker) to <dir>/codex.toml
	legacyBody := "[hooks]\npre_tool_use = \"arm harness-hook\"\nstop = \"arm harness-hook\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(legacyBody), 0o600))

	// 2. OwnsConfig should return true
	owns, err := adapter.OwnsConfig(dir)
	require.NoError(t, err)
	assert.True(t, owns, "OwnsConfig should recognise pre-marker legacy root file as owned")

	// 3. WriteConfig should succeed
	require.NoError(t, adapter.WriteConfig(dir))

	// 4. .codex/config.toml must now exist and contain the marker
	data, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "# armature:managed", "new config must contain marker")

	// 5. Root codex.toml must have been removed
	_, statErr := os.Stat(filepath.Join(dir, "codex.toml"))
	assert.True(t, os.IsNotExist(statErr), "legacy root codex.toml must be removed after migration")
}

// TestCodexAdapterMigratesMarkerBearingRootConfig verifies that a root codex.toml
// carrying the "# armature:managed" marker (written by earlier commits before the
// .codex/ location was adopted) is also recognised as owned and cleaned up on migration.
func TestCodexAdapterMigratesMarkerBearingRootConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	adapter := NewCodexAdapter()

	// Write a marker-bearing root codex.toml (the format earlier commits produced)
	markerBody := "# armature:managed\n[hooks]\npre_tool_use = \"arm harness-hook\"\nstop = \"arm harness-hook\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(markerBody), 0o600))

	// OwnsConfig should return true
	owns, err := adapter.OwnsConfig(dir)
	require.NoError(t, err)
	assert.True(t, owns, "OwnsConfig should recognise marker-bearing root file as owned")

	// WriteConfig should succeed
	require.NoError(t, adapter.WriteConfig(dir))

	// .codex/config.toml must now exist and contain the marker
	data, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "# armature:managed", "new config must contain marker")

	// Root codex.toml must have been removed
	_, statErr := os.Stat(filepath.Join(dir, "codex.toml"))
	assert.True(t, os.IsNotExist(statErr), "marker-bearing root codex.toml must be removed after migration")
}

func TestCodexAdapterEncodeApproveDecision(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	event := Event{Kind: EventPreToolUse}

	out, code, err := adapter.Encode(event, Decision{Action: DecisionAllow})
	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, string(out), `"approve"`)
}

func TestCodexAdapterNormalizeEventPostToolUse(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	payload := []byte(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, EventPostToolUse, evt.Kind)
}

func TestCodexAdapterNormalizeEventPostToolUseLower(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	payload := []byte(`{"hook_event_name":"post_tool_use","tool_name":"Bash","tool_input":{}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, EventPostToolUse, evt.Kind)
}

func TestCodexAdapterNormalizeEventStop(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	payload := []byte(`{"hook_event_name":"Stop","tool_name":"","tool_input":{}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, EventStop, evt.Kind)
}

func TestCodexAdapterNormalizeEventStopLower(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	payload := []byte(`{"hook_event_name":"stop","tool_name":"","tool_input":{}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, EventStop, evt.Kind)
}

func TestCodexAdapterNormalizeEventUnknown(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	payload := []byte(`{"hook_event_name":"CustomEvent","tool_name":"","tool_input":{}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, EventKind("CustomEvent"), evt.Kind)
}

func TestCodexAdapterExtractPathsWithPathKey(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	payload := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"path":"/some/file.go"}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, []string{"/some/file.go"}, evt.Paths)
}

func TestCodexAdapterExtractPathsWithChangesArray(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	payload := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"changes":[{"path":"a.go"},{"path":"b.go"}]}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.go", "b.go"}, evt.Paths)
}

func TestCodexAdapterExtractCommandWithCmdKey(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	payload := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"cmd":"ls -la"}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, "ls -la", evt.Command)
}

func TestCodexAdapterExtractCommandFallback(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	// No "command" or "cmd" key — falls back to fmt.Sprint(input["input"])
	payload := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"input":"something"}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, "something", evt.Command)
}

func TestDevinAdapterDecode_PreToolUse(t *testing.T) {
	t.Parallel()
	adapter := NewDevinAdapter()
	payload := []byte(`{"hook_event_name":"PreToolUse","tool_name":"edit","tool_input":{}}`)

	evt, err := adapter.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, EventPreToolUse, evt.Kind)
}

func TestDevinAdapterEncode_ApproveDecision(t *testing.T) {
	t.Parallel()
	adapter := NewDevinAdapter()

	data, exitCode, err := adapter.Encode(Event{}, Decision{Action: DecisionAllow})
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, string(data), "approve")
}

func TestDevinAdapterEncode_BlockDecision(t *testing.T) {
	t.Parallel()
	adapter := NewDevinAdapter()

	data, exitCode, err := adapter.Encode(Event{}, Decision{Action: DecisionBlock, Message: "blocked"})
	require.NoError(t, err)
	// Devin processes the response on exit 0 always.
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, string(data), "block")
}
