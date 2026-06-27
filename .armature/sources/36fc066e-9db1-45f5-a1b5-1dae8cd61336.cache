# Deepening Armature Modules — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move state-loading and decision orchestration out of the `cmd/armature` handler layer and into deep modules with narrow interfaces, so the logic becomes testable through a seam instead of only end-to-end.

**Architecture:** Five phases, executed in dependency order **5 → 1 → 2 → 3 → 4** (numbered by the architecture-review candidate they implement). Phase 5 (Materialize options) collapses the materialize entry points so the new Snapshot store has one clean dependency. Phase 1 (Snapshot store) deepens `internal/snapshot` into the single way handlers read Materialized State. Phases 2–4 (Claim resolver, Render Context assembly, Harness Hook) each consume the Snapshot store and pull their decision logic behind a testable interface.

**Tech Stack:** Go 1.x, cobra (CLI), testify (assertions). Storage is append-only JSONL ops + materialized JSON state under `.armature/`.

## Global Constraints

- **TDD is mandatory** — failing test → minimum implementation → refactor. No implementation without a test. (copied from CLAUDE.md)
- **`make check` must pass before every commit/push** — lint + test + coverage (≥85%) + mutate (≥95% mcover, ≥99% efficacy) + validate-skills + build. Fix failures; never suppress.
- **No `//nolint` without justification.** Linters: `govet`, `errcheck`, `ineffassign`, `staticcheck`, `misspell`, `unconvert`, `goimports`.
- **Do not lower the coverage (85%) or mutation thresholds.**
- **Respect ADR-0004 depguard boundaries** (`.golangci.yml`): `ops`, `claim`, `traceability`, `materialize`, `sources`, `validate`, `output` are port-clean deep modules; `dag`, `issuetype` are pure (no internal imports). New code in these packages must keep their allow-lists green. If a new import is required, update the allow-list in the same commit and justify it in the commit message.
- **Materialized state is derived, not source of truth.** Ops are the source of truth; never special-case state files as authoritative.
- **Commit message footer** (every commit): end with the Co-Authored-By and Claude-Session trailers used by this repo.
- **Module vocabulary** (CONTEXT.md): use Issue, Op, Materialization, Materialized State, Snapshot, Claim, Worker, Render Context, Index, Scope — not "node", "ticket", "service".

---

## File Structure

**Phase 5 — Materialize options**
- Modify: `internal/materialize/pipeline.go` — add `Options` struct + unified `Run`; reimplement the four public `Materialize*` functions as thin wrappers.
- Test: `internal/materialize/pipeline_test.go` — table tests asserting each Option combination's behaviour.

**Phase 1 — Snapshot store**
- Modify: `internal/snapshot/snapshot.go` — add `Store` type owning ops-read → materialize → snapshot, with `Load`/`Refresh`/`Issue`/`Index`/`IssuePath`/`IndexPath`.
- Test: `internal/snapshot/snapshot_test.go` — store behaviour incl. caching and refresh-after-append.
- Modify: `cmd/armature/helpers.go` — add `newSnapshotStore(ctx)` constructor.
- Modify: ~24 handler files in `cmd/armature/` — replace inline `materialize.Materialize`/`materialize.LoadIndex`/`materialize.LoadIssue` + hand-built `filepath.Join(StateDir,…)` with store calls.

**Phase 2 — Claim resolver**
- Modify: `internal/claim/claim.go` (or new `internal/claim/resolver.go`) — add `Resolver` producing a `Plan` value from a Snapshot.
- Test: `internal/claim/resolver_test.go` — table tests for granted / stale-takeover / overlap-block / overlap-dismiss / overlap-force.
- Modify: `cmd/armature/claim.go` — handler calls `Resolver.Plan(...)`, then performs only IO (append ops, worktree).

**Phase 3 — Render Context assembly**
- Modify: `internal/context/assemble.go` — inject a `FileReader`; derive the graph internally; collapse inputs.
- Create: `internal/context/filereader.go` — `FileReader` interface + `OSFileReader` adapter.
- Test: `internal/context/assemble_test.go` — layer-selection tests through an in-memory `FileReader` fake.
- Modify: `cmd/armature/render_context.go` — pass the store snapshot + an `OSFileReader`; delete `buildGraphFromState` (moved).

**Phase 4 — Harness Hook**
- Create: `internal/harnesshook/hook.go` — `Hook` type with one `Evaluate` method owning adapter selection + policy resolution + evaluation + staleness.
- Test: `internal/harnesshook/hook_test.go` — Pass/Block/pass-through decisions across platforms and binding states.
- Modify: `cmd/armature/harness_hook.go` — handler builds `EvaluateInput`, calls `Hook.Evaluate`, maps result to exit code.

---

## Phase 5 — Candidate 05: Materialize options

**Why first:** It is small, self-contained, and gives the Snapshot store (Phase 1) one entry point to depend on instead of four. It collapses `Materialize`, `MaterializeAndReturn`, `MaterializeAndReturnQuiet`, and `MaterializeExcludeWorker` into one `Run(stateDir, allOps, byteOffsets, Options)` that owns the write/warn/exclude decisions as data; the four names survive as thin convenience wrappers, so **no existing caller changes in this phase**.

> ⚠ Touches ADR-0004 territory (`materialize` is a protected deep module). This does not contradict the ADR — the ADR governs imports/coupling, not the public interface's shape. No new imports are introduced here.

### Task 5.1: Introduce `Options` and unified `Run`

**Files:**
- Modify: `internal/materialize/pipeline.go:111-258`
- Test: `internal/materialize/pipeline_test.go`

**Interfaces:**
- Produces:
  - `type Options struct { SingleBranch bool; EmitWarnings bool; WriteStateFiles bool; ExcludeWorkerID string }`
  - `func Run(stateDir string, allOps []ops.Op, byteOffsets map[string]int64, opts Options) (*State, Result, error)`
  - Semantics: when `ExcludeWorkerID != ""`, ops from that worker are filtered out with missing-target tolerance, `WriteStateFiles` is forced `false`, and no checkpoint/state files are written (diagnostic mode). Otherwise behaves like the current `runMaterializePipeline` with `emitWarnings = opts.EmitWarnings` and state/checkpoint files written only when `opts.WriteStateFiles` is `true`.

- [ ] **Step 1: Write the failing test**

Add to `internal/materialize/pipeline_test.go`:

```go
func TestRun_WriteStateFilesControlsDiskWrites(t *testing.T) {
	dir := t.TempDir()
	opsList := []ops.Op{
		{Type: ops.OpCreate, TargetID: "T1", Timestamp: 1, WorkerID: "w1",
			Payload: ops.Payload{Title: "Task one", Type: "task"}},
	}

	// WriteStateFiles=false must NOT create index.json on disk.
	state, _, err := materialize.Run(dir, opsList, nil, materialize.Options{
		SingleBranch: true, EmitWarnings: false, WriteStateFiles: false,
	})
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Contains(t, state.Issues, "T1")
	_, statErr := os.Stat(filepath.Join(dir, "index.json"))
	require.True(t, os.IsNotExist(statErr), "index.json must not be written when WriteStateFiles=false")

	// WriteStateFiles=true must create index.json.
	_, _, err = materialize.Run(dir, opsList, nil, materialize.Options{
		SingleBranch: true, EmitWarnings: false, WriteStateFiles: true,
	})
	require.NoError(t, err)
	_, statErr = os.Stat(filepath.Join(dir, "index.json"))
	require.NoError(t, statErr, "index.json must be written when WriteStateFiles=true")
}

func TestRun_ExcludeWorkerFiltersOpsAndSkipsWrites(t *testing.T) {
	dir := t.TempDir()
	opsList := []ops.Op{
		{Type: ops.OpCreate, TargetID: "T1", Timestamp: 1, WorkerID: "w1",
			Payload: ops.Payload{Title: "Task one", Type: "task"}},
		{Type: ops.OpClaim, TargetID: "T1", Timestamp: 2, WorkerID: "w2",
			Payload: ops.Payload{TTL: 60}},
	}
	state, _, err := materialize.Run(dir, opsList, nil, materialize.Options{
		SingleBranch: true, EmitWarnings: false, ExcludeWorkerID: "w2",
	})
	require.NoError(t, err)
	require.Contains(t, state.Issues, "T1")
	require.Empty(t, state.Issues["T1"].ClaimedBy, "claim from excluded worker must be ignored")
	_, statErr := os.Stat(filepath.Join(dir, "index.json"))
	require.True(t, os.IsNotExist(statErr), "exclude-worker mode must not write state files")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/materialize/ -run 'TestRun_' -v`
Expected: FAIL — `undefined: materialize.Run` / `materialize.Options`.

- [ ] **Step 3: Write minimal implementation**

In `internal/materialize/pipeline.go`, add the `Options` type and `Run`. Refactor the body of `runMaterializePipeline` into `Run`, then make `runMaterializePipeline` and the exclude path delegate. Concretely:

```go
// Options controls a single materialization run.
type Options struct {
	SingleBranch    bool
	EmitWarnings    bool
	WriteStateFiles bool
	// ExcludeWorkerID, when non-empty, filters out ops from that worker with
	// missing-target tolerance and runs in diagnostic mode: no state files or
	// checkpoint are written regardless of WriteStateFiles.
	ExcludeWorkerID string
}

// Run is the single materialization entry point. It owns the write/warn/exclude
// decisions as data. The four Materialize* functions are thin wrappers over it.
func Run(stateDir string, allOps []ops.Op, byteOffsets map[string]int64, opts Options) (*State, Result, error) {
	if opts.ExcludeWorkerID != "" {
		return runExcludeWorker(allOps, opts.ExcludeWorkerID, opts.SingleBranch, opts.EmitWarnings)
	}
	return runFullPipeline(stateDir, allOps, opts.SingleBranch, byteOffsets, opts.EmitWarnings, opts.WriteStateFiles)
}
```

Rename the existing `runMaterializePipeline` (pipeline.go:113) to `runFullPipeline` and give it a `writeStateFiles bool` parameter. Guard the three disk-writing blocks with it:

```go
func runFullPipeline(stateDir string, allOps []ops.Op, singleBranch bool, byteOffsets map[string]int64, emitWarnings, writeStateFiles bool) (*State, Result, error) {
	issuesStateDir := filepath.Join(stateDir, "issues")
	checkpointPath := filepath.Join(stateDir, "checkpoint.json")

	if writeStateFiles {
		if err := adapters.MkdirAll(issuesStateDir, 0755); err != nil {
			return nil, Result{}, fmt.Errorf("create state dir: %w", err)
		}
	}

	cp := Checkpoint{}
	if writeStateFiles {
		loaded, err := LoadCheckpoint(checkpointPath)
		if err != nil {
			return nil, Result{}, fmt.Errorf("load checkpoint: %w", err)
		}
		cp = loaded
	}

	fullReplay := len(cp.ByteOffsets) == 0
	var state *State
	if !fullReplay {
		loadedIssues, err := LoadAllIssues(issuesStateDir)
		if err != nil {
			return nil, Result{}, fmt.Errorf("load prior state: %w", err)
		}
		state = NewState()
		state.Issues = loadedIssues
		state.SingleBranchMode = singleBranch
	} else {
		state = NewState()
		state.SingleBranchMode = singleBranch
	}

	sortOpsByTimestamp(allOps)

	unhandledOps, err := applyOps(state, allOps)
	if err != nil {
		return nil, Result{}, err
	}

	state.RunRollup()

	if writeStateFiles {
		index := state.BuildIndex()
		if err := WriteIndex(filepath.Join(stateDir, "index.json"), index); err != nil {
			return nil, Result{}, fmt.Errorf("write index: %w", err)
		}
		for _, issue := range state.Issues {
			if err := WriteIssue(issuesStateDir, *issue); err != nil {
				return nil, Result{}, fmt.Errorf("write issue %s: %w", issue.ID, err)
			}
		}
		readyPath := filepath.Join(stateDir, "ready.json")
		_ = adapters.WriteFile(readyPath, []byte("[]"), 0644) //nolint:errcheck // best-effort derived state
	}

	if emitWarnings {
		emitUnhandledOpsWarning(unhandledOps)
	}

	if writeStateFiles {
		offsets := byteOffsets
		if offsets == nil {
			offsets = make(map[string]int64)
		}
		if err := WriteCheckpoint(checkpointPath, Checkpoint{ByteOffsets: offsets}); err != nil {
			return nil, Result{}, fmt.Errorf("write checkpoint: %w", err)
		}
		cov := traceability.Compute(toTraceabilityRefs(state.Issues))
		_ = traceability.Write(filepath.Join(stateDir, "traceability.json"), cov) //nolint:errcheck // best-effort derived state
	}

	warnings := formatUnhandledOpsWarnings(unhandledOps)
	return state, Result{
		IssueCount:   len(state.Issues),
		OpsProcessed: len(allOps),
		FullReplay:   fullReplay,
		UnhandledOps: unhandledOps,
		Warnings:     warnings,
	}, nil
}
```

Move the body of the current `MaterializeExcludeWorker` (pipeline.go:223-258) into `runExcludeWorker(allOps []ops.Op, excludeWorkerID string, singleBranch, emitWarnings bool)`, gating its `emitUnhandledOpsWarning` call on `emitWarnings`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/materialize/ -run 'TestRun_' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/materialize/pipeline.go internal/materialize/pipeline_test.go
git commit -m "feat(materialize): add Options + unified Run entry point"
```

### Task 5.2: Reimplement the four public functions as wrappers

**Files:**
- Modify: `internal/materialize/pipeline.go:196-258`

**Interfaces:**
- Consumes: `Run`, `Options` (Task 5.1).
- Produces: unchanged public signatures for `Materialize`, `MaterializeAndReturn`, `MaterializeAndReturnQuiet`, `MaterializeExcludeWorker` — now one-line wrappers.

- [ ] **Step 1: Verify existing tests cover the four functions**

Run: `go test ./internal/materialize/ -run 'TestMaterialize' -v`
Expected: PASS (these are the regression guard for this refactor).

- [ ] **Step 2: Rewrite the wrappers**

```go
func Materialize(stateDir string, allOps []ops.Op, singleBranch bool, byteOffsets map[string]int64) (Result, error) {
	_, result, err := Run(stateDir, allOps, byteOffsets, Options{SingleBranch: singleBranch, EmitWarnings: true, WriteStateFiles: true})
	return result, err
}

func MaterializeAndReturn(stateDir string, allOps []ops.Op, singleBranch bool, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{SingleBranch: singleBranch, EmitWarnings: true, WriteStateFiles: true})
}

func MaterializeAndReturnQuiet(stateDir string, allOps []ops.Op, singleBranch bool, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{SingleBranch: singleBranch, EmitWarnings: false, WriteStateFiles: true})
}

func MaterializeExcludeWorker(allOps []ops.Op, excludeWorkerID string, singleBranch bool) (*State, Result, error) {
	return Run("", allOps, nil, Options{SingleBranch: singleBranch, EmitWarnings: true, ExcludeWorkerID: excludeWorkerID})
}
```

- [ ] **Step 3: Run the full materialize suite**

Run: `go test ./internal/materialize/...`
Expected: PASS (no behaviour change for existing callers).

- [ ] **Step 4: Run `make check`**

Run: `make check`
Expected: PASS — lint, coverage, mutation all green. If mutation flags the new `writeStateFiles`/`emitWarnings` guards, add a focused test in `pipeline_test.go` asserting the relevant branch (e.g. `EmitWarnings: true` surfaces an unhandled-op warning).

- [ ] **Step 5: Commit**

```bash
git add internal/materialize/pipeline.go
git commit -m "refactor(materialize): collapse four entry points onto Run"
```

---

## Phase 1 — Candidate 01: Snapshot store (top recommendation)

**Why second:** It is the root the remaining candidates grow from. Today 37 call sites across 24 handlers re-run the `ops → materialize → LoadIndex/LoadIssue` recipe and hand-build 29 `filepath.Join(StateDir, "issues"/"index.json")` paths. This phase promotes the thin `snapshot.Load` facade into a deep `Store` that owns the recipe and the disk layout, then migrates handlers onto it.

### Task 1.1: Add the `Store` type to `internal/snapshot`

**Files:**
- Modify: `internal/snapshot/snapshot.go`
- Test: `internal/snapshot/snapshot_test.go`

**Interfaces:**
- Produces:
  - `type Store struct { /* unexported: opsDir, stateDir string; singleBranch bool; cached *Snapshot */ }`
  - `func New(opsDir, stateDir string, singleBranch bool) *Store`
  - `func (s *Store) Load() (*Snapshot, error)` — reads ops, materializes (quiet), caches and returns the Snapshot; subsequent calls return the cache.
  - `func (s *Store) Refresh() (*Snapshot, error)` — re-reads ops and re-materializes (writing state files), replacing the cache. Used after appending an Op.
  - `func (s *Store) Issue(id string) (*materialize.Issue, error)` — returns the Issue from the current Snapshot (calling `Load` if not yet loaded); error if absent.
  - `func (s *Store) Index() (materialize.Index, error)` — returns the Index from the current Snapshot.
  - `func (s *Store) IssuePath(id string) string` and `func (s *Store) IndexPath() string` — the canonical on-disk paths, so handlers never build them.
- Consumes: `materialize.Run`/`MaterializeAndReturnQuiet` (Phase 5), `ops.LoadFromDirWithOffsetsValidated`, `materialize.LoadIndex`.

- [ ] **Step 1: Write the failing test**

```go
func TestStore_IssueAfterRefresh(t *testing.T) {
	repo := t.TempDir()
	opsDir := filepath.Join(repo, "ops")
	stateDir := filepath.Join(repo, "state")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))

	writeOps(t, filepath.Join(opsDir, "w1.log"), []ops.Op{
		{Type: ops.OpCreate, TargetID: "T1", Timestamp: 1, WorkerID: "w1",
			Payload: ops.Payload{Title: "Task one", Type: "task"}},
	})

	store := snapshot.New(opsDir, stateDir, true)

	issue, err := store.Issue("T1")
	require.NoError(t, err)
	require.Equal(t, "open", issue.Status)

	// Append a claim op, then Refresh must surface the new state.
	appendOpLine(t, filepath.Join(opsDir, "w1.log"), ops.Op{
		Type: ops.OpClaim, TargetID: "T1", Timestamp: 2, WorkerID: "w1", Payload: ops.Payload{TTL: 60},
	})
	_, err = store.Refresh()
	require.NoError(t, err)

	issue, err = store.Issue("T1")
	require.NoError(t, err)
	require.Equal(t, "w1", issue.ClaimedBy)
}

func TestStore_IssueNotFound(t *testing.T) {
	repo := t.TempDir()
	store := snapshot.New(filepath.Join(repo, "ops"), filepath.Join(repo, "state"), true)
	_, err := store.Issue("nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "nope")
}

func TestStore_Paths(t *testing.T) {
	store := snapshot.New("/o", "/s", true)
	require.Equal(t, filepath.Join("/s", "issues", "T1.json"), store.IssuePath("T1"))
	require.Equal(t, filepath.Join("/s", "index.json"), store.IndexPath())
}
```

(`writeOps`/`appendOpLine` helpers: write each Op as a JSON line to the log file. If equivalents already exist in `snapshot_test.go`, reuse them.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/snapshot/ -run TestStore -v`
Expected: FAIL — `undefined: snapshot.New`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/snapshot/snapshot.go` (keep the existing `Load` free function as-is so current callers stay green until migrated):

```go
// Store owns the relationship between Ops, Materialization, and the on-disk
// state layout. It is the single way handlers read Materialized State.
type Store struct {
	opsDir       string
	stateDir     string
	singleBranch bool
	cached       *Snapshot
}

func New(opsDir, stateDir string, singleBranch bool) *Store {
	return &Store{opsDir: opsDir, stateDir: stateDir, singleBranch: singleBranch}
}

// Load returns the current Snapshot, materializing once and caching the result.
func (s *Store) Load() (*Snapshot, error) {
	if s.cached != nil {
		return s.cached, nil
	}
	return s.Refresh()
}

// Refresh re-reads ops and re-materializes, replacing the cache. Call after
// appending an Op.
func (s *Store) Refresh() (*Snapshot, error) {
	snap, err := Load(s.opsDir, s.stateDir, s.singleBranch)
	if err != nil {
		return nil, err
	}
	s.cached = snap
	return snap, nil
}

func (s *Store) Issue(id string) (*materialize.Issue, error) {
	snap, err := s.Load()
	if err != nil {
		return nil, err
	}
	issue, ok := snap.Issues[id]
	if !ok {
		return nil, fmt.Errorf("issue %s not found", id)
	}
	return issue, nil
}

func (s *Store) Index() (materialize.Index, error) {
	snap, err := s.Load()
	if err != nil {
		return nil, err
	}
	return snap.Index, nil
}

func (s *Store) IssuePath(id string) string {
	return filepath.Join(s.stateDir, "issues", id+".json")
}

func (s *Store) IndexPath() string {
	return filepath.Join(s.stateDir, "index.json")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/snapshot/ -run TestStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/snapshot/snapshot.go internal/snapshot/snapshot_test.go
git commit -m "feat(snapshot): add deep Store owning load + layout"
```

### Task 1.2: Add the `newSnapshotStore` constructor in the cmd layer

**Files:**
- Modify: `cmd/armature/helpers.go`
- Test: `cmd/armature/cmd_extra_test.go` (or nearest existing helpers test)

**Interfaces:**
- Consumes: `config.Context` fields `IssuesDir`, `StateDir`, `Mode`; `snapshot.New`.
- Produces: `func newSnapshotStore(ctx *config.Context) *snapshot.Store` — wires the store from context, applying the single-branch convention.

- [ ] **Step 1: Write the failing test**

```go
func TestNewSnapshotStore_UsesContextPaths(t *testing.T) {
	ctx := &config.Context{IssuesDir: "/repo/.armature", StateDir: "/repo/state", Mode: "single-branch"}
	store := newSnapshotStore(ctx)
	require.Equal(t, filepath.Join("/repo/state", "index.json"), store.IndexPath())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/armature/ -run TestNewSnapshotStore -v`
Expected: FAIL — `undefined: newSnapshotStore`.

- [ ] **Step 3: Write minimal implementation**

Add to `cmd/armature/helpers.go`:

```go
// newSnapshotStore builds a snapshot.Store from the command context, applying
// the single-branch convention. Handlers use this instead of calling
// materialize.Materialize / LoadIndex / LoadIssue directly.
func newSnapshotStore(ctx *config.Context) *snapshot.Store {
	return snapshot.New(
		filepath.Join(ctx.IssuesDir, "ops"),
		ctx.StateDir,
		ctx.Mode == "single-branch",
	)
}
```

Add `"github.com/scullxbones/armature/internal/snapshot"` to the import block.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/armature/ -run TestNewSnapshotStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/armature/helpers.go cmd/armature/cmd_extra_test.go
git commit -m "feat(cmd): add newSnapshotStore context constructor"
```

### Task 1.3: Migrate `render-context` onto the store

**Files:**
- Modify: `cmd/armature/render_context.go:52-61`

**Interfaces:**
- Consumes: `newSnapshotStore` (Task 1.2). The `rcAt` time-travel branch (`MaterializeAtSHA`) is unchanged — the store only replaces the non-time-travel `snapshot.Load` branch.

- [ ] **Step 1: Confirm the existing render-context tests pass (regression guard)**

Run: `go test ./cmd/armature/ -run 'RenderContext|render_context|HarnessContext' -v`
Expected: PASS.

- [ ] **Step 2: Replace the inline `snapshot.Load` branch**

In the `else` branch at render_context.go:52-61, replace:

```go
				snap, snapErr := snapshot.Load(filepath.Join(issuesDir, "ops"), appCtx.StateDir, appCtx.Mode == "single-branch")
				if snapErr != nil {
					return fmt.Errorf("load snapshot: %w", snapErr)
				}
				for _, w := range snap.Warnings {
					_, _ = fmt.Fprintf(os.Stderr, "warning: %s\n", w)
				}
				state = snap.State
```

with:

```go
				snap, snapErr := newSnapshotStore(appCtx).Load()
				if snapErr != nil {
					return fmt.Errorf("load snapshot: %w", snapErr)
				}
				for _, w := range snap.Warnings {
					_, _ = fmt.Fprintf(os.Stderr, "warning: %s\n", w)
				}
				state = snap.State
```

Remove the now-unused `issuesDir := appCtx.IssuesDir` line if nothing else references it, and drop the `snapshot` import if `goimports` flags it.

- [ ] **Step 3: Run render-context tests**

Run: `go test ./cmd/armature/ -run 'RenderContext|render_context|HarnessContext' -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/armature/render_context.go
git commit -m "refactor(cmd): render-context reads via snapshot.Store"
```

### Task 1.4: Migrate `transition` onto the store

**Files:**
- Modify: `cmd/armature/transition.go:93`, `transition.go:170` (`isIssueUncited`)

**Interfaces:**
- Consumes: `newSnapshotStore`, `Store.Index`, `Store.Issue`.

- [ ] **Step 1: Confirm transition tests pass**

Run: `go test ./cmd/armature/ -run 'Transition' -v`
Expected: PASS.

- [ ] **Step 2: Replace the index load**

At transition.go:93, replace:

```go
				index, _ := materialize.LoadIndex(filepath.Join(state.ctx.StateDir, "index.json")) //nolint:errcheck // missing index treated as empty; access uses ok-check
```

with:

```go
				index, _ := newSnapshotStore(state.ctx).Index() //nolint:errcheck // missing index treated as empty; access uses ok-check
```

In `isIssueUncited` (transition.go:169-177), replace the hand-built path + `materialize.LoadIssue` with:

```go
func isIssueUncited(issueID string) bool {
	issue, err := newSnapshotStore(appCtx).Issue(issueID)
	if err != nil {
		return false // cannot load — graceful degradation, don't warn
	}
	return len(issue.SourceLinks) == 0 && len(issue.CitationAcceptances) == 0
}
```

Drop the now-unused `materialize`/`filepath` imports if `goimports` flags them.

- [ ] **Step 3: Run transition tests**

Run: `go test ./cmd/armature/ -run 'Transition' -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/armature/transition.go
git commit -m "refactor(cmd): transition reads via snapshot.Store"
```

### Task 1.5: Migrate `claim`'s read paths onto the store

**Files:**
- Modify: `cmd/armature/claim.go:289-300`, `claim.go:356`, `claim.go:395-405`

**Interfaces:**
- Consumes: `newSnapshotStore`, `Store.Refresh`, `Store.Issue`, `Store.Index`.
- Note: claim materializes twice (before and after appending the claim Op). The store models this as `Load` then `Refresh`. Ops are still read directly here because the handler needs the raw `allOps`/`offsets` for the overlap-dismissal-note check and for appending; that direct read stays until Phase 2 moves the decision out.

- [ ] **Step 1: Confirm claim tests pass**

Run: `go test ./cmd/armature/ -run 'Claim' -v`
Expected: PASS.

- [ ] **Step 2: Replace the three read sites**

Introduce a store at the top of the read section and use it. Replace claim.go:293-300:

```go
				if _, err := materialize.Materialize(ctx.StateDir, allOps, ctx.Mode == "single-branch", offsets); err != nil {
					return err
				}

				issue, err := materialize.LoadIssue(filepath.Join(ctx.StateDir, "issues", issueID+".json"))
				if err != nil {
					return fmt.Errorf("issue %s not found: %w", issueID, err)
				}
```

with:

```go
				store := newSnapshotStore(ctx)
				if _, err := store.Refresh(); err != nil {
					return err
				}

				issuePtr, err := store.Issue(issueID)
				if err != nil {
					return fmt.Errorf("issue %s not found: %w", issueID, err)
				}
				issue := *issuePtr
```

Replace the index load at claim.go:356:

```go
				index, _ := store.Index() //nolint:errcheck // missing index treated as empty
```

Replace the post-append re-materialize + reload at claim.go:395-405:

```go
				allOps, offsets, err = readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
				if err != nil {
					return fmt.Errorf("read ops: %w", err)
				}
				if _, err := store.Refresh(); err != nil {
					return err
				}
				issueAfterPtr, err := store.Issue(issueID)
				if err != nil {
					return fmt.Errorf("issue %s not found after claim: %w", issueID, err)
				}
				issueAfter := *issueAfterPtr
```

(The `allOps, offsets` re-read is retained only if still used below; if not, delete it and drop the unused vars.)

- [ ] **Step 3: Run claim tests**

Run: `go test ./cmd/armature/ -run 'Claim' -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/armature/claim.go
git commit -m "refactor(cmd): claim reads via snapshot.Store"
```

### Task 1.6: Sweep the remaining handlers + enforce with a grep test

**Files:**
- Modify: every remaining `cmd/armature/*.go` that still calls `materialize.Materialize`, `materialize.MaterializeAndReturn*`, `materialize.LoadIndex`, `materialize.LoadIssue`, or builds `filepath.Join(…StateDir…, "issues"/"index.json")`. From the review, these include: `create.go`, `assign.go`, `dagsum.go`, `confirm.go`, `list.go`, `context_history.go`, `decompose.go`, `harness_context.go`, `hook.go`, `scope_delete.go`, `merged.go`, `materialize.go`, `scope_rename.go`, `ready.go`, `reparent.go`, `show.go`, `stalereview.go`, `sync.go`, `tui.go`, `validate.go`.
- Test: `cmd/armature/architecture_test.go` (new) — a guard test that fails if handlers reload state directly.

**Migration recipe (apply per file):**
1. Where a handler reads current state, replace the `read ops → materialize.Materialize → materialize.LoadIndex/LoadIssue` sequence with `store := newSnapshotStore(ctx)` + `store.Refresh()` (if it just appended an Op) or `store.Load()` (read-only), then `store.Issue(id)` / `store.Index()`.
2. Replace any `filepath.Join(ctx.StateDir, "issues", id+".json")` with `store.IssuePath(id)` and `filepath.Join(ctx.StateDir, "index.json")` with `store.IndexPath()`.
3. Keep the `MaterializeAtSHA` time-travel path and the `arm materialize` command's explicit `materialize.Materialize` call (that command's *purpose* is materialization — it is the one legitimate direct caller; allow-list it in the guard test below).
4. Drop now-unused imports (`materialize`, `filepath`) where `goimports` flags them.

- [ ] **Step 1: Write the failing guard test**

```go
// TestHandlersDoNotReloadStateDirectly enforces that command handlers read
// Materialized State through snapshot.Store, not by calling materialize.* or
// hand-building state paths. The only allowed direct callers are listed below.
func TestHandlersDoNotReloadStateDirectly(t *testing.T) {
	allowed := map[string]bool{
		"materialize.go": true, // the `arm materialize` command itself
		"helpers.go":     true, // newSnapshotStore lives here
	}
	patterns := []string{
		"materialize.Materialize",
		"materialize.LoadIndex",
		"materialize.LoadIssue",
	}
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || allowed[name] {
			continue
		}
		data, err := os.ReadFile(name) //nolint:gosec // test reads source files in package dir
		require.NoError(t, err)
		src := string(data)
		for _, p := range patterns {
			require.NotContains(t, src, p,
				"%s should read state via snapshot.Store, not %s", name, p)
		}
	}
}
```

- [ ] **Step 2: Run the guard test to see the current violations**

Run: `go test ./cmd/armature/ -run TestHandlersDoNotReloadStateDirectly -v`
Expected: FAIL, listing each file still calling `materialize.*`.

- [ ] **Step 3: Migrate each flagged file using the recipe**

Work one file at a time. After each file, run that file's command tests (e.g. `go test ./cmd/armature/ -run 'Merged' -v`) to keep changes verifiable. Note `MaterializeAtSHA` and `MaterializeAndReturn`/`MaterializeAndReturnQuiet` are not in the guard's pattern list — if a file legitimately needs time-travel or a one-off quiet materialize, that is fine; the guard targets the duplicated current-state reload only.

- [ ] **Step 4: Run the guard test to verify it passes**

Run: `go test ./cmd/armature/ -run TestHandlersDoNotReloadStateDirectly -v`
Expected: PASS.

- [ ] **Step 5: Run the full cmd suite + `make check`**

Run: `go test ./cmd/armature/... && make check`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/armature/
git commit -m "refactor(cmd): route all handler state reads through snapshot.Store"
```

---

## Phase 2 — Candidate 02: Claim resolver

**Why third:** With the Snapshot store in place, the claim decision becomes a pure transform of a Snapshot. Today the decision (detect Scope overlap → same-worker dismissal vs. block vs. force → resolve claim race) lives in `cmd/armature/claim.go:356-422` and is reachable only end-to-end. This phase moves it into `internal/claim` behind a `Resolver` that returns a `Plan` value; the handler keeps only IO.

### Task 2.1: Define `Plan` and the overlap decision

**Files:**
- Create: `internal/claim/resolver.go`
- Test: `internal/claim/resolver_test.go`

**Interfaces:**
- Produces:
  - ```go
    type OverlapDecision int
    const (
        OverlapNone OverlapDecision = iota
        OverlapDismissSameWorker // same worker claiming serially: write dismissal note, proceed
        OverlapBlock             // different worker, no --force: block
        OverlapForce             // different worker, --force: write warning notes, proceed
    )
    type Overlap struct {
        OtherID  string
        OtherTitle string
        Decision OverlapDecision
    }
    type Plan struct {
        Overlaps []Overlap
        Blocked  bool   // true if any Overlap is OverlapBlock
        BlockMsg string // populated when Blocked
    }
    type ResolveInput struct {
        IssueID    string
        IssueScope []string
        WorkerID   string
        Force      bool
        Index      materialize.Index
        AllOps     []ops.Op  // for HasOverlapDismissalNote
    }
    func PlanClaim(in ResolveInput) Plan
    ```
- Consumes: existing `ScopesOverlap`, `HasOverlapDismissalNote` (now internal collaborators), `materialize.Index`, `ops.Op`.
- Depguard note: `internal/claim` is port-clean; importing `materialize` for `Index` is consistent with other consumers, but confirm `.golangci.yml`'s `claim` allow-list permits `materialize`. If not, pass the needed index entries as a claim-local struct instead of importing `materialize` (preferred — keeps `claim` free of `materialize`). Define:
  ```go
  type IndexEntry struct { Status, Title, Assignee string; Scope []string }
  ```
  and have `ResolveInput.Index map[string]IndexEntry`; the handler adapts `materialize.Index` into it. **Use this struct form** to avoid widening the allow-list.

- [ ] **Step 1: Write the failing test**

```go
func TestPlanClaim_SameWorkerOverlapDismissed(t *testing.T) {
	in := claim.ResolveInput{
		IssueID:    "T2",
		IssueScope: []string{"pkg/a"},
		WorkerID:   "w1",
		Index: map[string]claim.IndexEntry{
			"T1": {Status: "claimed", Title: "one", Assignee: "w1", Scope: []string{"pkg/a"}},
		},
	}
	plan := claim.PlanClaim(in)
	require.False(t, plan.Blocked)
	require.Len(t, plan.Overlaps, 1)
	require.Equal(t, claim.OverlapDismissSameWorker, plan.Overlaps[0].Decision)
}

func TestPlanClaim_DifferentWorkerBlocksWithoutForce(t *testing.T) {
	in := claim.ResolveInput{
		IssueID: "T2", IssueScope: []string{"pkg/a"}, WorkerID: "w1", Force: false,
		Index: map[string]claim.IndexEntry{
			"T1": {Status: "in-progress", Title: "one", Assignee: "w2", Scope: []string{"pkg/a"}},
		},
	}
	plan := claim.PlanClaim(in)
	require.True(t, plan.Blocked)
	require.Equal(t, claim.OverlapBlock, plan.Overlaps[0].Decision)
	require.Contains(t, plan.BlockMsg, "T1")
}

func TestPlanClaim_DifferentWorkerForceProceeds(t *testing.T) {
	in := claim.ResolveInput{
		IssueID: "T2", IssueScope: []string{"pkg/a"}, WorkerID: "w1", Force: true,
		Index: map[string]claim.IndexEntry{
			"T1": {Status: "claimed", Title: "one", Assignee: "w2", Scope: []string{"pkg/a"}},
		},
	}
	plan := claim.PlanClaim(in)
	require.False(t, plan.Blocked)
	require.Equal(t, claim.OverlapForce, plan.Overlaps[0].Decision)
}

func TestPlanClaim_NoOverlap(t *testing.T) {
	in := claim.ResolveInput{
		IssueID: "T2", IssueScope: []string{"pkg/b"}, WorkerID: "w1",
		Index: map[string]claim.IndexEntry{
			"T1": {Status: "claimed", Title: "one", Assignee: "w2", Scope: []string{"pkg/a"}},
		},
	}
	plan := claim.PlanClaim(in)
	require.False(t, plan.Blocked)
	require.Empty(t, plan.Overlaps)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/claim/ -run TestPlanClaim -v`
Expected: FAIL — `undefined: claim.PlanClaim`.

- [ ] **Step 3: Write minimal implementation**

In `internal/claim/resolver.go`, port the loop from claim.go:357-385 into a pure function:

```go
func PlanClaim(in ResolveInput) Plan {
	var plan Plan
	for id, entry := range in.Index {
		if id == in.IssueID || (entry.Status != "claimed" && entry.Status != "in-progress") {
			continue
		}
		if !ScopesOverlap(in.IssueScope, entry.Scope) {
			continue
		}
		ov := Overlap{OtherID: id, OtherTitle: entry.Title}
		switch {
		case entry.Assignee == in.WorkerID:
			ov.Decision = OverlapDismissSameWorker
		case !in.Force:
			ov.Decision = OverlapBlock
			plan.Blocked = true
			plan.BlockMsg = fmt.Sprintf("scope overlap with %s (%s)", id, entry.Title)
		default:
			ov.Decision = OverlapForce
		}
		plan.Overlaps = append(plan.Overlaps, ov)
	}
	return plan
}
```

(Iteration order over a map is nondeterministic; the tests above assert per-overlap decisions, not ordering. If the handler needs stable note ordering, sort `plan.Overlaps` by `OtherID` before returning — add a `sort.Slice` and a test asserting order.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/claim/ -run TestPlanClaim -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/claim/resolver.go internal/claim/resolver_test.go
git commit -m "feat(claim): add PlanClaim decision over a Snapshot index"
```

### Task 2.2: Use `PlanClaim` in the claim handler

**Files:**
- Modify: `cmd/armature/claim.go:356-385`

**Interfaces:**
- Consumes: `claim.PlanClaim`, `claim.IndexEntry`, the overlap decision constants. The handler adapts `materialize.Index` → `map[string]claim.IndexEntry`, calls `PlanClaim`, then performs the IO each Overlap decision implies (append dismissal/warning notes, return block error).

- [ ] **Step 1: Confirm claim tests pass (regression guard)**

Run: `go test ./cmd/armature/ -run 'Claim' -v`
Expected: PASS.

- [ ] **Step 2: Replace the overlap loop with a Plan + IO**

Replace claim.go:356-385 with:

```go
				index, _ := store.Index() //nolint:errcheck // missing index treated as empty
				ci := make(map[string]claimPkg.IndexEntry, len(index))
				for id, e := range index {
					ci[id] = claimPkg.IndexEntry{Status: e.Status, Title: e.Title, Assignee: e.Assignee, Scope: e.Scope}
				}
				plan := claimPkg.PlanClaim(claimPkg.ResolveInput{
					IssueID: issueID, IssueScope: issue.Scope, WorkerID: workerID, Force: force, Index: ci,
				})
				if plan.Blocked {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", plan.BlockMsg)
					return fmt.Errorf("cannot claim %s: %s — use --force to override", issueID, plan.BlockMsg)
				}
				for _, ov := range plan.Overlaps {
					switch ov.Decision {
					case claimPkg.OverlapDismissSameWorker:
						if !claimPkg.HasOverlapDismissalNote(allOps, issueID, ov.OtherID) {
							noteOp := ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
								WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Serial claim: scope overlap with %s (same worker, dismissed)", ov.OtherID)}}
							appendOp(ctx, logPath, noteOp) //nolint:errcheck,gosec
						}
					case claimPkg.OverlapForce:
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: scope overlap with %s (%s)\n", ov.OtherID, ov.OtherTitle)
						appendOp(ctx, logPath, ops.Op{Type: ops.OpNote, TargetID: issueID, Timestamp: nowEpoch(),
							WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", ov.OtherID)}}) //nolint:errcheck,gosec
						appendOp(ctx, logPath, ops.Op{Type: ops.OpNote, TargetID: ov.OtherID, Timestamp: nowEpoch(),
							WorkerID: workerID, Payload: ops.Payload{Msg: fmt.Sprintf("Scope overlap with %s detected at claim time", issueID)}}) //nolint:errcheck,gosec
					}
				}
```

`workerID`/`logPath` must be resolved (via `resolveWorkerAndLog`) before this block — move that call up if needed so `workerID` is available.

- [ ] **Step 3: Run claim tests**

Run: `go test ./cmd/armature/ -run 'Claim' -v`
Expected: PASS.

- [ ] **Step 4: Run `make check`**

Run: `make check`
Expected: PASS. If mutation flags an overlap branch, add the missing case to `resolver_test.go`.

- [ ] **Step 5: Commit**

```bash
git add cmd/armature/claim.go
git commit -m "refactor(cmd): claim uses claim.PlanClaim for overlap decisions"
```

---

## Phase 3 — Candidate 03: Render Context assembly

**Why fourth:** With the store providing the Snapshot, `context.Assemble` can take one input and own its layer orchestration, with its one impure dependency (reading Context Files) behind an injected `FileReader` seam so layer selection is testable without disk.

### Task 3.1: Introduce the `FileReader` seam

**Files:**
- Create: `internal/context/filereader.go`
- Test: `internal/context/assemble_test.go`

**Interfaces:**
- Produces:
  - `type FileReader interface { ReadFile(relPath string) ([]byte, error) }`
  - `type OSFileReader struct { Root string }` with `func (r OSFileReader) ReadFile(relPath string) ([]byte, error)` joining `Root` + `relPath` and calling `os.ReadFile`.
- Consumes: nothing new.

- [ ] **Step 1: Write the failing test**

```go
func TestOSFileReader_JoinsRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "doc.md"), []byte("hello"), 0o600))
	r := context.OSFileReader{Root: root}
	data, err := r.ReadFile("doc.md")
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/context/ -run TestOSFileReader -v`
Expected: FAIL — `undefined: context.OSFileReader`.

- [ ] **Step 3: Write minimal implementation**

```go
package context

import (
	"os"
	"path/filepath"
)

// FileReader reads a Context File by its repo-relative path. It is the one
// impure dependency of context assembly; tests supply an in-memory fake.
type FileReader interface {
	ReadFile(relPath string) ([]byte, error)
}

// OSFileReader reads Context Files from the real filesystem, rooted at Root.
type OSFileReader struct{ Root string }

func (r OSFileReader) ReadFile(relPath string) ([]byte, error) {
	//nolint:gosec // G304: path joined from repo root and issue-defined relative path
	return os.ReadFile(filepath.Join(r.Root, relPath))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/context/ -run TestOSFileReader -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/context/filereader.go internal/context/assemble_test.go
git commit -m "feat(context): add FileReader seam for context files"
```

### Task 3.2: Thread `FileReader` through `Assemble` and derive the graph internally

**Files:**
- Modify: `internal/context/assemble.go:30-71`, `assemble.go:88-107` (`buildContextFiles`)
- Test: `internal/context/assemble_test.go`

**Interfaces:**
- Produces: new signature `func Assemble(issueID string, state *materialize.State, reader FileReader) (*Context, error)` — the `stateDir` and `*dag.Graph` parameters are removed. The graph is built internally from `state` (move `buildGraphFromState` from `cmd/armature/render_context.go:100-114` into this package as an unexported `graphFromState`). `buildContextFiles` takes the `FileReader` instead of a `repoRoot` string.
- Consumes: `materialize.State`, `dag.BuildGraph` (already an allowed import of `context`).

- [ ] **Step 1: Write the failing test (layer selection via fake reader)**

```go
type fakeReader map[string]string

func (f fakeReader) ReadFile(p string) ([]byte, error) {
	if v, ok := f[p]; ok {
		return []byte(v), nil
	}
	return nil, os.ErrNotExist
}

func TestAssemble_ContextFilesLayerUsesReader(t *testing.T) {
	state := materialize.NewState()
	state.Issues = map[string]*materialize.Issue{
		"T1": {ID: "T1", Title: "Task", Type: "task", ContextFiles: []string{"design.md"}},
	}
	ctx, err := context.Assemble("T1", state, fakeReader{"design.md": "DESIGN BODY"})
	require.NoError(t, err)

	var cf *context.Layer
	for i := range ctx.Layers {
		if ctx.Layers[i].Name == "context_files" {
			cf = &ctx.Layers[i]
		}
	}
	require.NotNil(t, cf)
	require.Contains(t, cf.Content, "DESIGN BODY")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/context/ -run TestAssemble_ContextFilesLayerUsesReader -v`
Expected: FAIL — compile error (signature mismatch / `os.ReadFile` still used).

- [ ] **Step 3: Update implementation**

Change `Assemble` to:

```go
func Assemble(issueID string, state *materialize.State, reader FileReader) (*Context, error) {
	issue, ok := state.Issues[issueID]
	if !ok {
		return nil, fmt.Errorf("issue %s not found in state", issueID)
	}
	graph := graphFromState(state)

	var layers []Layer
	layers = append(layers, buildCoreSpec(issue))
	layers = append(layers, buildContextFiles(issue, reader))
	layers = append(layers, buildSnippets(issue))
	layers = append(layers, buildBlockerOutcomes(issue, state))
	layers = append(layers, buildParentChain(issue, graph, state))
	layers = append(layers, buildDecisions(issue))
	layers = append(layers, buildNotes(issue))
	layers = append(layers, buildSiblingOutcomes(issue, graph, state))

	sort.Slice(layers, func(i, j int) bool { return layers[i].Priority < layers[j].Priority })
	return &Context{IssueID: issueID, Layers: layers}, nil
}
```

Change `buildContextFiles` to read via the reader:

```go
func buildContextFiles(issue *materialize.Issue, reader FileReader) Layer {
	if len(issue.ContextFiles) == 0 {
		return Layer{Name: "context_files", Priority: 2, Content: ""}
	}
	var sections []string
	for _, relPath := range issue.ContextFiles {
		data, err := reader.ReadFile(relPath)
		if err != nil {
			sections = append(sections, fmt.Sprintf("### %s\n(missing: %v)", relPath, err))
			continue
		}
		sections = append(sections, fmt.Sprintf("### %s\n```text\n%s\n```", relPath, strings.TrimRight(string(data), "\n")))
	}
	return Layer{Name: "context_files", Priority: 2, Content: "## Context Files\n" + strings.Join(sections, "\n\n")}
}
```

Move `buildGraphFromState` (render_context.go:100-114) into `assemble.go` as unexported `graphFromState`. Delete `inferRepoRoot` (assemble.go:73-86) if nothing else references it. Remove the now-unused `os` import from `assemble.go`.

- [ ] **Step 4: Run the context suite**

Run: `go test ./internal/context/...`
Expected: PASS (update any existing `Assemble`/`buildContextFiles` call sites in existing tests to the new signature).

- [ ] **Step 5: Commit**

```bash
git add internal/context/assemble.go internal/context/assemble_test.go
git commit -m "refactor(context): single-input Assemble with injected FileReader"
```

### Task 3.3: Update `render-context` to the new `Assemble`

**Files:**
- Modify: `cmd/armature/render_context.go:62-69`, delete `buildGraphFromState` (render_context.go:100-114)

**Interfaces:**
- Consumes: `context.Assemble(issueID, state, context.OSFileReader{Root: appCtx.RepoPath})`. For the `rcAt` time-travel branch, the `state` comes from `MaterializeAtSHA` as before.

- [ ] **Step 1: Confirm render-context tests pass**

Run: `go test ./cmd/armature/ -run 'RenderContext|render_context' -v`
Expected: PASS.

- [ ] **Step 2: Update the call site**

Replace render_context.go:63-69:

```go
				graph := buildGraphFromState(state)

				ctx, err := context.Assemble(rcIssue, appCtx.StateDir, state, graph)
				if err != nil {
					return fmt.Errorf("assemble context: %w", err)
				}
```

with:

```go
				ctx, err := context.Assemble(rcIssue, state, context.OSFileReader{Root: appCtx.RepoPath})
				if err != nil {
					return fmt.Errorf("assemble context: %w", err)
				}
```

Delete the `buildGraphFromState` function (render_context.go:97-114) and drop the now-unused `dag` import.

- [ ] **Step 3: Run render-context tests + the cmd suite**

Run: `go test ./cmd/armature/...`
Expected: PASS. Update any other caller of `buildGraphFromState` to `context`-internal usage or `dag.BuildGraph` directly (grep `buildGraphFromState` to find them).

- [ ] **Step 4: Run `make check`**

Run: `make check`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/armature/render_context.go
git commit -m "refactor(cmd): render-context uses single-input context.Assemble"
```

---

## Phase 4 — Candidate 04: Harness Hook

**Why last:** It is the most governance-sensitive path (ADR-0003) and benefits from the Snapshot store and the staleness predicate already in place. Today `cmd/armature/harness_hook.go:99-157` performs six setup steps and the decision is split across `harnesshook` (runner/evaluator) and `harnesspolicy`. This phase introduces a `Hook` module with one `Evaluate` method owning the whole decision; the handler maps the result to an exit code.

### Task 4.1: Add the `Hook` module with `Evaluate`

**Files:**
- Create: `internal/harnesshook/hook.go`
- Test: `internal/harnesshook/hook_test.go`

**Interfaces:**
- Produces:
  - ```go
    type EvaluateInput struct {
        TaskID   string
        Platform string // ARMATURE_HOOK_PLATFORM
        Input    []byte // raw hook stdin
    }
    type Hook struct { /* unexported: resolver PolicyResolver */ }
    func NewHook(resolver PolicyResolver) *Hook
    func (h *Hook) Evaluate(ctx context.Context, in EvaluateInput) (RunResult, error)
    ```
  - `Evaluate` selects the platform adapter (`NewAdapterForPlatform`), resolves the task policy, builds the evaluator from that policy, decodes the input, evaluates, and encodes — i.e. it absorbs the body of `Runner.Run` (runner.go:63-112) plus the adapter selection currently in the handler (harness_hook.go:122-125). The existing `Runner` becomes an internal collaborator or is replaced.
- Consumes: `PolicyResolver`, `NewAdapterForPlatform`, `NewEvaluator`, `harnesspolicy.NewScopePolicy`/`NewVerificationService`.
- Note: staleness and pass-through (no binding / stale binding) stay in the handler for now, because they depend on the Snapshot store and pass-through logging to the worktree git dir — out of scope for the pure decision module. The `Hook` module owns only the "binding is live, now decide Pass/Block" portion.

- [ ] **Step 1: Write the failing test**

```go
type stubResolver struct{ policy harnesspolicy.TaskPolicy; err error }

func (s stubResolver) Resolve(string) (harnesspolicy.TaskPolicy, error) { return s.policy, s.err }

func TestHook_Evaluate_AllowsInScopeEdit(t *testing.T) {
	h := harnesshook.NewHook(stubResolver{policy: harnesspolicy.TaskPolicy{Scope: []string{"pkg/a"}}})
	// Build a platform input that the "claude" adapter decodes into an in-scope edit.
	in := harnesshook.EvaluateInput{
		TaskID:   "T1",
		Platform: "claude",
		Input:    claudeEditPayload(t, "pkg/a/file.go"), // helper builds the platform JSON
	}
	res, err := h.Evaluate(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, harnesshook.DecisionAllow, res.Decision.Action)
}

func TestHook_Evaluate_ResolverErrorPropagates(t *testing.T) {
	h := harnesshook.NewHook(stubResolver{err: errors.New("boom")})
	_, err := h.Evaluate(context.Background(), harnesshook.EvaluateInput{TaskID: "T1", Platform: "claude", Input: []byte("{}")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "T1")
}
```

(`claudeEditPayload` and the exact `DecisionAllow` constant name must match `internal/harnesshook/types.go`; check `DecisionAction` constants there and adjust. If building a real platform payload is heavy, instead test `Evaluate` with `Platform: ""` against the default adapter and assert the resolver/decode error paths, plus keep the in-scope/out-of-scope assertions at the `Evaluator` level where they already exist.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/harnesshook/ -run TestHook_Evaluate -v`
Expected: FAIL — `undefined: harnesshook.NewHook`.

- [ ] **Step 3: Write minimal implementation**

```go
package harnesshook

import (
	"context"
	"fmt"

	"github.com/scullxbones/armature/internal/harnesspolicy"
)

type EvaluateInput struct {
	TaskID   string
	Platform string
	Input    []byte
}

// Hook owns the harness-hook decision: select the platform adapter, resolve the
// task policy, evaluate the event, and encode the result. Callers supply one
// EvaluateInput and receive one RunResult.
type Hook struct {
	resolver PolicyResolver
}

func NewHook(resolver PolicyResolver) *Hook {
	return &Hook{resolver: resolver}
}

func (h *Hook) Evaluate(ctx context.Context, in EvaluateInput) (RunResult, error) {
	adapter, err := NewAdapterForPlatform(in.Platform)
	if err != nil {
		return RunResult{}, err
	}

	task, err := h.resolver.Resolve(in.TaskID)
	if err != nil {
		return RunResult{}, fmt.Errorf("resolve policy for task %s: %w", in.TaskID, err)
	}

	service := harnesspolicy.NewVerificationService()
	evaluator := NewEvaluator(EvaluatorConfig{
		ScopePolicy:         harnesspolicy.NewScopePolicy(task.Scope),
		VerificationService: &service,
		VerificationInput: harnesspolicy.VerificationRequest{
			Acceptance: task.Acceptance,
			Citations:  task.Citations,
		},
	})

	event, err := adapter.Decode(in.Input)
	if err != nil {
		return RunResult{}, fmt.Errorf("decode hook input: %w", err)
	}
	decision, err := evaluator.Evaluate(ctx, event)
	if err != nil {
		return RunResult{}, fmt.Errorf("evaluate hook: %w", err)
	}
	output, exitCode, err := adapter.Encode(event, decision)
	if err != nil {
		return RunResult{}, fmt.Errorf("encode hook output: %w", err)
	}
	return RunResult{Output: output, Decision: decision, ExitCode: exitCode}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/harnesshook/ -run TestHook_Evaluate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/harnesshook/hook.go internal/harnesshook/hook_test.go
git commit -m "feat(harnesshook): add Hook.Evaluate owning the hook decision"
```

### Task 4.2: Use `Hook.Evaluate` in the handler

**Files:**
- Modify: `cmd/armature/harness_hook.go:122-156`

**Interfaces:**
- Consumes: `harnesshook.NewHook`, `harnesshook.EvaluateInput`. The handler keeps task-binding resolution, snapshot staleness, and pass-through logging (harness_hook.go:99-120) unchanged; it replaces the adapter/resolver/runner wiring (122-156) with a single `Hook.Evaluate` call.

- [ ] **Step 1: Confirm harness-hook tests pass (regression guard)**

Run: `go test ./cmd/armature/ -run 'HarnessHook|Hook' -v`
Expected: PASS.

- [ ] **Step 2: Replace the wiring**

Replace harness_hook.go:122-156 with:

```go
				input, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read hook input: %w", err)
				}

				resolver := harnesspolicy.NewTaskPolicyResolver(harnesspolicy.ResolverConfig{
					RepoPath:   appCtx.RepoPath,
					StateDir:   appCtx.StateDir,
					SourcesDir: filepath.Join(appCtx.IssuesDir, "sources"),
				})

				result, err := harnesshook.NewHook(resolver).Evaluate(cmd.Context(), harnesshook.EvaluateInput{
					TaskID:   taskID,
					Platform: os.Getenv("ARMATURE_HOOK_PLATFORM"),
					Input:    input,
				})
				if err != nil {
					return err
				}
				return applyRunResult(cmd.OutOrStdout(), result)
```

- [ ] **Step 3: Run harness-hook tests**

Run: `go test ./cmd/armature/ -run 'HarnessHook|Hook' -v`
Expected: PASS.

- [ ] **Step 4: Decide the fate of `Runner`**

If no caller other than this handler used `harnesshook.NewRunner`, delete `runner.go` and its tests, or keep it if other code/tests depend on it. Run `grep -rn "NewRunner\|harnesshook.Runner" --include=*.go .` first; only delete when the grep shows no remaining users.

- [ ] **Step 5: Run `make check`**

Run: `make check`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/armature/harness_hook.go internal/harnesshook/
git commit -m "refactor(cmd): harness-hook decision via Hook.Evaluate"
```

---

## Final Verification

- [ ] **Run the full check suite**

Run: `make check`
Expected: PASS — lint, test, coverage ≥85%, mutation (≥95% mcover, ≥99% efficacy), validate-skills, build.

- [ ] **Confirm the deepening landed**

Run: `go test ./cmd/armature/ -run TestHandlersDoNotReloadStateDirectly -v`
Expected: PASS — handlers read Materialized State only through `snapshot.Store`.

- [ ] **Smoke-test the CLI end to end**

Run: `make build && ./bin/arm doctor`
Expected: `arm doctor` reports a healthy repo (or its usual output for this working copy) with no regressions.

---

## Self-Review Notes

- **Spec coverage:** Each architecture-review candidate maps to a phase — 05→Phase 5, 01→Phase 1, 02→Phase 2, 03→Phase 3, 04→Phase 4 — executed in the requested order 5, 1, 2, 3, 4.
- **Dependencies:** Phase 5 must precede Phase 1 (the store depends on a clean materialize entry). Phases 2–4 each depend on Phase 1's store. Within Phase 1, Task 1.1→1.2 precede the handler migrations (1.3–1.6).
- **Type consistency:** `snapshot.New`/`Store.Issue`/`Store.Index`/`Store.IssuePath`/`Store.IndexPath` are used identically across Tasks 1.2–1.6. `claim.PlanClaim`/`ResolveInput`/`IndexEntry`/`Overlap`/`OverlapDecision` are consistent across Tasks 2.1–2.2. `context.Assemble(issueID, state, reader)` and `context.OSFileReader` are consistent across Tasks 3.1–3.3. `harnesshook.NewHook`/`EvaluateInput`/`RunResult` are consistent across Tasks 4.1–4.2.
- **Open verification points the implementer must check against live code:** the exact `DecisionAction` constant names in `internal/harnesshook/types.go` (Task 4.1 test); whether `.golangci.yml`'s `claim` allow-list permits `materialize` (Task 2.1 — plan defaults to the no-import `IndexEntry` struct to stay safe); whether `materialize.Index` exposes `Assignee` and `Scope` on `IndexEntry` (Task 2.2 adapter); and the precise set of files flagged by the Task 1.6 guard test (the list is from the review's grep and may shift as earlier tasks land).
