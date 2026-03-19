package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestSemanticPalette verifies that every semantic style variable has a
// foreground color set (i.e. is non-zero / not the empty Style).
func TestSemanticPalette(t *testing.T) {
	empty := lipgloss.NewStyle()

	cases := []struct {
		name  string
		style lipgloss.Style
	}{
		{"Critical", Critical},
		{"Warning", Warning},
		{"Advisory", Advisory},
		{"OK", OK},
		{"Info", Info},
		{"Muted", Muted},
		{"ActionRequired", ActionRequired},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.style.GetForeground() == empty.GetForeground() {
				t.Errorf("%s: expected a foreground color to be set, but it was not", tc.name)
			}
		})
	}
}
