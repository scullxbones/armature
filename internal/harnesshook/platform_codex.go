package harnesshook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type CodexAdapter struct{}

func NewCodexAdapter() *CodexAdapter { return &CodexAdapter{} }

func (a *CodexAdapter) Name() string { return "codex" }

func (a *CodexAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{
		PreToolUse:          true,
		Stop:                true,
		BlockingStop:        true,
		ShellInterception:   "best-effort",
		SupportedEditTools:  []string{"apply_patch", "Edit", "Write"},
		SupportedShellTools: []string{"Bash"},
	}
}

func (a *CodexAdapter) WriteConfig(workdir string) error {
	content := "[hooks]\npre_tool_use = \"arm harness-hook\"\nstop = \"arm harness-hook\"\n"
	return os.WriteFile(filepath.Join(workdir, "codex.toml"), []byte(content), 0o644)
}

func (a *CodexAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

func (a *CodexAdapter) Encode(_ Event, decision Decision) ([]byte, int, error) {
	if decision.Action != DecisionBlock {
		data, err := json.Marshal(map[string]any{"decision": "approve"})
		return data, 0, err
	}
	data, err := json.Marshal(map[string]any{"decision": "block", "reason": decision.Message})
	// Codex processes the JSON response on exit 0, so exit code is always 0.
	return data, 0, err
}

func normalizeEvent(name string) EventKind {
	switch name {
	case "PreToolUse", "pre_tool_use":
		return EventPreToolUse
	case "PostToolUse", "post_tool_use":
		return EventPostToolUse
	case "Stop", "stop":
		return EventStop
	default:
		return EventKind(name)
	}
}

func decodeStructuredHookEvent(input []byte) (Event, error) {
	var raw struct {
		HookEventName string         `json:"hook_event_name"`
		ToolName      string         `json:"tool_name"`
		ToolInput     map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal(input, &raw); err != nil {
		return Event{}, err
	}

	return Event{
		Kind:    normalizeEvent(raw.HookEventName),
		Tool:    raw.ToolName,
		Paths:   extractPaths(raw.ToolInput),
		Command: extractCommand(raw.ToolInput),
	}, nil
}

func extractPaths(input map[string]any) []string {
	if input == nil {
		return nil
	}
	for _, key := range []string{"file_path", "path"} {
		if value, ok := input[key].(string); ok && value != "" {
			return []string{value}
		}
	}
	if changes, ok := input["changes"].([]any); ok {
		paths := make([]string, 0, len(changes))
		for _, item := range changes {
			change, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if path, ok := change["path"].(string); ok && path != "" {
				paths = append(paths, path)
			}
		}
		return paths
	}
	return nil
}

func extractCommand(input map[string]any) string {
	if input == nil {
		return ""
	}
	for _, key := range []string{"command", "cmd"} {
		if value, ok := input[key].(string); ok {
			return value
		}
	}
	return fmt.Sprint(input["input"])
}
