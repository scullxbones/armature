<!-- CANONICAL SOURCE: edit this file, not .claude/skills/trls-worker/SKILL.md — run `make skill` to regenerate the deployed copy -->

# Trellis Worker Loop

Trellis is the source of truth for what to work on and how. Do not read external plan files during execution. `render-context` output is your complete task specification.

## Prerequisites

`trls` must be on your PATH. Run `make install` from the trellis repo root if it isn't:

```
make install   # installs to ~/.local/bin/trls
```

If `trls` is not found, stop and resolve this before proceeding.

## The Loop

```dot
digraph worker_loop {
    "worker-init" [shape=box];
    "trls ready" [shape=box];
    "Empty?" [shape=diamond];
    "Story done?" [shape=diamond];
    "Pick issue" [shape=box];
    "claim + render-context" [shape=box];
    "Dispatch subagent" [shape=box];
    "git commit" [shape=box];
    "transition --to done" [shape=box];
    "push + open PR" [shape=box];
    "Done" [shape=doublecircle];

    "worker-init" -> "trls ready";
    "trls ready" -> "Empty?" ;
    "Empty?" -> "Done" [label="yes"];
    "Empty?" -> "Pick issue" [label="no"];
    "Pick issue" -> "claim + render-context";
    "claim + render-context" -> "Dispatch subagent";
    "Dispatch subagent" -> "transition --to done";
    "transition --to done" -> "git commit";
    "git commit" -> "Story done?";
    "Story done?" -> "push + open PR" [label="yes"];
    "Story done?" -> "trls ready" [label="no"];
    "push + open PR" -> "trls ready";
}
```

## Step-by-Step

### 1. Initialize
```
trls worker-init --check || trls worker-init
trls doctor
```
Run `worker-init` once per machine/clone — the worker ID persists in local git config across sessions. `--check` is a no-op if the ID is already set. Re-running `worker-init` without `--check` generates a new UUID, which is almost never what you want.

Run `trls doctor` after init to verify repo health (no broken parent refs, no orphaned ops, no dependency cycles). Fix any errors before claiming work.

### 2. Find Ready Work
```
trls ready
```
Lists unblocked, unclaimed issues. If empty, all work is done or blocked — stop.

### 3. Claim and Assemble Context
```
trls claim ISSUE-ID
trls render-context ISSUE-ID --budget 4000
```
Claim before reading context. The `render-context` output is your complete task specification — it contains the issue description, definition of done, blocker outcomes, parent chain, decisions, and notes.

**Do not open plan files. Do not read docs/superpowers/plans/. The render-context output is sufficient.**

### 4. Dispatch Subagent

Dispatch a subagent with:
- The full `render-context` output as the task description
- The `trls` skill loaded for API reference

The subagent should:
- Record progress with `trls note ID --msg "..."`
- Record decisions with `trls decision ID --topic X --choice Y --rationale Z`
- **Call `trls heartbeat ID` for any work taking more than a few minutes — maximum once per minute.** Claims expire after the TTL; without periodic heartbeats another worker may steal the claim. Issue heartbeat calls at natural checkpoints (e.g. after each test run, after each file written).
- **Cite every issue it touches or creates** — before returning, run `trls source-link` for any issue that has a recoverable source doc, or `trls accept-citation --ci` if no source exists. Do not leave issues uncited.

### 5. Complete and Commit

Before completing work, ensure you are on a **feature branch** (not main/master):
```
git checkout -b feat/ISSUE-ID    # Create feature branch before starting
```

Then complete the task:
```
trls transition ISSUE-ID --to done --outcome "what was accomplished"
git add <code files...> .issues/   # always include .issues/ — ops must travel with code
git commit -m "feat(ISSUE-ID): brief description of what was implemented"
```

**Branch + PR discipline:** `trls transition --to done` will fail if you are on the main or master branch (unless you use `--force`). This enforcement ensures all work goes through feature branches and pull requests for review. The `--force` flag should only be used in exceptional cases (e.g., emergency hotfixes to main).

Record a concrete outcome. Commit immediately after each task — small focused commits are easier to review.

**Pro-tip: Bundled Workflow**
To avoid forgetting `.issues/` or the commit, combine these into a single command or use a shell alias:
```bash
trls transition ISSUE-ID --to done --outcome "..." && git add . .issues/ && git commit -m "feat(ISSUE-ID): ..."
```

**Always stage `.issues/` alongside code files.** Every `trls` command (claim, transition, note, decision, heartbeat) writes ops to `.issues/`. If you omit `.issues/` from the commit, those ops are left behind and will not be delivered with the code.

**Dual-branch mode exception:** If `git config --local trellis.mode` returns
`dual-branch`, ops are automatically committed to the `_trellis` branch by
each `trls` command — they do **not** appear as pending changes in the code
worktree. **Omit `.issues/` from `git add`.** Including it stages stale data
from the code branch's (unused) `.issues/` copy and will fail the pre-commit
guard.

**Commit message format:** `<type>(<ISSUE-ID>): <description>`
Types: `feat`, `fix`, `refactor`, `test`, `docs`

Then return to step 2.

### 6. Story Complete — Sync, Push, and PR

When `trls ready` returns empty and the story's tasks are all done:

**a. Verify citation coverage, then transition the story:**
```
trls validate   # must show COVERAGE: N/N cited with no ERROR lines
trls transition STORY-ID --to done --outcome "story-level summary"
git status   # check for unstaged .issues/ changes
git add .issues/ && git commit -m "chore(STORY-ID): sync trellis state"
```

If `trls validate` shows uncited nodes, source-link or accept-citation them before transitioning.

Story/epic-level transitions, and any notes or decisions recorded between task commits, generate ops that have no code to bundle with. This mop-up commit ensures nothing is left behind before pushing.

**b. Push and open a PR:**
```
git push -u origin HEAD
# Open a PR targeting your main/base branch
# PR title: the story title
# PR body: list each task ISSUE-ID and its outcome
```

**One PR per story** — not per task (creates review overhead), not per epic (too large to review). Story-level PRs give reviewers clear scope.

## Valid Transition Targets

| Target | When |
|---|---|
| `done` | Work complete |
| `blocked` | Cannot proceed, external dependency |
| `cancelled` | Work abandoned |

**Valid status values use hyphens:** `in-progress`, `done`, `cancelled`, `blocked`. Underscores are rejected.

## If `trls ready` Returns Nothing

- Check for blocked issues: state may be blocked by incomplete dependencies
- Check issue types: `ready` shows `task`, `feature`, and `story` types
- All work may genuinely be complete

## Parallel Subagents (Advanced)

By default the worker loop is sequential: claim → dispatch → commit → repeat.
When tasks are **independent** (no shared files, no ordering dependency), you can
dispatch subagents in parallel using git worktrees. To avoid merge conflicts on
the ops log, each subagent must write to its own log slot:

    # Set before dispatching each subagent
    export TRLS_LOG_SLOT=a   # subagent A
    export TRLS_LOG_SLOT=b   # subagent B

**Rules:**
- The orchestrator always runs with `TRLS_LOG_SLOT` **unset** (claims and story
  transitions go to the plain `<worker-id>.log`).
- Each parallel subagent sets a distinct, short slot name (`a`, `b`, `c`, or the
  issue ID) before invoking any `trls` command.
- Slot names must be unique within a parallel batch — reusing a slot across two
  concurrent agents defeats the purpose.
- Slot files (`<worker-id>~<slot>.log`) are permanent. They are committed
  alongside code files exactly like the plain log: `git add .issues/ code/`.
- After the batch, run `trls validate` and `make check` from the orchestrator
  (with `TRLS_LOG_SLOT` unset) before pushing.

**Common mistake:** Setting `TRLS_LOG_SLOT` in the orchestrator's shell and
forgetting to unset it — orchestrator ops land in a slot file and may not be
seen by `trls ready` until materialization catches up.

### When Claude Code is the Dispatcher

The pattern above assumes the orchestrator is itself a shell-running agent that
can `export` env vars before spawning subprocesses. When **Claude Code dispatches
agents via the Agent tool** (not via trls-worker), each agent runs in its own
isolated shell — the coordinator's environment is never inherited.

**The slot cannot be set by the coordinator's `export`. It must be embedded
verbatim in each agent's prompt.**

Coordinator checklist at dispatch time:

1. Assign a unique slot to each agent before writing the prompts:
   `T1 → slot=t1`, `T2 → slot=t2`, or use the short issue ID.
2. Include this line as the **first instruction** in each agent's prompt:
   ```
   Before running any trls command, run: export TRLS_LOG_SLOT=<assigned-slot>
   ```
3. Keep the coordinator's own shell free of `TRLS_LOG_SLOT` — orchestrator
   ops (claims, story transitions) must go to the plain `<worker-id>.log`.

If you forget to assign slots, all parallel agents collapse into one log file.
Ops are still recorded correctly, but per-agent attribution is lost and
concurrent `AppendAndCommit` calls may race on the `_trellis` branch.

### Parallel Dev Environment: go.work and Coordinator LSP Review

Subagents dispatched to isolated worktrees have no IDE or LSP connection. Every
compilation or lint error requires a full `make check` cycle (~20–30 s) rather
than instant inline feedback. Two complementary mitigations reduce this cost.

**1. Per-worktree go.work — fast `gopls` feedback inside the agent**

Before dispatching each subagent, write a minimal `go.work` file into the
worktree root (the file is gitignored):

```bash
# Coordinator writes this once per worktree before dispatch
cat > /tmp/trellis-w<slot>/go.work <<'EOF'
go 1.22

use .
EOF
```

With `go.work` present, the subagent can run:

```bash
gopls check ./...   # fast per-file feedback, no make required
```

This catches import-path and format errors (e.g. `goimports` violations) in
seconds rather than after a full test suite run.

**go.work must be gitignored.** Add `/go.work` (and `/go.work.sum`) to the
worktree's `.gitignore` (or the repo's root `.gitignore`) so it is never staged.

**2. Coordinator LSP review before merging**

After all subagents return and before merging their branches, the coordinator
runs `gopls diagnostics` on every file changed in the wave:

```bash
# From coordinator shell — runs against files in each worktree
for wt in /tmp/trellis-w<slot1> /tmp/trellis-w<slot2>; do
    gopls diagnostics "$wt"/... 2>/dev/null
done
```

Any ERROR-level diagnostic (undefined symbol, import cycle, format violation)
must be fixed before merging. This catches issues that pass `make check` inside
an agent's isolated worktree but fail after the branches are merged together
(e.g. a `goimports` ordering that was locally correct but conflicts with a
symbol renamed in a sibling agent's branch).

**Coordinator checklist before merge:**

1. All subagents have returned and committed.
2. Run `gopls diagnostics` on each worktree's changed files — fix any ERRORs.
3. Merge branches into the story feature branch.
4. Run `make check` once on the merged result.
5. Push only after `make check` is green.

## Batch Strategy (Advanced)

When a task involves a large number of files (e.g. refactoring 10+ files), do not
attempt to process them all in a single turn. This leads to incomplete work and
high token usage. Instead:

1.  **Build a Manifest:** Use `grep --names-only` or `glob` to find all files that
    need changes. Save this list to a temporary file or a note.
2.  **Process in Chunks:** Process the files in small batches (e.g. 3-5 files at a
    time).
3.  **Verify each Chunk:** Run tests/linting after each chunk to ensure no
    regressions were introduced.
4.  **Heartbeat:** Call `trls heartbeat ID` after each chunk.
5.  **Final Review:** Once all files are processed, run a final global check
    before transitioning the task to `done`.

## Common Mistakes

| Mistake | Fix |
|---|---|
| `trls: command not found` | Run `make install`, ensure `~/.local/bin` is on PATH |
| Reading plan files for task instructions | Use `render-context` output only |
| Using `in_progress` (underscore) | Use `in-progress` (hyphen) |
| Skipping `worker-init` on a fresh clone | Required once per clone — ops without worker ID will fail |
| Running `worker-init` every session | Generates a new UUID each time, creating phantom workers; use `--check` to verify instead |
| Skipping heartbeat on long tasks | Claim expires after TTL; other workers can steal it |
| Skipping commit after task | Small commits make review and revert tractable |
| Omitting `.issues/` from `git add` | Ops left behind, not delivered with code; always include `.issues/` in every commit (single-branch mode only) |
| Including `.issues/` in `git add` in dual-branch mode | Stages stale data; ops are already on `_trellis` branch — omit `.issues/` from code commits |
| Forgetting `TRLS_LOG_SLOT` when dispatching via Agent tool | All parallel agents share one log; embed `export TRLS_LOG_SLOT=<slot>` as the first line of each agent's prompt |
| No mop-up commit before push | Story/epic transitions and between-task ops never get committed; run `git add .issues/ && git commit` before `git push` |
| Auto-pushing after every task | Push once per story to avoid noisy remote history |
| Leave issues uncited | Run `trls source-link` or `trls accept-citation --ci` before the subagent returns |
| Repeating `transition` then `commit` manually | Use a bundled command or alias: `trls transition ID ... && git add . .issues/ && git commit -m ...` |
| Skipping `trls validate` at story close | Citation debt accumulates silently; validate before transitioning the story |
| Pushing directly to main branch | Create a feature branch first: `git checkout -b feat/ISSUE-ID` before starting work |
| Transitioning to done while on main | `trls transition --to done` will fail on main/master branch — use feature branch or `--force` only in emergencies |

| Scope overlap WARNING on `trls validate` | Add `trls link --source ISSUE-A --dep ISSUE-B` so overlapping tasks execute serially, not in parallel |
| MISSING entries in `trls sources verify` | Run `trls sources sync` to fetch and fingerprint; re-run `trls sources verify` until all show OK |
| No go.work in parallel worktrees | Agents have no LSP; write a minimal `go.work` (`use .`) into each worktree before dispatch so `gopls check ./...` works |
| Coordinator skips LSP review before merge | Run `gopls diagnostics` on each worktree's changed files after agents return; catches format/import errors before they land on the merged branch |
