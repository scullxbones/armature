# Test Coverage Hardening — 80% Enforcement and Mutation Efficacy

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Raise test coverage from 68.2% to ≥80%, kill surviving mutations (78.52% efficacy → ≥90%), and add Makefile enforcement that fails the build below threshold.

**Architecture:**
Tests use Go's standard `testing` package with `testify/assert` and `testify/require`. Integration tests in `cmd/trellis` use the existing `runTrls` helper and `initTempRepo`. Unit tests in `internal` packages operate directly on structs. No mocks — tests exercise real behavior.

Coverage gaps by package (current → target):
- `cmd/trellis`: 60.8% → 80% — workers, log, assign, heartbeat, decision, link, reopen commands untested
- `internal/context`: 56.9% → 80% — buildSnippets, buildDecisions, buildNotes, buildSiblingOutcomes, RenderAgent/RenderHuman uncovered
- `internal/materialize`: 63.3% → 80% — MaterializeAndReturn, appendUnique, RunRollup boundary untested
- `internal/ready`: 77.9% → 80% — isClaimStale, depth, assignmentTier edge cases missing

LIVED mutations require boundary-condition tests (exact threshold values) and tighter assertions.

**Tech Stack:** Go, testify/assert, testify/require

---

## Chunk 1: Makefile enforcement

### Task 1: Add coverage-check target to Makefile

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add coverage-check target**

In `Makefile`, after the `coverage` target, add:

```makefile
coverage-check:
	go test -coverprofile=coverage.out ./...
	@COVERAGE=$$(go tool cover -func=coverage.out | grep "^total:" | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${COVERAGE}%"; \
	if [ $$(echo "$${COVERAGE} < 80" | bc -l) -eq 1 ]; then \
		echo "FAIL: coverage $${COVERAGE}% is below 80% threshold"; \
		exit 1; \
	fi
```

Also update `help` to mention coverage-check, and add `coverage-check` to `.PHONY`:

```makefile
.PHONY: test coverage coverage-check lint clean mutate help skill install
```

Add to help text:
```
@echo "  make coverage-check - Check coverage meets 80% threshold (fails build if not)"
```

- [ ] **Step 2: Run coverage-check and confirm it currently fails**

Run: `make coverage-check`
Expected: Prints `FAIL: coverage 68.2% is below 80% threshold` and exits 1.

- [ ] **Step 3: Commit Makefile change**

```bash
git add Makefile
git commit -m "build: add coverage-check target enforcing 80% threshold"
```

---

## Chunk 2: context package coverage

### Task 2: Cover buildSnippets, buildDecisions, buildNotes, buildSiblingOutcomes

**Files:**
- Modify: `internal/context/context_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/context/context_test.go`:

```go
func TestAssembleContext_UnknownIssue(t *testing.T) {
    state := materialize.NewState()
    _, err := Assemble("MISSING-001", "/tmp/fake", state)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "MISSING-001")
}

func TestBuildSnippets_WithContext(t *testing.T) {
    state := materialize.NewState()
    state.Issues["TST-001"] = &materialize.Issue{
        ID:           "TST-001",
        Title:        "Test",
        Type:         "task",
        Status:       "open",
        Context:      []byte(`{"key": "value", "foo": "bar"}`),
        Children:     []string{},
        BlockedBy:    []string{},
        Blocks:       []string{},
        DecisionRefs: []string{},
    }

    ctx, err := Assemble("TST-001", "/tmp/fake", state)
    require.NoError(t, err)

    var snippetsLayer *Layer
    for i := range ctx.Layers {
        if ctx.Layers[i].Name == "snippets" {
            snippetsLayer = &ctx.Layers[i]
            break
        }
    }
    require.NotNil(t, snippetsLayer)
    assert.Contains(t, snippetsLayer.Content, "key")
    assert.Contains(t, snippetsLayer.Content, "value")
}

func TestBuildSnippets_InvalidJSON(t *testing.T) {
    state := materialize.NewState()
    state.Issues["TST-001"] = &materialize.Issue{
        ID:           "TST-001",
        Title:        "Test",
        Type:         "task",
        Status:       "open",
        Context:      []byte(`not json`),
        Children:     []string{},
        BlockedBy:    []string{},
        Blocks:       []string{},
        DecisionRefs: []string{},
    }

    ctx, err := Assemble("TST-001", "/tmp/fake", state)
    require.NoError(t, err)

    for _, l := range ctx.Layers {
        if l.Name == "snippets" {
            assert.Empty(t, l.Content)
        }
    }
}

func TestBuildDecisions(t *testing.T) {
    state := materialize.NewState()
    state.Issues["TST-001"] = &materialize.Issue{
        ID:     "TST-001",
        Title:  "Test",
        Type:   "task",
        Status: "open",
        Decisions: []materialize.Decision{
            {Topic: "db", Choice: "postgres", Rationale: "mature", Timestamp: 100},
        },
        Children:     []string{},
        BlockedBy:    []string{},
        Blocks:       []string{},
        DecisionRefs: []string{},
    }

    ctx, err := Assemble("TST-001", "/tmp/fake", state)
    require.NoError(t, err)

    var decLayer *Layer
    for i := range ctx.Layers {
        if ctx.Layers[i].Name == "decisions" {
            decLayer = &ctx.Layers[i]
            break
        }
    }
    require.NotNil(t, decLayer)
    assert.Contains(t, decLayer.Content, "db")
    assert.Contains(t, decLayer.Content, "postgres")
    assert.Contains(t, decLayer.Content, "mature")
}

func TestBuildNotes_WithNotes(t *testing.T) {
    state := materialize.NewState()
    state.Issues["TST-001"] = &materialize.Issue{
        ID:     "TST-001",
        Title:  "Test",
        Type:   "task",
        Status: "open",
        Notes: []materialize.Note{
            {WorkerID: "w1", Msg: "first note", Timestamp: 1000},
            {WorkerID: "w1", Msg: "second note", Timestamp: 2000},
        },
        Children:     []string{},
        BlockedBy:    []string{},
        Blocks:       []string{},
        DecisionRefs: []string{},
    }

    ctx, err := Assemble("TST-001", "/tmp/fake", state)
    require.NoError(t, err)

    var notesLayer *Layer
    for i := range ctx.Layers {
        if ctx.Layers[i].Name == "notes" {
            notesLayer = &ctx.Layers[i]
            break
        }
    }
    require.NotNil(t, notesLayer)
    assert.Contains(t, notesLayer.Content, "first note")
    assert.Contains(t, notesLayer.Content, "second note")
}

func TestBuildNotes_TruncatesAtFive(t *testing.T) {
    state := materialize.NewState()
    notes := make([]materialize.Note, 7)
    for i := range notes {
        notes[i] = materialize.Note{WorkerID: "w1", Msg: fmt.Sprintf("note-%d", i), Timestamp: int64(i * 100)}
    }
    state.Issues["TST-001"] = &materialize.Issue{
        ID:           "TST-001",
        Title:        "Test",
        Type:         "task",
        Status:       "open",
        Notes:        notes,
        Children:     []string{},
        BlockedBy:    []string{},
        Blocks:       []string{},
        DecisionRefs: []string{},
    }

    ctx, err := Assemble("TST-001", "/tmp/fake", state)
    require.NoError(t, err)

    var notesLayer *Layer
    for i := range ctx.Layers {
        if ctx.Layers[i].Name == "notes" {
            notesLayer = &ctx.Layers[i]
            break
        }
    }
    require.NotNil(t, notesLayer)
    // Should contain last 5 notes (note-2 through note-6), not note-0 or note-1
    assert.Contains(t, notesLayer.Content, "note-6")
    assert.Contains(t, notesLayer.Content, "note-2")
    assert.NotContains(t, notesLayer.Content, "note-0")
    assert.NotContains(t, notesLayer.Content, "note-1")
}

func TestBuildSiblingOutcomes(t *testing.T) {
    state := materialize.NewState()
    state.Issues["TST-P"] = &materialize.Issue{
        ID:       "TST-P",
        Title:    "Parent",
        Type:     "story",
        Status:   "in-progress",
        Children: []string{"TST-A", "TST-B"},
        BlockedBy: []string{},
        Blocks:    []string{},
        DecisionRefs: []string{},
    }
    state.Issues["TST-A"] = &materialize.Issue{
        ID:           "TST-A",
        Title:        "Task A",
        Type:         "task",
        Status:       "done",
        Outcome:      "completed A",
        Parent:       "TST-P",
        Children:     []string{},
        BlockedBy:    []string{},
        Blocks:       []string{},
        DecisionRefs: []string{},
    }
    state.Issues["TST-B"] = &materialize.Issue{
        ID:           "TST-B",
        Title:        "Task B",
        Type:         "task",
        Status:       "open",
        Parent:       "TST-P",
        Children:     []string{},
        BlockedBy:    []string{},
        Blocks:       []string{},
        DecisionRefs: []string{},
    }

    ctx, err := Assemble("TST-B", "/tmp/fake", state)
    require.NoError(t, err)

    var sibLayer *Layer
    for i := range ctx.Layers {
        if ctx.Layers[i].Name == "sibling_outcomes" {
            sibLayer = &ctx.Layers[i]
            break
        }
    }
    require.NotNil(t, sibLayer)
    assert.Contains(t, sibLayer.Content, "TST-A")
    assert.Contains(t, sibLayer.Content, "completed A")
    // TST-B (self) and non-done siblings should not appear
    assert.NotContains(t, sibLayer.Content, "TST-B")
}

func TestBuildSiblingOutcomes_NoParent(t *testing.T) {
    state := materialize.NewState()
    state.Issues["TST-001"] = &materialize.Issue{
        ID:           "TST-001",
        Title:        "Task",
        Type:         "task",
        Status:       "open",
        Children:     []string{},
        BlockedBy:    []string{},
        Blocks:       []string{},
        DecisionRefs: []string{},
    }

    ctx, err := Assemble("TST-001", "/tmp/fake", state)
    require.NoError(t, err)

    for _, l := range ctx.Layers {
        if l.Name == "sibling_outcomes" {
            assert.Empty(t, l.Content)
        }
    }
}
```

Note: this test file uses `fmt` — add to the import block:
```go
import (
    "fmt"
    "strings"
    "testing"
    ...
)
```

- [ ] **Step 2: Run tests and verify they pass**

Run: `go test ./internal/context/... -v -run "TestBuildSnippets|TestBuildDecisions|TestBuildNotes|TestBuildSiblingOutcomes|TestAssembleContext_UnknownIssue"`
Expected: All PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/context/context_test.go
git commit -m "test(context): add coverage for buildSnippets, decisions, notes, siblings"
```

### Task 3: Cover RenderAgent and RenderHuman

**Files:**
- Modify: `internal/context/context_test.go`

- [ ] **Step 1: Add render tests**

```go
func TestRenderAgent(t *testing.T) {
    ctx := &Context{
        IssueID: "TST-001",
        Layers: []Layer{
            {Name: "core_spec", Priority: 1, Content: "Issue: Fix bug"},
        },
    }

    out, err := RenderAgent(ctx)
    require.NoError(t, err)
    assert.Contains(t, out, "TST-001")
    assert.Contains(t, out, "core_spec")
    assert.Contains(t, out, "Fix bug")
    // Should be valid JSON
    assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "{"))
}

func TestRenderHuman(t *testing.T) {
    ctx := &Context{
        IssueID: "TST-001",
        Layers: []Layer{
            {Name: "core_spec", Priority: 1, Content: "Issue: Fix bug"},
            {Name: "notes", Priority: 6, Content: "Some note"},
        },
    }

    out := RenderHuman(ctx)
    assert.Contains(t, out, "=== core_spec ===")
    assert.Contains(t, out, "Issue: Fix bug")
    assert.Contains(t, out, "=== notes ===")
    assert.Contains(t, out, "Some note")
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/context/... -v -run "TestRenderAgent|TestRenderHuman"`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/context/context_test.go
git commit -m "test(context): add RenderAgent and RenderHuman tests"
```

### Task 4: Kill LIVED mutations in context/truncate.go (boundary conditions)

**Files:**
- Modify: `internal/context/context_test.go`

The LIVED mutations at truncate.go:20:19, 20:47, and 24:26 survive because the current test doesn't check:
1. When `totalChars() == charBudget` (should NOT truncate — boundary of `>`)
2. When only 1 layer remains (should NOT remove it — boundary of `> 1`)
3. Two layers with equal priority (last-wins behavior)

- [ ] **Step 1: Add boundary tests for Truncate**

```go
func TestTruncate_ExactlyAtBudget_NoTruncation(t *testing.T) {
    // 100 chars, budget = 25 tokens * 4 = 100 chars — exactly at boundary, no truncation
    ctx := &Context{
        IssueID: "TST-001",
        Layers: []Layer{
            {Name: "core_spec", Priority: 1, Content: strings.Repeat("a", 60)},
            {Name: "notes", Priority: 6, Content: strings.Repeat("b", 40)},
        },
    }

    result := Truncate(ctx, 25) // charBudget = 100, total = 100
    // At exact boundary (==), should NOT truncate
    assert.Len(t, result.Layers, 2, "should not truncate when total == charBudget")
}

func TestTruncate_OneBelowBudget_NoTruncation(t *testing.T) {
    // 99 chars, budget = 25 tokens * 4 = 100 chars
    ctx := &Context{
        IssueID: "TST-001",
        Layers: []Layer{
            {Name: "core_spec", Priority: 1, Content: strings.Repeat("a", 59)},
            {Name: "notes", Priority: 6, Content: strings.Repeat("b", 40)},
        },
    }

    result := Truncate(ctx, 25) // charBudget = 100, total = 99
    assert.Len(t, result.Layers, 2)
}

func TestTruncate_SingleLayer_NeverRemoved(t *testing.T) {
    // Even over budget, the last layer is always kept
    ctx := &Context{
        IssueID: "TST-001",
        Layers: []Layer{
            {Name: "core_spec", Priority: 1, Content: strings.Repeat("a", 1000)},
        },
    }

    result := Truncate(ctx, 1) // charBudget = 4, total = 1000
    assert.Len(t, result.Layers, 1, "single layer must never be removed")
    assert.Equal(t, "core_spec", result.Layers[0].Name)
}

func TestTruncate_EqualPriority_RemovesHigherIndex(t *testing.T) {
    // Two layers with equal priority — one gets removed, core_spec (p1) survives
    ctx := &Context{
        IssueID: "TST-001",
        Layers: []Layer{
            {Name: "core_spec", Priority: 1, Content: strings.Repeat("a", 60)},
            {Name: "decisions", Priority: 5, Content: strings.Repeat("b", 60)},
            {Name: "notes", Priority: 5, Content: strings.Repeat("c", 60)},
        },
    }

    result := Truncate(ctx, 30) // charBudget = 120, total = 180 — must remove one
    assert.Len(t, result.Layers, 2)
    // core_spec must survive
    found := false
    for _, l := range result.Layers {
        if l.Name == "core_spec" {
            found = true
        }
    }
    assert.True(t, found, "core_spec (priority 1) must survive truncation")
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/context/... -v -run "TestTruncate"`
Expected: All PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/context/context_test.go
git commit -m "test(context): add Truncate boundary condition tests"
```

---

## Chunk 3: materialize package coverage

### Task 5: Cover MaterializeAndReturn and appendUnique

**Files:**
- Modify: `internal/materialize/engine_test.go`

- [ ] **Step 1: Add tests**

```go
func TestMaterializeAndReturn_BasicPipeline(t *testing.T) {
    dir := t.TempDir()
    opsDir := filepath.Join(dir, "ops")
    stateDir := filepath.Join(dir, "state")
    issuesDir := filepath.Join(stateDir, "issues")
    os.MkdirAll(opsDir, 0755)
    os.MkdirAll(issuesDir, 0755)

    logPath := filepath.Join(opsDir, "worker-b1.log")
    ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
        WorkerID: "worker-b1", Payload: ops.Payload{Title: "My Task", NodeType: "task"}})

    state, result, err := MaterializeAndReturn(dir, true)
    require.NoError(t, err)
    assert.Equal(t, 1, result.IssueCount)
    require.NotNil(t, state)
    assert.Contains(t, state.Issues, "task-01")
    assert.Equal(t, "My Task", state.Issues["task-01"].Title)
}

func TestMaterializeAndReturn_EmptyDir(t *testing.T) {
    dir := t.TempDir()
    // No ops dir — should return empty state
    state, result, err := MaterializeAndReturn(dir, false)
    require.NoError(t, err)
    assert.NotNil(t, state)
    assert.Equal(t, 0, result.IssueCount)
    assert.Equal(t, 0, result.OpsProcessed)
}

func TestAppendUnique_AddsNew(t *testing.T) {
    slice := []string{"a", "b"}
    result := appendUnique(slice, "c")
    assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestAppendUnique_SkipsDuplicate(t *testing.T) {
    slice := []string{"a", "b", "c"}
    result := appendUnique(slice, "b")
    assert.Equal(t, []string{"a", "b", "c"}, result)
    assert.Len(t, result, 3, "duplicate should not be added")
}

func TestRunRollup_PromotesStoryWhenAllChildrenMerged(t *testing.T) {
    state := NewState()
    state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
        WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}})
    state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
        WorkerID: "w1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "story-01"}})
    state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
        WorkerID: "w1", Payload: ops.Payload{TTL: 60}})

    // In single branch mode, done → merged
    state.SingleBranchMode = true
    state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
        WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "done"}})

    state.RunRollup()
    assert.Equal(t, "merged", state.Issues["story-01"].Status)
}

func TestRunRollup_DoesNotPromoteWithUnmergedChild(t *testing.T) {
    state := NewState()
    state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
        WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}})
    state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
        WorkerID: "w1", Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}})
    state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 102,
        WorkerID: "w1", Payload: ops.Payload{Title: "Task B", NodeType: "task", Parent: "story-01"}})

    state.SingleBranchMode = true
    // Only task-01 done
    state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
        WorkerID: "w1", Payload: ops.Payload{TTL: 60}})
    state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
        WorkerID: "w1", Payload: ops.Payload{To: "done"}})

    state.RunRollup()
    assert.NotEqual(t, "merged", state.Issues["story-01"].Status, "story should not be merged with open task-02")
}

func TestApplyTransition_ReopenClearsPriorOutcome(t *testing.T) {
    state := NewState()
    state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
        WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
    state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
        WorkerID: "w1", Payload: ops.Payload{TTL: 60}})
    state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
        WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "First attempt done"}})
    state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 400,
        WorkerID: "w1", Payload: ops.Payload{To: "open"}})

    issue := state.Issues["task-01"]
    assert.Equal(t, "open", issue.Status)
    assert.Empty(t, issue.Outcome, "outcome should be cleared on reopen")
    assert.Contains(t, issue.PriorOutcomes, "First attempt done")
}

func TestPromoteParentToInProgress_SkipsAlreadyInProgress(t *testing.T) {
    state := NewState()
    state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
        WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}})
    state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
        WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task", Parent: "story-01"}})

    // First claim promotes story-01 to in-progress
    state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
        WorkerID: "w1", Payload: ops.Payload{TTL: 60}})
    assert.Equal(t, "in-progress", state.Issues["story-01"].Status)

    // Second claim (different worker, same task) should not error or corrupt state
    state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 300,
        WorkerID: "w2", Payload: ops.Payload{TTL: 60}})
    assert.Equal(t, "in-progress", state.Issues["story-01"].Status)
}

func TestSortOpsByTimestamp(t *testing.T) {
    allOps := []ops.Op{
        {Timestamp: 300, WorkerID: "w1"},
        {Timestamp: 100, WorkerID: "w1"},
        {Timestamp: 200, WorkerID: "w1"},
    }
    sortOpsByTimestamp(allOps)
    assert.Equal(t, int64(100), allOps[0].Timestamp)
    assert.Equal(t, int64(200), allOps[1].Timestamp)
    assert.Equal(t, int64(300), allOps[2].Timestamp)
}

func TestSortOpsByTimestamp_StableOnEqualTimestamp(t *testing.T) {
    // Equal timestamps should remain in original order (stable sort)
    allOps := []ops.Op{
        {Timestamp: 100, WorkerID: "w2", Type: "first"},
        {Timestamp: 100, WorkerID: "w1", Type: "second"},
    }
    sortOpsByTimestamp(allOps)
    // Insertion sort is stable for equal values — original order preserved
    assert.Equal(t, "first", allOps[0].Type)
    assert.Equal(t, "second", allOps[1].Type)
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/materialize/... -v -run "TestMaterializeAndReturn|TestAppendUnique|TestRunRollup|TestApplyTransition_Reopen|TestPromoteParent|TestSortOps"`
Expected: All PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/materialize/engine_test.go
git commit -m "test(materialize): cover MaterializeAndReturn, appendUnique, RunRollup boundaries"
```

---

## Chunk 4: ready package coverage and boundaries

### Task 6: Cover isClaimStale boundaries, depth, and assignmentTier

**Files:**
- Modify: `internal/ready/ready_test.go`

Look at the existing test file to understand its structure before adding. It should be in `internal/ready/ready_test.go`.

- [ ] **Step 1: Check existing test structure**

Run: `head -20 internal/ready/ready_test.go`

- [ ] **Step 2: Add boundary and missing coverage tests**

Add to `internal/ready/ready_test.go`:

```go
func TestIsClaimStale_ExactBoundary_NotStale(t *testing.T) {
    // claimedAt=0, ttl=1min, now=60 — exactly at boundary, should NOT be stale
    assert.False(t, isClaimStale(0, 0, 1, 60), "at exact TTL boundary should not be stale")
}

func TestIsClaimStale_OnePastBoundary_IsStale(t *testing.T) {
    // now=61 (1 second past 1-minute TTL)
    assert.True(t, isClaimStale(0, 0, 1, 61))
}

func TestIsClaimStale_ZeroTTL_NeverStale(t *testing.T) {
    assert.False(t, isClaimStale(0, 0, 0, 99999))
}

func TestIsClaimStale_HeartbeatExtends(t *testing.T) {
    // Claimed at 0, heartbeat at 100, TTL=1min
    // Without heartbeat: stale at now=61
    // With heartbeat: not stale until now=160
    assert.False(t, isClaimStale(0, 100, 1, 130))
    assert.True(t, isClaimStale(0, 100, 1, 161))
}

func TestDepth_DeepChain_CapsAt20(t *testing.T) {
    index := make(materialize.Index)
    // Build a chain deeper than 20
    for i := 0; i < 25; i++ {
        id := fmt.Sprintf("issue-%02d", i)
        parent := ""
        if i > 0 {
            parent = fmt.Sprintf("issue-%02d", i-1)
        }
        index[id] = materialize.IndexEntry{Parent: parent}
    }

    d := depth("issue-24", index)
    assert.Equal(t, 21, d, "depth should cap at 21 to break cycles")
    // Note: depth increments d before checking > 20, so cap is 21 not 20
}

func TestDepth_NoParent(t *testing.T) {
    index := materialize.Index{
        "task-01": {Parent: ""},
    }
    assert.Equal(t, 0, depth("task-01", index))
}

func TestDepth_MissingFromIndex(t *testing.T) {
    index := materialize.Index{}
    assert.Equal(t, 0, depth("missing", index))
}

func TestAssignmentTier_AssignedToMe(t *testing.T) {
    index := materialize.Index{
        "T-001": {AssignedWorker: "worker-x"},
    }
    assert.Equal(t, 0, assignmentTier("T-001", "worker-x", index))
}

func TestAssignmentTier_Unassigned(t *testing.T) {
    index := materialize.Index{
        "T-001": {AssignedWorker: ""},
    }
    assert.Equal(t, 1, assignmentTier("T-001", "worker-x", index))
}

func TestAssignmentTier_AssignedToOther(t *testing.T) {
    index := materialize.Index{
        "T-001": {AssignedWorker: "worker-other"},
    }
    assert.Equal(t, 2, assignmentTier("T-001", "worker-x", index))
}

func TestAssignmentTier_NoWorkerContext(t *testing.T) {
    index := materialize.Index{
        "T-001": {AssignedWorker: "worker-x"},
    }
    // Empty workerID means no assignment context — treat as unassigned tier
    assert.Equal(t, 1, assignmentTier("T-001", "", index))
}
```

Also add the `fmt` import if not present, and the `materialize` import if not present.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/ready/... -v -run "TestIsClaimStale|TestDepth|TestAssignmentTier"`
Expected: All PASS.

Note: if `depth` and `assignmentTier` are unexported and the test file is in a different package, use `package ready` (same package) in the test file, or these need to be tested via exported functions. Check the existing test file's package declaration.

- [ ] **Step 4: Commit**

```bash
git add internal/ready/ready_test.go
git commit -m "test(ready): add isClaimStale boundary, depth cycle cap, assignmentTier tests"
```

---

## Chunk 5: cmd/trellis coverage

### Task 7: Cover workers command and helper functions

**Files:**
- Modify: `cmd/trellis/main_test.go`

- [ ] **Step 1: Add unit tests for buildWorkerStatus and lastOpTimestampFromLog**

```go
func TestLastOpTimestampFromLog_Empty(t *testing.T) {
    assert.Equal(t, int64(0), lastOpTimestampFromLog(nil))
    assert.Equal(t, int64(0), lastOpTimestampFromLog([]ops.Op{}))
}

func TestLastOpTimestampFromLog_ReturnsMax(t *testing.T) {
    opsList := []ops.Op{
        {Timestamp: 100},
        {Timestamp: 500},
        {Timestamp: 200},
    }
    assert.Equal(t, int64(500), lastOpTimestampFromLog(opsList))
}

func TestBuildWorkerStatus_ActiveWorker(t *testing.T) {
    now := int64(1000)
    allOps := []ops.Op{
        {Type: ops.OpClaim, TargetID: "T-001", Timestamp: 900, WorkerID: "worker-a",
            Payload: ops.Payload{TTL: 10}}, // TTL 10 min = 600 sec; 900+600=1500 > now(1000) → active
    }
    status := buildWorkerStatus("worker-a", allOps, 60, now)
    assert.Equal(t, "active", status.Status)
    assert.Equal(t, "T-001", status.ActiveIssue)
    assert.Equal(t, "worker-a", status.WorkerID)
}

func TestBuildWorkerStatus_StaleWorker(t *testing.T) {
    now := int64(10000)
    allOps := []ops.Op{
        {Type: ops.OpClaim, TargetID: "T-001", Timestamp: 100, WorkerID: "worker-a",
            Payload: ops.Payload{TTL: 1}}, // TTL 1 min = 60 sec; 100+60=160 < now(10000) → stale
    }
    status := buildWorkerStatus("worker-a", allOps, 60, now)
    assert.Equal(t, "stale", status.Status)
    assert.Empty(t, status.ActiveIssue)
}

func TestBuildWorkerStatus_IdleWorker(t *testing.T) {
    now := int64(1000)
    allOps := []ops.Op{
        {Type: ops.OpNote, TargetID: "T-001", Timestamp: 900, WorkerID: "worker-a"},
    }
    // No claims, but recent op — idle within 2*TTL window
    status := buildWorkerStatus("worker-a", allOps, 1, now) // 2*1min=120s; 1000-900=100 < 120 → idle
    assert.Equal(t, "idle", status.Status)
    assert.Equal(t, int64(900), status.LastOpTime)
}

func TestBuildWorkerStatus_TransitionedClaim_NotActive(t *testing.T) {
    now := int64(10000)
    allOps := []ops.Op{
        {Type: ops.OpClaim, TargetID: "T-001", Timestamp: 100, WorkerID: "worker-a",
            Payload: ops.Payload{TTL: 999}}, // Would be active — but transitioned
        {Type: ops.OpTransition, TargetID: "T-001", Timestamp: 200, WorkerID: "worker-a",
            Payload: ops.Payload{To: "done"}},
    }
    status := buildWorkerStatus("worker-a", allOps, 60, now)
    // Transitioned to done — should not be "active"
    assert.NotEqual(t, "active", status.Status)
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/trellis/... -v -run "TestLastOpTimestamp|TestBuildWorkerStatus"`
Expected: All PASS.

- [ ] **Step 3: Add workers command integration test**

```go
func TestWorkersCommand_EmptyRepo(t *testing.T) {
    repo := initTempRepo(t)
    run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

    _, err := runTrls(t, repo, "init")
    require.NoError(t, err)

    buf := new(bytes.Buffer)
    cmd := newRootCmd()
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"workers", "--repo", repo})

    err = cmd.Execute()
    assert.NoError(t, err)
    assert.Contains(t, buf.String(), "No workers found")
}

func TestWorkersCommand_WithWorker(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    buf := new(bytes.Buffer)
    cmd := newRootCmd()
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"workers", "--repo", repo})

    err = cmd.Execute()
    assert.NoError(t, err)
    // Worker registered but no claims — should be listed
    out := buf.String()
    assert.NotEmpty(t, out)
}

func TestWorkersCommand_JSONOutput(t *testing.T) {
    repo := initTempRepo(t)
    run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
    _, err := runTrls(t, repo, "init")
    require.NoError(t, err)

    buf := new(bytes.Buffer)
    cmd := newRootCmd()
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"workers", "--json", "--repo", repo})

    err = cmd.Execute()
    assert.NoError(t, err)
    // Empty result — should not output anything (no workers)
}
```

- [ ] **Step 4: Run integration tests**

Run: `go test ./cmd/trellis/... -v -run "TestWorkers"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/main_test.go
git commit -m "test(cmd): add workers command and helper function tests"
```

### Task 8: Cover log, assign, heartbeat, decision, link, reopen commands

**Files:**
- Modify: `cmd/trellis/main_test.go`

- [ ] **Step 1: Add command integration tests**

```go
func TestLogCommand_Empty(t *testing.T) {
    repo := initTempRepo(t)
    run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

    _, err := runTrls(t, repo, "init")
    require.NoError(t, err)

    buf := new(bytes.Buffer)
    cmd := newRootCmd()
    cmd.SetOut(buf)
    cmd.SetArgs([]string{"log", "--repo", repo})

    err = cmd.Execute()
    assert.NoError(t, err)
    // Empty log — should succeed with no output
}

func TestLogCommand_WithEntries(t *testing.T) {
    repo := setupRepoWithTask(t)

    out, err := runTrls(t, repo, "log", "--repo", repo)
    require.NoError(t, err)
    // Should contain the create op for task-01
    assert.Contains(t, out, "create")
}

func TestLogCommand_JSONOutput(t *testing.T) {
    repo := setupRepoWithTask(t)

    out, err := runTrls(t, repo, "log", "--json", "--repo", repo)
    require.NoError(t, err)
    // Should output JSONL with type field
    assert.Contains(t, out, `"type"`)
}

func TestLogCommand_FilterByIssue(t *testing.T) {
    repo := setupRepoWithTask(t)

    out, err := runTrls(t, repo, "log", "--issue", "task-01", "--repo", repo)
    require.NoError(t, err)
    assert.Contains(t, out, "task-01")
}

func TestAssignCommand(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    out, err := runTrls(t, repo, "assign", "--issue", "task-01", "--worker", "worker-abc")
    require.NoError(t, err)
    assert.Contains(t, out, "task-01")
}

func TestUnassignCommand(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    _, err = runTrls(t, repo, "assign", "--issue", "task-01", "--worker", "worker-abc")
    require.NoError(t, err)

    out, err := runTrls(t, repo, "unassign", "--issue", "task-01")
    require.NoError(t, err)
    assert.Contains(t, out, "task-01")
}

func TestHeartbeatCommand(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "claim", "--issue", "task-01")
    require.NoError(t, err)

    out, err := runTrls(t, repo, "heartbeat", "--issue", "task-01")
    require.NoError(t, err)
    assert.Contains(t, out, "task-01")
}

func TestDecisionCommand(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "claim", "--issue", "task-01")
    require.NoError(t, err)

    out, err := runTrls(t, repo, "decision", "--issue", "task-01",
        "--topic", "db", "--choice", "postgres", "--rationale", "mature")
    require.NoError(t, err)
    assert.Contains(t, out, "task-01")
}

func TestLinkCommand(t *testing.T) {
    repo := initTempRepo(t)
    run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
    _, err := runTrls(t, repo, "init")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "worker-init")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task A", "--id", "T-A")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "create", "--type", "task", "--title", "Task B", "--id", "T-B")
    require.NoError(t, err)

    out, err := runTrls(t, repo, "link", "--issue", "T-A", "--dep", "T-B", "--rel", "blocked_by")
    require.NoError(t, err)
    assert.Contains(t, out, "T-A")
}

func TestReopenCommand(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "claim", "--issue", "task-01")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--outcome", "done")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "materialize")
    require.NoError(t, err)

    out, err := runTrls(t, repo, "reopen", "--issue", "task-01")
    require.NoError(t, err)
    assert.Contains(t, out, "task-01")
}

func TestLogPayloadSummary(t *testing.T) {
    cases := []struct {
        op      ops.Op
        expect  string
    }{
        {ops.Op{Type: ops.OpCreate, Payload: ops.Payload{Title: "My Task", NodeType: "task"}}, "My Task"},
        {ops.Op{Type: ops.OpClaim, Payload: ops.Payload{TTL: 60}}, "ttl=60"},
        {ops.Op{Type: ops.OpHeartbeat}, ""},
        {ops.Op{Type: ops.OpTransition, Payload: ops.Payload{To: "done", Outcome: "Fixed"}}, "→ done"},
        {ops.Op{Type: ops.OpNote, Payload: ops.Payload{Msg: "progress"}}, "progress"},
        {ops.Op{Type: ops.OpLink, Payload: ops.Payload{Rel: "blocked_by", Dep: "T-002"}}, "blocked_by T-002"},
        {ops.Op{Type: ops.OpDecision, Payload: ops.Payload{Topic: "db", Choice: "pg"}}, "db → pg"},
        {ops.Op{Type: ops.OpAssign, Payload: ops.Payload{AssignedTo: "worker-x"}}, "→ worker-x"},
        {ops.Op{Type: ops.OpAssign, Payload: ops.Payload{AssignedTo: ""}}, "unassigned"},
    }
    for _, tc := range cases {
        out := logPayloadSummary(tc.op)
        assert.Contains(t, out, tc.expect, "op type: %s", tc.op.Type)
    }
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/trellis/... -v -run "TestLog|TestAssign|TestUnassign|TestHeartbeat|TestDecision|TestLink|TestReopen|TestLogPayload"`
Expected: All PASS. Some commands may need `arm worker-init` in the test setup — adjust if tests fail with "worker not found" errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/trellis/main_test.go
git commit -m "test(cmd): add log, assign, heartbeat, decision, link, reopen command tests"
```

---

## Chunk 6: Verify coverage and mutation improvement

### Task 9: Run coverage-check and confirm ≥80%

- [ ] **Step 1: Run coverage check**

Run: `make coverage-check`
Expected: `Total coverage: XX.X%` with XX.X ≥ 80, exit code 0.

If still below 80%, look at the coverage report:
```bash
make coverage
open coverage.html  # or: go tool cover -func=coverage.out | sort -t% -k1 -n | head -30
```

Identify the packages still below threshold. Priority packages to improve:
- `internal/git` (72.6%) — `CurrentBranch` and `CommitMessage` are 0% covered
- `internal/ops` (79.4%) — `WorkerIDFromFilename` is 0%

For `WorkerIDFromFilename` (one-liner utility used in many places, trivially testable):

In `internal/ops/ops_test.go`, add:
```go
func TestWorkerIDFromFilename(t *testing.T) {
    assert.Equal(t, "worker-abc123", WorkerIDFromFilename("/path/to/ops/worker-abc123.log"))
    assert.Equal(t, "worker-abc123", WorkerIDFromFilename("worker-abc123.log"))
}
```

- [ ] **Step 2: Run mutation tests and compare scores**

Run: `make mutate 2>&1 | tail -20`
Expected: Internal test efficacy ≥ 85%, cmd efficacy stays at 100%.

- [ ] **Step 3: Commit final coverage fixes if any**

```bash
git add .
git commit -m "test: final coverage gap fills to meet 80% threshold"
```

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-03-17-test-coverage-hardening.md`. Ready to execute?

**Execution path:** Use superpowers:subagent-driven-development or superpowers:executing-plans.

Tasks 1–3 (Makefile + context) can run in parallel with Tasks 5–8 (materialize + ready + cmd) since they touch different packages.
