---
date: 2026-11-09
agent: cursor
area: integration
task: "https://github.com/scullxbones/armature/issues/435"
tags: [hostref, ref-read, github-token, dogfood]
---

# arm ref-read fails closed without GITHUB_TOKEN after successful arm ref

## User Goal

Bind `https://github.com/scullxbones/armature/issues/435` on a local Armature bug and read the live host text via `arm ref-read`.

## Observed

`arm ref bug-1794190919 <url>` and `arm list --external-ref <url>` succeeded. `arm ref-read bug-1794190919 --format agent` returned `REF-READ-1` / `GITHUB_TOKEN is missing` with next_actions only `arm doctor`. Binding a host URL does not surface that live body fetch needs a token until ref-read.

## Impact

Host-ref dogfood can record the URL and filter by it, but cannot exercise the read path in a Cloud Agent without a GITHUB_TOKEN secret. The brief does not call out the token prerequisite for ref-read.

## Evidence

- `arm ref` → `{"issue":"bug-1794190919","url":"https://github.com/scullxbones/armature/issues/435"}`
- `arm list --external-ref …` → count 1, id `bug-1794190919`
- `arm ref-read` → `REF-READ-1` / `GITHUB_TOKEN is missing`

## Suggested Follow-Up

Document GITHUB_TOKEN (or gh auth) as required for `arm ref-read` in host-ref dogfood briefs; optionally have `arm ref` warn when token is absent.
