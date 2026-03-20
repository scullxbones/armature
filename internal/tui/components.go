package tui

import (
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
)

// NewSpinner returns a spinner model with default settings.
func NewSpinner() spinner.Model {
	return spinner.New()
}

// NewProgressBar returns a progress bar model with default gradient colors.
func NewProgressBar() progress.Model {
	return progress.New(progress.WithDefaultGradient())
}

// NewTable returns a table model with default settings.
func NewTable() table.Model {
	return table.New()
}

// NewViewport returns a viewport model sized to the given width and height.
func NewViewport(width, height int) viewport.Model {
	return viewport.New(width, height)
}
