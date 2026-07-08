# ADR: Parked Surfaces Are Deleted Outright, Not Soft-Deprecated

## Status

Accepted

## Context

`docs/design/the-next-ten.html`, item №02 ("The Subtractive Release"), proposes an evidence-based census of every user-facing surface — issue types, statuses, confidence states, fields, commands, flags — ahead of `v0.1.0`, ruling each as kept or cut. The proposal's own language is emphatic that a cut is "parked, not purged": the surface leaves the product but stays in git history with a re-entry criterion. That language leaves open *how* a park is mechanically implemented, and the obvious options carry an ongoing-cost trade-off:

- **Soft removal with a redirect** — the command/type is deleted from normal operation, but invoking it prints an error naming the census entry and re-entry criterion.
- **Feature-flagged dormancy** — the code stays in the binary behind a hidden flag, fully functional but undocumented.
- **Literal deletion** — the code is removed outright; the only trace is the census row and the removing commit in git history.

The first two options exist specifically to soften the blow for a user who relies on a parked surface. But this project's own thesis for doing the census at all is that unused surface has a permanent, compounding cost — tests, docs, schema surface, and agent context, paid "forever, for everyone" (the census document's own red-team answer to "you'll cut something my team needs in month two"). A redirect shim or dormant flag keeps paying exactly that tax, just in a smaller, hidden form; it does not eliminate it. Choosing either would contradict the reason this census exists.

## Decision

A parked surface's code is **deleted outright** in the same commit that records the park. Nothing in the running binary references it. No redirect, no error shim naming the census entry, no feature-flagged dormancy. The **only** record of a park is:

1. The census row (`docs/design/subtractive-release.md`'s census table), carrying the surface's ruling and re-entry criterion.
2. The removing commit, discoverable via `git log`/`git blame` on the deleted code's former location.

**Resuscitation** — re-implementing a parked surface once its re-entry criterion is met — is a fresh implementation task, not a revert. It is expected to be re-derived from the census row's criterion and current requirements, not mechanically restored from history.

This makes **park** and **purge** behaviorally identical at runtime; the distinction is entirely about documentation and intent, not mechanism. A park has a census row, a re-entry criterion, and is the outcome of a deliberate ruling. A purge — code removed with no census row and no re-entry criterion — is reserved for surfaceless dead code and is not an outcome this census expects to produce.

## Consequences

A user who depends on a parked surface gets a plain "unknown command" / validation error, not a pointed explanation — the census document, not the running binary, is the source of "why did this go away and what would bring it back." This is a deliberate cost shift: the person restoring a parked verb pays a resuscitation task; everyone else stops paying the surface's maintenance tax immediately, rather than paying a smaller hidden version of it indefinitely. Anyone implementing the census's PR series (`docs/design/subtractive-release.md`) must not introduce redirect shims or hidden flags for parked surfaces — a park PR that leaves runtime traces does not satisfy this ADR.
