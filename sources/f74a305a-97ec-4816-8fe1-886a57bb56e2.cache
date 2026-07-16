# Collapse the `.arm/.armature/` Double-Dotdir

**Date:** 2026-07-08
**Source:** `docs/design/long-horizon-proposals.md`, item D5, "Collapse the `.arm/.armature/` double-dotdir before v0.1.0"
**Status:** Resolved via grilling; ready for `/to-issues`.
**Sequencing:** Tier S, position 6 in `docs/design/next-work-sequencing.md`.

## Thesis

Coordination state lives at `.arm/.armature/ops/...` — an ops *worktree* named `.arm/` containing a state *directory* named `.armature/`, two near-identical names with different meanings. This is residue of the Trellis→Armature rename and the confusion engine behind the README drift `TOPTIER-S7` fixes the *description* of; this fixes the *thing described*. It is a breaking layout change, which is exactly why it must land before `v0.1.0` ships to real adopters rather than after.

## Target layout

The worktree is renamed to `.armature/` — the product's own name, not the `arm` binary's name — and its former contents (`ops/`, `config.json`, `hooks/`, `sources/`, `review/`, `templates/`) move up to live directly at its root. The inner `.armature/` subdirectory is dropped entirely. One visible name, one meaning.

## Per-worker materialized state folds in, not out

Today `stateDirFor` (`cmd/armature/helpers.go:120`) puts per-worker materialized state at the worktree root (`.arm/state/<workerID>`), deliberately *outside* the inner `.armature/` subtree the comment says. That split was a side effect of the dual-branch migration, not a documented design principle — nothing in `architecture.md §2` argues for keeping it separate, and both `state/` locations are already `.gitignore`d as derived, regenerable-from-ops-replay content.

Under the collapse, `ctx.WorktreePath` and `ctx.IssuesDir` become the same path (today `IssuesDir := filepath.Join(WorktreePath, ".armature")`; post-collapse they're equal), so `stateDirFor`'s existing branch (`filepath.Join(ctx.WorktreePath, "state", workerID)`) resolves to `.armature/state/<workerID>` **with no code change** — this is the same code path legacy single-branch mode already exercises (`WorktreePath == ""`, everything keyed off `IssuesDir`), not a new special case.

Verified low-risk against three other subsystems before committing to this: the TUI's fsnotify watcher (`internal/tui/app/model.go:100`) only watches `issuesDir/ops/`, not the whole tree, so folding `state/` in adds no watch overhead; `doctor` references specific subpaths directly rather than an enumerated "expected top-level dirs" list that folding-in would violate; and the fold actually *removes* two existing special cases — `internal/context/assemble.go:96`'s `base == ".arm" || base == ".armature"` check collapses to one name, and `internal/review/prepare.go:188`'s two exclude prefixes collapse to one.

## A named constant, introduced now

`.armature` (and `.arm`) are hardcoded string literals across `config/context.go`, `context/assemble.go`, `review/prepare.go`, `bootstrap.go`, and others. Since this migration touches nearly every one of those call sites already, a single `StateDirName` constant is introduced so a third dotdir-confusion episode can't recur via a missed literal — no broader abstraction, just the one constant.

## Precursor: the sources lifecycle must auto-commit first

While grilling migration safety, live inspection of this repo's own `.arm` worktree turned up a real, current bug: `arm sources add/sync/verify` (`cmd/armature/sources.go`) write `manifest.json` and `.cache` files directly with no git commit, unlike ops writes, which always auto-commit via `ops.AppendAndCommit`. `manifest.json`'s entries are cited by ops payloads (`internal/materialize/engine.go:350`'s `SourceEntryID`), so losing an uncommitted sources change isn't like losing derived `state/` — it can orphan permanent citations in the append-only ops log if a re-registration assigns fresh UUIDs.

This is filed as its own bug (`LNGHZN-B1`) and is a hard precursor to the collapse migration, not an in-scope task of it: fixing it means the migration's own dirty-worktree check (below) is a cheap safety net that should never actually fire, rather than a condition the migration has to carry a diff through.

## Migration mechanics

- **Home:** extends the existing `migrateLegacySingleBranchOps` pattern in `bootstrap.go` — same idempotent entry point, not `doctor --fix` or a standalone command.
- **Chaining:** `bootstrap` detects whichever of the three possible states a repo is in (original single-branch `.armature/`, today's dual-branch `.arm/.armature/`, or already-collapsed `.armature/`) and always converges to the collapsed layout in one pass; a repo on the original layout chains through the existing single→dual migration first rather than getting a separate direct-jump code path.
- **Git-level move:** remove-and-re-add via the existing `RemoveWorktree`/`AddWorktree` (`internal/adapters/git.go`), not a new `git worktree move` wrapper — same precedent as the prior migration.
- **Rollback safety:** rename the old `.arm/` aside to a timestamped backup (e.g. `.arm.collapsed-<timestamp>`) before restructuring, with rollback-on-failure restoring `.arm/` and re-registering it — matching `migrateLegacySingleBranchOps`'s existing discipline exactly.
- **Dirty-worktree precondition:** the migration refuses to run if the `_armature` worktree has uncommitted changes (`git status --porcelain`), given `LNGHZN-B1` above is a precondition rather than something this migration works around.
- **Git-exclude cleanup:** actively removes the stale `.arm/` line from `.git/info/exclude` (added by `bootstrap.go:830-863`) and adds `.armature/` in its place — no permanent debris left behind, the same principle motivating the whole proposal.

## No shim expiry — detect and refuse, don't silently migrate

Unlike LH D3's shim-retirement policy (timed sunset, then deletion), this migration's old-layout detection is not time-boxed: any non-`bootstrap` command that encounters the old `.arm/.armature/` layout errors immediately (*"repo uses the pre-collapse worktree layout; run `arm bootstrap` to migrate"*) rather than silently migrating as a side effect, and that detection code is not scheduled for deletion. The migration only ever runs when a user explicitly invokes `bootstrap`.

## Docs and skills sweep

A separate task within the same story (not a separate story): `AGENTS.md`, `docs/design/architecture.md §2`, the bundled skills, and `docs/agents/*.md` all name `.arm/.armature/` (or `.arm/`) explicitly today and need the rename applied, so the collapse isn't "done" while docs still describe the old layout — but this doesn't block the harder migration-correctness work from landing first.

## Release gate

A hard `blocked_by` edge from `TOPTIER-S6-T3` ("Cut v0.1.0") onto this story's completion — not just doc-level tier ordering in `next-work-sequencing.md`. This is exactly the case that document's own preamble says prose-only ordering can't express with confidence: a breaking layout change must not slip past a release by accident.

## The plan produces

This document (the resolved spec); `LNGHZN-B1` (sources auto-commit fix, precursor bug); `LNGHZN-S1` (the collapse story, decomposed into prefactor / migration / chaining / detect-and-refuse / docs-sweep tasks); a `blocked_by` edge gating `TOPTIER-S6-T3` on `LNGHZN-S1`.
