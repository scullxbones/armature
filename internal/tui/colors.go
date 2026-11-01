package tui

import "github.com/charmbracelet/lipgloss"

//nolint:gochecknoglobals // immutable TUI color palette
var (
	Critical = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	Warning  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	Advisory = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	OK       = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC44"))
	Info     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	Muted    = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
)
