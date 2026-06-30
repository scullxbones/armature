# Documentation Clarity Improvements — Spec

**Date:** 2026-06-06  
**Scope:** README.md, docs/, internal/skillsembed/skills/armature-planner/

## Context

Armature's documentation has correctness bugs that will cause agents to fail on first encounter, and gaps that require agents to read source code to self-diagnose errors. The value proposition text is for human decision-makers; all operational content (quickstarts, guides, command references) is consumed by agents.

## Correctness Bugs Found

**B1** `README.md` Step 2: `arm sources add docs/armature-prd.md` — positional arg not accepted by the binary; requires `--url` and `--type` flags.

**B2** `README.md` Step 3: `arm decompose-context --sources src-001` — sources are assigned UUIDs, not user-chosen IDs; `--sources all` is the correct form.

**B3** `README.md` missing `arm worker-init` step — required once per clone to register worker identity in local git config.

**B4** `README.md` missing `arm install-skills` step — required to deploy bundled agent skills to `.claude/skills/`, `.gemini/skills/`, `.codex/skills/`.

**B5** `README.md` Steps 4 and 5 both show `arm transition` with different syntax — creates ambiguity. Remove redundancy; keep one consistent `--to done` form.

**B6** `docs/use-cases.md` P4 Wrangler section documents `arm init --repair` — this flag does not exist in the binary.

**B7** `docs/use-cases.md` P4 config.json schema is entirely wrong. Documented: `ttl_seconds`, `stale_threshold_seconds`, `verification_commands`, `hooks` as string-keyed object. Actual schema:
- `mode`: "single-branch"|"dual-branch"
- `project_type`: auto-detected (go/node/python/rust/make/unknown)
- `default_ttl`: integer, **minutes** (not seconds), default 60
- `token_budget`: integer, default 1600
- `low_stakes_push_threshold`: integer ops before auto-push, default 5
- `hooks`: JSON array of `{name, command: string[], required: bool}` objects

**B8** `internal/skillsembed/skills/armature-planner/SKILL.md` Planner Loop digraph: node labeled `decompose-apply --apply` — `--apply` flag does not exist. Default behavior (no `--dry-run`) applies the plan.

**B9** `docs/getting-started.md` missing `arm worker-init` step.

**B10** `docs/getting-started.md` missing `arm install-skills` step.

## Documentation Gaps Found

**G1–G3** Validation codes not documented anywhere in operator-facing docs:
- Errors: E2/E3 (unresolved parent/link), E4 (cycle), E5 (invalid hierarchy), E6 (missing required field), E7/E8/E12 (citation issues), E9 (DoD >500 chars), E10 (invalid glob)
- Warnings: W1 (scope overlap), W2 (no test criteria), W3 (budget exceeded), W4 (broad scope), W5 (missing context_files with broad scope), W6 (complexity mismatch), W7 (vague DoD), W8 (conflicting decisions), W10 (phantom scope), W11 (vague outcome)
- Doctor: D1 (git divergence), D2 (stale claims), D3 (orphaned ops), D4 (broken parent refs), D5 (dependency cycles), D6 (uncited issues)

**G4** `arm validate --quiet` flag exists but missing from `docs/commands.md`.

**G5** `arm doctor --verbose` flag exists but missing from `docs/commands.md`.

**G6** `ARM_LOG_SLOT` env var: sets log slot suffix (`workerID~slot`), creates separate log file per parallel agent. Mentioned once in use-cases.md without explanation. Needs a reference entry.

**G7** No accurate `.armature/config.json` reference document.

**G8** No harness-hook integration guide. Manual configuration required: agents must write platform config files themselves. Claude Code: `.claude/settings.json` hooks block. Codex: `codex.toml` [hooks] section. Env vars required at runtime: `ARMATURE_TASK_ID`, `ARMATURE_HOOK_PLATFORM` (claude|codex|devin).

**G9** No concepts reference for agents. Key concepts needed: ops log/materialization, worker identity, claim lifecycle, DAG hierarchy, citations/validation, confidence levels (draft→verified), single vs dual-branch, source documents and decomposition.

## Approach

- Fix bugs first (Story 1, critical priority)
- Add missing reference docs (Story 2, high priority)
- Add integration and concepts guides (Story 3, medium priority)
- All new/updated docs are agent-facing: use operational rules and exact commands, not narrative explanations
