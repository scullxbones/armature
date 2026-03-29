# TUI Redesign + Multi-Agent Worktree Remediation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the kanban board TUI with a DAG tree viewer + Workers/Validate/Sources screens, fix the semantic color palette, and eliminate multi-agent checkpoint collisions by giving each worker an isolated `stateDir`.

**Architecture:** Foundation first — add `StateDir` to `config.Context`, push it through all 16 `Materialize` callsites and 5 internal package APIs, then build the new TUI on top of the clean abstraction. Each Bubble Tea screen is its own package implementing a `Screen` interface; a root `app/` model owns the `tea.Program` and delegates to them.

**Tech Stack:** Go, Bubble Tea v1.3.4, Lipgloss v1.1.0, Bubbles v0.20.0, fsnotify (add as direct dep), atotto/clipboard (promote to direct dep).

---

## Chunk 1: Foundation

### Task 1: Fix Semantic Color Palette

**Files:**
- Modify: `internal/tui/colors.go`
- Modify: `internal/tui/colors_test.go`

- [ ] **Step 1: Write failing tests** — replace `colors_test.go` with assertions on specific color values and new styles:

`Critical`, `OK`, and `Muted` are kept at their existing hex values (`"#FF0000"`, `"#00CC44"`, `"#808080"`). Only the four wrong styles plus two new ones are asserted with xterm numbers.

```go
package tui

import (
    "testing"

    "github.com/charmbracelet/lipgloss"
)

func TestSemanticPalette(t *testing.T) {
    cases := []struct {
        name string
        style lipgloss.Style
        fg   lipgloss.Color
        bg   lipgloss.Color
        bold bool
    }{
        // Unchanged — keep hex form, assert exact values.
        {name: "Critical", style: Critical, fg: "#FF0000"},
        {name: "OK",       style: OK,       fg: "#00CC44"},
        {name: "Muted",    style: Muted,    fg: "#808080"},
        // Fixed — now use xterm-256 numbers per architecture.md §15.
        {name: "Warning",        style: Warning,        fg: "214", bold: true},
        {name: "Advisory",       style: Advisory,       fg: "226"},
        {name: "Info",           style: Info,           fg: "39"},
        {name: "ActionRequired", style: ActionRequired, fg: "15", bg: "196", bold: true},
        // New styles.
        {name: "MyClaim",    style: MyClaim,    fg: "#00CC44", bold: true},
        {name: "TheirClaim", style: TheirClaim, fg: "214",     bold: true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            if tc.fg != "" && tc.style.GetForeground() != lipgloss.Color(tc.fg) {
                t.Errorf("%s: foreground = %v, want %v", tc.name, tc.style.GetForeground(), tc.fg)
            }
            if tc.bg != "" && tc.style.GetBackground() != lipgloss.Color(tc.bg) {
                t.Errorf("%s: background = %v, want %v", tc.name, tc.style.GetBackground(), tc.bg)
            }
            if tc.bold && !tc.style.GetBold() {
                t.Errorf("%s: expected bold", tc.name)
            }
        })
    }
}
```

> `MyClaim` reuses OK green (`#00CC44`). `TheirClaim` reuses Warning orange (`"214"`). Both are bold.

- [ ] **Step 2: Run test — expect failures**

```
go test ./internal/tui/ -run TestSemanticPalette -v
```
Expected: FAIL (Warning/Advisory/Info/ActionRequired wrong, MyClaim/TheirClaim missing)

- [ ] **Step 3: Update `internal/tui/colors.go`** — change only the four wrong styles and add two new ones; leave `Critical`, `OK`, `Muted` untouched:

```go
package tui

import "github.com/charmbracelet/lipgloss"

var (
    // Unchanged.
    Critical = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
    OK       = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC44"))
    Muted    = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
    // Fixed per architecture.md §15.
    Warning        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
    Advisory       = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
    Info           = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
    ActionRequired = lipgloss.NewStyle().Bold(true).
                        Foreground(lipgloss.Color("15")).
                        Background(lipgloss.Color("196"))
    // New.
    MyClaim    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00CC44"))
    TheirClaim = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
)
```

- [ ] **Step 4: Run test — expect pass**

```
go test ./internal/tui/ -run TestSemanticPalette -v
```
Expected: PASS

- [ ] **Step 5: Run full suite — check existing TUI tests still pass**

```
go test ./internal/tui/... -v
```
Expected: all PASS (dagsum/stalereview/ready tests assert on behavior not ANSI codes)

- [ ] **Step 6: Commit**

```bash
git add internal/tui/colors.go internal/tui/colors_test.go
git commit -m "fix: correct semantic color palette to match architecture.md §15

Warning↔Advisory were swapped; Info was wrong cyan hex; ActionRequired was
magenta instead of bold-white-on-red. Adds MyClaim and TheirClaim styles."
```

---

### Task 2: Fix `trls init` Absolute Path + Idempotency (Problem A)

**Files:**
- Modify: `cmd/trellis/init.go`

- [ ] **Step 1: Write a failing integration test** in `cmd/trellis/main_test.go`

`runTrls` returns `(string, error)`; use `run()` (not `mustRun`) for shell commands. Add below `initTempRepo`:

```go
func TestInitDualBranchIdempotent(t *testing.T) {
    dir := t.TempDir()
    run(t, dir, "git", "init")
    run(t, dir, "git", "config", "user.email", "test@test.com")
    run(t, dir, "git", "config", "user.name", "Test")
    run(t, dir, "git", "config", "commit.gpgsign", "false")
    run(t, dir, "git", "commit", "--allow-empty", "-m", "init")

    // First init — must succeed.
    if _, err := runTrls(t, dir, "init", "--dual-branch"); err != nil {
        t.Fatalf("first init failed: %v", err)
    }

    // Stored worktree path must be absolute.
    out, err := exec.Command("git", "-C", dir, "config", "--local", "trellis.ops-worktree-path").Output()
    if err != nil {
        t.Fatalf("read ops-worktree-path: %v", err)
    }
    stored := strings.TrimSpace(string(out))
    if !filepath.IsAbs(stored) {
        t.Errorf("ops-worktree-path not absolute: %q", stored)
    }

    // Second init on the same repo must be idempotent (no error).
    if _, err := runTrls(t, dir, "init", "--dual-branch"); err != nil {
        t.Fatalf("second init should be idempotent, got error: %v", err)
    }
}
```

- [ ] **Step 2: Run test — expect failure**

```
go test ./cmd/trellis/ -run TestInitDualBranchIdempotent -v
```
Expected: FAIL (path is relative, second init errors)

- [ ] **Step 3: Update `cmd/trellis/init.go`** — fix lines 58–68:

```go
// Resolve worktreePath to absolute before use.
worktreePath, err := filepath.Abs(filepath.Join(repoPath, ".trellis"))
if err != nil {
    return fmt.Errorf("resolve worktree path: %w", err)
}

// Idempotency: skip worktree creation if already set and path exists.
existing, _ := gitClient.ReadGitConfig("trellis.ops-worktree-path")
if existing != "" {
    if _, statErr := os.Stat(existing); statErr == nil {
        // Already initialised — skip worktree creation.
        goto setIssuesDir
    }
}

if err := gitClient.AddWorktree("_trellis", worktreePath); err != nil {
    return fmt.Errorf("add .trellis worktree: %w", err)
}
if err := gitClient.SetGitConfig("trellis.mode", "dual-branch"); err != nil {
    return fmt.Errorf("set trellis.mode: %w", err)
}
if err := gitClient.SetGitConfig("trellis.ops-worktree-path", worktreePath); err != nil {
    return fmt.Errorf("set trellis.ops-worktree-path: %w", err)
}

setIssuesDir:
issuesDir = filepath.Join(worktreePath, ".issues")
```

> If you prefer to avoid `goto`, restructure as a helper function or conditional block — the logic is: check existing → skip or create → set issuesDir.

- [ ] **Step 4: Run test — expect pass**

```
go test ./cmd/trellis/ -run TestInitDualBranchIdempotent -v
```
Expected: PASS

- [ ] **Step 5: Run full cmd tests**

```
go test ./cmd/trellis/ -v 2>&1 | tail -20
```
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/init.go cmd/trellis/main_test.go
git commit -m "fix: trls init stores absolute worktree path and is idempotent

Second-worktree init no longer re-adds the _trellis checkout or stores
a relative path that breaks resolution from other working directories."
```

---

### Task 3: Add `StateDir` to `config.Context`

**Files:**
- Modify: `internal/config/context.go`

- [ ] **Step 1: Write a failing test** — add to the existing `internal/config/context_test.go`:

```go
func TestContextHasStateDirField(t *testing.T) {
    ctx := Context{StateDir: "/some/state"}
    if ctx.StateDir != "/some/state" {
        t.Errorf("StateDir not set: %q", ctx.StateDir)
    }
}
```

- [ ] **Step 2: Run test — expect compile failure**

```
go test ./internal/config/ -run TestContextHasStateDirField -v
```
Expected: compile error — `unknown field StateDir`

- [ ] **Step 3: Add `StateDir` field to `config.Context`** in `internal/config/context.go`:

```go
type Context struct {
    RepoPath     string
    IssuesDir    string
    WorktreePath string
    Mode         string
    Config       Config
    StateDir     string // per-worker materialization directory (set by main.go after ResolveContext)
}
```

- [ ] **Step 4: Run test — expect pass**

```
go test ./internal/config/ -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/context.go internal/config/context_test.go
git commit -m "feat: add StateDir field to config.Context

Populated by main.go after ResolveContext; holds the per-worker
materialization path (IssuesDir/state/<workerID>)."
```

---

### Task 4: Set `appCtx.StateDir` in `main.go`

**Files:**
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write a failing integration test** in `cmd/trellis/main_test.go`:

```go
func TestStateDirUsesWorkerID(t *testing.T) {
    repo := initTempRepo(t)
    // Initialize a worker
    runTrls(t, repo, "worker-init")
    // Run materialize — if StateDir is set correctly, state goes into worker subdir.
    runTrls(t, repo, "materialize")
    // StateDir should be IssuesDir/state/<workerID>/ — NOT IssuesDir/state/
    // The flat state/ dir should NOT contain checkpoint.json directly.
    flatCheckpoint := filepath.Join(repo, ".issues", "state", "checkpoint.json")
    if _, err := os.Stat(flatCheckpoint); err == nil {
        t.Errorf("checkpoint.json written to flat state/ instead of worker subdir")
    }
}
```

- [ ] **Step 2: Run test — expect failure** (checkpoint written to flat state/)

```
go test ./cmd/trellis/ -run TestStateDirUsesWorkerID -v
```
Expected: FAIL

- [ ] **Step 3: Update `PersistentPreRunE` in `cmd/trellis/main.go`** — after `appCtx = ctx`, add:

```go
// Set per-worker state directory. worker.GetWorkerID is already
// imported transitively through helpers.go.
workerID, _ := worker.GetWorkerID(repoPath)
if workerID == "" {
    workerID = "default"
}
appCtx.StateDir = filepath.Join(appCtx.IssuesDir, "state", workerID)
```

Add `"path/filepath"` and `"github.com/scullxbones/trellis/internal/worker"` to imports if not already present (worker is already imported via helpers.go in the same package, but main.go needs to import it directly since it's used here).

- [ ] **Step 4: Run test — expect pass** (after Task 5 updates Materialize callsite; this test will pass once pipeline.go also uses the stateDir param)

> Note: This test will only fully pass after Task 5 changes the Materialize signature. Proceed to Task 5 immediately.

- [ ] **Step 5: Commit** (after Task 5 passes — commit both together)

---

### Task 5: Update `Materialize` / `MaterializeAndReturn` Signatures

This task is a build-breaking refactor: update the signatures and ALL callsites in one atomic step.

**Files:**
- Modify: `internal/materialize/pipeline.go`
- Modify: `internal/materialize/engine_test.go`
- Modify: `cmd/trellis/assign.go`, `claim.go`, `confirm.go`, `dagsum.go`, `decompose.go`, `list.go`, `materialize.go`, `merged.go`, `ready.go`, `render_context.go`, `show.go`, `stalereview.go`, `status.go`, `sync.go`, `validate.go`, `tui.go`

- [ ] **Step 1: Update `internal/materialize/engine_test.go`** — change the two direct calls to 3-arg form:

Find line ~120 (`TestMaterializePipeline`):
```go
// Before:
result, err := Materialize(dir, true)
// After:
stateDir := filepath.Join(dir, "state", "default")
result, err := Materialize(dir, stateDir, true)
```

Find line ~383 (`TestMaterializeAndReturn_BasicPipeline`):
```go
// Before:
state, result, err := MaterializeAndReturn(dir, true)
// After:
stateDir := filepath.Join(dir, "state", "default")
state, result, err := MaterializeAndReturn(dir, stateDir, true)
```

Add `"path/filepath"` to imports if not already present.

- [ ] **Step 2: Run tests — expect compile error** (signatures not updated yet)

```
go build ./... 2>&1 | head -20
```
Expected: errors about wrong number of arguments

- [ ] **Step 3: Update `internal/materialize/pipeline.go`** signatures and remove internal `stateDir` derivations:

`Materialize` (line 34):
```go
func Materialize(issuesDir, stateDir string, singleBranch bool) (Result, error) {
    opsDir := filepath.Join(issuesDir, "ops")
    issuesStateDir := filepath.Join(stateDir, "issues")
    checkpointPath := filepath.Join(stateDir, "checkpoint.json")
    // (remove: stateDir := filepath.Join(issuesDir, "state"))
```

`MaterializeAndReturn` (line 135):
```go
func MaterializeAndReturn(issuesDir, stateDir string, singleBranch bool) (*State, Result, error) {
    opsDir := filepath.Join(issuesDir, "ops")
    issuesStateDir := filepath.Join(stateDir, "issues")
    checkpointPath := filepath.Join(stateDir, "checkpoint.json")
    // (remove: stateDir := filepath.Join(issuesDir, "state"))
```

Also update the two internal writes that still use `stateDir`:
```go
// index.json (pipeline.go ~102):
if err := WriteIndex(filepath.Join(stateDir, "index.json"), index); err != nil {

// ready.json (pipeline.go ~112):
readyPath := filepath.Join(stateDir, "ready.json")

// traceability.json (pipeline.go ~125):
_ = traceability.Write(filepath.Join(stateDir, "traceability.json"), cov)
```
These are already correct relative to `stateDir` — just ensure no remaining `filepath.Join(issuesDir, "state", ...)` lines exist.

- [ ] **Step 4: Update all 16 `cmd/trellis/` callsites** — add `appCtx.StateDir` as second argument:

For each of the following files, find the `Materialize(...)` or `MaterializeAndReturn(...)` call and add `appCtx.StateDir` as the second argument:

| File | Change |
|---|---|
| `assign.go` | `Materialize(issuesDir, appCtx.StateDir, ...)` |
| `claim.go` | `Materialize(issuesDir, appCtx.StateDir, ...)` |
| `confirm.go` | `MaterializeAndReturn(issuesDir, appCtx.StateDir, ...)` |
| `dagsum.go` | `MaterializeAndReturn(issuesDir, appCtx.StateDir, ...)` |
| `decompose.go` (×2) | `MaterializeAndReturn(issuesDir, appCtx.StateDir, ...)` |
| `list.go` | `Materialize(issuesDir, appCtx.StateDir, ...)` |
| `materialize.go` | `Materialize(issuesDir, appCtx.StateDir, ...)` (not MaterializeExcludeWorker) |
| `merged.go` | `Materialize(issuesDir, appCtx.StateDir, ...)` |
| `ready.go` | `Materialize(issuesDir, appCtx.StateDir, ...)` |
| `render_context.go` | `Materialize(issuesDir, appCtx.StateDir, ...)` |
| `show.go` | `Materialize(issuesDir, appCtx.StateDir, ...)` |
| `stalereview.go` | `MaterializeAndReturn(issuesDir, appCtx.StateDir, ...)` |
| `status.go` | `Materialize(issuesDir, appCtx.StateDir, ...)` |
| `sync.go` (×2) | `Materialize(issuesDir, appCtx.StateDir, ...)` |
| `validate.go` | `MaterializeAndReturn(issuesDir, appCtx.StateDir, ...)` |
| `tui.go` | `MaterializeAndReturn(issuesDir, filepath.Join(appCtx.IssuesDir, "state", ".tui"), ...)` |

For `tui.go` specifically: the TUI uses its own isolated state path, not `appCtx.StateDir`. Add `"path/filepath"` to imports if needed.

`MaterializeExcludeWorker` in `materialize.go` is **unchanged** — it keeps its 2-arg signature.

- [ ] **Step 5: Verify build compiles**

```
go build ./...
```
Expected: no errors

- [ ] **Step 6: Run tests**

```
go test ./internal/materialize/... ./cmd/trellis/... -v 2>&1 | tail -30
```
Expected: all PASS (including `TestStateDirUsesWorkerID` from Task 4)

- [ ] **Step 7: Commit Tasks 4 and 5 together**

```bash
git add internal/materialize/pipeline.go internal/materialize/engine_test.go \
    internal/config/context.go internal/config/context_test.go \
    cmd/trellis/main.go \
    cmd/trellis/assign.go cmd/trellis/claim.go cmd/trellis/confirm.go \
    cmd/trellis/dagsum.go cmd/trellis/decompose.go cmd/trellis/list.go \
    cmd/trellis/materialize.go cmd/trellis/merged.go cmd/trellis/ready.go \
    cmd/trellis/render_context.go cmd/trellis/show.go cmd/trellis/stalereview.go \
    cmd/trellis/status.go cmd/trellis/sync.go cmd/trellis/validate.go \
    cmd/trellis/tui.go cmd/trellis/main_test.go
git commit -m "feat: add StateDir to config.Context; give Materialize explicit stateDir param

Each worker now materializes into IssuesDir/state/<workerID>/ instead of
the shared IssuesDir/state/ directory. Resolves Problem B (checkpoint collision).
TUI uses its own IssuesDir/state/.tui/ path."
```

---

## Chunk 2: Secondary Path Propagation

### Task 6: Update cmd/trellis Secondary State Paths

> **Depends on Task 5** — these files call `Materialize`/`MaterializeAndReturn` with the new 3-arg signature; do not start Task 6 until Task 5 compiles and passes.

These files still construct `filepath.Join(issuesDir, "state", ...)` directly, bypassing `appCtx.StateDir`.

**Files:**
- Modify: `cmd/trellis/dagsum.go` (lines 40, 197)
- Modify: `cmd/trellis/list.go` (line 36)
- Modify: `cmd/trellis/merged.go` (line 27)
- Modify: `cmd/trellis/render_context.go` (line 89 — rename function)
- Modify: `cmd/trellis/show.go` (line 31)
- Modify: `cmd/trellis/status.go` (line 36)

- [ ] **Step 1: Update `cmd/trellis/dagsum.go`**

Line ~40 (tracePath):
```go
// Before:
tracePath := filepath.Join(issuesDir, "state", "traceability.json")
// After:
tracePath := filepath.Join(appCtx.StateDir, "traceability.json")
```

Line ~197 (`writeDAGSummaryArtifact`): add `stateDir string` parameter:
```go
// Before:
func writeDAGSummaryArtifact(issuesDir string, ...) error {
    path := filepath.Join(issuesDir, "state", "dag-summary.md")
// After:
func writeDAGSummaryArtifact(stateDir string, ...) error {
    path := filepath.Join(stateDir, "dag-summary.md")
```
Update the callsite at line ~137 to pass `appCtx.StateDir`.

- [ ] **Step 2: Update `cmd/trellis/list.go`** (line ~36):

```go
// Before:
index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
// After:
index, err := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
```

- [ ] **Step 3: Update `cmd/trellis/merged.go`** (line ~27):

```go
// Before:
index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
// After:
index, err := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
```

- [ ] **Step 4: Update `cmd/trellis/render_context.go`** (line ~89):

Rename `loadStateFromIssuesDir` to `loadStateFromStateDir` and update its path construction:
```go
// Before:
func loadStateFromIssuesDir(issuesDir string) (*materialize.State, error) {
    stateIssuesDir := filepath.Join(issuesDir, "state", "issues")
// After:
func loadStateFromStateDir(stateDir string) (*materialize.State, error) {
    stateIssuesDir := filepath.Join(stateDir, "issues")
```
Update all callsites of `loadStateFromIssuesDir` within `render_context.go` to call `loadStateFromStateDir(appCtx.StateDir)`.

- [ ] **Step 5: Update `cmd/trellis/show.go`** (line ~31):

```go
// Before:
path := filepath.Join(issuesDir, "state", "issues", id+".json")
// After:
path := filepath.Join(appCtx.StateDir, "issues", id+".json")
```

- [ ] **Step 6: Update `cmd/trellis/status.go`** (line ~36):

```go
// Before:
index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
// After:
index, err := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
```

- [ ] **Step 7: Verify build + run tests**

```
go build ./... && go test ./cmd/trellis/... -v 2>&1 | tail -20
```
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add cmd/trellis/dagsum.go cmd/trellis/list.go cmd/trellis/merged.go \
    cmd/trellis/render_context.go cmd/trellis/show.go cmd/trellis/status.go
git commit -m "fix: route secondary state paths through appCtx.StateDir

dagsum tracePath/artifact, list/merged/status index reads, render_context
loadState, show issue path — all now use worker-specific StateDir."
```

---

### Task 7: `sync.DetectMerges` stateDir Parameter

**Files:**
- Modify: `internal/sync/sync.go`
- Modify: `internal/sync/sync_test.go`
- Modify: `cmd/trellis/sync.go`

- [ ] **Step 1: Update `internal/sync/sync_test.go`** — change calls at lines ~50 and ~68 to pass stateDir:

```go
// Before:
ids, err := trellissync.DetectMerges(dir, "main", mc)
// After:
stateDir := filepath.Join(dir, "state")
ids, err := trellissync.DetectMerges(stateDir, "main", mc)
```
Add `"path/filepath"` import if needed.

- [ ] **Step 2: Run tests — expect compile error**

```
go test ./internal/sync/... 2>&1 | head -10
```
Expected: compile error (DetectMerges still takes issuesDir)

- [ ] **Step 3: Update `internal/sync/sync.go`** (line 19):

```go
// Before:
func DetectMerges(issuesDir, targetBranch string, mc MergeChecker) ([]string, error) {
    issuesStateDir := filepath.Join(issuesDir, "state", "issues")
// After:
func DetectMerges(stateDir, targetBranch string, mc MergeChecker) ([]string, error) {
    issuesStateDir := filepath.Join(stateDir, "issues")
```

- [ ] **Step 4: Update `cmd/trellis/sync.go`** callsite (line ~38):

```go
// Before:
mergedIDs, err := trellissync.DetectMerges(issuesDir, targetBranch, gc)
// After:
mergedIDs, err := trellissync.DetectMerges(appCtx.StateDir, targetBranch, gc)
```

- [ ] **Step 5: Run tests — expect pass**

```
go test ./internal/sync/... ./cmd/trellis/... -v 2>&1 | tail -10
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/sync/sync.go internal/sync/sync_test.go cmd/trellis/sync.go
git commit -m "fix: sync.DetectMerges accepts stateDir instead of issuesDir"
```

---

### Task 8: `validate.Options.StateDir`

**Files:**
- Modify: `internal/validate/validate.go`
- Modify: `cmd/trellis/validate.go`

**Important:** `validate.go` has TWO uses of `opts.IssuesDir`:
- Line ~44: `checkE7E8E12Citations(targets, opts.IssuesDir)` — reads actual issue source files from `.issues/`. **Do NOT change this to StateDir.**
- Line ~66: `filepath.Join(opts.IssuesDir, "state", "traceability.json")` — reads materialized state. **This is the only line that changes.**

- [ ] **Step 1: Write failing tests** — add to `internal/validate/validate_test.go` (or create if absent):

```go
func TestValidateUsesStateDirForTraceability(t *testing.T) {
    dir := t.TempDir()
    stateDir := filepath.Join(dir, "state", "worker1")
    _ = os.MkdirAll(stateDir, 0755)
    // Traceability file is in stateDir, NOT in issuesDir/state/.
    tracePath := filepath.Join(stateDir, "traceability.json")
    _ = os.WriteFile(tracePath, []byte(`{"coverage":[]}`), 0644)

    state := &materialize.State{Issues: map[string]*materialize.Issue{}}
    result := validate.Validate(state, validate.Options{
        IssuesDir: dir,
        StateDir:  stateDir,
    })
    if result.Coverage == nil {
        t.Error("expected traceability coverage from StateDir, got nil")
    }
}

func TestValidateTraceabilityIgnoredWhenStateDirEmpty(t *testing.T) {
    // When StateDir is empty, traceability read is skipped (no panic).
    state := &materialize.State{Issues: map[string]*materialize.Issue{}}
    result := validate.Validate(state, validate.Options{IssuesDir: t.TempDir()})
    // Coverage is nil when StateDir not set — IssuesDir alone does not trigger traceability read.
    if result.Coverage != nil {
        t.Error("expected nil coverage when StateDir is empty")
    }
}
```

- [ ] **Step 2: Run tests — expect compile failure** (StateDir field not on Options)

```
go test ./internal/validate/... -run TestValidate -v
```

- [ ] **Step 3: Add `StateDir` to `validate.Options`** — update **only** the traceability block (line ~66); leave `checkE7E8E12Citations` at line ~44 unchanged:

```go
type Options struct {
    ScopeID   string
    Strict    bool
    IssuesDir string
    RepoPath  string
    StateDir  string // worker-specific state directory; used for traceability.json ONLY
}
```

```go
// Before (line ~65-66):
if opts.IssuesDir != "" {
    tracePath := filepath.Join(opts.IssuesDir, "state", "traceability.json")
// After:
if opts.StateDir != "" {
    tracePath := filepath.Join(opts.StateDir, "traceability.json")
```

Line ~44 stays: `checkE7E8E12Citations(targets, opts.IssuesDir)` — **do not touch**.

- [ ] **Step 4: Update `cmd/trellis/validate.go`** callsite — set `StateDir: appCtx.StateDir`:

Find the `validate.Options{...}` construction and add `StateDir: appCtx.StateDir`.

- [ ] **Step 5: Run tests — expect pass**

```
go test ./internal/validate/... ./cmd/trellis/... -v 2>&1 | tail -10
```

- [ ] **Step 6: Commit**

```bash
git add internal/validate/validate.go cmd/trellis/validate.go
git commit -m "fix: validate.Options gains StateDir; traceability read from worker state dir"
```

---

### Task 9: `doctor.Run` stateDir Parameter

**Files:**
- Modify: `internal/doctor/doctor.go`
- Modify: `internal/doctor/doctor_test.go`
- Modify: `cmd/trellis/doctor.go`

- [ ] **Step 1: Update `internal/doctor/doctor_test.go`** — lines 142, 165, 191 call `doctor.Run(issuesDir, "")` with 2 args; change to 3:

```go
// Before:
report, err := doctor.Run(issuesDir, "")
// After:
stateDir := filepath.Join(issuesDir, "state", "default")
report, err := doctor.Run(issuesDir, stateDir, "")
```
Apply this to all 3 occurrences.

- [ ] **Step 2: Run test — expect compile error**

```
go test ./internal/doctor/... 2>&1 | head -5
```

- [ ] **Step 3: Update `internal/doctor/doctor.go`** signature and paths:

```go
// Before:
func Run(issuesDir string, repoPath string) (Report, error) {
    singleBranch := true
    if _, err := materialize.Materialize(issuesDir, singleBranch); err != nil {
    ...
    index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
    ...
    allIssues, err := loadAllIssues(issuesDir, index)

// After:
func Run(issuesDir, stateDir, repoPath string) (Report, error) {
    singleBranch := true
    if _, err := materialize.Materialize(issuesDir, stateDir, singleBranch); err != nil {
    ...
    index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
    ...
    allIssues, err := loadAllIssues(stateDir, index)
```

Update `loadAllIssues` parameter and its internal path:
```go
// Before:
func loadAllIssues(issuesDir string, index materialize.Index) (map[string]*materialize.Issue, error) {
    ...
    path := filepath.Join(issuesDir, "state", "issues", id+".json")
// After:
func loadAllIssues(stateDir string, index materialize.Index) (map[string]*materialize.Issue, error) {
    ...
    path := filepath.Join(stateDir, "issues", id+".json")
```

- [ ] **Step 4: Update `cmd/trellis/doctor.go`** callsite:

```go
// Before:
report, err := doctor.Run(issuesDir, repoPath)
// After:
report, err := doctor.Run(issuesDir, appCtx.StateDir, repoPath)
```

- [ ] **Step 5: Run tests — expect pass**

```
go test ./internal/doctor/... ./cmd/trellis/... -v 2>&1 | tail -10
```

- [ ] **Step 6: Commit**

```bash
git add internal/doctor/doctor.go internal/doctor/doctor_test.go cmd/trellis/doctor.go
git commit -m "fix: doctor.Run accepts stateDir; reads index/issues from worker state dir"
```

---

### Task 10: `context.Assemble` Parameter Rename

**Files:**
- Modify: `internal/context/assemble.go`
- Modify: `internal/context/context_test.go`
- Modify: `cmd/trellis/render_context.go`

- [ ] **Step 1: Update `internal/context/context_test.go`** — rename `issuesDir` variable to `stateDir` in the test setup for the 11 `Assemble` calls (lines ~29, 67, 105, 145, 164, 193, 219, 252, 285, 338, 367):

```go
// Before (typical pattern):
issuesDir := t.TempDir()
ctx, err := context.Assemble("E1-S1-T1", issuesDir, state)
// After:
stateDir := t.TempDir()
ctx, err := context.Assemble("E1-S1-T1", stateDir, state)
```

Verify the test still passes (rename is non-breaking since the parameter name doesn't affect the API yet):
```
go test ./internal/context/... -v 2>&1 | tail -10
```

- [ ] **Step 2: Update `internal/context/assemble.go`** — rename parameter in all three functions and fix internal paths:

```go
// Before:
func Assemble(issueID string, issuesDir string, state *materialize.State) (*Context, error) {
func buildBlockerOutcomes(issue *materialize.Issue, issuesDir string, state *materialize.State) Layer {
func buildParentChain(issue *materialize.Issue, issuesDir string, state *materialize.State) Layer {

// After:
func Assemble(issueID string, stateDir string, state *materialize.State) (*Context, error) {
func buildBlockerOutcomes(issue *materialize.Issue, stateDir string, state *materialize.State) Layer {
func buildParentChain(issue *materialize.Issue, stateDir string, state *materialize.State) Layer {
```

Update internal paths at lines 117, 139, 196, 212:
```go
// Before:
path := filepath.Join(issuesDir, "state", "issues", blockerID+".json")
// After:
path := filepath.Join(stateDir, "issues", blockerID+".json")
```
(Apply same change to the other 3 occurrences.)

- [ ] **Step 3: Update `cmd/trellis/render_context.go`** callsite at line ~54:

```go
// Before:
ctx, err := context.Assemble(rcIssue, issuesDir, state)
// After:
ctx, err := context.Assemble(rcIssue, appCtx.StateDir, state)
```

- [ ] **Step 4: Run tests — expect pass**

```
go test ./internal/context/... ./cmd/trellis/... -v 2>&1 | tail -10
```

- [ ] **Step 5: Commit**

```bash
git add internal/context/assemble.go internal/context/context_test.go \
    cmd/trellis/render_context.go
git commit -m "fix: context.Assemble renamed param issuesDir→stateDir; reads issues from stateDir directly"
```

---

### Task 11: `ops.NewFilePushTracker` Parameter Rename

**Files:**
- Modify: `internal/ops/tracker.go`
- Modify: `cmd/trellis/helpers.go`

- [ ] **Step 1: Write a failing test** in `internal/ops/tracker_test.go` (or add to existing):

```go
func TestNewFilePushTrackerUsesStateDirDirectly(t *testing.T) {
    stateDir := t.TempDir()
    tracker := ops.NewFilePushTracker(stateDir)
    expected := filepath.Join(stateDir, "pending-push-count")
    if tracker.Path != expected {
        t.Errorf("Path = %q, want %q", tracker.Path, expected)
    }
}
```

- [ ] **Step 2: Run test — expect failure** (current path is `stateDir/state/pending-push-count`)

```
go test ./internal/ops/... -run TestNewFilePushTrackerUsesStateDirDirectly -v
```

- [ ] **Step 3: Update `internal/ops/tracker.go`** — rename parameter and remove `"state"` segment:

```go
// Before:
func NewFilePushTracker(issuesDir string) *FilePushTracker {
    return &FilePushTracker{
        Path: filepath.Join(issuesDir, "state", "pending-push-count"),
    }
}
// After:
func NewFilePushTracker(stateDir string) *FilePushTracker {
    return &FilePushTracker{
        Path: filepath.Join(stateDir, "pending-push-count"),
    }
}
```

- [ ] **Step 4: Update `cmd/trellis/helpers.go`** callsite (line ~47):

```go
// Before:
appTracker = ops.NewFilePushTracker(appCtx.IssuesDir)
// After:
appTracker = ops.NewFilePushTracker(appCtx.StateDir)
```

- [ ] **Step 5: Run all tests**

```
go test ./internal/ops/... ./cmd/trellis/... -v 2>&1 | tail -10
```
Expected: PASS

- [ ] **Step 6: Run full `make check` for Chunk 2**

```
make check
```
Expected: lint + test + coverage ≥ 80% + mutate all green

- [ ] **Step 7: Commit**

```bash
git add internal/ops/tracker.go cmd/trellis/helpers.go
git commit -m "fix: NewFilePushTracker accepts stateDir directly; pending-push-count in worker dir"
```

---

## Chunk 3: TUI

> **Depends on Chunks 1 and 2 being merged.** Before starting Task 12, confirm: `config.Context.StateDir` exists, `Materialize`/`MaterializeAndReturn` take 3 args, and `internal/tui/screen.go` does NOT exist yet (it is created in Task 13). Run `go build ./...` to confirm the codebase compiles cleanly before proceeding.

### Task 12: Add `fsnotify` Direct Dependency + Promote `clipboard`

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add fsnotify as direct dependency**

```bash
cd /path/to/repo
go get github.com/fsnotify/fsnotify@latest
```

- [ ] **Step 2: Promote clipboard to direct dependency**

```bash
go get github.com/atotto/clipboard
```

- [ ] **Step 3: Tidy**

```bash
go mod tidy
```

- [ ] **Step 4: Verify build**

```
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add fsnotify as direct dep; promote clipboard to direct"
```

---

### Task 13: Add `Screen` Interface to `internal/tui`

The `Screen` interface lives in the **existing** `internal/tui` package (alongside `colors.go`) so that `app/` and each screen package can import it without creating circular dependencies. Screen packages already import `internal/tui` for colors — no new dependency is added.

**Files:**
- Create: `internal/tui/screen.go`

- [ ] **Step 1: Create `internal/tui/screen.go`**

```go
package tui

import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/scullxbones/trellis/internal/materialize"
)

// Screen is implemented by every TUI sub-screen (dagtree, workers, validate, sources).
// It is NOT a tea.Model — the root app.Model owns the tea.Program and delegates explicitly.
type Screen interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Screen, tea.Cmd)
    View() string
    HelpBar() string
    SetSize(width, height int)
    SetState(state *materialize.State)
}
```

- [ ] **Step 2: Verify it compiles**

```
go build ./internal/tui/...
```
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/tui/screen.go
git commit -m "feat: add Screen interface to internal/tui package

Shared interface avoids circular imports between app/ and screen packages."
```

---

### Task 14: `internal/tui/detail/` Overlay

**Files:**
- Create: `internal/tui/detail/model.go`
- Create: `internal/tui/detail/model_test.go`

- [ ] **Step 1: Write failing tests** in `internal/tui/detail/model_test.go`:

```go
package detail_test

import (
    "strings"
    "testing"

    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui/detail"
)

func TestDetailClosedByDefault(t *testing.T) {
    m := detail.New()
    if m.IsOpen() {
        t.Error("detail overlay should be closed by default")
    }
}

func TestDetailShowsIssueID(t *testing.T) {
    m := detail.New()
    issue := &materialize.Issue{ID: "E4-S1-T2", Title: "Build TUI", Status: "open", Type: "task"}
    m = m.Open(issue)
    if !m.IsOpen() {
        t.Error("Open() should set IsOpen = true")
    }
    v := m.View()
    if !strings.Contains(v, "E4-S1-T2") {
        t.Errorf("View() missing issue ID, got: %q", v)
    }
    if !strings.Contains(v, "Build TUI") {
        t.Errorf("View() missing title, got: %q", v)
    }
}

func TestDetailDismissedByEsc(t *testing.T) {
    m := detail.New()
    issue := &materialize.Issue{ID: "X1", Title: "t"}
    m = m.Open(issue)
    tea := detail.HandleKey(m, "esc")
    if tea.IsOpen() {
        t.Error("Esc should close the detail overlay")
    }
}

func TestDetailSetsSize(t *testing.T) {
    m := detail.New()
    m.SetSize(120, 40)
    // No assertion needed — just verify no panic.
}

func TestDetailCopyIDKey(t *testing.T) {
    m := detail.New()
    issue := &materialize.Issue{ID: "E4-S1-T2"}
    m = m.Open(issue)
    // 'c' copies the ID — should not panic even in headless env.
    _ = detail.HandleKey(m, "c")
}
```

- [ ] **Step 2: Run tests — expect compile error** (package doesn't exist)

```
go test ./internal/tui/detail/... -v 2>&1 | head -10
```

- [ ] **Step 3: Create `internal/tui/detail/model.go`**

```go
package detail

import (
    "fmt"
    "strings"

    "github.com/atotto/clipboard"
    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui"
)

// Model is the shared detail overlay used by dagtree and workers screens.
// It renders a centred scrollable box showing all fields of a single issue.
type Model struct {
    open     bool
    issue    *materialize.Issue
    viewport viewport.Model
    width    int
    height   int
}

func New() Model {
    return Model{}
}

func (m Model) IsOpen() bool { return m.open }

func (m Model) Open(issue *materialize.Issue) Model {
    m.open = true
    m.issue = issue
    content := buildContent(issue)
    vp := viewport.New(min(m.width-4, 90), m.height-6)
    vp.SetContent(content)
    m.viewport = vp
    return m
}

func (m Model) Close() Model {
    m.open = false
    m.issue = nil
    return m
}

func (m Model) SetSize(width, height int) Model {
    m.width = width
    m.height = height
    if m.open {
        m.viewport.Width = min(width-4, 90)
        m.viewport.Height = height - 6
    }
    return m
}

// HandleKey processes a key press and returns an updated Model.
func HandleKey(m Model, key string) Model {
    switch key {
    case "esc":
        return m.Close()
    case "j":
        m.viewport.LineDown(1)
    case "k":
        m.viewport.LineUp(1)
    case "c":
        if m.issue != nil {
            if err := clipboard.WriteAll(m.issue.ID); err != nil {
                // Headless fallback: no-op (clipboard unavailable in SSH/container).
                _ = err
            }
        }
    }
    return m
}

// Update implements the Bubble Tea update cycle for embedding in a parent model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    if !m.open {
        return m, nil
    }
    switch msg := msg.(type) {
    case tea.KeyMsg:
        m = HandleKey(m, msg.String())
    }
    return m, nil
}

// View renders the overlay. The caller is responsible for layering it on top of
// the dimmed screen content.
func (m Model) View() string {
    if m.issue == nil {
        return ""
    }
    boxWidth := min(m.width-4, 90)
    border := lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(lipgloss.Color("39")).
        Width(boxWidth).
        Padding(0, 1)
    return border.Render(m.viewport.View())
}

func buildContent(issue *materialize.Issue) string {
    var b strings.Builder
    fmt.Fprintf(&b, "%s  %s\n", tui.Info.Render(issue.ID), issue.Title)
    fmt.Fprintf(&b, "Type: %s  Status: %s  Priority: %s\n",
        issue.Type, issue.Status, issue.Priority)
    if issue.Acceptance != "" {
        fmt.Fprintf(&b, "\nAcceptance:\n%s\n", issue.Acceptance)
    }
    if issue.Outcome != "" {
        fmt.Fprintf(&b, "\nOutcome:\n%s\n", issue.Outcome)
    }
    if len(issue.Decisions) > 0 {
        fmt.Fprintf(&b, "\nDecisions:\n")
        for _, d := range issue.Decisions {
            fmt.Fprintf(&b, "  - %s\n", d)
        }
    }
    if len(issue.Notes) > 0 {
        fmt.Fprintf(&b, "\nNotes:\n")
        for _, n := range issue.Notes {
            fmt.Fprintf(&b, "  - %s\n", n)
        }
    }
    return b.String()
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

> Fix the `if !m.issue` check — use `if m.issue == nil { return "" }` since `issue` is a pointer. The code above has a bug there.

- [ ] **Step 4: Run tests — expect pass**

```
go test ./internal/tui/detail/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/detail/
git commit -m "feat: add detail/ overlay for issue detail panel

Centred scrollable box with j/k scrolling, Esc to dismiss,
c to copy ID to clipboard (no-op in headless environments)."
```

---

### Task 15: `internal/tui/app/` Root Model

**Files:**
- Create: `internal/tui/app/model.go`
- Create: `internal/tui/app/model_test.go`

The `app/` package owns the `tea.Program`, nav bar, screen routing, terminal size, and state refresh loop.

- [ ] **Step 1: Write failing tests** in `internal/tui/app/model_test.go`:

```go
package app_test

import (
    "strings"
    "testing"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui/app"
)

func TestInitialScreenIsDAGTree(t *testing.T) {
    m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
    if m.CurrentScreen() != app.ScreenDAGTree {
        t.Errorf("initial screen = %v, want ScreenDAGTree", m.CurrentScreen())
    }
}

func TestScreenSwitchByNumber(t *testing.T) {
    m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
    for key, want := range map[string]app.ScreenID{
        "1": app.ScreenDAGTree,
        "2": app.ScreenWorkers,
        "3": app.ScreenValidate,
        "4": app.ScreenSources,
    } {
        updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
        got := updated.(app.Model).CurrentScreen()
        if got != want {
            t.Errorf("key %q: screen = %v, want %v", key, got, want)
        }
    }
}

func TestSetStatePropagates(t *testing.T) {
    m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
    state := &materialize.State{Issues: map[string]*materialize.Issue{
        "T1": {ID: "T1", Status: "open"},
    }}
    m = m.WithState(state)
    // Nav bar should render without panic.
    v := m.View()
    if !strings.Contains(v, "[1]") {
        t.Errorf("nav bar missing screen tab, got: %q", v)
    }
}

func TestNavBarShowsValidateBadgeWhenErrors(t *testing.T) {
    m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
    state := &materialize.State{Issues: map[string]*materialize.Issue{
        "T1": {ID: "T1", Status: "open"},
    }}
    m = m.WithState(state).WithValidateErrors(3)
    nav := m.NavBar()
    if !strings.Contains(nav, "⚠3") {
        t.Errorf("nav bar missing error badge, got: %q", nav)
    }
}

func TestQuitKey(t *testing.T) {
    m := app.New("/tmp/issues", "/tmp/issues/state/.tui", "default")
    _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
    if cmd == nil {
        t.Error("q should return a quit command")
    }
}
```

- [ ] **Step 2: Run tests — expect compile error**

```
go test ./internal/tui/app/... -v 2>&1 | head -10
```

- [ ] **Step 3: Create `internal/tui/app/model.go`**

`app/model.go` imports `tui.Screen` (defined in `internal/tui/screen.go` from Task 13) to avoid circular imports with the screen packages.

```go
package app

import (
    "fmt"
    "path/filepath"
    "strings"
    "time"

    "github.com/fsnotify/fsnotify"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
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

// refreshMsg triggers a re-materialisation.
type refreshMsg struct{}

// fetchMsg triggers a git fetch.
type fetchMsg struct{}

// pollTickMsg is sent by the fallback poll ticker.
type pollTickMsg time.Time

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
        m.screens[i].SetState(state)
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
        if cmd := s.Init(); cmd != nil {
            cmds = append(cmds, cmd)
        }
    }
    cmds = append(cmds, m.startWatcher())
    cmds = append(cmds, m.scheduleFetch())
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
            w.Close()
            return pollTickMsg(time.Now())
        }
        // Return watcher — caller stores it and starts listening.
        return watcherReadyMsg{watcher: w}
    }
}

type watcherReadyMsg struct{ watcher *fsnotify.Watcher }

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
        label := "[" + string(rune('1'+i)) + "] " + name
        // Add badge if applicable.
        if ScreenID(i) == ScreenValidate && m.validateErrors > 0 {
            label += tui.Critical.Render(" ⚠" + itoa(m.validateErrors))
        }
        if ScreenID(i) == ScreenSources && m.staleSources > 0 {
            label += tui.Advisory.Render(" ⚠" + itoa(m.staleSources))
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
            m.screens[i].SetSize(msg.Width, msg.Height-2)
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
            updated, cmd := m.screens[m.current].Update(msg)
            m.screens[m.current] = updated
            return m, cmd
        }
    case watcherReadyMsg:
        m.watcher = msg.watcher
        m.liveMode = true
        return m, m.listenForChanges()
    case refreshMsg:
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
            return refreshMsg{}
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

type stateUpdatedMsg struct{ state *materialize.State }

func (m Model) doFetch() tea.Cmd {
    issuesDir := m.issuesDir
    return func() tea.Msg {
        // Best-effort git fetch with 10s timeout — errors are silent.
        _ = issuesDir // git fetch implementation goes here via git.Client
        return nil
    }
}

// View renders nav bar + active screen + help bar.
func (m Model) View() string {
    nav := m.NavBar()
    content := m.screens[m.current].View()
    help := m.screens[m.current].HelpBar()
    return nav + "\n" + content + "\n" + help
}

// nilScreen is a placeholder Screen used before real screens are injected.
type nilScreen struct{}

func (nilScreen) Init() tea.Cmd                               { return nil }
func (n nilScreen) Update(_ tea.Msg) (tui.Screen, tea.Cmd)   { return n, nil }
func (nilScreen) View() string                                { return "" }
func (nilScreen) HelpBar() string                             { return "" }
func (nilScreen) SetSize(_, _ int)                            {}
func (nilScreen) SetState(_ *materialize.State)               {}

func itoa(n int) string {
    return fmt.Sprintf("%d", n)
}
```

> Add missing imports: `"fmt"`, `"strings"`. Also add a `HelpBar() string` method to the `Screen` interface and to `nilScreen`. The `View()` type assertion to the interface will panic if the screen doesn't implement it — add it to the `Screen` interface directly.

- [ ] **Step 4: Run tests — expect pass**

```
go test ./internal/tui/app/... -v
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tui/app/
git commit -m "feat: add app/ root model with nav bar, screen routing, fsnotify watcher

Owns the tea.Program; delegates to Screen interface implementors.
Fallback poll at 5s if fsnotify unavailable. Remote fetch every 30s."
```

---

### Task 16: `internal/tui/dagtree/` Screen

**Files:**
- Create: `internal/tui/dagtree/model.go`
- Create: `internal/tui/dagtree/model_test.go`

- [ ] **Step 1: Write failing tests** in `internal/tui/dagtree/model_test.go`:

```go
package dagtree_test

import (
    "strings"
    "testing"

    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui/dagtree"
)

func makeState(issues ...*materialize.Issue) *materialize.State {
    s := materialize.NewState()
    for _, i := range issues {
        issueCopy := *i
        s.Issues[i.ID] = &issueCopy
    }
    return s
}

func TestViewContainsIssueIDs(t *testing.T) {
    m := dagtree.New()
    m.SetSize(120, 40)
    m.SetState(makeState(
        &materialize.Issue{ID: "E1", Type: "epic", Status: "open", Title: "Epic One"},
        &materialize.Issue{ID: "E1-S1", Type: "story", Status: "open", Title: "Story One", Parent: "E1"},
    ))
    v := m.View()
    if !strings.Contains(v, "E1") {
        t.Errorf("View missing epic ID, got:\n%s", v)
    }
    if !strings.Contains(v, "E1-S1") {
        t.Errorf("View missing story ID, got:\n%s", v)
    }
}

func TestMergedNodeShowsCheckGlyph(t *testing.T) {
    m := dagtree.New()
    m.SetSize(120, 40)
    m.SetState(makeState(&materialize.Issue{ID: "T1", Type: "task", Status: "merged", Title: "Done task"}))
    v := m.View()
    if !strings.Contains(v, "✓") {
        t.Errorf("merged node should show ✓ glyph, got:\n%s", v)
    }
}

func TestBlockedNodeShowsXGlyph(t *testing.T) {
    m := dagtree.New()
    m.SetSize(120, 40)
    m.SetState(makeState(&materialize.Issue{ID: "T1", Type: "task", Status: "blocked", Title: "Blocked"}))
    v := m.View()
    if !strings.Contains(v, "✗") {
        t.Errorf("blocked node should show ✗ glyph, got:\n%s", v)
    }
}

func TestFilterHidesNonMatchingNodes(t *testing.T) {
    m := dagtree.New()
    m.SetSize(120, 40)
    m.SetState(makeState(
        &materialize.Issue{ID: "E1", Type: "epic", Status: "open", Title: "Epic"},
        &materialize.Issue{ID: "E1-S1", Type: "story", Status: "open", Title: "visible", Parent: "E1"},
        &materialize.Issue{ID: "E1-S2", Type: "story", Status: "open", Title: "hidden", Parent: "E1"},
    ))
    m = m.WithFilter("visible")
    v := m.View()
    if strings.Contains(v, "hidden") {
        t.Errorf("filter should hide non-matching nodes; got:\n%s", v)
    }
    if !strings.Contains(v, "E1") {
        t.Errorf("filter should keep ancestors visible; got:\n%s", v)
    }
}

func TestHelpBarContainsKeyHints(t *testing.T) {
    m := dagtree.New()
    m.SetSize(120, 40)
    h := m.HelpBar()
    if !strings.Contains(h, "j/k") {
        t.Errorf("help bar missing j/k hint, got: %q", h)
    }
    if !strings.Contains(h, "/") {
        t.Errorf("help bar missing filter hint, got: %q", h)
    }
}
```

- [ ] **Step 2: Run tests — expect compile error**

```
go test ./internal/tui/dagtree/... 2>&1 | head -5
```

- [ ] **Step 3: Create `internal/tui/dagtree/model.go`**

Key design points:
- `Model` struct holds: `state`, `roots []string` (epic IDs in order), `expanded map[string]bool`, `cursor int`, `filter string`, `width int`, `height int`
- `SetState` builds the ordered flat visible list from the DAG by doing a depth-first traversal from roots (epics with no parent), using `expanded` map to skip collapsed subtrees
- `View()` renders each visible node as: `PREFIX GLYPH ID  TITLE … STATUS`, with the selected row highlighted
- `glyphFor(status) string` returns ✓/▶/○/✗/— per the spec table
- `WithFilter(q string) Model` sets the filter and rebuilds visible list

```go
package dagtree

import (
    "fmt"
    "sort"
    "strings"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui"
    "github.com/scullxbones/trellis/internal/tui/detail"
)

// visibleNode is a rendered tree row.
type visibleNode struct {
    issue  *materialize.Issue
    depth  int
    isLast bool
}

// Model implements app.Screen for the DAG tree view.
type Model struct {
    state   *materialize.State
    visible []visibleNode
    expanded map[string]bool
    cursor  int
    filter  string
    width   int
    height  int
    detail  detail.Model
}

func New() *Model {
    return &Model{expanded: make(map[string]bool)}
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) SetSize(width, height int) {
    m.width = width
    m.height = height
    m.detail.SetSize(width, height)
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
        m.detail, _ = m.detail.Update(msg)
        return m, nil
    }
    if msg, ok := msg.(tea.KeyMsg); ok {
        switch msg.String() {
        case "j", "down":
            if m.cursor < len(m.visible)-1 {
                m.cursor++
            }
        case "k", "up":
            if m.cursor > 0 {
                m.cursor--
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

// renderTree renders the paged node list with no overlay.
func (m *Model) renderTree() string {
    var lines []string
    start := 0
    end := len(m.visible)
    pageHeight := m.height - 2
    if pageHeight > 0 && m.cursor >= start+pageHeight {
        start = m.cursor - pageHeight + 1
    }
    if end > start+pageHeight && pageHeight > 0 {
        end = start + pageHeight
    }
    for i := start; i < end && i < len(m.visible); i++ {
        lines = append(lines, m.renderNode(i))
    }
    return strings.Join(lines, "\n")
}

func (m *Model) View() string {
    tree := m.renderTree()
    if !m.detail.IsOpen() {
        return tree
    }
    return m.renderWithOverlay(tree)
}

func (m *Model) renderNode(idx int) string {
    node := m.visible[idx]
    issue := node.issue
    prefix := strings.Repeat("│   ", node.depth)
    if node.isLast {
        prefix += "└── "
    } else {
        prefix += "├── "
    }
    glyph := glyphFor(issue.Status)
    idStr := tui.Info.Render(issue.ID)
    title := issue.Title
    available := m.width - lipgloss.Width(prefix+glyph+" "+issue.ID+"  ") - 12
    if available > 0 && lipgloss.Width(title) > available {
        title = title[:available-1] + "…"
    }
    status := ""
    if issue.Status != "merged" && issue.Status != "cancelled" {
        status = tui.Muted.Render(" " + issue.Status)
    }
    row := tui.Muted.Render(prefix) + glyph + " " + idStr + "  " + title + status
    if idx == m.cursor {
        row = lipgloss.NewStyle().Background(lipgloss.Color("39")).
            Foreground(lipgloss.Color("0")).Width(m.width).Render(row)
    }
    return row
}

func (m *Model) renderWithOverlay(tree string) string {
    overlay := m.detail.View()
    // Centre the overlay box over the tree content.
    lines := strings.Split(tree, "\n")
    overlayLines := strings.Split(overlay, "\n")
    midRow := len(lines)/2 - len(overlayLines)/2
    for i, ol := range overlayLines {
        row := midRow + i
        if row >= 0 && row < len(lines) {
            lines[row] = ol
        }
    }
    return strings.Join(lines, "\n")
}

func (m *Model) rebuild() {
    if m.state == nil {
        m.visible = nil
        return
    }
    m.visible = buildVisible(m.state, m.expanded, m.filter)
    if m.cursor >= len(m.visible) && len(m.visible) > 0 {
        m.cursor = len(m.visible) - 1
    }
}

func buildVisible(state *materialize.State, expanded map[string]bool, filter string) []visibleNode {
    // Find root nodes (no parent, or parent not in state).
    var roots []*materialize.Issue
    for _, issue := range state.Issues {
        if issue.Parent == "" || state.Issues[issue.Parent] == nil {
            roots = append(roots, issue)
        }
    }
    // Sort roots by type: epics first, then stories, then tasks.
    sortByTypeAndID(roots)

    var nodes []visibleNode
    var walk func(issue *materialize.Issue, depth int, isLast bool)
    walk = func(issue *materialize.Issue, depth int, isLast bool) {
        show := filter == "" || matchesFilter(issue, filter) || hasMatchingDescendant(state, issue, filter)
        if show {
            nodes = append(nodes, visibleNode{issue: issue, depth: depth, isLast: isLast})
        }
        if expanded[issue.ID] || depth == 0 {
            var children []*materialize.Issue
            for _, candidate := range state.Issues {
                if candidate.Parent == issue.ID {
                    children = append(children, candidate)
                }
            }
            sortByTypeAndID(children)
            for i, child := range children {
                walk(child, depth+1, i == len(children)-1)
            }
        }
    }
    for i, root := range roots {
        walk(root, 0, i == len(roots)-1)
    }
    return nodes
}

func matchesFilter(issue *materialize.Issue, q string) bool {
    q = strings.ToLower(q)
    return strings.Contains(strings.ToLower(issue.ID), q) ||
        strings.Contains(strings.ToLower(issue.Title), q) ||
        strings.Contains(strings.ToLower(issue.Status), q)
}

func hasMatchingDescendant(state *materialize.State, issue *materialize.Issue, filter string) bool {
    for _, candidate := range state.Issues {
        if candidate.Parent == issue.ID {
            if matchesFilter(candidate, filter) || hasMatchingDescendant(state, candidate, filter) {
                return true
            }
        }
    }
    return false
}

func glyphFor(status string) string {
    switch status {
    case "merged":
        return tui.OK.Render("✓")
    case "done":
        return tui.Advisory.Render("✓")
    case "in-progress", "claimed":
        return tui.Warning.Render("▶")
    case "open":
        return tui.Info.Render("○")
    case "blocked":
        return tui.Critical.Render("✗")
    case "cancelled":
        return tui.Muted.Render("—")
    default:
        return tui.Muted.Render("?")
    }
}

func sortByTypeAndID(issues []*materialize.Issue) {
    typeOrder := map[string]int{"epic": 0, "story": 1, "task": 2}
    sort.Slice(issues, func(i, j int) bool {
        oi := typeOrder[issues[i].Type]
        oj := typeOrder[issues[j].Type]
        if oi != oj {
            return oi < oj
        }
        return issues[i].ID < issues[j].ID
    })
}
```

- [ ] **Step 4: Run tests — expect pass**

```
go test ./internal/tui/dagtree/... -v
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tui/dagtree/
git commit -m "feat: add dagtree/ screen — indented DAG tree with collapse/expand/filter

Depth-first traversal from roots; status glyphs per semantic palette;
filter mode shows matching nodes and their ancestors; detail overlay on Enter."
```

---

### Task 17: `internal/tui/workers/` Screen

**Files:**
- Create: `internal/tui/workers/model.go`
- Create: `internal/tui/workers/model_test.go`

- [ ] **Step 1: Write failing tests**:

```go
package workers_test

import (
    "strings"
    "testing"
    "time"

    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui/workers"
)

// Worker data comes from Issue.ClaimedBy — there is no separate Claims map on State.

func TestViewShowsWorkerIDs(t *testing.T) {
    m := workers.New()
    m.SetSize(120, 40)
    now := time.Now().Unix()
    m.SetState(&materialize.State{
        Issues: map[string]*materialize.Issue{
            "T1": {ID: "T1", Title: "Build something", Status: "claimed",
                ClaimedBy: "agent-1", ClaimedAt: now, ClaimTTL: 3600, LastHeartbeat: now},
        },
    })
    v := m.View()
    if !strings.Contains(v, "agent-1") {
        t.Errorf("View missing worker ID, got:\n%s", v)
    }
}

func TestHealthGlyphGreenForActiveWorker(t *testing.T) {
    m := workers.New()
    m.SetSize(120, 40)
    now := time.Now().Unix()
    m.SetState(&materialize.State{
        Issues: map[string]*materialize.Issue{
            "T1": {ID: "T1", Title: "t", ClaimedBy: "w1",
                ClaimedAt: now, ClaimTTL: 3600, LastHeartbeat: now},
        },
    })
    v := m.View()
    if !strings.Contains(v, "●") {
        t.Errorf("active worker should show ● health glyph, got:\n%s", v)
    }
}

func TestExpiredWorkerShowsWarning(t *testing.T) {
    m := workers.New()
    m.SetSize(120, 40)
    stale := time.Now().Unix() - 7200 // 2h ago — exceeds 1h TTL
    m.SetState(&materialize.State{
        Issues: map[string]*materialize.Issue{
            "T1": {ID: "T1", Title: "t", ClaimedBy: "w1",
                ClaimedAt: stale, ClaimTTL: 3600, LastHeartbeat: stale},
        },
    })
    v := m.View()
    if !strings.Contains(v, "✗") {
        t.Errorf("expired worker should show ✗ glyph, got:\n%s", v)
    }
}
```

> Check the `materialize.State` struct to verify field names (`Claims`, `Claim` struct fields). Adjust test code to match actual struct definitions.

- [ ] **Step 2: Run tests — expect compile error**

- [ ] **Step 3: Create `internal/tui/workers/model.go`**

Key design:
- Struct: `state`, `cursor int`, `width int`, `height int`
- `View()`: upper 60% shows worker list table (columns: workerID, task title, TTL remaining, heartbeat age, health glyph); lower 40% shows selected worker detail
- Health glyph: `●` OK green if heartbeat < TTL/2 ago; `⚠` Advisory yellow if overdue by <50%; `✗` Critical red if expired
- `HelpBar()`: `"j/k move  x force-expire  r refresh  q quit"`

Worker data is derived from `Issue` fields — there is no separate `Claims` map on `State`. Issues with a non-empty `ClaimedBy` are active workers.

```go
package workers

import (
    "fmt"
    "strings"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui"
)

type Model struct {
    state   *materialize.State
    cursor  int
    width   int
    height  int
    workers []workerRow
}

// workerRow mirrors the claim fields on materialize.Issue.
type workerRow struct {
    workerID      string // Issue.ClaimedBy
    issueID       string // Issue.ID
    issueTitle    string // Issue.Title
    claimedAt     int64  // Issue.ClaimedAt
    lastHeartbeat int64  // Issue.LastHeartbeat
    claimTTL      int    // Issue.ClaimTTL (seconds)
}

func New() *Model { return &Model{} }

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) SetSize(w, h int) { m.width = w; m.height = h }

func (m *Model) SetState(state *materialize.State) {
    m.state = state
    m.workers = buildWorkerRows(state)
    if m.cursor >= len(m.workers) && len(m.workers) > 0 {
        m.cursor = len(m.workers) - 1
    }
}

func (m *Model) HelpBar() string {
    return tui.Muted.Render("j/k move  x force-expire  r refresh  enter →tree  q quit")
}

func (m *Model) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
    if km, ok := msg.(tea.KeyMsg); ok {
        switch km.String() {
        case "j", "down":
            if m.cursor < len(m.workers)-1 {
                m.cursor++
            }
        case "k", "up":
            if m.cursor > 0 {
                m.cursor--
            }
        }
    }
    return m, nil
}

func (m *Model) View() string {
    if len(m.workers) == 0 {
        return tui.Muted.Render("No active workers.")
    }
    listHeight := (m.height * 6) / 10
    var lines []string
    for i, w := range m.workers {
        health := healthGlyph(w)
        ttl := time.Duration(w.claimTTL) * time.Second
        ttlStr := formatDuration(ttl - time.Since(time.Unix(w.claimedAt, 0)))
        hbStr := formatDuration(time.Since(time.Unix(w.lastHeartbeat, 0)))
        title := w.issueTitle
        if len(title) > 30 {
            title = title[:29] + "…"
        }
        row := fmt.Sprintf("%s  %-16s  %-30s  TTL:%-8s  HB:%-8s",
            health, w.workerID, title, ttlStr, hbStr)
        if i == m.cursor {
            row = tui.Info.Render(row)
        }
        lines = append(lines, row)
        if i >= listHeight {
            break
        }
    }
    return strings.Join(lines, "\n") + "\n\n" + m.selectedDetail()
}

func (m *Model) selectedDetail() string {
    if m.cursor >= len(m.workers) {
        return ""
    }
    w := m.workers[m.cursor]
    var b strings.Builder
    fmt.Fprintf(&b, "Worker: %s\n", tui.Info.Render(w.workerID))
    fmt.Fprintf(&b, "Task:   %s  %s\n", w.issueID, w.issueTitle)
    fmt.Fprintf(&b, "Claimed: %s  Heartbeat: %s ago\n",
        time.Unix(w.claimedAt, 0).Format("15:04:05"),
        formatDuration(time.Since(time.Unix(w.lastHeartbeat, 0))))
    return b.String()
}

// buildWorkerRows collects issues that are currently claimed (ClaimedBy != "").
func buildWorkerRows(state *materialize.State) []workerRow {
    if state == nil {
        return nil
    }
    var rows []workerRow
    for _, issue := range state.Issues {
        if issue.ClaimedBy == "" {
            continue
        }
        rows = append(rows, workerRow{
            workerID:      issue.ClaimedBy,
            issueID:       issue.ID,
            issueTitle:    issue.Title,
            claimedAt:     issue.ClaimedAt,
            lastHeartbeat: issue.LastHeartbeat,
            claimTTL:      issue.ClaimTTL,
        })
    }
    return rows
}

func healthGlyph(w workerRow) string {
    age := time.Since(time.Unix(w.lastHeartbeat, 0))
    ttl := time.Duration(w.claimTTL) * time.Second
    switch {
    case age > ttl:
        return tui.Critical.Render("✗")
    case age > ttl/2:
        return tui.Advisory.Render("⚠")
    default:
        return tui.OK.Render("●")
    }
}

func formatDuration(d time.Duration) string {
    if d < 0 {
        return "0s"
    }
    if d < time.Minute {
        return fmt.Sprintf("%ds", int(d.Seconds()))
    }
    if d < time.Hour {
        return fmt.Sprintf("%dm", int(d.Minutes()))
    }
    return fmt.Sprintf("%dh", int(d.Hours()))
}
```

- [ ] **Step 4: Run tests — expect pass**

```
go test ./internal/tui/workers/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui/workers/
git commit -m "feat: add workers/ screen — worker list with health glyphs and detail pane"
```

---

### Task 18: `internal/tui/tuivalidate/` Screen

Package name: `tuivalidate` (avoids collision with `internal/validate`).

**Files:**
- Create: `internal/tui/tuivalidate/model.go`
- Create: `internal/tui/tuivalidate/model_test.go`

- [ ] **Step 1: Write failing tests**:

```go
package tuivalidate_test

import (
    "strings"
    "testing"

    "github.com/scullxbones/trellis/internal/tui/tuivalidate"
    "github.com/scullxbones/trellis/internal/validate"
)

func TestErrorsSectionShownFirst(t *testing.T) {
    m := tuivalidate.New()
    m.SetSize(120, 40)
    m.SetResult(&validate.Result{
        OK:       false,
        Errors:   []string{"E4: Missing acceptance"},
        Warnings: []string{"W2: No test criteria"},
    })
    v := m.View()
    errPos := strings.Index(v, "E4:")
    warnPos := strings.Index(v, "W2:")
    if errPos < 0 || warnPos < 0 {
        t.Errorf("expected both error and warning in view, got:\n%s", v)
    }
    if errPos > warnPos {
        t.Errorf("errors should appear before warnings, got:\n%s", v)
    }
}

func TestCleanShowsOKMessage(t *testing.T) {
    m := tuivalidate.New()
    m.SetSize(120, 40)
    m.SetResult(&validate.Result{OK: true})
    v := m.View()
    if !strings.Contains(v, "✓") && !strings.Contains(v, "clean") && !strings.Contains(v, "No issues") {
        t.Errorf("clean result should show OK indicator, got:\n%s", v)
    }
}

func TestNilResultShowsPlaceholder(t *testing.T) {
    m := tuivalidate.New()
    m.SetSize(120, 40)
    m.SetState(nil) // SetState with nil state shouldn't panic
    v := m.View()
    _ = v // just verify no panic
}
```

- [ ] **Step 2: Create `internal/tui/tuivalidate/model.go`**

```go
package tuivalidate

import (
    "fmt"
    "strings"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui"
    "github.com/scullxbones/trellis/internal/validate"
)

type Model struct {
    result *validate.Result
    cursor int
    filter filterMode
    width  int
    height int
}

type filterMode int

const (
    filterAll    filterMode = iota
    filterErrors            // errors only
    filterWarn              // warnings only
)

func New() *Model { return &Model{} }

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) SetSize(w, h int) { m.width = w; m.height = h }

func (m *Model) SetState(_ *materialize.State) {}

// SetResult updates the validate result to display.
func (m *Model) SetResult(r *validate.Result) { m.result = r }

func (m *Model) HelpBar() string {
    return tui.Muted.Render("j/k move  r re-run  f filter  enter →tree  q quit")
}

func (m *Model) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
    if km, ok := msg.(tea.KeyMsg); ok {
        switch km.String() {
        case "j", "down":
            m.cursor++
        case "k", "up":
            if m.cursor > 0 {
                m.cursor--
            }
        case "f":
            m.filter = (m.filter + 1) % 3
        }
    }
    return m, nil
}

func (m *Model) View() string {
    if m.result == nil {
        return tui.Muted.Render("Run 'r' to validate.")
    }
    if m.result.OK && len(m.result.Warnings) == 0 {
        return tui.OK.Render("✓ No issues found.")
    }

    var lines []string

    if m.filter != filterWarn && len(m.result.Errors) > 0 {
        lines = append(lines, tui.Critical.Render("── ERRORS ──"))
        for _, e := range m.result.Errors {
            lines = append(lines, fmt.Sprintf("  %s", e))
        }
    }

    if m.filter != filterErrors && len(m.result.Warnings) > 0 {
        if len(lines) > 0 {
            lines = append(lines, "")
        }
        lines = append(lines, tui.Advisory.Render("── WARNINGS ──"))
        for _, w := range m.result.Warnings {
            lines = append(lines, fmt.Sprintf("  %s", w))
        }
    }

    return strings.Join(lines, "\n")
}
```

- [ ] **Step 3: Run tests — expect pass**

```
go test ./internal/tui/tuivalidate/... -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/tuivalidate/
git commit -m "feat: add tuivalidate/ screen — errors/warnings list with filter toggle"
```

---

### Task 19: `internal/tui/sources/` Screen

**Files:**
- Create: `internal/tui/sources/model.go`
- Create: `internal/tui/sources/model_test.go`

- [ ] **Step 1: Write failing tests**:

```go
package sources_test

import (
    "strings"
    "testing"

    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui/sources"
)

// Sources are derived from Issue.SourceLinks. materialize.SourceLink has fields:
// SourceEntryID string, SourceURL string, Title string.

func TestViewShowsSourceIDs(t *testing.T) {
    m := sources.New()
    m.SetSize(120, 40)
    m.SetState(&materialize.State{
        Issues: map[string]*materialize.Issue{
            "T1": {ID: "T1", SourceLinks: []materialize.SourceLink{
                {SourceEntryID: "SRC-1", Title: "RFC 9110", SourceURL: "https://example.com"},
            }},
        },
    })
    v := m.View()
    if !strings.Contains(v, "SRC-1") {
        t.Errorf("View missing source entry ID, got:\n%s", v)
    }
}

func TestMultipleIssuesReferenceCount(t *testing.T) {
    m := sources.New()
    m.SetSize(120, 40)
    m.SetState(&materialize.State{
        Issues: map[string]*materialize.Issue{
            "T1": {ID: "T1", SourceLinks: []materialize.SourceLink{{SourceEntryID: "SRC-1"}}},
            "T2": {ID: "T2", SourceLinks: []materialize.SourceLink{{SourceEntryID: "SRC-1"}}},
        },
    })
    v := m.View()
    if !strings.Contains(v, "2 refs") {
        t.Errorf("should show 2 refs for SRC-1, got:\n%s", v)
    }
}
```

- [ ] **Step 3: Create `internal/tui/sources/model.go`**

Two-pane layout (left ~50% source list, right ~50% selected source detail):

```go
package sources

import (
    "fmt"
    "strings"

    "sort"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/tui"
)

type Model struct {
    state   *materialize.State
    sources []sourceRow
    cursor  int
    focus   int // 0 = source list, 1 = node list
    width   int
    height  int
}

// sourceRow aggregates all SourceLinks across Issues that share a SourceEntryID.
// There is no separate Sources map on State — links live on Issue.SourceLinks.
type sourceRow struct {
    entryID      string   // materialize.SourceLink.SourceEntryID
    title        string   // materialize.SourceLink.Title
    url          string   // materialize.SourceLink.SourceURL
    referencedBy []string // issue IDs that reference this source
}

func New() *Model { return &Model{} }

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) SetSize(w, h int) { m.width = w; m.height = h }

func (m *Model) SetState(state *materialize.State) {
    m.state = state
    m.sources = buildSourceRows(state)
}

func (m *Model) HelpBar() string {
    return tui.Muted.Render("j/k move  tab focus  s sync  S sync-all  r stale-review  q quit")
}

func (m *Model) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
    if km, ok := msg.(tea.KeyMsg); ok {
        switch km.String() {
        case "j", "down":
            if m.cursor < len(m.sources)-1 {
                m.cursor++
            }
        case "k", "up":
            if m.cursor > 0 {
                m.cursor--
            }
        case "tab":
            m.focus = (m.focus + 1) % 2
        }
    }
    return m, nil
}

func (m *Model) View() string {
    if len(m.sources) == 0 {
        return tui.Muted.Render("No source links found.")
    }
    half := m.width / 2
    var leftLines []string
    for i, src := range m.sources {
        refCount := tui.Muted.Render(fmt.Sprintf("(%d refs)", len(src.referencedBy)))
        row := fmt.Sprintf("%-24s  %s", src.entryID, refCount)
        if i == m.cursor {
            row = tui.Info.Render(row)
        }
        leftLines = append(leftLines, row)
    }
    left := strings.Join(leftLines, "\n")
    right := m.selectedDetail()

    // Side-by-side render: each left line + right content.
    leftSplit := strings.Split(left, "\n")
    rightSplit := strings.Split(right, "\n")
    maxRows := len(leftSplit)
    if len(rightSplit) > maxRows {
        maxRows = len(rightSplit)
    }
    var rows []string
    for i := 0; i < maxRows; i++ {
        l, r := "", ""
        if i < len(leftSplit) {
            l = leftSplit[i]
        }
        if i < len(rightSplit) {
            r = rightSplit[i]
        }
        pad := half - len(l)
        if pad < 1 {
            pad = 1
        }
        rows = append(rows, l+strings.Repeat(" ", pad)+" │ "+r)
    }
    return strings.Join(rows, "\n")
}

func (m *Model) selectedDetail() string {
    if m.cursor >= len(m.sources) {
        return ""
    }
    src := m.sources[m.cursor]
    var b strings.Builder
    fmt.Fprintf(&b, "Entry:  %s\n", tui.Info.Render(src.entryID))
    if src.title != "" {
        fmt.Fprintf(&b, "Title:  %s\n", src.title)
    }
    if src.url != "" {
        fmt.Fprintf(&b, "URL:    %s\n", src.url)
    }
    fmt.Fprintf(&b, "Used by: %s\n", strings.Join(src.referencedBy, ", "))
    return b.String()
}

// buildSourceRows collects unique SourceEntryIDs from all Issue.SourceLinks.
func buildSourceRows(state *materialize.State) []sourceRow {
    if state == nil {
        return nil
    }
    byID := make(map[string]*sourceRow)
    for issueID, issue := range state.Issues {
        for _, link := range issue.SourceLinks {
            if _, ok := byID[link.SourceEntryID]; !ok {
                byID[link.SourceEntryID] = &sourceRow{
                    entryID: link.SourceEntryID,
                    title:   link.Title,
                    url:     link.SourceURL,
                }
            }
            byID[link.SourceEntryID].referencedBy = append(byID[link.SourceEntryID].referencedBy, issueID)
        }
    }
    rows := make([]sourceRow, 0, len(byID))
    for _, r := range byID {
        rows = append(rows, *r)
    }
    sort.Slice(rows, func(i, j int) bool { return rows[i].entryID < rows[j].entryID })
    return rows
}
```

> `buildSourceRows` is a stub. Read `internal/materialize/` to find how sources are represented (likely in `Issue.SourceLinks` or a separate `Sources` map) and implement accordingly.

- [ ] **Step 4: Run tests — expect pass**

```
go test ./internal/tui/sources/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui/sources/
git commit -m "feat: add sources/ screen — two-pane source freshness view"
```

---

### Task 20: Wire `cmd/trellis/tui.go` + Delete `internal/tui/board/`

**Files:**
- Rewrite: `cmd/trellis/tui.go`
- Delete: `internal/tui/board/` (entire directory)
- Modify: `cmd/trellis/tui_test.go`

- [ ] **Step 1: Update `cmd/trellis/tui_test.go`**

Find any tests that reference `board.` or import `internal/tui/board` and remove/update them to test the new non-interactive code path.

- [ ] **Step 2: Rewrite `cmd/trellis/tui.go`**

```go
package main

import (
    "fmt"
    "path/filepath"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/scullxbones/trellis/internal/tui"
    "github.com/scullxbones/trellis/internal/tui/app"
    "github.com/scullxbones/trellis/internal/tui/dagtree"
    "github.com/scullxbones/trellis/internal/tui/sources"
    "github.com/scullxbones/trellis/internal/tui/tuivalidate"
    "github.com/scullxbones/trellis/internal/tui/workers"
    "github.com/scullxbones/trellis/internal/worker"
    "github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
    return &cobra.Command{
        Use:   "tui",
        Short: "Interactive DAG tree viewer with live updates",
        RunE: func(cmd *cobra.Command, args []string) error {
            issuesDir := appCtx.IssuesDir
            tuiStateDir := filepath.Join(issuesDir, "state", ".tui")

            workerID, _ := worker.GetWorkerID(appCtx.RepoPath)
            if workerID == "" {
                workerID = "default"
            }

            if !tui.IsInteractive() {
                _, _ = fmt.Fprintf(cmd.OutOrStdout(), "tui: non-interactive mode\n")
                return nil
            }

            m := app.New(issuesDir, tuiStateDir, workerID).
                WithScreens(
                    dagtree.New(),
                    workers.New(),
                    tuivalidate.New(),
                    sources.New(),
                )

            p := tea.NewProgram(m, tea.WithAltScreen())
            _, err := p.Run()
            return err
        },
    }
}
```

- [ ] **Step 3: Delete `internal/tui/board/`**

```bash
rm -rf internal/tui/board/
```

- [ ] **Step 4: Verify build**

```
go build ./...
```
Expected: no errors (no remaining imports of `internal/tui/board`)

- [ ] **Step 5: Run all tests**

```
go test ./... -v 2>&1 | grep -E "^(ok|FAIL|---)" | head -40
```
Expected: all packages pass

- [ ] **Step 6: Run `make check`**

```
make check
```
Expected: lint + test + coverage ≥ 80% + mutate all green

- [ ] **Step 7: Commit**

```bash
git add cmd/trellis/tui.go cmd/trellis/tui_test.go
git rm -r internal/tui/board/
git commit -m "feat: replace kanban board with DAG tree TUI

New trls tui launches app/ root model with dagtree/workers/tuivalidate/sources
screens. TUI materializes into state/.tui/ for isolation from agent state dirs.
Removes internal/tui/board/ package."
```

---

## Final Verification

- [ ] **Run full `make check`**

```
make check
```
Expected: lint + test + coverage ≥ 80% + mutation testing all green

- [ ] **Smoke test from a fresh clone / `trls init` + `trls worker-init` + `trls tui`** to verify the live TUI launches, shows the DAG tree, and navigation keys work.
