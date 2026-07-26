# Conventions

This document defines the naming and formatting conventions that all workers must follow when implementing tasks in this repository. Test naming is surfaced by `make trace-report` (report-only, not a `make check` dependency); commit format and branch naming are checked by review, not enforced by tooling today. These conventions are load-bearing for traceability and integration regardless of enforcement mechanism.

## Test Naming and Traceability

Test functions that verify acceptance criteria must follow the naming convention:

```
func Test<Description>_REQ_<ISSUE-ID>(t *testing.T)
```

Where `<ISSUE-ID>` is the task or story ID (e.g., `ARCHIMP-S14-T6`, `TOPTIER-S2-T3`). This pattern makes the test visible to `make trace-report` and ties it back to the requirement that motivated it.

**Examples:**
- `TestHandlersDoNotReloadStateDirectly_REQ_ARCHIMP_S14_T6`
- `TestMergedFailsOnViolations_REQ_HOOKBIND_T4`
- `TestSkillLint_REQ_TOPTIER_S1_T1`

The `_REQ_<ISSUE-ID>` suffix is mandatory for any test that verifies an acceptance criterion from the task definition of done. Tests that verify general correctness or regression testing do not require the suffix, but doing so will make them visible in trace reports and is encouraged.

See [armature-worker SKILL.md](../internal/skillsembed/skills/armature-worker/SKILL.md#test-naming-and-traceability) for additional context on test naming.

## Commit Format

All commits must follow the conventional commit format with an issue-ID scope:

```
<type>(<ISSUE-ID>): <description>
```

**Valid types:**
- `feat` — new feature
- `fix` — bug fix
- `refactor` — code structure change without feature/fix
- `test` — test coverage additions or fixes
- `docs` — documentation-only changes
- `style` — formatting and lint changes
- `polish` — minor UX improvements, comments, or non-essential changes

**Merge commits** use a special format:
```
merge: <ISSUE-ID> <description>
```

**Examples:**
- `feat(TOPTIER-S2-T2): Embed CLI-generated examples into skills at build time`
- `fix(NXTTN-S2): validate link --rel before writing the op, not only at replay`
- `test(ARCHIMP-S14): add round-trip JSON serialization tests`
- `docs(TOPTIER-S2-T3): document naming and commit conventions`
- `merge: TOPTIER-S2-T2 embed CLI-generated examples`

The scope (the `<ISSUE-ID>` in parentheses) is mandatory for all commits except merge commits, which use the alternate format above. The description should be concise and in imperative mood (use "add" not "adds", "fix" not "fixed").

**Per-task commits:**
Each task must be completed with a single focused commit. Do not leave partial work uncommitted. Stage the scoped files and commit with the format above *before* running `arm transition ISSUE-ID --to done` — the delivery gate's Clean Tree and Commit Reference checks require a clean worktree and an existing conventional commit at transition time, not the other way around.

## Branch Naming

Branch names follow these conventions:

**Feature branches:**
```
feat/<ISSUE-ID>
```
Used for story-level feature development. Created by the Coordinator before dispatching workers.

**Task branches:**
```
task/<ISSUE-ID>
```
Used by workers for isolated task implementation. Created in a temporary worktree by the Coordinator's worker dispatch. Workers commit to these branches and the Coordinator merges them back to the feature branch.

**Operations branches:**
```
_armature
```
Reserved for armature's internal state (ops). Do not commit to this branch directly; ops are managed by `arm` commands and automatically persisted.

**Examples:**
- `feat/ARCHIMP-S14` — story branch
- `feat/TOPTIER-S2` — story branch for second top-tier item
- `task/TOPTIER-S2-T3` — task branch for the conventions task
- `task/ARCHIMP-S14-T6` — task branch for a specific acceptance criterion

## Rationale

These conventions serve three purposes:

1. **Traceability:** The `_REQ_<ISSUE-ID>` suffix on test names and `<ISSUE-ID>` in commit scopes makes the connection between code and requirements explicit and machine-readable. `make trace-report` uses these patterns to build a coverage report.

2. **Auditability:** Commit scopes make it clear which task or story each change belongs to, enabling quick navigation between commits and issues, and making blame/bisect investigations more tractable.

3. **Workflow isolation:** Branch naming separates feature-level work (one shared `feat/` branch) from task-level work (one `task/` branch per worker), enabling parallel execution while maintaining merge-conflict-free DAG integrity.

Workers that violate these conventions will fail review. Enforce these in code review—they are not suggestions.
