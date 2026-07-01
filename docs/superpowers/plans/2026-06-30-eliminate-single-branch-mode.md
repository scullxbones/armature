# Eliminate Single-Branch Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove single-branch mode from Armature entirely so dual-branch operation (coordination data on the `_armature` ops branch, accessed through the ops worktree) is the only supported mode — in code, tests, docs, and skills.

**Architecture:** Work bottom-up through the dependency graph: the materialize engine's auto-merge behavior first (it's the one real behavior change, not just dead code), then the packages that thread a `singleBranch`/`Mode` parameter through it (snapshot, doctor), then config/context (removes the `Mode` field entirely), then `cmd/armature` call sites, then bootstrap (which also gains a forced-migration path per ADR 0006), then docs and skills, then a final repo-wide verification pass.

**Tech Stack:** Go, Cobra CLI, testify (`assert`/`require`).

## Global Constraints

- Reference: `docs/adr/0006-eliminate-single-branch-mode.md` records the decision this plan implements. Do not deviate from it without updating the ADR.
- Reference: `CONTEXT.md` no longer defines `Single-Branch Mode` / `Dual-Branch Mode` — do not reintroduce those glossary entries.
- The `_armature` branch name is a literal, unchanged constant — never parameterize or rename it.
- Historical docs under `docs/superpowers/plans/` and `docs/superpowers/specs/` (except this plan and ADR 0006) are NOT touched — they are historical records of past decisions, not living documentation.
- After every task: run `make check` (or the narrower `go test ./...` if `make check` is unavailable) before committing. Do not leave the tree in a non-compiling state between tasks.
- Never use `git commit --amend`; each task gets its own commit.

---

### Task 1: Remove `SingleBranchMode` auto-merge behavior from the materialize engine

This is the one real behavior change in this plan (not just dead-code removal): today, when `SingleBranchMode` is true, a transition to `done` is silently rewritten to `merged`. That auto-promotion goes away — `done` now always means `done` until a real merge is detected (via `arm sync` or `arm merged`).

**Files:**
- Modify: `internal/materialize/engine.go:14-24` (State struct), `internal/materialize/engine.go:143-172` (`applyTransition`)
- Modify: `internal/materialize/engine_test.go:254-265` (delete `TestSingleBranchAutoMerge`), `internal/materialize/engine_test.go:604-641` (`TestRunRollup_PromotesStoryWhenAllChildrenMerged`, `TestRunRollup_DoesNotPromoteWithUnmergedChild`), `internal/materialize/engine_test.go:1311-1313` and `:1364` (`BenchmarkRunRollup_10kIssues`), `internal/materialize/engine_test.go:312` (property test)

**Interfaces:**
- Produces: `materialize.State` with no `SingleBranchMode` field. `applyTransition` no longer auto-promotes `done` → `merged`.
- Consumes: nothing from later tasks.

- [ ] **Step 1: Remove the field and the auto-merge branch**

In `internal/materialize/engine.go`, change:

```go
// State holds the complete materialized state built from op replay.
type State struct {
	Issues           map[string]*Issue
	SingleBranchMode bool
}
```

to:

```go
// State holds the complete materialized state built from op replay.
type State struct {
	Issues map[string]*Issue
}
```

And in `applyTransition`, remove:

```go
	if s.SingleBranchMode && newStatus == ops.StatusDone {
		issue.Status = ops.StatusMerged
	}
	return nil
```

leaving:

```go
	return nil
```

- [ ] **Step 2: Update engine_test.go — delete the auto-merge test**

Delete this whole test (lines 254-265):

```go
func TestSingleBranchAutoMerge(t *testing.T) {
	t.Parallel()
	state := NewState()
	state.SingleBranchMode = true
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "Done"}}))
	assert.Equal(t, "merged", state.Issues["task-01"].Status)
}
```

- [ ] **Step 3: Remove the `SingleBranchMode` assignment from the property test (line 312)**

Change:

```go
			state := NewState()
			state.SingleBranchMode = true

			_ = state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: targetID, Timestamp: ts, //nolint:errcheck // property test checks for panic, not error correctness
```

to:

```go
			state := NewState()

			_ = state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: targetID, Timestamp: ts, //nolint:errcheck // property test checks for panic, not error correctness
```

- [ ] **Step 4: Fix `TestRunRollup_PromotesStoryWhenAllChildrenMerged` to test rollup directly instead of relying on auto-merge**

Change (lines 604-621):

```go
func TestRunRollup_PromotesStoryWhenAllChildrenMerged(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "story-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))

	// In single branch mode, done → merged
	state.SingleBranchMode = true
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "done"}}))

	state.RunRollup()
	assert.Equal(t, "merged", state.Issues["story-01"].Status)
}
```

to:

```go
func TestRunRollup_PromotesStoryWhenAllChildrenMerged(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "story-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "merged", Outcome: "done"}}))

	state.RunRollup()
	assert.Equal(t, "merged", state.Issues["story-01"].Status)
}
```

- [ ] **Step 5: Remove the `SingleBranchMode` assignment from `TestRunRollup_DoesNotPromoteWithUnmergedChild` (line 633)**

Change:

```go
	state.SingleBranchMode = true
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
```

to:

```go
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
```

- [ ] **Step 6: Fix the benchmark (lines 1311-1364)**

Remove the line `state.SingleBranchMode = true` after `state := NewState()` (line 1312-1313), and change the comment and payload in the "mark all tasks as done" loop:

```go
	// Mark all tasks as done, which becomes merged in single branch mode
	for si := range 100 {
		for ti := range 100 {
			taskID := taskIDs[si][ti]
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpClaim, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{TTL: 60},
			}))
			timestamp++
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpTransition, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{To: "done"},
			}))
			timestamp++
		}
	}
```

to:

```go
	// Mark all tasks as merged so the rollup benchmark exercises full cascade promotion.
	for si := range 100 {
		for ti := range 100 {
			taskID := taskIDs[si][ti]
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpClaim, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{TTL: 60},
			}))
			timestamp++
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpTransition, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{To: "merged"},
			}))
			timestamp++
		}
	}
```

- [ ] **Step 7: Run the materialize package tests**

Run: `go test ./internal/materialize/...`
Expected: PASS (compile errors about `SingleBranchMode` are gone; rollup and property tests pass).

- [ ] **Step 8: Commit**

```bash
git add internal/materialize/engine.go internal/materialize/engine_test.go
git commit -m "materialize: remove single-branch auto-merge-on-done behavior"
```

---

### Task 2: Remove `singleBranch`/`SingleBranch` parameter threading from materialize pipeline

**Files:**
- Modify: `internal/materialize/pipeline.go` (Options struct, `runFullPipeline`, `runExcludeWorker`, `Run`, `Materialize`, `MaterializeAndReturnQuiet`, `MaterializeAndReturn`, `MaterializeExcludeWorker`)
- Test: `internal/materialize/pipeline_test.go` (grep for `SingleBranch`/`singleBranch` and update any call sites — see Step 3)

**Interfaces:**
- Consumes: `materialize.State` from Task 1 (no `SingleBranchMode` field).
- Produces: `materialize.Materialize(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (Result, error)` — 3 params instead of 4. Same signature change for `MaterializeAndReturnQuiet`, `MaterializeAndReturn`, `MaterializeExcludeWorker(allOps []ops.Op, excludeWorkerID string) (*State, Result, error)`. `Run(stateDir string, allOps []ops.Op, byteOffsets map[string]int64, opts Options) (*State, Result, error)` unchanged in shape but `Options` no longer has `SingleBranch`.

- [ ] **Step 1: Remove `SingleBranch` from `Options`**

Change:

```go
type Options struct {
	// WriteStateFiles controls whether state files and checkpoints are written to disk.
	// When false, checkpoint reads are also skipped, forcing a full in-memory replay
	// (no incremental mode). Use false for read-only/diagnostic calls.
	WriteStateFiles bool
	ExcludeWorkerID string // If set, filters out ops from this worker (diagnostic mode only)
	EmitWarnings    bool   // Controls whether warnings are emitted to stderr
	SingleBranch    bool   // Controls single-branch mode for auto-merging
}
```

to:

```go
type Options struct {
	// WriteStateFiles controls whether state files and checkpoints are written to disk.
	// When false, checkpoint reads are also skipped, forcing a full in-memory replay
	// (no incremental mode). Use false for read-only/diagnostic calls.
	WriteStateFiles bool
	ExcludeWorkerID string // If set, filters out ops from this worker (diagnostic mode only)
	EmitWarnings    bool   // Controls whether warnings are emitted to stderr
}
```

- [ ] **Step 2: Update `runFullPipeline` and `runExcludeWorker` signatures**

Change:

```go
func runFullPipeline(stateDir string, allOps []ops.Op, singleBranch bool,
	byteOffsets map[string]int64, emitWarnings bool, writeStateFiles bool) (*State, Result, error) {
```

to:

```go
func runFullPipeline(stateDir string, allOps []ops.Op,
	byteOffsets map[string]int64, emitWarnings bool, writeStateFiles bool) (*State, Result, error) {
```

Remove both `state.SingleBranchMode = singleBranch` assignments (in the `!fullReplay` branch and the `else` branch):

```go
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
```

becomes:

```go
	if !fullReplay {
		loadedIssues, err := LoadAllIssues(issuesStateDir)
		if err != nil {
			return nil, Result{}, fmt.Errorf("load prior state: %w", err)
		}
		state = NewState()
		state.Issues = loadedIssues
	} else {
		state = NewState()
	}
```

Change:

```go
func runExcludeWorker(allOps []ops.Op, excludeWorkerID string, singleBranch bool, emitWarnings bool) (*State, Result, error) {
```

to:

```go
func runExcludeWorker(allOps []ops.Op, excludeWorkerID string, emitWarnings bool) (*State, Result, error) {
```

and remove `state.SingleBranchMode = singleBranch` after `state := NewState()` in that function.

- [ ] **Step 3: Update `Run` and the public `Materialize*` functions**

Change:

```go
func Run(stateDir string, allOps []ops.Op, byteOffsets map[string]int64, opts Options) (*State, Result, error) {
	if opts.ExcludeWorkerID != "" {
		return runExcludeWorker(allOps, opts.ExcludeWorkerID, opts.SingleBranch, opts.EmitWarnings)
	}
	return runFullPipeline(stateDir, allOps, opts.SingleBranch, byteOffsets, opts.EmitWarnings, opts.WriteStateFiles)
}

// Materialize runs the full materialization pipeline.
// It accepts pre-read ops and writes state and checkpoint files to stateDir.
// issuesDir is used to resolve stateDir paths; allOps should be pre-read from the log files.
// byteOffsets maps log filename -> byte offset (end position). Can be nil for no checkpoint tracking.
func Materialize(stateDir string, allOps []ops.Op, singleBranch bool, byteOffsets map[string]int64) (Result, error) {
	_, result, err := Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: true, SingleBranch: singleBranch})
	return result, err
}

// MaterializeAndReturnQuiet runs the full materialization pipeline without emitting
// warnings to stderr. Snapshot-backed commands use this to avoid duplicate warnings
// because they render returned warnings themselves.
func MaterializeAndReturnQuiet(stateDir string, allOps []ops.Op, singleBranch bool, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: false, SingleBranch: singleBranch})
}

// MaterializeAndReturn runs the full materialization pipeline and returns the resulting State.
// It accepts pre-read ops and writes state and checkpoint files to stateDir.
// byteOffsets maps log filename -> byte offset (end position). Can be nil for no checkpoint tracking.
func MaterializeAndReturn(stateDir string, allOps []ops.Op, singleBranch bool, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: true, SingleBranch: singleBranch})
}

// MaterializeExcludeWorker replays ops excluding all ops from the given
// workerID. This is a diagnostic-only mode: state files and checkpoint are NOT
// updated. Returns the resulting State and Result.
// allOps should be pre-read from log files.
func MaterializeExcludeWorker(allOps []ops.Op, excludeWorkerID string, singleBranch bool) (*State, Result, error) {
	return Run("", allOps, nil, Options{ExcludeWorkerID: excludeWorkerID, EmitWarnings: true, SingleBranch: singleBranch})
}
```

to:

```go
func Run(stateDir string, allOps []ops.Op, byteOffsets map[string]int64, opts Options) (*State, Result, error) {
	if opts.ExcludeWorkerID != "" {
		return runExcludeWorker(allOps, opts.ExcludeWorkerID, opts.EmitWarnings)
	}
	return runFullPipeline(stateDir, allOps, byteOffsets, opts.EmitWarnings, opts.WriteStateFiles)
}

// Materialize runs the full materialization pipeline.
// It accepts pre-read ops and writes state and checkpoint files to stateDir.
// issuesDir is used to resolve stateDir paths; allOps should be pre-read from the log files.
// byteOffsets maps log filename -> byte offset (end position). Can be nil for no checkpoint tracking.
func Materialize(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (Result, error) {
	_, result, err := Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: true})
	return result, err
}

// MaterializeAndReturnQuiet runs the full materialization pipeline without emitting
// warnings to stderr. Snapshot-backed commands use this to avoid duplicate warnings
// because they render returned warnings themselves.
func MaterializeAndReturnQuiet(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: false})
}

// MaterializeAndReturn runs the full materialization pipeline and returns the resulting State.
// It accepts pre-read ops and writes state and checkpoint files to stateDir.
// byteOffsets maps log filename -> byte offset (end position). Can be nil for no checkpoint tracking.
func MaterializeAndReturn(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (*State, Result, error) {
	return Run(stateDir, allOps, byteOffsets, Options{WriteStateFiles: true, EmitWarnings: true})
}

// MaterializeExcludeWorker replays ops excluding all ops from the given
// workerID. This is a diagnostic-only mode: state files and checkpoint are NOT
// updated. Returns the resulting State and Result.
// allOps should be pre-read from log files.
func MaterializeExcludeWorker(allOps []ops.Op, excludeWorkerID string) (*State, Result, error) {
	return Run("", allOps, nil, Options{ExcludeWorkerID: excludeWorkerID, EmitWarnings: true})
}
```

- [ ] **Step 4: Fix any remaining call sites within the materialize package's own tests**

Run: `grep -n "SingleBranch\|singleBranch" internal/materialize/*_test.go`

For every match found (e.g. `internal/materialize/pipeline_test.go` calling `Materialize(...)`/`MaterializeAndReturn(...)` with a boolean argument), drop that boolean argument from the call. These are mechanical: the 3rd positional `bool` argument (`false` or `true`) is removed from each call.

- [ ] **Step 5: Run the materialize package tests**

Run: `go test ./internal/materialize/...`
Expected: PASS. (This will fail to compile until Task 3 also updates `internal/snapshot`, since `snapshot.go` calls `materialize.MaterializeAndReturnQuiet` — if `go test ./internal/materialize/...` passes in isolation but `go build ./...` fails elsewhere, that's expected at this point; full green comes after Task 3.)

- [ ] **Step 6: Commit**

```bash
git add internal/materialize/pipeline.go internal/materialize/pipeline_test.go
git commit -m "materialize: drop singleBranch parameter from pipeline functions"
```

---

### Task 3: Remove `singleBranch` parameter from `internal/snapshot`

**Files:**
- Modify: `internal/snapshot/snapshot.go`

**Interfaces:**
- Consumes: `materialize.MaterializeAndReturnQuiet(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (*State, Result, error)` from Task 2.
- Produces: `snapshot.Load(opsDir, stateDir string) (*Snapshot, error)` and `snapshot.NewStore(opsDir, stateDir string) *Store` — both drop the trailing `bool`.

- [ ] **Step 1: Update `Load`**

Change:

```go
// Load materializes state from opsDir and stateDir, returning a populated Snapshot.
// Returns a non-nil Snapshot with empty collections when opsDir is empty.
func Load(opsDir, stateDir string, singleBranch bool) (*Snapshot, error) {
	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops: %w", err)
	}

	allOps := ops.ExtractOps(items)

	state, result, err := materialize.MaterializeAndReturnQuiet(stateDir, allOps, singleBranch, offsets)
```

to:

```go
// Load materializes state from opsDir and stateDir, returning a populated Snapshot.
// Returns a non-nil Snapshot with empty collections when opsDir is empty.
func Load(opsDir, stateDir string) (*Snapshot, error) {
	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops: %w", err)
	}

	allOps := ops.ExtractOps(items)

	state, result, err := materialize.MaterializeAndReturnQuiet(stateDir, allOps, offsets)
```

- [ ] **Step 2: Update `Store`**

Change:

```go
type Store struct {
	opsDir       string
	stateDir     string
	singleBranch bool
	current      *Snapshot
}

// NewStore creates a new Store for loading snapshots from the given directories.
func NewStore(opsDir, stateDir string, singleBranch bool) *Store {
	return &Store{
		opsDir:       opsDir,
		stateDir:     stateDir,
		singleBranch: singleBranch,
	}
}

// Load loads the snapshot from disk and caches it.
func (s *Store) Load(ctx context.Context) (*Snapshot, error) {
	snap, err := Load(s.opsDir, s.stateDir, s.singleBranch)
```

to:

```go
type Store struct {
	opsDir   string
	stateDir string
	current  *Snapshot
}

// NewStore creates a new Store for loading snapshots from the given directories.
func NewStore(opsDir, stateDir string) *Store {
	return &Store{
		opsDir:   opsDir,
		stateDir: stateDir,
	}
}

// Load loads the snapshot from disk and caches it.
func (s *Store) Load(ctx context.Context) (*Snapshot, error) {
	snap, err := Load(s.opsDir, s.stateDir)
```

- [ ] **Step 3: Check for a snapshot package test file**

Run: `ls internal/snapshot/*_test.go 2>/dev/null && grep -n "singleBranch\|SingleBranch\|Load(\|NewStore(" internal/snapshot/*_test.go`

If any test file exists and calls `Load(...)` or `NewStore(...)` with a trailing bool, drop that argument from each call (mechanical, same pattern as Task 2 Step 4).

- [ ] **Step 4: Run the snapshot package tests**

Run: `go test ./internal/snapshot/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/snapshot/snapshot.go
git commit -m "snapshot: drop singleBranch parameter from Load and NewStore"
```

---

### Task 4: Simplify `internal/doctor` materialize call

**Files:**
- Modify: `internal/doctor/doctor.go:89-104`

**Interfaces:**
- Consumes: `materialize.Materialize(stateDir string, allOps []ops.Op, byteOffsets map[string]int64) (Result, error)` from Task 2.

- [ ] **Step 1: Remove the hardcoded `singleBranch` variable**

Change:

```go
func Run(issuesDir string, stateDir string, repoPath string, verbose bool, now time.Time) (Report, error) {
	singleBranch := true // single-branch is the default for doctor

	// Read ops from the ops directory using validated stream (excludes worker-ID mismatches)
	opsDir := filepath.Join(issuesDir, "ops")
	opItems, _, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return Report{}, fmt.Errorf("read ops: %w", err)
	}

	// Extract ops from OpItems
	allOps := ops.ExtractOps(opItems)

	if _, err := materialize.Materialize(stateDir, allOps, singleBranch, nil); err != nil {
		return Report{}, fmt.Errorf("materialize: %w", err)
	}
```

to:

```go
func Run(issuesDir string, stateDir string, repoPath string, verbose bool, now time.Time) (Report, error) {
	// Read ops from the ops directory using validated stream (excludes worker-ID mismatches)
	opsDir := filepath.Join(issuesDir, "ops")
	opItems, _, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return Report{}, fmt.Errorf("read ops: %w", err)
	}

	// Extract ops from OpItems
	allOps := ops.ExtractOps(opItems)

	if _, err := materialize.Materialize(stateDir, allOps, nil); err != nil {
		return Report{}, fmt.Errorf("materialize: %w", err)
	}
```

- [ ] **Step 2: Check doctor tests for stale calls**

Run: `grep -n "singleBranch\|SingleBranch\|materialize\.Materialize(" internal/doctor/doctor_test.go`

Fix any call sites the same mechanical way as prior tasks (drop the bool argument).

- [ ] **Step 3: Run the doctor package tests**

Run: `go test ./internal/doctor/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/doctor/doctor.go
git commit -m "doctor: drop hardcoded singleBranch flag from materialize call"
```

---

### Task 5: Remove `Mode` field from config/context; always require the ops worktree

This collapses `RepoProbeResult`/`Context`/`Config` so there is no mode to branch on: `ResolveContext` always resolves `IssuesDir` from `armature.ops-worktree-path`, and errors if that git config key isn't set (same error path dual-branch already used when misconfigured).

**Files:**
- Modify: `internal/config/config.go`, `internal/config/context.go`
- Modify: `internal/config/config_test.go`, `internal/config/context_test.go`, `internal/config/context_probe_test.go`
- Modify: `internal/adapters/shell.go` (`GitConfigMode` removal — folded in here since `context.go`'s `defaultRepoProbe.Probe` is its only caller), `internal/adapters/shell_test.go`

**Interfaces:**
- Produces: `config.Config` with no `Mode` field. `config.Context` and `config.RepoProbeResult` with no `Mode` field. `ResolveContextWithProbe` errors with `"armature.ops-worktree-path must be set; run \`arm bootstrap\`"` when `WorktreePath` is empty.
- Consumes: nothing from later tasks (this task is self-contained; `cmd/armature` callers are fixed in Task 7/8/9, which run after this).

- [ ] **Step 1: Remove `Mode` from `Config` and `DefaultConfig`**

In `internal/config/config.go`, change:

```go
type Config struct {
	Mode                   string       `json:"mode"` // "single-branch" or "dual-branch"
	ProjectType            string       `json:"project_type"`
	DefaultTTL             int          `json:"default_ttl"` // minutes
	TokenBudget            int          `json:"token_budget"`
	LowStakesPushThreshold int          `json:"low_stakes_push_threshold"` // ops before auto-push
	Hooks                  []HookConfig `json:"hooks"`
}
```

to:

```go
type Config struct {
	ProjectType            string       `json:"project_type"`
	DefaultTTL             int          `json:"default_ttl"` // minutes
	TokenBudget            int          `json:"token_budget"`
	LowStakesPushThreshold int          `json:"low_stakes_push_threshold"` // ops before auto-push
	Hooks                  []HookConfig `json:"hooks"`
}
```

Change:

```go
// DefaultConfig returns a config with sensible defaults for single-branch mode.
func DefaultConfig(projectType string) Config {
	return Config{
		Mode:                   "single-branch",
		ProjectType:            projectType,
		DefaultTTL:             60,
		TokenBudget:            1600,
		LowStakesPushThreshold: 5,
		Hooks:                  []HookConfig{},
	}
}
```

to:

```go
// DefaultConfig returns a config with sensible defaults.
func DefaultConfig(projectType string) Config {
	return Config{
		ProjectType:            projectType,
		DefaultTTL:             60,
		TokenBudget:            1600,
		LowStakesPushThreshold: 5,
		Hooks:                  []HookConfig{},
	}
}
```

- [ ] **Step 2: Remove `Mode` from `Context` and `RepoProbeResult`, simplify resolution**

In `internal/config/context.go`, change:

```go
// Context holds resolved paths and config for the current armature session.
type Context struct {
	RepoPath     string // resolved repo root
	IssuesDir    string // path to issues directory
	WorktreePath string // path to .arm/ worktree; empty in single-branch mode
	StateDir     string // path to runtime state directory
	Mode         string // "single-branch" or "dual-branch"
	Config       Config // loaded from IssuesDir/config.json
}

// RepoProbeResult holds the repository facts collected through adapter-backed probing.
type RepoProbeResult struct {
	RepoPath     string
	Mode         string
	WorktreePath string
}
```

to:

```go
// Context holds resolved paths and config for the current armature session.
type Context struct {
	RepoPath     string // resolved repo root
	IssuesDir    string // path to issues directory
	WorktreePath string // path to the ops worktree (.arm/)
	StateDir     string // path to runtime state directory
	Config       Config // loaded from IssuesDir/config.json
}

// RepoProbeResult holds the repository facts collected through adapter-backed probing.
type RepoProbeResult struct {
	RepoPath     string
	WorktreePath string
}
```

Change `ResolveContext`:

```go
func ResolveContext(repoPath string) (*Context, error) {
	probeResult, err := defaultRepoProbe{}.Probe(repoPath)
	if err != nil {
		return nil, err
	}
	issuesDir := filepath.Join(probeResult.RepoPath, ".armature")
	if probeResult.Mode == "dual-branch" {
		issuesDir = filepath.Join(probeResult.WorktreePath, ".armature")
	}

	cfg, err := LoadConfig(filepath.Join(issuesDir, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return ResolveContextWithProbe(repoPath, staticRepoProbe{result: probeResult}, cfg)
}
```

to:

```go
func ResolveContext(repoPath string) (*Context, error) {
	probeResult, err := defaultRepoProbe{}.Probe(repoPath)
	if err != nil {
		return nil, err
	}
	if probeResult.WorktreePath == "" {
		return nil, fmt.Errorf("armature.ops-worktree-path must be set; run `arm bootstrap`")
	}
	issuesDir := filepath.Join(probeResult.WorktreePath, ".armature")

	cfg, err := LoadConfig(filepath.Join(issuesDir, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return ResolveContextWithProbe(repoPath, staticRepoProbe{result: probeResult}, cfg)
}
```

Change `ResolveContextWithProbe`:

```go
func ResolveContextWithProbe(repoPath string, probe RepoProbe, cfg Config) (*Context, error) {
	probeResult, err := probe.Probe(repoPath)
	if err != nil {
		return nil, err
	}

	issuesDir := filepath.Join(probeResult.RepoPath, ".armature")
	if probeResult.Mode == "dual-branch" {
		if probeResult.WorktreePath == "" {
			return nil, fmt.Errorf("dual-branch mode requires armature.ops-worktree-path to be set")
		}
		issuesDir = filepath.Join(probeResult.WorktreePath, ".armature")
	}
	if probeResult.Mode != "single-branch" && probeResult.Mode != "dual-branch" {
		return nil, fmt.Errorf("unknown armature mode: %q", probeResult.Mode)
	}

	return &Context{
		RepoPath:     probeResult.RepoPath,
		IssuesDir:    issuesDir,
		WorktreePath: probeResult.WorktreePath,
		Mode:         probeResult.Mode,
		Config:       cfg,
	}, nil
}
```

to:

```go
func ResolveContextWithProbe(repoPath string, probe RepoProbe, cfg Config) (*Context, error) {
	probeResult, err := probe.Probe(repoPath)
	if err != nil {
		return nil, err
	}
	if probeResult.WorktreePath == "" {
		return nil, fmt.Errorf("armature.ops-worktree-path must be set; run `arm bootstrap`")
	}

	return &Context{
		RepoPath:     probeResult.RepoPath,
		IssuesDir:    filepath.Join(probeResult.WorktreePath, ".armature"),
		WorktreePath: probeResult.WorktreePath,
		Config:       cfg,
	}, nil
}
```

Change `defaultRepoProbe.Probe`:

```go
func (defaultRepoProbe) Probe(repoPath string) (RepoProbeResult, error) {
	isWorktree, err := isGitWorktree(repoPath)
	if err != nil {
		return RepoProbeResult{}, fmt.Errorf("check git worktree: %w", err)
	}

	actualRepoPath := repoPath
	if isWorktree {
		actualRepoPath, err = resolveParentRepoFromWorktree(repoPath)
		if err != nil {
			return RepoProbeResult{}, fmt.Errorf("resolve parent repo from worktree: %w", err)
		}
	}

	mode, err := adapters.GitConfigMode(actualRepoPath)
	if err != nil {
		return RepoProbeResult{}, fmt.Errorf("read armature mode: %w", err)
	}

	result := RepoProbeResult{
		RepoPath: actualRepoPath,
		Mode:     mode,
	}
	if mode == "dual-branch" {
		worktreePath, err := adapters.GitConfig(actualRepoPath, "armature.ops-worktree-path")
		if err != nil {
			return RepoProbeResult{}, fmt.Errorf("dual-branch mode requires armature.ops-worktree-path to be set: %w", err)
		}
		result.WorktreePath = worktreePath
	}
	return result, nil
}
```

to:

```go
func (defaultRepoProbe) Probe(repoPath string) (RepoProbeResult, error) {
	isWorktree, err := isGitWorktree(repoPath)
	if err != nil {
		return RepoProbeResult{}, fmt.Errorf("check git worktree: %w", err)
	}

	actualRepoPath := repoPath
	if isWorktree {
		actualRepoPath, err = resolveParentRepoFromWorktree(repoPath)
		if err != nil {
			return RepoProbeResult{}, fmt.Errorf("resolve parent repo from worktree: %w", err)
		}
	}

	worktreePath, err := adapters.GitConfig(actualRepoPath, "armature.ops-worktree-path")
	if err != nil {
		return RepoProbeResult{RepoPath: actualRepoPath}, nil //nolint:nilerr // missing key surfaces as empty WorktreePath, checked by callers
	}

	return RepoProbeResult{
		RepoPath:     actualRepoPath,
		WorktreePath: worktreePath,
	}, nil
}
```

- [ ] **Step 2: Remove `GitConfigMode` from `internal/adapters/shell.go`**

Delete this function entirely:

```go
// GitConfigMode reads armature.mode from git config, defaulting to "single-branch" if unset.
func GitConfigMode(repoPath string) (string, error) {
	cmd := NonInteractiveGitCommand(repoPath, "config", "armature.mode")
	out, err := cmd.Output()
	if err != nil {
		// Exit code 1 means key not set — default to single-branch
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "single-branch", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
```

Check whether `errors` is still used elsewhere in `shell.go` after this removal (`grep -n "errors\." internal/adapters/shell.go`); if `errors.As` was its only use, remove the `"errors"` import too.

- [ ] **Step 3: Delete `TestGitConfigMode_Default` from `internal/adapters/shell_test.go`**

Delete:

```go
func TestGitConfigMode_Default(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := exec.CommandContext(context.Background(), "git", "init", dir)
	if err := cmd.Run(); err != nil {
		t.Skip("git not available")
	}
	mode, err := GitConfigMode(dir)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "single-branch" {
		t.Fatalf("expected default 'single-branch', got %q", mode)
	}
}
```

Check whether `context` and `exec` imports are still used elsewhere in the file after this deletion; keep them if so (other tests in the file likely use them).

- [ ] **Step 4: Rewrite `internal/config/config_test.go`**

Change:

```go
func TestConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := Config{
		Mode:        "single-branch",
		ProjectType: "go",
		DefaultTTL:  60,
		TokenBudget: 1600,
		Hooks:       []HookConfig{},
	}

	require.NoError(t, WriteConfig(configPath, cfg))

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "single-branch", loaded.Mode)
	assert.Equal(t, "go", loaded.ProjectType)
	assert.Equal(t, 60, loaded.DefaultTTL)
}
```

to:

```go
func TestConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := Config{
		ProjectType: "go",
		DefaultTTL:  60,
		TokenBudget: 1600,
		Hooks:       []HookConfig{},
	}

	require.NoError(t, WriteConfig(configPath, cfg))

	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "go", loaded.ProjectType)
	assert.Equal(t, 60, loaded.DefaultTTL)
}
```

Change:

```go
	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "single-branch", loaded.Mode)
	assert.Equal(t, "go", loaded.ProjectType)
}
```

(the second occurrence, inside `TestDefaultConfigHasNoOrchestratorSection`) to:

```go
	loaded, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, "go", loaded.ProjectType)
}
```

- [ ] **Step 5: Rewrite `internal/config/context_probe_test.go`**

Change:

```go
type fakeRepoProbe struct {
	mode         string
	worktreePath string
}

func (f fakeRepoProbe) Probe(repoPath string) (RepoProbeResult, error) {
	return RepoProbeResult{
		RepoPath:     repoPath,
		Mode:         f.mode,
		WorktreePath: f.worktreePath,
	}, nil
}

func TestResolveContextSeparatesRepoProbeFromContextDerivation(t *testing.T) {
	t.Parallel()
	probe := fakeRepoProbe{
		mode:         "dual-branch",
		worktreePath: "/repo/.arm",
	}

	ctx, err := ResolveContextWithProbe("/repo", probe, Config{})

	require.NoError(t, err)
	assert.Equal(t, "/repo", ctx.RepoPath)
	assert.Equal(t, "/repo/.arm/.armature", ctx.IssuesDir)
}
```

to:

```go
type fakeRepoProbe struct {
	worktreePath string
}

func (f fakeRepoProbe) Probe(repoPath string) (RepoProbeResult, error) {
	return RepoProbeResult{
		RepoPath:     repoPath,
		WorktreePath: f.worktreePath,
	}, nil
}

func TestResolveContextSeparatesRepoProbeFromContextDerivation(t *testing.T) {
	t.Parallel()
	probe := fakeRepoProbe{
		worktreePath: "/repo/.arm",
	}

	ctx, err := ResolveContextWithProbe("/repo", probe, Config{})

	require.NoError(t, err)
	assert.Equal(t, "/repo", ctx.RepoPath)
	assert.Equal(t, "/repo/.arm/.armature", ctx.IssuesDir)
}
```

- [ ] **Step 6: Rewrite `internal/config/context_test.go`**

Replace the whole file with:

```go
package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

func setOpsWorktreeConfig(t *testing.T, repo, worktreePath string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "config", "armature.ops-worktree-path", worktreePath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git config: %s", out)
}

func TestResolveContext_ResolvesIssuesDirFromWorktree(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	worktreePath := filepath.Join(repo, ".arm")
	issuesDir := filepath.Join(worktreePath, ".armature")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))
	setOpsWorktreeConfig(t, repo, worktreePath)

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, issuesDir, ctx.IssuesDir)
	assert.Equal(t, repo, ctx.RepoPath)
	assert.Equal(t, worktreePath, ctx.WorktreePath)
	assert.Equal(t, "go", ctx.Config.ProjectType)
}

func TestResolveContext_RequiresOpsWorktreePath(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	// No armature.ops-worktree-path set — resolution must fail.
	_, err := ResolveContext(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "armature.ops-worktree-path")
}

func TestContextStateDir(t *testing.T) {
	t.Parallel()
	ctx := &Context{
		StateDir: "/tmp/armature-state",
	}
	assert.Equal(t, "/tmp/armature-state", ctx.StateDir)
}

func TestResolveContext_GitWorktree(t *testing.T) {
	t.Parallel()
	// Create a "parent" repo that will be the actual git repo
	parentRepo := initTestRepo(t)

	// Create parent repo's ops worktree and .armature directory
	worktreePath := filepath.Join(parentRepo, ".arm")
	parentIssuesDir := filepath.Join(worktreePath, ".armature")
	require.NoError(t, os.MkdirAll(parentIssuesDir, 0755))
	require.NoError(t, WriteConfig(filepath.Join(parentIssuesDir, "config.json"), DefaultConfig("go")))
	setOpsWorktreeConfig(t, parentRepo, worktreePath)

	// Create a worktree checkout directory (simulates git worktree add)
	worktreeCheckout := filepath.Join(parentRepo, "worktree-checkout")
	require.NoError(t, os.MkdirAll(worktreeCheckout, 0755))

	// In a git worktree, .git is a FILE (not directory) containing "gitdir: <path>"
	// The gitdir typically points to .git/worktrees/<name> in the parent repo
	gitdirPath := filepath.Join(parentRepo, ".git", "worktrees", "test-wt")
	require.NoError(t, os.MkdirAll(gitdirPath, 0755))
	gitFileContent := fmt.Sprintf("gitdir: %s\n", gitdirPath)
	require.NoError(t, os.WriteFile(filepath.Join(worktreeCheckout, ".git"), []byte(gitFileContent), 0644))

	// When ResolveContext is called from the worktree checkout path,
	// it should detect that .git is a file and resolve IssuesDir relative to parentRepo
	ctx, err := ResolveContext(worktreeCheckout)
	require.NoError(t, err)
	assert.Equal(t, parentRepo, ctx.RepoPath)
	assert.Equal(t, parentIssuesDir, ctx.IssuesDir)
}
```

- [ ] **Step 7: Run the config and adapters package tests**

Run: `go test ./internal/config/... ./internal/adapters/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/config/config.go internal/config/context.go internal/config/config_test.go \
  internal/config/context_test.go internal/config/context_probe_test.go \
  internal/adapters/shell.go internal/adapters/shell_test.go
git commit -m "config: remove Mode field, always require ops-worktree-path"
```

---

### Task 6: Update `cmd/armature` snapshot/materialize call sites

These are the mechanical call sites: every `snapshot.Load(...)` / `snapshot.NewStore(...)` call drops its trailing `ctx.Mode == "single-branch"` argument (per Task 3's new signature), and the two `materialize.Materialize*` calls in `materialize.go` drop theirs (per Task 2).

**Files:**
- Modify: `cmd/armature/show.go:34-36`, `cmd/armature/materialize.go:25,38`, `cmd/armature/harness_context.go:23`, `cmd/armature/harness_hook.go:108`, `cmd/armature/reparent.go:36`, `cmd/armature/ready.go:52`, `cmd/armature/review.go:67-70,256-258`, `cmd/armature/sync.go:38-41,109`, `cmd/armature/tui.go:39-40`, `cmd/armature/helpers.go:349-361`

**Interfaces:**
- Consumes: `snapshot.Load(opsDir, stateDir string)`, `snapshot.NewStore(opsDir, stateDir string)` from Task 3; `materialize.MaterializeExcludeWorker(allOps, excludeWorker string)`, `materialize.Materialize(stateDir, allOps, offsets)` from Task 2.

- [ ] **Step 1: `show.go`**

Change:

```go
			ctx := currentCtx(cmd)
			issuesDir := ctx.IssuesDir
			singleBranch := ctx.Mode == "single-branch"

			snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir, singleBranch)
```

to:

```go
			ctx := currentCtx(cmd)
			issuesDir := ctx.IssuesDir

			snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir)
```

- [ ] **Step 2: `materialize.go`**

Change:

```go
			if excludeWorker != "" {
				_, result, err := materialize.MaterializeExcludeWorker(allOps, excludeWorker, appCtx.Mode == "single-branch")
```

to:

```go
			if excludeWorker != "" {
				_, result, err := materialize.MaterializeExcludeWorker(allOps, excludeWorker)
```

Change:

```go
			result, err := materialize.Materialize(appCtx.StateDir, allOps, appCtx.Mode == "single-branch", offsets)
```

to:

```go
			result, err := materialize.Materialize(appCtx.StateDir, allOps, offsets)
```

- [ ] **Step 3: `harness_context.go`**

Change:

```go
	snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), stateDir, appCtx.Mode == "single-branch")
```

to:

```go
	snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), stateDir)
```

- [ ] **Step 4: `harness_hook.go`**

Change:

```go
			snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir, appCtx.Mode == "single-branch")
```

to:

```go
			snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir)
```

- [ ] **Step 5: `reparent.go`**

Change:

```go
			snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir, appCtx.Mode == "single-branch")
```

to:

```go
			snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), appCtx.StateDir)
```

- [ ] **Step 6: `ready.go`**

Change:

```go
			snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir, ctx.Mode == "single-branch")
```

to:

```go
			snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir)
```

- [ ] **Step 7: `review.go`**

Change (first occurrence, around line 67):

```go
	ctx := currentCtx(cmd)
	issuesDir := ctx.IssuesDir
	singleBranch := ctx.Mode == "single-branch"

	// Load snapshot to get issue metadata
	snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir, singleBranch)
```

to:

```go
	ctx := currentCtx(cmd)
	issuesDir := ctx.IssuesDir

	// Load snapshot to get issue metadata
	snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir)
```

Change (second occurrence, around line 256):

```go
	// Load snapshot to check for duplicates
	ctx := currentCtx(cmd)
	issuesDir := ctx.IssuesDir
	singleBranch := ctx.Mode == "single-branch"

	snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir, singleBranch)
```

to:

```go
	// Load snapshot to check for duplicates
	ctx := currentCtx(cmd)
	issuesDir := ctx.IssuesDir

	snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), ctx.StateDir)
```

- [ ] **Step 8: `sync.go`**

Change:

```go
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			// Load snapshot to ensure state is up to date
			snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), appCtx.StateDir, singleBranch)
```

to:

```go
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			// Load snapshot to ensure state is up to date
			snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), appCtx.StateDir)
```

And further down in the same function:

```go
			// Re-load snapshot so state files reflect the new merged status
			snap, err = snapshot.Load(filepath.Join(issuesDir, "ops"), appCtx.StateDir, singleBranch)
```

to:

```go
			// Re-load snapshot so state files reflect the new merged status
			snap, err = snapshot.Load(filepath.Join(issuesDir, "ops"), appCtx.StateDir)
```

- [ ] **Step 9: `tui.go`**

Change:

```go
				tuiOpsDir := filepath.Join(appCtx.IssuesDir, "ops")
				singleBranch := appCtx.Mode == "single-branch"
				store := snapshot.NewStore(tuiOpsDir, stateDir, singleBranch)
```

to:

```go
				tuiOpsDir := filepath.Join(appCtx.IssuesDir, "ops")
				store := snapshot.NewStore(tuiOpsDir, stateDir)
```

- [ ] **Step 10: `helpers.go`**

Change:

```go
// newSnapshotStore creates a snapshot.Store from the given config context.
// It wires opsDir from IssuesDir/ops, stateDir from StateDir, and
// singleBranch from the context mode.
//
// Note: the Store always derives singleBranch from ctx.Mode == "single-branch".
// Several handlers previously passed a hardcoded true to MaterializeAndReturn,
// creating an inconsistency between their read path and the write path. The Store
// corrects this by consistently reading singleBranch from ctx.Mode.
func newSnapshotStore(ctx *config.Context) *snapshot.Store {
	opsDir := filepath.Join(ctx.IssuesDir, "ops")
	stateDir := ctx.StateDir
	singleBranch := ctx.Mode == "single-branch"
	return snapshot.NewStore(opsDir, stateDir, singleBranch)
```

to:

```go
// newSnapshotStore creates a snapshot.Store from the given config context.
// It wires opsDir from IssuesDir/ops and stateDir from StateDir.
func newSnapshotStore(ctx *config.Context) *snapshot.Store {
	opsDir := filepath.Join(ctx.IssuesDir, "ops")
	stateDir := ctx.StateDir
	return snapshot.NewStore(opsDir, stateDir)
```

- [ ] **Step 11: Build (tests will still fail until Tasks 7-9 fix remaining `.Mode` references)**

Run: `go build ./... 2>&1 | grep -v "\.Mode\b"`
Expected: only errors mentioning `.Mode` remain (from `list.go`, `merged.go`, `render_context.go`, `context_history.go`, `bootstrap.go`, `hook.go` — fixed in the next tasks). No errors about `snapshot.Load`/`NewStore`/`materialize.Materialize*` arg counts.

- [ ] **Step 12: Commit**

```bash
git add cmd/armature/show.go cmd/armature/materialize.go cmd/armature/harness_context.go \
  cmd/armature/harness_hook.go cmd/armature/reparent.go cmd/armature/ready.go \
  cmd/armature/review.go cmd/armature/sync.go cmd/armature/tui.go cmd/armature/helpers.go
git commit -m "cmd: drop singleBranch argument from snapshot/materialize call sites"
```

---

### Task 7: Update `cmd/armature` behavior-bearing Mode references (list, merged, render_context, context_history)

Unlike Task 6, these sites don't just drop a parameter — they collapse an `if`/`else` behavior split down to the single surviving branch.

**Files:**
- Modify: `cmd/armature/list.go:131,139`, `cmd/armature/merged.go:125,139-141,191-197`, `cmd/armature/render_context.go:39`, `cmd/armature/context_history.go:24`
- Modify: `cmd/armature/merged_test.go` (delete the two single-branch-specific tests)

**Interfaces:**
- Consumes: `config.Context` with no `Mode` field (Task 5).

- [ ] **Step 1: `list.go` — always show the "awaiting merge" label**

Change:

```go
				for _, status := range statuses {
					label := status
					if status == ops.StatusDone && ctx.Mode != "single-branch" {
						label = "done (awaiting merge)"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", label)
					sort.Strings(groups[status])
					for _, id := range groups[status] {
						e := index[id]
						line := fmt.Sprintf("  %-12s  %s", id, e.Title)
						if status == ops.StatusDone && ctx.Mode != "single-branch" && e.Branch != "" {
```

to:

```go
				for _, status := range statuses {
					label := status
					if status == ops.StatusDone {
						label = "done (awaiting merge)"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", label)
					sort.Strings(groups[status])
					for _, id := range groups[status] {
						e := index[id]
						line := fmt.Sprintf("  %-12s  %s", id, e.Title)
						if status == ops.StatusDone && e.Branch != "" {
```

- [ ] **Step 2: `merged.go` — keep only the dual-branch (surviving) behavior**

Change:

```go
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)
			singleBranch := ctx.Mode == "single-branch"

			// Read index and issue directly from disk; no full rematerialization needed.
			store := newSnapshotStore(ctx)
			index, err := store.ReadIndex()
			if err != nil {
				return fmt.Errorf("read index: %w", err)
			}

			entry, ok := index[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found", issueID)
			}

			// Require status=done (dual-branch) or status=merged (single-branch, where done auto-advances)
			if singleBranch {
				if entry.Status != ops.StatusMerged && entry.Status != ops.StatusDone {
					return fmt.Errorf("issue %s is in status %q; arm merged in single-branch mode requires status=merged (or done)", issueID, entry.Status)
				}
			} else {
				if entry.Status != ops.StatusDone && entry.Status != ops.StatusMerged {
					return fmt.Errorf("issue %s is in status %q; arm merged requires status=done (transition it to done first)", issueID, entry.Status)
				}
			}
```

to:

```go
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)

			// Read index and issue directly from disk; no full rematerialization needed.
			store := newSnapshotStore(ctx)
			index, err := store.ReadIndex()
			if err != nil {
				return fmt.Errorf("read index: %w", err)
			}

			entry, ok := index[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found", issueID)
			}

			if entry.Status != ops.StatusDone && entry.Status != ops.StatusMerged {
				return fmt.Errorf("issue %s is in status %q; arm merged requires status=done (transition it to done first)", issueID, entry.Status)
			}
```

Further down, change:

```go
			if singleBranch {
				if entry.Status == ops.StatusMerged {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note: %s already merged. Worktree cleaned up.\n", issueID)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note: in single-branch mode, done→merged is automatic. Op recorded for %s.\n", issueID)
				}
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Marked %s as merged", issueID)
				if pr != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), " (PR #%s)", pr)
```

to:

```go
			if entry.Status == ops.StatusMerged {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note: %s already merged. Worktree cleaned up.\n", issueID)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Marked %s as merged", issueID)
				if pr != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), " (PR #%s)", pr)
```

Read `cmd/armature/merged.go` lines 190-212 after this edit to confirm the brace structure closes correctly (the surviving `else` block's closing braces must line up one level shallower than before — remove one matching `}` for the `if singleBranch { ... } else { ... }` that no longer exists after this edit, since it's now a plain `if/else`). Run `gofmt -l cmd/armature/merged.go` after editing and fix any reported formatting.

- [ ] **Step 3: `render_context.go` — worktree path is now always used**

Change:

```go
			var state *materialize.State
			if rcAt != "" {
				// Time-travel: replay ops as they existed at the given commit SHA.
				opsRepoPath := appCtx.RepoPath
				if appCtx.Mode == "dual-branch" && appCtx.WorktreePath != "" {
					opsRepoPath = appCtx.WorktreePath
				}
				gc := adapters.New(opsRepoPath)
```

to:

```go
			var state *materialize.State
			if rcAt != "" {
				// Time-travel: replay ops as they existed at the given commit SHA.
				opsRepoPath := appCtx.WorktreePath
				gc := adapters.New(opsRepoPath)
```

- [ ] **Step 4: `context_history.go` — same simplification**

Change:

```go
		RunE: func(cmd *cobra.Command, args []string) error {
			opsRepoPath := appCtx.RepoPath
			if appCtx.Mode == "dual-branch" && appCtx.WorktreePath != "" {
				opsRepoPath = appCtx.WorktreePath
			}

			gc := adapters.New(opsRepoPath)
```

to:

```go
		RunE: func(cmd *cobra.Command, args []string) error {
			gc := adapters.New(appCtx.WorktreePath)
```

- [ ] **Step 5: Delete the two single-branch-specific tests in `merged_test.go`**

Delete `TestMergedRejectsSingleBranchModeWithoutMergedStatus` (starts at the comment `// TestMergedRejectsSingleBranchModeWithoutMergedStatus verifies...`, ends at the closing `}` right before `// TestMergedRecordsOpBeforeRemovingWorktree verifies the P2 bug fix.`):

```go
// TestMergedRejectsSingleBranchModeWithoutMergedStatus verifies that merged requires
// status=merged (or status=done) in single-branch mode. Currently, in single-branch mode,
// the status guard is skipped, allowing arm merged to be called on in-progress tasks,
// which deletes the worktree and any uncommitted worker state (P2 bug).
func TestMergedRejectsSingleBranchModeWithoutMergedStatus(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap in single-branch mode (default, no --dual-branch flag)
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"})
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	// Claim the task to create a worktree
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Verify worktree exists
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	// Transition to in-progress (NOT to done)
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "in-progress"})
	require.NoError(t, transitionCmd.Execute())

	// Materialize so the in-progress status is reflected in index.json before merged reads it.
	_, errMat5 := runTrls(t, repo, "materialize")
	require.NoError(t, errMat5)

	// Call merged command — should fail because status is not merged/done in single-branch mode
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	require.Error(t, err, "merged should reject in-progress status in single-branch mode")
	assert.Contains(t, err.Error(), "status=merged", "error message should indicate merged status required")
	assert.Contains(t, err.Error(), "single-branch", "error message should mention single-branch mode")

	// Verify worktree still exists (should not be deleted on error)
	assert.DirExists(t, worktreePath, "worktree should NOT be removed when merged fails")
}
```

Note the in-progress-status rejection behavior itself must still be covered — add this replacement test in its place:

```go
// TestMergedRejectsNonDoneStatus verifies that merged requires status=done (or
// already status=merged for idempotent retries). Calling it on an in-progress
// issue must fail without touching the worktree.
func TestMergedRejectsNonDoneStatus(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, cmd.Execute())

	cmd2 := newRootCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"})
	require.NoError(t, cmd2.Execute())

	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())
	assert.DirExists(t, worktreePath, "worktree should exist after claim")

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "in-progress"})
	require.NoError(t, transitionCmd.Execute())

	_, errMat := runTrls(t, repo, "materialize")
	require.NoError(t, errMat)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01"})
	err := mergedCmd.Execute()
	require.Error(t, err, "merged should reject in-progress status")
	assert.Contains(t, err.Error(), "status=done", "error message should indicate done status required")

	assert.DirExists(t, worktreePath, "worktree should NOT be removed when merged fails")
}
```

- [ ] **Step 6: Delete `TestMergedRecordsPRInSingleBranchModeOnRetry`**

Delete (the whole function, from the `// TestMergedRecordsPRInSingleBranchModeOnRetry tests...` comment through its closing `}` right before `// TestMergedSkipsUnboundWorktree tests...`):

```go
// TestMergedRecordsPRInSingleBranchModeOnRetry tests the P2 bug fix: when a new --pr flag
// is provided with a different PR number, the merge op must be recorded even if the issue
// is already in 'merged' status. This ensures that `arm merged --issue <id> --pr 123` in
// single-branch mode captures the PR reference and doesn't silently discard it.
//
// The bug: the idempotent skip (entry.Status == ops.StatusMerged) unconditionally skips
// re-recording, so the PR field stays empty even when --pr is provided.
//
// The fix: only skip op re-recording if there is no new PR to attach OR if the issue
// already has the same PR recorded.
func TestMergedRecordsPRInSingleBranchModeOnRetry(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap in single-branch mode (default, no --dual-branch flag)
	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetOut(new(bytes.Buffer))
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, bootstrapCmd.Execute())

	workerCmd := newRootCmd()
	workerCmd.SetOut(new(bytes.Buffer))
	workerCmd.SetArgs([]string{"worker-init", "--repo", repo})
	require.NoError(t, workerCmd.Execute())

	// Create a task
	createCmd := newRootCmd()
	createCmd.SetOut(new(bytes.Buffer))
	createCmd.SetArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"})
	require.NoError(t, createCmd.Execute())

	// Materialize to initialize state
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	// Claim the task
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	// Materialize to update status
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Transition to done (auto-advances to merged in single-branch mode)
	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Completed", "--force"})
	require.NoError(t, transitionCmd.Execute())

	// Materialize to finalize the transition to merged
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Now the issue is in 'merged' status. Call merged again with a new PR.
	// The fix requires this to record the PR, not skip it.
	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "456"})
	require.NoError(t, mergedCmd.Execute())

	// Materialize to apply the new PR op
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Load the issue and verify the PR field is set
	stateDir := getTestStateDir(t, repo)
	issue, err := materialize.LoadIssue(filepath.Join(stateDir, "issues", "task-01.json"))
	require.NoError(t, err)

	assert.Equal(t, "456", issue.PR, "issue PR field should equal the new PR number provided via --pr flag")
}
```

Note the underlying P2 fix (idempotent retry re-records a new `--pr`) must still be covered — add this replacement in its place, using the still-supported explicit `bootstrap --dual-branch`-free flow (bootstrap is unconditionally the surviving mode now, so the flow is identical minus the label):

```go
// TestMergedRecordsPROnRetry tests the P2 bug fix: when a new --pr flag is
// provided with a different PR number, the merge op must be recorded even if
// the issue is already in 'merged' status.
//
// The bug: the idempotent skip (entry.Status == ops.StatusMerged) unconditionally skips
// re-recording, so the PR field stays empty even when --pr is provided.
//
// The fix: only skip op re-recording if there is no new PR to attach OR if the issue
// already has the same PR recorded.
func TestMergedRecordsPROnRetry(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetOut(new(bytes.Buffer))
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, bootstrapCmd.Execute())

	workerCmd := newRootCmd()
	workerCmd.SetOut(new(bytes.Buffer))
	workerCmd.SetArgs([]string{"worker-init", "--repo", repo})
	require.NoError(t, workerCmd.Execute())

	createCmd := newRootCmd()
	createCmd.SetOut(new(bytes.Buffer))
	createCmd.SetArgs([]string{"create", "--repo", repo, "--title", "Test task", "--type", "task", "--id", "task-01"})
	require.NoError(t, createCmd.Execute())

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	worktreePath := filepath.Join(t.TempDir(), "task-worktree")

	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "--issue", "task-01", "--worktree", worktreePath})
	require.NoError(t, claimCmd.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	transitionCmd := newRootCmd()
	transitionCmd.SetOut(new(bytes.Buffer))
	transitionCmd.SetArgs([]string{"transition", "--repo", repo, "--issue", "task-01", "--to", "done", "--outcome", "Completed", "--force"})
	require.NoError(t, transitionCmd.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	mergedCmd := newRootCmd()
	mergedCmd.SetOut(new(bytes.Buffer))
	mergedCmd.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "123"})
	require.NoError(t, mergedCmd.Execute())
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	mergedCmd2 := newRootCmd()
	mergedCmd2.SetOut(new(bytes.Buffer))
	mergedCmd2.SetArgs([]string{"merged", "--repo", repo, "--issue", "task-01", "--pr", "456"})
	require.NoError(t, mergedCmd2.Execute())

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	stateDir := getTestStateDir(t, repo)
	issue, err := materialize.LoadIssue(filepath.Join(stateDir, "issues", "task-01.json"))
	require.NoError(t, err)

	assert.Equal(t, "456", issue.PR, "issue PR field should equal the new PR number provided via --pr flag")
}
```

- [ ] **Step 7: Run cmd/armature tests scoped to merged/list**

Run: `go test ./cmd/armature/... -run 'TestMerged|TestList'`
Expected: this will still fail to compile until Tasks 8-9 remove remaining `.Mode` references elsewhere in the package — that's expected. Confirm no *new* errors are introduced by grepping the diff for typos: `gofmt -l cmd/armature/list.go cmd/armature/merged.go cmd/armature/render_context.go cmd/armature/context_history.go`.

- [ ] **Step 8: Commit**

```bash
git add cmd/armature/list.go cmd/armature/merged.go cmd/armature/render_context.go \
  cmd/armature/context_history.go cmd/armature/merged_test.go
git commit -m "cmd: collapse list/merged/render_context/context_history to dual-branch-only behavior"
```

---

### Task 8: Update `cmd/armature/hook.go` — pre-commit blocking is now unconditional

**Files:**
- Modify: `cmd/armature/hook.go`
- Modify: `cmd/armature/hook_test.go`

**Interfaces:**
- Consumes: `config.Context` with no `Mode` field (Task 5).

- [ ] **Step 1: Remove `hookIsDualBranch` and the single-branch bypass in `runPreCommitHook`**

Delete:

```go
// hookIsDualBranch reports whether the repo is in dual-branch mode.
func hookIsDualBranch(ctx *config.Context) bool {
	return ctx.Mode == "dual-branch"
}
```

Change:

```go
// runPreCommitHook implements the pre-commit hook logic natively.
// In dual-branch mode, it blocks additions/modifications to .armature/ops/ on non-_armature branches.
func runPreCommitHook(cmd *cobra.Command) error {
	appCtx := currentCtx(cmd)
	// Allow all commits on _armature branch
	branch := hookCurrentBranch(appCtx.RepoPath)
	if branch == "_armature" {
		return nil
	}

	// Single-branch mode: allow ops/ commits
	if !hookIsDualBranch(appCtx) {
		return nil
	}

	// Check for staged .armature/ops/ additions/modifications
	gitCmd := adapters.NonInteractiveGitCommand(appCtx.RepoPath, "diff", "--cached", "--name-only", "--diff-filter=AM")
	out, err := gitCmd.Output()
	if err != nil {
		// If git fails (e.g., no commits yet), allow the commit
		return nil
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(line, ".armature/ops/") {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "ERROR: Refusing to commit .armature/ops/ changes on a code branch.")
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "In dual-branch mode, ops are written directly to the _armature branch.")
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "If you are migrating to dual-branch mode, run: arm bootstrap --dual-branch")
			return fmt.Errorf("refusing to commit .armature/ops/ on branch %q in dual-branch mode", branch)
		}
	}
	return nil
}
```

to:

```go
// runPreCommitHook implements the pre-commit hook logic natively.
// It blocks additions/modifications to .armature/ops/ on non-_armature branches.
func runPreCommitHook(cmd *cobra.Command) error {
	appCtx := currentCtx(cmd)
	// Allow all commits on _armature branch
	branch := hookCurrentBranch(appCtx.RepoPath)
	if branch == "_armature" {
		return nil
	}

	// Check for staged .armature/ops/ additions/modifications
	gitCmd := adapters.NonInteractiveGitCommand(appCtx.RepoPath, "diff", "--cached", "--name-only", "--diff-filter=AM")
	out, err := gitCmd.Output()
	if err != nil {
		// If git fails (e.g., no commits yet), allow the commit
		return nil
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(line, ".armature/ops/") {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "ERROR: Refusing to commit .armature/ops/ changes on a code branch.")
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Ops are written directly to the _armature branch.")
			return fmt.Errorf("refusing to commit .armature/ops/ on branch %q", branch)
		}
	}
	return nil
}
```

- [ ] **Step 2: Update the hook doc comments in `newHookRunCmd`**

Change:

```go
Supported hooks:
  pre-commit          Block .armature/ops/ commits on code branches in dual-branch mode
  post-commit         Send heartbeat for active claim; push ops in dual-branch mode
```

to:

```go
Supported hooks:
  pre-commit          Block .armature/ops/ commits on code branches
  post-commit         Send heartbeat for active claim; push ops
```

- [ ] **Step 3: Update the `runPostCommitHook` doc comment**

Change:

```go
// Sends a heartbeat for any active claim and, in dual-branch mode, pushes ops.
func runPostCommitHook(cmd *cobra.Command) error {
```

to:

```go
// Sends a heartbeat for any active claim and pushes ops.
func runPostCommitHook(cmd *cobra.Command) error {
```

- [ ] **Step 4: Update `hook_test.go` — remove `Mode:` field from context literals**

Remove `Mode: "single-branch",` from both `config.Context{...}` literals (in `TestHookFindActiveClaimID_UsesLatestHeartbeat` and `TestHookFindActiveClaimID_IgnoresDoneTransitions`):

```go
	ctx := &config.Context{
		RepoPath:  repo,
		IssuesDir: issuesDir,
		Mode:      "single-branch",
		Config:    config.Config{DefaultTTL: 60},
	}
```

becomes (both occurrences):

```go
	ctx := &config.Context{
		RepoPath:  repo,
		IssuesDir: issuesDir,
		Config:    config.Config{DefaultTTL: 60},
	}
```

- [ ] **Step 5: Rename and fix `TestHookRunPreCommit_SingleBranch`**

This test never stages an `.armature/ops/` file, so it was passing trivially in both modes — rename it to reflect what it actually verifies:

Change:

```go
func TestHookRunPreCommit_SingleBranch(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Single-branch mode — pre-commit should always allow ops commits
	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	require.NoError(t, err)
}
```

to:

```go
func TestHookRunPreCommit_AllowsCommitWithNoOpsChanges(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	require.NoError(t, err)
}
```

- [ ] **Step 6: Add a test for the now-unconditional ops/ blocking on code branches**

This behavior was previously gated by dual-branch mode and had no test that actually staged an ops/ file and expected a block. Add, right after the renamed test from Step 5:

```go
// TestHookRunPreCommit_BlocksStagedOpsFile verifies that pre-commit refuses to
// commit .armature/ops/ changes on a code branch (not _armature).
func TestHookRunPreCommit_BlocksStagedOpsFile(t *testing.T) {
	repo := setupRepoWithTask(t)

	opsFile := filepath.Join(repo, ".armature", "ops", "fake-worker.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(opsFile), 0o755))
	require.NoError(t, os.WriteFile(opsFile, []byte(`{"type":"create"}`+"\n"), 0o644))
	run(t, repo, "git", "add", filepath.Join(".armature", "ops", "fake-worker.log"))

	_, err := runTrls(t, repo, "hook", "run", "pre-commit")
	require.Error(t, err, "pre-commit should block staged .armature/ops/ changes on a code branch")
	assert.Contains(t, err.Error(), ".armature/ops/")
}
```

- [ ] **Step 7: Run the hook tests**

Run: `go test ./cmd/armature/... -run TestHook`
Expected: still fails to compile until Task 9 finishes removing the last `.Mode` references in `bootstrap.go`/`cmd_extra_test.go`/`main_test.go`/`review_test.go` — that's expected at this point. Confirm with `gofmt -l cmd/armature/hook.go cmd/armature/hook_test.go` that formatting is clean.

- [ ] **Step 8: Commit**

```bash
git add cmd/armature/hook.go cmd/armature/hook_test.go
git commit -m "cmd/hook: make pre-commit ops/ blocking unconditional"
```

---

### Task 9: Rewrite `cmd/armature/bootstrap.go` — remove `--dual-branch` flag, add forced legacy migration

This is the largest remaining task: `runRepoSetup` always creates the `_armature` branch and worktree (no flag, no branching), and gains a migration step that detects a pre-existing single-branch layout (`.armature/ops/*.log` directly under the repo root, from before this change) and moves it into the new worktree layout, per ADR 0006.

**Files:**
- Modify: `cmd/armature/bootstrap.go`
- Modify: `cmd/armature/bootstrap_test.go`

**Interfaces:**
- Produces: `runRepoSetup(cmd *cobra.Command, repoPath string) (RepoSetupResult, error)` — drops the `dualBranch bool` parameter entirely. `runRepoSetupWithFormat(cmd *cobra.Command, repoPath string, format string) (RepoSetupResult, error)` — same. New: `migrateLegacySingleBranchOps(repoPath, worktreePath string) (migrated bool, migratedDir string, err error)`.
- Consumes: `config.DefaultConfig` with no `Mode` field (Task 5).

- [ ] **Step 1: Remove the `--dual-branch` flag and simplify `runRepoSetupWithFormat`/`newBootstrapCmd`**

Change:

```go
// runRepoSetupWithFormat calls runRepoSetup, silencing human output when format is "json".
func runRepoSetupWithFormat(cmd *cobra.Command, repoPath string, dualBranch bool, format string) (RepoSetupResult, error) {
	if format == "json" || format == "agent" {
		silentCmd := &cobra.Command{}
		silentCmd.SetOut(io.Discard)
		return runRepoSetup(silentCmd, repoPath, dualBranch)
	}
	return runRepoSetup(cmd, repoPath, dualBranch)
}
```

to:

```go
// runRepoSetupWithFormat calls runRepoSetup, silencing human output when format is "json".
func runRepoSetupWithFormat(cmd *cobra.Command, repoPath string, format string) (RepoSetupResult, error) {
	if format == "json" || format == "agent" {
		silentCmd := &cobra.Command{}
		silentCmd.SetOut(io.Discard)
		return runRepoSetup(silentCmd, repoPath)
	}
	return runRepoSetup(cmd, repoPath)
}
```

In `newBootstrapCmd`, remove the `var dualBranch bool` declaration, change:

```go
	cmd := &cobra.Command{
		Use:   "bootstrap [--global] [--with-hooks] [--dual-branch] [--platform <name>]",
		Short: "Bootstrap Armature: initialize repo and deploy harness artifacts",
		Long: `Initialize a repository for Armature coordination and optionally deploy harness artifacts
(skills, plugin metadata, harness hook configs).

By default, artifacts deploy to .claude/ (local). Use --global to deploy to ~/.claude/ instead.
Use --dual-branch to initialize in dual-branch mode (issues stored on separate _armature branch).
Use --with-hooks to also write harness hook configuration (both require --platform support).
Use --platform to restrict bootstrap to specific platforms (can be repeated); default is all verified platforms.

The command is idempotent: running it multiple times has the same effect as running it once.`,
```

to:

```go
	cmd := &cobra.Command{
		Use:   "bootstrap [--global] [--with-hooks] [--platform <name>]",
		Short: "Bootstrap Armature: initialize repo and deploy harness artifacts",
		Long: `Initialize a repository for Armature coordination and optionally deploy harness artifacts
(skills, plugin metadata, harness hook configs).

By default, artifacts deploy to .claude/ (local). Use --global to deploy to ~/.claude/ instead.
Issues are stored on a dedicated _armature branch, accessed through the .arm/ ops worktree.
Use --with-hooks to also write harness hook configuration (both require --platform support).
Use --platform to restrict bootstrap to specific platforms (can be repeated); default is all verified platforms.

The command is idempotent: running it multiple times has the same effect as running it once.
If the repository has a pre-existing single-branch layout (.armature/ committed directly
to a code branch, from before dual-branch became the only supported mode), bootstrap
migrates it automatically into the .arm/ worktree.`,
```

Change:

```go
			repoSetupResult, err := runRepoSetupWithFormat(cmd, repoPath, dualBranch, format)
```

to:

```go
			repoSetupResult, err := runRepoSetupWithFormat(cmd, repoPath, format)
```

Delete this flag registration:

```go
	cmd.Flags().BoolVar(&dualBranch, "dual-branch", false, "initialize in dual-branch mode (issues stored on separate _armature branch)")
```

- [ ] **Step 2: Update the hook shell templates to drop the mode-conditional bits**

Change `postCommitHookTemplate`:

```go
const postCommitHookTemplate = `#!/bin/sh
# armature:managed
# Armature post-commit hook: emit heartbeat and push ops in dual-branch mode.
# Branch-aware: skips on _armature since ops are committed directly there.
# To activate: cp this file to .git/hooks/post-commit && chmod +x .git/hooks/post-commit

# Skip on _armature branch where ops logs are committed directly
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

# Send heartbeat for active claim (if any)
arm heartbeat 2>/dev/null

# In dual-branch mode, push ops logs after each commit
if grep -q '"mode".*"dual-branch"' .armature/config.json 2>/dev/null; then
  arm push-ops 2>/dev/null
fi
`
```

to:

```go
const postCommitHookTemplate = `#!/bin/sh
# armature:managed
# Armature post-commit hook: emit heartbeat and push ops.
# Branch-aware: skips on _armature since ops are committed directly there.
# To activate: cp this file to .git/hooks/post-commit && chmod +x .git/hooks/post-commit

# Skip on _armature branch where ops logs are committed directly
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

# Send heartbeat for active claim (if any)
arm heartbeat 2>/dev/null

# Push ops logs after each commit
arm push-ops 2>/dev/null
`
```

Change `preCommitHookTemplate`:

```go
const preCommitHookTemplate = `#!/bin/sh
# armature:managed
# Armature pre-commit hook: block ops log commits on code branches in dual-branch mode.
# In dual-branch mode, ops live on _armature — never on a code branch.
# To activate: cp this file to .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit
#
# This is defense-in-depth; .armature/.gitignore also blocks ops/ from being staged.

# Allow commits on _armature — that's exactly where ops belong.
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

# Only block in dual-branch mode; check if config says dual-branch
if ! grep -q '"mode".*"dual-branch"' .armature/config.json 2>/dev/null; then
  # Single-branch mode allows ops/ commits
  exit 0
fi

# Only block additions/modifications — deletions are allowed (cleanup commits).
if git diff --cached --name-only --diff-filter=AM | grep -q '\.armature/ops/'; then
  echo "ERROR: Refusing to commit .armature/ops/ changes on a code branch."
  echo "In dual-branch mode, ops are written directly to the _armature branch."
  echo "If you are migrating to dual-branch mode, run: arm bootstrap --dual-branch"
  exit 1
fi
`
```

to:

```go
const preCommitHookTemplate = `#!/bin/sh
# armature:managed
# Armature pre-commit hook: block ops log commits on code branches.
# Ops live on _armature — never on a code branch.
# To activate: cp this file to .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit
#
# This is defense-in-depth; .armature/.gitignore also blocks ops/ from being staged.

# Allow commits on _armature — that's exactly where ops belong.
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

# Only block additions/modifications — deletions are allowed (cleanup commits).
if git diff --cached --name-only --diff-filter=AM | grep -q '\.armature/ops/'; then
  echo "ERROR: Refusing to commit .armature/ops/ changes on a code branch."
  echo "Ops are written directly to the _armature branch."
  exit 1
fi
`
```

Update the `installHooks` doc comment:

```go
// installHooks copies hook templates from .armature/hooks/ to .git/hooks/ and makes them executable.
// Existing hooks are skipped (and returned as skipped) only if they lack both the "# armature:managed"
// marker and the legacy Armature signature (#!/bin/sh shebang + "# Armature " header); legacy hooks
// are migrated/overwritten. Returns a list of skipped hook names.
// In dual-branch mode, the templates are in the worktree's .armature/hooks/.
func installHooks(repoPath string, issuesDir string) ([]string, error) {
```

to:

```go
// installHooks copies hook templates from .armature/hooks/ to .git/hooks/ and makes them executable.
// Existing hooks are skipped (and returned as skipped) only if they lack both the "# armature:managed"
// marker and the legacy Armature signature (#!/bin/sh shebang + "# Armature " header); legacy hooks
// are migrated/overwritten. Returns a list of skipped hook names.
// The templates live in the worktree's .armature/hooks/.
func installHooks(repoPath string, issuesDir string) ([]string, error) {
```

- [ ] **Step 3: Add the legacy migration function**

Add this new function right before `runRepoSetup`:

```go
// migrateLegacySingleBranchOps detects a pre-existing single-branch layout
// (.armature/ops/*.log committed directly under repoPath, from before dual-branch
// became the only supported mode — see ADR 0006) and copies any op logs found
// there into the new worktree's .armature/ops/ directory. The legacy directory
// is renamed (not deleted) so no history is lost; ops are the append-only
// source of truth, so preserving the originals is the safe default.
// Returns whether a legacy layout was found and migrated, and the path the
// legacy directory was renamed to.
func migrateLegacySingleBranchOps(repoPath, worktreePath string) (migrated bool, migratedDir string, err error) {
	legacyDir := filepath.Join(repoPath, ".armature")
	legacyOpsDir := filepath.Join(legacyDir, "ops")

	entries, err := os.ReadDir(legacyOpsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("read legacy ops dir: %w", err)
	}

	var logFiles []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			logFiles = append(logFiles, e)
		}
	}
	if len(logFiles) == 0 {
		return false, "", nil
	}

	newOpsDir := filepath.Join(worktreePath, ".armature", "ops")
	if err := os.MkdirAll(newOpsDir, 0o750); err != nil {
		return false, "", fmt.Errorf("create worktree ops dir: %w", err)
	}

	for _, e := range logFiles {
		src := filepath.Join(legacyOpsDir, e.Name())
		dst := filepath.Join(newOpsDir, e.Name())
		if _, statErr := os.Stat(dst); statErr == nil {
			// Already migrated on a prior run — don't overwrite.
			continue
		}
		content, readErr := os.ReadFile(src) //nolint:gosec // path built from internal legacy ops dir
		if readErr != nil {
			return false, "", fmt.Errorf("read legacy log %s: %w", e.Name(), readErr)
		}
		if writeErr := os.WriteFile(dst, content, 0o600); writeErr != nil {
			return false, "", fmt.Errorf("write migrated log %s: %w", e.Name(), writeErr)
		}
	}

	migratedDir = fmt.Sprintf("%s.migrated-%d", legacyDir, time.Now().Unix())
	if err := os.Rename(legacyDir, migratedDir); err != nil {
		return false, "", fmt.Errorf("rename legacy .armature dir: %w", err)
	}

	return true, migratedDir, nil
}
```

Add `"time"` to the import block at the top of `bootstrap.go`:

```go
import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)
```

becomes:

```go
import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 4: Rewrite `runRepoSetup` to always set up dual-branch layout unconditionally, and call the migration**

Change:

```go
// runRepoSetup initializes the repository structure for Armature.
// Returns RepoSetupResult with status and any skipped hooks.
func runRepoSetup(cmd *cobra.Command, repoPath string, dualBranch bool) (RepoSetupResult, error) {
	// Resolve repoPath to an absolute path so stored paths are never relative.
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("resolve repo path: %w", err)
	}
	repoPath = absRepoPath

	gitClient := adapters.New(repoPath)

	// Detect existing dual-branch mode from git config.
	// If the repo was already initialized with dual-branch mode, re-running bootstrap
	// (even without --dual-branch) should preserve the existing mode.
	if !dualBranch {
		existingMode, err := gitClient.ReadGitConfig("armature.mode")
		if err == nil && existingMode == "dual-branch" {
			dualBranch = true
		}
	}

	var issuesDir string
	if dualBranch {
		// Create orphan branch _armature (idempotent)
		if err := gitClient.CreateOrphanBranch("_armature"); err != nil {
			return RepoSetupResult{}, fmt.Errorf("create _armature branch: %w", err)
		}

		// Create .arm/ worktree (idempotent)
		worktreePath := filepath.Join(repoPath, ".arm")
		if err := gitClient.AddWorktree("_armature", worktreePath); err != nil {
			return RepoSetupResult{}, fmt.Errorf("add .arm worktree: %w", err)
		}

		// Set git config keys
		if err := gitClient.SetGitConfig("armature.mode", "dual-branch"); err != nil {
			return RepoSetupResult{}, fmt.Errorf("set armature.mode: %w", err)
		}
		if err := gitClient.SetGitConfig("armature.ops-worktree-path", worktreePath); err != nil {
			return RepoSetupResult{}, fmt.Errorf("set armature.ops-worktree-path: %w", err)
		}

		issuesDir = filepath.Join(worktreePath, ".armature")
	} else {
		issuesDir = filepath.Join(repoPath, ".armature")
	}
```

to:

```go
// runRepoSetup initializes the repository structure for Armature.
// Returns RepoSetupResult with status and any skipped hooks.
func runRepoSetup(cmd *cobra.Command, repoPath string) (RepoSetupResult, error) {
	// Resolve repoPath to an absolute path so stored paths are never relative.
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("resolve repo path: %w", err)
	}
	repoPath = absRepoPath

	gitClient := adapters.New(repoPath)

	// Create orphan branch _armature (idempotent)
	if err := gitClient.CreateOrphanBranch("_armature"); err != nil {
		return RepoSetupResult{}, fmt.Errorf("create _armature branch: %w", err)
	}

	// Create .arm/ worktree (idempotent)
	worktreePath := filepath.Join(repoPath, ".arm")
	if err := gitClient.AddWorktree("_armature", worktreePath); err != nil {
		return RepoSetupResult{}, fmt.Errorf("add .arm worktree: %w", err)
	}

	// Set git config key
	if err := gitClient.SetGitConfig("armature.ops-worktree-path", worktreePath); err != nil {
		return RepoSetupResult{}, fmt.Errorf("set armature.ops-worktree-path: %w", err)
	}

	issuesDir := filepath.Join(worktreePath, ".armature")

	// Migrate a pre-existing single-branch layout (.armature/ committed to a code
	// branch, from before dual-branch became the only supported mode) if present.
	migrated, migratedDir, err := migrateLegacySingleBranchOps(repoPath, worktreePath)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("migrate legacy single-branch layout: %w", err)
	}
```

- [ ] **Step 5: Remove the mode-dependent config write and status message**

Change:

```go
	// Detect project type and write config
	configPath := filepath.Join(issuesDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		projectType := config.DetectProjectType(repoPath)
		cfg := config.DefaultConfig(projectType)
		if dualBranch {
			cfg.Mode = "dual-branch"
		}
		if err := config.WriteConfig(configPath, cfg); err != nil {
			return RepoSetupResult{}, fmt.Errorf("write config: %w", err)
		}
	}

	// Init worker if not already configured
	if ok, _ := worker.CheckWorkerID(repoPath); !ok {
		if _, err := worker.InitWorker(repoPath); err != nil {
			return RepoSetupResult{}, fmt.Errorf("init worker: %w", err)
		}
	}

	mode := "single-branch"
	if dualBranch {
		mode = "dual-branch"
	}

	var status string
	if freshInit {
		status = "initialized"
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized Armature in %s mode at %s\n", mode, issuesDir)
	} else {
		status = "already_initialized"
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Armature already initialized in %s mode at %s\n", mode, issuesDir)
	}

	result := RepoSetupResult{
		Status:       status,
		SkippedHooks: skippedHooks,
	}
	return result, nil
}
```

to:

```go
	// Detect project type and write config
	configPath := filepath.Join(issuesDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		projectType := config.DetectProjectType(repoPath)
		cfg := config.DefaultConfig(projectType)
		if err := config.WriteConfig(configPath, cfg); err != nil {
			return RepoSetupResult{}, fmt.Errorf("write config: %w", err)
		}
	}

	// Init worker if not already configured
	if ok, _ := worker.CheckWorkerID(repoPath); !ok {
		if _, err := worker.InitWorker(repoPath); err != nil {
			return RepoSetupResult{}, fmt.Errorf("init worker: %w", err)
		}
	}

	var status string
	if freshInit {
		status = "initialized"
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized Armature at %s\n", issuesDir)
	} else {
		status = "already_initialized"
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Armature already initialized at %s\n", issuesDir)
	}
	if migrated {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Migrated legacy single-branch ops into %s (old directory preserved at %s)\n", issuesDir, migratedDir)
	}

	result := RepoSetupResult{
		Status:       status,
		SkippedHooks: skippedHooks,
	}
	return result, nil
}
```

- [ ] **Step 6: Rewrite `bootstrap_test.go` call sites and mode-specific tests**

Run this to drop the now-removed boolean argument from every `runRepoSetup(...)` call in the test file:

```bash
sed -i -E 's/runRepoSetup\(([a-zA-Z0-9_]+), (repo|repo2)?, ?(false|true)\)/runRepoSetup(\1, \2)/' cmd/armature/bootstrap_test.go
```

Verify every call site was caught:

```bash
grep -n "runRepoSetup(" cmd/armature/bootstrap_test.go
```

Expected: every call now has exactly two arguments (`cmd`/`cmd2`, `repo`). If the sed pattern missed any (e.g. different variable names), fix those individually by removing the trailing `, false` or `, true`.

Replace `TestRunRepoSetupDualBranchCreatesWorktree` (it asserted `"mode": "dual-branch"` in config.json, which no longer exists as a field):

```go
// TestRunRepoSetupDualBranchCreatesWorktree verifies that runRepoSetup creates a .arm worktree in dual-branch mode.
func TestRunRepoSetupDualBranchCreatesWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo, true)
	require.NoError(t, err)

	// Verify worktree exists at .arm/
	assert.DirExists(t, filepath.Join(repo, ".arm"))

	// Verify .armature/ is inside the worktree
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"))

	// Verify config is in dual-branch mode
	configPath := filepath.Join(repo, ".arm", ".armature", "config.json")
	content, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), `"mode": "dual-branch"`)
}
```

with:

```go
// TestRunRepoSetupCreatesWorktree verifies that runRepoSetup always creates a .arm worktree.
func TestRunRepoSetupCreatesWorktree(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify worktree exists at .arm/
	assert.DirExists(t, filepath.Join(repo, ".arm"))

	// Verify .armature/ is inside the worktree
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"))

	// Verify config.json was written inside the worktree
	configPath := filepath.Join(repo, ".arm", ".armature", "config.json")
	assert.FileExists(t, configPath)

	// The code repo itself must never have .armature/
	assert.False(t, pathExists(filepath.Join(repo, ".armature")),
		"code repo should never have .armature/ — issues live in the worktree")
}
```

Replace `TestRunRepoSetupDetectsExistingDualBranchMode` (its whole premise — re-running with `dualBranch=false` after `dualBranch=true` — no longer exists, since there's only one mode) with a migration test:

```go
// TestRunRepoSetupDetectsExistingDualBranchMode verifies that when re-running bootstrap with
// dualBranch=false on a repo that was originally initialized with dualBranch=true,
// the second run detects the existing dual-branch mode from git config and uses
// the existing .arm worktree instead of creating .armature/ in the code repo.
func TestRunRepoSetupDetectsExistingDualBranchMode(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	// First run: initialize with dual-branch mode
	_, err := runRepoSetup(cmd, repo, true)
	require.NoError(t, err)

	// Verify dual-branch mode was set
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should exist after first run")

	// Second run: call with dualBranch=false (simulating `arm bootstrap` without --dual-branch flag)
	cmd2 := newRootCmd()
	cmd2.SetOut(new(strings.Builder))
	_, err = runRepoSetup(cmd2, repo, false)
	require.NoError(t, err)

	// Verify the second run still uses .arm worktree (detected from git config)
	// not .armature/ in the code repo
	assert.DirExists(t, filepath.Join(repo, ".arm"), ".arm worktree should still exist")
	assert.DirExists(t, filepath.Join(repo, ".arm", ".armature"), ".armature should be in worktree")

	// The code repo should NOT have .armature/ directory
	assert.False(t, pathExists(filepath.Join(repo, ".armature")),
		"code repo should not have .armature/ when re-running with existing dual-branch mode")
}
```

with:

```go
// TestRunRepoSetupMigratesLegacySingleBranchLayout verifies that bootstrap detects a
// pre-existing single-branch layout (.armature/ops/*.log committed directly at the
// repo root, from before dual-branch became the only mode — ADR 0006) and migrates
// the op logs into the new .arm/ worktree, preserving the original directory under
// a .armature.migrated-<timestamp> name.
func TestRunRepoSetupMigratesLegacySingleBranchLayout(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Simulate a pre-existing single-branch clone: .armature/ops/<worker>.log at repo root.
	legacyOpsDir := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsDir, 0o750))
	legacyLogPath := filepath.Join(legacyOpsDir, "worker-1.log")
	legacyLogContent := `{"type":"create","target_id":"task-01","timestamp":100,"worker_id":"worker-1","payload":{"title":"Legacy task","node_type":"task"}}` + "\n"
	require.NoError(t, os.WriteFile(legacyLogPath, []byte(legacyLogContent), 0o600))

	buf := new(strings.Builder)
	cmd := newRootCmd()
	cmd.SetOut(buf)

	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	// Verify the op log was migrated into the worktree.
	migratedLogPath := filepath.Join(repo, ".arm", ".armature", "ops", "worker-1.log")
	migratedContent, readErr := os.ReadFile(migratedLogPath)
	require.NoError(t, readErr)
	assert.Equal(t, legacyLogContent, string(migratedContent))

	// Verify the legacy directory was renamed, not left in place or deleted.
	assert.False(t, pathExists(filepath.Join(repo, ".armature")),
		"legacy .armature/ should be renamed away, not left in place")
	matches, globErr := filepath.Glob(filepath.Join(repo, ".armature.migrated-*"))
	require.NoError(t, globErr)
	assert.Len(t, matches, 1, "exactly one .armature.migrated-* directory should exist")

	assert.Contains(t, buf.String(), "Migrated legacy single-branch ops")
}

// TestRunRepoSetupMigrationIsIdempotent verifies that re-running bootstrap after a
// migration does not fail or duplicate migrated directories.
func TestRunRepoSetupMigrationIsIdempotent(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	legacyOpsDir := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsDir, "worker-1.log"),
		[]byte(`{"type":"create","target_id":"task-01","timestamp":100,"worker_id":"worker-1","payload":{"title":"Legacy task","node_type":"task"}}`+"\n"), 0o600))

	cmd := newRootCmd()
	cmd.SetOut(new(strings.Builder))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	cmd2 := newRootCmd()
	cmd2.SetOut(new(strings.Builder))
	_, err = runRepoSetup(cmd2, repo)
	require.NoError(t, err, "second bootstrap run must succeed with no legacy directory left to migrate")

	matches, globErr := filepath.Glob(filepath.Join(repo, ".armature.migrated-*"))
	require.NoError(t, globErr)
	assert.Len(t, matches, 1, "second run must not create another migrated directory")
}
```

Remove the now-stale comment/context on the two calls at lines 315 and 334 that referenced `dualBranch` explicitly (`_, err := runRepoSetup(cmd, repo, false)` / `true` — already handled by the `sed` in this step).

- [ ] **Step 7: Run the bootstrap tests**

Run: `go test ./cmd/armature/... -run TestRunRepoSetup`
Expected: PASS, including the two new migration tests.

- [ ] **Step 8: Full package build check**

Run: `go build ./... 2>&1 | grep "\.Mode\b"`
Expected: only `cmd/armature/cmd_extra_test.go`, `cmd/armature/main_test.go`, `cmd/armature/review_test.go`, and `cmd/armature/claim_test.go` should still reference `.Mode` or `--dual-branch` — fixed in Task 10.

- [ ] **Step 9: Commit**

```bash
git add cmd/armature/bootstrap.go cmd/armature/bootstrap_test.go
git commit -m "bootstrap: remove --dual-branch flag, always init worktree, migrate legacy layout"
```

---

### Task 10: Clean up remaining test references (`cmd_extra_test.go`, `main_test.go`, `review_test.go`, `claim_test.go`)

**Files:**
- Modify: `cmd/armature/cmd_extra_test.go`
- Modify: `cmd/armature/main_test.go`
- Modify: `cmd/armature/review_test.go`
- Modify: `cmd/armature/claim_test.go`

**Interfaces:**
- Consumes: `config.Context` with no `Mode` field, `runRepoSetup`/bootstrap with no `--dual-branch` flag (Tasks 5, 9).

- [ ] **Step 1: Rewrite `TestNewSnapshotStore_UsesContextPaths_REQ_ARCHIMP_S14_T2` in `cmd_extra_test.go`**

Change:

```go
// TestNewSnapshotStore_UsesContextPaths verifies that newSnapshotStore wires
// opsDir from IssuesDir/ops, stateDir from StateDir, and singleBranch from Mode.
func TestNewSnapshotStore_UsesContextPaths_REQ_ARCHIMP_S14_T2(t *testing.T) {
	t.Parallel()

	// Single-branch mode
	ctx := &config.Context{
		IssuesDir: "/repo/.armature",
		StateDir:  "/repo/.armature/state/worker-1",
		Mode:      "single-branch",
	}
	store := newSnapshotStore(ctx)
	require.NotNil(t, store)

	// Verify stateDir is wired correctly by checking IndexPath
	expectedIndexPath := filepath.Join(ctx.StateDir, "index.json")
	assert.Equal(t, expectedIndexPath, store.IndexPath())

	// Verify IssuePath also uses StateDir
	expectedIssuePath := filepath.Join(ctx.StateDir, "issues", "test-id.json")
	assert.Equal(t, expectedIssuePath, store.IssuePath("test-id"))

	// Dual-branch mode (Mode != "single-branch")
	ctx2 := &config.Context{
		IssuesDir: "/repo/.arm/.armature",
		StateDir:  "/repo/.arm/state/worker-1",
		Mode:      "dual-branch",
	}
	store2 := newSnapshotStore(ctx2)
	require.NotNil(t, store2)

	// Verify paths for dual-branch
	expectedIndexPath2 := filepath.Join(ctx2.StateDir, "index.json")
	assert.Equal(t, expectedIndexPath2, store2.IndexPath())

	expectedIssuePath2 := filepath.Join(ctx2.StateDir, "issues", "test-id.json")
	assert.Equal(t, expectedIssuePath2, store2.IssuePath("test-id"))
}
```

to:

```go
// TestNewSnapshotStore_UsesContextPaths verifies that newSnapshotStore wires
// opsDir from IssuesDir/ops and stateDir from StateDir.
func TestNewSnapshotStore_UsesContextPaths_REQ_ARCHIMP_S14_T2(t *testing.T) {
	t.Parallel()

	ctx := &config.Context{
		IssuesDir: "/repo/.arm/.armature",
		StateDir:  "/repo/.arm/state/worker-1",
	}
	store := newSnapshotStore(ctx)
	require.NotNil(t, store)

	// Verify stateDir is wired correctly by checking IndexPath
	expectedIndexPath := filepath.Join(ctx.StateDir, "index.json")
	assert.Equal(t, expectedIndexPath, store.IndexPath())

	// Verify IssuePath also uses StateDir
	expectedIssuePath := filepath.Join(ctx.StateDir, "issues", "test-id.json")
	assert.Equal(t, expectedIssuePath, store.IssuePath("test-id"))
}
```

- [ ] **Step 2: Strip the `--dual-branch` flag from every remaining test invocation**

Run:

```bash
grep -rln -- '"--dual-branch"' cmd/armature/*_test.go
```

For each matched file, run:

```bash
sed -i -E 's/, "--dual-branch"//g; s/"--dual-branch", //g' cmd/armature/main_test.go cmd/armature/merged_test.go cmd/armature/claim_test.go cmd/armature/review_test.go
```

Verify no occurrences remain:

```bash
grep -rn -- '"--dual-branch"' cmd/armature/*_test.go
```

Expected: no output.

- [ ] **Step 3: Delete `TestInitCommand_SingleBranch` from `main_test.go`**

Delete:

```go
func TestInitCommand_SingleBranch(t *testing.T) {
	repo := initTempRepo(t)
	// Create an initial commit so git is fully initialized
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bootstrap", "--repo", repo, "--format", "human"})

	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "single-branch")

	// Verify .armature directory structure was created
	assert.DirExists(t, filepath.Join(repo, ".armature"))
	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"))
	assert.DirExists(t, filepath.Join(repo, ".armature", "state"))
	assert.FileExists(t, filepath.Join(repo, ".armature", "config.json"))
	assert.FileExists(t, filepath.Join(repo, ".armature", "ops", "SCHEMA"))
}
```

This scenario (bootstrap always creating the worktree layout) is already covered by `TestRunRepoSetupCreatesWorktree` in `bootstrap_test.go` (Task 9) — no replacement needed here.

- [ ] **Step 4: Rename `TestMaterialize_SingleBranchMode_AfterModeRefactor`**

Change:

```go
func TestMaterialize_SingleBranchMode_AfterModeRefactor(t *testing.T) {
```

to:

```go
func TestMaterialize_AfterBootstrap(t *testing.T) {
```

(body is unchanged — it only verifies `arm materialize` succeeds after `arm bootstrap`, which is mode-agnostic).

- [ ] **Step 5: Delete `TestMerged_AcceptsDoneIssue_SingleBranch`**

Delete (it's redundant with `TestMerged_AcceptsDoneIssue_DualBranch`, which covers the same scenario under the surviving behavior):

```go
func TestMerged_AcceptsDoneIssue_SingleBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "my task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001", "--worktree", filepath.Join(t.TempDir(), "claim-worktree-wt"))
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done", "--force", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// In single-branch mode, merged accepts status=done (which auto-advances to merged)
	out, err := runTrls(t, repo, "merged", "--issue", "T-001", "--pr", "123")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
}
```

- [ ] **Step 6: Fix the comment in `TestMerged_RequiresDoneState_InDualBranchMode`**

Change:

```go
	// Try to merge an open issue in dual-branch mode — should fail with clear error
	_, err = runTrls(t, repo, "merged", "--issue", "T-001")
```

to:

```go
	// Try to merge an open issue — should fail with clear error
	_, err = runTrls(t, repo, "merged", "--issue", "T-001")
```

- [ ] **Step 7: Fix the stray comments referencing `--dual-branch` flag usage that the sed in Step 2 didn't need to touch (comments only, no code)**

Run:

```bash
grep -n "dual-branch\|single-branch" cmd/armature/main_test.go
```

For each comment-only match (not inside a `SetArgs`/`runTrls` call — those were already fixed by Step 2's sed), reword to drop the now-meaningless mode qualifier. For example, change:

```go
	// Full workflow: init --dual-branch → create → claim → in-progress → done →
```

to:

```go
	// Full workflow: init → create → claim → in-progress → done →
```

and:

```go
	// First init --dual-branch should succeed
```

to:

```go
	// First init should succeed
```

and:

```go
	// Second init --dual-branch should also succeed (idempotent)
```

to:

```go
	// Second init should also succeed (idempotent)
```

and:

```go
	// Re-init using "." as repo path — simulates running trls init --dual-branch in the repo root
```

to:

```go
	// Re-init using "." as repo path — simulates running trls bootstrap in the repo root
```

- [ ] **Step 8: Rename `TestReview_SingleBranchLifecycle` in `review_test.go`**

Change:

```go
func TestReview_SingleBranchLifecycle(t *testing.T) {
```

to:

```go
func TestReview_Lifecycle(t *testing.T) {
```

(body unchanged — it exercises the general review lifecycle and never asserted mode-specific behavior).

- [ ] **Step 9: Run the full `cmd/armature` test suite**

Run: `go test ./cmd/armature/...`
Expected: PASS.

- [ ] **Step 10: Run the full repo test suite**

Run: `go build ./... && go test ./...`
Expected: PASS with no compile errors anywhere in the module.

- [ ] **Step 11: Commit**

```bash
git add cmd/armature/cmd_extra_test.go cmd/armature/main_test.go cmd/armature/review_test.go cmd/armature/claim_test.go
git commit -m "cmd: finish removing single-branch/dual-branch references from tests"
```

---

### Task 11: Update living reference docs

Historical docs under `docs/superpowers/plans/` and `docs/superpowers/specs/` are explicitly out of scope (they're records of past decisions, not living documentation) — do not touch them in this task.

**Files:**
- Modify: `README.md`
- Modify: `docs/commands.md`
- Modify: `docs/configuration.md`
- Modify: `docs/concepts.md`
- Modify: `docs/getting-started.md`
- Modify: `docs/design/architecture.md`
- Modify: `docs/design/gap-resolutions.md`
- Modify: `docs/design/roles.md`
- Modify: `docs/use-cases.md`
- Modify: `docs/harness-hook.md`
- Modify: `docs/design/dogfooding-learnings.md` (if it exists — verify with `ls docs/design/dogfooding-learnings.md`)

**Interfaces:** None — documentation only, no code interfaces.

- [ ] **Step 1: Find every remaining mention**

Run:

```bash
grep -rn "single-branch\|single branch\|dual-branch\|dual branch\|Single-Branch\|Dual-Branch" \
  README.md docs/commands.md docs/configuration.md docs/concepts.md docs/getting-started.md \
  docs/design/architecture.md docs/design/gap-resolutions.md docs/design/roles.md \
  docs/use-cases.md docs/harness-hook.md docs/design/dogfooding-learnings.md 2>/dev/null
```

Work through the output file by file. For each match, apply the following rule: describe the system as always operating with the `_armature` ops branch and `.arm/` ops worktree — drop any "mode" framing, any `--dual-branch` flag mention (the flag no longer exists per Task 9), and any single-branch alternative.

- [ ] **Step 2: `README.md`**

Find the line (originally around line 88):

```
Armature will detect if your repository has branch protection and set up either a dual-branch (`_armature` orphan branch) or single-branch mode accordingly.
```

Replace with:

```
Armature initializes with coordination data on the `_armature` orphan branch, accessed through the `.arm/` ops worktree.
```

- [ ] **Step 3: `docs/commands.md`**

Remove the `--dual-branch` flag from the `bootstrap` command's flag table/description. Update any example invocation using `arm bootstrap --dual-branch` to plain `arm bootstrap`.

- [ ] **Step 4: `docs/configuration.md`**

Remove the `mode` field from the documented `config.json` schema (it no longer exists per Task 5). Remove any "migrate from single-branch to dual-branch" instructions. Reword the hook-environment description and `low_stakes_push_threshold` description to drop the "in dual-branch mode" qualifier (it's unconditional now). Remove any "Dual-Branch Architecture" section heading that implied an alternative; if the section documents useful architecture detail (worktree layout, `_armature` branch), keep the content but retitle it to something like "Ops Branch and Worktree" without the "dual" framing.

- [ ] **Step 5: `docs/concepts.md`**

Delete the "Single-Branch vs. Dual-Branch Modes" section heading and its mode-detection description. If the section contains information still relevant (e.g., that `.armature/` lives under a worktree, that ops are append-only), fold that into the surrounding architecture description without the mode-comparison framing.

- [ ] **Step 6: `docs/getting-started.md`**

Delete the "Solo vs Dual-Branch Modes" subsection. Update the setup walkthrough to state that `arm bootstrap` always creates the `_armature` branch and `.arm/` worktree — no branch-protection detection or mode choice involved.

- [ ] **Step 7: `docs/design/architecture.md`**

Delete any "Single-Branch Fallback" section. Update "Worktree Lifecycle" and "Directory Structure" sections to describe only the worktree-backed layout (no mode conditional). Update the CLI command reference entry for `bootstrap` to remove the `--dual-branch` flag and mode-detection description.

- [ ] **Step 8: `docs/design/gap-resolutions.md` and `docs/design/roles.md`**

Remove any persona/role distinctions based on single-branch vs. dual-branch usage; describe the single surviving workflow.

- [ ] **Step 9: `docs/use-cases.md`**

Update any use-case scenario written against single-branch mode to use the ops-branch/ops-worktree workflow instead.

- [ ] **Step 10: `docs/harness-hook.md`**

Remove any mode-conditional description of hook behavior (e.g., pre-commit blocking being dual-branch-only) — the harness hook behavior is unconditional now.

- [ ] **Step 11: `docs/design/dogfooding-learnings.md` (if present)**

Update or remove any note framed as "single-branch mode causes friction X" — if the underlying friction observation is still useful context (e.g., about worktree ergonomics), keep it without the mode-comparison framing.

- [ ] **Step 12: Final grep verification**

Run the Step 1 grep again. Expected: no output (or only intentional literal mentions describing the historical rationale, e.g., inside a "why we removed this" note you added — if so, confirm that's deliberate).

- [ ] **Step 13: Commit**

```bash
git add README.md docs/commands.md docs/configuration.md docs/concepts.md docs/getting-started.md \
  docs/design/architecture.md docs/design/gap-resolutions.md docs/design/roles.md \
  docs/use-cases.md docs/harness-hook.md
git add docs/design/dogfooding-learnings.md 2>/dev/null || true
git commit -m "docs: remove single-branch/dual-branch mode language from living docs"
```

---

### Task 12: Update skills

**Files:**
- Modify: `internal/skillsembed/skills/armature-worker/SKILL.md`
- Delete: `internal/skillsembed/skills/armature-worker/references/dual-branch.md` (merge its content into the main skill first)
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md`

**Interfaces:** None — skill content only. Note per `[[feedback_project_local_skills]]`: these are project-local skills read directly from disk, not registered with the `Skill` tool.

- [ ] **Step 1: Read the reference file before merging it**

Run: `cat internal/skillsembed/skills/armature-worker/references/dual-branch.md`

Identify the operative guidance (expected: "omit `.armature/` from `git add` on code branches" and similar ops-branch-awareness notes for workers).

- [ ] **Step 2: Fold `references/dual-branch.md` into `armature-worker/SKILL.md`**

In `internal/skillsembed/skills/armature-worker/SKILL.md`, find the two conditional references (originally around lines 161 and 224):

```
If using dual-branch mode, see `references/dual-branch.md`
```

and

```
see `references/dual-branch.md` for dual-branch mode exception
```

Replace both with the actual guidance inlined directly (not a conditional pointer, since it's unconditional now) — e.g., if the reference file's content is "never `git add .armature/` on a code branch; ops commits happen automatically on `_armature` via the harness hook", inline that sentence at each of the two call sites instead of linking out.

- [ ] **Step 3: Delete the now-merged reference file**

```bash
rm internal/skillsembed/skills/armature-worker/references/dual-branch.md
```

If `references/` is now empty, check whether other files still live there (`ls internal/skillsembed/skills/armature-worker/references/`) — only remove the directory itself if it's empty.

- [ ] **Step 4: Update `armature-coordinator/SKILL.md`**

Run: `grep -n "single-branch\|dual-branch\|Single-Branch\|Dual-Branch" internal/skillsembed/skills/armature-coordinator/SKILL.md`

For each match, remove the mode-conditional framing the same way as Task 11 — describe the ops-branch/worktree workflow unconditionally.

- [ ] **Step 5: Check whether skills are embedded at build time and need regeneration**

Run: `grep -rn "go:embed" internal/skillsembed/*.go`

If skills are embedded via `//go:embed`, no separate build step is needed (the embed directive picks up file changes automatically at compile time) — just confirm with `go build ./...` that the module still compiles.

- [ ] **Step 6: Run any skillsembed package tests**

Run: `go test ./internal/skillsembed/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/skillsembed/skills/armature-worker/SKILL.md internal/skillsembed/skills/armature-coordinator/SKILL.md
git rm internal/skillsembed/skills/armature-worker/references/dual-branch.md
git commit -m "skills: remove single-branch/dual-branch mode language, inline ops-branch guidance"
```

---

### Task 13: Final verification pass

**Files:** None modified — verification only.

**Interfaces:** None.

- [ ] **Step 1: Repo-wide grep for leftover references**

Run:

```bash
grep -rin "single.branch\|single_branch\|SingleBranch\|dual.branch\|dual_branch\|DualBranch" \
  --include="*.go" --include="*.md" . \
  | grep -v "docs/superpowers/plans/" \
  | grep -v "docs/superpowers/specs/" \
  | grep -v "docs/archive/"
```

Expected: no output, except possibly this plan file itself and `docs/adr/0006-eliminate-single-branch-mode.md` (both of which intentionally describe the historical mode being removed — that's correct and should stay).

- [ ] **Step 2: Confirm `CONTEXT.md` has no stale glossary entries**

Run: `grep -n "Single-Branch\|Dual-Branch" CONTEXT.md`
Expected: no output (already removed during the domain-modeling session that produced ADR 0006).

- [ ] **Step 3: Full build and test suite**

Run: `make check`

If `make check` is unavailable in this environment, run the narrower equivalent:

```bash
go build ./... && go vet ./... && go test ./... -count=1
```

Expected: all pass.

- [ ] **Step 4: Manual smoke test of the bootstrap → migration → normal workflow**

Run in a scratch directory:

```bash
d=$(mktemp -d) && cd "$d" && git init -q && git commit -q --allow-empty -m init
arm bootstrap
arm worker-init
arm create --type task --title "Smoke test task" --id SMOKE-01
arm materialize
arm ready
```

Expected: every command succeeds; `arm ready` lists `SMOKE-01`; `.arm/.armature/` exists; the repo root has no `.armature/` directory.

Then simulate the legacy-layout migration path:

```bash
d2=$(mktemp -d) && cd "$d2" && git init -q && git commit -q --allow-empty -m init
mkdir -p .armature/ops
echo '{"type":"create","target_id":"LEGACY-01","timestamp":1,"worker_id":"w1","payload":{"title":"Legacy","node_type":"task"}}' > .armature/ops/w1.log
arm bootstrap
```

Expected: output mentions "Migrated legacy single-branch ops"; `.arm/.armature/ops/w1.log` exists with the same content; `.armature/` at the repo root is gone (renamed to `.armature.migrated-*`).

- [ ] **Step 5: Report completion**

Summarize in the final commit message or a short note: all `Mode`/single-branch/dual-branch references removed from code, tests, docs, and skills; forced migration path added to `arm bootstrap` per ADR 0006; full test suite green.

No commit for this task — it's verification-only. If Step 1 or Step 3 finds anything, go back to the relevant earlier task and fix it before considering the plan complete.
