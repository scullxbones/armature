package tui

import "github.com/charmbracelet/lipgloss"

// Semantic color palette for trellis TUI output.
// Colors are chosen to convey meaning at a glance across standard terminals.
var (
	// Critical indicates a fatal error or blocking condition (bright red).
	Critical = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))

	// Warning indicates a non-fatal issue that warrants attention (xterm 214, bold).
	Warning = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)

	// Advisory indicates a suggestion or low-priority notice (xterm 226).
	Advisory = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))

	// OK indicates a successful or healthy state (green).
	OK = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC44"))

	// Info provides neutral informational output (xterm 39).
	Info = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	// Muted de-emphasises secondary content (dark grey).
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))

	// ActionRequired highlights that the user must take an action
	// (bold white foreground on xterm 196 background).
	ActionRequired = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("196")).
			Bold(true)

	// MyClaim indicates a task claimed by the current worker (bright green).
	MyClaim = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))

	// TheirClaim indicates a task claimed by another worker (bright blue).
	TheirClaim = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
)
