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

### 3. Record Progress

While implementing, record progress and decisions:

```
arm note ISSUE-ID --msg "..."
arm decision ISSUE-ID --topic X --choice Y --rationale Z
```

**Call `arm heartbeat ISSUE-ID` for any work taking more than a few minutes —
maximum once per minute.** Claims expire after the TTL; without periodic heartbeats
another worker may steal the claim. Issue heartbeat calls at natural checkpoints
(e.g. after each test run, after each file written).

### 4. Cite Every Issue Touched

Before completing work, cite every issue you touched or created:

```
arm source-link ISSUE-ID --source-id SOURCE-UUID        # if a source doc exists
# or
arm accept-citation ISSUE-ID --rationale "No external source; self-citing" --ci  # if no source exists
```

Do not leave issues uncited.

### 5. Pre-Transition Verification (mandatory)

Before transitioning any task to `done`, you **must** run the following checks.
Do NOT transition if either fails — fix the errors first.

```bash
go build ./...   # must exit zero; stops transition if compilation fails
make check       # lint + test + coverage-check + mutate + validate-skills + build
```

If `make check` is unavailable (e.g., the repo has no Makefile), fall back to:

```bash
go run ./cmd/armature --help   # confirms the binary at least compiles
```

**Completion order (never deviate):**
1. Run `go build ./...` — fix any compile errors.
2. Run `make check` — fix any lint/test/coverage failures.
3. `arm transition ISSUE-ID --to done --outcome "..."` — only after both pass.
4. Immediately stage scoped files and commit — do not leave the transition uncommitted before moving to step 6.

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

### 6. Complete and Commit

```
arm transition ISSUE-ID --to done --outcome "what was accomplished"
git add <each file from the task scope>
git commit -m "feat(ISSUE-ID): brief description of what was implemented"
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

See [docs/conventions.md](../../docs/conventions.md#commit-format) for the full commit format specification and examples.

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

**Full details:** See [docs/conventions.md](../../docs/conventions.md#test-naming-and-traceability) for comprehensive documentation of all naming and formatting conventions.

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
| Leave issues uncited | Run `arm source-link` or `arm accept-citation --ci` before returning |
| Repeating `transition` then `commit` manually | Use a bundled command: `arm transition ID ... && git add . .armature/ && git commit -m ...` |
| Transitioning to done while on main | `arm transition --to done` will fail on main/master branch — use feature branch or `--force` only in emergencies |
| Scope overlap WARNING on `arm validate` | Add `arm link --source ISSUE-A --dep ISSUE-B` so overlapping tasks execute serially, not in parallel |
| MISSING entries in `arm sources verify` | Run `arm sources sync` to fetch and fingerprint; re-run `arm sources verify` until all show OK |
| Test function named `TestFoo` instead of `TestFoo_REQ_ID` | Test skips `make trace-report`; requirement has no traceability | Use `TestFoo_REQ_ISSUE_ID`; see Test Naming and Traceability section |
| Struct-only tests when task touches a JSON-documented Go type | Tests are green but end-to-end serialization is broken; your skill documents `"status":"satisfied"` but Go unmarshals 0 or vice versa | Add a round-trip JSON fixture test (unmarshal from string, assert Go value; marshal Go value, assert string form). See Section 5b and `examples/json-roundtrip-test.go` |
