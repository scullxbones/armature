package tui

import (
	"testing"
)

func TestIsTerminalReturnsFalseInTests(t *testing.T) {
	if IsTerminal() {
		t.Error("expected IsTerminal() to return false in test runner (no TTY attached)")
	}
}
