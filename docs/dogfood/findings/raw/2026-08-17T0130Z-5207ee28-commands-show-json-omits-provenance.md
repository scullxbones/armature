---
date: 2026-08-17
agent: 5207ee28
writer: 5207ee28
area: commands
task: LNGHZN-S10-T12
story: LNGHZN-S10
tags: [arm-show, json, confidence, source-links, context-files]
---

# `arm show --format json` reports null confidence, source_links, and context_files right after they were written

## User Goal

After `arm create --confidence draft --source … --context-file …` plus
`arm sources link`, confirm the issue actually carried draft confidence,
the source link, and the context files before promoting it.

## Observed

```
arm show LNGHZN-S10-T12 --format json | jq '{confidence: .provenance.confidence, source_links, context_files, blocked_by}'
```

returned `confidence: null`, `source_links: null`, `context_files: null`.
`blocked_by` was populated. Human `arm show` also omitted confidence.
`arm dag transition --issue LNGHZN-S10-T12` then succeeded, so the node
existed; I still could not see whether create had written draft or the
default verified.

## Impact

Lost a verification step. I could not tell create-with-`--source` from
create-then-forgot-to-link without reading the raw op log. Same class of
hole as `arm show --field dod` returning empty: the command looks like it
answered and the answer looks like "not set."

## Evidence

Create used `--confidence draft`, `--source b1b73229-…`, three
`--context-file` flags; `sources link` returned
`{"issue":"LNGHZN-S10-T12","source_id":"7675b1c9-…"}`. Immediate JSON
show still null on those three fields.

## Suggested Follow-Up

JSON show should emit the same materialized provenance, source_links, and
context_files the human view (or the snapshot) has. Null vs omitted vs
empty-array should not be indistinguishable from "the write never landed."
