package workers

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui"
)

// WorkerInfo represents a worker and the issues they have claimed.
type WorkerInfo struct {
	ID     string
	Issues []*materialize.Issue
}

// Model implements tui.Screen for the Workers screen.
type Model struct {
	state   *materialize.State
	workers []WorkerInfo
	cursor  int
	width   int
	height  int
}

// New creates a new Workers screen model.
func New() *Model {
	return &Model{}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd { return nil }

// SetSize updates the model's dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// SetState updates the materialized state and rebuilds the worker list.
func (m *Model) SetState(state *materialize.State) {
	m.state = state
	m.rebuild()
}

// HelpBar returns the help bar content for the Workers screen.
func (m *Model) HelpBar() string {
	return tui.Muted.Render("j/k move  q quit  ? help")
}

// Update handles messages and returns the updated screen.
func (m *Model) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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

// View renders the Workers screen.
func (m *Model) View() string {
	if m.state == nil {
		return "No state available."
	}
	if len(m.workers) == 0 {
		return "No active workers."
	}

	var lines []string
	for i, w := range m.workers {
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
