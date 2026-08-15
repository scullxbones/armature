---
area: tooling
writer: 5207ee28
date: 2026-08-15T13:15Z
story: LNGHZN-S10
task: LNGHZN-S10-T3
---

# WOZCODE free-plan cap blocked code Search mid-prfix

## What I was trying to do

Classify PR #106 P1s. After the `code` MCP connected I used `code__Search`
to find the dogfood raw-finding template and related paths.

## What happened

The tool returned:

```
Failed to call Search: WOZCODE free plan reached. Run /woz login to upgrade.
Resets on: 2026-09-01T00:00:00+00:00
```

I fell back to workspace Grep/Read. Classification continued, but the
session lost the combined search-and-read path the MCP is meant to provide.

## How it changed behavior, confidence, or time spent

A few extra Grep/Read round-trips. Not a correctness risk. It did burn
attention on tooling recovery in the middle of review-comment classification,
which is the expensive part of `/prfix`.

## Evidence

- MCP `code` server reported ready with `code__Search` / `code__Edit` /
  `code__Sql`
- First `code__Search` call in this session hit the free-plan cap
- Reset date reported as 2026-09-01T00:00:00+00:00
