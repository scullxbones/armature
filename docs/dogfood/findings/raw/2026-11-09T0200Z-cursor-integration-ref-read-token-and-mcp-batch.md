---
date: 2026-11-09
agent: cursor
area: integration
task: "437 dogfood host-ref + mcp"
tags: [arm-ref-read, GITHUB_TOKEN, arm-mcp]
---

# ref-read needs GITHUB_TOKEN; MCP batch tools/call drops trailing calls

## User Goal

Exercise `arm ref-read` and `arm mcp` tools/call for `list --external-ref` / `ref-read` against issue #437.

## Observed

1. With no `GITHUB_TOKEN` in the environment, `arm ref-read` failed `REF-READ-1: GITHUB_TOKEN is missing` even though `gh auth token` worked. Exporting `GITHUB_TOKEN=$(gh auth token)` made CLI `ref-read` succeed and return the issue title/body.
2. Feeding `arm mcp` multiple JSON-RPC `tools/call` messages on one stdin batch reliably answered `initialize` + the first `tools/call`, then exited before later calls (id 3 for `ref` / `ref-read` never appeared). Single-call sessions worked.

## Impact

Host-ref dogfood in agent environments that only have `gh` auth (not `GITHUB_TOKEN`) fails closed on `ref-read`. MCP clients that batch several `arm` tool calls on one stdio session silently lose trailing calls.

## Evidence

```text
arm ref-read task-1794189466 --format json
→ REF-READ-1 GITHUB_TOKEN is missing

GITHUB_TOKEN=$(gh auth token) arm ref-read ... --format json
→ {"title":"repoguard: unused-waiver ...","url":".../issues/437", ...}

printf 'initialize\ninitialized\ntools/call list\ntools/call ref-read\n' | arm mcp
→ responses for id 1 and id 2 only; id 3 absent; stderr empty
```

## Suggested Follow-Up

Accept `gh` auth as a token source for `ref-read`, and keep the MCP stdio session open across multiple `tools/call` messages until stdin EOF (or document one-call-per-session).
