package dagtree

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/tui/detail"
)

// visibleNode is a rendered tree row.
type visibleNode struct {
	issue  *materialize.Issue
	depth  int
	isLast bool
}

// Model implements app.Screen for the DAG tree view.
type Model struct {
	state        *materialize.State
	visible      []visibleNode
	expanded     map[string]bool
	cursor       int
	scrollOffset int
	filter       string
	width        int
	height       int
	detail       detail.Model
}

func New() *Model {
	return &Model{
		expanded: make(map[string]bool),
		detail:   detail.New(),
	}
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.detail = m.detail.SetSize(width, height)
}

func (m *Model) SetState(state *materialize.State) {
	m.state = state
	m.rebuild()
}

func (m *Model) WithFilter(q string) *Model {
	m.filter = q
	m.rebuild()
	return m
}

func (m *Model) HelpBar() string {
	return tui.Muted.Render("j/k move  h/l collapse/expand  enter detail  / filter  q quit  ? help")
}

func (m *Model) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	if m.detail.IsOpen() {
		newDetail, cmd := m.detail.Update(msg)
		m.detail = newDetail
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.visible)-1 {
				m.cursor++
				m.clampScroll()
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.clampScroll()
			}
		case "l", "right":
			if m.cursor < len(m.visible) {
				id := m.visible[m.cursor].issue.ID
				m.expanded[id] = true
				m.rebuild()
			}
		case "h", "left":
			if m.cursor < len(m.visible) {
				id := m.visible[m.cursor].issue.ID
				m.expanded[id] = false
				m.rebuild()
			}
		case "enter":
			if m.cursor < len(m.visible) {
				m.detail = m.detail.Open(m.visible[m.cursor].issue)
			}
		}
	}
	return m, nil
}

func (m *Model) View() string {
	if m.state == nil {
		return "No state available."
	}

	var lines []string
	for i := range m.visible {
		lines = append(lines, m.renderNode(i))
	}

	// Clip to viewport height when height is set.
	if m.height > 0 && len(lines) > m.height {
		start := m.scrollOffset
		end := start + m.height
		if end > len(lines) {
			end = len(lines)
		}
		lines = lines[start:end]
	}

	tree := strings.Join(lines, "\n")

	if m.detail.IsOpen() {
		return m.renderWithOverlay(tree)
	}
	return tree
}

// clampScroll adjusts scrollOffset to keep cursor within the visible window.
func (m *Model) clampScroll() {
	if m.height <= 0 {
		return
	}
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+m.height {
		m.scrollOffset = m.cursor - m.height + 1
	}
}

func (m *Model) renderNode(idx int) string {
	node := m.visible[idx]
	issue := node.issue

	prefix := strings.Repeat("│   ", node.depth)
	if node.depth > 0 {
		if node.isLast {
			prefix = strings.Repeat("│   ", node.depth-1) + "└── "
		} else {
			prefix = strings.Repeat("│   ", node.depth-1) + "├── "
		}
	}

	glyph := glyphFor(issue.Status)
	idStr := tui.Info.Render(issue.ID)
	title := issue.Title

	row := fmt.Sprintf("%s%s %s  %s", tui.Muted.Render(prefix), glyph, idStr, title)
	if idx == m.cursor {
		row = lipgloss.NewStyle().Background(lipgloss.Color("39")).
			Foreground(lipgloss.Color("0")).Width(m.width).Render(row)
	}
	return row
}

func (m *Model) renderWithOverlay(tree string) string {
	overlay := m.detail.View()
	// Simple overlay for now
	return tree + "\n" + overlay
}

func (m *Model) rebuild() {
	if m.state == nil {
		m.visible = nil
		return
	}

	var roots []*materialize.Issue
	for _, issue := range m.state.Issues {
		if issue.Parent == "" || m.state.Issues[issue.Parent] == nil {
			roots = append(roots, issue)
		}
	}

	sort.Slice(roots, func(i, j int) bool {
		return roots[i].ID < roots[j].ID
	})

	m.visible = nil
	for i, root := range roots {
		m.walk(root, 0, i == len(roots)-1)
	}
}

func (m *Model) walk(issue *materialize.Issue, depth int, isLast bool) {
	if m.filter != "" && !matchesFilter(issue, m.filter) && !m.hasMatchingDescendant(issue, m.filter) {
		return
	}

	m.visible = append(m.visible, visibleNode{issue: issue, depth: depth, isLast: isLast})

	if m.expanded[issue.ID] || depth == 0 {
		var children []*materialize.Issue
		for _, child := range m.state.Issues {
			if child.Parent == issue.ID {
				children = append(children, child)
			}
		}
		sort.Slice(children, func(i, j int) bool {
			return children[i].ID < children[j].ID
		})
		for i, child := range children {
			m.walk(child, depth+1, i == len(children)-1)
		}
	}
}

func (m *Model) hasMatchingDescendant(issue *materialize.Issue, filter string) bool {
	for _, child := range m.state.Issues {
		if child.Parent == issue.ID {
			if matchesFilter(child, filter) || m.hasMatchingDescendant(child, filter) {
				return true
			}
		}
	}
	return false
}

func matchesFilter(issue *materialize.Issue, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(issue.ID), q) ||
		strings.Contains(strings.ToLower(issue.Title), q)
}

func glyphFor(status string) string {
	switch status {
	case "merged":
		return "✓"
	case "in-progress":
		return "▶"
	case "blocked":
		return "✗"
	case "cancelled":
		return "—"
	default:
		return "○"
	}
}
