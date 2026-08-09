---
area: workflow
writer: claude
date: 2026-08-09T15:00Z
pr: 89
story: LNGHZN-S5
---

# `arm reopen` records no rationale, in a system whose whole premise is provenance

## What the agent-user was trying to do

Reopen story `LNGHZN-S5` after splitting its PR review remediation into tracked
tasks. The story was `done`, but its PR was still open and it had just acquired
three unmerged children, so `done` was no longer true. Readiness confirms this:
`internal/ready/compute.go` requires a parent in `open`/`claimed`/`in-progress`,
so the new tasks could not become ready until the story reopened.

## What happened

`arm reopen` accepts only `--issue`. There is no `--reason`, `--rationale`, or
`--outcome`:

```
$ arm reopen LNGHZN-S5 --reason "..."
Error: unknown flag: --reason
```

The reopen op was appended carrying who and when, but not why. Capturing the
why took a second, unrelated command:

```
$ arm note --issue LNGHZN-S5 --msg "Reopened after PR #89 review remediation was split into..."
```

Nothing links the note to the reopen op. A later reader sees a status change and
a free-text note that happen to be adjacent in time.

## How it changed behavior / confidence / time spent

- Two ops for one intentional act, with the explanation detached from the
  decision it explains.
- `arm transition --to done` requires `--outcome` and enforces a delivery gate;
  reopening — which *undoes* a completion — asks for nothing. The asymmetry is
  surprising: reverting a terminal state is the change most likely to need an
  explanation later.
- CONTEXT.md defines **Provenance** as "the recorded chain of evidence and
  authorship explaining why an issue exists and how its current shape was
  justified", and I7 makes humans accountable for outcomes. A status reversal
  with no recorded why is a gap in exactly that chain.
- Low time cost (one extra command), but it depends on the agent choosing to
  write a note at all. Nothing prompts it, so the common case is that the reason
  is simply lost.

## What would have helped

A `--reason` on `arm reopen`, recorded in the reopen op's payload, so the
rationale travels with the transition instead of beside it.
