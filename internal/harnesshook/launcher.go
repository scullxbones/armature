package harnesshook

import "fmt"

// NewAdapterForPlatform is the single registry for platform adapter selection.
// The hook runner and bootstrap install path both go through here so adapter
// instantiation cannot drift across the harness-hook subsystem.
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
