package tui

import "github.com/charmbracelet/lipgloss"

// Semantic color palette for trellis TUI output.
// Colors are chosen to convey meaning at a glance across standard terminals.
var (
	// Critical indicates a fatal error or blocking condition (bright red).
	Critical = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))

	// Warning indicates a non-fatal issue that warrants attention (yellow).
	Warning = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00"))

	// Advisory indicates a suggestion or low-priority notice (orange).
	Advisory = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C00"))

	// OK indicates a successful or healthy state (green).
	OK = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC44"))

	// Info provides neutral informational output (cyan).
	Info = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CCFF"))

	// Muted de-emphasises secondary content (dark grey).
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))

	// ActionRequired highlights that the user must take an action (magenta).
	ActionRequired = lipgloss.NewStyle().Foreground(lipgloss.Color("#CC44FF"))
)
