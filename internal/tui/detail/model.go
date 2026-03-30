package detail

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui"
)

// Model represents a detail overlay for a single issue.
type Model struct {
	Issue  *materialize.Issue
	Width  int
	Height int
	Hidden bool
}

// New creates a new detail Model.
func New(issue *materialize.Issue, width, height int) Model {
	return Model{
		Issue:  issue,
		Width:  width,
		Height: height,
		Hidden: issue == nil,
	}
}

// SetSize updates the dimensions for the centered overlay.
func (m *Model) SetSize(width, height int) {
	m.Width = width
	m.Height = height
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.Hidden || m.Issue == nil {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.Hidden = true
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.Hidden || m.Issue == nil {
		return ""
	}

	// Define style for the modal box
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(60)

	// Build content
	var sb strings.Builder
	sb.WriteString(tui.Info.Bold(true).Render("ISSUE DETAIL") + "\n\n")
	
	fields := []struct {
		Label string
		Value string
	}{
		{"ID", m.Issue.ID},
		{"Title", m.Issue.Title},
		{"Status", m.Issue.Status},
		{"Type", m.Issue.Type},
		{"Priority", m.Issue.Priority},
		{"Assignee", m.Issue.Assignee},
		{"Complexity", m.Issue.EstComplexity},
	}

	for _, f := range fields {
		if f.Value == "" {
			continue
		}
		label := tui.Muted.Render(fmt.Sprintf("%-12s", f.Label+":"))
		sb.WriteString(fmt.Sprintf("%s %s\n", label, f.Value))
	}

	if m.Issue.DefinitionOfDone != "" {
		sb.WriteString("\n" + tui.Muted.Render("Definition of Done:") + "\n")
		sb.WriteString(m.Issue.DefinitionOfDone + "\n")
	}

	sb.WriteString("\n" + tui.Muted.Render("(press esc/q to close)"))

	content := modalStyle.Render(sb.String())

	// Center the modal if width/height are set, otherwise just return content
	if m.Width > 0 && m.Height > 0 {
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, content)
	}
	return content
}
