package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/ready"
	readytui "github.com/scullxbones/armature/internal/tui/ready"
)

// runReadyTUI is the interactive boundary for `arm ready`: construct the
// ready picker, run it, and return the selected issue ID (empty if the
// user quit without selecting). Post-TUI claim side effects stay in ready.go.
func runReadyTUI(entries []ready.ReadyEntry) (string, error) {
	m := readytui.New(entries)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}
	final, ok := finalModel.(readytui.Model)
	if !ok {
		return "", fmt.Errorf("unexpected model type from TUI")
	}
	return final.Selected(), nil
}
