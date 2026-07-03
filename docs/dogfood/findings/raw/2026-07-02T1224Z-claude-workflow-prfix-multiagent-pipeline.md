## What I was trying to do

Run `/prfix` against GitHub PR #66 (scullxbones/armature) using a user-specified
multi-agent pipeline: haiku agent fixes findings via TDD + `make build/lint/test`,
fable agent reviews the fixes, sonnet agent implements any follow-ups, commits,
pushes, replies to review comments, and resolves conversations.

## What happened

- REST `gh api repos/.../pulls/66/comments --paginate` returned all inline review
  comments but with no resolution status field, and the payload was large (151.7KB,
  auto-saved to a file instead of inline). Had to switch to a GraphQL
  `reviewThreads` query to get `isResolved`/`isOutdated` per thread — the two IDs
  (thread ID like `PRRT_...` vs comment `databaseId`) are different identifiers and
  both were needed downstream (comment ID for reply-to-comment REST endpoint,
  thread ID for the `resolveReviewThread` GraphQL mutation). Easy to mix up if not
  tracked carefully together.
- Replying "in-thread" to a review comment requires the specific REST endpoint
  `pulls/{n}/comments/{id}/replies`, not a plain new comment POST — had to spell
  this out explicitly in the subagent prompt, otherwise the natural instinct is to
  post a top-level PR comment referencing the finding instead of a true threaded
  reply.
- After the haiku and later sonnet agents' work, the IDE/diagnostics linter flagged
  `interface{}` → `any` and a `strings.SplitSeq`-range efficiency nit in the touched
  files, but `make lint` reported clean both times. This is a two-tier lint signal
  mismatch: the repo's `make lint` (presumably golangci-lint with a specific config)
  doesn't run the same ruleset as whatever backs the inline diagnostics, so agents
  following "run make lint and treat clean as done" will miss issues visible via
  the editor/diagnostics channel. The fable review agent caught and correctly
  triaged these as pre-existing/out-of-scope, but a less careful pass could easily
  ship them or spend time chasing lint tools that disagree.

## How it changed behavior, confidence, time spent

- Had to do one extra GraphQL round-trip up front (beyond the REST comments call)
  purely to get resolution state — added a step that wasn't obvious from the PR URL
  alone.
- Spent explicit prompt real-estate in the sonnet agent's brief mapping each
  comment ID to its paired thread ID 1:1, to avoid a mismatch (e.g. resolving the
  wrong thread).
- No time lost overall — the three-stage agent pipeline (haiku fix → fable review →
  sonnet finalize) worked cleanly end to end with no back-and-forth or rework
  needed between stages.

## Evidence

- `gh api repos/scullxbones/armature/pulls/66/comments --paginate` output saved to
  `/home/brian/.claude/projects/.../tool-results/bhi70eq57.txt` (151.7KB, no
  resolution field).
- GraphQL `reviewThreads` query returned `isResolved`/`isOutdated` plus thread IDs
  (`PRRT_kwDORnVQE86NtEk1`, etc.) distinct from comment `databaseId`s
  (`3508712071`, etc.).
- New-diagnostics blocks appeared after both the haiku and sonnet agent edits
  flagging `interface{}`/`any` and `stringsseq` in `bootstrap_test.go`/`bootstrap.go`,
  while both agents' `make lint` runs reported clean.
