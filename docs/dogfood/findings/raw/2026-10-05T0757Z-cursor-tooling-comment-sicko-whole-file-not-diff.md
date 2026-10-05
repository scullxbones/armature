---
date: 2026-10-05
agent: cursor
area: tooling
task: overnight no-comments fence on PRs 302-311
tags: [comment-sicko, fence, gh-pr-diff]
---

# Comment Sicko judged whole fenced files, not merged PR diffs

## User Goal

Run an overnight `/no-comments` pass whose fence is files changed by PRs merged into `main` in a 24-hour window, using `gh pr diff`, not the whole repo.

## Observed

[Comment Sicko](bc-d80874a6-3042-54c0-8ce8-5dcf2dae20be) returned 538 comment lines to delete across 45 files. `gh pr diff` for those PRs added about 98 comments in #307 and far fewer in the others. Most of the 91 `internal/adapters/git.go` kills were comments that existed before #307 added the delivery git helpers.

## Impact

Triage had to subtract pre-existing public-API docs, git protocol comments, and ADR cites from a whole-file kill list. That took a full pass over current files plus the diffs. Shipping the report as-is would have stripped comments the caller listed as keeps.

## Evidence

- Caller fence: use merged PR diffs (`gh pr diff`), skip docs, `ops/*.log`, goldens.
- Comment Sicko deletion table: `internal/adapters/git.go` die 91 / keep 54.
- Added-comment extract from `gh pr diff 307`: eight new git helper docs at the end of `git.go`, not the porcelain/`GIT_DIR` comments.

## Suggested Follow-Up

Pass Comment Sicko the `gh pr diff` hunks (or a list of added comment lines) instead of current file paths when the fence is a merge window.
