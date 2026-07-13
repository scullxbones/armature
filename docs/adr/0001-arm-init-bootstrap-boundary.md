# ADR: `arm init` Owns Repository Bootstrap

## Status

Accepted

## Principles touched

I1, I4

## Context

Armature's first-run path had drifted into several separate setup actions: repository initialization, worker identity, git hook installation, bundled skill deployment, and harness-hook configuration. The primary user is often an agent, so missing or manual setup steps create product friction quickly and can make integrations appear active when they are not.

## Decision

`arm init` is the canonical repository bootstrap command. It should prepare the local clone for normal Armature workflow, including repository state, worker identity, safe git-hook setup, and local bundled skill installation by default. Focused commands such as `arm install-skills` remain available for repair or re-run, and global skill installation remains explicit.

Harness-hook installation is not default-on. It remains explicit and provider-scoped until live dogfood evidence proves the provider's current hook payloads, launch posture, environment behavior, and blocking semantics. If requested harness configuration cannot be safely created or merged because existing provider config is present, Armature must leave that config untouched and clearly report that the hook was not installed and what the user can do next.

## Consequences

The happy path becomes `arm init` rather than a sequence of setup commands an agent must discover. Generated skill directories and Armature-owned plugin metadata may be idempotently overwritten, but harness provider configuration and git hook integration need conservative ownership rules. `arm init` should report installed and skipped bootstrap pieces explicitly and print `arm validate --ci` and `arm doctor` as next steps rather than running them automatically.

The intended harness set is Claude, Codex, Antigravity, and Devin, but support must be represented as a matrix because skill installation, plugin integration, and harness-hook automation mature independently. Unknown platforms may be listed and skipped in default/all flows, but explicitly requesting an unsupported platform should fail with a clear message.
