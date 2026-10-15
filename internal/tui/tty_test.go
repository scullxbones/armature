package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStdoutIsTerminalReturnsFalseInTests(t *testing.T) {
	t.Parallel()
	if StdoutIsTerminal() {
		t.Error("expected StdoutIsTerminal() to return false in test runner (no TTY attached)")
	}
}

func TestRendererSetNonInteractive(t *testing.T) {
	t.Parallel()
	r := NewRenderer(false, nil)
	r.Apply("human", true)
	if !r.IsNonInteractive() {
		t.Error("expected IsNonInteractive() to return true after Apply(..., true)")
	}
}

func TestRendererIsNonInteractive_DefaultFalse(t *testing.T) {
	t.Parallel()
	r := NewRenderer(false, nil)
	if r.IsNonInteractive() {
		t.Error("expected IsNonInteractive() to return false by default")
	}
}

func TestRendererIsInteractiveReturnsFalseWhenNonInteractiveSet(t *testing.T) {
	t.Parallel()
	r := NewRenderer(true, nil)
	r.Apply("human", true)
	if r.IsInteractive() {
		t.Error("expected IsInteractive() to return false when non-interactive is set")
	}
}

func TestRendererIsInteractiveReturnsFalseWhenFormatJSON(t *testing.T) {
	t.Parallel()
	r := NewRenderer(true, mapLookup(nil))
	r.Apply("json", false)
	if r.IsInteractive() {
		t.Error("expected IsInteractive() to return false when format=json")
	}
}

func TestRendererIsInteractiveReturnsFalseWhenFormatAgent(t *testing.T) {
	t.Parallel()
	r := NewRenderer(true, mapLookup(nil))
	r.Apply("agent", false)
	if r.IsInteractive() {
		t.Error("expected IsInteractive() to return false when format=agent")
	}
}

func TestRendererIsInteractiveReturnsFalseWhenNotTTY(t *testing.T) {
	t.Parallel()
	r := NewRenderer(false, mapLookup(nil))
	r.Apply("human", false)
	if r.IsInteractive() {
		t.Error("expected IsInteractive() to return false when not a TTY")
	}
}

func TestRendererIsInteractiveReturnsFalseWhenGeminiCLISet(t *testing.T) {
	t.Parallel()
	r := NewRenderer(true, mapLookup(map[string]string{"GEMINI_CLI": "1"}))
	r.Apply("human", false)
	if r.IsInteractive() {
		t.Error("expected IsInteractive() to return false when GEMINI_CLI is set")
	}
}

func TestRendererIsInteractiveReturnsFalseWhenTermIsDumb(t *testing.T) {
	t.Parallel()
	r := NewRenderer(true, mapLookup(map[string]string{"TERM": "dumb"}))
	r.Apply("human", false)
	if r.IsInteractive() {
		t.Error("expected IsInteractive() to return false when TERM=dumb")
	}
}

func TestRendererIsInteractiveOnTTYHuman(t *testing.T) {
	t.Parallel()
	r := NewRenderer(true, mapLookup(nil))
	r.Apply("human", false)
	require.True(t, r.IsInteractive())
	require.True(t, r.IsTerminal())
}

func TestRendererInstancesDoNotShareState(t *testing.T) {
	t.Parallel()
	a := NewRenderer(true, mapLookup(nil))
	b := NewRenderer(false, mapLookup(nil))
	a.Apply("human", false)
	b.Apply("json", true)
	require.True(t, a.IsInteractive())
	require.False(t, b.IsInteractive())
	require.False(t, a.IsNonInteractive())
	require.True(t, b.IsNonInteractive())
}

func mapLookup(m map[string]string) EnvLookup {
	return func(key string) (string, bool) {
		if m == nil {
			return "", false
		}
		v, ok := m[key]
		return v, ok
	}
}
