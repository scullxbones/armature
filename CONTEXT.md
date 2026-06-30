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

**Single-Branch Mode**:
The operating mode where Armature's coordination data lives on the same branch as code. In single-branch mode, the system does not separate code history from coordination history by branch.
_Avoid_: Dual-branch mode, ops branch mode

**Dual-Branch Mode**:
The operating mode where Armature's coordination data lives on a dedicated ops branch separate from code branches. In dual-branch mode, coordination state and code changes follow distinct branch flows.
_Avoid_: Single-branch mode, main-only mode

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

**Task Binding**:
The association between a claimed task and a specific worktree, established when `arm claim` writes the task ID into the worktree's local git state. Task binding is what allows the harness hook to enforce a task's scope within that worktree. It is distinct from the claim itself: the claim records reservation in the ops log; the binding records the active task in the worktree.
_Avoid_: Claim, worktree assignment

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
