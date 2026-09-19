package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/tui/dagsummary"
)

func runDAGSummaryTUI(items []dagsummary.Item, rootID string) (dagsummary.Model, error) {
	m := dagsummary.New(items, rootID)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return dagsummary.Model{}, fmt.Errorf("dag-summary TUI: %w", err)
	}
	final, ok := finalModel.(dagsummary.Model)
	if !ok {
		return dagsummary.Model{}, fmt.Errorf("unexpected model type from TUI")
	}
	return final, nil
}
