---
area: coordination
writer: 5207ee28
date: 2026-08-14T23:38Z
story: LNGHZN-S10
---

# A story made an acceptance rule normative in an earlier task than the one implementing its evidence mechanism

## What the agent-user was trying to do

Coordinate `LNGHZN-S10` (gate efficiency). Dispatch `LNGHZN-S10-T1` — the
text-only task that makes spec decisions D1/D4/D5/D6 normative in
`docs/agents/workflow.md` and the three embedded skills — then run the
mandated semantic review before promoting it past the wave gate.

## What happened

The worker delivered cleanly: all four decisions made normative with
MUST/MUST NOT language, scope exactly the four contracted files, no drift,
`make validate-skills` green. The reviewer confirmed all of that.

The reviewer still rated the delivery **red**, failing both acceptance
criteria, for a single reason: the review bundle contained no gate-evidence op,
so there was no citable proof (`exit=0`, `profile=full`, SHA equal to bundle
head) that the gates were green at head.

That is the D4 acceptance rule — which `T1` itself had just written into
`internal/skillsembed/skills/armature-reviewer/SKILL.md`. The reviewer applied
the new rule correctly, and correctly declined to soften the verdict to
`indeterminate`, because the rule it had just been handed says a missing
evidence op means "rerun required, never indeterminate".

But the mechanism that emits an evidence op — `arm gate run`, the evidence op
itself, and its inclusion in the ReviewBundle — is `LNGHZN-S10-T3`, which had
not been implemented. `T3` was in fact blocked behind `LNGHZN-S7-T1` at the
time.

So `T1` failed a rule it authored, against tooling that does not yet exist.

## How it changed behavior, confidence, or time spent

The failure is unremediable by construction. A rerun cannot produce an evidence
op when no code can write one, so the normal remediation loop would have burned
its full 3-cycle cap without any possible change in verdict. Recognizing that
and escalating to the human (I7) rather than iterating was the only terminating
path, and it cost a full review cycle plus the escalation round-trip to
discover.

The blast radius is not limited to `T1`: **every** task reviewed between `T1`
landing and `T3` landing auto-reds on the same two criteria, regardless of
delivery quality. A red rating that is guaranteed independent of the work is
worse than no rating — it trains the coordinator to discount reviewer verdicts
exactly when they should be load-bearing.

The human accepted `T1` with the red on record and deferred re-review until
`T3` lands.

## What would have helped

The story's own D8 (vertical-slice planning validation) is the closest existing
rule, but it only catches scope-level coupling — one task touching a censused
surface while another owns its census file. This is the *temporal* version of
the same defect: task A makes a rule normative, task B implements the mechanism
that rule depends on, and nothing in plan-release validation notices that A
without B is unsatisfiable.

Candidate remedies, roughly in order of cheapness:

- **Plan-release check**: flag when a task introduces normative acceptance
  language referencing a capability whose implementing task is not among its
  blockers. `T1` should have been `blocked_by: T3`, or the two should have been
  one vertical slice.
- **Bootstrap escape hatch**: let a normative rule declare the commit or task
  from which it becomes enforceable, so a reviewer can tell "rule not yet in
  force" from "rule violated". This is distinct from `indeterminate` and should
  not reuse that status.
- **Reviewer input**: give the reviewer the story DAG, not just the bundle, so
  it can recognize that the evidence mechanism it is citing is itself pending
  work in the same story and say so explicitly rather than reporting a bare
  `missing_evidence`.

The general shape worth naming: *a task that writes a rule and a task that
makes the rule satisfiable belong in the same slice, or in dependency order —
never in parallel.*
