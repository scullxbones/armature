# Armature — Agent Setup

This file is the shallow entrypoint for agents working in this repository.
Canonical workflow and command truth live in the embedded skills under
`internal/skillsembed/skills/` and the docs under `docs/`.

## Bootstrap

1. **Install `arm`** from the repo root:
   ```bash
   make install
   ```

2. **Bootstrap the repository and deploy bundled skills**:
   ```bash
   arm bootstrap
   ```
   This initializes Armature and deploys the bundled skills to local harness directories
   such as `.claude/skills/`, `.gemini/skills/`, and `.codex/skills/`.

3. **Register a worker identity once per clone**:
   ```bash
   arm worker-init --check || arm worker-init
   ```
   Do not re-run `arm worker-init` without `--check` unless you intentionally
   want a new worker UUID.

## Current Operating Model

- Armature coordinates work; it does **not** execute or supervise external
  harnesses.
- The normal loop is: `arm ready` -> `arm claim` -> `arm render-context` ->
  launch the worker outside Armature -> `arm transition`.
- `arm harness-hook` is an integration surface for harness-native guardrails,
  not a queue runner.
- Before closing out work, run:
  ```bash
  arm validate --ci
  arm doctor
  ```

## Capture Dogfood Findings

Armature is used to build Armature — treat every task as a live dogfood run. Use the `capturing-dogfood-findings` skill whenever you encounter friction:

- an `arm` command fails, returns confusing output, or contradicts the docs
- a skill does not fire as expected, or fires with wrong content
- a workflow step requires undocumented knowledge
- `make check`, `arm validate`, or `arm doctor` behaves unexpectedly
- a doc or error message is misleading or absent

Invoke the skill, write one raw finding under `docs/dogfood/findings/raw/` from the agent-user perspective, then continue.

Writer identity: `arm worker-init --check` (UUID) + `ARM_LOG_SLOT` when set. Local areas: `bootstrap` | `hooks` | `skills` | `commands` | `workflow` | `validation` | `coordination` | `tooling` | `documentation` | `other`.

## Use The Repo-Local Skills

Invoke the bundled skill that matches your role:

- `armature` — quick command reference
- `armature-worker` — execute a claimed task
- `armature-coordinator` — dispatch and integrate task waves
- `armature-planner` — decompose source-backed work into issues
- `armature-auditor` — verify completed work before sign-off

## Canonical References

- `docs/commands.md` — current CLI command surface
- `docs/harness-hook.md` — hook integration details
- `docs/design/architecture.md` — system architecture
- `CONTEXT.md` - domain glossary
