# Architecture Deepening Follow-Up PRD

Date: 2026-06-13

## Problem Statement

Armature's live codebase still has three places where implementation knowledge
is spread across too many callers.

First, command and TUI flows repeatedly reconstruct current task truth by
loading validated ops, materializing state, reopening index data, and then
reloading issue state. The interface is shallow because each caller must know
too much about the implementation.

Second, graph knowledge is not yet fully concentrated behind one seam. Some
graph facts already live in the graph projection, but hierarchy legality,
dependency-cycle reasoning, descendant filtering, and ancestry-oriented working
memory assembly still leak across multiple modules.

Third, validated op evidence is not the only log-reading seam. Some callers
still read raw logs or rebuild location tracking by hand, which weakens the
filename-worker-ID invariant and makes structural health checks diverge from the
same truth used by materialization.

From the user's perspective, this creates avoidable architecture drift:

- the ready queue, `doctor`, and render-context can disagree for avoidable
  reasons
- bug fixes land in multiple places instead of one
- tests have to cross lower seams than the callers actually use
- future architecture work reopens the same implementation details instead of
  standing on a deeper module

## Solution

Deepen three related seams in the live Armature architecture:

1. Create a `RepoSnapshot` module that owns validated op loading,
   materialization, warnings, and current task truth for command and TUI
   callers.
2. Finish the `GraphFacts` seam so pure graph knowledge derived from
   materialized task truth has one canonical owner.
3. Make validated op evidence the only log-reading seam for runtime and health
   consumers that need offsets, warnings, or source locations.

The result should be higher leverage for callers and stronger locality for
maintainers: one module to load current truth, one module to answer graph
questions, and one module to explain which ops counted and why.

## User Stories

1. As an Armature maintainer, I want one repo snapshot interface, so that I can fix task-truth loading once instead of across many commands.
2. As an Armature maintainer, I want ready queue and `doctor` flows to consume the same validated op evidence, so that they cannot silently diverge.
3. As an Armature maintainer, I want graph legality and traversal facts to live behind one seam, so that future graph changes do not require scattered edits.
4. As an Armature maintainer, I want render-context to ask for graph facts instead of walking parent and blocker relationships by hand, so that working memory stays truthful.
5. As an Armature maintainer, I want invalid cross-worker ops excluded through one interface, so that every consumer preserves the same filename-worker-ID invariant.
6. As an Armature maintainer, I want warning emission for rejected ops to be centralized, so that callers do not invent incompatible warning behavior.
7. As an Armature maintainer, I want byte offsets and source locations to come from one module, so that checkpoint and evidence behavior stay aligned.
8. As an Armature planner, I want source-backed architecture work to land on stable seams, so that later decomposition can target real modules instead of implementation noise.
9. As an Armature coordinator, I want ready-task ordering to depend on canonical graph facts, so that wave planning uses the same task truth as validation.
10. As an Armature worker, I want `arm render-context` to reflect the same current state that `arm ready` and `arm show` see, so that I do not work from stale or partial truth.
11. As an Armature auditor, I want `arm doctor` to use the same validated log evidence as materialization, so that structural health findings are credible.
12. As an Armature maintainer, I want tests to cross the same seam as production callers, so that they validate behavior instead of implementation detail.
13. As an Armature maintainer, I want new command surfaces to depend on deep modules by default, so that architecture debt does not regrow with every new subcommand.
14. As an Armature maintainer, I want graph-facts ownership to stay pure, so that queue policy and human-facing wording do not leak into the graph module.
15. As an Armature maintainer, I want follow-on work to preserve prior `ARCHIMP` completions as prior art, so that completed stories are not reopened informally.
16. As an Armature maintainer, I want repo snapshot loading to expose the current materialized truth in one step, so that future context, queue, and release work starts from the same interface.
17. As an Armature maintainer, I want the validated op evidence seam to serve both structural checks and live runtime consumers, so that evidence and execution share one source of truth.
18. As an Armature maintainer, I want this follow-up scoped to the top three remaining findings, so that the work stays focused on the highest-leverage deepening opportunities.

## Implementation Decisions

- The PRD covers three follow-on stories only: repo snapshot loading, graph-facts completion, and validated op evidence consolidation.
- `RepoSnapshot` is the highest new seam in this slice. It should own validated op loading, materialization, warning collection, and current task truth for command and TUI consumers.
- `RepoSnapshot` should expose materialized state, index view, and issue lookup through one interface so callers do not need to understand checkpoint or reload behavior.
- `GraphFacts` remains a pure in-process module derived from materialized task truth. It should own ancestry, descendants, depth, blockers, dependency cycles, and hierarchy legality facts.
- `GraphFacts` must not absorb queue-specific DTOs, claim-TTL policy, or human-facing validation wording. Those stay in consumer modules.
- Validated op evidence becomes the only supported seam for consumers that need accepted ops, offsets, warnings, or source locations.
- Structural health checks should consume validated op evidence rather than raw log parsing so that `doctor` and materialization agree on which ops count.
- Working memory assembly should consume repo snapshot and graph facts rather than reopening issue files opportunistically from disk.
- Existing completed architecture stories remain prior art, not extension points. This PRD must not reopen completed `ARCHIMP` work as if it were unfinished.
- The issue tracker publication for this PRD should be a cited, verified story under the existing `ARCHIMP` epic so the planner can decompose it later without citation remediation.
- Context for later decomposition should remain source-backed by this PRD plus the existing architecture document and accepted ADRs already in the repo.

## Testing Decisions

- Good tests must cross the highest seam the caller uses and assert external behavior, not internal helper structure.
- `RepoSnapshot` tests should prove that commands and TUI callers receive the same current task truth, warnings, and accepted-op behavior from one interface.
- `GraphFacts` tests should prove graph behavior such as ancestry, descendants, dependency cycles, and hierarchy legality through the graph-facts interface rather than through ad-hoc consumer helpers.
- Validated op evidence tests should prove that mismatched or corrupt ops are excluded consistently while offsets and source locations remain available to callers that need them.
- Command-level regression tests should focus on observable agreement between `arm ready`, `arm render-context`, `arm show`, `arm validate`, and `arm doctor`.
- Prior art for the tests already exists in the repo: validated opstream integration tests, graph projection tests, ready queue tests, validation tests, and render-context assembly tests.
- The final verification gate for implementation work derived from this PRD should remain `go test ./...`, `go run ./cmd/armature validate --ci`, and `go run ./cmd/armature doctor`.

## Out of Scope

- Reopening or rewriting previously completed `ARCHIMP-S2`, `ARCHIMP-S3`, or `ARCHIMP-S4` work.
- The speculative task-context projection beyond what is required by repo snapshot and graph-facts adoption.
- Harness-hook truth recovery, platform revalidation, or dogfood evidence gathering.
- Broad documentation cleanup outside the immediately affected architecture surfaces.
- Ready queue policy redesign, claim policy redesign, or orchestration lifecycle redesign.
- UI-only changes that do not improve the architecture seams described here.

## Further Notes

- This PRD is intentionally anchored to the live codebase, not the earlier architecture-review artifact. It excludes findings that have already landed, such as the `context_files` lifecycle story.
- `ARCHIMP` already exists and contains completed prior stories. This PRD is for the next slice of deepening opportunities that remain after those completions.
- The repo's own domain language should be preserved during decomposition: task truth comes from materialized Armature state, graph facts come from the task DAG, and working memory remains the rendered context for an orchestration run.
