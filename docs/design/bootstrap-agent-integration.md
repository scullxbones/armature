# Bootstrap And Agent Integration

## Status

Draft from design discussion on 2026-06-15.

## Context

Armature's installation and first-run path has accumulated separate setup steps:
repository initialization, git hook installation, worker identity, bundled skill
deployment, and harness-hook configuration. The intended user is often an agent,
so missing setup steps quickly become product friction.

## Current Direction

`arm init` is the canonical repository bootstrap surface.

The happy path should prepare a local clone for Armature workflow without asking
the user to discover separate setup commands. Opt-out flags are preferred over a
second primary bootstrap command.

## Settled Decisions

- `arm init` owns repository bootstrap.
- `arm init` installs local bundled skills by default.
- `arm init --no-skills` skips local skill deployment.
- `arm install-skills` remains available as an idempotent repair/re-run command.
- Global skill installation remains explicit and is never part of `arm init`.
- Harness-hook installation is explicit until live provider behavior is proven.
- Harness-hook installation should be provider-scoped, for example
  `--harness-platform claude` or `--harness-platform codex`.
- The default `arm init` path should not silently write harness config before
  dogfood evidence proves that provider's current hook behavior.
- `arm init` should not run `arm validate --ci` or `arm doctor` automatically;
  it should print them as explicit next steps in its bootstrap summary.
- Dogfood observations are repository-maintenance artifacts, not Armature
  product features.
- The final bootstrap boundary should be recorded as an ADR after the decision
  set is complete.

## File Ownership Policy

Generated skill directories and Armature-owned plugin metadata may be
idempotently overwritten by `arm init` or `arm install-skills`.

Harness provider configuration must be conservative:

- create missing provider config when explicitly requested
- merge only when the adapter can preserve existing settings safely
- if preexisting config prevents installation, leave it untouched and print a
  clear message that the harness hook was not installed, why it was not
  installed, and what the user can do next

## Git Hook TODO

The current git hook installer writes directly to `.git/hooks/`. Revisit this
before expanding bootstrap automation:

- detect preexisting hooks and hook managers
- avoid silently overwriting user-managed hook files
- consider whether Armature should install through or generate snippets for a
  hook manager
- clearly report when git hook installation is skipped or requires manual
  integration

## Skill And Plugin Follow-Ups

Skill directory support and plugin integration are separate concerns.

`arm install-skills` should support local skill-directory installation for:

- Claude
- Codex
- Gemini
- Devin

Claude plugin metadata may be installed where the existing Claude plugin
registry contract is known. Antigravity and Codex plugin support should be
tracked as discovery work: create plugins only if those platforms expose a real
plugin contract Armature can support honestly.

## Harness-Hook Follow-Up

Harness-hook automation depends on the truth-recovery work in
`docs/design/harness-hooks-review-and-retooling.md`.

The intended harness set should be named once and reused by code and docs:

- Claude
- Codex
- Antigravity
- Devin

Each harness needs an explicit support matrix rather than a single yes/no
label, because skill installation, plugin integration, and harness-hook
automation may reach support at different times.

Antigravity should appear in the matrix as an intended harness, but Armature
must not install Antigravity skills or plugins until its skill/plugin contracts
are researched. TODO: verify Antigravity's current skill directory, plugin
format, hook support, and whether project-local installation is possible.

Before a provider becomes default-on in `arm init`, Armature needs current
dogfood evidence for:

- actual hook event payloads
- launch posture and environment behavior
- whether `arm harness-hook` decisions are visible and blocking as documented
- any unsupported or advisory-only behaviors
