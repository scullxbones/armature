package detail

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui"
)

// Model is the shared detail overlay used by dagtree and workers screens.
// It renders a centred scrollable box showing all fields of a single issue.
type Model struct {
	open     bool
	issue    *materialize.Issue
	viewport viewport.Model
	width    int
	height   int
}

func New() Model {
	return Model{}
}

func (m Model) IsOpen() bool { return m.open }

func (m Model) Open(issue *materialize.Issue) Model {
	m.open = true
	m.issue = issue
	content := buildContent(issue)
	vp := viewport.New(min(m.width-4, 90), m.height-6)
	vp.SetContent(content)
	m.viewport = vp
	return m
}

func (m Model) Close() Model {
	m.open = false
	m.issue = nil
	return m
}

func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	if m.open {
		m.viewport.Width = min(width-4, 90)
		m.viewport.Height = height - 6
	}
	return m
}

// Update implements the Bubble Tea update cycle.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.open {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m.Close(), nil
		case "j", "down":
			m.viewport.LineDown(1)
		case "k", "up":
			m.viewport.LineUp(1)
		case "c":
			if m.issue != nil {
				_ = clipboard.WriteAll(m.issue.ID)
			}
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the overlay.
func (m Model) View() string {
	if !m.open || m.issue == nil {
		return ""
	}
	boxWidth := min(m.width-4, 90)
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Width(boxWidth).
		Padding(0, 1)

	content := border.Render(m.viewport.View())

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	return content
}

func buildContent(issue *materialize.Issue) string {
	if issue == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(tui.Info.Render(issue.ID) + "  " + issue.Title + "\n")
	fmt.Fprintf(&b, "Type: %s  Status: %s  Priority: %s\n",
		issue.Type, issue.Status, issue.Priority)

	if issue.DefinitionOfDone != "" {
		b.WriteString("\n" + tui.Muted.Render("Definition of Done:") + "\n")
		b.WriteString(issue.DefinitionOfDone + "\n")
	}

	if issue.Outcome != "" {
		b.WriteString("\n" + tui.Muted.Render("Outcome:") + "\n")
		b.WriteString(issue.Outcome + "\n")
	}

	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
