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

	cfg["hooks"] = map[string]any{
		"PreToolUse": []any{map[string]any{
			"matcher": "Edit|Write|MultiEdit|Bash",
			"hooks": []any{map[string]any{
				"type":    "command",
				"command": "arm harness-hook",
			}},
		}},
		"Stop": []any{map[string]any{
			"hooks": []any{map[string]any{
				"type":    "command",
				"command": "arm harness-hook",
			}},
		}},
	}
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
