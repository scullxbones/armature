---
date: 2026-07-19
agent: claude
area: workflow
task: TOPTIER-S9 coordination
tags: [coordinator-skill, wave-type, grep]
---

# Coordinator skill's wave-type auto-promotion check false-positives on docs-only waves

## User Goal

Running the coordinator skill's "Wave Verification Gate" step for a docs-only wave
(TOPTIER-S9-T1: CONTRIBUTING.md, SECURITY.md, `.github/ISSUE_TEMPLATE/*.yml`), where the
skill instructs checking `CHANGED_FILES` against `^(Makefile|cmd/|internal/)` to decide
whether to promote from `docs-skill-only` to the `code` verification profile.

## Observed

The skill's shell snippet is:

```bash
echo "$CHANGED_FILES" | grep -E '^(Makefile|cmd/|internal/)' | grep -qvE 'internal/skillsembed' && echo PROMOTE
```

When `CHANGED_FILES` contains no lines matching `^(Makefile|cmd/|internal/)` (true for this
wave — only `.github/` and root-level `.md` files changed), the first `grep` emits nothing
and exits 1. The second `grep -qv` then receives empty stdin — and GNU grep's `-v` on empty
input exits 0 ("no lines fail to match the inverted pattern"), not 1. `&&` sees exit 0 and
prints `PROMOTE`, even though zero code files changed. Confirmed in isolation:
`printf '' | grep -qvE 'internal/skillsembed'; echo $?` -> `0`.

## Impact

Wasted a `go build`/`make check` cycle (which itself hit a sandbox read-only-cache failure,
requiring a sandbox-disabled retry) on a wave that never touched a `.go` file. Low cost here
since the retry was cheap, but on a larger docs-only story this silently forces the expensive
code profile every time, defeating the purpose of the two-profile split.

## Suggested Follow-Up

Fix the pipeline to not rely on `grep -v`'s empty-input behavior, e.g. check for non-emptiness
explicitly instead of chaining `&&` off the second grep's exit code:

```bash
CODE_HITS=$(echo "$CHANGED_FILES" | grep -E '^(Makefile|cmd/|internal/)' | grep -v 'internal/skillsembed')
[ -n "$CODE_HITS" ] && echo PROMOTE
```
