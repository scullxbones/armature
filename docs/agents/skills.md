# Repo-Local Skills

Deployed via `arm bootstrap` or `make skill`, to `.claude/skills/`, `.gemini/skills/`, `.codex/skills/`.

Invoke the bundled skill that matches your role:

- `armature` — paved-road command reference only (see `docs/paved-road.md`)
- `armature-worker` — execute a claimed task
- `armature-coordinator` — dispatch and integrate task waves
- `armature-planner` — decompose source-backed work into issues
- `armature-auditor` — verify completed work before sign-off

Off-road commands in role skills are marked `[escape hatch]`. The base `armature` skill does not teach them.

`make validate-skills` enforces skill bodies don't reference `make install`.
