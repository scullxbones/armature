package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ClaudeAdapter struct{}

func NewClaudeAdapter() *ClaudeAdapter { return &ClaudeAdapter{} }

func (a *ClaudeAdapter) Name() string { return "claude" }

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

func (a *ClaudeAdapter) WriteConfig(workdir string) error {
	dir := filepath.Join(workdir, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	cfg := map[string]any{
		"hooks": map[string]any{
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
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644)
}

func (a *ClaudeAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

func (a *ClaudeAdapter) Encode(decision Decision) ([]byte, int, error) {
	if decision.Action != DecisionBlock {
		data, err := json.Marshal(map[string]any{
			"continue":       true,
			"suppressOutput": true,
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
