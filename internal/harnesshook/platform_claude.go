package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ClaudeAdapter implements PlatformAdapter for the Claude Code harness.
type ClaudeAdapter struct{}

// NewClaudeAdapter constructs a ClaudeAdapter.
func NewClaudeAdapter() *ClaudeAdapter { return &ClaudeAdapter{} }

// containsArmatureHook checks if a hooks array already contains an "arm harness-hook" entry.
// It iterates through the hooks array and checks each hook entry's nested "hooks" array
// for a command matching "arm harness-hook".
func containsArmatureHook(hooksArray []any) bool {
	for _, hookEntry := range hooksArray {
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
				return true
			}
		}
	}
	return false
}

// Name returns the platform identifier.
func (a *ClaudeAdapter) Name() string { return "claude" }

// Capabilities returns the hook event support matrix for Claude Code.
func (a *ClaudeAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{
		PreToolUse:          true,
		Stop:                true,
		BlockingStop:        true,
		ShellInterception:   "structured",
		SupportedEditTools:  []string{"Edit", "Write", "MultiEdit"},
		SupportedShellTools: []string{"Bash"},
	}
}

// WriteConfig writes the Claude Code settings.json hook configuration into workdir/.claude/.
// It merges Armature hooks with existing user-managed hooks rather than replacing them.
func (a *ClaudeAdapter) WriteConfig(workdir string) error {
	dir := filepath.Join(workdir, ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	settingsPath := filepath.Join(dir, "settings.json")
	cfg := map[string]any{}
	if existing, err := os.ReadFile(settingsPath); err == nil { //nolint:gosec // G304: internal settings path
		_ = json.Unmarshal(existing, &cfg) //nolint:errcheck // corrupt settings treated as empty and overwritten below
	}

	// Merge hooks instead of replacing them
	hooks := map[string]any{}
	if existing, ok := cfg["hooks"].(map[string]any); ok {
		hooks = existing
	}

	// Armature hooks to add
	armaturePreToolUse := map[string]any{
		"matcher": "Edit|Write|MultiEdit|Bash",
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": "arm harness-hook",
		}},
	}

	armatureStop := map[string]any{
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": "arm harness-hook",
		}},
	}

	// Merge PreToolUse hooks: append Armature hook to existing user hooks if not already present
	preToolUseHooks := []any{}
	if existing, ok := hooks["PreToolUse"].([]any); ok {
		preToolUseHooks = append(preToolUseHooks, existing...)
	}
	if !containsArmatureHook(preToolUseHooks) {
		preToolUseHooks = append(preToolUseHooks, armaturePreToolUse)
	}
	hooks["PreToolUse"] = preToolUseHooks

	// Merge Stop hooks: append Armature hook to existing user hooks if not already present
	stopHooks := []any{}
	if existing, ok := hooks["Stop"].([]any); ok {
		stopHooks = append(stopHooks, existing...)
	}
	if !containsArmatureHook(stopHooks) {
		stopHooks = append(stopHooks, armatureStop)
	}
	hooks["Stop"] = stopHooks

	cfg["hooks"] = hooks

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, data, 0o600)
}

// Decode parses a Claude Code hook payload into a normalised Event.
func (a *ClaudeAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

// Encode serialises the Decision into the JSON payload Claude Code expects on stdout.
func (a *ClaudeAdapter) Encode(event Event, decision Decision) ([]byte, int, error) {
	if decision.Action != DecisionBlock {
		data, err := json.Marshal(map[string]any{
			"continue":       true,
			"suppressOutput": true,
		})
		return data, 0, err
	}

	if event.Kind == EventStop {
		data, err := json.Marshal(map[string]any{
			"decision": "block",
			"reason":   decision.Message,
		})
		return data, 0, err
	}

	data, err := json.Marshal(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": decision.Message,
		},
	})
	return data, 0, err
}
