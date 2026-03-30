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

---

## L12: `decompose-apply` plan JSON schema is undiscoverable — agents fall back to reading Go source

**Observed**: When loading a plan into trellis via `trls decompose-apply --plan <file>`, the agent had no way to discover the required JSON schema without reading the Go source (`internal/decompose/plan.go`). `trls decompose-apply --help` describes only the `--plan` flag, not the file format. There is no example file, schema reference, or `--schema` flag.

**Impact**: Agents must either guess the format (and fail silently or with a cryptic parse error) or read the implementation source. Both paths are friction. This was observed directly when a session fell back to `find`/`cat` on the Go source instead of using `trls` queries.

**Recommendation**: One or more of: (a) add `trls decompose-apply --example` that prints a minimal valid JSON plan; (b) add a `--schema` flag that prints the JSON schema; (c) document the format in `docs/commands.md` under `trls decompose-apply`; (d) improve the `--help` text to include the schema inline.

**File**: `cmd/trellis/decompose.go`, `internal/decompose/plan.go`, `docs/commands.md`

---

## L13: No `trls list` command — agents fall back to filesystem reads to discover existing issue IDs

**Observed**: Before creating a plan JSON, the agent needed to know which issue IDs already exist (to avoid conflicts) and to understand the naming convention (E4 → E5, E5-S1-T1, etc.). `trls status` lists all issues grouped by status but is not filterable by type, parent, or scope. There is no `trls list` command that supports `--type`, `--parent`, or `--format json` with structured output for programmatic use. The agent fell back to `ls .issues/state/issues/` to read issue IDs from the filesystem.

**Impact**: Agents cannot reliably query the issue graph using `trls` tooling alone. Filesystem reads bypass the op-log abstraction, are fragile, and are exactly the kind of behavior the CLI is meant to prevent.

**Recommendation**: Add a `trls list` command (or extend `trls status`) with: `--type [epic|story|task|feature|bug]` filter, `--parent ID` filter, `--format json` output with full issue fields (id, title, type, status, parent, blocked_by), and optionally `--ids-only` for quick ID enumeration. Agents should be able to answer "what IDs exist under E5?" without touching the filesystem.

**File**: `cmd/trellis/status.go` (or new `cmd/trellis/list.go`), `internal/materialize/`

---

## L14: `trls sources` traceability bypassed — decompose pipeline undiscoverable as a workflow

**Observed**: When loading the E5 plan from `docs/superpowers/plans/2026-03-19-e5-ux-tui-forensics-docs.md`, the agent skipped `trls sources` entirely and hand-crafted the plan JSON directly from a manual reading of the markdown file. The intended pipeline — `sources add` → `sources sync` → `decompose-context --sources <id> --existing-dag` → AI generates JSON → `decompose-apply` — was never followed. The 21 E5 issues are now loaded with no traceability link back to the source document. The `trls sources` feature provides this link, but it was completely invisible.

**Impact**: Source traceability — the primary value of `trls sources` — was entirely bypassed. The plan document and the loaded issues have no recorded connection. If the plan is updated, `stale-review` cannot surface the drift. Additionally, the manual JSON authoring is slower and more error-prone than the AI-assisted decompose-context path.

**Recommendation**: Make the decompose pipeline a first-class documented workflow. The trls skill (SKILL.md) should include a "Loading a plan" section that walks through: (1) `sources add`, (2) `sources sync`, (3) `decompose-context`, (4) human/AI reviews and edits the JSON, (5) `decompose-apply`. Without this narrative, agents treat the commands as unrelated and skip the connective tissue. Consider also whether `decompose-apply` should require or at least warn when no source is linked to the resulting issues.

**File**: `.claude/skills/trls/SKILL.md`, `cmd/trellis/decompose.go`

---

## L15: `decompose-apply` has no `--dry-run` — agents apply blindly

**Observed**: When running `trls decompose-apply --plan e5-plan.json`, there was no way to preview which issues would be created, what IDs they would get, or whether any conflicts existed before committing. The command applied 21 issues in a single shot with no confirmation step. If the JSON had contained a bad parent ID, duplicate ID, or wrong type, the error would only surface after ops were written.

**Impact**: A malformed plan is hard to recover from — `decompose-revert` exists but requires the same plan file to be intact. An agent cannot audit the plan's effect before applying it, increasing the risk of polluting the op log with bad data.

**Recommendation**: Add `trls decompose-apply --dry-run` that validates the plan against the current graph state and prints what would be created (IDs, types, parents, blocked_by edges) without writing any ops. Also consider `--validate-only` as an alias. This gives agents and humans a chance to catch mistakes before they're committed to the op log.

**File**: `cmd/trellis/decompose.go`, `internal/decompose/apply.go`

---

## L16: Fresh worker session sees empty ready queue — parent epic/story must be manually transitioned to `in-progress`

**Observed**: Starting a new worker session on a repo where the epic and story are both `open`, `trls ready` returned "No tasks ready" even though the story had ready subtasks with no blockers. Root cause: `ComputeReady` gates tasks on `parent.Status == in-progress`. The parent epic (E5) and story (E5-S0) had never been transitioned, so no tasks surfaced. Required two manual `trls transition --to in-progress` calls (one for the epic, one for the story) before any tasks appeared.

**Impact**: A fresh agent session hits an empty queue with no explanation and would stop, even though work is ready. The agent must know to walk up the parent chain and manually start each ancestor — this is non-obvious and not documented in the worker skill.

**Recommendation**: Fix has two parts: (1) change `ComputeReady` to surface tasks whose parent is `open` (not just `in-progress`) — this removes the bootstrap deadlock; (2) when a task is claimed, auto-transition any `open` ancestor story/epic to `in-progress`. Together these eliminate the manual pre-flight. E5-S1-T3 addresses part (2) but not part (1).

**File**: `internal/ready/compute.go` (parent status gate), `cmd/trellis/claim.go` (auto-advance on claim)

---

## L17: Flag-heavy ID arguments increase command noise

**Observed**: Commands like `trls show`, `trls claim`, and `trls transition` require the `--issue` flag for the primary target ID.

**Impact**: Higher token usage and more typing/boilerplate for every operation. Standard CLI patterns often allow positional IDs for primary targets.

**Recommendation**: Support positional arguments for issue IDs while keeping the flag for backward compatibility or explicit disambiguation.

**File**: `cmd/trellis/`

---

## L18: `trls ready` hangs in non-TTY/agent environments

**Observed**: `trls ready` defaults to an interactive TUI. In an agent environment (like Gemini CLI), this causes the command to hang or timeout because there is no human to provide input for the TUI.

**Impact**: Agents fail to get work without explicit `--format agent` or `--format json` flags, which are easy to forget or discover.

**Recommendation**: Auto-detect TTY status. If `stdout` is not a TTY, default to a machine-readable format like `agent`.

**File**: `cmd/trellis/ready.go`

---

## L19: Lack of aggregated "Story Board" view makes tracking progress hard

**Observed**: `trls list --parent ID` shows children, but doesn't provide a high-level view of progress (outcomes, claims, blockers) for the whole story at once in a single output.

**Impact**: Users/agents must run `trls show` on every child to understand the current state of a story, leading to high turn counts and context usage.

**Recommendation**: Add a "board" or "summary" view that aggregates child statuses, outcomes, and current claims for a given parent in a table format.

**File**: `cmd/trellis/`

---

## L20: `trls-worker` loop is repetitive: transition then commit

**Observed**: The standard workflow involves `trls transition` followed immediately by `git add <files> .issues/` and `git commit`.

**Impact**: High repetition and risk of forgetting to include `.issues/` in the commit, leaving trellis state behind. This happened multiple times during E5-S3 execution.

**Recommendation**: The `trls-worker` skill should suggest a helper or the CLI should provide a bundled command that handles both transition and commit in one go, automatically including the op files.

**File**: `.claude/skills/trls-worker/SKILL.md`

---

## L21: Sub-agents struggle with large batch refactors without explicit strategy

**Observed**: The `generalist` sub-agent often completed only a few files of a 16+ file batch task (like TASK-05) before returning, even when instructed to do "all".

**Impact**: Requires multiple turns and manual verification by the main agent to ensure complete coverage across all files.

**Recommendation**: Add a "Batch Strategy" section to the worker skill instructing agents to build a manifest (e.g. `grep --names-only`) and process in small, verified chunks rather than attempting all at once.

**File**: `.claude/skills/trls-worker/SKILL.md`
