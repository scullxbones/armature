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

// removeArmatureHooks removes all Armature-managed hook entries (identified by the "arm harness-hook" command)
// and returns a new array containing only user-managed hooks.
// This ensures idempotency: subsequent calls to WriteConfig won't accumulate duplicate Armature hooks.
func removeArmatureHooks(hooksArray []any) []any {
	var filtered []any
	for _, hookEntry := range hooksArray {
		hookMap, ok := hookEntry.(map[string]any)
		if !ok {
			filtered = append(filtered, hookEntry)
			continue
		}

		// Check if this hook entry contains hooks
		hooksList, ok := hookMap["hooks"].([]any)
		if !ok {
			filtered = append(filtered, hookEntry)
			continue
		}

		// Filter out "arm harness-hook" commands while preserving any other hooks in this entry
		var userHooks []any
		for _, hook := range hooksList {
			h, ok := hook.(map[string]any)
			if !ok {
				userHooks = append(userHooks, hook)
				continue
			}
			if cmd, ok := h["command"].(string); ok && cmd == "arm harness-hook" {
				// Skip the Armature-managed hook
				continue
			}
			userHooks = append(userHooks, hook)
		}

		// Only keep the entry if there are user-managed hooks remaining
		if len(userHooks) > 0 {
			hookMap["hooks"] = userHooks
			filtered = append(filtered, hookMap)
		}
		// If no user hooks remain, the entire entry is dropped (it was purely Armature-managed)
	}
	return filtered
}

// Name returns the platform identifier.
func (a *ClaudeAdapter) Name() string { return "claude" }

// Capabilities returns the hook event support matrix for Claude Code.
func (a *ClaudeAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{
		PreToolUse:          true,
		Stop:                true,
		PostToolUse:         true,
		BlockingStop:        true,
		ShellInterception:   "structured",
		SupportedEditTools:  []string{"Edit", "Write", "MultiEdit"},
		SupportedShellTools: []string{"Bash"},
	}
}

// OwnsConfig returns whether Claude owns the config in workdir.
// Claude's settings.json is always managed by Armature via JSON merge approach.
func (a *ClaudeAdapter) OwnsConfig(workdir string) (bool, error) {
	return true, nil
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

	armaturePostToolUse := map[string]any{
		"matcher": "Bash",
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

	// Merge PreToolUse hooks: remove any existing Armature-managed entries, then add the current version
	preToolUseHooks := []any{}
	if existing, ok := hooks["PreToolUse"].([]any); ok {
		preToolUseHooks = removeArmatureHooks(existing)
	}
	preToolUseHooks = append(preToolUseHooks, armaturePreToolUse)
	hooks["PreToolUse"] = preToolUseHooks

	// Merge PostToolUse hooks: remove any existing Armature-managed entries, then add the current version
	postToolUseHooks := []any{}
	if existing, ok := hooks["PostToolUse"].([]any); ok {
		postToolUseHooks = removeArmatureHooks(existing)
	}
	postToolUseHooks = append(postToolUseHooks, armaturePostToolUse)
	hooks["PostToolUse"] = postToolUseHooks

	// Merge Stop hooks: remove any existing Armature-managed entries, then add the current version
	stopHooks := []any{}
	if existing, ok := hooks["Stop"].([]any); ok {
		stopHooks = removeArmatureHooks(existing)
	}
	stopHooks = append(stopHooks, armatureStop)
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
