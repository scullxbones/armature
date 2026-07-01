---
date: 2026-07-01
agent: claude
area: tooling
task: SB-ELIM
tags: [regression, hooks, silent-failure]
---

# Removing a config field silently defanged a shell-script safety check

## User Goal

Verify SB-ELIM (single-branch mode elimination) was fully and safely complete
before opening a PR, per the story's own acceptance criteria (`make check`
green, `arm validate`/`arm doctor` clean, no repo-wide single-branch/
dual-branch references).

## Observed

`cmd/armature/bootstrap.go` writes two installed git hook shell script
templates (`postCommitHookTemplate`, `preCommitHookTemplate`). Both had a
`grep -q '"mode".*"dual-branch"' .armature/config.json` conditional gating
real behavior (pushing ops after commit; blocking `.armature/ops/` commits on
code branches). SB-T5 removed the `Mode` field from `config.Config` entirely,
so `config.json` never contains a `"mode"` key anymore — but nothing updated
these shell templates. The result: the grep always fails, so
`preCommitHookTemplate` silently takes the "single-branch mode allows ops/
commits" branch unconditionally, defeating the exact protection SB-T8 had just
added to the Go-side `hook.go` pre-commit logic. No test caught this because
the shell templates are string constants embedded in Go, not directly
exercised the same way as `hookIsDualBranch` was.

## Impact

This was a real, shippable regression — a silent safety-check bypass — that
survived 12 of 13 tasks, `go build`, `go test`, and even an initial `make
check` pass, because nothing in the DAG's tasks or tests exercised the
generated shell script content against the new (Mode-less) config.json shape.
It was only caught by a manual repo-wide grep for `dual-branch`/`single-branch`
during the final verification pass — i.e., by an explicit acceptance criterion
asking for exactly that grep.

## Evidence

- `cmd/armature/bootstrap.go` lines ~410, ~451-462 (pre-fix): both templates
  gated on `grep -q '"mode".*"dual-branch"' .armature/config.json`.
- `internal/config/config.go`: `Config` struct has no `Mode` field after
  SB-T5 (`git log` shows SB-T5 commit `65074d1b`).
- Fix commit on `feat/SB-ELIM`: "fix(SB-ELIM): remove dead mode-gating from
  hook templates and glossary" — made both templates unconditional.

## Suggested Follow-Up

When a config field that gates generated artifacts (shell scripts, templates,
embedded strings) is removed, the DAG task removing it should explicitly
enumerate "grep for `<removed-field-name>`" across the whole repo (including
string literals / heredocs, not just Go struct usages) as part of its own
definition of done — `go build`/`go vet` can't catch a dead conditional
embedded in a string constant.
