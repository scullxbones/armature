# Armature Context

Armature is a git-native work orchestration system for coordinating human and AI workers through append-only ops, materialized state, and a typed work graph in git.
This document is the canonical glossary for Armature's domain language, including the storage and topology terms agents need to reason about the full system.

## Language

**Issue**:
A unit of work tracked by Armature. `Epic`, `Story`, `Feature`, `Task`, and `Bug` are issue types within the work graph.
_Avoid_: Node, ticket, item

**Node**:
A graph-structural view of an issue when discussing parentage, dependency edges, or subtree operations. Use `Node` for DAG structure, not as the default name for work.
_Avoid_: Ticket, work item

**Leaf Issue**:
An issue with no children in the current DAG. Leaf is a graph-shape property, not a lifecycle status; Task and Bug are always leaf issues in a valid DAG.
_Avoid_: Terminal issue, completed issue

**Epic**:
A top-level issue representing the broadest body of planned work in Armature's issue graph. An epic can contain stories, features, tasks, or bugs.
_Avoid_: Initiative, project

**Story**:
A first-class issue type for a coherent slice of work that groups smaller executable work items. A story can contain tasks or bugs.
_Avoid_: Feature, task group

**Feature**:
A first-class issue type for feature-shaped work. A feature can contain tasks or bugs.
_Avoid_: Story, enhancement

**Task**:
A first-class issue type for the smallest executable unit of planned work. Tasks cannot contain other issue types and are therefore always leaf issues in a valid DAG.
_Avoid_: Subtask, step

**Bug**:
A first-class issue type for defect-shaped work. Bugs cannot contain other issue types and are therefore always leaf issues in a valid DAG.
_Avoid_: Defect task, hotfix task

**Ready**:
A derived condition meaning an issue is currently actionable under Armature's queue rules. `Ready` is not an issue status; it is computed from issue type, dependencies, claim state, and other gating conditions.
_Avoid_: Status, open

**Open**:
An issue status meaning the issue exists in the workflow but is not currently claimed, completed, or otherwise terminal. Open issues may or may not be ready.
_Avoid_: Ready, untracked

**Claimed**:
An issue status meaning a worker currently holds the claim on the issue. `Claimed` records reservation, not completion.
_Avoid_: In-progress, assigned

**In-Progress**:
An issue status meaning work is actively underway. It is distinct from `Claimed`, which only records who currently holds the issue.
_Avoid_: Claimed, started

**Done**:
An issue status meaning a worker considers the issue's work complete. `Done` does not necessarily mean the work has been integrated into the main branch.
_Avoid_: Merged, shipped

**Merged**:
An issue status meaning the completed work has been integrated into the main branch or otherwise promoted to Armature's integrated completion state. For downstream readiness, `Merged` is stronger than `Done`.
_Avoid_: Done, submitted

**Blocked**:
An issue status meaning work cannot currently proceed because a blocking condition prevents progress. Blocked is a workflow state, not merely the existence of a `blocked_by` relationship.
_Avoid_: Not ready, waiting

**Cancelled**:
An issue status meaning the issue has been intentionally abandoned or withdrawn from active workflow while preserving its history. Cancelled is terminal without implying successful completion.
_Avoid_: Deleted, done

**Source**:
A registered authority document or reference artifact that provides provenance for planned work. A source exists independently of any single issue.
_Avoid_: Citation, requirement link

**Source Link**:
The explicit relationship between an issue and a source entry. A source link records which source grounds that issue's existence or constraints.
_Avoid_: Source, citation acceptance

**Citation Acceptance**:
An explicit record that a worker accepted the cited provenance for an issue. Citation acceptance does not create the provenance link; it acknowledges it.
_Avoid_: Source link, citation

**Provenance**:
The recorded chain of evidence and authorship explaining why an issue exists and how its current shape was justified. In Armature, provenance spans source grounding, worker authorship, and confidence in the issue's validity.
_Avoid_: Citation, history

**Confidence**:
The provenance state describing how certain Armature is that an issue is valid for normal workflow progression. Confidence is separate from issue status and readiness.
_Avoid_: Status, readiness

**Draft**:
A confidence state meaning an issue exists in a not-yet-confirmed form. Draft issues remain outside the normal ready-flow until they are promoted.
_Avoid_: Verified, ready

**Verified**:
A confidence state meaning an issue is confirmed for normal workflow use. Verified issues participate in the ready-flow and ordinary execution rules.
_Avoid_: Draft, inferred

**Inferred**:
A confidence state meaning an issue was derived or imported with weaker certainty than a normal verified issue. Inferred issues belong to the confidence vocabulary, not the status vocabulary.
_Avoid_: Verified, done

**Worker**:
An actor identity that reads, claims, and updates issues in Armature. A worker is the system's unit of authorship and coordination.
_Avoid_: Agent session, assignee

**Reviewer**:
A fresh LLM worker role that uses the `armature-reviewer` skill to produce a conformance assessment for an assessable delivery. A reviewer judges the delivery diff with bounded read-only repository access for interpretation; it does not implement or remediate the delivery.
_Avoid_: Implementer, auditor, deterministic gate

**Auditor**:
A worker role that verifies deterministic governance and repository health before story sign-off. An auditor does not perform the reviewer's semantic conformance judgment.
_Avoid_: Reviewer, implementer, conformance assessor

**Bootstrap**:
The first-time preparation that makes a repository clone ready for Armature workflow. Bootstrap establishes local participation in the workflow; it is not issue decomposition or work execution.
_Avoid_: Installation, dispatch

**Dogfood Finding**:
A concise observation captured while Armature is used on Armature itself. A dogfood finding is a repository-maintenance artifact for later triage; it is not an Armature product feature or automatically a planned issue.
_Avoid_: Issue, note, retrospective

**Assignment**:
The routing of an issue toward an intended worker. Assignment expresses intended ownership before or apart from an active claim.
_Avoid_: Claim, reservation

**Assigned Worker**:
The worker an issue is explicitly routed toward. Assignment expresses intended ownership, not current claim ownership.
_Avoid_: Claimed by, worker

**Claimed By**:
The worker currently holding the active claim on an issue. `Claimed By` records current reservation, not long-term assignment.
_Avoid_: Assigned worker, owner

**Ops Branch**:
The git branch that stores Armature's coordination data under `.armature/`. It is the branch of record for append-only operational history.
_Avoid_: Main branch, code branch

**Code Branch**:
The git branch that carries code changes through the repository's normal review and merge flow. A code branch is distinct from the branch that stores Armature coordination state.
_Avoid_: Ops branch, state branch

**Ops Worktree**:
The local worktree used to operate on the ops branch and its `.armature/` data. The ops worktree is the local working surface for coordination state, not the main code editing surface.
_Avoid_: Code worktree, repo root

**Op**:
An append-only recorded action in Armature's operational history. Ops are the atomic facts from which Armature derives current issue state.
_Avoid_: State row, record update

**Materialization**:
The process of replaying ops into Armature's current derived view of the system. Materialization computes present state from append-only history.
_Avoid_: Mutation, synchronization

**Materialized State**:
The derived current view produced by materialization. Materialized state is operationally useful, but it is not the source of truth.
_Avoid_: Source of truth, raw history

**DAG**:
The directed acyclic graph of Armature issues and their relationships. The DAG includes hierarchy and dependency relationships, but it is not the same concept as queue ordering or workflow status.
_Avoid_: Queue, tree

**Parent**:
The containing issue above another issue in the hierarchy. Parentage defines structural decomposition, not execution ordering.
_Avoid_: Blocker, dependency

**Blocked By**:
The dependency relationship meaning one issue cannot proceed until another issue reaches the required completion state. `Blocked By` expresses execution ordering, not containment.
_Avoid_: Parent, child, blocked status

**Render Context**:
The assembled working input Armature presents for an issue before execution. Render context is the issue-specific view a worker reads to understand what to do and what constraints apply.
_Avoid_: Prompt, raw issue dump

**Context Files**:
Curated stable reference files that should be read as background for an issue. Context files inform rendered context without expanding the issue's write scope.
_Avoid_: Scope, source files

**Scope**:
The declared set of repository paths an issue is allowed to change. Scope constrains write boundaries; it does not describe background reading material.
_Avoid_: Context files, acceptance

**Acceptance**:
The criteria Armature uses to judge whether an issue's work satisfies its intended checks. Acceptance describes how correctness is evaluated; it is narrower than the issue's overall completion meaning.
_Avoid_: Outcome, definition of done

**Definition of Done**:
The plain-language statement of what it means for an issue to be complete. Definition of done sets the completion bar; it is not the record of what actually happened.
_Avoid_: Acceptance, outcome

**Task Contract**:
The authoritative description of an issue's intended delivery: its definition of done, acceptance, scope, and linked requirement references. Phase-one conformance assessment judges definition of done and acceptance; deterministic policy owns scope, while requirement references remain provenance context for a future phase.
_Avoid_: Render context, outcome, parent description

**Outcome**:
The recorded result of work on an issue when a worker reports completion or another terminal change. Outcome states what was actually delivered or observed, not the abstract completion bar.
_Avoid_: Definition of done, acceptance

**Verification Evidence**:
A machine-readable record that a deterministic check ran and what it reported for an issue or requirement. Verification evidence supports review but does not by itself establish semantic conformance with the intended work.
_Avoid_: Conformance assessment, acceptance, outcome

**Conformance Assessment**:
A structured LLM-as-judge evaluation of whether delivered work is semantically faithful to an issue's definition of done and acceptance. A conformance assessment informs review; it neither reruns deterministic checks nor acts as a completion gate.
_Avoid_: Verification evidence, acceptance, outcome

**Criterion Result**:
The evidence-cited assessment of one definition-of-done or acceptance criterion as `satisfied`, `partially_satisfied`, `not_satisfied`, or `indeterminate`. Criterion results are the authoritative semantic judgments within a conformance assessment.
_Avoid_: Conformance rating, verification result

**Conformance Rating**:
The reviewer-facing green, yellow, or red summary derived from criterion results. Green means all criteria are satisfied; yellow means at least one is partial or indeterminate and none are unsatisfied; red means at least one is not satisfied.
_Avoid_: Criterion result, confidence score, completion status

**Assessment Attestation**:
The compact durable record that a conformance assessment was performed against a specific delivery and task contract. An assessment attestation preserves identity, fingerprints, conformance rating, and criterion-result counts without retaining the full assessment report.
_Avoid_: Conformance assessment, verification evidence, audit transcript

**Assessable Delivery**:
Work recorded by an issue's transition to `Done` with an outcome and an associated repository diff. Any issue type can produce an assessable delivery, although work should ordinarily be planned on leaf issues.
_Avoid_: Leaf issue, outcome, pull request

**Review Bundle**:
The canonical read-only package Armature assembles for a reviewer, containing the assessable delivery's task contract, outcome, delivery identity, and diff. A review bundle excludes implementation activity history and customer check configuration.
_Avoid_: Render context, activity log, verification pipeline

**Transition**:
The intentional state change of an issue from one status to another. A transition records lifecycle movement, not just current state.
_Avoid_: Status, update

**Reopen**:
The act of returning a previously completed or blocked issue to active workflow. Reopening changes the issue's workflow posture without erasing its prior history.
_Avoid_: Reset, delete

**Heartbeat**:
The periodic signal that keeps an active claim alive. A heartbeat extends claim liveness without changing the issue's broader workflow meaning.
_Avoid_: Claim, transition

**Claim TTL**:
The time window during which a claim remains live without renewal. Claim TTL governs when a claimed issue becomes stale unless a heartbeat refreshes it.
_Avoid_: Heartbeat, status timeout

**Ready Queue**:
The ordered set of issues Armature currently considers actionable for claiming. The ready queue is derived from readiness rules plus queue ordering, not stored as source truth.
_Avoid_: DAG, status list

**Harness Hook**:
The integration boundary through which Armature applies scope and verification policy to an external harness. A harness hook is part of Armature's control surface, not an external worker in itself.
_Avoid_: Queue runner, orchestrator

**Execution Evidence**:
Harness-recorded command-and-output facts captured during a bound issue's work, admissible in semantic review as a weaker evidence class than the delivery diff. Execution evidence is upgrade-only: it can lift an indeterminate criterion toward satisfied but can never substitute for diff evidence or suppress a contradiction the diff supports.
_Avoid_: Activity trace, transcript, worker claims

**Activity Log**:
The complete, unselected record of a bound issue's harness-recorded executions, kept locally with the issue's workspace and never entering durable Armature history. Completeness is what distinguishes it from worker-curated evidence.
_Avoid_: Audit log, ops log, decision log

**Activity Index**:
A structured, schema-validated finding aid summarizing an activity log for reviewer navigation. The index routes a reviewer to raw activity entries; it is never itself citable evidence.
_Avoid_: Summary report, activity evidence

**Issue Binding**:
The association between a claimed leaf issue and a specific worktree, established when `arm claim` writes the issue ID into the worktree's local git state. Issue binding is what allows the harness hook to enforce that issue's scope within that worktree, and it holds for any leaf issue (Task or Bug), not only Tasks. It is distinct from the claim itself: the claim records reservation in the ops log; the binding records the active issue in the worktree. Invariant: one harness process operates under exactly one issue binding at a time. **The binding is the sole authority for worktree identity.** A branch name and a directory basename are descriptions, never identity: a worktree detached mid-rebase is still bound to its issue, and a stranger's worktree holding the expected branch is still not. No code may establish, infer, or synthesize a binding from either.
_Avoid_: Task binding, claim, worktree assignment, marker

**Managed Worktree**:
A worktree Armature provisioned and owns, living under the canonical worktree root and carrying an issue binding. Managed worktrees are the ones reconciliation classifies and `arm worktree gc` may remove; a worktree a person created for their own purposes is unmanaged and is never a removal candidate, whatever its branch. Distinct from the ops worktree, which holds coordination state rather than issue work.
_Avoid_: Ops worktree, code worktree, checkout

**Canonical Worktree Path**:
The path Armature provisions a managed worktree at, derived from the issue ID under the canonical worktree root. It is distinct from the **recorded worktree path** materialized onto the issue from the claim op, which is the path that claim actually used — usually the canonical one, but a legacy or explicitly-placed worktree can differ. Where several worktrees share one binding, the recorded path is the tiebreak that decides which is real.
_Avoid_: Worktree root, recorded path, worktree location

**Adoption**:
Claim-time reuse of an existing worktree already bound to the issue, moved to the canonical worktree path instead of provisioning a second one. Adoption preserves the worktree's branch and uncommitted work, and is selected by binding alone. A bound worktree that is not on the issue branch is not adopted and not provisioned around: the claim fails closed, because the alternatives — relocating an in-progress git operation, or checking out over it — both risk the work adoption exists to protect.
_Avoid_: Reuse, takeover, migration

**Bound Worktree**:
The reconciliation class for a worktree whose binding names an issue holding a live claim, at the path that claim recorded. Bound is the healthy steady state of a managed worktree.
_Avoid_: Claimed, active, assigned

**Orphan Worktree**:
The reconciliation class for a worktree whose binding names a known issue that holds no live claim — unclaimed, past its claim TTL, or claimed in another clone. An orphan is a real worktree with no live owner, not an error.
_Avoid_: Ghost, stale worktree, unrecognized worktree

**Ghost Worktree**:
The reconciliation class for the inverse of an orphan: an issue holding a live claim whose recorded worktree path has no worktree on disk. A terminal issue whose worktree is gone is the expected end state, not a ghost.
_Avoid_: Orphan, missing worktree, stale claim

**Unrecognized Worktree**:
The reconciliation class for a worktree carrying no issue binding, or one naming an issue Armature does not know. Unrecognized is what a worktree whose binding was lost or never written becomes; it is never repaired by inferring an issue from the directory name, because doctor and the delivery gate will reject the same worktree.
_Avoid_: Orphan, unmanaged worktree, unknown

**Ambiguous Binding**:
The condition where more than one worktree carries the same issue's binding and the recorded worktree path picks none of them. Ambiguity is reported and refused, never resolved by guessing: the operations that consume a resolved worktree remove it forcibly, so choosing wrongly discards uncommitted work. Distinct from a worktree simply not being found, which is an ordinary outcome.
_Avoid_: Duplicate worktree, conflict, not found

**Hook Pass-through**:
The condition where the harness hook allows a tool operation without evaluating scope or verification policy. Pass-through occurs when no task is bound to the current worktree or when the bound task is no longer in an active state (claimed or in-progress). Pass-through events are logged to the worktree's hook log.
_Avoid_: Allow, bypass, skip

**Priority**:
The relative urgency Armature records for an issue when ordering or reviewing work. Priority influences how work is discussed and queued, but it is not a lifecycle state.
_Avoid_: Status, complexity

**Estimated Complexity**:
The expected size or difficulty of an issue before execution. Estimated complexity describes anticipated effort, not urgency or completion.
_Avoid_: Priority, outcome

**Decision**:
A structured recorded choice attached to an issue, including its rationale and affected scope. A decision captures intentional direction, not just incidental commentary.
_Avoid_: Note, comment

**Note**:
An unstructured annotation attached to an issue for progress, observations, or reminders. A note provides context without carrying the stronger meaning of a recorded decision.
_Avoid_: Decision, rationale record

**Surface**:
A user-facing point of the product: an issue type, status, confidence state, field, command, or flag. Surface is the general concept; specific processes (the subtractive-release census, the CLI grammar contract) each govern a subset of surfaces for their own purpose. A surface is distinct from a config knob (operator-facing, not user-facing) and from a skill (agent-facing prose, not a typed interface point).
_Avoid_: Feature, config knob, endpoint

**Deep Module**:
A package or command group with a narrow public interface hiding substantial implementation, per ADR 0004. Deep modules exist at two layers that should stay aligned: the Go package (`internal/sources`, `internal/validate`, etc.) and, where a package has a CLI-facing counterpart, the command group (`sources`, `validate`) that exposes it. A hyphenated command with no corresponding deep module is a signal that either a module boundary needs drawing or the command doesn't deserve group status.
_Avoid_: Package, module, component

**Census**:
The permanent, one-row-per-surface audit record pairing every surface subject to the subtractive-release census with its dogfood-corpus evidence and its ruling. The census table is the record of decision; it does not require a separate ADR per surface. A census row's identity survives a rename: renaming a surface updates the row's aliases, it does not reset its ruling or require fresh evidence.
_Avoid_: Audit, inventory

**Ruling**:
The recorded keep-or-park decision for a single surface in the census, made by a single accountable person and justified by corpus evidence or written reasoning. A ruling is not a vote or a consensus process.
_Avoid_: Decision, verdict

**Park**:
The census outcome for a cut surface: its code is deleted outright, but its census row, re-entry criterion, and the removing commit persist as an intentional, documented, in-principle-reversible record. Park is the only cut outcome this process produces.
_Avoid_: Purge, deprecate, feature-flag off

**Purge**:
A hypothetical cut outcome with no census row, no re-entry criterion, and no documented path back — reserved for code with no product surface at all. The subtractive-release census does not use this outcome; everything it cuts is parked.
_Avoid_: Park, delete

**Re-entry Criterion**:
The written condition, recorded on a surface's census row at park time, that would justify resuscitating that surface. A re-entry criterion is a standing test, not a promise of future work.
_Avoid_: Justification, ruling

**Resuscitation**:
The act of re-implementing a parked surface after its re-entry criterion is met. Resuscitation is triggered by evidence, not by request alone.
_Avoid_: Restore, unpark, revert
