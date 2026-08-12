# Theme: Test Coverage Gaps — False Green Test Suites

## Summary

Tests pass while critical end-to-end contracts are broken. Two recurring causes: (1) test fixtures are constructed as Go structs and never exercise the JSON serialization path a real user would follow; (2) eval corpus data is duplicated across multiple files and inline fixtures, so tests are self-referentially consistent even when diverged from the canonical source.

## Evidence

- [JSON string/int mismatch between skill docs and Go types hidden by struct-based tests](../../raw/2026-06-28T2200Z-claude-workflow-json-string-int-mismatch-hidden-by-tests.md) — `CriterionStatus`/`Rating` marshaled as integers but skill instructed string values. Every test constructed Go structs — the mismatch was never exercised. The end-to-end reviewer→`arm review record` loop was non-functional with a real reviewer agent while all tests were green. Caught only by opus code review reading the actual usage contract.
- [Eval corpus divergence — four copies, no single source of truth](../../raw/2026-06-29T0001Z-5207ee28-validation-eval-corpus-divergence.md) — A single logical change (case-008 `expected_rating: yellow → red`) required edits in four locations: `cases.json`, `reviewer_eval_results.json`, and two inline `setUp()` fixtures in `test_reviewer_eval_report.py`. Python tests passed throughout because the inline fixtures were internally consistent with each other, not with the canonical source. The divergence was invisible to CI until manually inspected.
- [Removing a config field silently defanged a shell-script safety check](../../raw/2026-07-01T0912Z-claude-tooling-config-mode-removal-left-dead-shell-check.md) — Installed git hook shell templates gated real behavior on `grep -q '"mode".*"dual-branch"' .armature/config.json`. Removing the `Mode` field (SB-ELIM) left the grep permanently non-matching, silently disabling the safety check it implemented — no test exercised the installed hook scripts themselves, only the Go code that generates them.
- [make build/lint/test all passing did not catch a P0 regression the fix itself introduced](../../raw/2026-07-01T1330Z-claude-validation-green-ci-missed-p0-regression.md) — A haiku fix for `arm doctor` on legacy repos gave it its own `PersistentPreRunE` that set `appCtx` directly instead of delegating to root's, leaving `appCtx.StateDir` empty on every ordinary (non-legacy) `arm doctor` invocation. All three make gates passed because no test exercised the normal-repo path through the new code.
- [DAG task scope left real call sites uncovered, only caught at final verification](../../raw/2026-07-01T0910Z-claude-planning-dag-scope-gaps-left-files-uncovered.md) — Two call sites using pre-refactor function signatures weren't listed in any task's scope across a 13-task DAG; every per-task wave passed its own gates, and the gap surfaced only at the story's final verification task.
- [S18 seam refactor introduced a class of silent-failure regressions, not one bug](../../raw/2026-07-05T0000Z-claude-validation-s18-silent-error-class.md) — The single reviewer-flagged bug (`verifyResults, _ := lc.VerifyAll()` discarding an error) turned out to be one instance of a repeated pattern one layer up; a broad follow-up review, not the task-scoped fix-and-gate cycle, found the rest of the class.
- [Cross-task interface drift survives per-task semantic review; only whole-story deep review caught it](../../raw/2026-07-06T1257Z-claude-workflow-cross-task-format-drift-missed-by-per-task-review.md) — Every task in a 5-task sequential story (log writer → bundle section → citation validation → skills → docs) passed its own task-scoped semantic review and `make check` (including 100% mutation efficacy), yet a final whole-story deep review found 3 critical + 10 major defects, several caused by log-format/entry-ID drift between tasks that no single task's review scope could see.

- [Unconditionally-enforcing new gate broke ~40 pre-existing tests it wasn't designed to touch](../../raw/2026-07-25T2340Z-claude-workflow-hard-enforcing-gate-broke-existing-tests.md) — The inverse instance, and it exposes the same underlying fact. Wiring the delivery gate into `arm transition --to done` broke ~40 tests across `cmd/armature`, `internal/e2eharness`, and `internal/skilltranscript` — because nearly every one transitions a fixture issue to `done` without a clean tree or a conventional-commit reference. The suite was green for years while never exercising real delivery conditions; the gate is what revealed it. Remediation meant adding `--skip-delivery-gate` to all ~40 call sites, including baking it into the shared `TestRepo.Transition` helper — and since the golden transcript feeds generated skill documentation, that risks teaching workers the ungated path.

## Pattern

Green tests and green per-task reviews create false confidence in (at least) four distinct ways:

1. **Struct-only test construction**: When every test builds domain objects via Go struct literals and never parses a JSON file authored the way a real agent or user would, the serialization contract between skill documentation and Go types is never exercised. Bugs in `MarshalJSON`/`UnmarshalJSON`, or mismatches between documented string values and integer enum representations, survive until end-to-end testing or code review.

2. **Duplicated fixture data**: When test setUp methods maintain inline copies of canonical data (eval cases, expected results), internal consistency passes even when the inline copy diverges from the canonical source. The test validates the copy against itself, not against the specification.

3. **Generated artifacts (shell scripts, templates) untested at the artifact level**: Tests exercise the Go code that produces a shell script template, but never the installed script's actual runtime behavior. A field removal that's fully covered on the Go side can silently defang logic embedded in a string template with no test coverage of its own.

4. **Task-scoped review can't see whole-story defects**: When a story is decomposed into tasks and each task's DoD/acceptance is reviewed independently against its own scope, interface drift between tasks (call-site signature mismatches, log format inconsistency, entry-ID conventions) is invisible to any single task's review. Only a review — or verification pass — with the whole story's diff in scope catches it, and by design that only happens once, at the end, after all individual gates already reported green.

5. **Existence-only assertions mask a wholly-inert feature**: When tests assert only `NotEmpty(output)` / `NotNil(jsonKey)` / "runs without error", they pass regardless of whether the feature does anything — an empty reconcile still emits non-empty text and non-nil (empty) arrays. On PR #89 this let `arm worktree list`/`gc` read issues from the wrong directory (a total no-op) survive *multiple* review rounds; each round hardened logic downstream of a reconcile that was always fed zero issues. A test that cannot fail when the feature is a no-op is not a test. See [Hollow tests let a dead worktree-lifecycle path survive multiple review rounds](../../raw/2026-08-08T1900Z-claude-validation-hollow-tests-masked-dead-worktree-path.md).

## Impact

- End-to-end workflows broken while CI is green — discovered late (during code review or manual testing) with higher remediation cost.
- Diverged eval corpus fixtures make it impossible to know from test results alone whether the evaluation logic is correct.
- Agent workers implementing skill docs and Go types independently have no forcing function to check that their outputs are compatible unless a cross-layer test exists.
- Per-task quality gates (including 100% mutation efficacy) are not a substitute for a whole-story integration pass — this has now been observed as the failure mode on multiple stories (SB-ELIM, EXECEV), each time only caught by a final/broad review rather than the per-task gate sequence.

## Candidate Follow-Ups

- For any task that both adds a new Go type and documents that type in a skill (JSON format, string values, field names): require at least one test that round-trips via a JSON fixture file, not only via Go struct construction.
- Replace inline `setUp()` fixtures in `test_reviewer_eval_report.py` with loads from the canonical `cases.json` and `reviewer_eval_results.json` files so tests cannot diverge from the source of truth.
- The armature-worker skill's Definition of Done could require a cross-layer JSON fixture test when both a type and its skill documentation are in scope.
- When a task removes a config field, require a grep across generated shell templates / embedded strings for references to that field's old value, not just Go struct usages.
- Ban existence-only assertions (`NotEmpty`/`NotNil`/"runs without error") as the sole check for a command's behavior; require at least one assertion on a specific classified outcome that would fail if the feature were a no-op (e.g. a named worktree lands in `gc_ready` and is `os.Stat`-gone after `gc`).
- Treat a whole-story deep review (not just the final verification task's own scope) as a mandatory gate before opening a PR for any multi-task DAG story — the per-task review discipline has now repeatedly missed cross-task drift that only a story-wide pass catches.
