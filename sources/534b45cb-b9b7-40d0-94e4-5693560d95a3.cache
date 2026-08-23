# Theme: Scope-Overlap Validation Gaps

## Summary

`arm validate` and `arm claim` warn/error on scope overlap using only *direct* `blocked_by` edges and literal parent/child scope containment — never transitive ordering, never cross-story dependency edges, and never "file created by an upstream task" awareness. This produces both false positives (forcing busywork to silence warnings that are already safe) and false negatives (real unordered overlaps across story boundaries pass silently).

## Evidence

- [`arm claim` flags scope overlap against the parent story itself, every time](../../raw/2026-06-30T2200Z-claude-workflow-story-level-scope-overlap-false-positive.md) — All 11 DF-S5 task claims failed with "scope overlap with DF-S5" — the overlap was against the parent story record, not another task, for tasks touching entirely disjoint files.
- [`arm claim` flags scope overlap with the task's own parent story](../../raw/2026-07-05T2226Z-claude-workflow-claim-scope-overlap-with-parent-story.md) — Same false positive recurring on ARCHIMP-S18: every claim after the first needed `--force`, training the coordinator to reach for `--force` habitually.
- [Transitive dependencies don't suppress scope-overlap warnings](../../raw/2026-07-02T0000Z-claude-validation-transitive-deps-dont-suppress-scope-overlap.md) — 8 scope-overlap WARNINGs fired for task pairs already strictly ordered through a transitive `blocked_by` chain (e.g. T3 vs T5, ordered via T4). Only direct edges suppress the warning; 8 redundant direct edges had to be added purely to silence noise.
- [`arm validate` scope-overlap warnings ignore transitive `blocked_by` ordering](../../raw/2026-07-03T1500Z-claude-validation-scope-overlap-ignores-transitive-deps.md) — Same gap recurring on HOOKBIND: T5/T1, T3/T1, T4/T1 all warned despite being transitively ordered through the linear chain.
- [Cross-story scope overlaps do not warn while same-story overlaps do](../../raw/2026-07-03T1600Z-claude-validation-cross-story-scope-overlap-silent.md) — EXECEV-T1 and HOOKBIND-T1 shared scope (`harness_hook.go`) with no ordering edge between them and no warning at all — the validator apparently only compares scope within a single story's task set.
- [Phantom-scope INFO fires for files a blocking task creates](../../raw/2026-07-02T0000Z-claude-validation-phantom-scope-for-file-created-by-earlier-task.md) — A downstream task's scope legitimately includes a file an upstream blocking task marks `(new)`; validator checks scope against the filesystem at plan time and doesn't know the file will exist once the blocker runs.
- [Scope-overlap matches on directory prefix, so unrelated files in the same tree collide](../../raw/2026-08-14T2352Z-5207ee28-tooling-scope-overlap-matches-on-directory-not-file.md) — T2 and S7-T1 shared no file; claim still refused. Matcher treated a directory as overlapping every path under it.
- [`arm claim` blocks on an in-progress story's aggregate scope, though no worker holds it](../../raw/2026-08-15T0018Z-5207ee28-tooling-claim-treats-in-progress-story-as-a-competing-worker.md) — T3 blocked on story LNGHZN-S7 after the only overlapping *task* had already merged. The story record is not a claimant.
- [A cited create landed five cross-story W1 overlaps; the planner became janitor](../../raw/2026-08-17T0131Z-5207ee28-validation-create-conscripts-planner-as-janitor.md) — `arm create` succeeded; the next `validate` printed five W1s against foreign open tasks. Planner then amended scope and added `blocked_by` edges *onto T12 from other stories* so the queue would look clean.

## Pattern

The scope-overlap checker in both `arm validate` and `arm claim` reasons about two tasks' file scopes without reasoning about the DAG they're embedded in:

1. **No transitive closure**: only direct `blocked_by` edges suppress the warning, so any linear or diamond-shaped dependency chain longer than one hop produces warnings for already-ordered pairs.
2. **Parent/child scope containment misread as overlap**: a story's scope is the union of its children's scope by design; the checker doesn't special-case this and treats it as two competing claims.
3. **No cross-story awareness**: the same file scoped in two different stories with no ordering edge produces no warning, which is the most dangerous case (a real potential conflict) — silent where the same-story case would be noisy.
4. **No plan-time awareness of "will exist" files**: a task's `(new)` file annotation isn't cross-referenced against downstream tasks' scope declarations of the same path.
5. **Directory-prefix matching**: a scope entry that is a directory collides with every file under it, including files no task named.
6. **Create is fail-open on W1**: the write that introduces cross-story overlap succeeds; the planner inherits janitor work on stories they do not own.

## Impact

- Coordinators either add redundant direct edges purely to silence WARNINGs (extra `arm link` calls with no semantic value), or reach for `--force` on every single claim after the first — habituating an override that would otherwise be a real safety signal.
- The most dangerous case (unordered cross-story file overlap) is the one that produces no warning at all.
- Repeated across at least 4 distinct stories (DF-S5, MIGH, HOOKBIND, EXECEV, ARCHIMP-S18) — this is now the single most frequently reported planning-time friction.

## Candidate Follow-Ups

- Compute the transitive closure of `blocked_by` before flagging scope overlap, not just direct edges.
- Special-case story-vs-child-task scope containment: a task's scope overlapping its own parent story's declared scope should never warn.
- Extend scope-overlap checking across story boundaries — this is the highest-value fix since it's the direction that's currently silent. Pair it with write-time refusal (T12's contract): do not let `arm create` land a W1 the planner then has to clean by reparenting foreign tasks.
- Cross-reference `(new)` file annotations from blocking tasks against scope declarations of blocked tasks before emitting phantom-scope INFO.
- Match overlap on files/globs, not containing directory (S10-T6's contract; still biting T2 vs S7).
- Do not treat an in-progress *story* as a competing claimant once its overlapping children are merged.
