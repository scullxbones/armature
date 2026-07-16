# ADR: CLI Command Groups Are Discovered From Deep Module Boundaries

## Status

Accepted

## Context

`docs/design/the-next-ten.html`, item №05 ("The CLI Grammar Contract"), catalogs a real inconsistency in Armature's 46-command surface: `sources` is a proper command group (`sources add/sync/verify`) but `source-link` sits beside it as a hyphenated orphan; `review` has subcommands but `stale-review` doesn't; `workers` is a group but `worker-init` isn't; `scope-delete`/`scope-rename` got hyphens where a comparable pattern elsewhere got subcommands. The obvious fix is a CLI-only naming convention — pick a taxonomy rule and apply it.

But this codebase already has a relevant, independently-motivated structural concept: ADR 0004 designates seven packages as **deep modules** (`ops`, `claim`, `traceability`, `materialize`, `sources`, `validate`, `output` — narrow public interfaces hiding substantial implementation), plus `dag` and `issuetype` as pure packages. Cross-referencing the inconsistent CLI evidence against this list surfaces a pattern that isn't coincidental: `sources`, `validate`, `claim`, and `materialize` are *already* both deep modules and command groups, aligned. The commands that don't fit an obvious group (`source-link`, `worker-init`, `scope-delete`/`scope-rename`) are exactly the ones with no clean deep-module counterpart (`source-link` is `sources`-adjacent and folds in cleanly; `worker-init` sits beside a `worker` package that ADR 0004 doesn't designate as deep; `scope` isn't a package at all — it's a field on `Issue`).

Inventing a CLI-only grammar rule (e.g., "any noun with 2+ verbs becomes a group") would treat this as a purely cosmetic naming problem and risks producing CLI groups with no matching code boundary — a taxonomy that looks tidy in `--help` but doesn't reflect how the system is actually built, and can drift from it silently afterward.

## Decision

A command becomes (or joins) a **group** if and only if a corresponding deep module already exists (an ADR-0004-listed package) or is being deliberately promoted to that status alongside the CLI change. The group's subcommands are that module's public interface, surfaced at the CLI layer. A command with no corresponding deep module stays a flat top-level verb — it is not grouped merely because a second verb appears on the same noun, and it does not get an invented hyphen-compound name either.

A hyphenated command with no corresponding deep module is treated as a **signal**, not just a style violation: either the underlying package boundary needs to be drawn deliberately (a `internal/` refactor, e.g. promoting `internal/worker` to deep-module status if `worker-init`/`workers` should unify into one group), or the command is correctly flat and stays a plain verb. The CLI Grammar Contract's 46-command audit (deferred until after `NXTTN-S2`'s cuts land) makes this determination per command, case by case — this ADR fixes the *rule*, not the audit's individual outcomes.

Renamed commands break immediately, with no compatibility alias — this is the same reasoning as ADR 0010: pre-`v0.1.0`, zero external adopters, cheapest breaking-change window this project will ever have.

## Consequences

Command taxonomy becomes a derived property of the package architecture rather than an independent CLI-layer decision — a future package that earns deep-module status (via an ADR 0004 amendment) is expected to also justify a CLI group review, and conversely a proposed new CLI group without a corresponding package boundary should be treated as premature. This couples two previously independent surfaces (Go package boundaries and CLI grammar) more tightly than before; anyone changing ADR 0004's deep-module list should consider whether the CLI surface needs to follow. `cmd/armature/grammar_test.go` (the Grammar Contract's conformance test) enforces the naming outcome of this rule but does not itself verify the deep-module correspondence — that check remains a human judgment made during the audit, not an automated gate.
