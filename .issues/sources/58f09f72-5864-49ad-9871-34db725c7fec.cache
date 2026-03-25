# E4 TUI Foundation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install the Charm TUI stack, define the semantic color palette, add TTY/IsInteractive detection, and create Bubbles component wrappers.

**Spec:** `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` (E3-S1 section)

**Depends on:** none

**Execution order within E4:** S1 → S4 → S7 → S2 → S3 → S5 → S6 → S8 → S9

**Tech Stack:** Go 1.26, Bubble Tea v1.3, Lip Gloss v1.1, Glamour v0.8, Bubbles v0.20, Cobra v1.8, testify

---

## File Structure

| Package | File | Responsibility |
|---|---|---|
| `internal/tui` | `colors.go` | Semantic color palette (7 lipgloss.Style vars) |
| `internal/tui` | `tty.go` | TTY detection + IsInteractive helper |
| `internal/tui` | `colors_test.go` | Palette render tests |
| `internal/tui` | `tty_test.go` | TTY detection and IsInteractive tests |
| `internal/tui` | `components.go` | Bubbles component wrappers |
| `go.mod` / `go.sum` | — | Add bubbletea, lipgloss, glamour, bubbles |
| `cmd/trellis/main.go` | — | Wire SetFormat in PersistentPreRunE |

---

## Tasks

### Task 1: Add Charm stack dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/charmbracelet/bubbletea@v1.3.4
go get github.com/charmbracelet/lipgloss@v1.1.0
go get github.com/charmbracelet/glamour@v0.8.0
go get github.com/charmbracelet/bubbles@v0.20.0
go mod tidy
```

- [ ] **Step 2: Verify build compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add Charm TUI stack (bubbletea, lipgloss, glamour, bubbles)"
```

---

### Task 2: Semantic color palette

**Files:**
- Create: `internal/tui/colors.go`
- Create: `internal/tui/colors_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/colors_test.go
package tui_test

import (
	"testing"

	"github.com/scullxbones/trellis/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestSemanticPalette(t *testing.T) {
	tests := []struct {
		name   string
		render func(string) string
	}{
		{"Critical", tui.Critical.Render},
		{"Warning", tui.Warning.Render},
		{"Advisory", tui.Advisory.Render},
		{"OK", tui.OK.Render},
		{"Info", tui.Info.Render},
		{"Muted", tui.Muted.Render},
		{"ActionRequired", tui.ActionRequired.Render},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Rendered output must at least contain the input string
			assert.Contains(t, tt.render("x"), "x")
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/tui/... -run TestSemanticPalette -v
```

Expected: FAIL — `package not found` or compile error.

- [ ] **Step 3: Implement color palette**

```go
// internal/tui/colors.go
package tui

import "github.com/charmbracelet/lipgloss"

// Semantic color palette (architecture.md §TUI Color Palette).
var (
	// Critical — blocking, must act. Bold red.
	Critical = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))

	// Warning — needs attention. Bold orange.
	Warning = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))

	// Advisory — informational flag. Yellow.
	Advisory = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))

	// OK — confirmed good. Green.
	OK = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))

	// Info — neutral information. Blue.
	Info = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	// Muted — secondary content. Gray.
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	// ActionRequired — explicit operator action. Bold white on red.
	ActionRequired = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("196"))
)
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/tui/... -run TestSemanticPalette -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/colors.go internal/tui/colors_test.go
git commit -m "feat(tui): semantic color palette"
```

---

### Task 3: TTY detection

**Files:**
- Create: `internal/tui/tty.go`
- Create: `internal/tui/tty_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/tty_test.go
package tui_test

import (
	"testing"

	"github.com/scullxbones/trellis/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestIsTerminalReturnsFalseInTests(t *testing.T) {
	// Test runner stdout is never a real TTY
	assert.False(t, tui.IsTerminal())
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/tui/... -run TestIsTerminal -v
```

Expected: FAIL — `IsTerminal undefined`.

- [ ] **Step 3: Implement TTY detection**

```go
// internal/tui/tty.go
package tui

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal reports whether stdout is connected to an interactive terminal.
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
```

Note: `golang.org/x/term` is an indirect dependency pulled in by bubbletea — no need to `go get` it separately. If it's missing run `go mod tidy`.

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/tui/... -run TestIsTerminal -v
```

Expected: PASS (returns false in CI/test environment).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tty.go internal/tui/tty_test.go
git commit -m "feat(tui): TTY detection helper"
```

---

### Task 4: IsInteractive helper

**Files:**
- Modify: `internal/tui/tty.go`
- Modify: `internal/tui/tty_test.go`

IsInteractive() returns true only when stdout is a TTY AND the cobra --format flag is NOT "json" or "agent".
Because cobra's flag is not available to internal/tui, we use a package-level setter called from the command layer.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/tty_test.go  (append to existing file)

func TestIsInteractiveRespectsMachineFormat(t *testing.T) {
    // When format is "json", IsInteractive must return false even if TTY were true
    tui.SetFormat("json")
    assert.False(t, tui.IsInteractive())
    tui.SetFormat("human")
}

func TestIsInteractiveRespectsTTY(t *testing.T) {
    tui.SetFormat("human")
    // In test runner, stdout is not a TTY — so IsInteractive is false
    assert.False(t, tui.IsInteractive())
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/tui/... -run TestIsInteractive -v
```

Expected: FAIL — IsInteractive undefined.

- [ ] **Step 3: Implement**

```go
// internal/tui/tty.go  (append to existing file)

var currentFormat string

// SetFormat is called by the root command PersistentPreRunE to propagate the --format flag value.
func SetFormat(f string) { currentFormat = f }

// IsInteractive reports whether the current invocation should run an interactive TUI.
// Returns false if stdout is not a terminal or if --format is "json" or "agent".
func IsInteractive() bool {
    if currentFormat == "json" || currentFormat == "agent" {
        return false
    }
    return IsTerminal()
}
```

- [ ] **Step 4: Wire SetFormat in root command**

In `cmd/trellis/main.go`, inside `PersistentPreRunE`, after resolving appCtx, add:

    format, _ := cmd.Root().PersistentFlags().GetString("format")
    tui.SetFormat(format)

Add import: "github.com/scullxbones/trellis/internal/tui"

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/tui/... -run TestIsInteractive -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/tty.go internal/tui/tty_test.go cmd/trellis/main.go
git commit -m "feat(tui): IsInteractive helper respects --format flag"
```

---

### Task 5: Bubbles component wrappers

**Files:**
- Create: `internal/tui/components.go`

No unit tests for thin wrappers — they delegate entirely to external libraries. Integration exercised by dag-summary and stale-review models.

- [ ] **Step 1: Implement component constructors**

```go
// internal/tui/components.go
package tui

import (
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// NewSpinner returns a dot spinner with the Info color.
func NewSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = Info
	return s
}

// NewProgressBar returns a gradient progress bar.
func NewProgressBar() progress.Model {
	return progress.New(
		progress.WithScaledGradient("#82ff82", "#ff4444"),
	)
}

// NewTable returns a focused table with the given column definitions and rows.
func NewTable(columns []table.Column, rows []table.Row) table.Model {
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.Bold(true).Foreground(lipgloss.Color("39"))
	s.Selected = s.Selected.Foreground(lipgloss.Color("82"))
	t.SetStyles(s)
	return t
}

// NewViewport returns a viewport sized to the given width and height.
func NewViewport(w, h int) viewport.Model {
	return viewport.New(w, h)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/tui/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/components.go
git commit -m "feat(tui): Bubbles component wrappers"
```
