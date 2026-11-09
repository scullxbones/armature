---
date: 2026-11-09
agent: cursor
area: integration
task: "https://github.com/scullxbones/armature/issues/435"
tags: [hostref, mcp, bootstrap, dogfood]
---

# Host-ref / list --external-ref require bootstrap before worker-init alone

## User Goal

Dogfood the landed GitHub host-ref + `arm mcp` path while fixing #435: bind the GitHub issue as an external-ref and exercise `arm ref` / `ref-read` / `list --external-ref`.

## Observed

After `make install` and `arm worker-init`, `arm doctor` and `arm list` still fail with `DOCTOR-1` / `LIST-1`: `armature.ops-worktree-path must be set` and next_actions naming `arm bootstrap`. There is no `.armature/` in this clone yet. `arm ref` / `arm mcp` help render, but the host-ref workflow cannot run until bootstrap creates the ops worktree config.

## Impact

The dogfood brief's step order (`worker-init --check` then bind external-ref) skips an explicit bootstrap gate. An agent that treats worker-init as sufficient stalls on every host-ref command until it discovers bootstrap independently.

## Evidence

- `arm worker-init` → Worker ID `9800d4d2-d529-4e0c-b31c-d7592fccacc1`
- `arm doctor` → `DOCTOR-1` / `armature.ops-worktree-path must be set`
- `arm list --format agent` → `LIST-1` / same cause
- Brief: `/cursor/stores/self/internal/dogfood-hostref-mcp-435-437.md`

## Suggested Follow-Up

Make the dogfood brief (and maybe `arm ref` next_actions) say `arm bootstrap` before host-ref commands when ops-worktree-path is unset.
