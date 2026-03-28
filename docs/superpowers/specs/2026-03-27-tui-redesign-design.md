# TUI Redesign + Multi-Agent Worktree Remediation

**Date:** 2026-03-27
**Status:** Approved for implementation planning
**Scope:** Replaces `trls tui` kanban board with a DAG tree viewer; adds Workers, Validate, and Sources screens; fixes semantic color palette; fully remediates multi-agent worktree conflicts on a single developer machine.

---

## 1. Problem Statement

### 1.1 TUI UX

The existing `trls tui` command renders a three-column kanban board (`internal/tui/board/`). User-reported problems:

- Screen space underutilised — column widths are `terminal_width / 3`, leaving titles heavily truncated
- Detail panel shows only five fields (ID, Title, Status, Priority, Type); most `Issue` data is inaccessible
- No keybinding help bar
- No screen for monitoring active workers or their heartbeat health
- No screen for validate output or source freshness
- The kanban flattens the DAG into status buckets, losing all relationship information (blocked-by edges, epic→story→task hierarchy)

### 1.2 Semantic Color Palette Drift

`internal/tui/colors.go` disagrees with `architecture.md §15` on four roles:

| Role | colors.go (wrong) | Architecture spec |
|---|---|---|
| Warning | `#FFCC00` (yellow) | orange(214) |
| Advisory | `#FF8C00` (orange) | yellow(226) |
| Info | `#00CCFF` (cyan) | blue(39) |
| ActionRequired | `#CC44FF` (magenta) | bold white on red |

Warning and Advisory are swapped, causing "needs attention" items to render yellow and "informational" items orange — the reverse of intent. This affects every existing TUI command (`trls ready`, `trls stale-review`, `trls dag-summary`, `trls dagsummary`).

### 1.3 Multi-Agent Worktree Conflicts

When multiple agents (or a human + agents) run in separate git worktrees on the same machine, three problems occur:

**Problem A — `_trellis` branch checkout conflict.** Git disallows the same branch checked out in more than one worktree simultaneously. `trls init` creates the ops worktree using a path relative to the current directory (`filepath.Join(repoPath, ".trellis")`). If run from a second git worktree (e.g. `../repo-feat-a/`), it calls `git worktree add .trellis/ origin/_trellis` again, which git rejects because `_trellis` is already checked out elsewhere. Additionally, the relative path stored in `trellis.ops-worktree-path` config resolves incorrectly when read from a different worktree's working directory.

**Problem B — `state/checkpoint.json` collision.** All agents materialise into the same flat `state/` directory (`filepath.Join(issuesDir, "state")` in `pipeline.go:36`), sharing `checkpoint.json`, `index.json`, and `state/issues/*.json`. Concurrent materializations corrupt the checkpoint, causing missed or double-applied ops on subsequent runs.

**Problem C — TUI/agent state contention.** The TUI re-materialises on every refresh. If it shares a state directory with a running agent, their checkpoints interfere.

### 1.4 Materialized State Committed to Git

Materialized state files were being committed in single-branch mode due to a missing `.gitignore` guard. **Already fixed** in commit `7699161`: `.issues/.gitignore` excludes `state/` and `trls init` writes this file on setup.

---

## 2. Design

### 2.1 Package Architecture

Delete `internal/tui/board/`. Add:

```
internal/tui/
  app/          # root Bubble Tea model — nav bar, screen routing, terminal size, state refresh
  dagtree/      # screen 1 — indented DAG tree with collapse/expand and detail overlay
  workers/      # screen 2 — active claims, heartbeat health, force-expire
  validate/     # screen 3 — errors/warnings with explainers and fix hints
  sources/      # screen 4 — source freshness, stale citations, sync actions
  detail/       # shared overlay — node detail panel, used by dagtree and workers
  colors/       # corrected semantic palette (replaces root-level colors.go values)
```

Unchanged: `internal/tui/ready/`, `internal/tui/dagsum/`, `internal/tui/dagsummary/`, `internal/tui/stalereview/`.

**Root model (`app/`):** The only `tea.Program` in the application. Implements `tea.Model` directly. Holds four sub-screens and a `currentScreen` enum. Owns terminal dimensions and propagates `tea.WindowSizeMsg`. Owns the current `*materialize.State` and sends it to all screens on refresh via `SetState()`. Renders nav bar (1 line, top) + active screen (remaining height) + help bar (1 line, bottom). Routes `1`–`4` and `Tab`/`Shift+Tab` for screen switching; all other keys delegate to the active sub-screen.

**Sub-screen interface:**
```go
type Screen interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Screen, tea.Cmd)
    View() string
    SetSize(width, height int)
    SetState(state *materialize.State)
}
```

`Screen` intentionally does not satisfy `tea.Model` (whose `Update` returns `tea.Model`, not `Screen`). Sub-screens are not registered with any `tea.Program`. The root model owns delegation explicitly:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    updated, cmd := m.screens[m.current].Update(msg)
    m.screens[m.current] = updated
    return m, cmd
}
```

`Init()` on sub-screens is called once during root model construction, with their commands batched via `tea.Batch`. Sub-screens' `Init()` is not called again on screen switch — each sub-screen is long-lived for the duration of the program.

The detail overlay is a separate `detail.Model` struct that any screen instantiates. It renders as a centred box over the screen's dimmed content when `showDetail` is true.

`cmd/trellis/tui.go` is updated to launch `app.New(issuesDir, stateDir, workerID)` instead of the board model.

### 2.2 Nav Bar

One line at the top, always visible:

```
 [1] Tree  [2] Workers  [3] Validate ⚠2  [4] Sources ⚠1    trls tui · E4,E5 · ↺ live
```

- Active screen tab: Info blue background
- Badge counts on tabs with pending action items: `⚠N` in Critical red for errors (Validate), Advisory yellow for stale (Sources). No badge when clean.
- Right side: repo context summary + live-update indicator (`↺ live` Info blue when fsnotify active; `↺ poll` Advisory yellow on fallback; pulses briefly on each refresh)
- `1`–`4`: direct jump to screen; `Tab`: cycle forward; `Shift+Tab`: cycle back
- `q` / `Ctrl+C`: quit from anywhere

### 2.3 Help Bar + Global Help Overlay

One line at the bottom showing context-sensitive shortcuts for the active screen. Pressing `?` opens a full keybinding reference overlay (centred box, same visual treatment as the detail panel). `Esc` or `?` dismisses.

### 2.4 Screen 1 — DAG Tree

**Layout:** Nav bar (top 1 line) → tree content (fills remaining height) → help bar (bottom 1 line).

**Tree rendering:** Indented tree using box-drawing characters. Each line:

```
PREFIX  GLYPH  ID    TITLE (fills terminal width, truncated with … if needed)    STATUS
```

- `PREFIX`: `│   ├──` / `└──` depth indentation, Muted gray
- `GLYPH + color`: shape and color encode status per the semantic palette:

| Status | Glyph | Color |
|---|---|---|
| `merged` | `✓` | OK green |
| `in-progress` | `▶` | Warning orange |
| `claimed` | `▶` | Warning orange (same as in-progress; claim is the pre-heartbeat in-progress state) |
| `done` | `✓` | Advisory yellow (done but not yet merged — awaiting PR review) |
| `open` | `○` | Info blue |
| `blocked` | `✗` | Critical red |
| `cancelled` | `—` | Muted gray |

- `ID`: Info blue
- `TITLE`: fills available width; selected row highlighted full-width in Info blue; STATUS label omitted for `merged` and `cancelled` nodes to reduce noise on completed work
- My-claim annotation: `[you]` MyClaim style (OK bold green) after title
- Other worker's claim: worker ID short-prefix in TheirClaim style (Warning bold orange)

**Collapse/expand:**
- Default: all epics and stories expanded, tasks visible
- `l` / `→`: expand; `h` / `←`: collapse to parent level
- `H`: collapse all to epic level; `L`: expand all
- Collapsed node shows count badge: `▸ E4  TUI + Governance  (5 stories, 10 open)`

**Filter mode (`/`):** Inline filter bar replaces help bar. Filters by status keyword or free text against title/ID. Non-matching nodes hidden; ancestors remain visible to preserve tree structure. `Esc` clears.

**Validation annotations:** Nodes with errors show an `ACTION` badge (ActionRequired: bold white on red). Nodes with warnings show `⚠` Advisory yellow inline.

**Detail overlay (`Enter`):** Opens the shared `detail/` component as a centred overlay; tree dims behind it. `Esc` dismisses.

**Human-oriented action bar** (no claim shortcut):
```
j/k move  h/l expand/collapse  enter detail  / filter  v validate subtree  p parent  c copy ID  1-4 screens  ? help  q quit
```

### 2.5 Live Updates + State Isolation

**Filesystem watch:** Root model starts an `fsnotify.Watcher` on `.issues/ops/` at init. Any `*.log` file change triggers a 200ms debounced re-materialisation.

> **Dependency note:** `fsnotify` (`github.com/fsnotify/fsnotify`) is not currently in `go.mod` and must be added as a direct dependency. The architecture's "zero external deps" constraint (§1 Key Constraints) applies to core CLI operations only — claim, transition, render-context, materialize — which must never make network calls beyond git. The TUI is a human-interactive monitoring tool, not a core operation, and is explicitly exempt. `fsnotify` is a pure Go library with no runtime network calls; it uses OS-native inotify/kqueue/FSEvents.

**Fallback poll:** If `fsnotify` fails to start (rare in some container or remote environments), the root model falls back to a 5-second `tea.Tick`. Nav bar shows `↺ poll` Advisory yellow.

**TUI state path:** TUI always materialises into `state/.tui/` via a dedicated `stateDir` — never into an agent's state directory (see §3.2 for the `stateDir` parameter design).

**Remote agent updates:** A background `git fetch` runs every 30 seconds when a remote is configured. The fetch runs with a 10-second timeout; if it times out or errors, the failure is logged to the nav bar status briefly and the next cycle continues. If the `_trellis` ref changed post-fetch, re-materialise. Gives ~30s latency for remote agents vs near-instant for local ones.

### 2.6 Screen 2 — Workers

**Layout:** Worker list (upper ~60%) + selected worker detail (lower ~40%).

**Worker list columns:** worker ID, current task title (truncated), TTL remaining, heartbeat age, health glyph (`●` OK green / `⚠` Advisory yellow / `✗` Critical red for expired).

**Worker detail:** recent ops (last 3: claim/heartbeat/note with timestamps), completed tasks this session (merged), branch, stale heartbeat warning with Advisory yellow if overdue.

**Actions:**
- `x`: force-expire a stale claim (only available when heartbeat is overdue)
- `Enter`: switch to Tree screen with the worker's current task selected
- `r`: refresh

### 2.7 Screen 3 — Validate

**Layout:** Flat list, errors first (Critical red section header), then warnings (Advisory yellow section header).

**Per-item format (3–4 lines):**
```
E4  Missing required field: acceptance                              [enter →]
    node: E4-S5-T2  "Log ops for per-item acknowledgment"
    ⓘ Without acceptance criteria there's no machine-checkable definition of done.
    fix: trls amend E4-S5-T2 --acceptance "..."
```

Implication sentence is plain-language impact — what goes wrong if this is ignored, not just what the rule says.

**Actions:**
- `Enter`: switch to Tree screen with offending node selected
- `r`: re-run validation and refresh
- `f`: cycle filter — all / errors only / warnings only

### 2.8 Screen 4 — Sources

**Layout:** Source list (left ~50%) + selected source detail (right ~50%).

**Source list:** source ID, provider, title, cache date, status (`fresh` OK green / `stale ⚠` Advisory yellow).

**Source detail:** SHA before/after, affected nodes with citation section and review status per node.

**Actions:**
- `s`: sync selected source
- `S`: sync all sources
- `r`: launch `trls stale-review` for selected stale source
- `Enter` on an affected node: switch to Tree with that node selected
- `Tab`: move focus between source list and node list

### 2.9 Shared Detail Overlay (`detail/`)

Centred box, `width = min(terminal_width - 4, 90)`. Uses `viewport.Model` for scrollable content. `j/k` scrolls. `Esc` dismisses. `c` copies node ID to the system clipboard via `github.com/atotto/clipboard` (already an indirect dependency; promoted to direct). In headless/SSH environments where clipboard is unavailable, `c` prints the ID to the terminal's alternate screen instead of silently failing.

Content sections shown conditionally by node type × status:

| Node + Status | Sections |
|---|---|
| Task, open | Header (ID/type/status), parent link, scope, priority+complexity, acceptance criteria, source citation, context files |
| Task, claimed / in-progress (my claim) | Above + TTL countdown, heartbeat age, branch, notes, decisions. `H` to extend TTL |
| Task, claimed / in-progress (their claim) | Above but read-only. `x` force-expire if heartbeat overdue |
| Task, done | Header, branch, outcome text. Status shown as Advisory yellow to indicate awaiting merge |
| Task, merged | Header, branch, PR, outcome text, source citation confirmation |
| Story | Header, progress bar, task list with status, scope, blocked-by/blocks |
| Epic | Header, story rollup, dual progress bars (stories + tasks), active workers on this epic, blockers summary |

### 2.10 Semantic Color Palette Correction

Fix `internal/tui/colors.go` to match `architecture.md §15` using xterm-256 color numbers:

```go
Warning        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
Advisory       = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
Info           = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
ActionRequired = lipgloss.NewStyle().Bold(true).
                   Foreground(lipgloss.Color("15")).
                   Background(lipgloss.Color("196"))
```

Add two new styles:
```go
MyClaim    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
TheirClaim = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
```

`Critical`, `OK`, and `Muted` are already correct. `colors_test.go` must be updated to assert the corrected values. All existing TUI command test packages that assert on rendered ANSI output (`ready/`, `dagsum/`, `dagsummary/`, `stalereview/`) must be audited for color-value assertions and updated — the change from hex strings to xterm color numbers changes the ANSI escape sequences produced by `lipgloss`.

---

## 3. Multi-Agent Worktree Remediation

### 3.1 Problem A — `_trellis` Branch Checkout Conflict

**Root cause:** `cmd/trellis/init.go` creates the ops worktree with a relative path:
```go
worktreePath := filepath.Join(repoPath, ".trellis")  // ".trellis" when repoPath is "."
```
This relative path is stored in `trellis.ops-worktree-path` git config. When read from a second git worktree whose working directory is different (e.g., `../repo-feat-a/`), the relative path resolves to the wrong location. Additionally, running `trls init` from a second worktree attempts `git worktree add .trellis/ origin/_trellis` again, which git rejects because `_trellis` is already checked out.

**Fix — `cmd/trellis/init.go`:**
1. Resolve `worktreePath` to an absolute path before use — with proper error handling:
   ```go
   worktreePath, err := filepath.Abs(filepath.Join(repoPath, ".trellis"))
   if err != nil {
       return fmt.Errorf("resolve worktree path: %w", err)
   }
   ```
2. Before calling `git worktree add`, check if `trellis.ops-worktree-path` is already set in git config and the path exists. If so, skip worktree creation (idempotent for the second worktree case).
3. Store the absolute path in git config: `trellis.ops-worktree-path = /absolute/path/.trellis`

**Fix — `cmd/trellis/helpers.go` (context resolution):** When resolving `issuesDir` in a feature worktree, read `trellis.ops-worktree-path` from git config to find the shared ops worktree. The path is now always absolute and resolves correctly regardless of the calling worktree's working directory.

`cmd/trellis/worker_init.go` does not perform worktree creation and requires no changes for this issue. It sets/reads worker identity via `git config --local` only.

### 3.2 Problem B — Agent-vs-Agent Checkpoint Collision

**Root cause:** `internal/materialize/pipeline.go` derives `stateDir` from `issuesDir` unconditionally:
```go
stateDir := filepath.Join(issuesDir, "state")   // line 36 (Materialize)
stateDir := filepath.Join(issuesDir, "state")   // line 137 (MaterializeAndReturn)
```
All agents and the TUI write to the same directory, colliding on `checkpoint.json`, `index.json`, `ready.json`, `traceability.json`, and `state/issues/*.json`.

**Fix — `internal/materialize/pipeline.go`:**

Change all three public function signatures to accept an explicit `stateDir` parameter:

```go
func Materialize(issuesDir, stateDir string, singleBranch bool) (Result, error)
func MaterializeAndReturn(issuesDir, stateDir string, singleBranch bool) (*State, Result, error)
func MaterializeExcludeWorker(issuesDir, excludeWorkerID string, singleBranch bool) (*State, Result, error)
```

Remove the internal `stateDir := filepath.Join(issuesDir, "state")` derivations. The `issuesStateDir` (for `WriteIssue`) remains `filepath.Join(stateDir, "issues")` — preserving the existing nesting convention.

`MaterializeExcludeWorker` is a diagnostic-only replay that does not write state files or a checkpoint. It does **not** receive a `stateDir` parameter; its current signature is unchanged except for reordering to maintain consistency with the other functions. Its single callsite in `cmd/trellis/materialize.go` passes no `stateDir`.

**Fix — `internal/config/context.go`:** Add `StateDir string` to `config.Context`.

**Fix — `cmd/trellis/main.go` `PersistentPreRunE`:** After `appCtx = ctx`, read the worker ID and set `StateDir`:
```go
workerID, _ := worker.GetWorkerID(repoPath)  // empty string if not yet initialised
if workerID == "" {
    workerID = "default"  // CI or pre-worker-init fallback
}
appCtx.StateDir = filepath.Join(appCtx.IssuesDir, "state", workerID)
```
`worker.GetWorkerID` is already imported via `helpers.go`; adding the call here does not introduce a new import. Setting `StateDir` in `PersistentPreRunE` — after `ResolveContext` has populated `IssuesDir` — avoids any import cycle between `internal/config` and `internal/worker`.

**Callsite inventory** — all 16 call sites in `cmd/trellis/` that call `Materialize`, `MaterializeAndReturn`, or `MaterializeExcludeWorker` must be updated to pass `appCtx.StateDir`:

| File | Function called |
|---|---|
| `assign.go` | `Materialize` |
| `claim.go` | `Materialize` |
| `confirm.go` | `MaterializeAndReturn` |
| `dagsum.go` | `MaterializeAndReturn` |
| `decompose.go` (×2) | `MaterializeAndReturn` |
| `list.go` | `Materialize` |
| `materialize.go` (×2) | `MaterializeExcludeWorker`, `Materialize` |
| `merged.go` | `Materialize` |
| `ready.go` | `Materialize` |
| `render_context.go` | `Materialize` |
| `show.go` | `Materialize` |
| `stalereview.go` | `MaterializeAndReturn` |
| `status.go` | `Materialize` |
| `sync.go` (×2) | `Materialize` |
| `validate.go` | `MaterializeAndReturn` |

**`tui.go`** is replaced entirely by `app.New(issuesDir, stateDir, workerID)`. The `stateDir` argument is constructed in `tui.go` as `filepath.Join(appCtx.IssuesDir, "state", ".tui")` — not `appCtx.StateDir`, since the TUI is not a worker and needs its own isolated path.

**`MaterializeAtSHA`** (called in `render_context.go:39` and `context_history.go:57`) is exempt from the signature change. It reads from git object storage directly and writes no state files — it has no `stateDir` to accept.

**`internal/materialize/engine_test.go`** — unit tests at lines ~120 (`TestMaterializePipeline`) and ~383 (`TestMaterializeAndReturn_BasicPipeline`) call `Materialize(dir, true)` and `MaterializeAndReturn(dir, true)` directly. Both must be updated to the 3-arg form, passing a `stateDir` argument (e.g. `filepath.Join(dir, "state", "default")`).

**Secondary hardcoded state paths** — every location that constructs a `state/` path outside the pipeline's `stateDir` parameter must also be updated. Complete inventory:

`cmd/trellis/` — replace all `filepath.Join(issuesDir, "state", ...)` with `filepath.Join(appCtx.StateDir, ...)`:

| File | Line | Old path fragment | Fix |
|---|---|---|---|
| `dagsum.go` | 40 | `state/traceability.json` | `appCtx.StateDir` |
| `dagsum.go` | 197 (`writeDAGSummaryArtifact`) | `state/dag-summary.md` | add `stateDir string` param; callsite at line 137 passes `appCtx.StateDir` |
| `list.go` | 36 | `state/index.json` | `appCtx.StateDir` |
| `merged.go` | 27 | `state/index.json` | `appCtx.StateDir` |
| `render_context.go` | 89 (`loadStateFromIssuesDir`) | `state/issues/` | `appCtx.StateDir`; rename function to `loadStateFromStateDir(stateDir string)` |
| `show.go` | 31 | `state/issues/<id>.json` | `appCtx.StateDir` |
| `status.go` | 36 | `state/index.json` | `appCtx.StateDir` |

`internal/` — four packages have their own `state/` path derivations and need API changes:

- **`internal/sync/sync.go:20`** (`DetectMerges`): `issuesStateDir := filepath.Join(issuesDir, "state", "issues")` — add a `stateDir string` parameter; use `filepath.Join(stateDir, "issues")`. Update the single callsite `cmd/trellis/sync.go:38` to pass `appCtx.StateDir`. Update `internal/sync/sync_test.go:50` to pass `filepath.Join(dir, "state")` as `stateDir`.
- **`internal/validate/validate.go:66`** (`Validate`): `tracePath := filepath.Join(opts.IssuesDir, "state", "traceability.json")` — add `StateDir string` to `validate.Options`; use `filepath.Join(opts.StateDir, "traceability.json")`. Update `cmd/trellis/validate.go` callsite to set `StateDir: appCtx.StateDir`.
- **`internal/doctor/doctor.go:86,90,116`** (`Run`, `loadAllIssues`): `Run(issuesDir, repoPath string)` calls `Materialize(issuesDir, singleBranch)` and reads from `issuesDir/state/`. Change signature to `Run(issuesDir, stateDir, repoPath string)`; pass `stateDir` to `Materialize` and use it for index/issue reads. Update `loadAllIssues(issuesDir, index)` → `loadAllIssues(stateDir, index)`. Update `cmd/trellis/doctor.go` callsite to pass `appCtx.StateDir`. Update `internal/doctor/doctor_test.go` lines 142, 165, and 191 (all call `doctor.Run(issuesDir, "")` with 2 args) to pass `stateDir` as the second argument.
- **`internal/context/assemble.go:117,139,196,212`** (`buildBlockerOutcomes`, `buildParentChain`): fallback disk reads use `filepath.Join(issuesDir, "state", "issues", id+".json")` — rename the `issuesDir` parameter to `stateDir` in `Assemble`, `buildBlockerOutcomes`, and `buildParentChain`; use `filepath.Join(stateDir, "issues", id+".json")`. Update `cmd/trellis/render_context.go:54` callsite to pass `appCtx.StateDir` instead of `issuesDir`. Update `internal/context/context_test.go` (11 calls to `Assemble` at lines ~29, 67, 105, 145, 164, 193, 219, 252, 285, 338, 367) — the second argument passed is already a stateDir-like path; rename any local variables named `issuesDir` to `stateDir` for clarity.
- **`internal/ops/tracker.go:37`** (`NewFilePushTracker`): `Path: filepath.Join(issuesDir, "state", "pending-push-count")` — rename parameter from `issuesDir` to `stateDir`; use `filepath.Join(stateDir, "pending-push-count")`. Update `cmd/trellis/helpers.go:47` callsite to pass `appCtx.StateDir` instead of `appCtx.IssuesDir`.

**Not changed:** `cmd/trellis/init.go:79–80` creates `state/` and `state/issues/` as base directories during repo initialization. This is correct — the flat `state/` directory serves as the parent for all worker subdirs, and must exist. These lines are not altered.

New state directory layout:
```
.issues/state/
  <worker-id-1>/      # agent 1's isolated materialization
    checkpoint.json
    index.json
    issues/
    ready.json
    traceability.json
  <worker-id-2>/      # agent 2's isolated materialization (no conflict)
    ...
  default/            # CI / pre-worker-init fallback
    ...
  .tui/               # TUI's isolated materialization
    ...
```

The existing `.issues/.gitignore` (`state/`) already excludes all subdirectories correctly.

### 3.3 Problem C — TUI/Agent State Contention

Resolved by §3.2 (per-agent state dirs) and §2.5 (TUI uses `state/.tui/`).

---

## 4. What Is Preserved Unchanged

- `internal/tui/ready/` — `trls ready` interactive TUI is already satisfactory
- `internal/tui/dagsum/`, `internal/tui/dagsummary/`, `internal/tui/stalereview/` — functional logic unchanged; their rendered output will change slightly due to the palette correction (§2.10)
- All non-TUI commands — unaffected by palette fix except rendered color values, which improve automatically
- `trls ready --format=json` agent path — untouched
- `.issues/.gitignore` fix from commit `7699161` — already in place

---

## 5. Testing Approach

- **Unit tests per screen model:** each `dagtree/`, `workers/`, `validate/`, `sources/` package gets table-driven tests calling `View()` with representative `*materialize.State` fixtures, asserting on key string content (status glyphs, node IDs, section headers)
- **Detail overlay:** tests for each node type × status combination covering all conditional sections
- **Root app model:** tests for screen switching, nav bar badge counts (error/stale counts), `SetState()` propagation to all screens, `Init()` batching
- **colors.go:** `colors_test.go` updated to assert corrected xterm color numbers. All existing TUI test packages (`ready/`, `dagsum/`, `dagsummary/`, `stalereview/`) audited for ANSI escape sequence assertions and updated to match the new palette values
- **Multi-agent worktree:** integration tests using `initTempRepo` + two simulated workers with separate `stateDir` values writing concurrently; assert no checkpoint corruption and correct independent state
- **`trls init` idempotency with absolute path:** test that running `trls init` a second time from a different working directory does not attempt a second `git worktree add` and stores the same absolute path
- **`stateDir` propagation:** test that `AppContext.StateDir` uses worker ID and falls back to `"default"` when no worker is configured
- Coverage target: ≥ 80% per package, consistent with repo standard (`make check`)

---

## 6. Out of Scope

- Browser-based D3 force-directed graph visualization (future effort)
- Claim action from the TUI (human manager persona does not need it)
- `trls ready` changes (already satisfactory)
- Compaction of op logs (deferred per architecture)
- Multi-repo trellis instances (v1 limitation, unrelated)
