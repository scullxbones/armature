# E4 DAG Summary Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the interactive dag-summary BubbleTea model for reviewing and signing off DAG items, the dag-summary CLI command with dag-summary.md artifact output, and a non-interactive JSON path for agent/CI use.

**Spec:** `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` (E3-S6 section)

**Depends on:** `2026-03-18-e4-s5-decompose-context.md`

**Execution order within E4:** S1 → S4 → S7 → S2 → S3 → S5 → S6 → S8 → S9

**Tech Stack:** Go 1.26, Bubble Tea v1.3, Lip Gloss v1.1, Cobra v1.8, testify

---

## File Structure

| Package | File | Responsibility |
|---|---|---|
| `internal/tui/dagsum` | `model.go` | BubbleTea model for dag-summary interactive TUI |
| `internal/tui/dagsum` | `keys.go` | Key bindings for dag-summary |
| `internal/tui/dagsum` | `model_test.go` | Unit tests for model state transitions |
| `cmd/trellis/dagsum.go` | — | `dag-summary` command + non-interactive JSON path |
| `cmd/trellis/main.go` | — | Register `dag-summary` command |

---

## Tasks

### Task 1: dag-summary BubbleTea model

**Files:**
- Create: `internal/tui/dagsum/model.go`
- Create: `internal/tui/dagsum/keys.go`
- Create: `internal/tui/dagsum/model_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/tui/dagsum/model_test.go
package dagsum_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui/dagsum"
	"github.com/stretchr/testify/assert"
)

func TestNewModelHasAllItems(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "First task", Type: "task"},
		{ID: "TSK-2", Title: "Second task", Type: "task"},
	}
	m := dagsum.New(issues, "worker-1", "")
	assert.Equal(t, 2, m.Total())
	assert.Equal(t, 0, m.Confirmed())
}

func TestConfirmAdvancesCursor(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task 1", Type: "task"},
		{ID: "TSK-2", Title: "Task 2", Type: "task"},
	}
	m := dagsum.New(issues, "worker-1", "")
	m2, _ := m.Update(dagsum.ConfirmMsg{})
	updated := m2.(dagsum.Model)
	assert.Equal(t, 1, updated.Confirmed())
	assert.Equal(t, 1, updated.Cursor())
}

func TestAllConfirmedProducesOps(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task", Type: "task"},
	}
	m := dagsum.New(issues, "worker-1", "")
	m2, cmd := m.Update(dagsum.ConfirmMsg{})
	_ = m2
	assert.NotNil(t, cmd) // should produce a command to emit dag-transition op
}

func TestSkipDoesNotConfirm(t *testing.T) {
	issues := []*materialize.Issue{
		{ID: "TSK-1", Title: "Task", Type: "task"},
	}
	m := dagsum.New(issues, "worker-1", "")
	m2, _ := m.Update(dagsum.SkipMsg{})
	updated := m2.(dagsum.Model)
	assert.Equal(t, 0, updated.Confirmed())
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/dagsum/... -v
```

Expected: FAIL.

- [ ] **Step 3: Implement key bindings**

```go
// internal/tui/dagsum/keys.go
package dagsum

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines key bindings for dag-summary.
type KeyMap struct {
	Confirm key.Binding
	Skip    key.Binding
	Quit    key.Binding
	Up      key.Binding
	Down    key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Confirm: key.NewBinding(
			key.WithKeys("enter", "y"),
			key.WithHelp("enter/y", "confirm item"),
		),
		Skip: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "skip item"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
	}
}
```

- [ ] **Step 4: Implement BubbleTea model**

```go
// internal/tui/dagsum/model.go
package dagsum

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/armature/internal/materialize"
)

// ConfirmMsg signals the user confirmed the current item.
type ConfirmMsg struct{}

// SkipMsg signals the user skipped the current item.
type SkipMsg struct{}

// EmitDAGTransitionMsg carries a dag-transition op to emit.
type EmitDAGTransitionMsg struct {
	IssueID  string
	WorkerID string
}

// itemState tracks per-item review status.
type itemState int

const (
	itemPending itemState = iota
	itemConfirmed
	itemSkipped
)

// Model is the BubbleTea model for the dag-summary interactive TUI.
type Model struct {
	issues   []*materialize.Issue
	states   []itemState
	cursor   int
	workerID string
	opsDir   string
	keys     KeyMap
	done     bool
}

// New creates a dag-summary model for the given issues.
// workerID is the reviewing worker's UUID. opsDir is for emitting ops.
func New(issues []*materialize.Issue, workerID, opsDir string) Model {
	return Model{
		issues:   issues,
		states:   make([]itemState, len(issues)),
		workerID: workerID,
		opsDir:   opsDir,
		keys:     DefaultKeyMap(),
	}
}

// Total returns the number of items to review.
func (m Model) Total() int { return len(m.issues) }

// Confirmed returns the count of confirmed items.
func (m Model) Confirmed() int {
	n := 0
	for _, s := range m.states {
		if s == itemConfirmed {
			n++
		}
	}
	return n
}

// Cursor returns the current item index.
func (m Model) Cursor() int { return m.cursor }

// Done reports whether all items have been reviewed.
func (m Model) Done() bool { return m.done }

// ConfirmedIDs returns the IDs of all confirmed issues (used post-run for op emission).
func (m Model) ConfirmedIDs() []string {
	var ids []string
	for i, s := range m.states {
		if s == itemConfirmed {
			ids = append(ids, m.issues[i].ID)
		}
	}
	return ids
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
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

// View implements tea.Model.
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

	progress := fmt.Sprintf("Confirmed: %d/%d | enter=confirm  s=skip  q=quit",
		m.Confirmed(), len(m.issues))

	return fmt.Sprintf("%s\n%s  %s  %s\n\n%s\n",
		header, title, typeLabel, stateLabel, progress)
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/tui/dagsum/... -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/dagsum/
git commit -m "feat(tui/dagsum): BubbleTea model for dag-summary interactive review"
```

---

### Task 2: dag-summary command + non-interactive JSON path

**Files:**
- Create: `cmd/trellis/dagsum.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Implement dag-summary command**

```go
// cmd/trellis/dagsum.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/tui/dagsum"
	"github.com/scullxbones/armature/internal/traceability"
	"github.com/spf13/cobra"
)

func newDAGSummaryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dag-summary",
		Short: "Interactive TUI for reviewing and signing off DAG items",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			state, _, err := materialize.MaterializeAndReturn(issuesDir, true)
			if err != nil {
				return err
			}

			// Collect issues that need review (not yet dag-confirmed)
			var unconfirmed []*materialize.Issue
			for _, issue := range state.Issues {
				if !issue.Provenance.DAGConfirmed {
					unconfirmed = append(unconfirmed, issue)
				}
			}
			sort.Slice(unconfirmed, func(i, j int) bool {
				return unconfirmed[i].ID < unconfirmed[j].ID
			})

			if len(unconfirmed) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "All items already reviewed.")
				return nil
			}

			cov, _ := traceability.Read(issuesDir)

			// Non-interactive path: print pending items as JSON and return
			if !tui.IsInteractive() {
				type pendingItem struct {
					IssueID string `json:"issue_id"`
					Title   string `json:"title"`
					Status  string `json:"status"`
				}
				var pending []pendingItem
				for _, issue := range unconfirmed {
					pending = append(pending, pendingItem{
						IssueID: issue.ID,
						Title:   issue.Title,
						Status:  string(issue.Status),
					})
				}
				out, _ := json.Marshal(map[string]interface{}{
					"pending_dag_confirmation": pending,
					"count":                   len(pending),
				})
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"Traceability: %.1f%% (%d/%d nodes cited)\n\n",
				cov.CoveragePct, cov.CitedNodes, cov.TotalNodes)

			m := dagsum.New(unconfirmed, workerID, "")

			p := tea.NewProgram(m)
			finalModel, err := p.Run()
			if err != nil {
				return fmt.Errorf("dag-summary TUI: %w", err)
			}
			final := finalModel.(dagsum.Model)

			// Emit dag-transition ops for each confirmed item post-run.
			confirmedIDs := final.ConfirmedIDs()
			for _, id := range confirmedIDs {
				o := ops.Op{
					Type:      ops.OpDAGTransition,
					TargetID:  id,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
				}
				if err := appendLowStakesOp(logPath, o); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: emit dag-transition for %s: %v\n", id, err)
				}
			}

			// Write dag-summary.md artifact
			if err := writeDAGSummaryArtifact(issuesDir, unconfirmed, confirmedIDs, cov); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: write dag-summary.md: %v\n", err)
			}

			return nil
		},
	}
}
```

- [ ] **Step 2: Implement writeDAGSummaryArtifact**

Add to `cmd/trellis/dagsum.go`:

```go
func writeDAGSummaryArtifact(issuesDir string, reviewed []*materialize.Issue,
	confirmedIDs []string, cov traceability.Coverage) error {

	confirmedSet := make(map[string]struct{}, len(confirmedIDs))
	for _, id := range confirmedIDs {
		confirmedSet[id] = struct{}{}
	}

	var sb strings.Builder
	sb.WriteString("# DAG Summary Review\n\n")
	sb.WriteString(fmt.Sprintf("**Date:** %s\n\n", time.Now().UTC().Format("2006-01-02T15:04:05Z")))
	sb.WriteString(fmt.Sprintf("**Traceability:** %.1f%% (%d/%d cited)\n\n",
		cov.CoveragePct, cov.CitedNodes, cov.TotalNodes))

	sb.WriteString("## Review Results\n\n")
	sb.WriteString("| ID | Title | Status |\n|---|---|---|\n")
	for _, issue := range reviewed {
		status := "skipped"
		if _, ok := confirmedSet[issue.ID]; ok {
			status = "✓ confirmed"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", issue.ID, issue.Title, status))
	}

	path := filepath.Join(issuesDir, "state", "dag-summary.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}
```

- [ ] **Step 3: Register command in main.go**

Add to `cmd/trellis/main.go`:
```go
rootCmd.AddCommand(newDAGSummaryCmd())
```

- [ ] **Step 4: Build and verify**

```bash
go build ./cmd/trellis/... && ./bin/arm dag-summary --help
```

Expected: no errors, help displayed.

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/dagsum.go cmd/trellis/main.go internal/tui/dagsum/model.go internal/tui/dagsum/model_test.go
git commit -m "feat(dagsum): interactive TUI for DAG sign-off with non-interactive JSON path and dag-summary.md artifact"
```
