---
writer: claude
area: validation
slug: validate-strict-broad-scope-errors
date: 2026-06-28T22:00Z
---

# `arm validate --strict` fails on stories/tasks with broad scope lacking `context_files`

## What I was trying to do

Running the auditor's step 4 check: `arm validate --strict` to confirm no scope overlap between tasks before story transition.

## What happened

```
$ arm validate --strict
ERROR: missing context_files on SMTC-S1 with broad scope ...
ERROR: missing context_files on SMTC-S1-T7 with broad scope ...
ERROR: missing context_files on SMTC-S1-T8 with broad scope ...
ERROR: missing context_files on SMTC-S1-T9 with broad scope ...
ERROR: missing context_files on SMTC-S1-T10 with broad scope ...
COVERAGE: 530/530 cited
Error: validation failed with 5 error(s)
```

These warnings existed before implementation began (they were in the initial `arm validate` output as WARNINGs). `--strict` promotes them to ERRORs. The auditor skill's step 4 says `arm validate --strict` "must exit zero" but doesn't mention that broad-scope warnings become blocking errors.

Had to amend all 5 issues with `--context-file` to add at least one context_files entry each:
```
arm amend SMTC-S1    --context-file "docs/superpowers/specs/2026-06-27-semantic-conformance-review-design.md"
arm amend SMTC-S1-T7 --context-file "internal/skillsembed/skills/armature-reviewer/SKILL.md"
arm amend SMTC-S1-T8 --context-file "internal/review/testdata/evals/README.md"
arm amend SMTC-S1-T9 --context-file "docs/commands.md"
arm amend SMTC-S1-T10 --context-file "cmd/armature/review_test.go"
```

After that, `arm validate --strict` passed clean.

## Why it matters

- The auditor skill says "Step 4 — Scope Overlap Resolution" and frames `arm validate --strict` as checking scope overlap between tasks. The actual failure was a different category: missing context_files on broad-scope issues, not overlap.
- The two error types are conflated under `--strict`, making it unclear which to fix.
- 5 pre-existing warnings from story planning became blockers at audit time — the coordinator skill could warn about these earlier (e.g., as part of wave dispatch pre-flight).

## Evidence

- `arm validate` (without `--strict`) at story start: 5 WARNINGs, `OK: no issues found`
- `arm validate --strict` at story completion: 5 ERRORs, exit 1
- After 5 `arm amend --context-file` calls: `arm validate --strict` → `OK: no issues found`

## Potential mitigations

- Auditor skill step 4 should mention that "broad scope" warnings also become errors under `--strict` and are separate from scope-overlap warnings.
- Coordinator pre-flight could run `arm validate --strict` before the first wave and require fixing any broad-scope warnings before dispatch.
- The `arm validate` warning message could link to the amend flag: `--context-file` (it currently only mentions `--scope <glob>`).
