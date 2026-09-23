package harnesshook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const legacyCodexConfig = "[hooks]\npre_tool_use = \"arm harness-hook\"\nstop = \"arm harness-hook\"\n"

const legacyCodexConfigPath = "codex.toml"

type CodexAdapter struct{}

func NewCodexAdapter() *CodexAdapter { return &CodexAdapter{} }

func (a *CodexAdapter) Name() string { return "codex" }

func (a *CodexAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{
		PreToolUse:          true,
		Stop:                true,
		PostToolUse:         true,
		BlockingStop:        true,
		ShellInterception:   "best-effort",
		SupportedEditTools:  []string{"apply_patch", "Edit", "Write"},
		SupportedShellTools: []string{"shell", "local_shell", "Bash"},
	}
}

func (a *CodexAdapter) OwnsConfig(workdir string) (bool, error) {
	path := filepath.Join(workdir, ".codex", "config.toml")
	content, err := os.ReadFile(path) //nolint:gosec // G304: internal config path
	if err != nil {
		if os.IsNotExist(err) {
			legacyPath := filepath.Join(workdir, legacyCodexConfigPath)
			legacyContent, legacyErr := os.ReadFile(legacyPath) //nolint:gosec // G304: internal config path
			if legacyErr != nil {
				if os.IsNotExist(legacyErr) {
					return true, nil
				}
				return false, legacyErr
			}

			return codexConfigOwned(string(legacyContent)), nil
		}
		return false, err
	}
	return codexConfigOwned(string(content)), nil
}

func codexConfigOwned(content string) bool {
	if strings.TrimSpace(content) == strings.TrimSpace(legacyCodexConfig) {
		return true
	}
	firstLine, _, _ := strings.Cut(content, "\n")
	return strings.TrimSpace(firstLine) == "# armature:managed"
}

func (a *CodexAdapter) WriteConfig(workdir string) error {
	codexDir := filepath.Join(workdir, ".codex")
	if err := os.MkdirAll(codexDir, 0o750); err != nil {
		return err
	}

	content := `# armature:managed
[[hooks.PreToolUse]]
[[hooks.PreToolUse.hooks]]
type = "command"
command = "arm harness-hook"

[[hooks.PostToolUse]]
[[hooks.PostToolUse.hooks]]
type = "command"
command = "arm harness-hook"

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = "arm harness-hook"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(content), 0o600); err != nil {
		return err
	}

	legacyPath := filepath.Join(workdir, legacyCodexConfigPath)
	legacyBytes, err := os.ReadFile(legacyPath) //nolint:gosec // G304: internal config path
	if err == nil && codexConfigOwned(string(legacyBytes)) {
		_ = os.Remove(legacyPath)
	}

	return nil
}

func (a *CodexAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

func (a *CodexAdapter) Encode(_ Event, decision Decision) ([]byte, int, error) {
	return encodeApproveOrBlockJSON(decision)
}

func encodeApproveOrBlockJSON(decision Decision) ([]byte, int, error) {
	if decision.Action != DecisionBlock {
		data, err := json.Marshal(map[string]any{"decision": "approve"})
		return data, 0, err
	}
	data, err := json.Marshal(map[string]any{"decision": "block", "reason": decision.Message})
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
		ToolResponse  map[string]any `json:"tool_response"`
		Cwd           string         `json:"cwd"`
	}
	if err := json.Unmarshal(input, &raw); err != nil {
		return Event{}, err
	}

	exitCode, exitCodeKnown := ExtractExitCode(raw.ToolResponse)
	output := ExtractOutput(raw.ToolResponse)

	return Event{
		Kind:          normalizeEvent(raw.HookEventName),
		Tool:          raw.ToolName,
		Paths:         extractPaths(raw.ToolInput),
		Command:       extractCommand(raw.ToolInput),
		Cwd:           raw.Cwd,
		ToolInput:     raw.ToolInput,
		ExitCode:      exitCode,
		ExitCodeKnown: exitCodeKnown,
		Output:        output,
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
