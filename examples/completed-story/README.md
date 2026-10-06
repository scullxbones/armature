# Completed story artifact trail: `TOPTIER-S7-T1`

Real delivery from this repository: rewrite the README quickstart and run it in
CI. Merged as [PR #308](https://github.com/scullxbones/armature/pull/308)
(`3e815b3e09edbe58ed5b407502999e13d26e5c94`).

This directory is a **read-only museum**. You do not need to run Armature to
follow the trail. Parent story: `TOPTIER-S7` (front-door documentation).

## How each artifact was produced

| File | Produced by | Role |
| --- | --- | --- |
| [`registered-source.json`](registered-source.json) | `arm sources add` / sync against `docs/design/top-tier-gap-analysis.md` | Gap D1/D3 source the task cites (`34bc4866-…`) |
| [`plan.json`](plan.json) | Reconstructed from live create/amend ops + materialized issues for the examples trail | Shape of `arm dag apply --plan` input (story + task, both cited) |
| [`ops-excerpts.jsonl`](ops-excerpts.jsonl) | Worker ops log lines targeting `TOPTIER-S7-T1` | create → source-link → amend → claim → transition done → merged |
| [`render-context.json`](render-context.json) | `arm render-context TOPTIER-S7-T1 --format agent` | What a worker receives as the task spec |
| [`issue.json`](issue.json) / [`parent-story.json`](parent-story.json) | Materialized state under `.armature/state/` | Post-merge issue records |
| [`conformance-assessment.json`](conformance-assessment.json) | Reconstructed against PR #308 (no stored assessment under `.armature/review/` for this id) | Contract check: DoD + acceptance + scope |

## Paved-road sequence this trail corresponds to

```text
sources add/sync  →  cite (source-link)  →  dag transition (draft → verified)
  →  ready  →  claim --worktree  →  render-context  →  implement
  →  transition done  →  merge on main  →  merged
```

Birth was `draft` (see the create op). `ready` stayed empty until cite +
`dag transition`. That is the same footgun called out in
[`docs/why-armature.md`](../../docs/why-armature.md).

## PR

https://github.com/scullxbones/armature/pull/308
