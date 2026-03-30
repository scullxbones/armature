package tui

import (
	"os"

	"golang.org/x/term"
)

// currentFormat holds the output format set by SetFormat.
var currentFormat string

// SetFormat stores the current output format for use by IsInteractive.
func SetFormat(f string) {
	currentFormat = f
}

// IsTerminal returns true if stdout is connected to a TTY.
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsInteractive returns true only when stdout is a TTY, the output format
// is neither "json" nor "agent", and we're not running in a known agent environment.
// It is safe to call before SetFormat; in that case currentFormat is "" which
// is treated as interactive.
func IsInteractive() bool {
	if os.Getenv("GEMINI_CLI") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return IsTerminal() && currentFormat != "json" && currentFormat != "agent"
}
