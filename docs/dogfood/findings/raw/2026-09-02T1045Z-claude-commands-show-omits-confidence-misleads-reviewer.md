---
date: 2026-09-02
agent: claude
writer: claude
area: commands
task: PR #127 review remediation
tags: [arm-show, provenance, confidence, agent-facing-views, reviewer]
---

# `arm show` omitting `provenance.confidence` led a reviewing agent to demand an assertion on a field that does not exist

## User Goal

Address a review finding asking the `verify-armature` skill to prove that
a freshly created task is `draft` in **materialized** state, not just in
the op stream.

## Observed

The reviewer's instruction was specific and confident:

> this structured `show` predicate still omits `.provenance.confidence`
> … require `.provenance.confidence == "draft"` here as well.

That field is not in `arm show`'s output. `arm show --format json` on a
created task emits exactly:

```
acceptance, definition_of_done, id, scope, status, title, type
```

(plus `claimed_by` once claimed). `rg 'provenance|confidence'
cmd/armature/show.go` returns nothing.

The field does exist on the model — `internal/materialize/state.go:100`
`Provenance`, `:138` `Confidence` — and `confidenceOrDefault` in
`internal/materialize/engine.go` treats an absent confidence as
`verified`. The reviewer had read the Go model and assumed the CLI
surfaced it.

## Impact

This is the known `show`-omits-provenance hole (see sibling below) showing
up in a new place: it did not just hide state from *me*, it caused a
second agent to specify an impossible fix. Following the instruction
literally would have produced an assertion that silently passes forever —
`jq '.provenance.confidence == "draft"'` on an object with no
`provenance` key is `false`, so the drive would have failed; and had I
written it as a `// empty` guard it would have passed vacuously. Either
way the "proof" would be worthless.

Cost: a round of source reading to establish that the requested field is
not emitted, plus designing an indirect proxy, plus writing up why the
literal instruction could not be followed. Any agent-to-agent review loop
over armature state pays this tax whenever the internal model and the
agent-facing view disagree.

## Evidence

- Reviewer comment id `3912402679` on PR #127.
- `jq -S 'keys'` on captured `arm show --format json` evidence → the seven
  keys above.
- `rg -n 'provenance|Provenance|confidence' cmd/armature/show.go` → no
  matches.
- `internal/materialize/state.go:100,138`;
  `internal/materialize/engine.go` `confidenceOrDefault`.
- Workaround shipped in `34aeea5d`: assert the consequence instead — a
  draft issue must be **absent from `arm ready`** (documented in
  `features/ready-claim.md`: fresh create is draft, so `ready` prints
  `null`). Captured evidence `drive/04b-ready/stdout.txt` → `null`.
- Sibling, same hole, different consumer:
  `2026-08-17T0130Z-5207ee28-commands-show-json-omits-provenance.md`
  and `2026-08-17T0236Z-5207ee28-commands-show-json-omits-pr-and-branch.md`.

## Suggested Follow-Up

Adds a third consumer to the existing case for putting `provenance`
(confidence, source_links, context_files) into `IssueJSON` and the human
`show` template. The new argument here is that the gap misleads *other
agents reading the Go source*, not only agents reading the CLI — the
model is the de-facto spec, so anything it holds and `show` drops becomes
a source of confidently-wrong instructions.
