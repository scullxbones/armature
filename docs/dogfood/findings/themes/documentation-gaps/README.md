# Documentation Gaps

Skills and tools describe task-level examples or semantics but omit the envelope or wrapper structure, forcing users to infer or discover the correct format via trial-and-error.

## Findings

- [armature-planner: missing plan JSON wrapper format](../../raw/2026-06-22-documentation-plan-json-wrapper-format.md) — The "Writing Good Plan JSON" section shows a complete task-level example but never wraps it in the required `{ "version": 1, "issues": [...] }` envelope. User had to run `arm decompose-apply --example` to discover the format after a failed attempt.

## Pattern

Documentation that teaches a structure often focuses on the most complex/interesting part (individual tasks) and assumes the wrapper is obvious. When the wrapper is non-trivial or unexpected (versioning, specific key names), users must guess or rediscover via error messages and example commands.

## Impact

- Extra rework and tool invocations to discover the correct format.
- Momentary loss of confidence when error messages don't hint at the solution.

## Mitigation

Always show the minimal complete wrapper example alongside or immediately before the first detailed task-level example. Include a note pointing to the example command (`arm decompose-apply --example`) in the section intro.
