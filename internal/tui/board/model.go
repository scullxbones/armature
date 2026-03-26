package board

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui"
)

// Column indices for the three board states.
const (
	ColOpen   = 0
	ColActive = 1
	ColDone   = 2
)

var colHeaders = [3]string{"OPEN", "ACTIVE", "DONE"}

// Model is a BubbleTea model representing a kanban board with three columns.
type Model struct {
	columns    [3][]*materialize.Issue
	activeCol  int
	cursors    [3]int
	viewport   viewport.Model
	showDetail bool
	keys       KeyMap
	width      int
	height     int
}

// New creates a new board Model, distributing issues into columns by status.
func New(issues []*materialize.Issue, width, height int) Model {
	m := Model{
		keys:   DefaultKeyMap(),
		width:  width,
		height: height,
	}
	// Distribute issues into columns based on status.
	for _, issue := range issues {
		switch issue.Status {
		case "open":
			m.columns[ColOpen] = append(m.columns[ColOpen], issue)
		case "in-progress":
			m.columns[ColActive] = append(m.columns[ColActive], issue)
		case "done":
			m.columns[ColDone] = append(m.columns[ColDone], issue)
		}
	}
	m.viewport = viewport.New(width, height/3)
	return m
}

// ActiveCol returns the index of the currently focused column.
func (m Model) ActiveCol() int { return m.activeCol }

// Cursor returns the cursor position within the active column.
func (m Model) Cursor() int { return m.cursors[m.activeCol] }

// ShowDetail returns whether the detail viewport is visible.
func (m Model) ShowDetail() bool { return m.showDetail }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "q" || msg.String() == "ctrl+c":
			return m, tea.Quit

		case msg.String() == "h" || msg.String() == "left":
			if m.activeCol > 0 {
				m.activeCol--
			}
			m.showDetail = false
			return m, nil

		case msg.String() == "l" || msg.String() == "right":
			if m.activeCol < 2 {
				m.activeCol++
			}
			m.showDetail = false
			return m, nil

		case msg.String() == "k" || msg.String() == "up":
			if m.cursors[m.activeCol] > 0 {
				m.cursors[m.activeCol]--
			}
			return m, nil

		case msg.String() == "j" || msg.String() == "down":
			col := m.columns[m.activeCol]
			if len(col) > 0 && m.cursors[m.activeCol] < len(col)-1 {
				m.cursors[m.activeCol]++
			}
			return m, nil

		case msg.String() == "enter":
			col := m.columns[m.activeCol]
			if len(col) == 0 {
				return m, nil
			}
			cursor := m.cursors[m.activeCol]
			if cursor >= len(col) {
				cursor = len(col) - 1
			}
			issue := col[cursor]
			content := renderDetail(issue)
			m.viewport.SetContent(content)
			m.showDetail = true
			return m, nil
		}
	}
	return m, nil
}

func renderDetail(issue *materialize.Issue) string {
	return fmt.Sprintf(
		"ID:       %s\nTitle:    %s\nStatus:   %s\nPriority: %s\nType:     %s\n",
		issue.ID,
		issue.Title,
		issue.Status,
		issue.Priority,
		issue.Type,
	)
}

// View implements tea.Model.
func (m Model) View() string {
	colWidth := m.width / 3
	if colWidth < 20 {
		colWidth = 20
	}

	colStyle := lipgloss.NewStyle().
		Width(colWidth).
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)

	activeColStyle := colStyle.
		BorderForeground(lipgloss.Color("#00CCFF"))

	var cols [3]string
	for i := 0; i < 3; i++ {
		header := tui.Info.Bold(true).Render(
			fmt.Sprintf("%s (%d)", colHeaders[i], len(m.columns[i])),
		)
		var items []string
		items = append(items, header)
		items = append(items, strings.Repeat("─", colWidth-4))

		col := m.columns[i]
		if len(col) == 0 {
			items = append(items, tui.Muted.Render("  (empty)"))
		} else {
			for j, issue := range col {
				prefix := "  "
				if i == m.activeCol && j == m.cursors[i] {
					prefix = tui.OK.Render("> ")
				}
				title := issue.Title
				maxLen := colWidth - 6
				if len(title) > maxLen {
					title = title[:maxLen-1] + "…"
				}
				line := prefix + title
				if i == m.activeCol && j == m.cursors[i] {
					line = tui.Info.Render(prefix + title)
				}
				items = append(items, line)
			}
		}

		content := strings.Join(items, "\n")
		if i == m.activeCol {
			cols[i] = activeColStyle.Render(content)
		} else {
			cols[i] = colStyle.Render(content)
		}
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols[0], cols[1], cols[2])

	if m.showDetail {
		detailStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			Width(m.width - 4)
		detail := detailStyle.Render(m.viewport.View())
		return board + "\n" + detail
	}

	return board
}
