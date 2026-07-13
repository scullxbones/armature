// Package hooks runs armature's git hooks (e.g. pre-commit/pre-push checks) against the working tree.
package hooks

import (
	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
)

// RunPreTransition runs all pre-transition hooks defined in config.
// Returns nil if all hooks allow the transition, or an error if any hook rejects.
// Hook is called with JSON HookInput on stdin.
// Hook must output JSON HookResult on stdout.
// If hook exits non-zero or output is invalid, transition is blocked.
func RunPreTransition(cfg *config.Config, input adapters.HookInput) error {
	if cfg == nil || len(cfg.Hooks) == 0 {
		return nil
	}

	for _, hook := range cfg.Hooks {
		if err := adapters.ExecuteHook(hook.Name, hook.Command, input); err != nil {
			return err
		}
	}

	return nil
}
