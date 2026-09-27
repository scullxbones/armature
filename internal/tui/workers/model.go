// Package workers implements the TUI view listing active worker claims.
package workers

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui"
)

type WorkerInfo struct {
	ID     string
	Issues []*materialize.Issue
}

type Model struct {
	state        *materialize.State
	workers      []WorkerInfo
	cursor       int
	scrollOffset int
	width        int
	height       int
}

func New() *Model {
	return &Model{}
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *Model) SetState(state *materialize.State) {
	m.state = state
	m.rebuild()
}

func (m *Model) HelpBar() string {
	return tui.Muted.Render("j/k move  q quit  ? help")
}

func (m *Model) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.workers)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

func (m *Model) View() string {
	if m.state == nil {
		return "No state available."
	}
	if len(m.workers) == 0 {
		return "No active workers."
	}

	var lines []string
	workerStart := make([]int, len(m.workers))
	for i, w := range m.workers {
		workerStart[i] = len(lines)
		workerRow := fmt.Sprintf("👷 %s", tui.Info.Render(w.ID))
		if i == m.cursor {
			workerRow = lipgloss.NewStyle().Background(lipgloss.Color("39")).
				Foreground(lipgloss.Color("0")).Width(m.width).Render(workerRow)
		}
		lines = append(lines, workerRow)
		for _, issue := range w.Issues {
			lines = append(lines, fmt.Sprintf("  • %s: %s", tui.Muted.Render(issue.ID), issue.Title))
		}
		lines = append(lines, "")
	}

	if m.height > 0 && len(lines) > m.height {
		cursorLine := workerStart[m.cursor]
		if cursorLine < m.scrollOffset {
			m.scrollOffset = cursorLine
		}
		if cursorLine >= m.scrollOffset+m.height {
			m.scrollOffset = cursorLine - m.height + 1
		}
		start := m.scrollOffset
		end := start + m.height
		if end > len(lines) {
			end = len(lines)
		}
		lines = lines[start:end]
	}

	return strings.Join(lines, "\n")
}

func (m *Model) rebuild() {
	if m.state == nil {
		m.workers = nil
		return
	}

	workerMap := make(map[string][]*materialize.Issue)
	for _, issue := range m.state.Issues {
		if issue.ClaimedBy != "" {
			workerMap[issue.ClaimedBy] = append(workerMap[issue.ClaimedBy], issue)
		}
	}

	m.workers = nil
	for id, issues := range workerMap {
		sort.Slice(issues, func(i, j int) bool {
			return issues[i].ID < issues[j].ID
		})
		m.workers = append(m.workers, WorkerInfo{
			ID:     id,
			Issues: issues,
		})
	}

	sort.Slice(m.workers, func(i, j int) bool {
		return m.workers[i].ID < m.workers[j].ID
	})
}
