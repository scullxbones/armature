package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type DevinAdapter struct{}

func NewDevinAdapter() *DevinAdapter { return &DevinAdapter{} }

func (a *DevinAdapter) Name() string { return "devin" }

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

	if strings.Contains(string(data), "arm harness-hook") {
		return true, nil
	}

	return false, nil
}

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
	return writeJSONFile(filepath.Join(dir, "hooks.json"), cfg)
}

func (a *DevinAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

// Encode serialises the Decision into the JSON payload Devin expects on stdout.
func (a *DevinAdapter) Encode(_ Event, decision Decision) ([]byte, int, error) {
	// Devin processes the JSON response on exit 0, so exit code is always 0.
	return encodeApproveOrBlockJSON(decision)
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
