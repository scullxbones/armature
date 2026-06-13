# Architecture Deepening Follow-Up Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deepen three seams — RepoSnapshot, GraphFacts, and validated op evidence — so that commands, TUI, and health checks share one canonical source of truth for ops loading, graph queries, and structural verification.

**Architecture:** Introduce `internal/snapshot` as the single entry point for loading materialized state (ops → materialize → index → issues). Promote `dag.Graph` to canonical owner of graph facts by adding a public `FromIndex` factory and `IsLegalHierarchy` helper, then migrate callers that walk parent/sibling chains by hand. Fix `internal/doctor` to use validated op loading instead of raw `ops.ReadLog`.

**Tech Stack:** Go stdlib, existing internal packages (`ops`, `materialize`, `dag`, `ready`, `context`, `doctor`), Cobra commands, Bubble Tea TUI.

**Source PRD:** `docs/superpowers/specs/2026-06-13-architecture-deepening-follow-up-prd.md`

---

## Chunk 1: RepoSnapshot Module

### Task 1: Create snapshot package with Load function

**Files:**
- Create: `internal/snapshot/snapshot.go`
- Create: `internal/snapshot/snapshot_test.go`

#### Background

Every command that reads materialized state repeats the same three steps:
1. `ops.LoadFromDirWithOffsetsValidated(opsDir)` → ops + offsets + warnings
2. `materialize.MaterializeAndReturn(stateDir, ops, singleBranch, offsets)` → state
3. `materialize.LoadIndex(...)` + per-issue loads from disk

A `Snapshot` type collapses these into a single call. The TUI `doRefresh` and every
command migrated in Tasks 2–4 will use it.

The snapshot package may import `ops` and `materialize` but must not import any command
package or `tui`.

- [ ] **Step 1: Write the failing tests**

Create `internal/snapshot/snapshot_test.go`:

```go
package snapshot_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/snapshot"
)

// writeOp writes a single JSON op line to path, creating parent dirs.
func writeOp(t *testing.T, path string, op ops.Op) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	line, err := ops.MarshalOp(op)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

func TestLoad_EmptyOpsDir(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	snap, err := snapshot.Load(opsDir, stateDir, true)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if snap == nil {
		t.Fatal("Load returned nil snapshot")
	}
	if snap.State == nil {
		t.Error("snapshot.State is nil")
	}
	if snap.Index == nil {
		t.Error("snapshot.Index is nil")
	}
	if snap.Issues == nil {
		t.Error("snapshot.Issues is nil")
	}
	if len(snap.Index) != 0 {
		t.Errorf("expected empty index, got %d entries", len(snap.Index))
	}
}

func TestLoad_SingleIssue(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	logPath := filepath.Join(opsDir, "worker1.log")
	writeOp(t, logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "TASK-1",
		WorkerID:  "worker1",
		Timestamp: time.Now().Unix(),
		Payload:   ops.Payload{Title: "My task", Type: "task"},
	})

	snap, err := snapshot.Load(opsDir, stateDir, true)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if _, ok := snap.Index["TASK-1"]; !ok {
		t.Error("TASK-1 not in index")
	}
	if _, ok := snap.Issues["TASK-1"]; !ok {
		t.Error("TASK-1 not in issues map")
	}
	if snap.State.Issues["TASK-1"] == nil {
		t.Error("TASK-1 not in state")
	}
}

func TestLoad_WorkerIDMismatchProducesWarning(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	// Op with wrong worker ID in file (file=worker1.log but op.WorkerID=other)
	logPath := filepath.Join(opsDir, "worker1.log")
	writeOp(t, logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "TASK-X",
		WorkerID:  "intruder",
		Timestamp: time.Now().Unix(),
		Payload:   ops.Payload{Title: "Bad", Type: "task"},
	})

	snap, err := snapshot.Load(opsDir, stateDir, true)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(snap.Warnings) == 0 {
		t.Error("expected at least one warning for worker ID mismatch, got none")
	}
}

func TestLoad_StateAndIssuesAgree(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	logPath := filepath.Join(opsDir, "w1.log")
	writeOp(t, logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "EPIC-1",
		WorkerID:  "w1",
		Timestamp: time.Now().Unix(),
		Payload:   ops.Payload{Title: "Epic", Type: "epic"},
	})

	snap, err := snapshot.Load(opsDir, stateDir, true)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// State and Issues map must have same keys
	for id := range snap.State.Issues {
		if _, ok := snap.Issues[id]; !ok {
			t.Errorf("state has %s but Issues map does not", id)
		}
	}
	for id := range snap.Issues {
		if snap.State.Issues[id] == nil {
			t.Errorf("Issues map has %s but state does not", id)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/snapshot/... -v
```

Expected: compile error (package doesn't exist yet)

- [ ] **Step 3: Create the snapshot package**

Create `internal/snapshot/snapshot.go`:

```go
package snapshot

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// Snapshot holds the fully loaded current state of the Armature repo.
// It is the canonical source of truth for commands and the TUI.
type Snapshot struct {
	State    *materialize.State
	Index    materialize.Index
	Issues   map[string]*materialize.Issue
	Warnings []string
}

// Load reads ops from opsDir, materializes into stateDir, and returns a Snapshot.
// Warnings include any worker-ID mismatches or corrupt log lines detected during load.
// Returns a valid (possibly empty) Snapshot on success; never returns nil on nil error.
func Load(opsDir, stateDir string, singleBranch bool) (*Snapshot, error) {
	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops: %w", err)
	}
	allOps := ops.ExtractOps(items)

	state, _, err := materialize.MaterializeAndReturn(stateDir, allOps, singleBranch, offsets)
	if err != nil {
		return nil, fmt.Errorf("materialize: %w", err)
	}

	index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}

	issuesDir := filepath.Join(stateDir, "issues")
	issues, err := materialize.LoadAllIssues(issuesDir)
	if err != nil {
		return nil, fmt.Errorf("load issues: %w", err)
	}

	return &Snapshot{
		State:    state,
		Index:    index,
		Issues:   issues,
		Warnings: warnings,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/snapshot/... -v
```

Expected: all four tests PASS

- [ ] **Step 5: Run full test suite**

```bash
make check
```

Expected: green (no existing tests broken)

- [ ] **Step 6: Commit**

```bash
git add internal/snapshot/snapshot.go internal/snapshot/snapshot_test.go
git commit -m "feat(snapshot): add RepoSnapshot.Load for one-call state loading"
```

---

### Task 2: Migrate ready, show, and validate commands to use snapshot

**Files:**
- Modify: `cmd/armature/ready.go`
- Modify: `cmd/armature/show.go`
- Modify: `cmd/armature/validate.go`

These three commands follow the same 3-step pattern. Replace them with a single
`snapshot.Load(...)` call, emit warnings to stderr exactly as the existing helpers do.

Note: `validate.go` calls `materialize.MaterializeAndReturn` (not `Materialize`) to
get the in-memory state. After migration it calls `snapshot.Load` and uses `snap.State`.

- [ ] **Step 1: Update ready.go**

Replace the ops-read and materialize block in `ready.go`:

```go
// Before (lines 50–68 in ready.go):
allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
if err != nil {
    return fmt.Errorf("read ops: %w", err)
}
if _, err := materialize.Materialize(appCtx.StateDir, allOps, appCtx.Mode == "single-branch", offsets); err != nil {
    return fmt.Errorf("materialize: %w", err)
}

index, err := materialize.LoadIndex(filepath.Join(appCtx.StateDir, "index.json"))
if err != nil {
    return err
}

issues := make(map[string]*materialize.Issue)
for id := range index {
    issue, err := materialize.LoadIssue(filepath.Join(appCtx.StateDir, "issues", id+".json"))
    if err == nil {
        issues[id] = &issue
    }
}
```

```go
// After:
snap, err := snapshot.Load(
    filepath.Join(issuesDir, "ops"),
    appCtx.StateDir,
    appCtx.Mode == "single-branch",
)
if err != nil {
    return fmt.Errorf("load snapshot: %w", err)
}
for _, w := range snap.Warnings {
    fmt.Fprintf(os.Stderr, "warning: %s\n", w)
}
index := snap.Index
issues := snap.Issues
```

Add import: `"github.com/scullxbones/armature/internal/snapshot"` and `"os"` if not already present.
Remove now-unused imports: `"github.com/scullxbones/armature/internal/materialize"` if only used for Materialize/LoadIndex/LoadIssue.

- [ ] **Step 2: Update show.go**

In `show.go`'s RunE, replace:
```go
allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
if err != nil {
    return fmt.Errorf("read ops: %w", err)
}
if _, err := materialize.Materialize(appCtx.StateDir, allOps, singleBranch, offsets); err != nil {
    return err
}
```

With:
```go
snap, err := snapshot.Load(
    filepath.Join(issuesDir, "ops"),
    appCtx.StateDir,
    singleBranch,
)
if err != nil {
    return fmt.Errorf("load snapshot: %w", err)
}
for _, w := range snap.Warnings {
    fmt.Fprintf(os.Stderr, "warning: %s\n", w)
}
```

Then use `snap.State`, `snap.Index`, and `snap.Issues` in place of the previously
separate variables. Check the rest of show.go's RunE for any remaining direct calls
to `materialize.LoadIssue(...)` and replace with `snap.Issues[id]` lookups.

- [ ] **Step 3: Update validate.go**

Replace the ops-read + MaterializeAndReturn block:
```go
allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(appCtx.IssuesDir, "ops"))
if err != nil {
    return fmt.Errorf("read ops: %w", err)
}
state, _, err := materialize.MaterializeAndReturn(appCtx.StateDir, allOps, true, offsets)
if err != nil {
    return err
}
```

With:
```go
snap, err := snapshot.Load(
    filepath.Join(appCtx.IssuesDir, "ops"),
    appCtx.StateDir,
    true,
)
if err != nil {
    return fmt.Errorf("load snapshot: %w", err)
}
for _, w := range snap.Warnings {
    fmt.Fprintf(os.Stderr, "warning: %s\n", w)
}
state := snap.State
```

- [ ] **Step 4: Verify compilation**

```bash
go build ./cmd/armature/...
```

Expected: compiles without errors

- [ ] **Step 5: Run tests**

```bash
make test
```

Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add cmd/armature/ready.go cmd/armature/show.go cmd/armature/validate.go
git commit -m "refactor(cmd): migrate ready, show, validate to snapshot.Load"
```

---

### Task 3: Migrate render-context command and TUI to snapshot

**Files:**
- Modify: `cmd/armature/render_context.go`
- Modify: `internal/tui/app/model.go`

`render_context.go` has two paths: normal and `--at` (time-travel via `MaterializeAtSHA`).
Only the normal path uses snapshot; the `--at` path keeps `MaterializeAtSHA`.

`model.go`'s `doRefresh` duplicates the ops-read + materialize step. It also has its
own unexported `readAllOpsFromDirWithOffsets` helper. After migration, that helper
can be removed.

- [ ] **Step 1: Update render_context.go normal path**

In the `else` branch (non `--at` path):
```go
// Before:
allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(issuesDir, "ops"))
if err != nil {
    return fmt.Errorf("read ops: %w", err)
}
_, err = materialize.Materialize(appCtx.StateDir, allOps, appCtx.Mode == "single-branch", offsets)
if err != nil {
    return fmt.Errorf("materialize: %w", err)
}
state, err = loadStateFromStateDir(appCtx.StateDir)
if err != nil {
    return fmt.Errorf("load state: %w", err)
}
```

```go
// After:
snap, err := snapshot.Load(
    filepath.Join(issuesDir, "ops"),
    appCtx.StateDir,
    appCtx.Mode == "single-branch",
)
if err != nil {
    return fmt.Errorf("load snapshot: %w", err)
}
for _, w := range snap.Warnings {
    fmt.Fprintf(os.Stderr, "warning: %s\n", w)
}
state = snap.State
```

Remove the `loadStateFromStateDir` helper from `render_context.go` (it is only used
in the else branch). The `--at` path retains its `MaterializeAtSHA` call unchanged.

- [ ] **Step 2: Update TUI model.go doRefresh**

Replace the `doRefresh` function body:
```go
// Before:
func (m Model) doRefresh() tea.Cmd {
    issuesDir := m.issuesDir
    stateDir := m.stateDir
    return func() tea.Msg {
        opsDir := filepath.Join(issuesDir, "ops")
        allOps, offsets, err := readAllOpsFromDirWithOffsets(opsDir)
        if err != nil {
            return nil
        }
        state, _, err := materialize.MaterializeAndReturn(stateDir, allOps, true, offsets)
        if err != nil || state == nil {
            return nil
        }
        return stateUpdatedMsg{state: state}
    }
}
```

```go
// After:
func (m Model) doRefresh() tea.Cmd {
    issuesDir := m.issuesDir
    stateDir := m.stateDir
    return func() tea.Msg {
        snap, err := snapshot.Load(filepath.Join(issuesDir, "ops"), stateDir, true)
        if err != nil || snap.State == nil {
            return nil
        }
        return stateUpdatedMsg{state: snap.State}
    }
}
```

Remove the local `readAllOpsFromDirWithOffsets` helper from `model.go` (lines 286–298)
and its imports `ops` and `os` if they are no longer used.

Add import: `"github.com/scullxbones/armature/internal/snapshot"`

- [ ] **Step 3: Build and test**

```bash
go build ./...
make test
```

Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add cmd/armature/render_context.go internal/tui/app/model.go
git commit -m "refactor(cmd,tui): migrate render-context and TUI doRefresh to snapshot.Load"
```

---

## Chunk 2: GraphFacts Seam Completion

### Task 4: Add FromIndex factory and IsLegalHierarchy to dag package

**Files:**
- Modify: `internal/dag/dag.go`
- Modify: `internal/dag/dag_test.go`

`buildDAGFromIndex` is duplicated privately in `internal/ready/compute.go`. Promoting
it to `dag.FromIndex` gives every consumer — ready, doctor, context — a single
canonical factory. `IsLegalHierarchy` formalizes the type-hierarchy rules (which are
currently implicit in validate).

Legal parent→child relationships (matching existing validate rules):
- epic → story, task, feature
- story → task, feature
- task → (none; tasks are leaves)
- feature → task
- "" (root) → epic, story, task, feature

- [ ] **Step 1: Write the failing tests**

Append to `internal/dag/dag_test.go`:

```go
func TestFromIndex_BasicStructure(t *testing.T) {
	index := materialize.Index{
		"EPIC-1": materialize.IndexEntry{Type: "epic", Status: "open", Children: []string{"TASK-1"}},
		"TASK-1": materialize.IndexEntry{Type: "task", Status: "open", Parent: "EPIC-1", BlockedBy: []string{}},
	}
	dagObj := dag.FromIndex(index)
	if dagObj == nil {
		t.Fatal("FromIndex returned nil")
	}
	g := dag.NewGraph(dagObj)
	ancestors := g.Ancestry("TASK-1")
	if len(ancestors) != 1 || ancestors[0] != "EPIC-1" {
		t.Errorf("expected [EPIC-1], got %v", ancestors)
	}
	descendants := g.Descendants("EPIC-1")
	if len(descendants) != 1 || descendants[0] != "TASK-1" {
		t.Errorf("expected [TASK-1], got %v", descendants)
	}
}

func TestFromIndex_Empty(t *testing.T) {
	dagObj := dag.FromIndex(materialize.Index{})
	if dagObj == nil {
		t.Fatal("FromIndex returned nil for empty index")
	}
	g := dag.NewGraph(dagObj)
	if g.HasCycle() {
		t.Error("empty graph should not have cycle")
	}
}

func TestIsLegalHierarchy(t *testing.T) {
	cases := []struct {
		parent string
		child  string
		legal  bool
	}{
		{"epic", "story", true},
		{"epic", "task", true},
		{"epic", "feature", true},
		{"story", "task", true},
		{"story", "feature", true},
		{"story", "epic", false},
		{"task", "task", false},
		{"feature", "task", true},
		{"feature", "epic", false},
		{"", "epic", true},
		{"", "story", true},
		{"", "task", true},
		{"", "feature", true},
	}
	for _, tc := range cases {
		got := dag.IsLegalHierarchy(tc.parent, tc.child)
		if got != tc.legal {
			t.Errorf("IsLegalHierarchy(%q, %q) = %v, want %v", tc.parent, tc.child, got, tc.legal)
		}
	}
}
```

Note: the test file must import `"github.com/scullxbones/armature/internal/materialize"`.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/dag/... -v -run "TestFromIndex|TestIsLegal"
```

Expected: compile error (FromIndex, IsLegalHierarchy not defined)

- [ ] **Step 3: Implement FromIndex and IsLegalHierarchy in dag.go**

Add to `internal/dag/dag.go` (after the existing `Depth` function):

```go
// FromIndex constructs a DAG from a materialized Index.
// This is the canonical factory for producing a DAG from Armature task state.
func FromIndex(index materialize.Index) *DAG {
	d := New()
	for id, entry := range index {
		node := &Node{
			ID:        id,
			Type:      entry.Type,
			Parent:    entry.Parent,
			Children:  make([]string, len(entry.Children)),
			BlockedBy: make([]string, len(entry.BlockedBy)),
			Blocks:    make([]string, len(entry.Blocks)),
		}
		copy(node.Children, entry.Children)
		copy(node.BlockedBy, entry.BlockedBy)
		copy(node.Blocks, entry.Blocks)
		d.AddNode(node) //nolint:errcheck
	}
	return d
}

// legalChildren maps each parent type to the set of allowed child types.
// An empty parent key represents a root (no parent).
var legalChildren = map[string]map[string]bool{
	"":        {"epic": true, "story": true, "task": true, "feature": true},
	"epic":    {"story": true, "task": true, "feature": true},
	"story":   {"task": true, "feature": true},
	"feature": {"task": true},
	"task":    {},
}

// IsLegalHierarchy reports whether childType may be a direct child of parentType.
// parentType "" means the child has no parent (root level).
func IsLegalHierarchy(parentType, childType string) bool {
	allowed, ok := legalChildren[parentType]
	if !ok {
		return false
	}
	return allowed[childType]
}
```

Add `"github.com/scullxbones/armature/internal/materialize"` to the dag package imports.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/dag/... -v
```

Expected: all dag tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/dag/dag.go internal/dag/dag_test.go
git commit -m "feat(dag): add FromIndex factory and IsLegalHierarchy helper"
```

---

### Task 5: Migrate ready package to use dag.FromIndex

**Files:**
- Modify: `internal/ready/compute.go`
- (No test changes needed — behavior is identical)

`buildDAGFromIndex` in `ready/compute.go` is now a private duplicate of `dag.FromIndex`.
`CollectDescendants` does its own BFS over the index; after the migration it can delegate
to `dag.Graph.Descendants`.

- [ ] **Step 1: Replace buildDAGFromIndex with dag.FromIndex**

In `internal/ready/compute.go`:

1. Remove the `buildDAGFromIndex` function entirely (lines 258–277).
2. Replace the call site in `ComputeReady`:

```go
// Before:
dagObj := buildDAGFromIndex(index)
graph := dag.NewGraph(dagObj)
```

```go
// After:
graph := dag.NewGraph(dag.FromIndex(index))
```

- [ ] **Step 2: Replace CollectDescendants with graph-based version**

Replace `CollectDescendants` body:

```go
// Before:
func CollectDescendants(root string, index materialize.Index) map[string]bool {
	result := make(map[string]bool)
	queue := []string{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		entry, ok := index[current]
		if !ok {
			continue
		}
		for _, child := range entry.Children {
			if !result[child] {
				result[child] = true
				queue = append(queue, child)
			}
		}
	}
	return result
}
```

```go
// After:
func CollectDescendants(root string, index materialize.Index) map[string]bool {
	g := dag.NewGraph(dag.FromIndex(index))
	result := make(map[string]bool)
	for _, id := range g.Descendants(root) {
		result[id] = true
	}
	return result
}
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/ready/...
go test ./internal/ready/... -v
```

Expected: all pass (behavior identical, just delegated to dag)

- [ ] **Step 4: Commit**

```bash
git add internal/ready/compute.go
git commit -m "refactor(ready): delegate DAG building and descendant collection to dag package"
```

---

### Task 6: Migrate context assembler to use dag.Graph

**Files:**
- Modify: `internal/context/assemble.go`
- Modify: `internal/context/context_test.go`
- Modify: `cmd/armature/render_context.go`

`buildParentChain` and `buildSiblingOutcomes` currently walk `state.Issues` and fall
back to loading from disk. With `dag.Graph`, ancestry and sibling IDs come from the
graph; per-issue data comes from `state.Issues` only (no disk fallback).

This changes the `Assemble` signature from:
```go
func Assemble(issueID string, stateDir string, state *materialize.State) (*Context, error)
```
to:
```go
func Assemble(issueID string, stateDir string, state *materialize.State, graph *dag.Graph) (*Context, error)
```

`stateDir` is kept because `inferRepoRoot(stateDir)` still needs it for
`buildContextFiles`. The only change to the signature is adding `graph *dag.Graph`.

- [ ] **Step 1: Update Assemble signature and helpers**

In `internal/context/assemble.go`:

1. Change `Assemble` to add `graph *dag.Graph` as a new final parameter:

```go
func Assemble(issueID string, stateDir string, state *materialize.State, graph *dag.Graph) (*Context, error) {
	issue, ok := state.Issues[issueID]
	if !ok {
		return nil, fmt.Errorf("issue %s not found in state", issueID)
	}

	var layers []Layer
	repoRoot := inferRepoRoot(stateDir)

	layers = append(layers, buildCoreSpec(issue))
	layers = append(layers, buildContextFiles(issue, repoRoot))
	layers = append(layers, buildSnippets(issue))
	layers = append(layers, buildBlockerOutcomes(issue, state))
	layers = append(layers, buildParentChain(issue, state, graph))
	layers = append(layers, buildDecisions(issue))
	layers = append(layers, buildNotes(issue))
	layers = append(layers, buildSiblingOutcomes(issue, state, graph))

	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Priority < layers[j].Priority
	})

	return &Context{IssueID: issueID, Layers: layers}, nil
}
```

`inferRepoRoot` is unchanged. Leave it as-is.

2. Rewrite `buildBlockerOutcomes` without disk fallback:

```go
func buildBlockerOutcomes(issue *materialize.Issue, state *materialize.State) Layer {
	if len(issue.BlockedBy) == 0 {
		return Layer{Name: "blocker_outcomes", Priority: 3, Content: ""}
	}
	var lines []string
	for _, blockerID := range issue.BlockedBy {
		outcome := "outcome unknown"
		if blocker, ok := state.Issues[blockerID]; ok {
			if blocker.Outcome != "" {
				outcome = blocker.Outcome
			} else if blocker.Status != "" {
				outcome = fmt.Sprintf("%s (outcome unknown)", blocker.Status)
			}
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", blockerID, outcome))
	}
	return Layer{Name: "blocker_outcomes", Priority: 4,
		Content: "## Blocking Issue Outcomes\n" + strings.Join(lines, "\n")}
}
```

4. Rewrite `buildParentChain` using `graph.Ancestry`:

```go
func buildParentChain(issue *materialize.Issue, state *materialize.State, graph *dag.Graph) Layer {
	ancestors := graph.Ancestry(issue.ID)
	if len(ancestors) > 3 {
		ancestors = ancestors[:3]
	}
	var lines []string
	for _, id := range ancestors {
		if parent, ok := state.Issues[id]; ok {
			lines = append(lines, fmt.Sprintf("- %s: %s [%s]", id, parent.Title, parent.Status))
		}
	}
	if len(lines) == 0 {
		return Layer{Name: "parent_chain", Priority: 5, Content: ""}
	}
	return Layer{Name: "parent_chain", Priority: 5,
		Content: "## Parent Chain\n" + strings.Join(lines, "\n")}
}
```

5. Rewrite `buildSiblingOutcomes` using `graph.Hierarchy`:

```go
func buildSiblingOutcomes(issue *materialize.Issue, state *materialize.State, graph *dag.Graph) Layer {
	if issue.Parent == "" {
		return Layer{Name: "sibling_outcomes", Priority: 8, Content: ""}
	}
	_, children := graph.Hierarchy(issue.Parent)
	var lines []string
	for _, sibID := range children {
		if sibID == issue.ID {
			continue
		}
		sib, ok := state.Issues[sibID]
		if !ok {
			continue
		}
		if sib.Status == "done" || sib.Status == "merged" {
			outcome := sib.Outcome
			if outcome == "" {
				outcome = "(none)"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", sibID, outcome))
		}
	}
	if len(lines) == 0 {
		return Layer{Name: "sibling_outcomes", Priority: 8, Content: ""}
	}
	return Layer{Name: "sibling_outcomes", Priority: 8,
		Content: "## Sibling Outcomes\n" + strings.Join(lines, "\n")}
}
```

Remove imports no longer needed: `"os"` (if only used in old disk reads), `"path/filepath"` (if only used for issue paths).

- [ ] **Step 2: Update tests in context_test.go**

The test file calls `context.Assemble(...)` with the old signature. Update all callers
to pass a `*dag.Graph` built from the test state:

In each test that calls `context.Assemble`:
```go
// Build a graph from the test state
index := testState.BuildIndex()
g := dag.NewGraph(dag.FromIndex(index))
ctx, err := context.Assemble(issueID, stateDir, testState, g)
```

`stateDir` remains; pass the same test temp dir value as before.

- [ ] **Step 3: Update render_context.go caller**

In `cmd/armature/render_context.go`, update the `Assemble` call:

```go
// Before (in the else branch after loading snap):
ctx, err := context.Assemble(rcIssue, appCtx.StateDir, snap.State)
```

```go
// After:
g := dag.NewGraph(dag.FromIndex(snap.Index))
ctx, err := context.Assemble(rcIssue, appCtx.StateDir, snap.State, g)
```

For the `--at` time-travel path, build the index from state:
```go
// After MaterializeAtSHA returns state:
index := state.BuildIndex()
g := dag.NewGraph(dag.FromIndex(index))
ctx, err := context.Assemble(rcIssue, appCtx.StateDir, state, g)
```

Add imports to render_context.go:
```go
"github.com/scullxbones/armature/internal/dag"
```

- [ ] **Step 4: Build and test**

```bash
go build ./...
make test
```

Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add internal/context/assemble.go internal/context/context_test.go cmd/armature/render_context.go
git commit -m "refactor(context): use dag.Graph for ancestry and sibling queries, remove disk fallbacks"
```

---

### Task 7: Migrate doctor D5 cycle detection to dag.Graph

**Files:**
- Modify: `internal/doctor/doctor.go`

Doctor's `checkD5DependencyCycles` implements its own DFS cycle detection with
`color` states. This duplicates `dag.Graph.HasCycle()`. Replace it with:
1. Build a graph from the index via `dag.FromIndex(index)`.
2. Call `dag.NewGraph(dagObj).HasCycle()` to detect cycles.
3. For the items list (which cycles specifically), keep a simplified version that
   collects violating edges from the blocking graph only (the current DFS already
   searches both child and blocker edges, but D5's semantics are blocker-only cycles).

Note: The existing D5 check reports cycle edges in `blocked_by` only (the D5 message
says "Dependency cycles detected in blocked_by chains"). The new version uses
`dag.Graph.HasCycle()` which checks both parent-child and blocker edges. Verify that
the test suite passes — if not, keep the blocker-only DFS for D5 but just move it
inline (removing the color-state boilerplate).

- [ ] **Step 1: Replace D5 implementation**

In `internal/doctor/doctor.go`, replace `checkD5DependencyCycles`:

```go
func checkD5DependencyCycles(index materialize.Index) Finding {
	f := Finding{Check: "D5", Severity: SeverityOK, Message: "No dependency cycles"}

	g := dag.NewGraph(dag.FromIndex(index))
	if !g.HasCycle() {
		return f
	}

	// Collect cycle edges from blocked_by chains for diagnostic output.
	var cycleNodes []string
	visited := make(map[string]int) // 0=white, 1=gray, 2=black

	var dfs func(id string) bool
	dfs = func(id string) bool {
		visited[id] = 1
		entry, ok := index[id]
		if !ok {
			visited[id] = 2
			return false
		}
		for _, dep := range entry.BlockedBy {
			switch visited[dep] {
			case 1:
				cycleNodes = append(cycleNodes, fmt.Sprintf("%s -> %s", id, dep))
				return true
			case 0:
				if dfs(dep) {
					return true
				}
			}
		}
		visited[id] = 2
		return false
	}

	for id := range index {
		if visited[id] == 0 {
			dfs(id)
		}
	}

	sort.Strings(cycleNodes)
	f.Severity = SeverityError
	f.Message = "Dependency cycles detected in blocked_by chains"
	f.Items = cycleNodes
	return f
}
```

Add import: `"github.com/scullxbones/armature/internal/dag"`

- [ ] **Step 2: Build and test doctor**

```bash
go test ./internal/doctor/... -v
```

Expected: all pass

- [ ] **Step 3: Commit**

```bash
git add internal/doctor/doctor.go
git commit -m "refactor(doctor): use dag.Graph.HasCycle for D5 cycle detection"
```

---

## Chunk 3: Validated Op Evidence in doctor

### Task 8: Replace raw log reading in doctor with validated op stream

**Files:**
- Modify: `internal/doctor/doctor.go`

Doctor's `Run` function calls `readAllOpsFromOpsDir` (uses raw `ops.ReadLog`) and
`readAllOpsFromOpsDirWithLocations` (also raw). These bypass the worker-ID validation
that `ops.LoadFromDirValidated` enforces. Doctor and materialization can silently
disagree about which ops count.

After this task, doctor uses `ops.LoadFromDirValidated` (without location tracking)
and `ops.LoadFromDirWithOffsetsValidated` (for the non-verbose path, providing
warnings). For the verbose path (location tracking), we derive locations from the
`OpItem` metadata returned by the validated stream.

- [ ] **Step 1: Write a failing test that proves doctor uses validated loading**

In `internal/doctor/doctor_test.go` (or a new file), add a test that:
1. Creates an ops dir with a log file named `worker1.log`
2. Writes an op with `WorkerID: "intruder"` (mismatch)
3. Writes a valid op for a different issue
4. Materializes state that would include the intruder op if raw reading were used
5. Runs `doctor.Run` and asserts the intruder's target ID is NOT in any D3 orphan list
   (it would appear orphaned if the raw reader picked it up but the materializer rejected it)

```go
func TestRun_ValidatedOpsExcludesMismatches(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")

	if err := os.MkdirAll(opsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Valid op for TASK-1
	validLog := filepath.Join(opsDir, "worker1.log")
	writeOp(t, validLog, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "TASK-1",
		WorkerID:  "worker1",
		Timestamp: time.Now().Unix(),
		Payload:   ops.Payload{Title: "Task one", Type: "task"},
	})

	// Mismatched op: file=worker1.log but WorkerID=intruder, targeting GHOST-99
	writeOp(t, validLog, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "GHOST-99",
		WorkerID:  "intruder",
		Timestamp: time.Now().Unix(),
		Payload:   ops.Payload{Title: "Ghost", Type: "task"},
	})

	report, err := doctor.Run(dir, stateDir, "", false)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// D3 should not report GHOST-99 as orphaned (it should be excluded by validation)
	for _, f := range report.Checks {
		if f.Check == "D3" && f.Severity == doctor.SeverityError {
			for _, item := range f.Items {
				if strings.Contains(item, "GHOST-99") {
					t.Error("D3 reported GHOST-99 as orphaned: mismatch should have been excluded")
				}
			}
		}
	}
}
```

Note: `writeOp` is a helper to write a single op JSON line to a file. Define it locally
in doctor_test.go (same implementation as the one in snapshot_test.go — each package
keeps its own copy since they are in separate packages and the helper is unexported).

- [ ] **Step 2: Run to verify it fails (or passes vacuously)**

```bash
go test ./internal/doctor/... -run TestRun_ValidatedOpsExcludesMismatches -v
```

If the test already passes, the raw reader is NOT picking up GHOST-99. Check whether
the existing test is meaningful by temporarily changing the assertion direction.

- [ ] **Step 3: Replace readAllOpsFromOpsDir with validated loading**

In `internal/doctor/doctor.go`, replace the two private raw-reading functions with
validated equivalents:

```go
// Run executes all health checks and returns a Report.
func Run(issuesDir string, stateDir string, repoPath string, verbose bool) (Report, error) {
	singleBranch := true
	opsDir := filepath.Join(issuesDir, "ops")

	// Use validated loading: worker-ID mismatches are excluded, not silently accepted.
	items, _, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return Report{}, fmt.Errorf("read ops: %w", err)
	}
	allOps := ops.ExtractOps(items)

	// Build location map for verbose D3 output from OpItem metadata.
	var opLocations map[string][]string
	if verbose {
		opLocations = buildLocationMap(items)
	} else {
		opLocations = make(map[string][]string)
	}

	// Emit warnings from validation (worker ID mismatches, corrupt lines).
	_ = warnings // callers of Run may choose to expose these; leave for now

	if _, err := materialize.Materialize(stateDir, allOps, singleBranch, nil); err != nil {
		return Report{}, fmt.Errorf("materialize: %w", err)
	}

	// ... rest of Run unchanged ...
}

// buildLocationMap derives a target-ID → file:line map from validated OpItems.
// Line numbers are per-file (1-based index of each accepted op within its log file).
// This is more accurate than the old readAllOpsFromOpsDirWithLocations which used
// a global counter across all files, producing incorrect cross-file line numbers.
func buildLocationMap(items []ops.OpItem) map[string][]string {
	locations := make(map[string][]string)
	lineByFile := make(map[string]int)
	for _, item := range items {
		base := filepath.Base(item.LogFilename)
		lineByFile[base]++
		line := lineByFile[base]
		locStr := fmt.Sprintf("%s:%d", base, line)
		locations[item.Op.TargetID] = append(locations[item.Op.TargetID], locStr)
	}
	return locations
}
```

Remove the now-unused `readAllOpsFromOpsDir` and `readAllOpsFromOpsDirWithLocations`
functions entirely (they are private to the doctor package).

- [ ] **Step 4: Run doctor tests**

```bash
go test ./internal/doctor/... -v
```

Expected: all pass, including the new mismatch test

- [ ] **Step 5: Run full check**

```bash
make check
```

Expected: green

- [ ] **Step 6: Commit**

```bash
git add internal/doctor/doctor.go internal/doctor/doctor_test.go
git commit -m "fix(doctor): use validated op stream so D3 and materialization agree on accepted ops"
```

---

### Task 9: Integration verification

**Files:** None — run commands against a real repo

Verify that `arm ready`, `arm render-context`, `arm show`, `arm validate`, and
`arm doctor` all produce consistent output using the same underlying op evidence.

- [ ] **Step 1: Build the binary**

```bash
make build
```

- [ ] **Step 2: Run the verification commands**

```bash
go run ./cmd/armature validate --ci
go run ./cmd/armature doctor
go run ./cmd/armature ready --format json | head -5
```

Expected: all exit 0 (or the same exit codes as before this change)

- [ ] **Step 3: Run the full check suite**

```bash
make check
```

Expected: all stages green (lint, test, coverage ≥ 80%, mutate, validate-skills, build)

- [ ] **Step 4: Tag completion**

If all green, the architecture-deepening work is complete. No commit needed for this
verification task.

---

## Summary of changed files

| File | Change |
|------|--------|
| `internal/snapshot/snapshot.go` | NEW — RepoSnapshot.Load |
| `internal/snapshot/snapshot_test.go` | NEW — snapshot tests |
| `internal/dag/dag.go` | ADD FromIndex, IsLegalHierarchy |
| `internal/dag/dag_test.go` | ADD tests for FromIndex, IsLegalHierarchy |
| `internal/ready/compute.go` | REMOVE buildDAGFromIndex; delegate to dag |
| `internal/context/assemble.go` | USE dag.Graph; remove disk fallbacks; update Assemble signature |
| `internal/context/context_test.go` | UPDATE Assemble call sites |
| `internal/doctor/doctor.go` | USE validated op stream; USE dag.Graph for D5 |
| `internal/doctor/doctor_test.go` | ADD mismatch exclusion test |
| `cmd/armature/ready.go` | USE snapshot.Load |
| `cmd/armature/show.go` | USE snapshot.Load |
| `cmd/armature/validate.go` | USE snapshot.Load |
| `cmd/armature/render_context.go` | USE snapshot.Load + dag.Graph |
| `internal/tui/app/model.go` | USE snapshot.Load in doRefresh |
