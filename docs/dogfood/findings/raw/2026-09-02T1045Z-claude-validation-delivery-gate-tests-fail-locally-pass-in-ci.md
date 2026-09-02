---
date: 2026-09-02
agent: claude
writer: claude
area: validation
task: PR #127 review remediation
tags: [make-check, delivery-gate, local-vs-ci, commit-reference, test-environment]
---

# `make check` fails 5 delivery-gate tests locally on a clean tree while CI passes the same commit

## User Goal

Run the documented repo gate (`make check`) before pushing a change to
`.agents/skills/verify-armature/scripts/arm-verify.sh` — a shell-only
change that touches no Go code.

## Observed

`make check` failed with 5 test failures, all in `cmd/armature`, all
delivery-gate related:

```
- TestDeliveryGateSurvivesWorktreeRecreation_REQ_LNGHZN_S4_T1
- TestDeliveryGateSurvivesRebaseOntoUpdatedParent_REQ_LNGHZN_S4_T1
- TestDeliveryGateFallsBackWhenParentBranchConfigIsLiteralHEAD_REQ_LNGHZN_S4_T2
- TestTransitionDoneRepoNotBoundToIssueFailsClosed_REQ_LNGHZN_S4_T2
- TestTransitionDoneFromWorktreeSubdirectory_REQ_LNGHZN_S4
```

Each with the same cause:

```
[TRANSITION-1] delivery gate check failed:
  1. CommitReference: No commits found matching conventional-commit
     format [type](<ISSUE-ID>): ... since <sha>
Use --skip-delivery-gate to override (audit trail will record the override)
```

`Summary: 49 packages passed, 1 packages failed`.

These tests build their own temp repos and make their own commits, so the
commits they are asserting on are ones they just created.

## Impact

Cost a decision I should not have had to make: is my change responsible?
I resolved it by stashing (`git stash -u`), re-running one failing test on
the pristine tree, and getting the identical failure — so pre-existing —
and by confirming CI's `check` job passes on the same commit.

That is ~5 minutes and a stash/pop cycle on every gate run. Worse, it
trains the habit of treating a red `make check` as noise. For the
remaining 21 commits of the session I skipped the gate entirely and said
so, because re-running an 8-minute gate that is known-red on a shell-only
change buys nothing. A gate that is red for environmental reasons is a
gate that stops being read.

## Evidence

- `make check` → `make: *** [Makefile:63: coverage] Error 1`, failure list
  above.
- Pre-existing confirmed: `git stash -u && go test ./cmd/armature -run
  'TestTransitionDoneFromWorktreeSubdirectory_REQ_LNGHZN_S4$' -count=1`
  → same `CommitReference` failure with my changes stashed.
- CI green on the same tree: run `33611458678` — `check` pass (7m18s),
  `e2eharness` pass, `validate-graph` pass, on head `34aeea5d`.
- Host: WSL2, `git version` from the distro, tests run unsandboxed.

## Suggested Follow-Up

Root-cause not established — recording the divergence, not a diagnosis.
Worth checking whether these tests inherit something from the developer's
environment that CI's fresh container does not have. Candidates worth
ruling out: this repo sets `core.hooksPath` to an **absolute** path
(`/home/brian/development/armature/.git/hooks`), and at the end of this
session that directory no longer existed — so hook behaviour here is not
what a fresh clone gets. If the tests can be affected by ambient git
config, they should pin it (`GIT_CONFIG_GLOBAL=/dev/null`,
`core.hooksPath=`) so local and CI agree.
