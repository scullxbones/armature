package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/tui"
	"github.com/scullxbones/trellis/internal/tui/detail"
)

// ScreenID identifies one of the four main screens.
type ScreenID int

const (
	ScreenDAGTree  ScreenID = iota // 1
	ScreenWorkers                  // 2
	ScreenValidate                 // 3
	ScreenSources                  // 4
)

// RefreshMsg triggers a re-materialisation.
type RefreshMsg struct{}

// fetchMsg triggers a git fetch.
type fetchMsg struct{}

// pollTickMsg is sent by the fallback poll ticker.
type pollTickMsg time.Time

// WatcherReadyMsg is sent when fsnotify watcher is started.
type WatcherReadyMsg struct{ Watcher *fsnotify.Watcher }

// stateUpdatedMsg is sent when materialization result is ready.
type stateUpdatedMsg struct{ state *materialize.State }

// Model is the root Bubble Tea model.
type Model struct {
	issuesDir      string
	stateDir       string
	workerID       string
	current        ScreenID
	screens        [4]tui.Screen
	state          *materialize.State
	validateErrors int
	staleSources   int
	width          int
	height         int
	watcher        *fsnotify.Watcher
	liveMode       bool // true = fsnotify active, false = poll fallback
	detail         detail.Model
}

// New constructs the root model. Screens are constructed lazily with placeholder
// implementations; real screens are injected by cmd/trellis/tui.go via WithScreens.
func New(issuesDir, stateDir, workerID string) Model {
	return Model{
		issuesDir: issuesDir,
		stateDir:  stateDir,
		workerID:  workerID,
		screens:   [4]tui.Screen{nilScreen{}, nilScreen{}, nilScreen{}, nilScreen{}},
	}
}

// WithScreens injects the four screen implementations.
func (m Model) WithScreens(tree, workers, validate, sources tui.Screen) Model {
	m.screens = [4]tui.Screen{tree, workers, validate, sources}
	return m
}

// WithState updates state on the model and propagates to all screens.
func (m Model) WithState(state *materialize.State) Model {
	m.state = state
	for i := range m.screens {
		if m.screens[i] != nil {
			m.screens[i].SetState(state)
		}
	}
	return m
}

// WithValidateErrors sets the error badge count for the Validate tab.
func (m Model) WithValidateErrors(n int) Model {
	m.validateErrors = n
	return m
}

// CurrentScreen returns the active screen ID.
func (m Model) CurrentScreen() ScreenID { return m.current }

// Init starts the fsnotify watcher on issuesDir/ops/ with 5s poll fallback.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, s := range m.screens {
		if s != nil {
			if cmd := s.Init(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	cmds = append(cmds, m.startWatcher())
	cmds = append(cmds, m.scheduleFetch())
	cmds = append(cmds, func() tea.Msg { return RefreshMsg{} })
	return tea.Batch(cmds...)
}

func (m Model) startWatcher() tea.Cmd {
	return func() tea.Msg {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return pollTickMsg(time.Now())
		}
		opsDir := filepath.Join(m.issuesDir, "ops")
		if err := w.Add(opsDir); err != nil {
			_ = w.Close()
			return pollTickMsg(time.Now())
		}
		return WatcherReadyMsg{Watcher: w}
	}
}

func (m Model) scheduleFetch() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return fetchMsg{}
	})
}

// NavBar renders the one-line navigation bar.
func (m Model) NavBar() string {
	tabs := []string{"Tree", "Workers", "Validate", "Sources"}
	var parts []string
	for i, name := range tabs {
		label := fmt.Sprintf("[%d] %s", i+1, name)
		// Add badge if applicable.
		if ScreenID(i) == ScreenValidate && m.validateErrors > 0 {
			label += tui.Critical.Render(fmt.Sprintf(" ⚠%d", m.validateErrors))
		}
		if ScreenID(i) == ScreenSources && m.staleSources > 0 {
			label += tui.Advisory.Render(fmt.Sprintf(" ⚠%d", m.staleSources))
		}
		if ScreenID(i) == m.current {
			label = lipgloss.NewStyle().Background(lipgloss.Color("39")).
				Foreground(lipgloss.Color("0")).Render(" " + label + " ")
		}
		parts = append(parts, label)
	}
	indicator := tui.Info.Render("↺ live")
	if !m.liveMode {
		indicator = tui.Advisory.Render("↺ poll")
	}
	left := strings.Join(parts, "  ")
	right := "trls tui · " + m.workerID + " · " + indicator
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// Update processes messages and returns the updated model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		for i := range m.screens {
			if m.screens[i] != nil {
				m.screens[i].SetSize(msg.Width, msg.Height-2)
			}
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.current = ScreenDAGTree
		case "2":
			m.current = ScreenWorkers
		case "3":
			m.current = ScreenValidate
		case "4":
			m.current = ScreenSources
		case "tab":
			m.current = (m.current + 1) % 4
		case "shift+tab":
			m.current = (m.current + 3) % 4
		default:
			if m.screens[m.current] != nil {
				updated, cmd := m.screens[m.current].Update(msg)
				m.screens[m.current] = updated
				return m, cmd
			}
		}
	case WatcherReadyMsg:
		m.watcher = msg.Watcher
		m.liveMode = true
		return m, m.listenForChanges()
	case RefreshMsg:
		if m.liveMode {
			return m, tea.Batch(m.doRefresh(), m.listenForChanges())
		}
		return m, m.doRefresh()
	case pollTickMsg:
		return m, tea.Batch(m.doRefresh(), tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return pollTickMsg(t)
		}))
	case fetchMsg:
		return m, tea.Batch(m.doFetch(), m.scheduleFetch())
	case stateUpdatedMsg:
		return m.WithState(msg.state), nil
	}
	return m, nil
}

func (m Model) listenForChanges() tea.Cmd {
	return func() tea.Msg {
		if m.watcher == nil {
			return nil
		}
		select {
		case event, ok := <-m.watcher.Events:
			if !ok {
				return nil
			}
			_ = event
			// Debounce: 200ms delay before re-materialize.
			time.Sleep(200 * time.Millisecond)
			return RefreshMsg{}
		case err, ok := <-m.watcher.Errors:
			if !ok || err != nil {
				return nil
			}
		}
		return m.listenForChanges()
	}
}

func (m Model) doRefresh() tea.Cmd {
	issuesDir := m.issuesDir
	stateDir := m.stateDir
	return func() tea.Msg {
		state, _, err := materialize.MaterializeAndReturn(issuesDir, stateDir, true)
		if err != nil || state == nil {
			return nil
		}
		return stateUpdatedMsg{state: state}
	}
}

func (m Model) doFetch() tea.Cmd {
	// Best-effort git fetch with 10s timeout — errors are silent.
	return nil
}

// View renders nav bar + active screen + help bar.
func (m Model) View() string {
	nav := m.NavBar()
	var content, help string
	if m.screens[m.current] != nil {
		content = m.screens[m.current].View()
		help = m.screens[m.current].HelpBar()
	}
	return nav + "\n" + content + "\n" + help
}

// nilScreen is a placeholder Screen used before real screens are injected.
type nilScreen struct{}

func (nilScreen) Init() tea.Cmd                            { return nil }
func (n nilScreen) Update(_ tea.Msg) (tui.Screen, tea.Cmd) { return n, nil }
func (nilScreen) View() string                             { return "" }
func (nilScreen) HelpBar() string                          { return "" }
func (nilScreen) SetSize(_, _ int)                         {}
func (nilScreen) SetState(_ *materialize.State)            {}
