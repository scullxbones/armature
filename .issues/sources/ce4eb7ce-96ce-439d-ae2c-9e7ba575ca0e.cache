# Citation Remediation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `trls source-link` and `trls accept-citation` commands, a `citation-accepted` op type, and remediate all 106 existing uncited issue nodes so `trls validate` reports 0 uncited nodes.

**Architecture:** Event-sourced: all state changes are new ops appended to `.issues/ops/*.log` files, replayed by the materialize engine into `materialize.Issue` structs. Validation reads the materialized state and checks citation coverage. The two new commands emit ops; the engine, traceability, and validation packages are updated to understand them.

**Tech Stack:** Go, Cobra (CLI), `golang.org/x/term` (TTY detection, already in dep graph), testify (assertions), `make check` (lint + test + coverage ≥ 80% + mutation)

**Spec:** `docs/superpowers/specs/2026-03-23-citation-remediation-design.md`

---

## File Structure

**Modified:**
- `internal/ops/types.go` — add `OpCitationAccepted` constant, `ConfirmedNoninteractively bool` field to `Payload`
- `internal/materialize/state.go` — add `CitationAcceptance` struct, `CitationAcceptances []CitationAcceptance` field to `Issue`
- `internal/materialize/engine.go` — add `case ops.OpCitationAccepted` to apply switch
- `internal/traceability/traceability.go` — add `CitationAcceptanceCount` to `IssueRef`; add `AcceptedRiskNodes`, `AcceptedRiskPct` to `Coverage`; update `Compute`
- `internal/materialize/pipeline.go` — update `toTraceabilityRefs` to populate `CitationAcceptanceCount`
- `internal/validate/validate.go` — update `checkE7E8E12Citations` to treat `CitationAcceptances` as satisfying citation
- `cmd/trellis/validate.go` — update coverage output line (human and JSON formats)
- `cmd/trellis/main.go` — register `newSourceLinkCmd()` and `newAcceptCitationCmd()`

**Created:**
- `cmd/trellis/source_link.go` — `newSourceLinkCmd()` implementation
- `cmd/trellis/accept_citation.go` — `newAcceptCitationCmd()` implementation

**Tests (existing files extended):**
- `internal/materialize/engine_test.go` — citation-accepted apply case (`package materialize`, uses `NewState()` and `state.ApplyOp()`)
- `internal/traceability/traceability_test.go` — Compute with CitationAcceptanceCount. This file uses `package traceability_test` (external); new tests follow the same style with `traceability.IssueRef{...}` prefix
- `internal/validate/validate_extra_test.go` — citation-accepted satisfies citation check
- `cmd/trellis/cmd_extra_test.go` — source-link and accept-citation command integration tests
- `cmd/trellis/main_test.go` — validate coverage output format

---

## Chunk 1: Foundation — op type, materialize, traceability, validation

### Task 1: Register `citation-accepted` op type

**Files:**
- Modify: `internal/ops/types.go`

- [ ] **Step 1: Write failing test**

There is no `internal/ops/types_test.go` — create it:

```go
// internal/ops/types_test.go
package ops

import "testing"

func TestOpCitationAccepted_RegisteredInValidOpTypes(t *testing.T) {
    if !ValidOpTypes[OpCitationAccepted] {
        t.Errorf("OpCitationAccepted not in ValidOpTypes")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```
cd /home/brian/development/trellis && go test ./internal/ops/ -run TestOpCitationAccepted -v
```

Expected: FAIL — `OpCitationAccepted` undefined.

- [ ] **Step 3: Add constant and ValidOpTypes entry**

In `internal/ops/types.go`, append after `OpAssign`:

```go
    OpCitationAccepted  = "citation-accepted"
```

Add to `ValidOpTypes`:
```go
    OpCitationAccepted: true,
```

Add `ConfirmedNoninteractively bool` to the `Payload` struct, in the `// dag-transition` section (keeping it grouped with the boolean fields):

```go
    // citation-accepted
    ConfirmedNoninteractively bool `json:"confirmed_noninteractively,omitempty"`
```

Note: `Payload.Rationale` (`json:"rationale,omitempty"`) already exists — reuse it, do not add a duplicate.

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/ops/ -run TestOpCitationAccepted -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ops/types.go internal/ops/types_test.go
git commit -m "feat(E5-S0-ext): add citation-accepted op type and payload field"
```

---

### Task 2: Add `CitationAcceptance` to materialize state + engine apply

**Files:**
- Modify: `internal/materialize/state.go`
- Modify: `internal/materialize/engine.go`
- Modify: `internal/materialize/engine_test.go`

- [ ] **Step 1: Write failing test**

In `internal/materialize/engine_test.go`, add (file is `package materialize`; use `NewState()` and `state.ApplyOp()`):

```go
func TestApplyCitationAccepted(t *testing.T) {
    state := NewState()
    createOp := ops.Op{
        Type: ops.OpCreate, TargetID: "TSK-1", Timestamp: 100,
        Payload: ops.Payload{Title: "Test", NodeType: "task"},
    }
    require.NoError(t, state.ApplyOp(createOp))

    op := ops.Op{
        Type: ops.OpCitationAccepted, TargetID: "TSK-1",
        Timestamp: 200, WorkerID: "worker-1",
        Payload: ops.Payload{
            Rationale:                 "no source doc exists",
            ConfirmedNoninteractively: false,
        },
    }
    require.NoError(t, state.ApplyOp(op))

    issue := state.Issues["TSK-1"]
    require.Len(t, issue.CitationAcceptances, 1)
    assert.Equal(t, "no source doc exists", issue.CitationAcceptances[0].Rationale)
    assert.Equal(t, "worker-1", issue.CitationAcceptances[0].WorkerID)
    assert.Equal(t, int64(200), issue.CitationAcceptances[0].Timestamp)
    assert.False(t, issue.CitationAcceptances[0].ConfirmedNoninteractively)
}

func TestApplyCitationAccepted_UnknownIssue_NoError(t *testing.T) {
    state := NewState()
    op := ops.Op{
        Type: ops.OpCitationAccepted, TargetID: "NONEXISTENT",
        Timestamp: 100,
        Payload:   ops.Payload{Rationale: "test"},
    }
    assert.NoError(t, state.ApplyOp(op))
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/materialize/ -run TestApplyCitationAccepted -v
```

Expected: FAIL — compile error (`CitationAcceptances` undefined) or test failure after struct is added but engine case is missing.

- [ ] **Step 3: Add struct and field to state.go**

In `internal/materialize/state.go`, add after the `SourceLink` struct:

```go
// CitationAcceptance records an explicit risk acceptance for a missing citation.
type CitationAcceptance struct {
    Rationale                 string `json:"rationale"`
    WorkerID                  string `json:"worker_id"`
    Timestamp                 int64  `json:"timestamp"`
    ConfirmedNoninteractively bool   `json:"confirmed_noninteractively,omitempty"`
}
```

Add field to `Issue` struct, after `SourceLinks`:

```go
CitationAcceptances []CitationAcceptance `json:"citation_acceptances,omitempty"`
```

- [ ] **Step 4: Add engine apply case**

In `internal/materialize/engine.go`, in the `Apply` switch (after `case ops.OpSourceLink`), add:

```go
case ops.OpCitationAccepted:
    return s.applyCitationAccepted(op)
```

Add method. `s.Issues` stores `*Issue` (pointer), so direct field mutation via `issue.Field = ...` is safe — no map reassignment needed. This matches the existing `applySourceLink` pattern.

```go
func (s *State) applyCitationAccepted(op ops.Op) error {
    issue, ok := s.Issues[op.TargetID]
    if !ok {
        return nil
    }
    issue.CitationAcceptances = append(issue.CitationAcceptances, CitationAcceptance{
        Rationale:                 op.Payload.Rationale,
        WorkerID:                  op.WorkerID,
        Timestamp:                 op.Timestamp,
        ConfirmedNoninteractively: op.Payload.ConfirmedNoninteractively,
    })
    issue.Updated = op.Timestamp
    return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```
go test ./internal/materialize/ -run TestApplyCitationAccepted -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/materialize/state.go internal/materialize/engine.go internal/materialize/engine_test.go
git commit -m "feat(E5-S0-ext): add CitationAcceptance to materialize state and engine"
```

---

### Task 3: Update traceability — split coverage count

**Files:**
- Modify: `internal/traceability/traceability.go`
- Modify: `internal/traceability/traceability_test.go` (create if absent)

- [ ] **Step 2: Write failing tests**

`internal/traceability/traceability_test.go` exists and uses `package traceability_test` (external test package). Append the new tests to it, keeping the same package declaration and import style:

```go
// Append to internal/traceability/traceability_test.go
// (package traceability_test — already declared at top of file)

func TestCompute_AllSourceLinked(t *testing.T) {
    refs := []traceability.IssueRef{
        {ID: "A", SourceLinkCount: 1},
        {ID: "B", SourceLinkCount: 2},
    }
    cov := traceability.Compute(refs)
    assert.Equal(t, 2, cov.TotalNodes)
    assert.Equal(t, 2, cov.CitedNodes)
    assert.Equal(t, 0, cov.AcceptedRiskNodes)
    assert.InDelta(t, 100.0, cov.CoveragePct, 0.01)
    assert.InDelta(t, 0.0, cov.AcceptedRiskPct, 0.01)
    assert.Empty(t, cov.Uncited)
}

func TestCompute_MixedCitation(t *testing.T) {
    refs := []traceability.IssueRef{
        {ID: "A", SourceLinkCount: 1},
        {ID: "B", CitationAcceptanceCount: 1},
        {ID: "C"},
    }
    cov := traceability.Compute(refs)
    assert.Equal(t, 3, cov.TotalNodes)
    assert.Equal(t, 2, cov.CitedNodes)
    assert.Equal(t, 1, cov.AcceptedRiskNodes)
    assert.InDelta(t, 66.67, cov.CoveragePct, 0.01)
    // AcceptedRiskPct = accepted_risk / total = 1/3 * 100
    assert.InDelta(t, 33.33, cov.AcceptedRiskPct, 0.01)
    assert.Equal(t, []string{"C"}, cov.Uncited)
}

func TestCompute_BothSourceLinkAndAcceptance_CountsAsSourceLinked(t *testing.T) {
    refs := []traceability.IssueRef{
        {ID: "A", SourceLinkCount: 1, CitationAcceptanceCount: 1},
    }
    cov := traceability.Compute(refs)
    assert.Equal(t, 1, cov.CitedNodes)
    assert.Equal(t, 0, cov.AcceptedRiskNodes) // has a source-link, not acceptance-only
}
```

Add `"github.com/stretchr/testify/assert"` to the import block if not already present.

- [ ] **Step 3: Run tests to verify they fail**

```
go test ./internal/traceability/ -run TestCompute -v
```

Expected: FAIL — `CitationAcceptanceCount` and `AcceptedRiskNodes` undefined.

- [ ] **Step 4: Update IssueRef and Coverage**

In `internal/traceability/traceability.go`:

```go
type IssueRef struct {
    ID                      string
    SourceLinkCount         int
    CitationAcceptanceCount int
}

type Coverage struct {
    TotalNodes       int      `json:"total_nodes"`
    CitedNodes       int      `json:"cited_nodes"`
    CoveragePct      float64  `json:"coverage_pct"`
    AcceptedRiskNodes int     `json:"accepted_risk_nodes"`
    AcceptedRiskPct  float64  `json:"accepted_risk_pct"`
    Uncited          []string `json:"uncited"`
}
```

Update `Compute`:

```go
func Compute(refs []IssueRef) Coverage {
    total := len(refs)
    cited := 0
    acceptedRisk := 0
    var uncited []string

    for _, ref := range refs {
        citedByLink := ref.SourceLinkCount > 0
        citedByAcceptance := ref.CitationAcceptanceCount > 0
        if citedByLink || citedByAcceptance {
            cited++
            if !citedByLink && citedByAcceptance {
                acceptedRisk++
            }
        } else {
            uncited = append(uncited, ref.ID)
        }
    }

    sort.Strings(uncited)

    var covPct, riskPct float64
    if total > 0 {
        covPct = float64(cited) / float64(total) * 100.0
        riskPct = float64(acceptedRisk) / float64(total) * 100.0
    }

    return Coverage{
        TotalNodes:        total,
        CitedNodes:        cited,
        CoveragePct:       covPct,
        AcceptedRiskNodes: acceptedRisk,
        AcceptedRiskPct:   riskPct,
        Uncited:           uncited,
    }
}
```

- [ ] **Step 5: Run tests to verify they pass**

```
go test ./internal/traceability/ -run TestCompute -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/traceability/traceability.go internal/traceability/traceability_test.go
git commit -m "feat(E5-S0-ext): extend traceability coverage to track accepted-risk citations"
```

---

### Task 4: Update pipeline to populate CitationAcceptanceCount

**Files:**
- Modify: `internal/materialize/pipeline.go`
- Modify: `internal/materialize/engine_test.go` or `pipeline_test.go` (add pipeline test)

- [ ] **Step 1: Write failing test**

`toTraceabilityRefs` is package-private — the test must be in `package materialize`. Add to `internal/materialize/engine_test.go` (already `package materialize`):

```go
func TestToTraceabilityRefs_PopulatesCitationAcceptanceCount(t *testing.T) {
    state := NewState()
    require.NoError(t, state.ApplyOp(ops.Op{
        Type: ops.OpCreate, TargetID: "TSK-1", Timestamp: 1,
        Payload: ops.Payload{Title: "T", NodeType: "task"},
    }))
    require.NoError(t, state.ApplyOp(ops.Op{
        Type: ops.OpCitationAccepted, TargetID: "TSK-1", Timestamp: 2,
        Payload: ops.Payload{Rationale: "no source"},
    }))

    refs := toTraceabilityRefs(state.Issues)
    require.Len(t, refs, 1)
    assert.Equal(t, 1, refs[0].CitationAcceptanceCount)
    assert.Equal(t, 0, refs[0].SourceLinkCount)
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/materialize/ -run TestToTraceabilityRefs -v
```

Expected: FAIL — `CitationAcceptanceCount` not populated.

- [ ] **Step 3: Update toTraceabilityRefs**

In `internal/materialize/pipeline.go`, update the function:

```go
func toTraceabilityRefs(issues map[string]*Issue) []traceability.IssueRef {
    refs := make([]traceability.IssueRef, 0, len(issues))
    for id, issue := range issues {
        refs = append(refs, traceability.IssueRef{
            ID:                      id,
            SourceLinkCount:         len(issue.SourceLinks),
            CitationAcceptanceCount: len(issue.CitationAcceptances),
        })
    }
    return refs
}
```

This function is called from two places in `pipeline.go` (lines ~123 and ~222 — both `Materialize` and `MaterializeAndReturn`). The function itself only needs to change once; both call sites will pick up the change automatically.

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/materialize/ -run TestToTraceabilityRefs -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/materialize/pipeline.go internal/materialize/engine_test.go
git commit -m "feat(E5-S0-ext): populate CitationAcceptanceCount in traceability refs"
```

---

### Task 5: Update validator to accept citation-accepted as satisfying citation

**Files:**
- Modify: `internal/validate/validate.go`
- Modify: `internal/validate/validate_extra_test.go`

- [ ] **Step 1: Write failing tests**

In `internal/validate/validate_extra_test.go`, add:

```go
func TestValidate_CitationAccepted_SatisfiesCitationRequirement(t *testing.T) {
    dir := t.TempDir()
    sourcesDir := filepath.Join(dir, "sources")
    require.NoError(t, os.MkdirAll(sourcesDir, 0755))

    // Write a manifest (citation-accepted doesn't need a manifest entry but manifest must exist)
    manifest := map[string]interface{}{"sources": []map[string]string{}}
    data, _ := json.Marshal(manifest)
    require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), data, 0644))

    state := makeState(
        &materialize.Issue{
            ID:   "TSK-1",
            Type: "task",
            CitationAcceptances: []materialize.CitationAcceptance{
                {Rationale: "no source exists", WorkerID: "w1", Timestamp: 100},
            },
        },
    )
    result := Validate(state, Options{IssuesDir: dir})
    for _, e := range result.Errors {
        if strings.Contains(e, "uncited node: TSK-1") {
            t.Errorf("TSK-1 with citation-accepted should not be uncited: %s", e)
        }
    }
}

func TestValidate_CitationAccepted_NoManifest_CitationCheckSkipped(t *testing.T) {
    // No manifest → citation check skipped entirely (existing behaviour preserved)
    dir := t.TempDir()
    state := makeState(
        &materialize.Issue{ID: "TSK-1", Type: "task"},
    )
    result := Validate(state, Options{IssuesDir: dir})
    for _, e := range result.Errors {
        if strings.Contains(e, "uncited node") {
            t.Errorf("citation check should be skipped when no manifest: %s", e)
        }
    }
}

func TestValidate_SourceLinkOnly_ManifestMembershipChecked(t *testing.T) {
    // source-link with unknown source-id still errors; citation-accepted does not check manifest
    dir := t.TempDir()
    sourcesDir := filepath.Join(dir, "sources")
    require.NoError(t, os.MkdirAll(sourcesDir, 0755))

    manifest := map[string]interface{}{"sources": []map[string]string{{"id": "known-src"}}}
    data, _ := json.Marshal(manifest)
    require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), data, 0644))

    state := makeState(
        &materialize.Issue{
            ID:   "TSK-1",
            Type: "task",
            SourceLinks: []materialize.SourceLink{
                {SourceEntryID: "unknown-src"},
            },
        },
    )
    result := Validate(state, Options{IssuesDir: dir})
    found := false
    for _, e := range result.Errors {
        if strings.Contains(e, "unknown source: unknown-src") {
            found = true
        }
    }
    assert.True(t, found, "expected unknown source error for source-link with bad source-id")
}
```

Ensure `strings` is imported in the test file.

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/validate/ -run "TestValidate_CitationAccepted|TestValidate_SourceLinkOnly" -v
```

Expected: FAIL — `materialize.CitationAcceptance` undefined or logic doesn't check it.

- [ ] **Step 3: Update checkE7E8E12Citations**

In `internal/validate/validate.go`, update the citation check function:

```go
func checkE7E8E12Citations(issues map[string]*materialize.Issue, issuesDir string) []string {
    var errs []string
    manifest, err := readManifestForValidate(issuesDir)
    if err != nil {
        if !errors.Is(err, os.ErrNotExist) {
            errs = append(errs, fmt.Sprintf("citation check skipped: cannot read source manifest: %v", err))
        }
        return errs
    }
    for id, issue := range issues {
        hasSrcLink := len(issue.SourceLinks) > 0
        hasAcceptance := len(issue.CitationAcceptances) > 0
        if !hasSrcLink && !hasAcceptance {
            errs = append(errs, fmt.Sprintf("uncited node: %s", id))
            continue
        }
        // Manifest-membership check applies only to source-link ops.
        for _, link := range issue.SourceLinks {
            if _, ok := manifest[link.SourceEntryID]; !ok {
                errs = append(errs, fmt.Sprintf("unknown source: %s in citation for %s", link.SourceEntryID, id))
            }
        }
    }
    return errs
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./internal/validate/ -run "TestValidate_CitationAccepted|TestValidate_SourceLinkOnly" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/validate/validate.go internal/validate/validate_extra_test.go
git commit -m "feat(E5-S0-ext): validator treats citation-accepted as satisfying citation requirement"
```

---

### Task 6: Update validate command coverage output (human + JSON)

**Files:**
- Modify: `cmd/trellis/validate.go`
- Modify: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write failing test**

In `cmd/trellis/main_test.go`, add only the format test (the accepted-risk test depends on `accept-citation` from Chunk 2 and is written there):

```go
func TestValidateCmd_CoverageOutput_HumanFormat(t *testing.T) {
    repo := setupRepoWithTask(t)

    // Add a manifest so citation check runs
    sourcesDir := filepath.Join(repo, ".issues", "sources")
    require.NoError(t, os.MkdirAll(sourcesDir, 0755))
    manifest := `{"sources":[]}`
    require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), []byte(manifest), 0644))

    out, err := runTrls(t, repo, "validate")
    require.NoError(t, err)
    // task-01 is uncited — coverage line should show 0/1
    assert.Contains(t, out, "COVERAGE: 0/1 cited")
}
```

Ensure `"os"` and `"path/filepath"` are imported in `main_test.go`.

- [ ] **Step 2: Run test to verify it fails (the format test)**

```
go test ./cmd/trellis/ -run TestValidateCmd_CoverageOutput_HumanFormat -v
```

Expected: FAIL — coverage line format does not match.

- [ ] **Step 3: Update validate.go — replace the entire format block**

Replace the existing `if format == "json" { ... } else { ... }` block (which currently excludes coverage from JSON) with this consolidated version:

```go
format, _ := cmd.Root().PersistentFlags().GetString("format")
if format == "json" {
    out, err := json.MarshalIndent(map[string]interface{}{
        "errors":   result.Errors,
        "warnings": result.Warnings,
        "coverage": result.Coverage,
    }, "", "  ")
    if err != nil {
        return err
    }
    _, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
} else {
    for _, e := range result.Errors {
        _, _ = fmt.Fprintf(cmd.OutOrStdout(), "ERROR: %s\n", e)
    }
    for _, w := range result.Warnings {
        _, _ = fmt.Fprintf(cmd.OutOrStdout(), "WARNING: %s\n", w)
    }
    if result.Coverage != nil {
        cov := result.Coverage
        sourceLinked := cov.CitedNodes - cov.AcceptedRiskNodes
        if cov.AcceptedRiskNodes > 0 {
            _, _ = fmt.Fprintf(cmd.OutOrStdout(),
                "COVERAGE: %d/%d cited (%d source-linked, %d accepted-risk)\n",
                cov.CitedNodes, cov.TotalNodes, sourceLinked, cov.AcceptedRiskNodes)
        } else {
            _, _ = fmt.Fprintf(cmd.OutOrStdout(),
                "COVERAGE: %d/%d cited\n",
                cov.CitedNodes, cov.TotalNodes)
        }
    }
    if result.OK {
        _, _ = fmt.Fprintln(cmd.OutOrStdout(), "OK: no issues found")
    }
}
```

- [ ] **Step 4: Run format test to verify it passes**

```
go test ./cmd/trellis/ -run TestValidateCmd_CoverageOutput_HumanFormat -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/validate.go cmd/trellis/main_test.go
git commit -m "feat(E5-S0-ext): update validate coverage output to show accepted-risk breakdown"
```

---

### Chunk 1 Gate: verify the foundation compiles

- [ ] **Build gate before starting Chunk 2**

```
go build ./...
```

Expected: no compile errors. Fix any before proceeding to Chunk 2 — the commands depend on types defined here.

---

## Chunk 2: Commands — source-link and accept-citation

### Task 7: Implement `trls source-link` command

**Files:**
- Create: `cmd/trellis/source_link.go`
- Modify: `cmd/trellis/main.go`
- Modify: `cmd/trellis/cmd_extra_test.go`

- [ ] **Step 1: Write failing tests**

In `cmd/trellis/cmd_extra_test.go`, add:

```go
func setupRepoWithTaskAndSource(t *testing.T) (repo string, sourceID string) {
    t.Helper()
    repo = setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    // Add a source
    out, err := runTrls(t, repo, "sources", "add",
        "--url", "docs/spec.md", "--type", "filesystem", "--title", "Spec")
    require.NoError(t, err)

    // Extract the UUID from "added source <UUID> (docs/spec.md)\n"
    parts := strings.Fields(out)
    require.GreaterOrEqual(t, len(parts), 3)
    sourceID = parts[2]
    return
}

func TestSourceLinkCmd_HappyPath(t *testing.T) {
    repo, sourceID := setupRepoWithTaskAndSource(t)

    out, err := runTrls(t, repo, "source-link", "--issue", "task-01", "--source-id", sourceID)
    require.NoError(t, err)
    assert.Contains(t, out, "Linked task-01")
    assert.Contains(t, out, sourceID)
}

func TestSourceLinkCmd_UnknownSourceID(t *testing.T) {
    repo, _ := setupRepoWithTaskAndSource(t)

    _, err := runTrls(t, repo, "source-link", "--issue", "task-01", "--source-id", "00000000-0000-0000-0000-000000000000")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "unknown source")
}

func TestSourceLinkCmd_MissingIssue(t *testing.T) {
    repo, sourceID := setupRepoWithTaskAndSource(t)

    _, err := runTrls(t, repo, "source-link", "--source-id", sourceID)
    require.Error(t, err)
}

func TestSourceLinkCmd_MissingSourceID(t *testing.T) {
    repo, _ := setupRepoWithTaskAndSource(t)

    _, err := runTrls(t, repo, "source-link", "--issue", "task-01")
    require.Error(t, err)
}

func TestSourceLinkCmd_MakesNodeCited(t *testing.T) {
    repo, sourceID := setupRepoWithTaskAndSource(t)

    _, err := runTrls(t, repo, "source-link", "--issue", "task-01", "--source-id", sourceID)
    require.NoError(t, err)

    // manifest already exists from sources add; validate should not show uncited error
    out, err := runTrls(t, repo, "validate")
    require.NoError(t, err)
    assert.NotContains(t, out, "uncited node: task-01")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/trellis/ -run TestSourceLinkCmd -v
```

Expected: FAIL — `source-link` command not found.

- [ ] **Step 3: Implement source_link.go**

Create `cmd/trellis/source_link.go`:

```go
package main

import (
    "fmt"

    "github.com/scullxbones/trellis/internal/ops"
    "github.com/scullxbones/trellis/internal/sources"
    "github.com/spf13/cobra"
)

func newSourceLinkCmd() *cobra.Command {
    var issueID, sourceID, section string

    cmd := &cobra.Command{
        Use:   "source-link",
        Short: "Link an issue to a registered source document",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Validate source-id against manifest
            manifest, err := sources.ReadManifest(sourcesDir())
            if err != nil {
                return fmt.Errorf("read manifest: %w", err)
            }
            entry, ok := manifest.Get(sourceID)
            if !ok {
                return fmt.Errorf("unknown source ID %s — run 'trls sources add' to register sources", sourceID)
            }

            workerID, logPath, err := resolveWorkerAndLog()
            if err != nil {
                return err
            }

            op := ops.Op{
                Type:      ops.OpSourceLink,
                TargetID:  issueID,
                Timestamp: nowEpoch(),
                WorkerID:  workerID,
                Payload: ops.Payload{
                    SourceID:  sourceID,
                    SourceURL: entry.URL,
                    Section:   section,
                },
            }
            if err := appendLowStakesOp(logPath, op); err != nil {
                return err
            }

            _, _ = fmt.Fprintf(cmd.OutOrStdout(), "Linked %s to source %s\n", issueID, sourceID)
            return nil
        },
    }

    cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to cite")
    cmd.Flags().StringVar(&sourceID, "source-id", "", "registered source UUID from manifest")
    cmd.Flags().StringVar(&section, "section", "", "optional section heading within the source")
    _ = cmd.MarkFlagRequired("issue")
    _ = cmd.MarkFlagRequired("source-id")
    return cmd
}
```

- [ ] **Step 4: Register command in main.go**

In `cmd/trellis/main.go`, in `newRootCmd()`, add:

```go
root.AddCommand(newSourceLinkCmd())
```

Place it after `root.AddCommand(newSourcesCmd())`.

- [ ] **Step 5: Run tests to verify they pass**

```
go test ./cmd/trellis/ -run TestSourceLinkCmd -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/source_link.go cmd/trellis/main.go cmd/trellis/cmd_extra_test.go
git commit -m "feat(E5-S0-ext): add trls source-link command"
```

---

### Task 8: Implement `trls accept-citation` command

**Files:**
- Create: `cmd/trellis/accept_citation.go`
- Modify: `cmd/trellis/main.go`
- Modify: `cmd/trellis/cmd_extra_test.go`

- [ ] **Step 1: Write failing tests**

In `cmd/trellis/cmd_extra_test.go`, add:

```go
func TestAcceptCitationCmd_CI_HappyPath(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    out, err := runTrls(t, repo, "accept-citation",
        "--issue", "task-01",
        "--rationale", "predates citation requirement exists",
        "--ci")
    require.NoError(t, err)
    assert.Contains(t, out, "Citation risk accepted for task-01")
}

func TestAcceptCitationCmd_RationaleTooShort(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    _, err = runTrls(t, repo, "accept-citation",
        "--issue", "task-01",
        "--rationale", "too short",
        "--ci")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "rationale must be at least 3 words")
}

func TestAcceptCitationCmd_TwoWords_Rejected(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    _, err = runTrls(t, repo, "accept-citation",
        "--issue", "task-01",
        "--rationale", "two words",
        "--ci")
    require.Error(t, err)
}

func TestAcceptCitationCmd_ThreeWords_Accepted(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    _, err = runTrls(t, repo, "accept-citation",
        "--issue", "task-01",
        "--rationale", "three exact words",
        "--ci")
    require.NoError(t, err)
}

func TestAcceptCitationCmd_MissingIssue(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "accept-citation", "--rationale", "some rationale here", "--ci")
    require.Error(t, err)
}

func TestAcceptCitationCmd_MissingRationale(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "accept-citation", "--issue", "task-01", "--ci")
    require.Error(t, err)
}

func TestAcceptCitationCmd_CI_SetsConfirmedNoninteractively(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    _, err = runTrls(t, repo, "accept-citation",
        "--issue", "task-01",
        "--rationale", "no source document exists",
        "--ci")
    require.NoError(t, err)

    // Materialize and check the CitationAcceptances field
    state, _, err2 := materialize.MaterializeAndReturn(
        filepath.Join(repo, ".issues"), false)
    require.NoError(t, err2)
    issue := state.Issues["task-01"]
    require.NotNil(t, issue)
    require.Len(t, issue.CitationAcceptances, 1)
    assert.True(t, issue.CitationAcceptances[0].ConfirmedNoninteractively)
    assert.Equal(t, "no source document exists", issue.CitationAcceptances[0].Rationale)
}
```

The last test uses `materialize.MaterializeAndReturn` — ensure `"github.com/scullxbones/trellis/internal/materialize"`, `"path/filepath"`, and `"strings"` are imported in `cmd_extra_test.go`. (`"strings"` is also needed by `setupRepoWithTaskAndSource` which calls `strings.Fields`.)

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./cmd/trellis/ -run TestAcceptCitationCmd -v
```

Expected: FAIL — `accept-citation` command not found.

- [ ] **Step 3: Implement accept_citation.go**

Create `cmd/trellis/accept_citation.go`:

```go
package main

import (
    "bufio"
    "fmt"
    "os"
    "strings"

    "github.com/scullxbones/trellis/internal/ops"
    "github.com/spf13/cobra"
    "golang.org/x/term"
)

func newAcceptCitationCmd() *cobra.Command {
    var issueID, rationale string
    var ci bool

    cmd := &cobra.Command{
        Use:   "accept-citation",
        Short: "Accept citation risk for an issue with recorded rationale",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Validate rationale word count
            if len(strings.Fields(rationale)) < 3 {
                return fmt.Errorf("rationale must be at least 3 words; got %q", rationale)
            }

            // Determine if interactive confirmation is needed
            nonInteractive := ci || !term.IsTerminal(int(os.Stdin.Fd()))

            if !nonInteractive {
                // Print prompt to stderr so it doesn't pollute stdout capture
                _, _ = fmt.Fprintf(os.Stderr,
                    "Accept citation risk for %s?\nThis decision will be recorded in the audit log under your git identity.\nType %q to confirm: ",
                    issueID, issueID)

                scanner := bufio.NewScanner(os.Stdin)
                scanner.Scan()
                input := strings.TrimSpace(scanner.Text())
                if input != issueID {
                    return fmt.Errorf("confirmation did not match; aborted")
                }
            }

            workerID, logPath, err := resolveWorkerAndLog()
            if err != nil {
                return err
            }

            op := ops.Op{
                Type:      ops.OpCitationAccepted,
                TargetID:  issueID,
                Timestamp: nowEpoch(),
                WorkerID:  workerID,
                Payload: ops.Payload{
                    Rationale:                 rationale,
                    ConfirmedNoninteractively: nonInteractive,
                },
            }
            if err := appendLowStakesOp(logPath, op); err != nil {
                return err
            }

            _, _ = fmt.Fprintf(cmd.OutOrStdout(), "Citation risk accepted for %s\n", issueID)
            return nil
        },
    }

    cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to accept citation risk for")
    cmd.Flags().StringVar(&rationale, "rationale", "", "reason no source citation is available (minimum 3 words)")
    cmd.Flags().BoolVar(&ci, "ci", false, "skip interactive confirmation (non-interactive mode)")
    _ = cmd.MarkFlagRequired("issue")
    _ = cmd.MarkFlagRequired("rationale")
    return cmd
}
```

- [ ] **Step 4: Register command in main.go**

In `cmd/trellis/main.go`, add after `newSourceLinkCmd()`:

```go
root.AddCommand(newAcceptCitationCmd())
```

- [ ] **Step 5: Run tests to verify they pass**

```
go test ./cmd/trellis/ -run TestAcceptCitationCmd -v
```

Expected: PASS.

- [ ] **Step 6: Add the accepted-risk coverage output test and run both**

In `cmd/trellis/main_test.go`, add alongside the test from Task 6:

```go
func TestValidateCmd_CoverageOutput_WithAcceptedRisk(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "worker-init")
    require.NoError(t, err)

    sourcesDir := filepath.Join(repo, ".issues", "sources")
    require.NoError(t, os.MkdirAll(sourcesDir, 0755))
    manifest := `{"sources":[]}`
    require.NoError(t, os.WriteFile(filepath.Join(sourcesDir, "manifest.json"), []byte(manifest), 0644))

    _, err = runTrls(t, repo, "accept-citation", "--issue", "task-01",
        "--rationale", "predates citation requirement exists", "--ci")
    require.NoError(t, err)

    out, err := runTrls(t, repo, "validate")
    require.NoError(t, err)
    assert.Contains(t, out, "1/1 cited")
    assert.Contains(t, out, "accepted-risk")
}
```

Then run both coverage output tests:

```
go test ./cmd/trellis/ -run TestValidateCmd_CoverageOutput -v
```

Expected: PASS.

- [ ] **Step 7: Run full make check**

```
make check
```

Expected: all four stages (lint, test, coverage ≥ 80%, mutation) GREEN. Fix any failures before continuing.

- [ ] **Step 8: Commit**

```bash
git add cmd/trellis/accept_citation.go cmd/trellis/main.go cmd/trellis/cmd_extra_test.go
git commit -m "feat(E5-S0-ext): add trls accept-citation command with interactive confirmation"
```

---

## Chunk 3: Retroactive Remediation

This chunk is data-only. All implementation code must be committed and `make check` must be green before starting here.

### Task 9: Register planning docs as sources

- [ ] **Step 1: Run worker-init**

```
./bin/trls worker-init
```

Expected: `Worker ID: <uuid>` (or "already initialized").

- [ ] **Step 2: Register all source docs**

Run `trls sources add` for each doc. Use the exact paths relative to the trellis repo root:

```bash
./bin/trls sources add --url "docs/trellis-prd.md" --type filesystem --title "Trellis PRD"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-15-e2-001-multi-branch-mode.md" --type filesystem --title "E2-001 Multi-Branch Mode"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-15-e2-002-branch-isolation.md" --type filesystem --title "E2-002 Branch Isolation"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-15-e2-003-merge-detection.md" --type filesystem --title "E2-003 Merge Detection"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-15-e2-004-pr-done-to-merged.md" --type filesystem --title "E2-004 PR Done to Merged"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-16-e3-collaboration.md" --type filesystem --title "E3 Collaboration"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-17-ci-validation-pipeline.md" --type filesystem --title "CI Validation Pipeline"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-17-test-coverage-hardening.md" --type filesystem --title "Test Coverage Hardening"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-index.md" --type filesystem --title "E4 Index"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-s1-tui-foundation.md" --type filesystem --title "E4-S1 TUI Foundation"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-s2-glamour-render.md" --type filesystem --title "E4-S2 Glamour Render"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-s3-full-validate.md" --type filesystem --title "E4-S3 Full Validate"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-s4-sources.md" --type filesystem --title "E4-S4 Sources"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-s5-decompose-context.md" --type filesystem --title "E4-S5 Decompose Context"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-s6-dag-summary.md" --type filesystem --title "E4-S6 DAG Summary"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-s7-traceability.md" --type filesystem --title "E4-S7 Traceability"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-s8-brownfield-import.md" --type filesystem --title "E4-S8 Brownfield Import"
./bin/trls sources add --url "docs/superpowers/plans/2026-03-18-e4-s9-stale-review.md" --type filesystem --title "E4-S9 Stale Review"
```

Each prints `added source <UUID> (<path>)`. Capture the UUIDs — you need them in Task 10.

The E5 plan is already registered. Get its UUID:

```bash
cat .issues/sources/manifest.json | python3 -c "import sys,json; m=json.load(sys.stdin); [print(k,v['title']) for k,v in m['entries'].items()]"
```

- [ ] **Step 3: Verify all sources registered**

```bash
cat .issues/sources/manifest.json | python3 -c "import sys,json; m=json.load(sys.stdin); print(len(m['entries']), 'sources registered')"
```

Expected: 20 sources (19 new + 1 already registered E5 plan).

- [ ] **Step 4: Commit sources manifest**

```bash
git add .issues/sources/
git commit -m "chore(E5-S0-ext): register planning docs as citation sources"
```

---

### Task 10: Emit source-link ops for all properly-sourceable issues

The following table maps issue IDs to source doc titles. Use the UUIDs captured in Task 9.

**Mapping:**

| Issues | Source doc title |
|---|---|
| E2, E2-001, E2-001-T1..T6 | "E2-001 Multi-Branch Mode" |
| E2-002, E2-002-T* | "E2-002 Branch Isolation" |
| E2-003, E2-003-T* | "E2-003 Merge Detection" |
| E2-004, E2-004-T* | "E2-004 PR Done to Merged" |
| E3, E3-001, E3-001-T* | "E3 Collaboration" |
| E3-002, E3-002-T* | "E3 Collaboration" |
| E3-003, E3-003-T* | "E3 Collaboration" |
| E3-004, E3-004-T* | "E3 Collaboration" |
| E3-T13 | "E3 Collaboration" |
| CI-001, CI-002, CI-003, CI-004 | "CI Validation Pipeline" |
| TC-001, TC-002..TC-010 | "Test Coverage Hardening" |
| E4, E4-S1, E4-S1-T1..T5 | "E4-S1 TUI Foundation" (tasks); "E4 Index" (E4, story) |
| E4-S2, E4-S2-T1 | "E4-S2 Glamour Render" |
| E4-S3, E4-S3-T1..T2 | "E4-S3 Full Validate" |
| E4-S4, E4-S4-T1..T6 | "E4-S4 Sources" |
| E4-S5, E4-S5-T1 | "E4-S5 Decompose Context" |
| E4-S6, E4-S6-T1..T2 | "E4-S6 DAG Summary" |
| E4-S7, E4-S7-T1..T2 | "E4-S7 Traceability" |
| E4-S8, E4-S8-T1 | "E4-S8 Brownfield Import" |
| E4-S9, E4-S9-T1 | "E4-S9 Stale Review" |
| E5, E5-S0, E5-S0-T1..T4 | "E5 Plan — Polish, UX Hardening & User-Facing Docs" (already registered) |
| E5-S1, E5-S1-T1..T8 | same E5 plan |
| E5-S2, E5-S2-T1..T3 | same E5 plan |
| E5-S3, E5-S3-T1..T5 | same E5 plan |
| E5-S4, E5-S4-T1..T4 | same E5 plan |

- [ ] **Step 1: Emit source-link ops**

For each issue in the mapping, run:

```bash
./bin/trls source-link --issue ISSUE-ID --source-id SOURCE-UUID
```

Work in order: E2 → E3 → CI/TC → E4 → E5. This will produce a large number of individual op emissions. They can be batched in a shell loop if preferred, but do not script the UUIDs — retrieve them from the manifest.

- [ ] **Step 2: Check intermediate progress**

After each epic group, run:

```bash
./bin/trls validate 2>&1 | grep -c "uncited node"
```

Expected count decreases as each group is linked.

- [ ] **Step 3: Commit ops after all source-link ops are emitted**

```bash
git add .issues/
git commit -m "chore(E5-S0-ext): emit source-link ops for E2/E3/CI/TC/E4/E5 issues"
```

---

### Task 11: Accept-citation the stragglers

The bare-timestamp IDs have no recoverable source material.

- [ ] **Step 1: Identify remaining uncited nodes**

```bash
./bin/trls validate 2>&1 | grep "uncited node"
```

Expected: only `task-1773804403`, `task-1773804476`, `story-1773804400` (and possibly `TC-001` if it predates coverage).

- [ ] **Step 2: Accept citation risk for each straggler**

```bash
./bin/trls accept-citation \
    --issue task-1773804403 \
    --rationale "Created before planning doc convention existed; no recoverable source material."
```

```bash
./bin/trls accept-citation \
    --issue task-1773804476 \
    --rationale "Created before planning doc convention existed; no recoverable source material."
```

```bash
./bin/trls accept-citation \
    --issue story-1773804400 \
    --rationale "Created before planning doc convention existed; no recoverable source material."
```

For any additional stragglers found in step 1, apply the same rationale if no source doc exists.

The interactive prompt requires typing the issue ID exactly for each. Use `--ci` only if running in a non-interactive context.

- [ ] **Step 3: Verify zero uncited nodes**

```bash
./bin/trls validate 2>&1
```

Expected: no lines beginning with `ERROR:`. Coverage line should show `106/106 cited (N source-linked, M accepted-risk)`. If any `ERROR: uncited node:` lines remain, resolve them before committing — either source-link the issue if a doc applies, or accept-citation with appropriate rationale.

- [ ] **Step 4: Final commit**

```bash
git add .issues/
git commit -m "chore(E5-S0-ext): accept-citation for pre-convention issues; 106/106 cited"
```

---

## Final Verification

- [ ] **Run make check**

```
make check
```

Expected: lint → test → coverage ≥ 80% → mutation — all GREEN.

- [ ] **Run validate**

```
./bin/trls validate
```

Expected: `COVERAGE: 106/106 cited (N source-linked, M accepted-risk)` with no ERROR lines.
