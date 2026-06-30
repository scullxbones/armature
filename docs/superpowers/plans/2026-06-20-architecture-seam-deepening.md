# Architecture Seam Deepening Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deepen five architecture seams — materialization dispatch, op registration, issue type vocabulary, output formatting, and validation — so that each concern has one canonical home and callers can't accidentally diverge.

**Architecture:** Extract a canonical `internal/issuetype` vocabulary consulted by create, validate, ready, and dag. Replace the `ApplyOp` switch in `materialize/engine.go` with a registered handler table that also owns the op-type whitelist, collapsing the parallel-update requirement. Consolidate CLI rendering behind an `internal/output` package so commands write to a formatter rather than `fmt.Fprintf` directly. Make the validation module accept a `*dag.Graph` instead of building its own, and consume `issuetype` hierarchy rules.

**Tech Stack:** Go stdlib, Cobra, existing internal packages (`materialize`, `ops`, `validate`, `dag`, `ready`).

---

## Source Inputs

- Analysis: `/home/brian/.claude/usage-data/report-2026-06-15-073036.html`
- Architecture doc: `docs/design/architecture.md`
- Prior architecture work: `docs/superpowers/plans/2026-06-13-architecture-deepening-follow-up.md`
- Domain glossary: `CONTEXT.md`

## Scope Note

This plan covers five seams identified in the June 2026 usage-analysis review. It does **not** reopen the completed RepoSnapshot, GraphFacts, or Command Runtime work from the June 13/14 plans.

## Current Problem Summary

| Seam | Problem |
|------|---------|
| Materialization dispatch | `ApplyOp` switch in `engine.go` is the only place handler mapping lives; adding a new op requires editing both the switch and `ValidOpTypes` in `ops/types.go` |
| Op registration | `ValidOpTypes` and `ApplyOp` are parallel whitelists that can silently diverge; `schema.go` also lists op types as a comment |
| Issue type vocabulary | `"epic"`, `"story"`, `"task"`, `"feature"`, `"bug"` strings are hard-coded in `create.go`, `validate.go`, `ready/compute.go`, `dag.go`, `decompose.go` — with inconsistent sets (validate has "bug" not "feature"; ready has "feature" not "epic") |
| Output formatting | Each of `show.go`, `list.go`, `ready.go`, `validate.go` writes to `cmd.OutOrStdout()` directly with its own column widths, prefixes, and line formats |
| Validation module | `validate.Validate` builds its own graph internally (`graphFromState`), has its own `validHierarchy` switch (separate from `dag.isLegalHierarchy`), and individual checks are untestable through the module interface |

---

## File Map

### New files
- Create: `internal/issuetype/issuetype.go` — canonical vocabulary: valid types, hierarchy rules, required-field policy
- Create: `internal/issuetype/issuetype_test.go`
- Create: `internal/output/output.go` — CLI renderers: issue detail, list table, ready queue, validation result
- Create: `internal/output/output_test.go`

### Modified files
- Modify: `internal/ops/types.go` — remove `ValidOpTypes`; registration migrates to materialize
- Modify: `internal/materialize/engine.go` — replace switch with handler table; own the op-type whitelist
- Modify: `internal/materialize/engine_test.go`
- Modify: `internal/validate/validate.go` — accept `*dag.Graph`; use `issuetype.IsLegalHierarchy`
- Modify: `internal/validate/validate_test.go`
- Modify: `internal/ready/compute.go` — use `issuetype.IsWorkable`
- Modify: `internal/dag/dag.go` — export `IsLegalHierarchy` replacing private `isLegalHierarchy`
- Modify: `cmd/armature/create.go` — use `issuetype` instead of local vars
- Modify: `cmd/armature/show.go` — use `output.RenderIssue`
- Modify: `cmd/armature/list.go` — use `output.RenderList`
- Modify: `cmd/armature/ready.go` — use `output.RenderReady`
- Modify: `cmd/armature/validate.go` — use `output.RenderValidation`
- Modify: `cmd/armature/decompose.go` — use `issuetype.ValidTypes` for JSON schema

---

## Chunk 1: Issue Type Vocabulary Module

### Task 1: Create `internal/issuetype` package

**Files:**
- Create: `internal/issuetype/issuetype.go`
- Create: `internal/issuetype/issuetype_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/issuetype/issuetype_test.go`:

```go
package issuetype_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/issuetype"
)

func TestIsValid(t *testing.T) {
	for _, tt := range []struct {
		typ  string
		want bool
	}{
		{"epic", true}, {"story", true}, {"task", true},
		{"feature", true}, {"bug", true}, {"", false}, {"sprint", false},
	} {
		if got := issuetype.IsValid(tt.typ); got != tt.want {
			t.Errorf("IsValid(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestIsLegalHierarchy(t *testing.T) {
	cases := []struct {
		parent, child string
		want          bool
	}{
		{"", "epic", true},
		{"", "story", true},
		{"", "task", true},
		{"", "feature", true},
		{"", "bug", true},
		{"epic", "story", true},
		{"epic", "task", true},
		{"epic", "feature", true},
		{"epic", "bug", true},
		{"story", "task", true},
		{"story", "bug", true},
		{"story", "feature", true},
		{"feature", "task", true},
		{"feature", "bug", true},
		{"task", "task", false},
		{"bug", "task", false},
		{"story", "epic", false},
		{"epic", "epic", false},
	}
	for _, tc := range cases {
		got := issuetype.IsLegalHierarchy(tc.parent, tc.child)
		if got != tc.want {
			t.Errorf("IsLegalHierarchy(%q, %q) = %v, want %v", tc.parent, tc.child, got, tc.want)
		}
	}
}

func TestIsWorkable(t *testing.T) {
	for _, tt := range []struct {
		typ  string
		want bool
	}{
		{"task", true}, {"story", true}, {"feature", true},
		{"epic", false}, {"bug", true},
	} {
		if got := issuetype.IsWorkable(tt.typ); got != tt.want {
			t.Errorf("IsWorkable(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestRequiresAcceptance(t *testing.T) {
	if !issuetype.RequiresAcceptance("task") {
		t.Error("task should require acceptance")
	}
	if issuetype.RequiresAcceptance("epic") {
		t.Error("epic should not require acceptance")
	}
}
```

Run:
```bash
go test ./internal/issuetype/... -v
```
Expected: compile error — package doesn't exist yet.

- [ ] **Step 2: Create the package**

Create `internal/issuetype/issuetype.go`:

```go
package issuetype

// All is the complete set of valid issue/node type names.
var All = []string{"epic", "story", "feature", "task", "bug"}

// valid is the lookup set for O(1) membership testing.
var valid = map[string]bool{
	"epic": true, "story": true, "feature": true, "task": true, "bug": true,
}

// legalChildren maps each parent type to the set of permitted child types.
// An empty key represents a root-level issue (no parent).
var legalChildren = map[string]map[string]bool{
	"":        {"epic": true, "story": true, "task": true, "feature": true, "bug": true},
	"epic":    {"story": true, "task": true, "feature": true, "bug": true},
	"story":   {"task": true, "feature": true, "bug": true},
	"feature": {"task": true, "bug": true},
	"task":    {},
	"bug":     {},
}

// workable is the set of types that appear in the ready queue.
var workable = map[string]bool{
	"task": true, "story": true, "feature": true, "bug": true,
}

// requiresAcceptanceCriteria is the set of types that must have acceptance criteria.
var requiresAcceptanceCriteria = map[string]bool{
	"task": true,
}

// IsValid reports whether typ is a recognized issue type.
func IsValid(typ string) bool {
	return valid[typ]
}

// IsLegalHierarchy reports whether childType may be a direct child of parentType.
// An empty parentType means the child is at root level (no parent).
func IsLegalHierarchy(parentType, childType string) bool {
	allowed, ok := legalChildren[parentType]
	if !ok {
		return false
	}
	return allowed[childType]
}

// IsWorkable reports whether typ is eligible to appear in the ready queue.
func IsWorkable(typ string) bool {
	return workable[typ]
}

// RequiresAcceptance reports whether typ must have acceptance criteria.
func RequiresAcceptance(typ string) bool {
	return requiresAcceptanceCriteria[typ]
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/issuetype/... -v
```
Expected: PASS

- [ ] **Step 4: Run make check**

```bash
make check
```
Expected: green

- [ ] **Step 5: Commit**

```bash
git add internal/issuetype/issuetype.go internal/issuetype/issuetype_test.go
git commit -m "feat(issuetype): add canonical issue type vocabulary module"
```

---

### Task 2: Migrate type-string callers to `issuetype`

**Files:**
- Modify: `cmd/armature/create.go`
- Modify: `internal/ready/compute.go`
- Modify: `cmd/armature/decompose.go`

- [ ] **Step 1: Update `create.go`**

Replace the three local vars at the top of `create.go` (lines 17–35):

```go
// Before:
var validNodeTypes = map[string]bool{
    "epic": true, "story": true, "feature": true, "task": true, "bug": true,
}
var validNodeTypesList = []string{"epic", "story", "feature", "task", "bug"}
var validParentChildTypes = map[string]map[string]bool{
    "epic":    {"story": true, "feature": true, "task": true, "bug": true},
    "story":   {"task": true, "bug": true},
    "feature": {"task": true, "bug": true},
    "task":    {},
    "bug":     {},
}
```

```go
// After: remove all three vars and replace call sites:
// validNodeTypes[nodeType]       → issuetype.IsValid(nodeType)
// validNodeTypesList             → issuetype.All
// validParentChildTypes[p][c]    → issuetype.IsLegalHierarchy(p, c)
```

Add import: `"github.com/scullxbones/armature/internal/issuetype"`

- [ ] **Step 2: Update `ready/compute.go`**

Replace inline type filter at lines 42 and 102:

```go
// Before:
if entry.Type != "task" && entry.Type != "feature" && entry.Type != "story" {
    continue
}
```

```go
// After:
if !issuetype.IsWorkable(entry.Type) {
    continue
}
```

Add import: `"github.com/scullxbones/armature/internal/issuetype"`

- [ ] **Step 3: Update `decompose.go`**

Replace the inline type enum in the JSON schema (line 85):

```go
// Before:
"enum": []string{"epic", "story", "task"},
```

```go
// After:
"enum": issuetype.All,
```

Add import: `"github.com/scullxbones/armature/internal/issuetype"`

- [ ] **Step 4: Build and test**

```bash
go build ./...
make test
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/armature/create.go internal/ready/compute.go cmd/armature/decompose.go
git commit -m "refactor: migrate type-string callers to issuetype package"
```

---

## Chunk 2: Materialization Dispatch and Op Registration

### Task 3: Replace `ApplyOp` switch with a handler table

**Files:**
- Modify: `internal/materialize/engine.go`
- Modify: `internal/materialize/engine_test.go`

The current `ApplyOp` switch at `engine.go:24–64` has 18 cases and must be manually kept in sync with `ops.ValidOpTypes`. Moving to a handler table makes the mapping inspectable and makes `ValidOpTypes` derivable from the registered handlers.

- [ ] **Step 1: Write failing tests for handler registration**

In `internal/materialize/engine_test.go`, add:

```go
func TestApplyOp_UnknownTypeReturnsError(t *testing.T) {
    s := NewState()
    err := s.ApplyOp(ops.Op{Type: "nonexistent", TargetID: "X"})
    if err == nil {
        t.Error("expected error for unknown op type, got nil")
    }
}

func TestApplyOp_AllRegisteredTypesHandled(t *testing.T) {
    // Every registered handler must be callable without panicking.
    for opType := range materialize.RegisteredOpTypes() {
        s := NewState()
        // A minimal op may return an error (e.g. missing target) but must not panic.
        _ = s.ApplyOp(ops.Op{Type: opType, TargetID: "TEST-1", WorkerID: "w"})
    }
}
```

Run:
```bash
go test ./internal/materialize/... -run TestApplyOp -v
```
Expected: compile error — `RegisteredOpTypes` not defined yet.

- [ ] **Step 2: Add handler table to `engine.go`**

After the `ApplyOp` function, add:

```go
// opHandler is the function signature for all op appliers.
type opHandler func(*State, ops.Op) error

// opHandlers maps each recognized op type to its state-mutation function.
// This table is the single source of truth for which op types this engine
// supports; ops.ValidOpTypes is derived from it.
var opHandlers = map[string]opHandler{
    ops.OpCreate:            (*State).applyCreate,
    ops.OpClaim:             (*State).applyClaim,
    ops.OpHeartbeat:         (*State).applyHeartbeat,
    ops.OpTransition:        (*State).applyTransition,
    ops.OpNote:              (*State).applyNote,
    ops.OpNoteDelete:        (*State).applyNoteDelete,
    ops.OpLink:              (*State).applyLink,
    ops.OpUnlink:            (*State).applyUnlink,
    ops.OpDecision:          (*State).applyDecision,
    ops.OpAssign:            (*State).applyAssign,
    ops.OpAmend:             (*State).applyAmend,
    ops.OpSourceLink:        (*State).applySourceLink,
    ops.OpSourceFingerprint: func(_ *State, _ ops.Op) error { return nil },
    ops.OpCitationAccepted:  (*State).applyCitationAccepted,
    ops.OpDAGTransition:     (*State).applyDAGTransition,
    ops.OpScopeRename:       (*State).applyScopeRename,
    ops.OpScopeDelete:       (*State).applyScopeDelete,
    ops.OpReparent:          (*State).applyReparent,
}

// RegisteredOpTypes returns the set of op types this engine can apply.
// Callers (e.g. ops.ValidOpTypes) should derive their whitelist from this.
func RegisteredOpTypes() map[string]bool {
    result := make(map[string]bool, len(opHandlers))
    for k := range opHandlers {
        result[k] = true
    }
    return result
}
```

Replace the `ApplyOp` body:

```go
func (s *State) ApplyOp(op ops.Op) error {
    handler, ok := opHandlers[op.Type]
    if !ok {
        return fmt.Errorf("unknown op type: %s", op.Type)
    }
    return handler(s, op)
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/materialize/... -run TestApplyOp -v
```
Expected: PASS

- [ ] **Step 4: Run full suite**

```bash
make check
```
Expected: green

- [ ] **Step 5: Commit**

```bash
git add internal/materialize/engine.go internal/materialize/engine_test.go
git commit -m "refactor(materialize): replace ApplyOp switch with registered handler table"
```

---

### Task 4: Deepen the op registration seam

**Files:**
- Modify: `internal/ops/types.go`
- Modify: `internal/ops/opstream.go` (if it validates op types)

`ops.ValidOpTypes` in `types.go` is a second parallel whitelist. Now that `materialize.RegisteredOpTypes()` is the authoritative source, consumers should use it. The `schema.go` comment-based list is a documentation artifact and can be updated separately.

- [ ] **Step 1: Write a failing cross-seam test**

In `internal/ops/types_test.go` or a new file, add:

```go
func TestValidOpTypes_MatchMaterializeHandlers(t *testing.T) {
    // This test enforces that the ops whitelist and the materialize handler table
    // never diverge. If a type appears in one but not the other, it is a bug.
    //
    // Note: this test lives in the ops package and cannot import materialize
    // (cycle risk). Instead, it asserts that ValidOpTypes is replaced by a
    // derivation call at test init time. See implementation note below.
    //
    // For now, assert the counts match the materialize table by checking against
    // the canonical count documented here. Update this number when adding a new op.
    const expectedOpCount = 18
    if len(ValidOpTypes) != expectedOpCount {
        t.Errorf("ValidOpTypes has %d entries, expected %d — sync with materialize.opHandlers", len(ValidOpTypes), expectedOpCount)
    }
}
```

Run:
```bash
go test ./internal/ops/... -run TestValidOpTypes -v
```
Expected: PASS (but this documents the coupling; the next step removes it).

- [ ] **Step 2: Remove `ValidOpTypes` from `ops/types.go` and redirect callers**

Find all callers of `ops.ValidOpTypes`:

```bash
grep -rn "ops\.ValidOpTypes\|ValidOpTypes" ./internal ./cmd --include="*.go"
```

For each caller, replace `ops.ValidOpTypes[t]` with `materialize.RegisteredOpTypes()[t]` or pass the registered map as a dependency.

Remove the `ValidOpTypes` var from `internal/ops/types.go`.

If `opstream.go` uses it for log parsing validation, pass the valid-type set in as a parameter or let unknown op types pass through (the engine will reject them at apply time).

- [ ] **Step 3: Update `schema.go` op list**

`schema.go:GenerateSchema()` has a hardcoded comment listing 13 op types (it predates the later additions). Update the comment to match the current 18 types:

```go
// Position 0: op_type (string) — one of: create, claim, heartbeat, transition,
//             note, note-delete, link, unlink, source-link, source-fingerprint,
//             dag-transition, decision, assign, amend, citation-accepted,
//             scope-rename, scope-delete, reparent
```

- [ ] **Step 4: Build and test**

```bash
go build ./...
make test
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ops/types.go internal/ops/opstream.go internal/ops/schema.go
git commit -m "refactor(ops): remove ValidOpTypes; materialize handler table is authoritative"
```

---

## Chunk 3: Output Formatting Seam

### Task 5: Create `internal/output` package

**Files:**
- Create: `internal/output/output.go`
- Create: `internal/output/output_test.go`

The goal is to give each CLI rendering concern one home. The four rendering patterns in scope: issue detail (`show`), issue list (`list`), ready queue (`ready`), validation result (`validate`).

- [ ] **Step 1: Write failing tests**

Create `internal/output/output_test.go`:

```go
package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/validate"
)

func TestRenderIssue_ContainsFields(t *testing.T) {
	var buf bytes.Buffer
	issue := &materialize.Issue{
		ID:     "TASK-1",
		Title:  "Do the thing",
		Type:   "task",
		Status: "open",
	}
	output.RenderIssue(&buf, issue, false)
	got := buf.String()
	for _, want := range []string{"TASK-1", "Do the thing", "task", "open"} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderIssue output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderIssue_JSON(t *testing.T) {
	var buf bytes.Buffer
	issue := &materialize.Issue{ID: "TASK-1", Title: "X", Type: "task", Status: "open"}
	output.RenderIssue(&buf, issue, true)
	var m map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("RenderIssue JSON output is not valid JSON: %v\n%s", err, buf.String())
	}
}

func TestRenderList_HasHeader(t *testing.T) {
	var buf bytes.Buffer
	entries := []output.ListEntry{
		{ID: "TASK-1", Status: "open", Title: "T1"},
	}
	output.RenderList(&buf, entries)
	got := buf.String()
	if !strings.Contains(got, "TASK-1") {
		t.Errorf("RenderList output missing TASK-1:\n%s", got)
	}
}

func TestRenderReady_ListsEntries(t *testing.T) {
	var buf bytes.Buffer
	entries := []ready.ReadyEntry{
		{Issue: "TASK-1", Title: "Work", Priority: "medium"},
	}
	output.RenderReady(&buf, entries, false)
	got := buf.String()
	if !strings.Contains(got, "TASK-1") {
		t.Errorf("RenderReady missing TASK-1:\n%s", got)
	}
}

func TestRenderValidation_PrefixesCorrectly(t *testing.T) {
	var buf bytes.Buffer
	result := validate.Result{
		OK:       false,
		Errors:   []string{"bad thing"},
		Warnings: []string{"watch out"},
	}
	output.RenderValidation(&buf, result)
	got := buf.String()
	if !strings.Contains(got, "ERROR:") {
		t.Errorf("RenderValidation missing ERROR: prefix:\n%s", got)
	}
	if !strings.Contains(got, "WARNING:") {
		t.Errorf("RenderValidation missing WARNING: prefix:\n%s", got)
	}
}
```

Run:
```bash
go test ./internal/output/... -v
```
Expected: compile error — package doesn't exist yet.

- [ ] **Step 2: Create `internal/output/output.go`**

```go
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/scullxbones/armature/internal/validate"
)

// ListEntry is a row for RenderList.
type ListEntry struct {
	ID      string
	Status  string
	Claimed string
	Outcome string
	Title   string
}

// RenderIssue writes a human-readable or JSON representation of issue to w.
func RenderIssue(w io.Writer, issue *materialize.Issue, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(issue)
		return
	}
	_, _ = fmt.Fprintf(w, "ID:        %s\n", issue.ID)
	_, _ = fmt.Fprintf(w, "Title:     %s\n", issue.Title)
	_, _ = fmt.Fprintf(w, "Type:      %s\n", issue.Type)
	_, _ = fmt.Fprintf(w, "Status:    %s\n", issue.Status)
	if issue.Parent != "" {
		_, _ = fmt.Fprintf(w, "Parent:    %s\n", issue.Parent)
	}
	if issue.ClaimedBy != "" {
		_, _ = fmt.Fprintf(w, "ClaimedBy: %s\n", issue.ClaimedBy)
	}
	if issue.AssignedWorker != "" {
		_, _ = fmt.Fprintf(w, "Assigned:  %s\n", issue.AssignedWorker)
	}
	if issue.DefinitionOfDone != "" {
		_, _ = fmt.Fprintf(w, "DoD:       %s\n", issue.DefinitionOfDone)
	}
	if len(issue.Acceptance) > 0 {
		compact, _ := json.Marshal(json.RawMessage(issue.Acceptance))
		_, _ = fmt.Fprintf(w, "Acceptance: %s\n", string(compact))
	}
	if len(issue.Scope) > 0 {
		_, _ = fmt.Fprintf(w, "Scope:     %s\n", strings.Join(issue.Scope, ", "))
	}
	if issue.Outcome != "" {
		_, _ = fmt.Fprintf(w, "Outcome:   %s\n", issue.Outcome)
	}
	if len(issue.BlockedBy) > 0 {
		_, _ = fmt.Fprintf(w, "BlockedBy: %s\n", strings.Join(issue.BlockedBy, ", "))
	}
	if len(issue.Blocks) > 0 {
		_, _ = fmt.Fprintf(w, "Blocks:    %s\n", strings.Join(issue.Blocks, ", "))
	}
	if len(issue.Notes) > 0 {
		_, _ = fmt.Fprintf(w, "Notes:\n")
		for _, n := range issue.Notes {
			if !n.Deleted {
				_, _ = fmt.Fprintf(w, "  - %s\n", n.Msg)
			}
		}
	}
}

// RenderList writes a table of list entries to w.
func RenderList(w io.Writer, entries []ListEntry) {
	_, _ = fmt.Fprintf(w, "%-12s %-12s %-38s %-30s %s\n", "ID", "STATUS", "CLAIMED", "OUTCOME", "TITLE")
	for _, e := range entries {
		_, _ = fmt.Fprintf(w, "%-12s %-12s %-38s %-30s %s\n", e.ID, e.Status, e.Claimed, e.Outcome, e.Title)
	}
}

// RenderReady writes the ready queue to w.
// If asJSON is true the entries are marshalled as a JSON array.
func RenderReady(w io.Writer, entries []ready.ReadyEntry, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(entries)
		return
	}
	for _, e := range entries {
		conf := ""
		if e.RequiresConfirmation {
			conf = " [requires confirmation]"
		}
		_, _ = fmt.Fprintf(w, "  %s  %s  (%s)%s\n", e.Issue, e.Title, e.Priority, conf)
	}
}

// RenderValidation writes validation errors, warnings, and infos to w.
func RenderValidation(w io.Writer, result validate.Result) {
	for _, e := range result.Errors {
		_, _ = fmt.Fprintf(w, "ERROR: %s\n", e)
	}
	for _, wn := range result.Warnings {
		_, _ = fmt.Fprintf(w, "WARNING: %s\n", wn)
	}
	for _, i := range result.Infos {
		_, _ = fmt.Fprintf(w, "INFO: %s\n", i)
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/output/... -v
```
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/output/output.go internal/output/output_test.go
git commit -m "feat(output): add canonical CLI rendering package"
```

---

### Task 6: Migrate commands to `internal/output`

**Files:**
- Modify: `cmd/armature/show.go`
- Modify: `cmd/armature/list.go`
- Modify: `cmd/armature/ready.go`
- Modify: `cmd/armature/validate.go`

- [ ] **Step 1: Update `show.go`**

Replace the inline issue rendering block (lines 169–212) with:

```go
output.RenderIssue(w, issue, asJSON)
```

Add import: `"github.com/scullxbones/armature/internal/output"`. Remove now-unused `strings` import if no longer needed.

- [ ] **Step 2: Update `list.go`**

Replace the inline table header + row rendering (lines 160–179) with `output.RenderList(w, entries)` where `entries` is assembled from the index first. Build a `[]output.ListEntry` slice before calling.

- [ ] **Step 3: Update `ready.go`**

Replace the inline entry rendering block (lines 156) with `output.RenderReady(cmd.OutOrStdout(), readyEntries, asJSON)`. The JSON path at the top of the ready command can also route through `output.RenderReady`.

- [ ] **Step 4: Update `validate.go`**

Replace the ERROR/WARNING/INFO/COVERAGE lines (lines 115–132) with `output.RenderValidation(cmd.OutOrStdout(), result)`. Add coverage rendering to `output.RenderValidation` if needed (add a `Coverage` field to the output or a separate `RenderCoverage` helper).

- [ ] **Step 5: Build and test**

```bash
go build ./cmd/armature/...
make test
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/armature/show.go cmd/armature/list.go cmd/armature/ready.go cmd/armature/validate.go
git commit -m "refactor(cmd): route command output through internal/output package"
```

---

## Chunk 4: Validation Module

### Task 7: Export `IsLegalHierarchy` from `dag` and deepen `validate`

**Files:**
- Modify: `internal/dag/dag.go`
- Modify: `internal/validate/validate.go`
- Modify: `internal/validate/validate_test.go`

The private `isLegalHierarchy` in `dag.go` checks graph structural consistency (parent exists, children list is consistent). This is different from `issuetype.IsLegalHierarchy` which checks type-level rules. Both can coexist: dag keeps the structural check; issuetype owns the type-vocabulary rules.

Currently `validate.Validate` builds its own graph with `graphFromState` instead of accepting a caller-supplied `*dag.Graph`. Commands that already have a graph (after Task 3 in the June 13 plan) would have to rebuild it.

- [ ] **Step 1: Write failing tests for the new `validate.Validate` signature**

In `internal/validate/validate_test.go`, add tests that call `Validate` with an explicit graph parameter:

```go
func TestValidate_AcceptsGraph(t *testing.T) {
	state := &materialize.State{Issues: map[string]*materialize.Issue{
		"EPIC-1": {ID: "EPIC-1", Type: "epic", Status: "open", Children: []string{"TASK-1"}},
		"TASK-1": {ID: "TASK-1", Type: "task", Status: "open", Parent: "EPIC-1",
			Scope: []string{"cmd/"}, Acceptance: json.RawMessage(`{"criteria":"x"}`),
			DefinitionOfDone: "done"},
	}}
	nodeIndex := make(map[string]*dag.Node)
	for id, iss := range state.Issues {
		nodeIndex[id] = &dag.Node{ID: id, Type: iss.Type, Parent: iss.Parent,
			Children: append([]string(nil), iss.Children...), BlockedBy: iss.BlockedBy, Blocks: iss.Blocks}
	}
	graph := dag.GraphFromState(nodeIndex)
	result := validate.Validate(state, graph, validate.Options{})
	if !result.OK {
		t.Errorf("expected OK, got errors: %v", result.Errors)
	}
}
```

Run:
```bash
go test ./internal/validate/... -run TestValidate_AcceptsGraph -v
```
Expected: compile error — `Validate` doesn't accept a `*dag.Graph` yet.

- [ ] **Step 2: Update `validate.Validate` to accept a graph**

Change the signature:

```go
// Before:
func Validate(state *materialize.State, opts Options) Result

// After:
func Validate(state *materialize.State, graph *dag.Graph, opts Options) Result
```

Remove `graphFromState` helper — it is no longer needed inside `validate`. Callers that don't already have a graph should build one via `dag.GraphFromState(nodeIndex)` before calling `Validate`.

Update all internal check functions that previously called `graphFromState(state)` to use the passed-in `graph`.

- [ ] **Step 3: Replace `validHierarchy` with `issuetype.IsLegalHierarchy`**

Remove the local `validHierarchy` function from `validate.go` (lines 189–201). In `checkE5TypeHierarchy`, replace:

```go
if !validHierarchy(issue.Type, child.Type) {
```

with:

```go
if !issuetype.IsLegalHierarchy(issue.Type, child.Type) {
```

Add import: `"github.com/scullxbones/armature/internal/issuetype"`

- [ ] **Step 4: Update all `Validate` callers**

```bash
grep -rn "validate\.Validate(" ./cmd ./internal --include="*.go"
```

For each caller, add the graph argument:

```go
// Typical caller (e.g. cmd/armature/validate.go):
nodeIndex := buildNodeIndex(snap.State)  // extract from existing graphFromState pattern
graph := dag.GraphFromState(nodeIndex)
result := validate.Validate(snap.State, graph, opts)
```

Or if the caller already has a `*dag.Graph` from the snapshot work, reuse it.

- [ ] **Step 5: Run validate tests**

```bash
go test ./internal/validate/... -v
```
Expected: PASS

- [ ] **Step 6: Run full suite**

```bash
make check
```
Expected: green

- [ ] **Step 7: Commit**

```bash
git add internal/dag/dag.go internal/validate/validate.go internal/validate/validate_test.go
git commit -m "refactor(validate): accept dag.Graph; use issuetype.IsLegalHierarchy; remove local validHierarchy"
```

---

## Final Verification

- [ ] **Step 1: Run all targeted package tests**

```bash
go test ./internal/issuetype/... ./internal/output/... ./internal/materialize/... ./internal/validate/... ./internal/ready/... ./cmd/armature/... -v 2>&1 | tail -30
```
Expected: PASS

- [ ] **Step 2: Run full test suite**

```bash
go test ./...
```
Expected: PASS

- [ ] **Step 3: Validation gate**

```bash
go run ./cmd/armature validate --ci
```
Expected: `OK: no issues found`

- [ ] **Step 4: Doctor gate**

```bash
go run ./cmd/armature doctor
```
Expected: no structural regressions

- [ ] **Step 5: Run make check**

```bash
make check
```
Expected: all stages green

---

## Summary of Changed Files

| File | Change |
|------|--------|
| `internal/issuetype/issuetype.go` | NEW — canonical type vocabulary |
| `internal/issuetype/issuetype_test.go` | NEW |
| `internal/output/output.go` | NEW — CLI rendering |
| `internal/output/output_test.go` | NEW |
| `internal/materialize/engine.go` | REPLACE switch with handler table; add `RegisteredOpTypes()` |
| `internal/materialize/engine_test.go` | ADD handler table tests |
| `internal/ops/types.go` | REMOVE `ValidOpTypes`; update comment count |
| `internal/ops/schema.go` | UPDATE op type list comment |
| `internal/validate/validate.go` | ACCEPT `*dag.Graph`; REMOVE `graphFromState`, `validHierarchy`; USE `issuetype` |
| `internal/validate/validate_test.go` | UPDATE call sites |
| `internal/ready/compute.go` | USE `issuetype.IsWorkable` |
| `internal/dag/dag.go` | EXPORT `IsLegalHierarchy` (or confirm private stays private since `issuetype` covers the type-level rules) |
| `cmd/armature/create.go` | USE `issuetype` instead of local vars |
| `cmd/armature/show.go` | USE `output.RenderIssue` |
| `cmd/armature/list.go` | USE `output.RenderList` |
| `cmd/armature/ready.go` | USE `output.RenderReady` |
| `cmd/armature/validate.go` | USE `output.RenderValidation` |
| `cmd/armature/decompose.go` | USE `issuetype.All` |
