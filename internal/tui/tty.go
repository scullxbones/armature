package tui

import (
	"os"

	"golang.org/x/term"
)

// EnvLookup resolves a single environment variable for TTY/agent detection.
type EnvLookup func(key string) (string, bool)

// Renderer holds per-invocation output format and interactivity. Command code
// must use a Renderer from the current invocation; package-level format state
// does not exist.
type Renderer struct {
	format         string
	nonInteractive bool
	tty            bool
	lookup         EnvLookup
}

// NewRenderer constructs invocation-scoped UI state. lookup may be nil, in
// which case os.LookupEnv is used for GEMINI_CLI / TERM.
func NewRenderer(tty bool, lookup EnvLookup) *Renderer {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	return &Renderer{tty: tty, lookup: lookup}
}

// Apply records the resolved --format and --non-interactive values for this
// invocation (including values implied by TTY policy).
func (r *Renderer) Apply(format string, nonInteractive bool) {
	if r == nil {
		return
	}
	r.format = format
	r.nonInteractive = nonInteractive
}

// IsNonInteractive returns true when TUI commands must skip BubbleTea and
// emit structured output instead.
func (r *Renderer) IsNonInteractive() bool {
	return r != nil && r.nonInteractive
}

// IsTerminal reports whether this invocation's stdout was a TTY at start.
func (r *Renderer) IsTerminal() bool {
	return r != nil && r.tty
}

// IsInteractive returns true only when stdout is a TTY, the output format
// is neither "json" nor "agent", we're not running in a known agent environment,
// and the --non-interactive flag has not been set.
func (r *Renderer) IsInteractive() bool {
	if r == nil || r.nonInteractive {
		return false
	}
	if v, ok := r.lookup("GEMINI_CLI"); ok && v != "" {
		return false
	}
	if v, ok := r.lookup("TERM"); ok && v == "dumb" {
		return false
	}
	return r.tty && r.format != "json" && r.format != "agent"
}

// StdoutIsTerminal probes the process stdout. Call this only from the CLI
// entrypoint when constructing an Invocation; commands read Renderer.IsTerminal.
func StdoutIsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
