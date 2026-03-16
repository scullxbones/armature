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

---

## L3: AgentSkill trls binary path is wrong

**Observed**: The trls AgentSkill references `scripts/trls` as the binary path, but the actual binary is at `bin/trls`. Running `scripts/trls ready` produces "no such file or directory".

**Impact**: Agents following the skill literally fail immediately. Requires manual discovery of the correct path.

**Recommendation**: Update the AgentSkill to reference `bin/trls`, or add a `scripts/trls` symlink/wrapper, or make `make install` install to a predictable location on PATH.

**File**: `.claude/skills/trls` (binary path reference)

---

## L4: Plan/reality drift undetected — executing a plan that was already partially done

**Observed**: E2-002's implementation plan was written before E2-001 was completed, but E2-001 ended up implementing most of E2-002 (AppendAndCommit, WorktreePath, appendOp wrapper, integration tests). When executing E2-002's plan, virtually all steps were already done. Only one remaining gap existed (`claim.go` scope-overlap path still called `ops.AppendOp` directly).

**Impact**: Agents blindly re-executing steps waste time and may inadvertently overwrite correct implementations. The gap that actually existed was easy to miss precisely because it was buried in an otherwise-complete file.

**Recommendation**: Plans should begin with a "current state check" step that diffs the plan's expected starting state against reality before executing any steps. Alternatively, cross-plan notes (like the one in E2-002) should be more prominent, listing which steps are expected to already be complete.
