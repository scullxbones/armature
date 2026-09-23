package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ClaudeAdapter struct{}

func NewClaudeAdapter() *ClaudeAdapter { return &ClaudeAdapter{} }

func removeArmatureHooks(hooksArray []any) []any {
	var filtered []any
	for _, hookEntry := range hooksArray {
		hookMap, ok := hookEntry.(map[string]any)
		if !ok {
			filtered = append(filtered, hookEntry)
			continue
		}

		hooksList, ok := hookMap["hooks"].([]any)
		if !ok {
			filtered = append(filtered, hookEntry)
			continue
		}

		var userHooks []any
		for _, hook := range hooksList {
			h, ok := hook.(map[string]any)
			if !ok {
				userHooks = append(userHooks, hook)
				continue
			}
			if cmd, ok := h["command"].(string); ok && cmd == "arm harness-hook" {
				continue
			}
			userHooks = append(userHooks, hook)
		}

		if len(userHooks) > 0 {
			hookMap["hooks"] = userHooks
			filtered = append(filtered, hookMap)
		}
	}
	return filtered
}

func mergeArmatureHookEvent(hooks map[string]any, key string, entry map[string]any) {
	merged := []any{}
	if existing, ok := hooks[key].([]any); ok {
		merged = removeArmatureHooks(existing)
	}
	hooks[key] = append(merged, entry)
}

func (a *ClaudeAdapter) Name() string { return "claude" }

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

func (a *ClaudeAdapter) OwnsConfig(workdir string) (bool, error) {
	return true, nil
}

func (a *ClaudeAdapter) WriteConfig(workdir string) error {
	dir := filepath.Join(workdir, ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	settingsPath := filepath.Join(dir, "settings.json")
	cfg := map[string]any{}
	if existing, err := os.ReadFile(settingsPath); err == nil { //nolint:gosec // G304: internal settings path
		parsed := map[string]any{}
		if err := json.Unmarshal(existing, &parsed); err == nil {
			cfg = parsed
		}
	}

	hooks := map[string]any{}
	if existing, ok := cfg["hooks"].(map[string]any); ok {
		hooks = existing
	}

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

	mergeArmatureHookEvent(hooks, "PreToolUse", armaturePreToolUse)
	mergeArmatureHookEvent(hooks, "PostToolUse", armaturePostToolUse)
	mergeArmatureHookEvent(hooks, "Stop", armatureStop)

	cfg["hooks"] = hooks

	return writeJSONFile(settingsPath, cfg)
}

func (a *ClaudeAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

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
