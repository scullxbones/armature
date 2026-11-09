---
date: 2026-11-09
agent: cursor
area: workflow
task: "https://github.com/scullxbones/armature/issues/435"
tags: [bootstrap, hostref, dogfood, dirty-tree]
---

# Bootstrap refuses dirty tree before host-ref dogfood

## User Goal

After installing `arm` and `worker-init`, bootstrap this clone so `arm ref` / `list --external-ref` can bind issue #435 while the fix is in progress.

## Observed

`arm bootstrap` exited with `repo_setup.status=error`: `working tree is dirty (contains uncommitted changes): please commit or stash your changes before running bootstrap`. Host-ref dogfood therefore cannot start until the #435 implementation is committed (or stashed), even though the dogfood brief treats bootstrap/host-ref as a side channel during the fix.

## Impact

Agents must either commit WIP early (possibly before `make check`) or stash mid-fix to exercise host-ref/MCP. That couples dogfood readiness to commit hygiene more tightly than the brief suggests.

## Evidence

- Command: `arm bootstrap`
- Error: working tree dirty; commit or stash before bootstrap
- Prior finding: `2026-11-09T0201Z-cursor-integration-hostref-needs-bootstrap.md`

## Suggested Follow-Up

Allow bootstrap against a dirty worktree when only binding host-ref tooling is needed, or document “commit or stash first” in the host-ref dogfood brief.
