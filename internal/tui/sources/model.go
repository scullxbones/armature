package sources

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui"
)

// Model implements app.Screen for the sources view.
type Model struct {
	state   *materialize.State
	sources []sourceRow
	width   int
	height  int
}

type sourceRow struct {
	id     string
	issues []string
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
	return tui.Muted.Render("q quit  ? help")
}

func (m *Model) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	return m, nil
}

func (m *Model) View() string {
	if m.state == nil {
		return "No state available."
	}

	var b strings.Builder
	b.WriteString(tui.Info.Render("Sources") + "\n\n")

	if len(m.sources) == 0 {
		b.WriteString(tui.Muted.Render("No sources cited.") + "\n")
		return b.String()
	}

	for _, row := range m.sources {
		fmt.Fprintf(&b, "%s: %s\n", tui.Info.Render(row.id), strings.Join(row.issues, ", "))
	}

	return b.String()
}

func (m *Model) rebuild() {
	if m.state == nil {
		m.sources = nil
		return
	}

	sourceMap := make(map[string][]string)
	for _, issue := range m.state.Issues {
		for _, link := range issue.SourceLinks {
			id := link.SourceEntryID
			if id != "" {
				sourceMap[id] = append(sourceMap[id], issue.ID)
			}
		}
	}

	var keys []string
	for k := range sourceMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	m.sources = nil
	for _, k := range keys {
		issues := sourceMap[k]
		sort.Strings(issues)
		m.sources = append(m.sources, sourceRow{id: k, issues: issues})
	}
}
