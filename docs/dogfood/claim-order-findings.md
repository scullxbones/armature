# Claim-order dogfood findings (QA)

Writer: `d7cab84c-4a46-4511-bd9d-2685977753a3` (coordinator, unslotted).
Date: 2026-09-27.

## Fixed in a slice

None yet that required product code beyond CLAIMORD-W11 itself.

## Open

### DF-1 — `arm sources sync` re-fingerprints the entire manifest
- **Command:** `arm sources sync` after `arm sources add` of the claim-order spec
- **Happened:** ~150s; several historical filesystem sources STALE (missing local paths, `/home/brian/...`)
- **Should:** sync the new source, or skip unreachable entries without blocking the new OK source
- **Severity:** medium (known theme; did not block apply)

### DF-2 — Introduction refuses W1 overlap with stale open TOPTIER README tasks
- **Command:** `arm dag apply --plan claimord-plan.json --dry-run`
- **Happened:** DAG-1 Introduction on `CLAIMORD-W20` vs `TOPTIER-S8-T2` (and other live README.md scopes)
- **Should:** overlapping scope with abandoned/open product-docs tasks should be warning-only for a new story, or `arm ready` should not require those tasks merged
- **Severity:** high for W2.0 scheduling. Workaround: `blocked_by` the live README tasks so apply succeeds; `arm unlink` is also Introduction-gated (DF-3)

### DF-3 — `arm unlink` is Introduction-gated the same as create
- **Command:** `arm unlink --source CLAIMORD-W20 --dep TOPTIER-S6-T3`
- **Happened:** UNLINK-1 cannot introduce Graph Finding (scope overlap W1)
- **Should:** unlink of a serial-overlap edge should be allowed so the new task can run; or Introduction should not apply to unlink
- **Severity:** high. CLAIMORD-W20 remains blocked by `TOPTIER-S6-T3`; CLAIMORD-W21 remains blocked by `bug-1783480206`

### DF-4 — Ready queue requires blockers **merged**, not `done` (I6)
- **Command:** `arm ready --explain`
- **Happened:** `CLAIMORD-W12` reason `blocker(s) not merged: CLAIMORD-W11` while W11 is only claimed
- **Should:** this is intended I6; coordinators must `arm merged` after each slice even though GitHub PRs stay stacked/unmerged
- **Severity:** low (correct; easy to miss). Recorded because stacked PRs never land on `main` during this run

### DF-5 — Doctor D1 warning for unrelated `OPSCLEAN-1` in-progress
- **Command:** `arm doctor`
- **Happened:** D1 Git commits reference issues not in done/merged: OPSCLEAN-1
- **Should:** planner skill says clean all D1 before planning; cannot transition someone else's in-progress task
- **Severity:** low (doctor still exit 0)

### DF-6 — `arm show` JSON omitted `worktree_path` after `arm claim --worktree`
- **Command:** `arm claim CLAIMORD-W11 --worktree`; `arm show CLAIMORD-W11 --format json`
- **Happened:** `worktree_path` null/absent; worktree exists at `.worktrees/CLAIMORD-W11`
- **Should:** show the recorded path from the claim op
- **Severity:** medium (coordinator has to `git worktree list`)

### DF-7 — `arm list --parent` JSON does not include `blocked_by`
- **Command:** `arm list --parent CLAIMORD --format json`
- **Happened:** `blocked_by` field missing/null even when `arm show` lists blockers
- **Should:** list includes dependency fields the coordinator needs for wave planning
- **Severity:** low

### DF-8 — Planner E13 forces census docs onto every `cmd/**` task
- **Command:** `arm dag transition --issue CLAIMORD`
- **Happened:** E13 CLAIMORD-W11/W13 touch cmd/** while W21 owned `docs/commands.md` and surface-census.md
- **Should:** documented; we amended W11/W13 scopes. Still a planning footgun
- **Severity:** low after amend

### DF-9 — `arm amend --scope a b` treats extra tokens as args
- **Command:** `arm amend CLAIMORD-W11 --scope file1 file2`
- **Happened:** USAGE accepts at most 1 arg
- **Should:** skill/docs show repeated `--scope` flags (StringSlice). Easy to get wrong
- **Severity:** low

### DF-10 — Test layout `.armature` is not a git worktree
- **Command:** claim tests via `setupArmatureLayout`
- **Happened:** LocateOps against `git -C .armature` walked up to the code repo; win check lost the claim until filesystem fallback
- **Should:** tests should use a real ops worktree, or LocateOps must detect missing `.git` (we did the latter in W11)
- **Severity:** medium; fixed in W11 by non-git fallback (local logs treated as published so existing tests keep working)
