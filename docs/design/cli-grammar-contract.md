# The CLI Grammar Contract

**Date:** 2026-07-07
**Source:** `docs/design/the-next-ten.html`, item №05, "The CLI Grammar Contract" (Σ 25)
**Status:** Resolved via `/grill-with-docs`; ready for `/to-issues`.
**Sequencing:** Tier S, position 7 in `docs/design/next-work-sequencing.md` — after `NXTTN-S2` (Subtractive Release)'s cuts land. The 46-command audit runs against the *post-cut* surface, not today's.

## Thesis

Every rename after `v0.1.0` is a breaking change; every inconsistency kept is a permanent tax on agents.

## Relationship to Surface (`CONTEXT.md`) and the census

This item governs the same `command`/`flag` surface categories as `NXTTN-S2`'s census, from a different angle: the census decides what survives, this decides what surviving commands are *named* and *shaped like*. A census row's identity survives a rename — renaming updates the row's alias record, it does not reset the row's ruling or require fresh corpus evidence.

## Taxonomy rule: groups are discovered, not invented

See ADR 0011. A command joins (or forms) a group **if and only if** a corresponding deep module (an ADR-0004-listed package, or one being deliberately created alongside it) exists. The group's subcommands are that module's public interface, surfaced. A hyphenated command with no corresponding deep module stays a flat top-level verb; it does not get grouped just because a second verb appears on the same noun.

Deferred until after `NXTTN-S2`'s cuts land: the full 46-command audit table cross-referencing each command against the deep-module list (`ops`, `claim`, `traceability`, `materialize`, `sources`, `validate`, `output`, `dag`, `issuetype`). Early read from this session: `sources`/`validate`/`claim`/`materialize` groups are already aligned; `source-link` → `sources link`; `worker-init`/`workers` and `scope-delete`/`scope-rename` are candidate signals that either a module boundary needs drawing (`internal/worker` → deep-module status) or the commands stay flat (no `internal/scope` package exists — `scope` is an `Issue` field, not a module).

## Flag conventions

1. **Positional vs. flag for the primary target**: a command acting on exactly one issue takes a positional `[issue-id]`; `--issue` (repeatable) is reserved for commands that can take *multiple* issues (e.g. bulk `accept-citation --issue A --issue B`).
2. **`--format`** (`human`/`json`/`agent`) is a reserved, global-conventions flag; every structured-output command must support it, with `agent` as the harness-facing contract.
3. **No new single-letter flags** beyond the existing `-h`.

## Exit codes

`internal/exitcodes/` already defines a complete typed set (success/general/usage/not-found/conflict/io/invalid-state) — this item documents and audits compliance rather than inventing new codes. A drive-by fix landed during grilling: the package doc comment said "the trls CLI" (pre-rename leftover); a broader residual-`trls` sweep (3 more files, plus an orphaned `.claude/worktrees/` directory) is tracked separately as `bug-1783480206` under the `ARM` epic.

## TTY policy

`main.go` already auto-sets `--non-interactive` when `--format=agent`, non-TTY, or `$GEMINI_CLI`/`$TERM=dumb`. Rule: this is the **one** TTY-detection mechanism. Commands must read `--non-interactive`, not hand-roll their own `tui.IsTerminal()` check (`dagsum.go` and `stalereview.go` currently do — to be fixed as part of this item's audit). `accept_citation.go`'s `--ci` flag is a duplicate of `--non-interactive`; its removal is `NXTTN-S2`'s job (census cut), not this item's.

## Renames: no compatibility aliases

Renamed commands break immediately — no `Aliases`, no sunset window, no migration period. This follows the same reasoning as ADR 0010 (Subtractive Release): pre-`v0.1.0`, zero external adopters, this is the cheapest window this project will ever have to make a breaking change. The rename map (this item's own deliverable) is the sole record of old → new names.

## Conformance test

`cmd/armature/grammar_test.go` walks the full Cobra command tree from `root` (`Command.Commands()`, recursive) and asserts: zero hyphenated `Use` strings (no grandfather list — this item's own rename PRs are expected to clear all of them), single-issue commands use a positional arg not `--issue`, every structured-output command supports the `--format` enum, no command outside `main.go` calls `tui.IsTerminal()` directly. Runs in `make check`; a new command violating the grammar fails CI immediately. This is the same enforcement shape as `NXTTN-S2-T5`'s census drift check and GAP·T1.1's skill lint — three instances of the same "structural check at registration/build time" pattern.

## The plan produces

The grammar spec (this document); the 46-command audit table (deferred until post-census); the rename map (no aliases, direct breaks); `grammar_test.go` as the conformance gate.

## Glossary additions

Resolved and written to `CONTEXT.md` during grilling: `Surface` broadened to the general concept (no longer census-specific), `Deep Module` (promoted from ADR 0004 into the domain glossary), `Census` updated to note row-identity-survives-rename.
