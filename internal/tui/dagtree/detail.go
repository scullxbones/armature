package dagtree

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

type issueDetail struct {
	open     bool
	issue    *materialize.Issue
	viewport viewport.Model
	width    int
	height   int
}

func (m issueDetail) isOpen() bool { return m.open }

func (m issueDetail) openIssue(issue *materialize.Issue) issueDetail {
	m.open = true
	m.issue = issue
	content := buildDetailContent(issue)
	vp := viewport.New(min(m.width-4, 90), m.height-6)
	vp.SetContent(content)
	m.viewport = vp
	return m
}

func (m issueDetail) close() issueDetail {
	m.open = false
	m.issue = nil
	return m
}

func (m issueDetail) setSize(width, height int) issueDetail {
	m.width = width
	m.height = height
	if m.open {
		m.viewport.Width = min(width-4, 90)
		m.viewport.Height = height - 6
	}
	return m
}

func (m issueDetail) update(msg tea.Msg) (issueDetail, tea.Cmd) {
	if !m.open {
		return m, nil
	}
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "esc", "q":
			return m.close(), nil
		case "j", "down":
			m.viewport.LineDown(1)
		case "k", "up":
			m.viewport.LineUp(1)
		case "c":
			if m.issue != nil {
				if err := clipboard.WriteAll(m.issue.ID); err != nil {
					return m, nil
				}
			}
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m issueDetail) view() string {
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

func buildDetailContent(issue *materialize.Issue) string {
	if issue == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(tui.Info().Render(issue.ID) + "  " + issue.Title + "\n")
	fmt.Fprintf(&b, "Type: %s  Status: %s  Priority: %s\n",
		issue.Type, issue.Status, issue.Priority)

	if issue.DefinitionOfDone != "" {
		b.WriteString("\n" + tui.Muted().Render("Definition of Done:") + "\n")
		b.WriteString(issue.DefinitionOfDone + "\n")
	}

	if issue.Outcome != "" {
		b.WriteString("\n" + tui.Muted().Render("Outcome:") + "\n")
		b.WriteString(issue.Outcome + "\n")
	}

	return b.String()
}
