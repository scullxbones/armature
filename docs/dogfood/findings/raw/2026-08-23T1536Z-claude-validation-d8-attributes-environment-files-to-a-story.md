---
date: 2026-08-23
agent: claude
area: validation
task: Close out LNGHZN-S10 and LNGHZN-S9
tags: [doctor, d8, scope, environment, sandbox]
---

# doctor D8 attributes environment-injected untracked files to an arbitrary in-progress story

## User Goal

Run the auditor's pre-merge gate (`arm doctor --strict` must exit zero) before closing
two stories.

## Observed

`arm doctor` exits 1 on a D8 **error**: out-of-scope artifacts attributed to
`LNGHZN-S7` — `.bash_profile`, `.bashrc`, `.zshrc`, `.profile`, `.gitconfig`,
`.gitmodules`, `.ripgreprc`, `.mcp.json`, `.claude/agents`, `.claude/commands`,
`.claude/hooks`, `.claude/launch.json`, `.claude/loop.md`, `.claude/output-styles`,
`.claude/routines`, `.claude/workflows`.

None of these are work product. They are untracked harness/sandbox environment files
that appear at the repo root in this execution context; the same check is expected not
to fire outside it. D8 nevertheless attributed all of them to whichever story happened
to be in progress.

## Impact

Blocks story closeout for *every* story, not just the one blamed — the auditor's five-check
gate requires `doctor --strict` to exit zero, and the auditor's DAG Hygiene Mandate says
warnings from other stories must be resolved rather than ignored. The failure is
environment-dependent, so it is not reproducible for another operator and cannot be fixed
by the story it names. An agent following the documented closeout literally would either
halt indefinitely or start deleting the operator's shell configuration.

## Evidence

- `arm doctor` → exit 1, D8 ERROR listing the files above against `LNGHZN-S7`
- `git status` → all listed paths are untracked (`??`)
- `.claude/skills/armature-auditor/SKILL.md` — five-check gate, `doctor --strict` exit zero
- `docs/agents/workflow.md`

## Suggested Follow-Up

D8 should ignore untracked files that no issue's scope claims, rather than attributing
them to the nearest in-progress story; or restrict attribution to tracked/staged changes.
Consider a distinct, non-blocking code for "untracked files present in the working tree"
so genuine scope violations stay separable from environment noise.
