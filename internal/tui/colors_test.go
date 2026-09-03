package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestSemanticPalette verifies that every semantic style variable has the
// correct color values and attributes as defined in the spec.
func TestSemanticPalette(t *testing.T) {
	t.Parallel()
	t.Run("Warning", func(t *testing.T) {
		t.Parallel()
		// Warning = xterm 214, bold
		wantFG := lipgloss.Color("214")
		if Warning.GetForeground() != wantFG {
			t.Errorf("Warning foreground: got %v, want %v", Warning.GetForeground(), wantFG)
		}
		if !Warning.GetBold() {
			t.Error("Warning: expected bold=true, got false")
		}
	})

	t.Run("Advisory", func(t *testing.T) {
		t.Parallel()
		// Advisory = xterm 226, no bold
		wantFG := lipgloss.Color("226")
		if Advisory.GetForeground() != wantFG {
			t.Errorf("Advisory foreground: got %v, want %v", Advisory.GetForeground(), wantFG)
		}
		if Advisory.GetBold() {
			t.Error("Advisory: expected bold=false, got true")
		}
	})

	t.Run("Info", func(t *testing.T) {
		t.Parallel()
		// Info = xterm 39, no bold
		wantFG := lipgloss.Color("39")
		if Info.GetForeground() != wantFG {
			t.Errorf("Info foreground: got %v, want %v", Info.GetForeground(), wantFG)
		}
		if Info.GetBold() {
			t.Error("Info: expected bold=false, got true")
		}
	})

	t.Run("Critical", func(t *testing.T) {
		t.Parallel()
		empty := lipgloss.NewStyle()
		if Critical.GetForeground() == empty.GetForeground() {
			t.Error("Critical: expected a foreground color to be set, but it was not")
		}
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		empty := lipgloss.NewStyle()
		if OK.GetForeground() == empty.GetForeground() {
			t.Error("OK: expected a foreground color to be set, but it was not")
		}
	})

	t.Run("Muted", func(t *testing.T) {
		t.Parallel()
		empty := lipgloss.NewStyle()
		if Muted.GetForeground() == empty.GetForeground() {
			t.Error("Muted: expected a foreground color to be set, but it was not")
		}
	})
}
