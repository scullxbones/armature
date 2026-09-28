# ADR 0022: Promotion is a recorded delivery on the target

A leaf becomes `merged` only when a delivery recorded in git is on the
integration branch. `arm sync` and `arm merged` are the only writers.
A missing assessment blocks the append. The rating does not.

## Status

Accepted — amends ADR-0005 and ADR-0008: assessment *presence* is a
prerequisite for the merged append; the rating remains unread.

## Principles touched

I1, I4, I5, I6, I7, N4

## Context

LH D4 (`docs/design/long-horizon-proposals.md`) asked for one
`done → merged` path. Closeout of `LNGHZN-S9` and `LNGHZN-S10` showed the
live path does not detect work that landed, and a bare
`arm transition --to merged` can record work that never landed
(`docs/dogfood/findings/themes/unknown-recorded-as-answered/`,
`docs/dogfood/findings/themes/i6-promotion-agent-owned/`).

Measured with git 2.54.0: `git merge-base --is-ancestor` misses a squash.
`git cherry` also misses it, because a two-commit squash is one combined
diff. The stable patch-id of `git diff base tip` equals that squash commit.
A later unrelated commit on the target does not remove the squash commit.
A cherry-pick of one commit does not equal the combined diff.

`issue.Branch` is copied only from a transition payload. `arm claim` does
not write it, so `DetectMerges` skips the issue before ancestry runs.
`arm show` drops `branch` and `pr`. `RunRollup` can show a parent `merged`
with no op. ADR 0017 is the success envelope. ADR 0020 is the Command
Failure `{error:{code,cause,next_actions,exit_code}}` on stdout.

## Decision

1. **Leaf.** An issue with no children becomes `merged` only when its
   recorded delivery is on its integration branch. Parents roll up in
   memory, as today, when every child is `merged` or `cancelled` and at
   least one is `merged`. `arm show` labels that parent status derived.
   Rollup writes no branch, PR, or assessment.

2. **Delivery.** `done` from the bound worktree records `branch`, `base`,
   `tip`, and `integration_branch`, and writes
   `refs/armature/deliveries/<issue-id>` at the tip. The base is the parent
   tip for a `--from` sub-task, otherwise the merge-base with the
   integration branch. Config `integration_branch` defaults to `main` when
   empty. `--skip-delivery-gate` and `--force` do not skip the snapshot.
   `arm delivery record --issue <id> --base <sha> --tip <sha>` and
   `arm transition --to done --base <sha> --tip <sha>` write the same record
   when both objects exist, the base is an ancestor of the tip, and the
   diff is non-empty. A readable worktree HEAD that disagrees refuses.
   Missing objects refuse, do not mark `done`, and do not recommend
   `delivery record` again. A missing or unreadable worktree does not
   relax the record. The tip must sit on recorded branch or claim
   provenance for that issue: it is the recorded claimed-branch tip, an
   ancestor of that tip, or a descendant of the recorded claim-time HEAD.
   Any other non-empty ancestor range refuses. No recorded branch or
   claim provenance fails closed: provenance-free recovery is the human
   override path, not another `delivery record` of an unrelated range.

3. **On the target.** Walk the first-parent history of the integration
   branch after the base. The matched commit is the earliest that qualifies:
   the tip is an ancestor; or the stable patch-id of `diff base tip` equals
   that commit's diff **and** a second check then proves either byte-exact
   `diff base tip` equality with that commit's patch or resulting-tree
   equality of the delivery paths (mode and object ID); or every path
   changed in the range has the same tree entry there as at the tip
   (mode and object ID, not blob ID alone). A patch-id hit alone is not
   proof: `git patch-id` ignores whitespace. A mode-only change such as
   `100644` to `100755` keeps the blob ID and must still fail the
   predicate until that tree entry lands. An empty diff matches nothing.
   Containment is path-literal. A rename still promotes when the landing
   is an ancestor or an equal combined diff.

4. **Assessment.** The append also requires an attestation whose `base_sha`
   and `head_sha` equal the recorded delivery, or a post-delivery ADR-0016
   Release Override bound to those same `base` and `tip` SHAs. Today's
   `arm dag override-release` records only `to=verified`,
   `skipped_validate_gate`, and a rationale
   (`cmd/armature/dag_override_release.go`); that plan-time op does not
   waive the promotion assessment. An unrelated planning override never
   unlocks promotion. The post-delivery override still needs a controlling
   terminal, an interactive type-the-id, and a recorded reason; the result
   is never green. An ordinary `arm decision` (including topic
   `missing-assessment` / choice `waive`) does not unlock promotion. One
   matching attestation or post-delivery override is enough. The rating
   is not read. The git check has no waiver. `--force` remains the
   hook-log override only.

   This is a presence-only narrowing of ADR-0005 and ADR-0008: delivery
   waits on a matching assessment record (or the post-delivery Release
   Override bound to the recorded SHAs), not on the review rating. Hooks
   still neither initiate review nor gate on results.

5. **Writers.** One function. `arm sync` runs it for every `done` leaf.
   `arm merged --issue` runs it for one. Both append only on a pass, and the
   op stores the target SHA, the combined patch-id, and the matched commit.
   `--pr` may be stored and is not evidence. The delivery ref is deleted
   after that append. The worktree is removed only after that append.
   `arm transition --to merged` refuses. The post-merge hook calls the same
   function, prints the same recovery lines, and exits 0. `--dry-run` writes
   nothing and uses the same exit rule. `arm sync` defaults `--into` to the
   recorded integration branch.

6. **Not checked.** No new status. A check that runs appends a
   `promotion-check` op, including a negative result. The same target SHA,
   tip, and result are not appended twice. No such op means not checked.
   Exit non-zero only when a leaf's delivery is on the target and the
   assessment is missing, or when a leaf has a delivery record and the check
   cannot run. A legacy `done` with no record is listed and does not change
   the exit code. It is not auto-promoted.

7. **Agent output.** A refusal is ADR 0020. `next_actions[0]` is the
   recovery argv with the issue id filled in. `done` without a snapshot uses
   `TRANSITION-1` and
   `arm delivery record --issue <id> --base <sha> --tip <sha>`.
   `<sha>` stays a placeholder. `arm delivery record` failures use a new
   ledger code `DELIVERY-1`. A completed `arm sync` or `arm show` is an
   ADR 0017 envelope even when the exit code is non-zero. `help[0]` is the
   most actionable command. Each blocked row carries that row's command in
   `next_action`. A load failure is the command's existing `*-1` code and
   replaces the envelope.

8. **History.** Issues already `merged` stay put. `arm doctor` check **D13**
   warns when a `merged` issue has no stored delivery match. It does not
   demote. D13 is reserved for `LNGHZN-S11-T4`.

## Consequences

- Squash of one range, merge commits, rebase plus fast-forward, and a stack
  squash whose blobs contain the range all promote. A squash that rewrites a
  delivered file, a partial cherry-pick, an empty diff, and a rename that is
  only contained inside a larger squash stay `done`.
- `LNGHZN` is a derived rollup. An open child returns it to the status it
  held before rollup until that child is `merged`.
- Follow-up is `LNGHZN-S11`. This ADR is the source. T1 (record the
  delivery) binds a manual record to branch/claim provenance and fails
  closed without it. T2 (promote one issue) verifies exact content after
  a patch-id hit and requires a post-delivery override whose payload
  names the recorded `base`/`tip`; the existing plan-time
  `override-release` payload cannot express that. Do not invent new
  issue IDs.
