package harnesshook

import (
	"fmt"
	"maps"
)

type Launcher struct{}

func NewLauncher() *Launcher {
	return &Launcher{}
}

func (l *Launcher) Install(workdir, platform string) error {
	adapter, err := adapterForPlatform(platform)
	if err != nil {
		return err
	}
	return adapter.WriteConfig(workdir)
}

func (l *Launcher) BuildEnv(base map[string]string, taskID, platform string) map[string]string {
	env := make(map[string]string, len(base)+2)
	maps.Copy(env, base)
	env["ARMATURE_TASK_ID"] = taskID
	env["ARMATURE_HOOK_PLATFORM"] = platform
	return env
}

// NewAdapterForPlatform is the single registry for platform adapter selection.
// It is used by both the launcher and the hook runner to ensure consistent
// adapter instantiation across the harness-hook subsystem.
func NewAdapterForPlatform(platform string) (PlatformAdapter, error) {
	switch platform {
	case "", "claude":
		return NewClaudeAdapter(), nil
	case "codex":
		return NewCodexAdapter(), nil
	case "devin":
		return NewDevinAdapter(), nil
	default:
		return nil, fmt.Errorf("unknown harness hook platform %q", platform)
	}
}

// adapterForPlatform is deprecated; use NewAdapterForPlatform instead.
func adapterForPlatform(platform string) (PlatformAdapter, error) {
	return NewAdapterForPlatform(platform)
}
