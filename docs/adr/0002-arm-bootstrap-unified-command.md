# ADR: `arm bootstrap` Owns Bootstrap And Reinstall

## Status

Accepted

## Context

Armature's setup surface had split across multiple commands and responsibilities:
repository initialization, worker identity, git hook installation, bundled skill
deployment, plugin metadata deployment, and harness-hook configuration.

The earlier bootstrap ADR established that setup should have one canonical happy
path, but the command surface still centered `arm init` plus a separate
`arm install-skills` repair path. The follow-up design review concluded that
this split was too narrow and would keep drifting from the real bootstrap
bundle, especially once plugins and harness-hook configuration were modeled
alongside skills.

The intended user is often an agent. For that user, bootstrap must be:

- discoverable through a single command
- idempotent
- explicit about supported versus unsupported platform contracts
- conservative about overwriting configuration Armature does not own

## Decision

`arm bootstrap` is the canonical bootstrap command for both first-run setup and
later repair or reinstall flows.

Armature will replace `arm init` and `arm install-skills` directly rather than
carrying a compatibility window. Documentation and embedded skills must be
updated as part of the command transition, especially the `armature` quick
reference.

`arm bootstrap` is a single user-facing command, but it may be factored
internally into two areas:

1. repo setup
2. harness setup

Repo setup covers repository state, worker identity, and git hooks.

Harness setup covers platform-scoped bootstrap artifacts:

- `skills`
- `plugin_metadata`
- `harness_hook_config`

Harness setup must be planned by a shared functional module that performs no
I/O and returns declarative per-platform decisions. That planner is the source
of truth for:

- the built-in default platform set
- verified-contract support by platform and artifact
- explicit-request validation
- per-artifact install or skip reasoning

The default `arm bootstrap` path uses the built-in default verified platform
set. If `--platform` is specified, bootstrap uses only the requested platforms.

Harness-hook configuration remains special in policy even though it shares the
same planning model. It is opt-in through `--with-hooks`; default bootstrap
installs skills and plugin metadata only. This preserves the earlier rule that
hook automation should not silently become default-on before hook truth recovery
and dogfood evidence are complete.

Bootstrap target is a first-class mode. A single invocation writes all selected
platforms to one target, either repository-local or user-home. Global target is
not a separate artifact type.

## Ownership Policy

Armature-owned artifacts may always be overwritten:

- `skills`
- `plugin_metadata`

Artifacts that may contain user-managed configuration must not be overwritten
unless Armature can prove it owns the specific content being replaced:

- `harness_hook_config`
- `git_hooks`

If bootstrap cannot safely install those artifacts, it must leave existing
content untouched and report the skip clearly.

## Failure And Reporting

Bootstrap should stay terse on success.

On failure, bootstrap should continue where it is safe to do so and then report
a grouped summary. The summary should be organized around the two internal
areas:

- `repo_setup`
- `harness_setup`

Machine-readable output should follow Armature's normal convention of
supporting `--json` and defaulting to JSON when stdout is not a user terminal.
The exact result schema can be elaborated during implementation planning.

## Consequences

The command surface becomes simpler for users: one idempotent bootstrap command
replaces separate init and bootstrap-repair commands.

Internally, Armature takes on a clearer modular boundary:

- repo setup remains command-orchestrated
- harness setup becomes planner-driven through a shared no-I/O module

Future implementation planning must still pin down:

- the exact ownership marker or detection strategy for hooks and harness config
- the concrete verified-contract threshold tests for each platform/artifact cell
- the final JSON result schema
