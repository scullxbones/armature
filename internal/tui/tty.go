package tui

import (
	"os"

	"golang.org/x/term"
)

var currentFormat string

var nonInteractive bool

func SetFormat(f string) {
	currentFormat = f
}

// SetNonInteractive records whether the --non-interactive flag was set (or
// auto-derived from format/TTY state).
func SetNonInteractive(v bool) {
	nonInteractive = v
}

// IsNonInteractive returns true when TUI commands must skip BubbleTea and
// emit structured output instead.
func IsNonInteractive() bool {
	return nonInteractive
}

func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsInteractive returns true only when stdout is a TTY, the output format
// is neither "json" nor "agent", we're not running in a known agent environment,
// and the --non-interactive flag has not been set.
// It is safe to call before SetFormat; in that case currentFormat is "" which
// is treated as interactive.
func IsInteractive() bool {
	if nonInteractive {
		return false
	}
	if os.Getenv("GEMINI_CLI") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return IsTerminal() && currentFormat != "json" && currentFormat != "agent"
}
