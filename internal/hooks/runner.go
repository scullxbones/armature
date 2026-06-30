package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/scullxbones/armature/internal/config"
)

type HookInput struct {
	IssueID    string `json:"issue_id"`
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
	WorkerID   string `json:"worker_id"`
}

type HookResult struct {
	Allowed bool   `json:"allowed"`
	Message string `json:"message"`
}

// RunPreTransition runs all pre-transition hooks defined in config.
// Returns nil if all hooks allow the transition, or an error if any hook rejects.
// Hook is called with JSON HookInput on stdin.
// Hook must output JSON HookResult on stdout.
// If hook exits non-zero or output is invalid, transition is blocked.
func RunPreTransition(cfg *config.Config, input HookInput) error {
	if cfg == nil || len(cfg.Hooks) == 0 {
		return nil
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal hook input: %w", err)
	}

	for _, hook := range cfg.Hooks {
		cmd := exec.Command("sh", "-c", hook.Command)
		cmd.Stdin = bytes.NewReader(inputJSON)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook %q failed: %s", hook.Name, err)
		}

		var result HookResult
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			return fmt.Errorf("hook %q failed: invalid output: %s", hook.Name, err)
		}

		if !result.Allowed {
			return fmt.Errorf("hook %q rejected transition: %s", hook.Name, result.Message)
		}
	}

	return nil
}
