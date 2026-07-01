---
date: 2026-06-30
agent: claude
area: documentation
task: PR #65 follow-up review remediation (armature repo)
tags: [skillsembed, prfix, doc-drift, doc-to-doc-consistency]
---

# Fixing two PR review comments in a SKILL.md left four neighboring passages inconsistent with the fix

## User Goal

Finish remediating PR #65 review feedback: two prior fixes (already staged as
uncommitted changes) corrected imprecise wording in
`armature-coordinator/SKILL.md` (Dispatch Protocol items 4-5, pointing workers
at isolated worktrees/task branches instead of the main checkout/shared story
branch) and `armature-reviewer/references/field-rules.md` (line-citation rule
scoped to added/modified `+` lines, not any line "in the hunk"). A follow-up
Opus review pass was dispatched specifically to check whether those two fixes
left anything else inconsistent in the surrounding docs.

## Observed

The Opus review found four more issues, all caused by the first two fixes
correcting a *local* passage without updating other passages in the *same
files* that assumed the old (wrong) semantics:

- `armature-reviewer/SKILL.md` lines ~205, ~214 still said a citation must
  correspond to "a specific line in a diff hunk" / "verify it exists in that
  file's diff hunk" — the same imprecision (context lines vs. `+` lines) that
  had just been fixed in `field-rules.md` item 2, but restated elsewhere in a
  different file without the fix applied.
- `armature-coordinator/SKILL.md` Section 1 (~line 98) still said "All workers
  commit to this branch [feat/STORY-ID]" — directly contradicting the just-fixed
  Dispatch Protocol items 4-5 two hundred lines later in the same file, which
  now correctly say workers commit to a per-task branch in an isolated worktree.
- `armature-coordinator/SKILL.md` "After Workers Return" sections a.2/a.3 used
  `git rev-list "$WAVE_BASE_SHA"..HEAD` and `git diff "$WAVE_BASE_SHA"..HEAD` to
  discover each task's commits — this assumed all task commits were already on
  the story branch's HEAD, which was true under the old shared-branch model but
  is false under the corrected worktree model (task commits live on
  `task/TASK-ID` branches, only reachable from HEAD *after* the merge step in
  section b, which runs *later* in the same document). Following the doc
  top-to-bottom as originally written would have silently produced empty commit
  ranges at exactly the step meant to gate wave sign-off.
- `field-rules.md` item 2 said "if the file was deleted, citations are not
  valid" but item 3 (path-level citations) doesn't carry that exclusion, and the
  actual validator (`ContainsFile` in `diffindex.go`) returns true for deleted
  files' diff entries — so a path-level citation to a deleted file would
  incorrectly pass validation contrary to the doc's blanket claim.

## Impact

- This is the same failure mode as an earlier captured finding
  (`2026-06-30T2340Z-...embedded-skill-docs-drift-from-implementation.md`), but
  one level removed: instead of doc-vs-code drift, this was doc-vs-doc drift —
  a targeted fix to one passage left other passages describing the same
  mechanism uncorrected, because the first fix pass (per this task's
  instructions) only touched the two commented-on lines rather than grepping
  the whole file (and sibling files) for other references to the same
  semantics.
- All four issues were the kind that would only surface when someone (or some
  agent) actually tried to follow the doc's later section and found the
  referenced git state didn't exist yet, or found the doc contradicting itself
  a few hundred lines apart.
- Confirms the suggested follow-up from the earlier finding: a mechanical
  cross-reference check (e.g., grep for related terms/phrases across a
  SKILL.md and its references/ siblings after any edit to citation/branch
  semantics) would catch this class of drift before a second human/bot review
  pass is needed to find it.

## Evidence

- `internal/skillsembed/skills/armature-reviewer/SKILL.md` lines ~205, ~214
- `internal/skillsembed/skills/armature-coordinator/SKILL.md` Section 1
  (~line 98) vs. Dispatch Protocol items 4-5 (~line 225) vs. "After Workers
  Return" a.2/a.3 (~lines 297-445) vs. section b (~line 458)
- `internal/skillsembed/skills/armature-reviewer/references/field-rules.md`
  item 2 vs. item 3 (~lines 187-196)
- `internal/review/diffindex.go` `ContainsFile`, `BuildDiffIndex`;
  `internal/review/validate.go` `ValidateResult`
- PR #65 review comments 3502849824, 3502849825; Opus follow-up review pass

## Suggested Follow-Up

Same as the earlier finding: consider a doc-consistency lint that, when a
SKILL.md or references/*.md passage describing citation/branch/worktree
semantics changes, flags other passages in the same file or sibling
references/ files containing overlapping keywords (e.g. "diff hunk", "commit
to this branch", "HEAD") for manual re-check, so a single review comment fix
doesn't need a second full-file review pass to catch what it broke elsewhere.
