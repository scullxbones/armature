package tui

import (
	"testing"
)

func TestIsTerminalReturnsFalseInTests(t *testing.T) {
	if IsTerminal() {
		t.Error("expected IsTerminal() to return false in test runner (no TTY attached)")
	}
}

func TestIsInteractiveReturnsFalseWhenFormatJSON(t *testing.T) {
	SetFormat("json")
	t.Cleanup(func() { SetFormat("") })
	if IsInteractive() {
		t.Error("expected IsInteractive() to return false when format=json")
	}
}

func TestIsInteractiveReturnsFalseWhenFormatAgent(t *testing.T) {
	SetFormat("agent")
	t.Cleanup(func() { SetFormat("") })
	if IsInteractive() {
		t.Error("expected IsInteractive() to return false when format=agent")
	}
}

func TestIsInteractiveReturnsFalseWhenNotTTY(t *testing.T) {
	// In the test runner stdout is never a TTY, so IsInteractive must be false
	// regardless of format.
	SetFormat("human")
	t.Cleanup(func() { SetFormat("") })
	if IsInteractive() {
		t.Error("expected IsInteractive() to return false when not a TTY")
	}
}
