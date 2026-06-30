# Parallel Stories Touching Same Files Silently Reverted Each Other's Work

**Date:** 2026-06-27  
**Writer:** 5207ee28 (coordinator)  
**Area:** coordination  
**Task:** Completing ARCHIMP-S16 / integrating with ARCHIMP-S17

## What the agent was trying to do

Coordinate two parallel stories (S16: FileReader injection, S17: Hook.Evaluate) that both touched `assemble.go`, `render_context.go`, `context_history.go`, and `harness_context.go`.

## What happened

S17 was dispatched while S16 was still in-progress, both branching from the same base commit. S17's implementation reversed S16's `Assemble()` API change (from FileReader injection back to `stateDir`/`*dag.Graph` params) because the S17 worker started from a state where S16 hadn't landed yet.

`arm doctor` reported no issues. `arm validate` reported no issues. Neither tool detected the semantic conflict. The reversion was only visible by running `git diff feat/ARCHIMP-S16..feat/ARCHIMP-S17 -- internal/context/assemble.go`.

When merging S17 into S16, git reported "automatic merge went well" — no conflict markers. This is because git's line-level merge saw S16's FileReader changes and S17's harness hook changes as touching different lines. The semantic conflict (S17 reverting S16's API) was not visible to git.

## How it changed behavior

- S17 was marked `merged` and a PR was opened with code that silently undid S16's architectural work
- The merged HEAD (`feat/ARCHIMP-S17`) was in an inconsistent state: `Assemble()` had old signature, but `filereader.go` was not present on that branch anyway
- The coordinator had to manually audit both branches and determine the "correct" API before integration

## Evidence

`git diff feat/ARCHIMP-S16..feat/ARCHIMP-S17 -- internal/context/assemble.go` showed S17 adding back `stateDir string` and `graph *dag.Graph` params and removing `reader FileReader` — a full reversion of S16-T2's core change.

## What would have helped

- If parallel stories share scope files, `arm validate` or `arm ready` could warn: "ARCHIMP-S16-T2 and ARCHIMP-S17-T1 both scope `internal/context/assemble.go` — verify integration order"
- The coordinator skill should note: when merging parallel branches that touch the same files, `git diff A..B` on each changed file is required even when git reports no merge conflicts
- A dependency (`blocked_by`) from S17 on S16 would have prevented this; the DAG decomposition should have caught the ordering requirement
