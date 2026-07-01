---
name: armature-coordinator
description: >
  Use when operating orchestration in an armature-managed repository — surveys
  the story DAG, dispatches workers wave by wave, integrates outcomes, validates
  citation coverage, and closes stories with a pull request.
  Requires a worker identity (arm worker-init) and arm on PATH.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Coordinator

The coordinator manages execution flow — it does not implement features itself.
Its job is to survey the story DAG, dispatch workers for each wave of ready tasks,
and close the story when all tasks are done.

## Prerequisites

1. If `arm` is not found, stop and resolve this before proceeding.

2. **Worker identity required.** Run `arm worker-init` once per clone before claiming any tasks:
   ```bash
   arm worker-init --check || arm worker-init
   ```
   `arm claim` calls `resolveWorkerAndLog`, which fails with "worker not initialized" if no worker ID is set in git config.

3. Understand the story DAG before dispatching. Run:
   ```
   arm list --parent STORY-ID          # all tasks + statuses
   arm list --status blocked           # diagnose any blockers
   arm doctor                          # repo health check
   ```
   Fix any `doctor` errors before claiming work.

## DAG Hygiene Mandate

**`arm validate` and `arm doctor` must exit clean at all times.** This is non-negotiable.

Before dispatching any worker and after each wave completes, run:
```bash
arm validate       # zero ERRORs; all issues cited
arm doctor        # zero errors; no broken refs, orphaned ops, or cycles
```

If either exits non-zero, stop. Fix the reported issues before proceeding. Treat DAG decay the same way you treat failing tests — it is a blocker, not a warning to ignore.

Warnings from other stories must be resolved, not ignored. If `arm doctor` reports a D1 (commits referencing non-done issues) or D2 (stale claims) from unrelated work, clean them up before starting your coordination wave. DAG health is cumulative.

---

## The Coordinator Loop

```dot
digraph coordinator_loop {
    "arm ready" [shape=box];
    "Empty?" [shape=diamond];
    "Parallel?" [shape=diamond];
    "Sequential wave" [shape=box];
    "Parallel wave" [shape=box];
    "Claim + render-context all" [shape=box];
    "dispatch workers" [shape=box];
    "wait + integrate" [shape=box];
    "arm validate" [shape=box];
    "transition story" [shape=box];
    "push + PR" [shape=box];
    "Done" [shape=doublecircle];

    "arm ready" -> "Empty?";
    "Empty?" -> "arm validate" [label="yes — all done"];
    "Empty?" -> "Parallel?" [label="no"];
    "Parallel?" -> "Sequential wave" [label="deps between tasks"];
    "Parallel?" -> "Parallel wave" [label="independent tasks"];
    "Sequential wave" -> "dispatch workers";
    "Parallel wave" -> "Claim + render-context all";
    "Claim + render-context all" -> "dispatch workers";
    "dispatch workers" -> "wait + integrate";
    "wait + integrate" -> "arm ready";
    "arm validate" -> "transition story";
    "transition story" -> "push + PR";
    "push + PR" -> "Done";
}
```

## Step-by-Step

### 1. Survey the Story and Create a Feature Branch

```bash
arm list --parent STORY-ID
arm doctor
git checkout -b feat/STORY-ID   # create the story branch NOW, before any worker is dispatched
```

Identify which tasks are `open` and which have `blocked_by` dependencies. Group
tasks into waves — tasks within the same wave have no dependencies on each other
and can run in parallel. Tasks in different waves must run sequentially.

**Create the feature branch before dispatching any worker.** This is the shared
story branch, but workers do not commit to it directly: each worker commits to
its own per-task branch (`task/TASK-ID`) in an isolated worktree created by
`arm claim --worktree` (see Dispatch Protocol steps 4-5). The coordinator later
merges each completed task branch into `feat/STORY-ID` (see "After Workers
Return", section b). If the story branch does not exist before dispatch, there
is nothing for the coordinator to merge task branches into, and the story
cannot be reviewed via PR.

### 2. Find Ready Work

```bash
arm ready                              # unblocked, unclaimed tasks
```

If `arm ready` returns nothing and not all tasks are `done`, check for
dependency cycles or stalled in-progress tasks:
```bash
arm ready --explain                    # why each open task is NOT ready (blocked/claimed/missing dep)
arm list --status in-progress          # claims that may have expired
arm list --status blocked              # diagnose blockers
```

`arm ready --explain` prints a per-task diagnosis for every open task that
did not make it into the ready queue. Use it as the first step whenever the
queue looks unexpectedly empty.

### 3. Record Wave Manifest

Before dispatching any worker, record the wave manifest so the verification gate
has a stable baseline to diff against:

```bash
WAVE_TASK_IDS="TASK-A TASK-B ..."      # exact IDs in dispatch order
WAVE_BASE_SHA=$(git rev-parse HEAD)    # commit HEAD at wave start
WAVE_BRANCH=$(git rev-parse --abbrev-ref HEAD)  # story feature branch

# Classify wave type (determines which verification profile to run)
WAVE_TYPE=docs-skill-only              # default; promoted below if code files present
```

**Wave type auto-promotion rule:** inspect the ready-task scope fields. If any
task touches files matching `*.go`, `go.mod`, `go.sum`, `Makefile`, `cmd/**`,
or `internal/**` outside of `internal/skillsembed/`, set `WAVE_TYPE=code`.
A wave is docs-skill-only only when every changed file is a `SKILL.md`,
`references/*.md`, or other non-compiled documentation.

```bash
# Collect scope files from arm render-context output for each task in WAVE_TASK_IDS,
# or use `git diff --name-only "$WAVE_BASE_SHA"..HEAD` after workers return.
# Example: auto-promote based on task scope fields before dispatch:
WAVE_SCOPE_FILES=$(arm ready --parent STORY-ID --format json | python3 -c "import sys,json; [print(f) for t in json.load(sys.stdin) for f in t.get('scope',[])]")

if echo "$WAVE_SCOPE_FILES" | grep -E '\.(go|mod|sum)$' | grep -q . || \
   echo "$WAVE_SCOPE_FILES" | grep -E '^(Makefile|cmd/|internal/)' | grep -qvE 'internal/skillsembed'; then
    WAVE_TYPE=code
fi
```

### 4. Dispatch Workers

For each wave of ready tasks:

1. Claim and get context for each task:
   ```bash
   arm claim TASK-ID --ttl <minutes> --worktree /tmp/arm-task-TASK-ID
   arm render-context TASK-ID --format agent
   ```
   **Worktree is required.** When workers are dispatched as background agents without
   an active terminal session, they cannot inherit the parent session's Bash
   permissions, causing shell commands to hang. Pass `--worktree <path>` to `arm claim`
   (using a path that does not yet exist) — `arm claim` creates the worktree and a
   correctly-bound task branch automatically. Do not pre-create the worktree with
   `git worktree add`; let `arm claim` handle creation.

   Set `--ttl` to exceed your expected worker runtime. Default is 60 minutes; use
   `--ttl 240` or higher for complex tasks. If the TTL expires while a worker is
   still running, the claim becomes stale and another coordinator may re-dispatch
   the same task. Workers send periodic heartbeats (`arm heartbeat TASK-ID`) to
   reset the TTL — the worker skill handles this — but the coordinator's initial
   TTL must cover the time until the first heartbeat.

2. Dispatch each task to a worker agent using your platform's agent dispatch
   capability. Pass the full `render-context` output as the task specification.

3. For parallel waves, assign each worker a log slot before dispatch:
   ```bash
   export ARM_LOG_SLOT=<slot-number>
   ```

See [Dispatch Protocol](#dispatch-protocol) below for the full worker prompt format.

### 5. Parallel Dispatch (independent tasks in one wave)

Pre-claim all tasks in the wave, then dispatch workers concurrently. Each worker:
1. receives the pre-claimed issue context
2. implements and transitions to `done`
3. does NOT run `arm claim` again

Claim collisions are handled at pre-claim time by the coordinator.

---

## Dispatch Protocol

Each worker's context package must contain:

0. **Skill invocation (VERY FIRST instruction):**
   ```
   You are an armature worker. Invoke the `armature-worker` skill via the Skill tool before proceeding.
   ```
   This must appear before everything else — the skill loads the worker's
   operating procedure and pre-flight checks.

1. **Log slot (second instruction, before any `arm` command):**
   ```
   Before running any arm command, run: export ARM_LOG_SLOT=<assigned-slot>
   ```
   This must be the second line of the worker's prompt — immediately after
   the skill invocation.

2. **Full `render-context` output** — this is the worker's complete task spec.
   Do not summarize it; pass it verbatim.

3. **Pre-claimed notice** — tell the worker the issue is already claimed and it
   must NOT run `arm claim` again:
   ```
   This issue has been pre-claimed. Do NOT run `arm claim`. Do NOT run `arm worker-init`.
   ```

4. **Repository location:**
   Use the isolated git worktree created for this task by `arm claim --worktree`, not the main repository:
   ```
   Working directory: /tmp/arm-task-TASK-ID
   ```

5. **Task-specific branch:**
   The task-specific branch was created and is already checked out by `arm claim --worktree`.
   Do NOT run `git checkout feat/STORY-ID` (the shared story branch) — this causes collisions with parallel workers.
   Commit directly to the current branch:
   ```
   Working branch: (task-specific branch from render-context)  — do not run `git checkout feat/STORY-ID`
   ```

6. **Commit instruction** — instruct the worker to stage files explicitly using
   the task's `scope` field, not `git commit -am`:
   ```
   Commit: git add <each file listed in scope> && git commit -m "feat(ISSUE-ID): ..."
   ```

> **Background agent Bash limitation:** Background agents dispatched without an
> active terminal session cannot inherit the parent session's Bash permissions.
> Shell commands will block silently, causing the worker to hang indefinitely.
> To avoid this, prefer:
> - **Direct implementation** — have the coordinator implement small, well-scoped
>   tasks itself rather than dispatching a background agent.
> - **Foreground worktrees** — create a git worktree manually and run the worker
>   in a foreground terminal session so it inherits Bash permissions.

---

## After Workers Return

Run this integration checklist after each wave completes:

### a. Check task status
```bash
arm list --parent STORY-ID            # confirm all wave tasks are done
arm list --status in-progress         # any stragglers?
```

### a.1. Worker Recovery — Unkept `arm transition`

If a worker returned but their task remains `in-progress` or `done` without running `arm transition` (e.g., the worker forgot or the agent timed out), manually transition the task:

```bash
# List all tasks still in-progress or done
arm list --parent STORY-ID --format json | grep -E '"status":\s*"(in-progress|done)"'

# For each task that should be transitioned, manually run:
arm transition TASK-ID --to done --outcome "CONCRETE_OUTCOME_DESCRIPTION"
```

The recovery step:
1. **Identify the gap** — run `arm list --parent STORY-ID` and look for tasks with `"status": "in-progress"` or `"status": "done"` that do not appear in the wave manifest or were not marked `merged` in step (c) below.
2. **Understand what the worker did** — check the commit log for `TASK-ID` commits and review the scope files modified. Use `git diff` to confirm the work is complete.
3. **Write a concrete outcome** — do not re-use generic phrases like "Done" or "Completed". Reference specific files changed, tests added, or commands verified. Example: `"Implemented TokenParser.Parse() method; all 8 token types pass new tests; coverage 82%"`.
4. **Transition manually** — run `arm transition TASK-ID --to done --outcome "..."` with the specific outcome. This unblocks dependent tasks and prepares the issue for merge validation.

This is common when workers return from background dispatch without explicit handoff, or when TTL expiration causes a race with the heartbeat mechanism. Recovery is safe — `arm transition` is idempotent once an issue is already `done`.

### a.2. Semantic Review (Reviewer Dispatch)

For each task that completed in the wave, dispatch semantic conformance review using task-scoped delivery bundles:

**Task-Scoped Semantic Review** — each task's review bundle must contain only that task's changes, not the cumulative wave diff. This ensures:
- Scope violations are detected correctly (task didn't modify unrelated files)
- Acceptance criteria are matched to the right task's delivery
- Code quality assessment applies to the right code
- Clear audit trail of which task changed what

**Workflow:**

1. **Capture per-task commit ranges** — each task was completed in its own isolated
   worktree on branch `task/TASK-ID` (Dispatch Protocol steps 4-5), so the task's
   commit range is simply that branch relative to the wave's base commit. No
   commit-message scanning or git-history reconciliation is required, because each
   task's commits already live on their own branch rather than interleaved on a
   shared one:
   ```bash
   declare -A TASK_COMMITS   # TASK_ID -> "$WAVE_BASE_SHA..task/TASK-ID"

   for TASK_ID in $WAVE_TASK_IDS; do
     if ! git rev-parse --verify "task/$TASK_ID" >/dev/null 2>&1; then
       echo "ERROR: branch task/$TASK_ID not found. Did the worker commit before returning?" >&2
       exit 1
     fi
     TASK_COMMITS["$TASK_ID"]="$WAVE_BASE_SHA..task/$TASK_ID"
   done
   ```

   **Important — ordering:** at this point the task branches have **not** yet been
   merged into the story branch (that happens in step (b) below, which runs after
   this semantic review and the overlap audit in a.3). Do not substitute `HEAD` or
   `feat/STORY-ID` for `task/$TASK_ID` here — until the merge in step (b), those
   refs do not contain the task's commits.

2. **Prepare per-task review bundles** — use task-specific commit ranges, not wave-combined ranges:
   ```bash
   # For each task, capture its delivery diff (task-scoped, not wave-scoped)
   TASK_BASE="<task's base commit from step 1>"
   TASK_HEAD="<task's head commit from step 1>"
   
   BUNDLE_FILE=$(mktemp)
   arm review prepare --issue TASK-ID \
     --base "$TASK_BASE" --head "$TASK_HEAD" \
     --output "$BUNDLE_FILE"
   ```
   
   This creates a JSON bundle file containing the issue's acceptance criteria, scope, and the diff of **only** that task's changed files. The bundle is written to `$BUNDLE_FILE` for later use in both the reviewer dispatch and assessment recording steps.

3. **Dispatch the armature-reviewer agent** — pass the task-scoped bundle file path to a reviewer subagent:
   ```
   Dispatch armature-reviewer with bundle file: $BUNDLE_FILE (pass this path; the reviewer reads the bundle from the file)
   ```
   The reviewer assesses whether the delivery conforms to the issue contract (acceptance criteria, scope adherence, code quality). It is a subagent whose final text output is the `ConformanceAssessment` JSON. After the subagent returns, write its output text to a temp file:
   ```bash
   RESULT_FILE=$(mktemp)
   # The reviewer subagent's returned text IS the ConformanceAssessment JSON.
   # Write it directly to $RESULT_FILE, e.g.:
   #   echo "$REVIEWER_OUTPUT" > "$RESULT_FILE"
   # where $REVIEWER_OUTPUT is the text returned by the reviewer subagent.
   ```

4. **Record the assessment** — persist the reviewer's findings:
   ```bash
   arm review record --issue TASK-ID --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE"
   ```
   This links the assessment to the issue and updates its review status. Red ratings may block further wave progression until remediated. Pass both `--assessment "$RESULT_FILE"` and `--bundle "$BUNDLE_FILE"` as file paths (not raw JSON content) so the recorded assessment is bound to the exact bundle (and its durable identity) the reviewer evaluated, preventing a stale or mismatched bundle from being credited.

**Note:** The reviewer checks *semantic conformance* to the contract — whether the code solves the stated problem cleanly. This is independent of the auditor's checks (citation coverage, repo health). Both gates must pass before story sign-off.

### a.3. Parallel Branch Overlap Audit

When multiple tasks run in parallel (same wave), there is a risk of **semantic revert**: one task may undo, contradict, or invalidate changes from another task in files they both touched.

**Identify overlapping files:**

After all parallel wave tasks have transitioned to `done`, audit for files modified by multiple tasks in the same wave:

```bash
# Build a list of files changed by each task
declare -A TASK_FILES
for TASK_ID in $WAVE_TASK_IDS; do
  TASK_BASE="${TASK_COMMITS[$TASK_ID]%%\.\.*}"   # extract base from range
  TASK_HEAD="${TASK_COMMITS[$TASK_ID]##*\.\.}"   # extract head from range
  TASK_FILES["$TASK_ID"]=$(git diff --name-only "$TASK_BASE".."$TASK_HEAD")
done

# Find overlaps: files touched by >1 task
# NOTE: use the union of each task's own file list, not "$WAVE_BASE_SHA"..HEAD —
# task branches are not yet merged into HEAD at this point (merge happens in step b).
OVERLAPPING_FILES=""
ALL_CHANGED_FILES=$(for TASK_ID in $WAVE_TASK_IDS; do echo "${TASK_FILES[$TASK_ID]}"; done | sort -u)
for FILE in $ALL_CHANGED_FILES; do
  TASK_COUNT=0
  for TASK_ID in $WAVE_TASK_IDS; do
    if echo "${TASK_FILES[$TASK_ID]}" | grep -q "^$FILE$"; then
      ((TASK_COUNT++))
    fi
  done
  if [ "$TASK_COUNT" -gt 1 ]; then
    OVERLAPPING_FILES="$OVERLAPPING_FILES $FILE"
  fi
done

if [ -n "$OVERLAPPING_FILES" ]; then
  echo "WARNING: Files modified by multiple parallel tasks in wave $WAVE_TASK_IDS:"
  echo "$OVERLAPPING_FILES" | tr ' ' '\n' | sort -u
fi
```

**Audit semantic compatibility:**

For each overlapping file, manually review the diffs from each task to confirm:
- Changes are **additive**, not contradictory (e.g., both tasks add to a list, not delete the same item)
- The combined effect preserves intended semantics (e.g., a refactoring in task A doesn't invalidate a bug fix in task B)
- Test coverage is sufficient to catch regressions (integration tests should exercise the overlapped file in multiple contexts)

**Failure mode:** If any overlapping file shows contradictory changes (e.g., task A sets a flag to false, task B sets it to true), the semantic revert risk is **HIGH**. Escalate to reviewer dispatch with explicit test evidence before marking tasks `merged`.

### b. Check for scope conflicts and merge conflicts

Now that semantic review (a.2) and the overlap audit (a.3) are complete, merge
each task's branch (`task/TASK-ID`) into the story feature branch. Resolve any
conflicts before proceeding. Only after this merge do the task branches' commits
become reachable from `feat/STORY-ID`'s `HEAD`.

### c. Wave Verification Gate

After confirming all wave tasks are `done`, run the verification gate against the
wave manifest recorded in step 3 before dispatch.

**Do not run `arm merged` until this gate passes.** If the gate fails, tasks must
remain in `done` (not `merged`) so the coordinator retains visibility into which
tasks need remediation.

**Terminal sanity check:**
```bash
echo "Wave: $WAVE_TASK_IDS"
echo "Base SHA: $WAVE_BASE_SHA"
echo "Branch: $WAVE_BRANCH"
echo "Wave type: $WAVE_TYPE"
```
If any variable is unset, stop — the manifest was not recorded before dispatch.
Reconstruct it from `arm list --status done` and `git log` before proceeding.

**Determine changed-file set:**
```bash
CHANGED_FILES=$(git diff --name-only "$WAVE_BASE_SHA"..HEAD)
```

**Auto-promote wave type:**
```bash
if echo "$CHANGED_FILES" | grep -E '\.(go|mod|sum)$' | grep -q . || \
   echo "$CHANGED_FILES" | grep -E '^(Makefile|cmd/|internal/)' | grep -qvE 'internal/skillsembed'; then
    WAVE_TYPE=code
fi
```

**Code profile** (run when `WAVE_TYPE=code`):
```bash
go build ./...   # compilation gate
make check       # lint + test + coverage-check + mutate + validate-skills + build
arm validate --quiet                                    # citation integrity
arm doctor                                              # repo health
```

If `go build` fails and `make` is unavailable, fall back to:
```bash
go run ./cmd/armature --help   # confirms the binary compiles
```

**Docs-skill-only profile** (run when `WAVE_TYPE=docs-skill-only`):
```bash
make validate-skills   # skills must reference arm, not install steps
arm validate --quiet   # citation integrity
arm doctor             # repo health
```

If any `*.go`, `go.mod`, `go.sum`, `Makefile`, `cmd/`, or `internal/` file
(outside `internal/skillsembed/`) appears in `$CHANGED_FILES`, auto-promote to
the code profile and re-run.

**Bounded remediation (2 attempts max):**

- **Attempt 1:** Fix reported failures. Be strict — address every error and
  warning before re-running the gate.
- **Attempt 2:** If failures persist, escalate: add an `arm note` on the story
  describing the blocker, do not transition, and surface the issue to the user
  before proceeding to the next wave.

Do not proceed to the next wave or story transition if the gate is red after
2 remediation attempts.

### d. Mark completed tasks merged

Once the verification gate passes, promote all completed wave tasks from `done` to `merged`:

```bash
arm merged --issue TASK-ID
```

This allows dependent work to unblock cleanly before the next wave begins.

### e. Check citation coverage
```bash
arm validate
```

If `validate` shows `uncited node: ID`, run:
```bash
arm source-link --issue ID --source SOURCE-UUID   # if a source doc exists
# or
arm accept-citation --issue ID --ci               # if no source, mark as self-citing
```

### f. Clean up worktrees

If workers used git worktrees, remove them after their branches are merged:

```bash
git worktree list
git worktree remove <path> --force
git branch -d <worker-branch>
```

### g. Continue to next wave
```bash
arm ready    # next wave should now be unblocked
```

---

## Story Completion

When `arm ready` returns empty and all tasks are `done`:

### 1. Run the Auditor (pre-merge gate)

Dispatch the **armature-auditor** skill as a subagent before any story transition.
The auditor is a five-step pre-merge gate — it must give all-clear before you proceed.

**Invoke via the `Skill` tool:**
```
Skill("armature-auditor")
```

The auditor checks:
1. Citation integrity (`arm validate` — zero ERRORs, `COVERAGE: N/N cited`)
2. Source freshness (`arm sources verify` — zero MISSING)
3. Outcome quality (concrete outcomes against acceptance criteria)
4. Scope overlap (`arm validate --strict` — zero overlap warnings)
5. Repo health (`arm doctor --strict` — exit zero)

**Do not proceed to step 2 until the auditor reports all five checks green.**

### 2. Transition the story
```bash
arm transition STORY-ID --to done --outcome "brief summary of what was delivered"
```

### 3. Commit armature ops (single-branch mode only)

```bash
git status
git add .armature/ && git commit -m "chore(STORY-ID): sync armature state"
```

In **dual-branch mode**, ops are automatically committed to the `_armature` branch.

### 4. Push and open PR
```bash
git push -u origin HEAD
# Open a PR targeting your main/base branch
# PR title: the story title
# PR body: list each task ISSUE-ID and its one-line outcome
```

**One PR per story.**

---

## Common Failure Modes

| Failure | Cause | Fix |
|---|---|---|
| Parallel agents share one log, attribution lost | Forgot to embed `ARM_LOG_SLOT` in each agent's prompt | Include `export ARM_LOG_SLOT=<slot>` as the first instruction in each agent's prompt before dispatch |
| Build breaks after merging parallel branches | Skipped integration verification | After each wave, run `make check` before claiming the next wave |
| Semantic revert when merging parallel task branches | Multiple parallel tasks touched the same file; merge did not account for interdependencies | After each parallel wave, run the Parallel Branch Overlap Audit (section a.3); review semantic compatibility of overlapping files before marking tasks `merged`; add integration tests if needed to exercise combined changes |
| `arm transition STORY-ID --to done` errors with uncited nodes | Story transitioned before all issues were cited | Run `arm validate`; for each `uncited node: ID`, run `arm source-link` or `arm accept-citation --ci`; then retry transition |
| Armature ops not committed | Forgot mop-up commit before push | After story transition, run `git status`; if `.armature/` has changes, commit them (single-branch mode only) |
