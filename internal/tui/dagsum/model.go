package dagsum

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/trellis/internal/materialize"
)

type ConfirmMsg struct{}
type SkipMsg struct{}
type EmitDAGTransitionMsg struct {
	IssueID  string
	WorkerID string
}

type itemState int

const (
	itemPending   itemState = iota
	itemConfirmed
	itemSkipped
)

type Model struct {
	issues   []*materialize.Issue
	states   []itemState
	cursor   int
	workerID string
	opsDir   string
	keys     KeyMap
	done     bool
}

func New(issues []*materialize.Issue, workerID, opsDir string) Model {
	return Model{
		issues:   issues,
		states:   make([]itemState, len(issues)),
		workerID: workerID,
		opsDir:   opsDir,
		keys:     DefaultKeyMap(),
	}
}

func (m Model) Total() int  { return len(m.issues) }
func (m Model) Cursor() int { return m.cursor }
func (m Model) Done() bool  { return m.done }

func (m Model) Confirmed() int {
	n := 0
	for _, s := range m.states {
		if s == itemConfirmed {
			n++
		}
	}
	return n
}

func (m Model) ConfirmedIDs() []string {
	var ids []string
	for i, s := range m.states {
		if s == itemConfirmed {
			ids = append(ids, m.issues[i].ID)
		}
	}
	return ids
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "q" || msg.String() == "ctrl+c":
			return m, tea.Quit
		case msg.String() == "enter" || msg.String() == "y":
			return m.confirm()
		case msg.String() == "s":
			return m.skip()
		case msg.String() == "up" || msg.String() == "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case msg.String() == "down" || msg.String() == "j":
			if m.cursor < len(m.issues)-1 {
				m.cursor++
			}
			return m, nil
		}
	case ConfirmMsg:
		return m.confirm()
	case SkipMsg:
		return m.skip()
	}
	return m, nil
}

func (m Model) confirm() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.issues) {
		return m, nil
	}
	m.states[m.cursor] = itemConfirmed
	issueID := m.issues[m.cursor].ID
	cmd := func() tea.Msg {
		return EmitDAGTransitionMsg{IssueID: issueID, WorkerID: m.workerID}
	}
	m.cursor = nextPending(m.states, m.cursor)
	if m.allReviewed() {
		m.done = true
		return m, tea.Sequence(cmd, tea.Quit)
	}
	return m, cmd
}

func (m Model) skip() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.issues) {
		return m, nil
	}
	m.states[m.cursor] = itemSkipped
	m.cursor = nextPending(m.states, m.cursor)
	if m.allReviewed() {
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) allReviewed() bool {
	for _, s := range m.states {
		if s == itemPending {
			return false
		}
	}
	return true
}

func nextPending(states []itemState, current int) int {
	for i := current + 1; i < len(states); i++ {
		if states[i] == itemPending {
			return i
		}
	}
	return current
}

func (m Model) View() string {
	if len(m.issues) == 0 {
		return "No items to review.\n"
	}
	if m.cursor >= len(m.issues) {
		return "Review complete.\n"
	}

	issue := m.issues[m.cursor]
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).
		Render(fmt.Sprintf("[%d/%d] %s", m.cursor+1, len(m.issues), issue.ID))
	title := lipgloss.NewStyle().Render(issue.Title)
	typeLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).
		Render(fmt.Sprintf("type: %s", issue.Type))
	stateLabel := "pending"
	switch m.states[m.cursor] {
	case itemConfirmed:
		stateLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("✓ confirmed")
	case itemSkipped:
		stateLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("→ skipped")
	}
	progress := fmt.Sprintf("Confirmed: %d/%d | enter=confirm  s=skip  q=quit", m.Confirmed(), len(m.issues))
	return fmt.Sprintf("%s\n%s  %s  %s\n\n%s\n", header, title, typeLabel, stateLabel, progress)
}
