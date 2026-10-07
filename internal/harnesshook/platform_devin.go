package harnesshook

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
)

type devinAdapter struct{}

func newDevinAdapter() *devinAdapter { return &devinAdapter{} }

func (a *devinAdapter) Name() string { return "devin" }

func (a *devinAdapter) Capabilities() PlatformCapabilities {
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

func (a *devinAdapter) OwnsConfig(workdir string) (bool, error) {
	path := filepath.Join(workdir, ".devin", "hooks.json")
	data, err := adapters.ReadFile(path)
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
	return ok && managed, nil
}

func (a *devinAdapter) WriteConfig(workdir string) error {
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

func (a *devinAdapter) Decode(input []byte) (Event, error) {
	return decodeStructuredHookEvent(input)
}

func (a *devinAdapter) Encode(_ Event, decision Decision) ([]byte, int, error) {
	return encodeApproveOrBlockJSON(decision)
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
