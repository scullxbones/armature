package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestSemanticPalette verifies that every semantic style variable has the
// correct color values and attributes as defined in the spec.
func TestSemanticPalette(t *testing.T) {
	t.Run("Warning", func(t *testing.T) {
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
		// Info = xterm 39, no bold
		wantFG := lipgloss.Color("39")
		if Info.GetForeground() != wantFG {
			t.Errorf("Info foreground: got %v, want %v", Info.GetForeground(), wantFG)
		}
		if Info.GetBold() {
			t.Error("Info: expected bold=false, got true")
		}
	})

	t.Run("ActionRequired", func(t *testing.T) {
		// ActionRequired = bold white foreground on xterm 196 background
		wantFG := lipgloss.Color("15") // white
		wantBG := lipgloss.Color("196")
		if ActionRequired.GetForeground() != wantFG {
			t.Errorf("ActionRequired foreground: got %v, want %v", ActionRequired.GetForeground(), wantFG)
		}
		if ActionRequired.GetBackground() != wantBG {
			t.Errorf("ActionRequired background: got %v, want %v", ActionRequired.GetBackground(), wantBG)
		}
		if !ActionRequired.GetBold() {
			t.Error("ActionRequired: expected bold=true, got false")
		}
	})

	t.Run("MyClaim", func(t *testing.T) {
		// MyClaim must have a foreground color set (distinct color for current worker)
		empty := lipgloss.NewStyle()
		if MyClaim.GetForeground() == empty.GetForeground() {
			t.Error("MyClaim: expected a foreground color to be set, but it was not")
		}
	})

	t.Run("TheirClaim", func(t *testing.T) {
		// TheirClaim must have a foreground color set (distinct color for other worker)
		empty := lipgloss.NewStyle()
		if TheirClaim.GetForeground() == empty.GetForeground() {
			t.Error("TheirClaim: expected a foreground color to be set, but it was not")
		}
	})

	t.Run("MyClaim_TheirClaim_distinct", func(t *testing.T) {
		// MyClaim and TheirClaim must use different colors
		if MyClaim.GetForeground() == TheirClaim.GetForeground() {
			t.Error("MyClaim and TheirClaim should use distinct foreground colors")
		}
	})

	// Verify pre-existing styles still have foreground colors set
	t.Run("Critical", func(t *testing.T) {
		empty := lipgloss.NewStyle()
		if Critical.GetForeground() == empty.GetForeground() {
			t.Error("Critical: expected a foreground color to be set, but it was not")
		}
	})

	t.Run("OK", func(t *testing.T) {
		empty := lipgloss.NewStyle()
		if OK.GetForeground() == empty.GetForeground() {
			t.Error("OK: expected a foreground color to be set, but it was not")
		}
	})

	t.Run("Muted", func(t *testing.T) {
		empty := lipgloss.NewStyle()
		if Muted.GetForeground() == empty.GetForeground() {
			t.Error("Muted: expected a foreground color to be set, but it was not")
		}
	})
}
