package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DevinAdapter implements PlatformAdapter for the Devin harness.
type DevinAdapter struct{}

// NewDevinAdapter constructs a DevinAdapter.
func NewDevinAdapter() *DevinAdapter { return &DevinAdapter{} }

// Name returns the platform identifier.
func (a *DevinAdapter) Name() string { return "devin" }

// Capabilities returns the hook event support matrix for Devin.
func (a *DevinAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{
		PreToolUse:          true,
		Stop:                true,
		PostToolUse:         true,
		BlockingStop:        true,
		ShellInterception:   "structured",
		SupportedEditTools:  []string{"edit"},
		SupportedShellTools: []string{"exec"},
	}
}

// OwnsConfig reports whether Armature may write .devin/hooks.json in workdir.
// Returns true when the file is absent (safe to create), when it contains the
// "_armature:managed" key written by WriteConfig, or when it contains
// "arm harness-hook" (legacy config written before the marker was introduced).
func (a *DevinAdapter) OwnsConfig(workdir string) (bool, error) {
	path := filepath.Join(workdir, ".devin", "hooks.json")
	data, err := os.ReadFile(path) //nolint:gosec // G304: internal config path
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false, err
	}

	managed, ok := parsed["_armature:managed"].(bool)
	if ok && managed {
		return true, nil
	}

	// Check for legacy configs written by the previous version that contain
	// "arm harness-hook" in the command but lack the managed marker.
	if strings.Contains(string(data), "arm harness-hook") {
		return true, nil
	}

	return false, nil
}

// WriteConfig writes the Devin hook configuration into workdir/.devin/hooks.json.
func (a *DevinAdapter) WriteConfig(workdir string) error {
	dir := filepath.Join(workdir, ".devin")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	cfg := map[string]any{
		"_armature:managed": true,
		"hooks": map[string]any{
			"PreToolUse": []any{map[string]any{
				"matcher": "edit|exec",
				"command": "arm harness-hook",
			}},
			"PostToolUse": []any{map[string]any{
				"matcher": "exec",
				"command": "arm harness-hook",
			}},
			"Stop": []any{map[string]any{
				"command": "arm harness-hook",
			}},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "hooks.json"), data, 0o600)
}

// Decode parses a Devin hook payload into a normalised Event.
func (a *DevinAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

// Encode serialises the Decision into the JSON payload Devin expects on stdout.
func (a *DevinAdapter) Encode(_ Event, decision Decision) ([]byte, int, error) {
	if decision.Action != DecisionBlock {
		data, err := json.Marshal(map[string]any{"decision": "approve"})
		return data, 0, err
	}
	data, err := json.Marshal(map[string]any{"decision": "block", "reason": decision.Message})
	// Devin processes the JSON response on exit 0, so exit code is always 0.
	return data, 0, err
}
