package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/tui/stalereview"
)

func runStaleReviewTUI(items []stalereview.ReviewItem, workerID string) (stalereview.Model, error) {
	m := stalereview.New(items, workerID)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return stalereview.Model{}, fmt.Errorf("stale-review TUI: %w", err)
	}
	final, ok := finalModel.(stalereview.Model)
	if !ok {
		return stalereview.Model{}, fmt.Errorf("unexpected model type from TUI")
	}
	return final, nil
}
