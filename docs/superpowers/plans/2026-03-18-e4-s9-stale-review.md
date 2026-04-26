# E4 Stale Review Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the stale-review BubbleTea TUI for reviewing issues affected by source document changes, with a non-interactive JSON path for agent/CI use.

**Spec:** `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` (E3-S9 section)

**Depends on:** `2026-03-18-e4-s8-brownfield-import.md`

**Execution order within E4:** S1 → S4 → S7 → S2 → S3 → S5 → S6 → S8 → S9

**Tech Stack:** Go 1.26, Bubble Tea v1.3, Lip Gloss v1.1, Cobra v1.8, testify

---

## File Structure

| Package | File | Responsibility |
|---|---|---|
| `internal/tui/stalereview` | `model.go` | BubbleTea model for stale-review TUI |
| `internal/tui/stalereview` | `model_test.go` | Unit tests |
| `cmd/trellis/stalereview.go` | — | `stale-review` command + non-interactive JSON path |
| `cmd/trellis/main.go` | — | Register `stale-review` command |

---

## Tasks

### Task 1: stale-review TUI

**Files:**
- Create: `internal/tui/stalereview/model.go`
- Create: `internal/tui/stalereview/model_test.go`
- Create: `cmd/trellis/stalereview.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/tui/stalereview/model_test.go
package stalereview_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	tuistalereview "github.com/scullxbones/armature/internal/tui/stalereview"
	"github.com/stretchr/testify/assert"
)

func TestNewModelHasItems(t *testing.T) {
	items := []tuistalereview.ReviewItem{
		{
			SourceID:      "prd",
			ChangeSummary: "Section 3 updated",
			CitedIssues:   []*materialize.Issue{{ID: "TSK-1", Title: "Task 1"}},
		},
	}
	m := tuistalereview.New(items, "worker-1")
	assert.Equal(t, 1, m.Total())
}

func TestConfirmRecordsDecision(t *testing.T) {
	items := []tuistalereview.ReviewItem{
		{SourceID: "prd", ChangeSummary: "Updated", CitedIssues: []*materialize.Issue{{ID: "TSK-1"}}},
	}
	m := tuistalereview.New(items, "worker-1")
	m2, _ := m.Update(tuistalereview.ConfirmMsg{})
	updated := m2.(tuistalereview.Model)
	assert.Equal(t, 1, updated.ConfirmedCount())
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/stalereview/... -v
```

Expected: FAIL.

- [ ] **Step 3: Implement stale-review model**

```go
// internal/tui/stalereview/model.go
package stalereview

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/armature/internal/materialize"
)

// ReviewItem represents a source change with its affected cited issues.
type ReviewItem struct {
	SourceID      string
	ChangeSummary string
	CitedIssues   []*materialize.Issue
}

// ConfirmMsg signals the user confirms the current source change is reviewed.
type ConfirmMsg struct{}

// FlagMsg signals the user wants to flag the current item for follow-up.
type FlagMsg struct{}

// SkipMsg signals the user skips the current item.
type SkipMsg struct{}

type itemDecision int

const (
	decisionPending itemDecision = iota
	decisionConfirmed
	decisionFlagged
	decisionSkipped
)

// Model is the BubbleTea model for the stale-review TUI.
type Model struct {
	items     []ReviewItem
	decisions []itemDecision
	cursor    int
	workerID  string
}

// New creates a stale-review model.
func New(items []ReviewItem, workerID string) Model {
	return Model{
		items:     items,
		decisions: make([]itemDecision, len(items)),
		workerID:  workerID,
	}
}

// Total returns the total number of review items.
func (m Model) Total() int { return len(m.items) }

// ConfirmedCount returns the number of confirmed items.
func (m Model) ConfirmedCount() int {
	n := 0
	for _, d := range m.decisions {
		if d == decisionConfirmed {
			n++
		}
	}
	return n
}

// Decisions returns the slice of decisions for post-run op emission.
func (m Model) Decisions() []itemDecision { return m.decisions }

// Items returns the review items.
func (m Model) Items() []ReviewItem { return m.items }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
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

// View implements tea.Model.
func (m Model) View() string {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return "Stale review complete.\n"
	}
	item := m.items[m.cursor]

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).
		Render(fmt.Sprintf("[%d/%d] Source changed: %s", m.cursor+1, len(m.items), item.SourceID))

	var sb strings.Builder
	sb.WriteString(header + "\n\n")
	sb.WriteString("Change summary: " + item.ChangeSummary + "\n\n")
	sb.WriteString("Affected issues:\n")
	for _, issue := range item.CitedIssues {
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", issue.ID, issue.Title))
	}
	sb.WriteString("\nenter=confirm  f=flag  s=skip  q=quit\n")
	return sb.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tui/stalereview/... -v
```

Expected: PASS.

- [ ] **Step 5: Implement stale-review command**

```go
// cmd/trellis/stalereview.go
package main

import (
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/scullxbones/armature/internal/tui"
	tuistalereview "github.com/scullxbones/armature/internal/tui/stalereview"
	"github.com/scullxbones/armature/internal/traceability"
	"github.com/spf13/cobra"
)

func newStaleReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stale-review",
		Short: "Review issues affected by source document changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			manifest, err := sources.ReadManifest(issuesDir)
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}

			cov, err := traceability.Read(issuesDir)
			if err != nil {
				return fmt.Errorf("read traceability: %w", err)
			}

			state, _, err := materialize.MaterializeAndReturn(issuesDir, true)
			if err != nil {
				return err
			}

			// Find sources with changed SHAs (those with fingerprint ops after last cache write)
			// Simplified: sources whose current file SHA differs from manifest.SHA
			var items []tuistalereview.ReviewItem
			for _, entry := range manifest.Sources {
				p, pErr := providerFor(entry)
				if pErr != nil {
					continue
				}
				_, currentSHA, _, fetchErr := p.Fetch(entry)
				if fetchErr != nil || currentSHA == entry.SHA {
					continue
				}
				// Find issues that cite this source
				var cited []*materialize.Issue
				for nodeID, links := range cov.Citations {
					for _, link := range links {
						if link.SourceID == entry.ID {
							if issue, ok := state.Issues[nodeID]; ok {
								cited = append(cited, issue)
							}
							break
						}
					}
				}
				items = append(items, tuistalereview.ReviewItem{
					SourceID:      entry.ID,
					ChangeSummary: fmt.Sprintf("SHA changed: %s → %s", entry.SHA[:8], currentSHA[:8]),
					CitedIssues:   cited,
				})
			}

			if len(items) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No stale sources detected.")
				return nil
			}

			// Non-interactive path: print stale items as JSON and return
			if !tui.IsInteractive() {
				type staleItem struct {
					SourceID       string   `json:"source_id"`
					ChangeSummary  string   `json:"change_summary"`
					AffectedIssues []string `json:"affected_issues"`
				}
				var out []staleItem
				for _, item := range items {
					var ids []string
					for _, iss := range item.CitedIssues {
						ids = append(ids, iss.ID)
					}
					out = append(out, staleItem{
						SourceID:       item.SourceID,
						ChangeSummary:  item.ChangeSummary,
						AffectedIssues: ids,
					})
				}
				b, _ := json.Marshal(map[string]interface{}{
					"stale_sources": out,
					"count":         len(out),
				})
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}

			m := tuistalereview.New(items, workerID)
			p := tea.NewProgram(m)
			finalModel, err := p.Run()
			if err != nil {
				return fmt.Errorf("stale-review TUI: %w", err)
			}
			final := finalModel.(tuistalereview.Model)

			// Emit note ops for each confirmed/flagged decision
			for i, decision := range final.Decisions() {
				if i >= len(final.Items()) {
					break
				}
				item := final.Items()[i]
				var msg string
				switch decision {
				case 1: // confirmed
					msg = fmt.Sprintf("stale-review: confirmed source %s change is accounted for", item.SourceID)
				case 2: // flagged
					msg = fmt.Sprintf("stale-review: flagged source %s change needs follow-up", item.SourceID)
				default:
					continue
				}
				for _, issue := range item.CitedIssues {
					o := ops.Op{
						Type:      ops.OpNote,
						TargetID:  issue.ID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload:   ops.Payload{Msg: msg},
					}
					_ = appendLowStakesOp(logPath, o)
				}
			}
			return nil
		},
	}
}
```

- [ ] **Step 6: Register command**

```go
rootCmd.AddCommand(newStaleReviewCmd())
```

- [ ] **Step 7: Build and verify**

```bash
go build ./cmd/trellis/...
```

Expected: no errors. Fix any type mismatches or import issues.

- [ ] **Step 8: Run full test suite**

```bash
make test
```

Expected: all tests pass, coverage ≥ 80%.

- [ ] **Step 9: Commit**

```bash
git add internal/tui/stalereview/ cmd/trellis/stalereview.go cmd/trellis/main.go
git commit -m "feat(stalereview): stale-review TUI for source change notification (E3-S9)"
```

---

## Final: Build + Coverage Check

- [ ] **Step 1: Full build**

```bash
make build
```

Expected: `./bin/arm` produced with no errors.

- [ ] **Step 2: Full test + coverage**

```bash
make test
make coverage-check
```

Expected: all tests pass, coverage ≥ 80%.

- [ ] **Step 3: Verify all new commands registered**

```bash
./bin/arm --help
```

Expected: output includes `sources`, `dag-summary`, `import`, `confirm`, `stale-review`.

- [ ] **Step 4: Smoke test sources workflow**

```bash
./bin/arm worker-init --check 2>/dev/null || ./bin/arm worker-init
./bin/arm sources add prd --provider filesystem --path docs/trellis-prd.md
./bin/arm sources verify
```

Expected: source added, verify passes.

- [ ] **Step 5: Smoke test validate**

```bash
./bin/arm validate --ci --format json
```

Expected: JSON output with `errors` and `warnings` arrays.

- [ ] **Step 6: Smoke test decompose-context**

```bash
./bin/arm decompose-context --plan docs/plan-post-bootstrap.json --sources prd
```

Expected: JSON output with `prompt_template`, `sources`, `plan_schema` fields.
