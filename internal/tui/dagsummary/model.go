// Package dagsummary provides a BubbleTea TUI for reviewing and signing off
// draft DAG nodes. Each item must be actioned (y/n/s) before sign-off unlocks.
// Uncited nodes require the user to type the node ID as acknowledgment before
// any action is accepted. Sign-off emits a dag-transition op for approved items.
package dagsummary

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/tui"
)

type Item struct {
	ID      string
	Title   string
	IsCited bool // false = uncited, requires explicit ID acknowledgment
}

type Model struct {
	items        []Item
	cursor       int
	actions      map[string]string
	awaitSignOff bool
	done         bool
	quitting     bool
	rootID       string
	pendingAck   string
}

func New(items []Item, rootID string) Model {
	return Model{
		items:   items,
		actions: make(map[string]string, len(items)),
		rootID:  rootID,
	}
}

func (m Model) Total() int            { return len(m.items) }
func (m Model) Cursor() int           { return m.cursor }
func (m Model) Done() bool            { return m.done }
func (m Model) Quitting() bool        { return m.quitting }
func (m Model) AwaitingSignOff() bool { return m.awaitSignOff }

// ActionFor returns the action string for the given item ID ("y", "n", "s", or "").
func (m Model) ActionFor(id string) string { return m.actions[id] }

// ApprovedIDs returns the IDs of items actioned as "y" (approved).
func (m Model) ApprovedIDs() []string {
	var ids []string
	for _, item := range m.items {
		if m.actions[item.ID] == "y" {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.String()

		if k == "q" || k == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		if m.awaitSignOff {
			return m.handleSignOff(k)
		}

		return m.handleItemKey(k)
	}
	return m, nil
}

func (m Model) handleSignOff(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "y":
		m.done = true
		return m, tea.Quit
	case "n":
		m.awaitSignOff = false
		if m.cursor >= len(m.items) && len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleItemKey(k string) (tea.Model, tea.Cmd) {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return m, nil
	}

	item := m.items[m.cursor]

	if !item.IsCited {
		if k == "y" || k == "n" || k == "s" {
			if m.pendingAck == item.ID {
				return m.recordAction(item.ID, k)
			}
			return m, nil
		}
		if len(k) == 1 {
			m.pendingAck += k
		}
		return m, nil
	}

	switch k {
	case "y", "n", "s":
		return m.recordAction(item.ID, k)
	}
	return m, nil
}

func (m Model) recordAction(id, action string) (tea.Model, tea.Cmd) {
	m.actions[id] = action
	m.pendingAck = ""
	m.cursor++
	if m.allActioned() {
		m.awaitSignOff = true
	}
	return m, nil
}

func (m Model) allActioned() bool {
	for _, item := range m.items {
		if m.actions[item.ID] == "" {
			return false
		}
	}
	return true
}

func (m Model) View() string {
	if len(m.items) == 0 {
		return "No draft nodes to review.\n"
	}

	if m.awaitSignOff || m.done {
		return m.signOffView()
	}

	if m.cursor >= len(m.items) {
		return "Review complete.\n"
	}

	return m.itemView()
}

func (m Model) itemView() string {
	item := m.items[m.cursor]
	var sb strings.Builder

	header := tui.Info.Bold(true).Render(
		fmt.Sprintf("[%d/%d] %s", m.cursor+1, len(m.items), item.ID),
	)
	sb.WriteString(header + "\n")
	sb.WriteString(item.Title + "\n")

	if !item.IsCited {
		warn := tui.Warning.Render("WARNING: uncited node — type the ID to acknowledge")
		sb.WriteString(warn + "\n")
		fmt.Fprintf(&sb, "Ack: %s\n", m.pendingAck)
	}

	action := m.actions[item.ID]
	if action != "" {
		sb.WriteString(tui.OK.Render("action: "+action) + "\n")
	}

	sb.WriteString("\n")
	actioned := m.countActioned()
	fmt.Fprintf(&sb, "Actioned: %d/%d | y=approve  n=reject  s=skip  q=quit\n",
		actioned, len(m.items))

	return sb.String()
}

func (m Model) signOffView() string {
	approvedIDs := m.ApprovedIDs()
	var sb strings.Builder
	sb.WriteString(tui.OK.Bold(true).Render("All items reviewed.") + "\n\n")
	if len(approvedIDs) > 0 {
		fmt.Fprintf(&sb, "Approved (%d): %s\n", len(approvedIDs), strings.Join(approvedIDs, ", "))
	} else {
		sb.WriteString("No items approved — no dag-transition will be emitted.\n")
	}
	sb.WriteString("\n")
	sb.WriteString(tui.Warning.Bold(true).Render("Sign off? [y/n]") + "\n")
	return sb.String()
}

func (m Model) countActioned() int {
	n := 0
	for _, item := range m.items {
		if m.actions[item.ID] != "" {
			n++
		}
	}
	return n
}
