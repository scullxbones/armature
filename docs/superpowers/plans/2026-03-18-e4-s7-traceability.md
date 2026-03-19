# E4 Traceability Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Materialize source-link ops into per-issue SourceLink records, handle dag-transition ops, compute traceability coverage metrics, and write traceability.json.

**Spec:** `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` (E3-S7 section)

**Depends on:** `2026-03-18-e4-s4-sources.md`

**Execution order within E4:** S1 → S4 → S7 → S2 → S3 → S5 → S6 → S8 → S9

**Tech Stack:** Go 1.26, testify

---

## File Structure

| Package | File | Responsibility |
|---|---|---|
| `internal/materialize` | `state.go` | Add `SourceLink` type, `SourceLinks` field on Issue, `DAGConfirmed` on Provenance |
| `internal/materialize` | `engine.go` | Handle `source-link` and `dag-transition` ops |
| `internal/materialize` | `engine_test.go` | Tests for new op handlers |
| `internal/traceability` | `traceability.go` | `Coverage` type; compute from `State.SourceLinks` |
| `internal/traceability` | `traceability_test.go` | Coverage computation tests |
| `internal/materialize` | `pipeline.go` | Write `traceability.json` after state files |

---

## Tasks

### Task 1: source-link op materialization + SourceLink type

**Files:**
- Modify: `internal/materialize/state.go`
- Modify: `internal/materialize/engine.go`
- Modify: `internal/materialize/engine_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/materialize/engine_test.go (append)
func TestApplySourceLinkOp(t *testing.T) {
	state := NewState()
	// First create the issue
	createOp := ops.Op{
		Type: ops.OpCreate, TargetID: "TSK-1", Timestamp: 1000, WorkerID: "w1",
		Payload: ops.Payload{Title: "My Task", NodeType: "task"},
	}
	require.NoError(t, state.ApplyOp(createOp))

	// Apply source-link
	linkOp := ops.Op{
		Type: ops.OpSourceLink, TargetID: "TSK-1", Timestamp: 1001, WorkerID: "w1",
		Payload: ops.Payload{SourceID: "prd", Section: "Auth", Anchor: "#auth", Quote: "Users must authenticate"},
	}
	require.NoError(t, state.ApplyOp(linkOp))

	issue := state.Issues["TSK-1"]
	require.Len(t, issue.SourceLinks, 1)
	assert.Equal(t, "prd", issue.SourceLinks[0].SourceID)
	assert.Equal(t, "Auth", issue.SourceLinks[0].Section)
}

func TestApplyDAGTransitionOp(t *testing.T) {
	state := NewState()
	createOp := ops.Op{
		Type: ops.OpCreate, TargetID: "TSK-2", Timestamp: 1000, WorkerID: "w1",
		Payload: ops.Payload{Title: "Inferred Task", NodeType: "task"},
	}
	require.NoError(t, state.ApplyOp(createOp))
	// Initially confidence is "decomposed" / default
	require.False(t, state.Issues["TSK-2"].Provenance.DAGConfirmed)

	dagOp := ops.Op{
		Type: ops.OpDAGTransition, TargetID: "TSK-2", Timestamp: 1002, WorkerID: "reviewer1",
	}
	require.NoError(t, state.ApplyOp(dagOp))
	assert.True(t, state.Issues["TSK-2"].Provenance.DAGConfirmed)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/materialize/... -run "TestApplySourceLink|TestApplyDAGTransition" -v
```

Expected: FAIL — `SourceLinks` field not found.

- [ ] **Step 3: Add SourceLink type and fields to state.go**

In `internal/materialize/state.go`, add the `SourceLink` type and fields:

```go
// SourceLink records a citation from an issue to a source document section.
type SourceLink struct {
	SourceID  string `json:"source_id"`
	Section   string `json:"section,omitempty"`
	Anchor    string `json:"anchor,omitempty"`
	Quote     string `json:"quote,omitempty"`
	WorkerID  string `json:"worker_id"`
	Timestamp int64  `json:"timestamp"`
}
```

In the `Issue` struct, add:
```go
SourceLinks []SourceLink `json:"source_links,omitempty"`
```

In the `Provenance` struct, add:
```go
DAGConfirmed bool `json:"dag_confirmed,omitempty"`
```

- [ ] **Step 4: Handle source-link and dag-transition ops in engine.go**

Replace the current no-op case in `ApplyOp`:
```go
case ops.OpSourceLink, ops.OpSourceFingerprint, ops.OpDAGTransition:
    return nil
```

With:
```go
case ops.OpSourceLink:
    return s.applySourceLink(op)
case ops.OpSourceFingerprint:
    return nil // fingerprint tracked in traceability.json, not per-issue state
case ops.OpDAGTransition:
    return s.applyDAGTransition(op)
```

Add the handler methods:
```go
func (s *State) applySourceLink(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil // tolerate source-link before create (log order)
	}
	issue.SourceLinks = append(issue.SourceLinks, SourceLink{
		SourceID:  op.Payload.SourceID,
		Section:   op.Payload.Section,
		Anchor:    op.Payload.Anchor,
		Quote:     op.Payload.Quote,
		WorkerID:  op.WorkerID,
		Timestamp: op.Timestamp,
	})
	issue.Updated = op.Timestamp
	return nil
}

func (s *State) applyDAGTransition(op ops.Op) error {
	issue, ok := s.Issues[op.TargetID]
	if !ok {
		return nil
	}
	issue.Provenance.DAGConfirmed = true
	issue.Updated = op.Timestamp
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/materialize/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/materialize/state.go internal/materialize/engine.go internal/materialize/engine_test.go
git commit -m "feat(materialize): handle source-link and dag-transition ops"
```

---

### Task 2: Traceability coverage computation + traceability.json

**Files:**
- Create: `internal/traceability/traceability.go`
- Create: `internal/traceability/traceability_test.go`
- Modify: `internal/materialize/pipeline.go`

- [ ] **Step 1: Write failing test**

```go
// internal/traceability/traceability_test.go
package traceability_test

import (
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/traceability"
	"github.com/stretchr/testify/assert"
)

func TestCoverageFullyCited(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TSK-1"] = &materialize.Issue{
		ID: "TSK-1", Type: "task",
		SourceLinks: []materialize.SourceLink{{SourceID: "prd"}},
	}
	state.Issues["TSK-2"] = &materialize.Issue{
		ID: "TSK-2", Type: "task",
		SourceLinks: []materialize.SourceLink{{SourceID: "arch"}},
	}

	cov := traceability.Compute(state)
	assert.Equal(t, 2, cov.TotalNodes)
	assert.Equal(t, 2, cov.CitedNodes)
	assert.InDelta(t, 100.0, cov.CoveragePct, 0.01)
	assert.Empty(t, cov.Uncited)
}

func TestCoveragePartial(t *testing.T) {
	state := materialize.NewState()
	state.Issues["TSK-1"] = &materialize.Issue{ID: "TSK-1", Type: "task",
		SourceLinks: []materialize.SourceLink{{SourceID: "prd"}},
	}
	state.Issues["TSK-2"] = &materialize.Issue{ID: "TSK-2", Type: "task"}

	cov := traceability.Compute(state)
	assert.Equal(t, 2, cov.TotalNodes)
	assert.Equal(t, 1, cov.CitedNodes)
	assert.InDelta(t, 50.0, cov.CoveragePct, 0.01)
	assert.Contains(t, cov.Uncited, "TSK-2")
}

func TestCoverageEmpty(t *testing.T) {
	state := materialize.NewState()
	cov := traceability.Compute(state)
	assert.Equal(t, 0, cov.TotalNodes)
	assert.InDelta(t, 0.0, cov.CoveragePct, 0.01)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/traceability/... -v
```

Expected: FAIL — package not found.

- [ ] **Step 3: Implement traceability**

```go
// internal/traceability/traceability.go
package traceability

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/scullxbones/trellis/internal/materialize"
)

// Coverage holds traceability coverage metrics for the DAG.
type Coverage struct {
	TotalNodes  int                        `json:"total_nodes"`
	CitedNodes  int                        `json:"cited_nodes"`
	CoveragePct float64                    `json:"coverage_pct"`
	Citations   map[string][]materialize.SourceLink `json:"citations"` // nodeID → links
	Uncited     []string                   `json:"uncited"`
}

// Compute calculates traceability coverage from the materialized state.
func Compute(state *materialize.State) Coverage {
	cov := Coverage{
		Citations: make(map[string][]materialize.SourceLink),
	}
	for id, issue := range state.Issues {
		cov.TotalNodes++
		if len(issue.SourceLinks) > 0 {
			cov.CitedNodes++
			cov.Citations[id] = issue.SourceLinks
		} else {
			cov.Uncited = append(cov.Uncited, id)
		}
	}
	if cov.TotalNodes > 0 {
		cov.CoveragePct = float64(cov.CitedNodes) / float64(cov.TotalNodes) * 100.0
	}
	return cov
}

// Write serializes coverage to .issues/state/traceability.json.
func Write(issuesDir string, cov Coverage) error {
	dir := filepath.Join(issuesDir, "state")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cov, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "traceability.json"), data, 0644)
}

// Read loads traceability.json; returns empty Coverage if missing.
func Read(issuesDir string) (Coverage, error) {
	data, err := os.ReadFile(filepath.Join(issuesDir, "state", "traceability.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return Coverage{Citations: make(map[string][]materialize.SourceLink)}, nil
		}
		return Coverage{}, err
	}
	var cov Coverage
	if cov.Citations == nil {
		cov.Citations = make(map[string][]materialize.SourceLink)
	}
	return cov, json.Unmarshal(data, &cov)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/traceability/... -v
```

Expected: PASS.

- [ ] **Step 5: Wire traceability.json into materialization pipeline**

In `internal/materialize/pipeline.go`, after the existing state file writes, add a call to write traceability:

First read `internal/materialize/pipeline.go` to find the correct insertion point before editing.

```go
import "github.com/scullxbones/trellis/internal/traceability"

// After writing state files:
cov := traceability.Compute(state)
if err := traceability.Write(issuesDir, cov); err != nil {
    return fmt.Errorf("write traceability: %w", err)
}
```

- [ ] **Step 6: Run all materialize tests**

```bash
go test ./internal/materialize/... -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/traceability/ internal/materialize/pipeline.go
git commit -m "feat(traceability): coverage computation and traceability.json state file"
```
