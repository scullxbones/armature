---
date: 2026-11-09
agent: cursor
area: tooling
task: "https://github.com/scullxbones/armature/issues/435"
tags: [mcp, stdio, notifications, dogfood]
---

# arm mcp NDJSON probe hung waiting for a response to notifications/initialized

## User Goal

Exercise `arm mcp` tool `arm` with `list --external-ref` for issue #435 after binding the host ref.

## Observed

`arm mcp` is newline-delimited JSON-RPC (confirmed in `internal/mcpsession`). A quick Python probe wrote one object per line and correctly received `initialize`. It then sent `notifications/initialized` and **blocked waiting for a response line**. Notifications have no `id` and get no reply, so the client deadlocked; a later write hit `BrokenPipeError` after the hung session was killed (~10 minutes). `arm mcp --help` does not show a minimal request sequence.

## Impact

Side-channel MCP dogfood stalled until the process was killed. Easy to misread as a framing (Content-Length) problem when the real footgun is treating notifications like requests.

## Evidence

- `initialize` returned `serverInfo.name=armature`
- Client `send()` always `readline()` after every write, including notifications
- Package comment: "newline-delimited MCP session over stdio"

## Suggested Follow-Up

Add a one-liner example to `arm mcp --help` / commands.md: initialize → (no wait on) notifications/initialized → tools/call; and note notifications must not wait for a response.
