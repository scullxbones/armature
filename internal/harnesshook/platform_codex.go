package harnesshook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// legacyCodexConfig is the exact pre-marker body written by WriteConfig at the root
// codex.toml before the new .codex/config.toml location was introduced. OwnsConfig
// matches against this string (after trimming whitespace) so that only the known
// legacy config is silently migrated, and user-authored files that merely mention
// "arm harness-hook" are left untouched. Root codex.toml files that carry the
// "# armature:managed" first-line marker are handled separately via the first-line
// check in OwnsConfig, not by this constant.
const legacyCodexConfig = "[hooks]\npre_tool_use = \"arm harness-hook\"\nstop = \"arm harness-hook\"\n"

// legacyCodexConfigPath is the old location where codex.toml was written at the root
const legacyCodexConfigPath = "codex.toml"

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

// OwnsConfig reports whether Armature may write .codex/config.toml in workdir.
// Returns true when the file is absent (safe to create), when the first line
// is the "# armature:managed" marker written by WriteConfig, or when the file
// is exactly the legacy config body (an exact match against legacyCodexConfig,
// trimming surrounding whitespace) written before the marker was introduced.
// An exact match is used instead of substring search so that user-authored
// files that merely mention "arm harness-hook" are never silently overwritten.
// Also recognizes the old legacy config at the root codex.toml for migration.
func (a *CodexAdapter) OwnsConfig(workdir string) (bool, error) {
	path := filepath.Join(workdir, ".codex", "config.toml")
	content, err := os.ReadFile(path) //nolint:gosec // G304: internal config path
	if err != nil {
		if os.IsNotExist(err) {
			// Check for old legacy config at root codex.toml for migration support
			legacyPath := filepath.Join(workdir, legacyCodexConfigPath)
			legacyContent, legacyErr := os.ReadFile(legacyPath) //nolint:gosec // G304: internal config path
			if legacyErr != nil {
				if os.IsNotExist(legacyErr) {
					return true, nil
				}
				return false, legacyErr
			}

			// Check if the legacy file is the old format owned by armature.
			// Two cases: pre-marker exact body, or marker-bearing file (earlier
			// commits wrote "# armature:managed" to root codex.toml directly).
			legacyContentStr := string(legacyContent)
			if strings.TrimSpace(legacyContentStr) == strings.TrimSpace(legacyCodexConfig) {
				return true, nil
			}
			firstLine := strings.SplitN(legacyContentStr, "\n", 2)[0]
			if strings.TrimSpace(firstLine) == "# armature:managed" {
				return true, nil
			}

			// Legacy file exists but is user-managed
			return false, nil
		}
		return false, err
	}

	contentStr := string(content)

	// Check for the marker at the beginning of the file
	firstLine := strings.SplitN(contentStr, "\n", 2)[0]
	if strings.TrimSpace(firstLine) == "# armature:managed" {
		return true, nil
	}

	// Check for legacy configs written by the previous version that exactly
	// match the known legacy body but lack the marker.
	if strings.TrimSpace(contentStr) == strings.TrimSpace(legacyCodexConfig) {
		return true, nil
	}

	return false, nil
}

// WriteConfig writes the Codex hook configuration into workdir/.codex/config.toml.
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

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = "arm harness-hook"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(content), 0o600); err != nil {
		return err
	}

	// Remove any stale root codex.toml that was written by an earlier version of
	// WriteConfig (before the .codex/ subdirectory location was adopted). We only
	// remove it when it is armature-owned: either the pre-marker exact body or a
	// file whose first line is "# armature:managed".
	legacyPath := filepath.Join(workdir, legacyCodexConfigPath)
	legacyBytes, err := os.ReadFile(legacyPath) //nolint:gosec // G304: internal config path
	if err == nil {
		legacyStr := string(legacyBytes)
		firstLine := strings.SplitN(legacyStr, "\n", 2)[0]
		owned := strings.TrimSpace(legacyStr) == strings.TrimSpace(legacyCodexConfig) ||
			strings.TrimSpace(firstLine) == "# armature:managed"
		if owned {
			os.Remove(legacyPath) //nolint:errcheck,gosec // G104: best-effort cleanup; failure leaves a stale but harmless file
		}
	}

	return nil
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
