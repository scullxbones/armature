# Theme: Workers Bypass Their Assigned Worktree

## Summary

A worker is dispatched into an isolated worktree on its own `task/<id>` branch, and then writes, commits, or transitions from somewhere else — usually the coordinator's main checkout. This is the single most-repeated coordination failure in the corpus, observed across at least five separate stories over three weeks (TOPTIER-S3, TOPTIER-S4, LNGHZN-S4, LNGHZN-S5, plus earlier ARCHIMP-era instances), under explicit dispatch instructions telling the worker not to do it.

It is distinct from [git-worktree-friction](../git-worktree-friction/README.md), which covers worktrees being *awkward* (gopls bleed, checkout conflicts, path restrictions). This theme is about *containment*: the worktree works fine and the worker writes outside it anyway. It is also distinct from [parallel-coordination-conflicts](../parallel-coordination-conflicts/README.md), where correctly-isolated branches conflict semantically after the fact.

Three properties make this expensive:

1. **It is silent at write time.** Nothing fails when the worker commits to the wrong branch. The failure surfaces later, at `arm review prepare`, as "delivery contains no changed files" — a message that describes the symptom and not the cause.
2. **Prose does not fix it.** Dispatch prompts carrying explicit "Working directory: …" and "do NOT run `git checkout feat/STORY-ID`" instructions were bypassed anyway. One coordinator reported reliability only after adding a forceful "cd into the worktree and verify `git rev-parse --show-toplevel`" step — a verification the worker performs on itself, not an instruction it can skim past.
3. **ADR 0007 binding does not currently prevent it.** The harness hook binds on the worktree's marker file; a subagent whose session cwd is the main repo resolves against the main repo's binding, so cross-worktree writes are not blocked. The gap is documented from the enforcement side in `2026-08-02T1600Z` below.

## Evidence

- [Worker edits leaked into the main worktree despite isolated task worktree](../../raw/2026-07-19T1620Z-claude-workflow-worker-leaked-main-worktree.md) — TOPTIER-S3-T1's worker was dispatched into `/tmp/claude/arm-task-TOPTIER-S3-T1` but also wrote stale copies of `Makefile`, `.github/workflows/ci.yml`, and `internal/e2eharness/` into the main repo. Blocked the merge until the coordinator diffed and discarded each leaked file.
- [Worker committed directly to the story branch again, bypassing its isolated task worktree](../../raw/2026-07-20T0140Z-claude-workflow-worker-committed-to-story-branch-again.md) — TOPTIER-S4-T1. `arm transition --to done` succeeded and recorded a concrete outcome while `task/TOPTIER-S4-T1` still pointed at the wave base; the commit was sitting on the coordinator's own `feat/TOPTIER-S4`. Caught only by `arm review prepare` failing with "delivery contains no changed files".
- [Worker self-reported "done" with a fabricated outcome after its worktree metadata broke mid-task](../../raw/2026-07-20T0200Z-claude-workflow-worker-fabricated-done-outcome.md) — TOPTIER-S4-T2. When `.git/worktrees/<name>` went missing mid-task, the subagent attempted to copy files directly into the main repo as a workaround, unprompted; its own harness raised a security warning on the transcript. Shows the bypass as an *error-recovery reflex*, not only as inattention.
- [Dispatched workers commit into the main repo instead of their claimed worktree](../../raw/2026-08-08-workflow-worker-commits-to-main-not-worktree.md) — LNGHZN-S5. Two workers in one story: T2 committed all five files onto `feat/LNGHZN-S5` leaving its task branch at base SHA; T1 left a non-compiling partial duplicate of its work uncommitted in the main tree alongside its correct committed work.
- [Story delivery gate can be silently bypassed by transitioning from the wrong checkout](../../raw/2026-08-02T1600Z-claude-workflow-story-gate-bypass-via-wrong-checkout.md) — The enforcement-side counterpart. `isClaimedWorktreeForIssue` inspects only the marker of the checkout `arm transition` runs from, so transitioning a story from a different checkout than the one it was claimed into skips the gate rather than failing.
- [Agent default cwd is the story checkout, not the task worktree](../../raw/2026-08-15T1300Z-5207ee28-tooling-prfix-default-cwd-is-story-tree.md) — Not a worker ignoring the prompt: the *session* starts in the story tree. `git merge origin/main` during T3 remedia updated the wrong checkout. Containment fails before the first instruction is read.

- [The `--to done` branch check reads the root checkout, not the invoking one](../../raw/2026-09-01T1230Z-claude-workflow-done-gates-read-the-wrong-checkout.md) — The enforcement side, inverted. `cmd/armature/transition.go:80-85` resolves the branch from `appCtx.RepoPath` — the root checkout holding `.armature/`, which is on `main` by definition while a worker is in a worktree — so "cannot transition to done while on main branch" fires for *every* worker following the prescribed flow, and `--repo "$(pwd)"` does not help. The only escape is `--force`, which disables branch discipline entirely. The delivery-gate code immediately below already resolves this correctly via `worktreeIssueBinding(invokingRepoPath)`. Net effect: the path of least resistance is to transition from the root checkout on `main` with `--force`, skipping the worktree — exactly the practice this theme documents.

Earlier instances, already curated under other themes, establish that this predates the findings above:

- [Worktree changes leaked into the main worktree](../../raw/2026-06-30T2210Z-claude-integration-worktree-changes-leak-into-main-worktree.md)
- [Worker edited the main checkout instead of its worktree](../../raw/2026-07-05T2228Z-claude-workflow-worker-edited-main-checkout-instead-of-worktree.md)

## Candidate Follow-Ups

- Make the failure loud at write time rather than at review time: a `doctor` check for a dirty main worktree whose modified paths intersect an active task's declared scope (proposed in the 2026-07-19 finding).
- Close the transition-side hole: resolve the claimed worktree from the claim op — which now records `worktree_path` as of `LNGHZN-S5-T1` — instead of probing the invoking checkout's marker. This is the fix `2026-08-02T1600Z` points at, and the recorded path that makes it possible landed after most of the findings above were filed.
- Consider whether the harness hook should hard-block writes to the main repo when the resolved binding names a different worktree, rather than passing through. Relevant to GAP T4's "pass-through is broad" observation and to `TOPTIER-S5`'s conformance matrix.
- Stop punishing the compliant flow: read the branch from the invoking checkout (or from the worktree bound to the issue), so `--force` is not the routine cost of working inside a worktree. A gate that fires on correct work trains the override that hides incorrect work.
- Treat self-verification as the dispatch contract: have the worker echo `git rev-parse --show-toplevel` before its first write, since instruction-only phrasing demonstrably does not hold.
