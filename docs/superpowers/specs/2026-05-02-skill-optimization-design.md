# Skill Optimization: Progressive Disclosure + Metadata Merge

**Date:** 2026-05-02
**Status:** Draft

## Overview

Optimize armature's agent skills for context efficiency using three levers: merge
`meta.yaml` into `SKILL.md` frontmatter (removing the assembly step), restructure
skill bodies with progressive disclosure so agents only load content relevant to
their current task, and ship a new `arm install-skills` subcommand so end users
can deploy skills without accessing the armature source repo.

The work is executed as an armature story using the skills themselves — each
improvement is immediately exercised by subsequent tasks, providing live
dogfooding across the full planner → coordinator → worker → auditor cycle.

## Goals

- Every skill core body under the 500-line / 5000-token spec limit
- Agents load only the content needed for their active role and mode
- No `meta.yaml` files; all skill metadata lives in `SKILL.md` frontmatter
- No `make install` references in skill bodies; `AGENTS.md` is the setup home
- End users can deploy skills with a single `arm install-skills` command
- `dist-skills` zip distribution deprecated

## Non-Goals

- Rewriting skill content beyond what's needed for structure and cleanup
- Changing the `arm` CLI beyond adding `install-skills`
- Skill evaluation / description trigger-rate testing (separate effort)
- Removing `dist-skills` from Makefile (tracked as follow-on; non-goal for this story)

---

## Design

### 1. Frontmatter Merge

Each skill's `meta.yaml` fields (`name`, `description`, `compatibility`) move
into the `---` frontmatter block at the top of `SKILL.md`. The `meta.yaml` files
are deleted. The `<!-- CANONICAL SOURCE -->` comment in each skill body is also
removed — it references the old assembly workflow.

**Before (two files):**
```
skills/armature-worker/
  meta.yaml       ← name, description, compatibility
  SKILL.md        ← body only, no frontmatter
```

**After (one file):**
```
skills/armature-worker/
  SKILL.md        ← frontmatter + body
```

**SKILL.md structure:**
```markdown
---
name: armature-worker
description: >
  Use when starting work in an armature-managed repository...
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Worker
...
```

### 2. Skills Directory Move (required for §4 embedding)

As part of the embedding work (see §3), `skills/` moves from the repo root to
`internal/skillsembed/skills/`. This co-locates the skills with their embed
package and is required by Go's `//go:embed` path constraints (see §3 for
rationale). The Makefile and any documentation referencing `skills/` must be
updated to `internal/skillsembed/skills/`.

### 3. Makefile Changes (skill + deploy-skills targets)

The `skill` target currently assembles `meta.yaml + banner + SKILL.md`. After
the merge it becomes a straight copy. A new `deploy-skills` target skips the
binary rebuild for skill-only changes. Both targets copy `SKILL.md`, `scripts/`,
and `references/` for each skill. The loop path changes from `skills/*/` to
`internal/skillsembed/skills/*/` following the directory move in §2.

**`skill` target (simplified):**
```makefile
SKILLS_DIR := internal/skillsembed/skills

skill: build
    @for name in $(SKILLS_DIR)/*/; do \
        name=$$(basename "$$name"); \
        [ -f "$(SKILLS_DIR)/$$name/SKILL.md" ] || continue; \
        for harness in claude gemini; do \
            mkdir -p ".$$harness/skills/$$name"; \
            cp "$(SKILLS_DIR)/$$name/SKILL.md" ".$$harness/skills/$$name/SKILL.md"; \
            if [ -d "$(SKILLS_DIR)/$$name/scripts" ]; then \
                mkdir -p ".$$harness/skills/$$name/scripts"; \
                cp "$(SKILLS_DIR)/$$name/scripts/"* ".$$harness/skills/$$name/scripts/"; \
                chmod +x ".$$harness/skills/$$name/scripts/"*; \
            fi; \
            if [ -d "$(SKILLS_DIR)/$$name/references" ]; then \
                mkdir -p ".$$harness/skills/$$name/references"; \
                cp "$(SKILLS_DIR)/$$name/references/"* ".$$harness/skills/$$name/references/"; \
            fi; \
        done; \
    done
    @echo "Deployed skills to .claude/skills/ and .gemini/skills/"
```

**New `deploy-skills` target** — no binary rebuild, for skill-only changes:
```makefile
deploy-skills:
    @for name in $(SKILLS_DIR)/*/; do \
        name=$$(basename "$$name"); \
        [ -f "$(SKILLS_DIR)/$$name/SKILL.md" ] || continue; \
        for harness in claude gemini; do \
            mkdir -p ".$$harness/skills/$$name"; \
            cp "$(SKILLS_DIR)/$$name/SKILL.md" ".$$harness/skills/$$name/SKILL.md"; \
            if [ -d "$(SKILLS_DIR)/$$name/scripts" ]; then \
                mkdir -p ".$$harness/skills/$$name/scripts"; \
                cp "$(SKILLS_DIR)/$$name/scripts/"* ".$$harness/skills/$$name/scripts/"; \
                chmod +x ".$$harness/skills/$$name/scripts/"*; \
            fi; \
            if [ -d "$(SKILLS_DIR)/$$name/references" ]; then \
                mkdir -p ".$$harness/skills/$$name/references"; \
                cp "$(SKILLS_DIR)/$$name/references/"* ".$$harness/skills/$$name/references/"; \
            fi; \
        done; \
    done
    @echo "Deployed skills to .claude/skills/ and .gemini/skills/"
```

`deploy-skills` is not added to `check` — the `check` target retains `skill`
unchanged (still rebuilds binary + deploys skills on every `make check` run;
the binary rebuild cost is acceptable in the CI context).

The `clean` target requires no modification. It removes `.claude/skills/` and
`.gemini/skills/` (deployed harness directories) — not `internal/skillsembed/skills/`
(source). The source directory must not be cleaned.

`dist-skills` remains in the Makefile but its `help` echo line is updated to:
```
  make dist-skills - DEPRECATED: use arm install-skills instead
```
Removal of the target itself is a follow-on task.

### 4. `arm install-skills` Subcommand

A new CLI subcommand that deploys armature's bundled skills from the binary to
the appropriate harness directories. Replaces the zip distribution model.

#### Embedding

Go's `//go:embed` prohibits `..` path components — the embedded path must be a
subdirectory of the package directory containing the directive. `skills/` cannot
be embedded from `cmd/armature/` or from any sibling package without moving the
directory.

**Solution: move `skills/` to `internal/skillsembed/skills/`** (see §2). The
embed directive then uses a simple relative path with no traversal:

**New file: `internal/skillsembed/embed.go`**
```go
package skillsembed

import "embed"

//go:embed skills
var SkillsFS embed.FS
```

The `install-skills` command imports
`github.com/scullxbones/armature/internal/skillsembed` and uses
`skillsembed.SkillsFS` to walk and deploy the embedded files.

#### Directory traversal

`install-skills` walks `skillsembed.SkillsFS` under the `skills/` root and
deploys all files, preserving subdirectory structure (`references/`, `scripts/`).
Files under `scripts/` receive `0755` permissions after writing (equivalent to
`chmod +x`). All other files receive `0644`.

#### Behavior

**Default — project-local:**
```bash
arm install-skills
```
Deploys to `.claude/skills/` and `.gemini/skills/` in the current working
directory. Creates subdirectories as needed. Always overwrites.

**Global flag:**
```bash
arm install-skills --global
```
Deploys to `~/.claude/skills/` and `~/.gemini/skills/` instead.

**Output:**
```
Deployed 5 skills to .claude/skills/
Deployed 5 skills to .gemini/skills/
```

#### Tests (TDD required per CLAUDE.md)

Write failing tests before implementation. At minimum cover:
- Deploy to a temp directory; verify `SKILL.md` contents match embedded source
- Verify `references/` subdirectory files are deployed correctly
- Verify `scripts/` files receive `0755` permissions
- Verify `--global` deploys to `~/.claude/skills/` and `~/.gemini/skills/`
- Verify idempotent: re-running overwrites without error

#### `AGENTS.md`

`AGENTS.md` exists at the repo root but is currently empty. A task in this story
populates it with setup instructions for users of armature on their own repos
(replacing `make install` references in skill bodies):

```markdown
## Setup

1. Install arm (see releases)
2. Run `arm install-skills` once to deploy agent skills
3. Run `arm worker-init` before claiming your first task
```

Skill bodies reference "arm must be on your PATH" without installation steps.
The full setup sequence lives in `AGENTS.md`.

### 5. `armature` Quick Reference Card

The `armature` skill is retained as a pure command syntax reference. All
workflow prose, prerequisites, rate limit tables, and extended explanations are
removed. Target: ~50 lines.

**New description:**
```yaml
description: >
  Quick reference for arm command syntax and flags. Use when you know
  your role and need the right command — for role-specific workflows,
  use armature-planner, armature-coordinator, armature-worker, or
  armature-auditor instead.
```

**Body structure:** grouped command blocks only — one line per command with key
flags. No paragraphs, no callouts, no "when to use" guidance in the body.

### 6. Description Cleanup (All Skills)

All five skill descriptions are trimmed to triggering conditions only — no
workflow summaries, no command lists. The `compatibility` field loses the
`make install` instruction. The "a armature" typo is fixed to "an armature"
across all skills.

| Skill | Change |
|-------|--------|
| `armature-coordinator` | Remove "finds unblocked tasks, assembles context, dispatches worker agents..." summary |
| `armature-planner` | Remove "covers decompose-apply, dag-transition, source registration..." detail |
| `armature-worker` | Remove "picks up ready issues, claims them, assembles context..." workflow detail |
| `armature-auditor` | Remove "Runs validate, sources verify, render-context, and doctor --strict" command list |
| `armature` | Fix "a armature-managed repo" → "an armature-managed repo" |
| All | `compatibility`: remove `(run make install)` |
| All | Fix "a armature-managed" → "an armature-managed" where present |

### 7. Progressive Disclosure Structure

Skills are split into a core body and `references/` files. Agents load the core
on skill activation and reference files only when their current task requires it.
Each reference file includes explicit trigger language in the core body.

**Skill size summary (actual line counts):**

| Skill | Core before | Core after (approx.) | References |
|-------|-------------|----------------------|------------|
| armature-coordinator | 435 lines | ~255 lines | ~180 lines across 2 files |
| armature-worker | 200 lines | ~140 lines | ~60 lines across 2 files |
| armature-planner | 444 lines | ~265 lines | ~180 lines across 2 files |
| armature-auditor | 233 lines | ~150 lines | ~80 lines in 1 file |
| armature (ref card) | 175 lines | ~50 lines | — |

All core skills land under the 500-line spec limit. "Core after" figures are
approximate; the implementing worker derives exact splits from the source.

#### `armature-coordinator`

**`references/parallel-dispatch.md`** — "Parallel Dispatch" section + "Log Slots
for Parallel Dispatch" section.

**`references/commands.md`** — "Querying JSON Output" + "Command Reference"
sections.

**Trigger language in core:**
```
If the story has tasks with no blocking dependencies between them,
read `references/parallel-dispatch.md` before dispatching.

For a full command reference, see `references/commands.md`.
```

**Core retains:** loop flowchart, survey + branch creation, find ready work,
sequential dispatch, after workers return, story completion, common failure modes.

#### `armature-worker`

**`references/dual-branch.md`** — all dual-branch mode callouts consolidated
from across the skill body.

**`references/batch-strategy.md`** — "Batch Strategy (Advanced)" section.

**Trigger language in core:**
```
If `git config --local armature.mode` returns `dual-branch`,
read `references/dual-branch.md` before any git add or commit.

If your task involves 10 or more files,
read `references/batch-strategy.md`.
```

**Core retains:** flow flowchart, prerequisites, full step-by-step with trigger
pointers replacing dual-branch callouts, log slot section, valid transition
targets, common mistakes table (dual-branch rows replaced by trigger pointer).

#### `armature-planner`

**`references/decompose-apply.md`** — full "Decompose-Apply Workflow" walkthrough:
schema inspection, writing plan.json, dry-run, apply, promote.

**`references/dependency-management.md`** — "Dependency Management" deep-dive:
when to use `arm link`, checking and resolving scope overlaps.

**Trigger language in core:**
```
For multi-task stories, read `references/decompose-apply.md`
for the full decompose-apply workflow.

If `arm validate` reports scope overlap WARNINGs,
read `references/dependency-management.md`.
```

**Core retains:** planner loop flowchart, prerequisites, step-by-step summary,
"Writing Good Plan JSON" section (always needed), source registration paths A
and B, release checklist, common failure modes.

#### `armature-auditor`

**`references/citation-errors.md`** — "Citation Integrity (E7 + E8)" full
remediation guide + extended "Source Freshness" workflow.

**Trigger language in core:**
```
If `arm validate` reports ERROR lines,
read `references/citation-errors.md`.
```

**Core retains:** when to run, all five checklist steps (commands + expected
output, no remediation prose), D6/E8 caveat note, pre-merge gate table, common
failure modes.

---

## Skill Freshness and Deployment

Claude Code loads skill metadata at session start and skill bodies once on first
invocation — mid-session disk changes are invisible to a running session.
Subagents always start fresh.

**Implication for workers:** every skill-modification task must run
`make deploy-skills` as its final step before `arm transition --to done`. This
ensures the next wave's subagent workers start with the updated skills on disk.

The coordinator verifies deployed skill files match expected content after each
wave before dispatching the next.

---

## Execution Order (Dogfooding Story)

The implementation is structured as an armature story so the skills are exercised
live as they are improved. Worker improvements land in wave 3 so wave 4 tasks
exercise the new worker skill.

| Wave | Tasks | Parallelizable | Dogfooding value |
|------|-------|---------------|-----------------|
| 1 | (A) Frontmatter merge + Makefile + skills dir move; (B) `arm install-skills` Go code | Yes — A touches SKILL.md/Makefile, B touches Go source | Baseline worker and planner flow test |
| 2 | (A) Trim `armature` ref card + description cleanup (all 5: `armature`, `armature-coordinator`, `armature-planner`, `armature-worker`, `armature-auditor`); (B) `AGENTS.md` content; (C) `armature-auditor` references | Yes | Auditor improvement live at story close |
| 3 | `armature-worker` references (`dual-branch`, `batch-strategy`) | No (unlocks wave 4) | Wave 4 workers start fresh with improved worker skill |
| 4 | (A) `armature-planner` references; (B) `armature-coordinator` references | Yes | Both workers exercise the wave-3 worker improvement |

Note: the coordinator skill improvement (wave 4) becomes available for
*subsequent stories*, not for wave 4 dispatch itself — the coordinator running
this story uses the pre-wave-4 coordinator skill throughout.

Each task's acceptance criteria includes `make deploy-skills` as a required
completion step before `arm transition --to done`.
