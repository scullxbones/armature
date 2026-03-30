package tuivalidate

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui"
	"github.com/scullxbones/trellis/internal/validate"
)

// Model implements app.Screen for the validation results view.
type Model struct {
	state   *materialize.State
	results validate.Result
	width   int
	height  int
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
	if state != nil {
		m.results = validate.Validate(state, validate.Options{})
	}
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
	b.WriteString(tui.Info.Render("Validation Results") + "\n\n")

	if len(m.results.Errors) == 0 && len(m.results.Warnings) == 0 {
		b.WriteString(tui.OK.Render("✓ No issues found.") + "\n")
		return b.String()
	}

	for _, err := range m.results.Errors {
		b.WriteString(tui.Critical.Render("ERROR: ") + err + "\n")
	}
	for _, warn := range m.results.Warnings {
		b.WriteString(tui.Warning.Render("WARN:  ") + warn + "\n")
	}

	return b.String()
}
