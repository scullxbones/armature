---
date: 2026-09-01
agent: claude
area: cli
task: Determine which stale done tasks were gating open work across the DAG
tags: [arm-list, json, blocked-by, dag, agent-ergonomics, i4]
---

# `arm list --format json` omits `blocked_by`, so any gating question costs N+1 calls

## User Goal

Answer "which of these stale `done` tasks are actually blocking open work?"
across a 765-issue DAG, before deciding what to promote.

## Observed

`arm list --format json` emits `id`, `status`, `title`, `type` and `parent` per
issue. It does not emit `blocked_by`, which is the one field the question needs.
The dependency edges exist and `arm show --issue <ID> --format json` returns them
per issue — so answering a whole-DAG gating question requires one `arm list` plus
one `arm show` per candidate.

At 765 issues, with `arm show` taking roughly two seconds (it re-materializes
state from the ops log on every invocation), a full sweep is on the order of 25
minutes of serial CLI calls. A batch of 48 sequential transitions in this session
exceeded a two-minute command timeout and had to be split and resumed.

`arm ready --explain` does surface blocker information, but only for issues that
are *not ready* — it answers "why is this blocked" rather than "what does this
block", and it says nothing about issues in terminal states, which is exactly the
population being audited.

## Impact

I4 says agents are the primary users and defaults should optimize for agent
consumption. The JSON list format is the natural bulk-read surface for an agent,
and it is missing the field that makes the graph a graph. What remains is a flat
node list — enough for status counts, not enough for any reachability question.

The practical outcome is that the expensive path gets skipped. The prior audit of
this same backlog enumerated blockers for four Tier A items by hand and explicitly
recorded that it had not checked the rest, because a complete sweep needed a
per-issue `arm show`. So the DAG-wide question went unanswered for a week, and the
handoff that followed proposed acting on a subset for want of the data to justify
more.

An agent that cannot cheaply ask "what does this block" will systematically
under-scope graph work to whatever it can afford to look at.

## Evidence

- `arm list --format json | jq '.[0] | keys'` → `["id","parent","status","title","type"]`
- `arm show --issue NXTTN-S3-T1 --format json | jq '.blocked_by'` →
  `["TOPTIER-S3-T1","LNGHZN-S10-T4"]` — the field exists per issue
- 765 issues in the repo; `arm list` is one call, a complete blocker sweep is 766
- A 48-item `arm transition` loop exceeded the 2-minute timeout at 44 items,
  requiring a resume pass to finish the remaining 4
- Prior audit (2026-08-27) enumerated blockers for only `AOC-S1-T3`,
  `LNGHZN-S6-T5`, `LNGHZN-S7-T6`, `NXTTN-S3-T1`, recording that a full sweep was
  not attempted for this reason

## Suggested Follow-Up

Include `blocked_by` and `blocks` in `arm list --format json`. Both are already
materialized on the issue — `arm show` reads them from the same state — so this is
a projection change in the list command, not new computation.

If payload size is the concern, a `--fields` flag would serve better than a fixed
minimal projection: agents asking graph questions need edges, agents rendering a
status table do not, and only the caller knows which it is.

Worth checking whether the same projection gap affects other bulk surfaces. The
pattern to watch for is a command whose JSON is a strict subset of what `arm show`
returns for the same issue — every such gap forces the N+1.
