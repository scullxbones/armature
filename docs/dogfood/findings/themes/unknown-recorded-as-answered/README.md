# Theme: "Not Checked" Is Recorded As a Definite Answer

## Summary

Armature has three possible truths about any claim it makes — **true**, **false**,
and **not checked**. Its data model and its output have only two. Wherever a check
is absent, skipped, or structurally incapable of seeing the thing it names, the
result is not "unknown"; it is silently rendered as one of the two confident values.
Most often that value is the reassuring one.

The sharpest instance is the constitutional one. `CONSTITUTION.md` I6 defines the
two states as *"self-reported completion and confirmed-on-main."* The word is
**confirmed**. But `arm transition --issue X --to merged` is a documented, unguarded
status change that consults no git evidence at all, and `RunRollup`
(`internal/materialize/engine.go:589`) promotes a parent once all children hold that
status. So `merged` — the state whose entire definition is "we checked" — is
reachable, and reachable transitively, without anything having been checked.
`ARCHIMP-S15` is that path executed to completion: a story recorded `merged` with a
detailed outcome describing code that has never existed on `main`, with
`arm validate` reporting 730/730 clean over it.

The same shape recurs with different surface symptoms:

- **A gate that did not run is recorded as a gate that passed.** The story delivery
  gate is skipped when `arm transition` is invoked from a checkout other than the
  claimed worktree — and the op is appended with `SkippedDeliveryGate: false`. The
  log positively asserts that no override occurred, for a check that never executed.
- **A precondition the tool cannot observe is assumed satisfied.** `DetectMerges`
  (`internal/sync/sync.go:17`) skips any issue with an empty `Branch` and then tests
  ancestry; the caller prints `No merged branches detected.` That sentence is a
  claim about the world. Its actual meaning is "I had nothing to look at, and/or the
  thing I know how to look at is the wrong thing for this repository."
- **A side effect that never happened is durably recorded as having happened.**
  `arm claim --worktree` under sandbox appends the claim op and then hangs; the issue
  is `claimed` against a worktree that does not exist.
- **Absence of evidence is presented identically to evidence of absence.** A task
  reviewed three times on GitHub carries no conformance assessment. `arm show` and
  the coverage rollup make "reviewed and clean" and "never reviewed here"
  indistinguishable — and `arm merged`'s worktree teardown then destroys the activity
  log that retroactive recovery would need.

The cost is not any single wrong value. It is that every downstream consumer —
`arm ready`, the rollup, the auditor's five-check gate, a fresh coordinator reading
`arm list` — is built on the assumption that a recorded state means something was
established. Once "not checked" and "checked and fine" share a representation, no
amount of care by the reader can separate them, and the specificity of the outcome
prose actively works against suspicion: `ARCHIMP-S15`'s outcome names a file, four
consts, a test count and a mutation score. Specificity read as evidence.

## Why this frame, and what it displaces

Three alternative framings were considered against this evidence:

- **"The system's model of git is narrower than git's actual workflows"** (the
  squash-merge framing). Real, but it explains one finding. And verification here
  shows it is not even the operative cause of that one: `LNGHZN-S10-T5` has no
  `branch` recorded on any op, so `DetectMerges` skips it at the empty-`Branch`
  guard and never reaches the ancestry test the finding names. See the discrepancy
  note below. Ancestry-vs-squash is a *second* blindness stacked on the first; the
  thing both have in common is that neither is reported as a blindness.
- **"Evidence stranded outside the system of record"** (I4/citability). This is a
  consequence, not the mechanism. Evidence lives on GitHub because nothing inside
  Armature ever asked for it at a moment when it could have been captured.
- **"Silence and false confidence where there should be a signal"** (the originating
  session's framing). Correct as far as it goes, but it describes the *symptom* and
  therefore cannot say which findings belong. Reframing it as a missing third value
  in the data model is what makes the boundary testable, tells you where to look
  next, and names the fix: distinguish `verified false` from `not checked`, and
  make the second one loud.

## Evidence

### The 2026-08-23 closeout session

- [The DAG records `merged` for work that has never been on main](../../raw/2026-08-23T1612Z-claude-validation-dag-records-merged-for-work-never-on-main.md) —
  `ARCHIMP-S15`. Verified independently while curating: `git ls-tree origin/main internal/claim/`
  shows only `claim.go`/`overlap.go` and their tests; `git log origin/main --grep=ARCHIMP-S15`
  is empty; `git log -S'PlanClaim' origin/main` hits only a dogfood file and a plan doc.
  `arm show ARCHIMP-S15` → `Status: merged`.
  **The mechanism the finding left open is now answered.** The ops log shows plain
  `["transition","ARCHIMP-S15-T1",…,{"to":"merged"}]` and the same for `T2` — no
  `arm merged`, no `pr`, no `branch`, no PR ever opened, both within ~12 minutes of the
  claim. The story then reached `merged` by rollup over those two. Nothing in the chain
  ever touched git.
- [`arm sync` is structurally blind to this repo's own squash-merge workflow](../../raw/2026-08-23T1533Z-claude-workflow-sync-blind-to-squash-merges.md) —
  "No merged branches detected" printed for eleven PRs that are merged on GitHub and
  present on `main` by content.
- [A task can be fully reviewed on GitHub and still have zero conformance evidence in Armature](../../raw/2026-08-23T1538Z-claude-workflow-review-evidence-stranded-on-github.md) —
  `LNGHZN-S10-T5`, three review cycles on PR #112, no `.armature/review/` entry. The
  recovery window closes when `arm merged` tears the worktree down. Worth noting what
  *did* work: the operator recorded a `decision` op ("I7 override: accept missing
  conformance review"). The escape hatch exists and is auditable; what is missing is
  anything that makes reaching for it non-optional.

### The same shape, earlier in the pile

- [Story delivery gate can be silently bypassed by transitioning from the wrong checkout](../../raw/2026-08-02T1600Z-claude-workflow-story-gate-bypass-via-wrong-checkout.md) —
  The single strongest historical instance, and the closest to a literal statement of the
  mechanism: *"the op is appended with `SkippedDeliveryGate: false`, so the audit trail
  shows no override even happened."* A field that exists precisely to record "the check
  was skipped" reports false when the check never ran. Contrast `--skip-delivery-gate`,
  which is loud and auditable — the documented override is safer than the accidental path.
- [`arm claim --worktree` silently hangs under sandbox instead of erroring](../../raw/2026-07-23T2200Z-claude-permissions-worktree-claim-hang.md) —
  Claim op durably recorded; worktree never created; no error. State asserts a binding
  the system never established.
- [Hollow tests let a dead worktree-lifecycle path survive multiple review rounds](../../raw/2026-08-08T1900Z-claude-validation-hollow-tests-masked-dead-worktree-path.md) —
  The same defect one layer down, in the machine-checkable evidence itself: a test that
  computes the path the same wrong way production does cannot distinguish "works" from
  "never ran." `NotEmpty`/`NotNil` assertions are the assertion-level version of a
  two-valued model.
- [S18 seam refactor introduced a class of silent-failure regressions](../../raw/2026-07-05T0000Z-claude-validation-s18-silent-error-class.md) —
  `verifyResults, _ := lc.VerifyAll()` renders an unreadable manifest as
  "No stale sources detected." Identical sentence-level failure to `arm sync`'s
  "No merged branches detected", four weeks earlier, in a different subsystem. The
  finding's own conclusion generalizes: *seam-deepening refactors tend to convert loud
  failures into silent skips at the new boundary.*
- [Cross-story scope overlaps do not warn while same-story overlaps do](../../raw/2026-07-03T1600Z-claude-validation-cross-story-scope-overlap-silent.md) —
  A clean `arm validate` means "no same-parent collision," and is read as "no collision."
  The most dangerous case is the silent one.
- [Green `make check` missed a P0 regression the fix itself introduced](../../raw/2026-07-01T1330Z-claude-validation-green-ci-missed-p0-regression.md) and
  [worker self-reports](../unreliable-worker-self-report/README.md) generally —
  green is emitted for paths no test exercises; `done` is emitted for work no gate saw.
- [Worker left task in `claimed` state despite reporting success](../../raw/2026-06-28T2200Z-claude-workflow-worker-left-task-claimed.md) —
  The oldest instance in the pile (2026-06-28), and the one that establishes this is not
  new: the report and the record disagreed, and only re-running a command distinguished them.
- [Coordinator skill's wave-promotion check false-positives on docs-only waves](../../raw/2026-07-19T2200Z-claude-workflow-wave-promotion-false-positive.md) —
  Included with a caveat. `grep -qv` on empty stdin exits 0, so *zero* code files reads
  as PROMOTE. It is a proxy-signal failure and it is confidently wrong, but it errs toward
  the expensive gate, not away from it. It belongs to the mechanism and not to the risk.

### Cross-listed, already curated elsewhere

- [`arm merged` reads the stale snapshot `arm transition` refuses to trust](../../raw/2026-08-12T0204Z-claude-tooling-arm-merged-reads-stale-snapshot-after-transition.md) —
  see [missing-remediation-verbs](../missing-remediation-verbs/README.md). Two halves of the
  documented closeout pair disagree about what has been established.
- [I6 promotion is agent-owned: claim never records `Branch`](../../raw/2026-08-17T0233Z-5207ee28-coordination-i6-promotion-agent-owned-metadata.md) —
  see [i6-promotion-agent-owned](../i6-promotion-agent-owned/README.md). That theme
  covers the *missing metadata*; this one covers the fact that its absence is reported
  as a negative finding rather than as an inability to answer.

### The 2026-08-31 → 2026-09-02 additions

- [`arm sync` skips every done issue because `branch` is never recorded](../../raw/2026-08-31T1142Z-claude-workflow-sync-skips-every-issue-branch-never-recorded.md) —
  This resolves the discrepancy noted at the bottom of this file, in favour of the
  verification: the population is not merely mis-examined, it is **never examined**.
  80 of 80 `done` issues have no `branch`; `DetectMerges` `continue`s before any
  ancestry call. `No merged branches detected.` is a claim about the world produced
  by a loop that inspected nothing, and it has never meant anything else in this repo.
- [`arm merged` exits 1 after the transition already succeeded](../../raw/2026-08-31T1150Z-claude-workflow-arm-merged-reports-failure-after-succeeding.md) —
  The mirror image, and the reason "just read the exit code" is not the fix. Here
  the durable state is correct and the command reports `general_error` because a
  disposable admin directory would not unlink. Both directions cost the same thing:
  the exit code is not a statement about what was established.
- [The harness hook's direct-commit block matches only a bare `git`](../../raw/2026-08-23T2010Z-claude-validation-direct-commit-block-trivially-bypassed.md) —
  The same shape one layer down, at the seam. `isDirectCommitCommand` returns false
  unless `fields[0] == "git"`, so `cd sub && git commit`, `/usr/bin/git commit`,
  `sh -c '…'` and `env git commit` all pass through — none of them adversarial
  constructions. Nothing is logged when the guard declines to match, so "not matched"
  is indistinguishable from "checked and allowed". Per ADR 0007 the hook is
  deliberately best-effort, not a sandbox; the finding is that this matcher's
  *breadth* is narrower than its message implies, and that extending the mechanism to
  state-transition verbs would inherit the porousness on day one.
- [A properly source-linked issue still fails the stop hook as "uncited"](../../raw/2026-09-01T1221Z-claude-validation-source-link-alone-fails-the-stop-hook.md) —
  Not a missing check but two checks with different definitions of the same predicate.
  `arm validate` counts source-linked and accepted-risk as alternatives and reports
  `767/767 cited`; `harnesspolicy/resolver.go:94-108` marks a link `Accepted` only via
  an acceptance op, in *both* branches, so `CheckCitations` blocks a real, registered,
  synced source. The natural repair inverts the meaning of `accept-citation` — which
  exists to record *accepted risk* — so the 72 accepted-risk entries stop counting
  undersourced work. Separately, `arm dag apply` writes a source-link op carrying only
  `source_id` where `arm sources link` writes `source_id` *and* `source_url`: two
  commands creating the same relationship produce different ops. Because the hook
  evaluates only the *claimed* issue, the identical gap on the sibling issue went
  unreported until someone claims it.

## The shared mechanism

Concretely, and in the order a fix would address them:

1. **No third value.** No op field, no `IssueJSON` key, and no CLI exit code
   distinguishes *checked and false* from *not checked*. `SkippedDeliveryGate: false`
   is the proof: the schema had room for the distinction and still collapsed it.
2. **Proxy signals stand in for ground truth, undeclared.** Ancestry stands in for
   "content is on main." A status transition stands in for "a PR landed." The
   invoking checkout's marker file stands in for "which worktree was claimed." Each
   proxy is cheaper and each is right most of the time; none of them announces the
   cases where it cannot see.
3. **The reassuring default.** When the proxy has nothing to work with, the emitted
   answer is the clean one — `No merged branches detected`, `No stale sources
   detected`, `OK: no issues found`, `runGate = false`. There is no instance in this
   pile of an unchecked path failing closed.
4. **Confident prose in the record.** Free-text `outcome` is where a worker's
   self-report is stored verbatim, and it is the most specific-looking field a reader
   sees. Nothing in the system distinguishes an outcome corroborated by a merged PR
   from one typed by an agent four minutes after claiming.

## On the I5 constraint

The originating session's constraint holds, and it is worth stating precisely
because it is load-bearing for every follow-up below.

I5 says LLM judgment is *advisory input only, never an automated merge decision*. A
gate that reads a `ConformanceAssessment`'s `rating` and refuses `merged` on `red`
would make an LLM's opinion the deciding input to a merge — squarely prohibited.
But three things a fix could gate on are not LLM judgment at all:

- **Is the delivery content present on the target branch?** Pure git. Patch-id,
  cherry-mark, or path/symbol presence. Deterministic, reproducible, no model involved.
- **Does a conformance assessment exist for this issue?** A file-existence test.
  Its *contents* are advisory; its *presence* is a fact. Gating on presence records
  that the human-or-agent review step occurred, without importing its verdict.
- **Did the gate actually run?** A boolean the system already writes down and
  currently writes wrongly.

So the I5-clean framing is: gate on *facts about the world and about the process*,
never on *the rating*. The reasoning survives scrutiny. One refinement worth adding:
the point is not merely that content-based checks avoid an LLM — it is that I5's
whole force depends on the deterministic gate having actually been evaluated. A gate
that silently does not run is an I5 violation of exactly the same kind as an LLM
deciding, because in both cases the merge decision was not made by a deterministic
check. The `SkippedDeliveryGate: false` hole is therefore not adjacent to I5; it is
inside it. I7 is the correct pressure valve, and the `LNGHZN-S10-T5` `decision` op
shows the shape: make the override explicit, attributed, and recorded — never the
default.

## What this theme does not cover

- **`arm doctor` D8 attributing environment dotfiles to a story**
  ([finding](../../raw/2026-08-23T1536Z-claude-validation-d8-attributes-environment-files-to-a-story.md)).
  Filed in the same session and often grouped with the other three, but it is the
  *inverse* on the axis that matters here: loud, blocking, reproducible, and impossible
  to miss. It shares the root — asserting a specific answer ("these files belong to
  `LNGHZN-S7`") where the honest answer is "these files belong to no issue and I cannot
  tell" — but its cost profile is opposite, and the remedy is opposite too: this theme
  argues for *more* failing-closed, and D8 is a case for *less*. Its curated home is
  [sandbox-environment-vs-gates](../sandbox-environment-vs-gates/README.md), where it is
  the third recurrence (2026-07-25, 2026-08-08, 2026-08-23) of a check that is always red.
  Keeping it out of this theme is the boundary: **a check that is confidently wrong in
  the noisy direction is a different product problem from one that is confidently wrong
  in the quiet direction**, even when the modelling defect underneath is the same.
- **Reviewer-agent schema and citation failures**
  ([reviewer-agent-reliability](../reviewer-agent-reliability/README.md)). Those fail
  loudly and correctly; `arm review record` is one of the few places in the corpus that
  refuses rather than assumes.
- **Worktree containment failures** ([worker-worktree-bypass](../worker-worktree-bypass/README.md)).
  Adjacent — "silent at write time" is that theme's own property #1 — but the defect
  there is that isolation is a convention rather than an enforced boundary. Only its
  *enforcement-side* instance (the wrong-checkout gate bypass) belongs here.
- **Documentation and render-context gaps.** Missing information that nobody claimed to
  have checked is a different failure from a check that reports an answer it did not compute.

## Discrepancy noted while curating

The `arm sync` finding attributes the failure to ancestry-based detection
(`BranchMergedInto`) versus squash merges. Verification says that is not the operative
cause for the issues in question: no `branch` value appears on any op for
`LNGHZN-S10-T5` (`claim`, `amend`, and both `transition` ops carry `worktree_path`,
`scope`, `outcome` — never `branch`), and `DetectMerges` `continue`s on
`issue.Branch == ""` before any ancestry call. Both blindnesses are real and both would
need fixing; the empty-`Branch` one fires first and is already documented under
[i6-promotion-agent-owned](../i6-promotion-agent-owned/README.md). The raw finding is
left unedited, per curation rules. Its conclusion — *"silence is the harmful part"* — is
unaffected and is the point.

## Candidate Follow-Ups

- **Refuse to reach `merged` without git evidence.** `arm transition --to merged`
  should require what `arm merged` requires, or be removed as a path. Today it is
  documented in `arm transition --help` (`--to merged --pr 1234`) and the `--pr` is
  optional, which is how `ARCHIMP-S15-T1` and `-T2` got there with nothing at all.
- **Make `RunRollup` verify rather than aggregate.** A parent promoted because its
  children hold a status inherits whatever was never checked about them. Rollup is
  the amplifier that turned two unverified task transitions into a `merged` story.
- **A `doctor` check: for every `merged` issue, is its declared scope's content on
  `origin/main`?** Content-based, no LLM, I5-clean, and it would have caught
  `ARCHIMP-S15` at any point in the last two months. This is the single highest-value
  item here — it is also a one-time audit worth running over the whole DAG now, since
  `ARCHIMP-S15` was found by accident during an unrelated branch cleanup.
- **Never emit a clean-sounding sentence for an unexamined population.** `arm sync`
  should report `N done issues skipped: no branch recorded` alongside
  `No merged branches detected`. Same for `stale-review`. Count what you looked at.
- **Fix `SkippedDeliveryGate` to mean what it says,** and fail closed when a claimed
  worktree exists for the issue regardless of the invoking checkout — the fix direction
  the 2026-08-02 finding already names.
- **Gate `arm merged` on the *presence* of a conformance assessment,** with an explicit
  recorded override (`--i7-override --reason …`) as the only bypass. Presence, never
  rating. Order it before worktree teardown so the recovery window is not closed by the
  same command that discovers the gap.
- **Consider recording what a worker claims it ran as structured fields** rather than
  prose — already proposed under
  [unreliable-worker-self-report](../unreliable-worker-self-report/README.md), and this
  theme is the argument for why it matters at the *record* level and not only at dispatch:
  `ARCHIMP-S15`'s outcome text is the artifact that made the false state credible.
