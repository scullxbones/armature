package harnesshook

import "fmt"

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
	for k, v := range base {
		env[k] = v
	}
	env["ARMATURE_TASK_ID"] = taskID
	env["ARMATURE_HOOK_PLATFORM"] = platform
	return env
}

func adapterForPlatform(platform string) (PlatformAdapter, error) {
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
