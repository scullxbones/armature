package tui

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal returns true if stdout is connected to a TTY.
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
