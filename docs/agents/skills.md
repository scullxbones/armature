# Repo-Local Skills

`.agents/skills` is canonical; `.cursor/skills` entries are symlinks.

`arm bootstrap` and `make skill` / `make deploy-skills` install both the embedded bundle (`internal/skillsembed/skills`) and repo-canonical skills under `.agents/skills` (including `capturing-dogfood-findings`) to `.claude/skills/`, `.gemini/skills/`, and `.codex/skills/`. Bootstrap overlays `.agents/skills` from the target repo when that directory exists.

Invoke the bundled skill that matches your role:

- `armature` — quick command reference
- `armature-worker` — execute a claimed task
- `armature-coordinator` — dispatch and integrate task waves
- `armature-planner` — decompose source-backed work into issues
- `armature-auditor` — verify completed work before sign-off
- `capturing-dogfood-findings` — side-channel dogfood capture (canonical under `.agents/skills`)

`make validate-skills` lints `.agents/skills/**` and `internal/skillsembed/skills/**` and enforces skill bodies don't reference `make install`.
