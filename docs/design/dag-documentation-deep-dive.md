# DAG Documentation Deep Dive

## Status

Proposed for a future Armature story and a dedicated future session.

## Source

This write-up captures the documentation work intentionally deferred on 2026-06-08 while reviewing DAG and hierarchy seams in the Armature codebase.

## Goal

Run a documentation-only deep dive that audits and reconciles the canonical DAG vocabulary, hierarchy rules, and graph-behavior descriptions across Armature's design docs, command docs, validation docs, and embedded skills.

## Why This Is Separate

The immediate code follow-up story should repair only the documentation that would otherwise become directly false because of the code change.

This deeper pass is broader:

- it needs a deliberate repo-wide audit
- it should not be mixed into a code refactor closeout
- it needs enough focus to resolve terminology rather than applying piecemeal edits

## Current Friction

- `feature` exists in real CLI and planning surfaces, but not all docs treat it as part of the canonical issue-type lattice.
- Some validation documentation still describes the older hierarchy rules.
- Different docs mix graph structure, queue behavior, and workflow guidance without a single canonical statement of what the DAG is allowed to contain.
- Skills and operator docs may teach behavior that is partially stale relative to current implementation.

## Scope

This future story should audit and reconcile DAG-related truth across:

- `docs/design/architecture.md`
- `docs/validation-codes.md`
- `docs/commands.md`
- any DAG/type references in planner and worker skills
- any design docs that describe ready-queue semantics, hierarchy legality, or graph traversal behavior

## Questions This Deep Dive Must Resolve

- Is `feature` a permanent canonical node type in Armature's domain model?
- What is the authoritative type lattice?
- Which behaviors are graph facts versus queue policy versus workflow guidance?
- Which docs should be canonical for structure, and which should defer to them?

## Non-Goals

- Code refactoring
- Hook integration work
- Planner/runtime graph unification
- Rewriting every historical design artifact for stylistic consistency

## Success Criteria

- One clear, repo-truthful statement of the canonical issue-type lattice exists.
- The main architecture, validation, and command docs no longer contradict one another on DAG semantics.
- Embedded skills that teach DAG-related behavior no longer rely on stale terminology.
- Follow-up sessions can point to one canonical place for structure and one canonical place for operator behavior.

## Handoff Notes

This work should start after the narrower graph-facts follow-up story lands, so the documentation pass can describe the settled implementation seam instead of a moving target.
