package stalereview

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/trellis/internal/materialize"
)

type ReviewItem struct {
	SourceID      string
	ChangeSummary string
	CitedIssues   []*materialize.Issue
}

type ConfirmMsg struct{}
type FlagMsg struct{}
type SkipMsg struct{}

type itemDecision int

const (
	decisionPending   itemDecision = iota
	decisionConfirmed
	decisionFlagged
	decisionSkipped
)

type Model struct {
	items     []ReviewItem
	decisions []itemDecision
	cursor    int
	workerID  string
}

func New(items []ReviewItem, workerID string) Model {
	return Model{items: items, decisions: make([]itemDecision, len(items)), workerID: workerID}
}

func (m Model) Total() int { return len(m.items) }

func (m Model) ConfirmedCount() int {
	n := 0
	for _, d := range m.decisions {
		if d == decisionConfirmed {
			n++
		}
	}
	return n
}

func (m Model) Decisions() []itemDecision { return m.decisions }
func (m Model) Items() []ReviewItem       { return m.items }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "y":
			return m.setDecision(decisionConfirmed)
		case "f":
			return m.setDecision(decisionFlagged)
		case "s":
			return m.setDecision(decisionSkipped)
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case ConfirmMsg:
		return m.setDecision(decisionConfirmed)
	case FlagMsg:
		return m.setDecision(decisionFlagged)
	case SkipMsg:
		return m.setDecision(decisionSkipped)
	}
	return m, nil
}

func (m Model) setDecision(d itemDecision) (tea.Model, tea.Cmd) {
	if m.cursor < len(m.decisions) {
		m.decisions[m.cursor] = d
		m.cursor++
	}
	if m.cursor >= len(m.items) {
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) View() string {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return "Stale review complete.\n"
	}
	item := m.items[m.cursor]
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).
		Render(fmt.Sprintf("[%d/%d] Source changed: %s", m.cursor+1, len(m.items), item.SourceID))
	var sb strings.Builder
	sb.WriteString(header + "\n\n")
	sb.WriteString("Change: " + item.ChangeSummary + "\n\nAffected issues:\n")
	for _, issue := range item.CitedIssues {
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", issue.ID, issue.Title))
	}
	sb.WriteString("\nenter=confirm  f=flag  s=skip  q=quit\n")
	return sb.String()
}
