# arm-worker Build Integration and Git Workflow Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate arm-worker skill deployment into `make skill`, update the skill to enforce per-task git commits, and establish a clear story-level push/PR strategy.

**Architecture:**
The `arm` skill (CLI reference) is deployed by `make skill` by concatenating source files from `docs/`. The `arm-worker` skill (workflow driver) lives directly in `.claude/skills/arm-worker/SKILL.md` with no build source. We move it to `docs/` for consistency, update `make skill` to deploy it alongside `arm`, and update the skill content to enforce per-task commits and story-level PR guidance.

**Design decisions baked in:**
- **Commits: per task** — after each `arm transition --to done`, the agent commits staged changes. Small focused commits are easiest to review and revert.
- **Push: per story** — the skill recommends (does not automate) pushing and opening a PR when a story is complete (all tasks merged). The user runs `git push` explicitly. Automating pushes risks sending half-baked branches to shared remotes.
- **PR granularity: one PR per story** — not per task (creates review overload), not per epic (too large to review meaningfully). Story-level PRs give reviewers clear context and scope.

**Tech Stack:** Makefile, Markdown (skill format)

---

## Chunk 1: Makefile and source file changes

### Task 1: Move arm-worker skill source to docs/ and update Makefile

**Files:**
- Create: `docs/arm-worker-skill-meta.yaml`
- Create: `docs/arm-worker-SKILL.md` (moved from `.claude/skills/arm-worker/SKILL.md`)
- Modify: `Makefile`

- [ ] **Step 1: Verify current state**

Run: `make skill && ls .claude/skills/`
Expected: `arm/` and `arm-worker/` directories exist, `arm-worker/SKILL.md` contains current skill content.

- [ ] **Step 2: Create metadata file for arm-worker**

Create `docs/arm-worker-skill-meta.yaml`:

```yaml
---
name: arm-worker
description: >
  Use when starting work in a trellis-managed repository — picks up ready
  issues, claims them, assembles context, and drives implementation. Enforces
  per-task commits and story-level push/PR strategy.
compatibility: Designed for Claude Code. Requires arm on PATH (run make install).
---

```

- [ ] **Step 3: Create docs/arm-worker-SKILL.md with updated content**

See Task 2 for skill content. Do not create the file yet — write both tasks before creating.

- [ ] **Step 4: Update Makefile skill target to deploy arm-worker**

In `Makefile`, replace the `skill` target with:

```makefile
skill: build
	mkdir -p .claude/skills/arm/scripts
	cat docs/arm-skill-meta.yaml docs/SKILL.md > .claude/skills/arm/SKILL.md
	cp bin/arm .claude/skills/arm/scripts/arm
	chmod +x .claude/skills/arm/scripts/arm
	mkdir -p .claude/skills/arm-worker
	cat docs/arm-worker-skill-meta.yaml docs/arm-worker-SKILL.md > .claude/skills/arm-worker/SKILL.md
	@echo "Deployed arm and arm-worker skills to .claude/skills/"
```

- [ ] **Step 5: Verify Makefile compiles**

Run: `make -n skill`
Expected: Prints commands without error.

- [ ] **Step 6: Commit the Makefile change**

```bash
git add Makefile
git commit -m "build: deploy arm-worker skill via make skill"
```

---

## Chunk 2: Skill content update

### Task 2: Update arm-worker skill with commit/push/PR guidance

**Files:**
- Create: `docs/arm-worker-SKILL.md`

The updated skill adds:
1. A "Commit" step after each task completion
2. A "Story complete" section guiding push + PR creation
3. Clearer guidance on commit message format

- [ ] **Step 1: Create docs/arm-worker-SKILL.md**

```markdown
# Armature Worker Loop

Armature is the source of truth for what to work on and how. Do not read external plan files during execution. `render-context` output is your complete task specification.

## Prerequisites

`arm` must be on your PATH. Run `make install` from the trellis repo root if it isn't:

\```
make install   # installs to ~/.local/bin/arm
\```

If `arm` is not found, stop and resolve this before proceeding.

## The Loop

\```dot
digraph worker_loop {
    "worker-init" [shape=box];
    "arm ready" [shape=box];
    "Empty?" [shape=diamond];
    "Story done?" [shape=diamond];
    "Pick issue" [shape=box];
    "claim + render-context" [shape=box];
    "Dispatch subagent" [shape=box];
    "git commit" [shape=box];
    "transition --to done" [shape=box];
    "push + open PR" [shape=box];
    "Done" [shape=doublecircle];

    "worker-init" -> "arm ready";
    "arm ready" -> "Empty?" ;
    "Empty?" -> "Done" [label="yes"];
    "Empty?" -> "Pick issue" [label="no"];
    "Pick issue" -> "claim + render-context";
    "claim + render-context" -> "Dispatch subagent";
    "Dispatch subagent" -> "transition --to done";
    "transition --to done" -> "git commit";
    "git commit" -> "Story done?";
    "Story done?" -> "push + open PR" [label="yes"];
    "Story done?" -> "arm ready" [label="no"];
    "push + open PR" -> "arm ready";
}
\```

## Step-by-Step

### 1. Initialize
\```
arm worker-init
\```
Run once per agent session. Registers a unique worker ID in git config.

### 2. Find Ready Work
\```
arm ready
\```
Lists unblocked, unclaimed issues. If empty, all work is done or blocked — stop.

### 3. Claim and Assemble Context
\```
arm claim --issue ISSUE-ID
arm render-context --issue ISSUE-ID --budget 4000
\```
Claim before reading context. The `render-context` output is your complete task specification — it contains the issue description, definition of done, blocker outcomes, parent chain, decisions, and notes.

**Do not open plan files. Do not read docs/superpowers/plans/. The render-context output is sufficient.**

### 4. Dispatch Subagent

Dispatch a subagent with:
- The full `render-context` output as the task description
- The `arm` skill loaded for API reference

The subagent should:
- Record progress with `arm note --issue ID --msg "..."`
- Record decisions with `arm decision --issue ID --topic X --choice Y --rationale Z`
- Call `arm heartbeat --issue ID` for long-running work (max once/minute)

### 5. Complete and Commit

\```
arm transition --issue ISSUE-ID --to done --outcome "what was accomplished"
git add -p   # stage relevant changes
git commit -m "feat(ISSUE-ID): brief description of what was implemented"
\```

Record a concrete outcome. Commit immediately after each task — small focused commits are easier to review.

**Commit message format:** `<type>(<ISSUE-ID>): <description>`
Types: `feat`, `fix`, `refactor`, `test`, `docs`

Then return to step 2.

### 6. Story Complete — Push and PR

When `arm ready` returns empty and the story's tasks are all done, push and open a PR:

\```
git push -u origin HEAD
# Open a PR targeting your main/base branch
# PR title: the story title
# PR body: list each task ISSUE-ID and its outcome
\```

**One PR per story** — not per task (creates review overhead), not per epic (too large to review). Story-level PRs give reviewers clear scope.

## Valid Transition Targets

| Target | When |
|---|---|
| `done` | Work complete |
| `blocked` | Cannot proceed, external dependency |
| `cancelled` | Work abandoned |

**Valid status values use hyphens:** `in-progress`, `done`, `cancelled`, `blocked`. Underscores are rejected.

## If `arm ready` Returns Nothing

- Check for blocked issues: state may be blocked by incomplete dependencies
- Check issue types: `ready` shows `task`, `feature`, and `story` types
- All work may genuinely be complete

## Common Mistakes

| Mistake | Fix |
|---|---|
| `arm: command not found` | Run `make install`, ensure `~/.local/bin` is on PATH |
| Reading plan files for task instructions | Use `render-context` output only |
| Using `in_progress` (underscore) | Use `in-progress` (hyphen) |
| Skipping `worker-init` | Required — ops without worker ID will fail |
| Skipping heartbeat on long tasks | Claim expires after TTL; other workers can steal it |
| Skipping commit after task | Small commits make review and revert tractable |
| Auto-pushing after every task | Push once per story to avoid noisy remote history |
```

- [ ] **Step 2: Run make skill to deploy updated skill**

```bash
make skill
```

Expected: `.claude/skills/arm-worker/SKILL.md` contains the new content including the "git commit" step and story PR guidance.

- [ ] **Step 3: Verify deployed content matches source**

Run: `diff <(cat docs/arm-worker-skill-meta.yaml docs/arm-worker-SKILL.md) .claude/skills/arm-worker/SKILL.md`
Expected: No diff output (files identical).

- [ ] **Step 4: Commit skill source and verify .claude/skills is in .gitignore**

Check `.gitignore` - the `.claude/skills/` directory should NOT be committed (it's a build artifact). Only `docs/arm-worker-SKILL.md` and `docs/arm-worker-skill-meta.yaml` are committed.

```bash
git add docs/arm-worker-skill-meta.yaml docs/arm-worker-SKILL.md
git commit -m "feat: add arm-worker skill source and commit/PR workflow guidance"
```

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-03-17-arm-worker-build-integration.md`. Ready to execute?

**Execution path:** Use superpowers:subagent-driven-development or superpowers:executing-plans.
