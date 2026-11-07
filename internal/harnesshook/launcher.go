package harnesshook

import "fmt"

// NewAdapterForPlatform is the single registry for platform adapter selection.
func NewAdapterForPlatform(platform string) (platformAdapter, error) {
	switch platform {
	case "", "claude":
		return newClaudeAdapter(), nil
	case "codex":
		return newCodexAdapter(), nil
	case "devin":
		return newDevinAdapter(), nil
	default:
		return nil, fmt.Errorf("unknown harness hook platform %q", platform)
	}
}
