# Trellis Dogfooding Learnings

Captured while using trellis to track its own E2 development.

## L1: `trls ready` only shows task/feature types

**Observed**: Created stories (E2-001 through E2-004) but `trls ready` returned empty because `ComputeReady` filters to `type == "task" || type == "feature"` only.

**Impact**: Solo devs or small teams using stories as their primary work unit get no ready queue output.

**Recommendation**: Either:
- Add `story` to the ready filter
- Document that stories must be decomposed into tasks to appear in ready queue
- Add a `--type` filter flag to `trls ready` so users can opt in

**File**: `internal/ready/compute.go:34`

---

## L2: `trls transition` accepts invalid status values silently

**Observed**: `trls transition --issue E2-001 --to in_progress` succeeded, but the canonical status is `in-progress` (hyphen). This caused the ready gate to fail because `ops.StatusInProgress == "in-progress"` didn't match the stored `"in_progress"`.

**Impact**: Silently corrupted state that was hard to debug. Ready queue appeared empty with no explanation.

**Recommendation**: Validate `--to` against the known status set (`open`, `in-progress`, `done`, `merged`, `blocked`) and reject unknown values with an error message listing valid options.

**File**: `cmd/trellis/transition.go`, `internal/ops/types.go:29-33`
