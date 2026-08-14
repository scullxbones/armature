---
name: armature-worker
description: >
  Use for task execution in an armature-managed repository. A worker receives
  a pre-claimed task from the Coordinator, implements it, records progress, and
  transitions the task to `done`. Enforces per-task commits and story-level
  push/PR strategy.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Worker

A worker receives a pre-claimed task from the Coordinator, implements it, records
progress, and transitions the task to `done`.

## Prerequisites

If `arm` is not found, stop and resolve this before proceeding.

Run `worker-init` once per machine/clone — the worker ID persists in local git config
across sessions:

```
arm worker-init --check || arm worker-init
```

`--check` is a no-op if the ID is already set. Re-running `worker-init` without
`--check` generates a new UUID, which is almost never what you want.

> Workers receive task context from the Coordinator at dispatch time.
> For story-level coordination and PR flow, see the **armature-coordinator** skill.

## DAG Hygiene Mandate

**`arm validate` and `arm doctor` must exit clean at all times.** This is non-negotiable.

Before transitioning any task to `done` and after completing your work, run:
```bash
arm validate       # zero ERRORs; all issues cited
arm doctor        # zero errors; no broken refs, orphaned ops, or cycles
```

If either exits non-zero, fix the reported issues before transitioning. Treat DAG decay the same way you treat failing tests — it is a blocker, not a warning to ignore.

Warnings from other stories must be resolved, not ignored. If `arm doctor` reports a D1 (commits referencing non-done issues) or D2 (stale claims) from unrelated work, clean them up before completing your task. DAG health is cumulative.

---

## Step-by-Step

### 1. Initialize
```
arm worker-init --check || arm worker-init
arm doctor
```

Run `arm doctor` to verify repo health (no broken parent refs, no orphaned ops,
no dependency cycles). Fix any errors before starting work.

### 2. Receive Task Context

The Coordinator dispatches you with a pre-claimed issue and the full output of
`arm render-context`. That output is your complete task specification — it
contains the issue description, definition of done, blocker outcomes, parent chain,
decisions, and notes.

**Do not open plan files. Do not read docs/superpowers/plans/. The render-context
output is sufficient.**

The issue is already claimed. Do NOT run `arm claim`. Do NOT run `arm worker-init`
again.

If this is a **remediation** dispatch, the coordinator has already run
`arm reopen` and `arm claim --worktree` (reusing the existing worktree).
Confirm with `arm show ISSUE-ID` that status is `claimed` or `in-progress`
before you edit files. If status is still `done` or `merged`, **stop** —
the harness hook treats those bindings as stale and passes through (no
scope enforcement, no hook heartbeats). Tell the coordinator to reopen
and reclaim; do not write under a stale binding. After the last remediating
commit, run the full gate at that HEAD and `arm transition ISSUE-ID --to done`
so the delivery gate evaluates the remediating commit.

**Transcript-free dispatch (normative).** Dispatch consists of the rendered
task spec and relevant file paths only — never an inherited transcript from a
prior agent. If you are re-dispatched for a remediation cycle, expect a
statement of what changed rather than a re-read of unchanged skills or
bundles; do not re-read files that were not called out as changed.

**Effort default (normative).** Task execution and task-level reviews default
to **medium** reasoning effort. High effort is assigned explicitly at
planning time (e.g., for concurrency, security, or cross-cutting refactor
work) or auto-escalates when a task enters remediation cycle 2 — treat a
second remediation dispatch as a signal to reason harder, not just to repeat
the previous pass.

When a coordinator creates a sub-task worktree from a story worktree, it should use
`arm claim SUBTASK-ID --worktree /path/to/new-task-worktree --from /path/to/story-worktree`
so the new task worktree retains the story branch and tip as its recorded parent
provenance.

### 3. Record Progress

While implementing, record progress and decisions:

```
arm note ISSUE-ID --msg "..."
arm decision ISSUE-ID --topic X --choice Y --rationale Z
```

**The harness hook automatically emits rate-limited heartbeats on every tool use
(PreToolUse events) for bound, non-stale claims** — one heartbeat per 5-minute window.
You only need to manually call `arm heartbeat ISSUE-ID` during **long stretches of
non-tool thinking work** (reading docs, analyzing code, etc.) where no tool calls
happen for more than 5 minutes. Without heartbeats, claims expire after the TTL and
another worker may steal the claim.

### 4. Cite Every Issue Touched

Before completing work, cite every issue you touched or created:

```
arm sources link ISSUE-ID --source-id SOURCE-UUID        # if a source doc exists
# or
arm sources accept-citation ISSUE-ID --rationale "No external source; self-citing" --ci  # if no source exists
```

Do not leave issues uncited.

### 5. Pre-Transition Verification (mandatory)

**Two-tier gate model (normative).** There are two gate profiles, and they play
different roles:

- **Fast gate** — use during implementation and on every intermediate
  remediation cycle. It is sufficient to keep iterating. Workers **MUST NOT**
  run the full gate on intermediate remediations — that wastes wall-clock time
  re-running suites the remediation could not have touched.
  - **When `make check-fast` exists** (LNGHZN-S10-T2), run that. It is
    deterministic and diff-routed.
  - **Until that target exists**, iterate with targeted existing commands
    (`make lint`, `make validate-skills`, `go test` on changed packages). Do
    not invoke a missing `make check-fast` target and do not substitute
    `make check`.
- **Full gate** (`make check`) — the publish gate. It is mandatory at the
  **final task head**: the clean delivery commit that will be reviewed and
  transitioned, immediately before `done`. After a remediation that creates a
  new delivery HEAD, that new HEAD is the final task head — run the full gate
  again there (this is not an "intermediate" run). A green fast gate never
  substitutes for it: only a green full gate confers delivery.

Run the fast gate as often as you like while iterating. Before transitioning
any task to `done`, you **must** run the full gate at the final committed
task head. Do NOT transition if it fails — fix, commit, and re-run the full
gate at the new HEAD.

```bash
go build ./...    # must exit zero; stops transition if compilation fails
# Fast gate (intermediate only): `make check-fast` if that target exists;
# otherwise targeted lint / validate-skills / package tests — not `make check`.
make check          # full/publish gate — at the clean delivery HEAD, before done
```

If `make check` is unavailable (e.g., the repo has no Makefile), fall back to:

```bash
go run ./cmd/armature --help   # confirms the binary at least compiles
```

**Completion order (never deviate):**
1. Run `go build ./...` — fix any compile errors.
2. Iterate with the fast gate during implementation and remediation (`make check-fast` when that target exists; otherwise targeted existing checks). Do not run the full gate on these intermediate passes.
3. Stage scoped files and commit with a conventional commit message (`<type>(ISSUE-ID): ...`) — the delivery gate's Clean Tree and Commit Reference checks require this to already be done before you transition. The full gate must run against this clean delivery HEAD, not a dirty pre-commit tree.
4. Run `make check` (the full gate) once at the commit from step 3. If it fails, fix, commit again, and re-run the full gate at the new HEAD. Do not transition on a failed full gate.
5. `arm transition ISSUE-ID --to done --outcome "..."` — only after the build, commit, and green full gate above are complete.

### 5b. Cross-Layer JSON Fixture Testing (when applicable)

If your task both **adds or modifies a Go type** AND **documents that type's JSON format**
in a skill (SKILL.md), CONTEXT.md, or other documentation, you must add at least one test
that exercises the serialization round-trip — not just struct construction.

**Why this matters:** A test using only Go struct literals never exercises `MarshalJSON`/
`UnmarshalJSON` and cannot catch a mismatch between a documented string value and an integer
enum representation. For example, a test can pass with integer marshaling while your skill
documents the field as a string, and end-to-end workflows break when another tool tries to
unmarshal that documented string format.

**How to implement:**
1. Parse JSON from a string literal matching your documented format
2. Unmarshal into the Go type and verify the value
3. Marshal the Go type back to JSON
4. Verify the JSON form matches your documented format (strings vs integers, field names, etc.)

See `examples/json-roundtrip-test.go` in this skill directory for a worked example to adapt
to your specific types and fields.

### 5c. The Delivery Gate

Before you transition to `done`, stage and commit your work (conventional commit format, see the Completion order above) — the gate expects a clean tree and an existing commit, not the other way around. When a claimed task, bug, feature, or story transitions to `done`, Armature runs a **delivery gate** against its bound worktree. An epic and an unclaimed coordinator-level story have no bound worktree and are exempt; a story with a recorded claimant but no discoverable claimed worktree fails closed.

**The Three Checks:**

1. **Clean Tree:** `git status --porcelain` must be empty. All work must be staged and committed; the worktree must be clean. (`.armature/` state is automatically excluded from this check — it is not considered outstanding work.)
2. **Scope Containment:** The diff between `HEAD` and a base commit must be a subset of the issue's declared scope (verified via `internal/claim.IsWithinScope`). The base is selected in order: a fresh `git merge-base` against the recorded parent branch, the claim-time recorded SHA, then a merge-base against the first available `origin/main`, `origin/master`, `main`, or `master`. This prevents scope creep: you cannot deliver changes outside the issue's boundaries.
3. **Commit Reference:** At least one commit since the base commit must match the conventional-commit format `<type>(<ISSUE-ID>): ...` per `docs/conventions.md`. This ensures your work is traceable and tied to the issue ID.

**On Failure:**

If any check fails, the transition is refused. The error message lists each failed check with a remediation step:

- **Clean Tree failure:** Commit or discard outstanding changes.
- **Scope Containment failure:** Narrow the diff to the declared scope or broaden the scope if the changes are justified.
- **Commit Reference failure:** Add at least one properly-formatted commit (e.g., `feat(LNGHZN-S4-T3): document delivery gate`).

**Skipping the Gate:**

The `--skip-delivery-gate` flag bypasses all three checks only with `--to done`; other target states reject it. Use this **only when the gate assumption does not hold** — for example, in a docs-only or demo-transcript task where you are not doing real delivery work, or when an external constraint makes gate compliance impossible.

```bash
arm transition LNGHZN-S4-T3 --to done --skip-delivery-gate \
  --outcome "Skipped gate: demo-transcript task does not execute real delivery"
```

When you skip the gate, the transition op records `Payload.SkippedDeliveryGate` as an audit flag and its `outcome` records your supplied reason. **However, prefer fixing the underlying issue over reaching for the override** — if you have uncommitted changes, commit them; if scope has drifted, narrow it; if commits are missing conventional-format messages, add them.

### 6. Complete and Commit

```
git add <each file from the task scope>
git commit -m "feat(ISSUE-ID): brief description of what was implemented"
make check   # full gate at the clean delivery HEAD from the commit above
arm transition ISSUE-ID --to done --outcome "what was accomplished"
```

Stage files **explicitly by name or path** — taken directly from the task's `scope` field.
Do **not** use `git commit -am`: the `-a` flag only auto-stages already-tracked files and
silently skips new files and directories created by the task.

**Do not stage `.armature/`** — ops are automatically committed to the `_armature` branch
and will be delivered separately.

Record a concrete outcome. Commit immediately after the task — small focused commits
are easier to review.

**Do NOT stage `.armature/` in code commits.** Armature automatically commits ops to
the separate `_armature` ops branch after each command. The `.armature/` directory
in your code worktree is stale — including it in code commits will cause pre-commit
checks to fail. Ops are already persisted on the `_armature` branch and will be
delivered separately.

Stage only the scoped code files (those listed in the task's `scope` field):

**Commit message format:** `<type>(<ISSUE-ID>): <description>`
Types: `feat`, `fix`, `refactor`, `test`, `docs`, `style`, `polish`

See `docs/conventions.md` (commit format section) in the armature repo for the full commit format specification and examples.

**Branch discipline:** `arm transition --to done` will fail if you are on the
main or master branch (unless you use `--force`). The `--force` flag should only
be used in exceptional cases (e.g., emergency hotfixes to main).

## Valid Transition Targets

| Target | When |
|---|---|
| `done` | Work complete |
| `blocked` | Cannot proceed, external dependency |
| `cancelled` | Work abandoned |

**Valid status values use hyphens:** `in-progress`, `done`, `cancelled`, `blocked`. Underscores are rejected.

## Setting Your Log Slot

When the Coordinator dispatches you as part of a parallel wave, it will assign you
a log slot. Set it before running any `arm` command:

```
export ARM_LOG_SLOT=<assigned-slot>
```

This ensures your ops go to a slot-specific log file and do not race with other
parallel workers. The Coordinator assigns slots — workers set the slot they are
given but do not assign slots to others.

For tasks spanning 10+ files, see `references/batch-strategy.md`.

## Test Naming and Traceability

Test functions that verify acceptance criteria must follow the naming convention:

```
Test<Description>_REQ_<ISSUE-ID>
```

Where `<ISSUE-ID>` is the task or story ID (e.g., `DF-S5-T5`). This pattern makes
the test visible to `make trace-report` and ties it back to the requirement that
motivated it.

Examples:
- `TestParseTokenTypes_REQ_STORY_T1`
- `TestEdgeCases_REQ_DF_S5_T5`

**Full details:** See `docs/conventions.md` (test naming and traceability section) in the armature repo for comprehensive documentation of all naming and formatting conventions.

## Common Mistakes

| Mistake | Fix |
|---|---|
| `arm: command not found` | Stop and resolve: install arm and ensure `~/.local/bin` is on PATH |
| Reading plan files for task instructions | Use `render-context` output only |
| Using `in_progress` (underscore) | Use `in-progress` (hyphen) |
| Skipping `worker-init` on a fresh clone | Required once per clone — ops without worker ID will fail |
| Running `worker-init` every session | Generates a new UUID each time, creating phantom workers; use `--check` to verify instead |
| Running `arm claim` when dispatched by Coordinator | The Coordinator pre-claims the issue; do not re-claim |
| Skipping heartbeat on long tasks | Claim expires after TTL; other workers can steal it |
| Skipping commit after task | Small commits make review and revert tractable |
| Using `git commit -am` | `-a` only stages tracked files — new files and directories are silently skipped; always use explicit `git add <scope files>` |
| Including `.armature/` in `git add` | Stages stale data; ops are already on `_armature` branch — omit `.armature/` from code commits |
| Leave issues uncited | Run `arm sources link` or `arm sources accept-citation --ci` before returning |
| Running `transition` before committing | The delivery gate requires a clean tree and an existing commit *before* you transition — stage scoped files and commit first, then run `arm transition ID --to done ...` (see Section 5c) |
| Transitioning to done while on main | `arm transition --to done` will fail on main/master branch — use feature branch or `--force` only in emergencies |
| Scope overlap WARNING on `arm validate` | Add `arm link --source ISSUE-A --dep ISSUE-B` so overlapping tasks execute serially, not in parallel |
| MISSING entries in `arm sources verify` | Run `arm sources sync` to fetch and fingerprint; re-run `arm sources verify` until all show OK |
| Test function named `TestFoo` instead of `TestFoo_REQ_ID` | Test skips `make trace-report`; requirement has no traceability | Use `TestFoo_REQ_ISSUE_ID`; see Test Naming and Traceability section |
| Struct-only tests when task touches a JSON-documented Go type | Tests are green but end-to-end serialization is broken; your skill documents `"status":"satisfied"` but Go unmarshals 0 or vice versa | Add a round-trip JSON fixture test (unmarshal from string, assert Go value; marshal Go value, assert string form). See Section 5b and `examples/json-roundtrip-test.go` |
