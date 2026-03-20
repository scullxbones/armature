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

**File**: `docs/superpowers/plans/`

---

## L5: Skills cannot deliver cross-platform binaries; `SKILL_ROOT` is not exposed to agents

**Observed**: The `trls` AgentSkill bundles the compiled binary at `scripts/trls` (relative to skill root). Agents cannot resolve this path because: (1) Claude Code does not expose `SKILL_ROOT` to agent context at runtime, and (2) the bundled binary is platform-specific — a skill targeting linux/amd64 breaks on darwin/arm64 and vice versa. Fixing path to `bin/trls` (repo-relative) only works inside the trellis repo itself.

**Impact**: Bundled binary delivery via skills is not viable for compiled Go binaries without per-platform skill variants. Agents guess the binary path relative to CWD and fail.

**Recommendation**: Install `trls` to PATH via `make install` (deploys to `~/.local/bin/trls`). Skills should reference bare `trls` and fail clearly if not found. The bundled `scripts/trls` in the skill directory is a dead end for Go binaries. If the Claude Code skills runtime were to inject `SKILL_ROOT` into agent context, bundled scripts would become viable — this is a platform feature request.

**File**: `.claude/skills/trls/SKILL.md`, `Makefile` that diffs the plan's expected starting state against reality before executing any steps. Alternatively, cross-plan notes (like the one in E2-002) should be more prominent, listing which steps are expected to already be complete.

---

## L6: Stale claim blocks `trls ready` with no diagnostic

**Observed**: `trls ready` returned "No tasks ready" even though open tasks existed (E4-S1-T2 through T5). The actual cause was E4-S1-T1 held an expired claim from a previous worker session (`0fb9a1c9-...`, stale since 2026-03-19T03:14:12Z). The dependent tasks couldn't surface because their blocker appeared to still be in-progress.

**Impact**: Agent got a false "nothing to do" signal and would have stopped. Required user intervention to diagnose. The `trls workers` output showed the holding worker as `stale`, but there was no connection drawn between the stale worker and the empty ready queue.

**Recommendation**: When `trls ready` returns empty and in-progress issues exist with stale claims, print a diagnostic: e.g. "1 task in-progress with stale claim (E4-S1-T1, claimed by 0fb9a1c9, expired 2h ago) — use `trls claim --issue E4-S1-T1` to steal." Alternatively, auto-expire stale claims when computing the ready queue.

**File**: `internal/ready/compute.go`, `cmd/trellis/ready.go`

---

## L7: `trls-worker` skill not discoverable from project root

**Observed**: The skill is deployed at `trellis/.claude/skills/trls-worker/` but the agent's working directory is `/home/brian/development` (the parent). The Skill tool only surfaces skills from the active project directory, so `trls-worker` was initially not found. The agent fell back to reading the skill file directly from disk before the user pointed out the correct deployment path.

**Impact**: Wasted turn discovering the skill location. Agent attempted to use the wrong skill file (`docs/trls-worker-SKILL.md`) before the correct one was loaded.

**Recommendation**: Skills should be deployed at the repo root (`.claude/skills/`) rather than a subdirectory if the agent's working directory may be the parent. Alternatively, document in the skill meta that the user must `cd` into the trellis repo or open it as the active workspace before invoking.

**File**: `trellis/.claude/skills/trls-worker/`

---

## L8: `worker-init` emits noisy `_encode`/`_decode` warnings

**Observed**: Running `trls worker-init` produced 12 lines of `setValueForKeyFakeAssocArray:27: command not found: _encode` / `valueForKeyFakeAssocArray:28: command not found: _decode` before the actual output. These are non-fatal shell warnings from the zsh completion or git config helper internals.

**Impact**: Output noise erodes agent confidence that the command succeeded. Agents may flag warnings as errors and stop.

**Recommendation**: Suppress or fix the underlying shell compat issue. At minimum, document in the skill that these warnings are harmless so agents don't halt on them.

**File**: `cmd/trellis/worker_init.go` or underlying git config shell helper

---

## L9: Claiming a story blocks its child tasks from `trls ready`

**Observed**: `trls ready` surfaced E4-S4 (a story, not a task) as ready. Running `trls claim --issue E4-S4` set the story to `claimed` status, which blocked E4-S4-T1 through T6 from appearing in `trls ready` — the parent was now claimed by me and the children showed their parent as `[claimed]`.

**Impact**: Claiming the wrong granularity stalls all downstream work. The worker loop skill says to claim tasks, but `trls ready` mixed stories into the queue without signaling they behave differently.

**Recommendation**: Either (a) exclude stories from `trls ready` output (tasks/features only), or (b) print a warning when claiming a story: "Claiming a story will block its child tasks — claim individual tasks instead." Or (c) make claiming a story auto-claim all its ready children instead.

**File**: `internal/ready/compute.go`, `cmd/trellis/claim.go`

---

## L10: `trls unassign` does not fully release a claim — explicit transition required

**Observed**: After claiming E4-S4 by mistake, `trls unassign --issue E4-S4` cleared the `assigned_to` field but left the issue in `claimed` status. Child tasks still showed parent as `[claimed]` and `trls ready` remained empty. A follow-up `trls transition --issue E4-S4 --to in-progress` was required to unblock the queue.

**Impact**: Two-step recovery is non-obvious. `unassign` appears to undo a claim but silently leaves a residual state.

**Recommendation**: `trls unassign` should transition the issue back to `in-progress` (or its prior status) automatically, or at least print: "Issue remains in 'claimed' state — run `trls transition --issue ID --to in-progress` to restore."

**File**: `cmd/trellis/unassign.go`

---

## L11: `trls transition --to open` is rejected — `open` is not a valid transition target

**Observed**: Attempted `trls transition --issue E4-S4 --to open` to undo an accidental claim. Got error: `invalid status "open": valid values are [blocked cancelled done in-progress merged]`. There is no way to return an issue to `open` once it has been moved to `in-progress` or `claimed`.

**Impact**: Agents have no recovery path for accidentally starting an issue. The only options are `in-progress`, `done`, `blocked`, `cancelled`, or `merged` — none of which mean "I didn't mean to touch this."

**Recommendation**: Add `open` as a valid transition target (or alias `reopen` to cover this case for non-done issues too), so agents can undo accidental claims cleanly.

**File**: `cmd/trellis/transition.go`, `internal/ops/types.go`
