# _REQ_ test-naming convention conflicts with superpowers plan test names

- **Writer:** claude (planner, MIGH story load)
- **Area:** workflow

## What I was trying to do

Write acceptance criteria for tasks loaded from a superpowers implementation plan (docs/superpowers/plans/2026-07-02-migration-path-hardening.md).

## What happened

The armature-planner skill mandates naming new tests `Test<Description>_REQ_<RequirementID>` so `make trace-report` picks them up. But the source plan prescribes exact test code with its own naming convention (`_P1`/`_P2` priority suffixes, e.g. `TestMigrationInvariantMatrix_P1`). Following the plan verbatim (the authoritative source) means every acceptance criterion skips traceability; renaming means the worker's diff deviates from the cited plan text.

## Impact

I followed the plan's names, so `make trace-report` will show the MIGH requirements as untagged. Any story planned via superpowers:writing-plans will hit the same conflict until one convention absorbs the other (e.g. `_P1_REQ_MIGH_T6`, or trace-report learning the plan-file linkage).

## Evidence

MIGH-T1..T7 acceptance criteria vs armature-planner SKILL.md §"Spec traceability", 2026-07-02.
