# Documentation Gaps

Skills and tools describe task-level examples or semantics but omit the envelope or wrapper structure, forcing users to infer or discover the correct format via trial-and-error.

## Findings

- [armature-planner: missing plan JSON wrapper format](../../raw/2026-06-22-documentation-plan-json-wrapper-format.md) — The "Writing Good Plan JSON" section shows a complete task-level example but never wraps it in the required `{ "version": 1, "issues": [...] }` envelope. User had to run `arm decompose-apply --example` to discover the format after a failed attempt.
- [`arm claim --worktree` flag omitted from coordinator skill examples](../../raw/2026-06-28T2200Z-claude-commands-arm-claim-worktree-required.md) — Coordinator skill step 4 shows `arm claim TASK-ID --ttl <minutes>` without `--worktree`, which is mandatory. Every wave dispatch fails until the coordinator reads `arm claim --help`.
- [`arm validate --strict` promotes "broad scope" to error but auditor skill only mentions scope overlap](../../raw/2026-06-28T2200Z-claude-validation-strict-broad-scope-context-files.md) — Auditor step 4 frames `--strict` as checking scope overlap between tasks. It also upgrades "missing context_files on broad scope" warnings to errors — a different category requiring `arm amend --context-file` fixes. Required amending 5 pre-existing issues before story could pass audit.
- [Planner rationalized `arm validate` broad-scope warnings as noise](../../raw/2026-06-28T2043Z-claude-workflow-validate-warnings-on-plan-load.md) — When `arm validate --ci` emitted 6 `context_files` WARNINGs, the planner dismissed them without inspecting actual task scope. T5 genuinely spanned 3 domain modules (ops + materialize + output) and should have been split. The planner skill's "Common Failure Modes" table doesn't mention context_files warnings as a decomposition signal, leaving the warning text ("split the task") as the only cue — easily rationalized away.
- [`arm review record --bundle` expects a file path but coordinator skill passes JSON content](../../raw/2026-06-29T0002Z-5207ee28-documentation-review-bundle-path-vs-content.md) — The coordinator SKILL.md shows `--bundle "$REVIEW_BUNDLE"` where `$REVIEW_BUNDLE` is the stdout JSON from `arm review prepare`. The CLI calls `os.ReadFile()` on the value, so passing JSON content fails with a cryptic file-not-found error. The `--output <file>` flag is the correct approach but is not documented in the coordinator workflow example.
- [Coordinator skill shell pseudocode used `feat` type prefix only; silently dropped other commit types](../../raw/2026-06-29T1823Z-5207ee28-documentation-coordinator-grep-type-prefix-gap.md) — A `--grep="feat($TASK_ID):"` pattern prevented prefix collisions but hardcoded the `feat` commit type. Workers using `fix`, `refactor`, `test`, or `docs` prefixes were silently dropped from review, making the HEAD-fallback guard permanently unreachable. `make test` passed throughout because the skill is shell pseudocode — semantic correctness required an opus-level review to surface.

## Pattern

Two recurring sub-patterns have emerged:

1. **Missing envelope / required flag**: Documentation shows the interesting part (a task body, a command) but omits a required wrapper or flag (the JSON envelope, `--worktree`, `--output`). The user discovers the gap via a cryptic error and must read source or `--help` to recover.

2. **Warning signal not surfaced as actionable**: Validation output emits warnings that are documented vaguely (or not at all) in the skill, so agents rationalize them away. When `--strict` is added later, those warnings become blocking errors. Shell pseudocode in skill markdown has the same property: `make test` cannot catch logic gaps, so semantic errors survive until a high-capability review pass.

## Impact

- Extra rework and tool invocations to discover the correct format.
- Momentary loss of confidence when error messages don't hint at the solution.
- Warnings dismissed during planning become blocking errors at audit time, requiring `arm amend` calls across multiple issues.
- Skill pseudocode logic gaps (wrong regex patterns, unreachable guards) survive the test suite and ship until a semantic review catches them.

## Mitigation

- Show the minimal complete invocation (wrapper, all required flags) before the detailed example. Add a note pointing to `--help` and example commands.
- In planner/coordinator skills, add a "Common Failure Modes" entry for `context_files` WARNINGs: treat each as a scope decomposition prompt, not presentation noise.
- For skill sections that include shell pseudocode, add a note that `make test` does not validate shell logic; a semantic review pass is required before shipping.
- Fix the `arm review prepare` / `arm review record` example in coordinator SKILL.md to use `--output <file>` and pass the file path to `--bundle`.
