package ready

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/ready"
	"github.com/scullxbones/trellis/internal/tui"
)

// ClaimMsg is sent when the user selects a task to claim.
type ClaimMsg struct{ IssueID string }

// Model is the BubbleTea model for the ready task selection TUI.
type Model struct {
	entries  []ready.ReadyEntry
	cursor   int
	selected string
	quit     bool
}

// New creates a new Model with the given ready entries.
func New(entries []ready.ReadyEntry) Model {
	return Model{entries: entries}
}

// Cursor returns the current cursor position.
func (m Model) Cursor() int { return m.cursor }

// Selected returns the issue ID of the selected entry, or "" if none selected.
func (m Model) Selected() string { return m.selected }

// Quit returns true if the user quit without selecting.
func (m Model) Quit() bool { return m.quit }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
			return m, nil
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "enter":
			if len(m.entries) > 0 {
				m.selected = m.entries[m.cursor].Issue
			}
			return m, tea.Quit
		case "q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if len(m.entries) == 0 {
		return "No tasks ready.\n"
	}

	var sb strings.Builder
	sb.WriteString("Select a task to claim (j/k=move  enter=claim  q=quit):\n\n")

	for i, e := range m.entries {
		priority := e.Priority
		if priority == "" {
			priority = "—"
		}
		line := fmt.Sprintf("%s  %s  (%s)", e.Issue, e.Title, priority)
		if i == m.cursor {
			sb.WriteString(tui.Info.Render("> "+line) + "\n")
		} else {
			sb.WriteString(tui.Muted.Render("  "+line) + "\n")
		}
	}

	return sb.String()
}
