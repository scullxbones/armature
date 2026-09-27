// Package ready provides the bubbletea TUI model for the arm ready command.
package ready

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/tui"
)

// ClaimMsg is sent when the user selects a task to claim.
type ClaimMsg struct{ IssueID string }

type Model struct {
	entries  []ready.ReadyEntry
	cursor   int
	selected string
	quit     bool
}

func New(entries []ready.ReadyEntry) Model {
	return Model{entries: entries}
}

func (m Model) Cursor() int { return m.cursor }

// Selected returns the issue ID of the selected entry, or "" if none selected.
func (m Model) Selected() string { return m.selected }

// Quit returns true if the user quit without selecting.
func (m Model) Quit() bool { return m.quit }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
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
