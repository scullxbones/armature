---
date: 2026-08-16
agent: 5207ee28
writer: 5207ee28
area: workflow
task: LNGHZN-S10-T10
story: LNGHZN-S10
tags: [prfix, implement-review-loop, skill-prose]
---

# Adversarial pass kept promoting "comment says loop" until the snippet exited 1

## User Goal

After the first implement pass on PR #108, run implement → adversarial
review until only medium/low findings remained, with T10 intent as spec.

## Observed

Three sequential review passes on the same coordinator a.2 fence:

1. First review: `$TASK_ORIGINAL_SLOT` undefined (high) and `CYCLE`
   leaking across wave tasks (high).
2. Second review: step 6 `unset RESULT_FILE` then never rebound it
   before step 7 `jq` (high). The "extract RESULT_FILE from the
   response" sentence was already in T10; making step 7 executable
   made that prose hole load-bearing.
3. Third review: after the bind existed, yellow|red with `CYCLE < 3`
   incremented and commented "repeat steps 5–6", then returned 0 into
   a.3. Reviewer refused to call that medium: headings after a
   successful fence do not bind a snippet-copier.

Stop condition was only met when every non-green arm used `exit 1`.
A `while` wrapper was not required.

Each review was a fresh read-only subagent (~7–10 min). Implement
passes for the later highs were 1–3 minutes.

## Impact

The loop did what a single implement pass would not: it treated
"agents copy snippets, not comments" as a severity rule and kept
re-opening the same fall-through until the executable path could not
reach a.3. Cost was four review dispatches plus three implement
dispatches after classification.

## Evidence

- PR #108, worktree `.worktrees/LNGHZN-S10-T10`
- Reviewer stop_conditions: CONTINUE, CONTINUE, CONTINUE, DONE
- Final step 7: `green) ;;` only fall-through; yellow|red and `*)`
  `exit 1`

## Suggested Follow-Up

Encode in `/prfix`: after applying VALID comments, a fresh reviewer
grades against the originating issue contract + the review comments;
re-implement only critical/high; stop when none remain. Severity
rule for skills: a comment that describes a loop/gate is not a gate.
