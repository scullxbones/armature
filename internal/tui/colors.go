package tui

import "github.com/charmbracelet/lipgloss"

func Critical() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
}

func Warning() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
}

func Advisory() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
}

func OK() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC44"))
}

func Info() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
}

func Muted() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
}
