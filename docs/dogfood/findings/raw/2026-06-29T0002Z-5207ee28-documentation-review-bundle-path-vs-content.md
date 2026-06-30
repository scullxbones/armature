---
area: documentation
slug: review-bundle-path-vs-content
writer: 5207ee28
date: 2026-06-29T00:02Z
---

# Finding: `--bundle` expects file path but SKILL.md passes JSON content

## What I was trying to do

Reviewing PR #64 (pr-63 branch) using `prfix`. The Codex bot left one inline review comment on the armature-coordinator SKILL.md at line 340.

## What happened

The documented flow at steps 2–4 of the "Coordinate review wave" section shows:

```bash
REVIEW_BUNDLE=$(arm review prepare --issue TASK-ID --base "$TASK_BASE" --head "$TASK_HEAD")
# ...
arm review record --issue TASK-ID --assessment <result.json> --bundle "$REVIEW_BUNDLE"
```

`$REVIEW_BUNDLE` captures the stdout JSON blob. But `runReviewRecord` in `cmd/armature/review.go` calls `os.ReadFile(filepath.Clean(bundleFile))` — it expects a file path. Passing JSON content as `--bundle` means the CLI tries to `open` a multi-KB JSON string as a filename, which fails with a file-not-found error.

## Evidence

- `cmd/armature/review.go:192`: `bundleData, err := os.ReadFile(filepath.Clean(bundleFile))`
- `review prepare` already supports `--output <file>` (line 49 of review.go)
- The variable name `REVIEW_BUNDLE` obscures whether it holds content or a path

## Impact

Any coordinator following the SKILL.md as written would hit a cryptic error when trying to record a review assessment with a bundle. The fix required reading the source code to discover the `--output` flag pattern.

## Behavioral change

Slowed discovery: had to cross-reference CLI source to identify the mismatch. Confidence dropped in the documented workflow until source confirmed the issue.
