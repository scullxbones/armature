package harnesshook

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CodexAdapter implements PlatformAdapter for the OpenAI Codex harness.
type CodexAdapter struct{}

// NewCodexAdapter constructs a CodexAdapter.
func NewCodexAdapter() *CodexAdapter { return &CodexAdapter{} }

// Name returns the platform identifier.
func (a *CodexAdapter) Name() string { return "codex" }

// Capabilities returns the hook event support matrix for Codex.
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

// OwnsConfig reports whether Armature may write codex.toml in workdir.
// Returns true when the file is absent (safe to create), when the first line
// is the "# armature:managed" marker written by WriteConfig, or when the file
// contains "arm harness-hook" (legacy config written before the marker was introduced).
func (a *CodexAdapter) OwnsConfig(workdir string) (bool, error) {
	path := filepath.Join(workdir, "codex.toml")
	content, err := os.ReadFile(path) //nolint:gosec // G304: internal config path
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}

	contentStr := string(content)

	// Check for the marker at the beginning of the file
	scanner := bufio.NewScanner(strings.NewReader(contentStr))
	if scanner.Scan() {
		firstLine := scanner.Text()
		if strings.TrimSpace(firstLine) == "# armature:managed" {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}

	// Check for legacy configs written by the previous version that contain
	// "arm harness-hook" in the command but lack the marker
	if strings.Contains(contentStr, "arm harness-hook") {
		return true, nil
	}

	return false, nil
}

// WriteConfig writes the Codex hook configuration into workdir/codex.toml.
func (a *CodexAdapter) WriteConfig(workdir string) error {
	content := "# armature:managed\n[hooks]\npre_tool_use = \"arm harness-hook\"\nstop = \"arm harness-hook\"\n"
	return os.WriteFile(filepath.Join(workdir, "codex.toml"), []byte(content), 0o600)
}

// Decode parses a Codex hook payload into a normalised Event.
func (a *CodexAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

// Encode serialises the Decision into the JSON payload Codex expects on stdout.
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
