package tui

import (
	"sync"
	"testing"
)

var ttyStateMu sync.Mutex

func lockTTYState(t *testing.T) {
	ttyStateMu.Lock()
	t.Cleanup(func() {
		SetFormat("")
		SetNonInteractive(false)
		ttyStateMu.Unlock()
	})
}

func TestIsTerminalReturnsFalseInTests(t *testing.T) {
	t.Parallel()
	if IsTerminal() {
		t.Error("expected IsTerminal() to return false in test runner (no TTY attached)")
	}
}

func TestSetNonInteractive(t *testing.T) {
	t.Parallel()
	lockTTYState(t)
	SetNonInteractive(true)
	if !IsNonInteractive() {
		t.Error("expected IsNonInteractive() to return true after SetNonInteractive(true)")
	}
}

func TestIsNonInteractive_DefaultFalse(t *testing.T) {
	t.Parallel()
	lockTTYState(t)
	SetNonInteractive(false)
	if IsNonInteractive() {
		t.Error("expected IsNonInteractive() to return false by default")
	}
}

func TestIsInteractiveReturnsFalseWhenNonInteractiveSet(t *testing.T) {
	t.Parallel()
	lockTTYState(t)
	SetNonInteractive(true)
	if IsInteractive() {
		t.Error("expected IsInteractive() to return false when non-interactive is set")
	}
}

func TestIsInteractiveReturnsFalseWhenFormatJSON(t *testing.T) {
	t.Parallel()
	lockTTYState(t)
	SetFormat("json")
	if IsInteractive() {
		t.Error("expected IsInteractive() to return false when format=json")
	}
}

func TestIsInteractiveReturnsFalseWhenFormatAgent(t *testing.T) {
	t.Parallel()
	lockTTYState(t)
	SetFormat("agent")
	if IsInteractive() {
		t.Error("expected IsInteractive() to return false when format=agent")
	}
}

func TestIsInteractiveReturnsFalseWhenNotTTY(t *testing.T) {
	t.Parallel()
	lockTTYState(t)
	// In the test runner stdout is never a TTY, so IsInteractive must be false
	// regardless of format.
	SetFormat("human")
	if IsInteractive() {
		t.Error("expected IsInteractive() to return false when not a TTY")
	}
}
