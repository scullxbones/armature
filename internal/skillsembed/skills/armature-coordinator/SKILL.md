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

The coordinator manages execution flow. It does not implement features.
Survey the story DAG, dispatch workers for each ready wave, integrate, close.

On-demand references (open when needed, not on every turn):

- `references/coordinator-loop.md` — loop diagram
- `references/parallel-dispatch.md` — waves, pre-claim, log slots
- `references/commands.md` — JSON field extraction
- `references/overlap-audit.md` — post-wave semantic overlap
- `references/wave-verification.md` — publish-gate profiles
- `references/violation-gate.md` — `arm merged` and worktree teardown
- `references/failure-modes.md` — recovery table

## Prerequisites

1. If `arm` is not found, stop and resolve this before proceeding.
2. Worker identity, once per clone, before any claim:
   ```bash
   arm worker-init --check || arm worker-init
   ```
   `arm claim` fails with "worker not initialized" otherwise.
3. Survey before dispatch:
   ```
   arm list --parent STORY-ID
   arm list --status blocked
   arm doctor
   ```
   Fix `doctor` errors before claiming.

## DAG Hygiene Mandate

**`arm validate` and `arm doctor` must exit clean.** Before every dispatch and after each wave:

```bash
arm validate
arm doctor
```

Non-zero means stop. Treat DAG decay like a failing test. Clean D1/D2 from other stories too. Health is cumulative.

## The Coordinator Loop

See `references/coordinator-loop.md` for the diagram. Procedure:

1. `arm ready`
2. Empty and all tasks done → validate, transition story, push, PR.
3. Else sequential (deps) or parallel (independent). Parallel: claim + render-context all, then dispatch. See `references/parallel-dispatch.md`.
4. Wait, integrate (below), loop.

## Step-by-Step

### 1. Survey the Story and Create a Feature Branch

```bash
arm list --parent STORY-ID
arm doctor
git checkout -b feat/STORY-ID
```

Create `feat/STORY-ID` before any worker. Workers commit to `task/TASK-ID` in worktrees from `arm claim --worktree`. You merge those into the story branch after the wave.

### 2. Find Ready Work

```bash
arm ready
```

If the queue is empty but work remains:

```bash
arm ready --explain
arm list --status in-progress
arm list --status blocked
```

### 3. Record Wave Manifest

```bash
WAVE_TASK_IDS="TASK-A TASK-B ..."
WAVE_BASE_SHA=$(git rev-parse HEAD)
WAVE_BRANCH=$(git rev-parse --abbrev-ref HEAD)
WAVE_TYPE=docs-skill-only
```

Promote to `WAVE_TYPE=code` if any ready-task scope matches `*.go`, `go.mod`, `go.sum`, `Makefile`, `cmd/**`, or `internal/**` outside `internal/skillsembed/`. Docs-skill-only only when every file is skill markdown or other non-compiled docs.

```bash
WAVE_SCOPE_FILES=$(arm ready --parent STORY-ID --format json | python3 -c "import sys,json; [print(f) for t in json.load(sys.stdin) for f in t.get('scope',[])]")
if echo "$WAVE_SCOPE_FILES" | grep -E '\.(go|mod|sum)$' | grep -q . || \
   echo "$WAVE_SCOPE_FILES" | grep -E '^(Makefile|cmd/|internal/)' | grep -qvE 'internal/skillsembed'; then
    WAVE_TYPE=code
fi
```

### 4. Dispatch Workers

For each ready task:

```bash
arm claim TASK-ID --ttl 120 --worktree
arm render-context TASK-ID --format agent
```

**`--worktree` is required.** The harness hook binds from the artifact path. `arm claim --worktree` provisions `.worktrees/<issue-id>`, writes `armature-issue-id`, and checks out the task branch. Do not `git worktree add` first.

Set `--ttl` past expected runtime (default 60). Use `--ttl 240` for heavy tasks. Workers heartbeat; the initial TTL must last until the first heartbeat.

Dispatch with the platform agent tool. Pass `render-context` verbatim. For parallel waves, assign a log slot in the prompt (`references/parallel-dispatch.md`). Prompt format: [Dispatch Protocol](#dispatch-protocol).

### 5. Parallel Dispatch

Pre-claim the wave, then dispatch concurrently. Workers must not `arm claim` again. Details: `references/parallel-dispatch.md`.

---

## Dispatch Protocol

**Transcript-free dispatch (normative).** Workers and reviewers get the rendered spec and file paths, never an inherited transcript. Reviewers get bundle **paths**. Confirmation reviewers also get the **findings-scope file path**. Remediation states what changed; do not re-send unchanged skills or bundles.

**Effort defaults (normative).** Medium for worker dispatch and task-level reviews. Assign **high** at planning time for concurrency, security, or cross-cutting refactors, and auto-escalate to high at remediation **cycle 2**. Story-level `armature-auditor` stays high.

Each worker context package:

0. **Skill invocation first:**
   ```
   You are an armature worker. Invoke the `armature-worker` skill via the Skill tool before proceeding.
   ```
1. **Log slot second, before any `arm` command:**
   ```
   Before running any arm command, run: export ARM_LOG_SLOT=<assigned-slot>
   ```
2. **Full `render-context` output**, verbatim.
3. **Pre-claimed notice:**
   ```
   This issue has been pre-claimed. Do NOT run `arm claim`. Do NOT run `arm worker-init`.
   ```
4. **Repository location:** `.worktrees/TASK-ID` (from `arm claim --worktree`).
5. **Task branch already checked out.** Do not `git checkout feat/STORY-ID`. See `docs/conventions.md`.
   ```
   Working branch: (task-specific branch from render-context)  — do not run `git checkout feat/STORY-ID`
   ```
6. **Commit via scope paths, not `git commit -am`:**
   ```
   Commit: git add <each file listed in scope> && git commit -m "feat(ISSUE-ID): ..."
   ```

Background agents without a terminal do not inherit Bash permissions and can hang. Prefer implementing small tasks in the coordinator session, or a foreground worktree so the worker inherits the shell.

---

## After Workers Return

### a. Check task status

```bash
arm list --parent STORY-ID
arm list --status in-progress
```

### a.1. Worker Recovery — Unkept `arm transition`

If the worker returned but the task is still `in-progress` (or `done` without `arm transition`):

```bash
arm list --parent STORY-ID --format json | grep -E '"status":\s*"(in-progress|done)"'
arm review commits TASK-ID --branch task/TASK-ID
arm transition TASK-ID --to done --outcome "CONCRETE_OUTCOME_DESCRIPTION"
```

Write a concrete outcome (files, tests, commands). `arm transition` is idempotent once `done`.

### a.2. Semantic Review (Reviewer Dispatch)

**Bounded, consolidated review protocol (normative).** Review is not an
open-ended back-and-forth. Per task:

1. **One comprehensive initial review** — the reviewer reports all findings in
   one pass, not the first defect found.
2. If independent perspectives are used, they run **in parallel**, each
   writing a **distinct** `.armature/review/` path (issue + bundle prefix +
   reviewer token). Aggregate their chat findings into **one** list
   **before** `arm review record` — never a serial review → fix → review
   → fix chain, and never one shared assessment file.
3. **One consolidated remediation request** covering every finding from step 1/2.
   The first review runs after the worker has already transitioned to `done`.
   **Do not remediate on a `done` (or `merged`) task.** `isBindingStale` treats
   any status other than `claimed` or `in-progress` as stale, so the harness
   hook passes through: no scope enforcement, no hook heartbeats, and no
   second `done` delivery gate on the remediating HEAD. Before dispatching
   the remediator, reopen and reclaim (workflow step 5 below). After the
   last remediating commit, the worker runs the full gate and transitions
   to `done` again. Then refresh every **stale** review artifact (step 6)
   before confirmation — do not reuse pre-remediation `$TASK_HEAD`,
   `$BUNDLE_FILE`, `$INDEX_OUTPUT`, or `$RESULT_FILE`. Keep `$FINDINGS_FILE`
   (the remediating set); it is confirmation **scope**, not a stale bundle.
4. **One narrow confirmation review**, hard-scoped to only the findings that
   were remediated; findings outside that scope are recorded but block
   further progress only at critical severity. **Refresh every stale
   review artifact first** (workflow step 6), then dispatch with the
   **same** `$FINDINGS_FILE` plus an explicit confirmation-scope
   instruction. A fresh reviewer given only a new bundle/index will
   repeat a comprehensive review. `arm review record` binds the
   assessment to the supplied bundle; a stale index still marks
   pre-remediation entries as head-anchored.
5. **Cap: 3 remediation cycles** per task (executable loop: workflow
   steps 5–7). After each confirmation, inspect the rating. Non-green
   repeats reopen / slotted-reclaim / remediate / confirm. After cycle 3
   still non-green, escalate to the human (Constitution I7) and **stop**.
   A non-green confirmation never proceeds to a.3 or merge.

For each completed wave task, dispatch a **task-scoped** review bundle (that task's diff only, not the wave). That keeps scope, acceptance, quality, and audit trail aligned to the right delivery.

**Workflow:**

Steps 1–7 run **per task**. At the start of each `TASK_ID`, `unset CYCLE`
and recover that task's highest `a.2 cycle N/3` note (steps 4 and 5).
Do not carry `CYCLE` from a previous task. Inside the remedia loop
(steps 5–7 for the same `TASK_ID`), keep the in-memory `CYCLE`.

1. **Capture per-task commit ranges** — each task lives on `task/TASK-ID` vs `$WAVE_BASE_SHA`. Do not scan commit messages:
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

   **Ordering:** do not use `HEAD` or `feat/STORY-ID` here. Merge is step (b), after a.2 and a.3.

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
   
   The bundle is that task's contract plus its own diff, written to `$BUNDLE_FILE`.

2.1. **Activity Index (if bundle has activity section)** — when the bundle includes execution evidence:

   `arm review prepare` discovers the worktree activity log itself. After prepare:
   ```bash
   HAS_ACTIVITY=$(jq -r 'if .activity then "yes" else "no" end' "$BUNDLE_FILE")
   ```

   If `HAS_ACTIVITY` is `yes`, dispatch the **armature-activity-indexer** as a subagent
   before dispatching the reviewer:
   ```
   Dispatch armature-activity-indexer with:
   - the bundle file: $BUNDLE_FILE (or at minimum, the bundle's activity.log_path,
     activity.digest, activity.delivery_head_count, and activity.earlier_count fields —
     read them out with jq if passing the whole file is inconvenient)

   The indexer reads the log at activity.log_path, verifies its digest against
   activity.digest, and returns an Activity Index JSON (schema_version, log_path,
   log_digest, entry_count, delivery_head_count, earlier_count, entries[]) as its
   final text output.
   ```
   Capture the indexer's returned text into a temp file:
   ```bash
   INDEX_OUTPUT=$(mktemp)
   # The indexer subagent's returned text IS the Activity Index JSON.
   # Write it directly to $INDEX_OUTPUT, e.g.:
   #   echo "$INDEXER_OUTPUT" > "$INDEX_OUTPUT"
   # where $INDEXER_OUTPUT is the text returned by the indexer subagent.
   ```

   The Activity Index is a finding aid only. Citations use raw log entry IDs (`"0"`, `"1"`).

3. **Dispatch the armature-reviewer agent** — pass both the bundle and activity index (if available). Assign each reviewer a distinct token (`r1`, `r2`, … or that reviewer's `ARM_LOG_SLOT`) and tell them to write `.armature/review/<issue-id>-<bundle-id-8>-<reviewer-token>.json` (see the reviewer skill). Do not `git add` those files (local recording input); do not delete existing assessments. Independent perspectives run **in parallel** under distinct tokens.
   ```
   Dispatch armature-reviewer with:
   - bundle file: $BUNDLE_FILE (the reviewer reads the bundle from the file)
   - activity index (if $HAS_ACTIVITY was "yes"): pass the contents of $INDEX_OUTPUT as
     additional context so the reviewer can route to raw entry IDs
   - reviewer token: r1   # distinct per parallel reviewer of this issue
   ```
   
   Behavioral activity evidence can lift indeterminate verdicts. It never replaces diff citations and never suppresses a diff-supported `not_satisfied`.

   The reviewer's chat/text response is **not** the `ConformanceAssessment` JSON.
   It is rating + actionable findings + the path to the assessment file under
   `.armature/review/` (see the reviewer skill's bounded chat contract). After
   every reviewer returns, collect those paths. Do
   **not** write any reviewer's chat text to `$RESULT_FILE` — `arm review record`
   will reject a summary as if it were the assessment.

   **Check each reviewer's response shape before collecting anything.** A
   reviewer returns a recordable path **only** on the success shape. Two
   shapes deliberately carry no path, and both end in
   `Assessment: not returned`:

   - `Validation: failed` — the reviewer exhausted its `arm review validate`
     retries. An assessment file may exist on disk, but it never validated.
   - `Validation: error` — `arm review validate` failed operationally
     (unreadable assessment or bundle path, bundle missing an issue ID,
     snapshot load failure, issue absent from state, Step 1 bundle
     preflight, or a `valid: false` report whose only suggestion is to
     re-run `arm review prepare`). Nothing recordable was assessed.

   For either shape, do **not** reconstruct or guess a
   `.armature/review/<issue>-<bundle8>-<token>.json` path, do **not** add one
   to `RESULT_FILES`, and do **not** call `arm review record` for that
   reviewer. A path you assembled yourself is not a validated assessment, and
   recording one asserts a review that did not happen. Instead:

   Track **any unrecovered** no-path response. A nonempty `RESULT_FILES` does
   **not** authorize recording when a sibling returned `Validation: failed`
   or an unrepaired `Validation: error`.

   - `Validation: error` → repair what the reviewer reported (re-run
     `arm review prepare` for a fresh `$BUNDLE_FILE`; confirm the issue
     exists in state). After refreshing `$BUNDLE_FILE`, recompute
     `HAS_ACTIVITY` from the new bundle and rebuild `$INDEX_OUTPUT`
     (same procedure as step 2.1). If `HAS_ACTIVITY` is `yes`, re-dispatch
     **armature-activity-indexer** on this new `$BUNDLE_FILE` into a fresh
     `$INDEX_OUTPUT`. If `no`, leave `$INDEX_OUTPUT` unset — do not pass the old index.
     Drop every path already in `RESULT_FILES` (those assessments are bound
     to the old bundle) and re-dispatch **every reviewer whose result will be recorded**,
     not only the failed one.
     Each re-dispatch is once. If the repaired reviewer returns the same
     shape again, mark it unrecovered and escalate rather than looping.
   - `Validation: failed` → the assessment is not recordable. Record the
     reported failures on the issue, mark that reviewer unrecovered, and
     escalate to a human (Constitution I7); do not treat the issue as
     reviewed.

   ```bash
   arm note --issue "$TASK_ID" --msg "review not recorded: <shape> from <reviewer-token>; <reason>"
   ```

   If **any unrecovered** no-path response remains, stop before step 4 even
   when `RESULT_FILES` is nonempty. Do not record the Green siblings, do
   not synthesize a rating, and do not treat a partial result as Green.
   Stop and escalate.

   ```bash
   # Each reviewer writes a DISTINCT path and returns it. Confirm each
   # file exists and is JSON. Do not share one .armature/review/ file.
   # Only paths from success-shape responses belong here.
   RESULT_FILES=(.armature/review/TASK-ID-bundle8-r1.json .armature/review/TASK-ID-bundle8-r2.json)
   UNRECOVERED=()  # append each token that returned a no-path shape that was not repaired
   if [ ${#UNRECOVERED[@]} -ne 0 ]; then
     echo "ERROR: unrecovered no-path reviewer(s): ${UNRECOVERED[*]}; escalate (I7); do not record" >&2
     exit 1
   fi
   if [ ${#RESULT_FILES[@]} -eq 0 ]; then
     echo "ERROR: no reviewer returned a validated assessment; escalate (I7)" >&2
     exit 1
   fi
   FINDINGS_FILE=$(mktemp)
   # Union every reviewer's chat findings (not the JSON bodies) into
   # $FINDINGS_FILE. Deduplicate by finding text. Confirmation uses
   # this consolidated list as hard scope.
   ```
   Do **not** call `arm review record` until `$FINDINGS_FILE` holds that
   single consolidated list. Then record **every** path in
   `$RESULT_FILES` (step 4) — one findings list, every assessment durable.

4. **Record every assessment** — persist after findings are consolidated.
   `CYCLE` is per-issue. At the start of this task (once, not inside the
   remedia loop), unset any leftover `CYCLE` and recover the highest
   persisted `a.2 cycle N/3` note for **this** `$TASK_ID`; default 0.
   Recovered `N` is **completed** remedia cycles. Do not reuse another
   task's `CYCLE`:
   ```bash
   unset CYCLE
   CYCLE=$(arm show "$TASK_ID" --format json | jq -r '
     [.notes // [] | .[] | strings
       | capture("a\\.2 cycle (?<n>[0-9]+)/3")?
       | select(.) | .n | tonumber]
     | if length == 0 then 0 else max end
   ')
   CYCLE_FOR="$TASK_ID"

   # Derive ratings first, then record least-conservative → most-conservative
   # so the last AssessmentAttestation (what arm show reads) matches $RATING.
   # RESULT_FILE = most conservative path (red > yellow > green). Ties: first
   # of that rating. Prefer .rating if present; else derive from results
   # (any not_satisfied => red; else any partially_satisfied or
   # indeterminate => yellow; else green).
   greens=()
   yellows=()
   reds=()
   best_rank=0
   RESULT_FILE="${RESULT_FILES[0]}"
   RATING=""
   for f in "${RESULT_FILES[@]}"; do
     test -s "$f" || { echo "ERROR: assessment missing or empty: $f; do not enter a.3" >&2; exit 1; }
     rating=$(jq -r '
       if .rating then (.rating | tostring | ascii_downcase)
       elif (.results | type) == "array" then
         if any(.results[]; .status == "not_satisfied") then "red"
         elif any(.results[]; .status == "partially_satisfied" or .status == "indeterminate") then "yellow"
         else "green" end
       else empty end
     ' "$f")
     case "$rating" in
       red) rank=3; reds+=("$f") ;;
       yellow) rank=2; yellows+=("$f") ;;
       green) rank=1; greens+=("$f") ;;
       *)
         echo "ERROR: $f has no .rating/.results; read that reviewer's chat Rating: Green|Yellow|Red and set RATING; do not enter a.3" >&2
         exit 1
         ;;
     esac
     if [ "$rank" -gt "$best_rank" ]; then
       best_rank=$rank
       RESULT_FILE="$f"
       RATING="$rating"
     fi
   done
   if [ -z "$RATING" ]; then
     echo "ERROR: no assessment rating derivable; do not enter a.3" >&2
     exit 1
   fi
   SORTED_RESULT_FILES=("${greens[@]}" "${yellows[@]}" "${reds[@]}")
   for ASSESSMENT in "${SORTED_RESULT_FILES[@]}"; do
     arm review record --issue "$TASK_ID" --assessment "$ASSESSMENT" --bundle "$BUNDLE_FILE" \
       || { echo "ERROR: review record failed for $ASSESSMENT; do not enter a.3" >&2; exit 1; }
   done
   ```
   Pass each `--assessment` path and `--bundle "$BUNDLE_FILE"` as file paths (not raw JSON content) so each recorded assessment is bound to the exact bundle (and its durable identity) the reviewer evaluated.

   Loop-control rating is `$RATING` from the conservative `$RESULT_FILE` above.
   An empty `$RATING` already exited. **Green:** this task is done with a.2 —
   skip steps 5–7. **Yellow or red:** if recovered `CYCLE` is already 3,
   escalate (step 7's cycle-3 branch) and do not remedia. Otherwise set
   `CYCLE=$((CYCLE + 1))` (recovered `N` is completed; next remedia is
   `N+1`) and enter the remedia loop at step 5. Do not start a.3 or merge
   on a non-green rating.

5. **Reopen and reclaim before remediating.** Semantic review runs after the
   worker has already transitioned to `done`. The remediator must re-enter
   the live-claim lifecycle before writing. Do this **before** any
   remediating edit, and **before** `arm merged` (merged is terminal —
   `arm reopen` refuses it).

   `applyHeartbeat` requires `op.WorkerID == issue.ClaimedBy`. ClaimedBy is
   the slotted identity written at claim time (`<worker-id>~<slot>`). Claim
   **as the remediator**: set `ARM_LOG_SLOT` on the same invocation as
   `arm claim` (prefix form below). Do not `export` then claim in a later
   tool call — a fresh-shell harness drops the export and claims unslotted.
   Prefix keeps later coordinator ops on the unslotted log with no `unset`.

   `REMEDIATOR_SLOT` must be unique across the wave (I3). Default:
   `rem-${TASK_ID}` — self-contained, unique per task, valid in
   `ARM_LOG_SLOT` (`^[A-Za-z0-9_-]+$`). Do not invent a shared `t1`
   and do not rely on a remembered original dispatch slot.
   Two concurrent remediations must not share one slotted op log.

   If `CYCLE` is not bound to this `$TASK_ID` (unset, or leftover from
   another task), unset it and recover with the step-4 `arm show` / jq
   snippet before claiming. Do not unset on remedia-loop re-entry for
   the same task — that would drop an in-memory increment. If recovered
   `CYCLE` is already 3, skip to step 7's cycle-3 escalate branch — do
   not remedia again.
   ```bash
   if [ "${CYCLE_FOR:-}" != "$TASK_ID" ]; then
     unset CYCLE
     CYCLE=$(arm show "$TASK_ID" --format json | jq -r '
       [.notes // [] | .[] | strings
         | capture("a\\.2 cycle (?<n>[0-9]+)/3")?
         | select(.) | .n | tonumber]
       | if length == 0 then 0 else max end
     ')
     CYCLE_FOR="$TASK_ID"
     if [ "$CYCLE" -ge 3 ]; then
       CLAIMED_BY=$(arm show "$TASK_ID" --field claimed_by)
       arm note --issue "$TASK_ID" --msg "a.2 cycle 3/3 recovered; escalated (I7); claim=${CLAIMED_BY:-unset} worktree=.worktrees/$TASK_ID"
       echo "ERROR: recovered cycle $CYCLE; escalate I7; do not remedia / enter a.3" >&2
       exit 1
     fi
     CYCLE=$((CYCLE + 1))
   fi
   # Unique per wave task; rem-${TASK_ID} needs no prior slot memory (not literal t1).
   # Run this block as one shell invocation. ARM_LOG_SLOT is a one-shot prefix
   # on claim — do not split the prefix and arm claim across tool calls.
   REMEDIATOR_SLOT="rem-${TASK_ID}"
   arm reopen "$TASK_ID"
   ARM_LOG_SLOT="$REMEDIATOR_SLOT" arm claim "$TASK_ID" --ttl 120 --worktree
   CLAIMED_BY=$(arm show "$TASK_ID" --field claimed_by)
   BASE_ID=$(arm worker-init --check | awk '/^Worker ID:/{print $3}')
   EXPECTED="${BASE_ID}~${REMEDIATOR_SLOT}"
   if [ "$CLAIMED_BY" != "$EXPECTED" ]; then
     echo "ERROR: ClaimedBy=$CLAIMED_BY expected $EXPECTED (ARM_LOG_SLOT dropped before claim?)" >&2
     exit 1
   fi
   arm render-context "$TASK_ID" --format agent
   ```
   `arm claim --worktree` reuses the existing `.worktrees/TASK-ID` checkout
   and rewrites its `armature-issue-id` binding; do not `git worktree add`
   a second tree. Then dispatch the remediator like any other pre-claimed
   worker (Dispatch Protocol) with `export ARM_LOG_SLOT=$REMEDIATOR_SLOT`
   as its second instruction, stating only what changed. The remediator
   iterates on the fast gate, commits, runs the full/publish gate at the
   new delivery HEAD, and runs `arm transition TASK-ID --to done` so the
   delivery gate evaluates that HEAD. Do not dispatch remediations onto a
   `done` or `merged` task.

6. **Confirmation after remediation — one protocol.** After the remediator
   commits and transitions to `done`, `task/$TASK_ID` has a new delivery
   HEAD. Refresh every **stale** artifact, keep the remediating findings
   as scope, then dispatch confirmation (not another comprehensive review):
   ```bash
   unset RESULT_FILE INDEX_OUTPUT
   # Do not unset FINDINGS_FILE — it is the confirmation scope.
   TASK_COMMITS["$TASK_ID"]="$WAVE_BASE_SHA..task/$TASK_ID"
   TASK_HEAD=$(git rev-parse "task/$TASK_ID")
   BUNDLE_FILE=$(mktemp)
   arm review prepare --issue "$TASK_ID" \
     --base "$TASK_BASE" --head "$TASK_HEAD" \
     --output "$BUNDLE_FILE"
   HAS_ACTIVITY=$(jq -r 'if .activity then "yes" else "no" end' "$BUNDLE_FILE")
   ```
   If `HAS_ACTIVITY` is `yes`, re-dispatch **armature-activity-indexer** on
   this new `$BUNDLE_FILE` into a fresh `$INDEX_OUTPUT` (same procedure as
   step 2.1). If `no`, leave `$INDEX_OUTPUT` unset — do not pass the old
   index. Then dispatch:
   ```
   Dispatch armature-reviewer in confirmation mode with:
   - bundle file: $BUNDLE_FILE
   - activity index (if $HAS_ACTIVITY was "yes"): $INDEX_OUTPUT
   - findings scope: $FINDINGS_FILE
     (the consolidated remediating set from the initial review)
   - reviewer token: confirm-$CYCLE   # expands; distinct from first-pass tokens
   - instruction: hard-scoped confirmation of those findings only;
     do not start a new comprehensive review. Out-of-scope findings
     are recorded but block only at critical severity.
   ```
   After the confirmation reviewer returns, apply the **same
   response-shape branch** as initial collection **before** assigning
   `$RESULT_FILE`:

   - Success shape → assign the returned path (token `confirm-$CYCLE`) and
     record it against the refreshed bundle. The refresh `unset` dropped
     the first-pass path so it cannot be reused.
   - `Validation: error` / `Validation: failed` / `Assessment: not returned` →
     do **not** treat the chat as a filename, do **not**
     `test -s` a guessed path, and do **not** call `arm review record`.
     Route as in step 3: repair-once (and re-dispatch every reviewer whose result will be recorded) for `Validation: error`; `arm note`
     and I7 escalate for `Validation: failed`.
   ```bash
   # Only after a success-shape response (distinct token confirm-$CYCLE).
   RESULT_FILE="<path from confirmation reviewer success chat>"
   test -s "$RESULT_FILE" || { echo "ERROR: confirmation assessment missing" >&2; exit 1; }
   arm review record --issue "$TASK_ID" --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE" \
     || { echo "ERROR: review record failed for $RESULT_FILE; do not enter a.3" >&2; exit 1; }
   ```
   Reusing the pre-remediation bundle lets a green confirmation attest
   the old delivery SHA, fingerprints, and diff. Reusing the
   pre-remediation index omits post-remediation evidence and routes the
   reviewer to entries whose `head_sha` is not the new bundle head.
   Reusing the first-pass `$RESULT_FILE` records the first-pass
   assessment against the new bundle. Omitting `$FINDINGS_FILE` (or
   relying on a prior reviewer transcript) turns confirmation into a
   second comprehensive review and restarts the discovery/remediation
   loop. Step 7 inspects this recorded confirmation — do not fall
   through to a.3 from here.

7. **Inspect confirmation rating; loop or escalate.** After the
   confirmation `arm review record` in step 6, derive `$RATING` from
   that `$RESULT_FILE` (`.results` only — assessments have no
   `.rating`), persist the cycle, and gate on the rating:
   ```bash
   RATING=$(jq -r '
     if (.results | type) == "array" then
       if any(.results[]; .status == "not_satisfied") then "red"
       elif any(.results[]; .status == "partially_satisfied" or .status == "indeterminate") then "yellow"
       else "green" end
     else empty end
   ' "$RESULT_FILE")
   if [ -z "$RATING" ]; then
     echo "ERROR: confirmation rating empty; read chat Rating: Green|Yellow|Red then set RATING; do not enter a.3" >&2
     exit 1
   fi
   if [ "${CYCLE:-0}" -eq 0 ]; then CYCLE=1; fi
   # Persist the cycle just completed only. Recovery takes max N; a pending
   # next-cycle note would make a fresh context escalate after 2 remediations.
   arm note --issue "$TASK_ID" --msg "a.2 cycle $CYCLE/3 rating=$RATING"
   case "$RATING" in
     green) ;;
     yellow|red)
       if [ "$CYCLE" -ge 3 ]; then
         CLAIMED_BY=$(arm show "$TASK_ID" --field claimed_by)
         arm note --issue "$TASK_ID" --msg "a.2 cycle 3/3 $RATING; escalated (I7); claim=${CLAIMED_BY:-unset} worktree=.worktrees/$TASK_ID"
         echo "ERROR: cycle $CYCLE still $RATING; escalate I7; do not enter a.3 / arm merged" >&2
         exit 1
       fi
       CYCLE=$((CYCLE + 1))
       echo "NON-GREEN: cycle $CYCLE $RATING; replace FINDINGS_FILE; repeat steps 5-6; do not enter a.3" >&2
       exit 1
       ;;
     *)
       echo "ERROR: unknown confirmation rating '$RATING'; do not enter a.3" >&2
       exit 1
       ;;
   esac
   ```
   Non-green always stops the snippet (`exit 1`); linear fall-through
   to a.3 is only for green. Do not re-derive or re-increment after
   the snippet exits.
   - **Green:** `;;` — this task is done with a.2 and may proceed to
     a.3. Do not remediate again.
     Residual risk: confirmation-green means the remediating set was
     fixed, not that the task is comprehensively clean. Remediating
     edits can introduce regressions that stay invisible unless they
     are critical (confirmation mode: out-of-scope findings block only
     at critical severity). That is the T1 trade-off; keep the loop
     as specified.
   - **Yellow or red, and `CYCLE` was < 3:** the snippet persisted the
     completed cycle, incremented `CYCLE` in memory only, and exited 1.
     The agent remediates: replace `$FINDINGS_FILE` with the
     confirmation's remaining findings (the new remediating set) and
     repeat steps 5–6. At `CYCLE` 2, auto-escalate remediator effort to
     high (Dispatch Protocol).
   - **Yellow or red, and `CYCLE` was already 3:** the snippet noted the
     surviving claim and worktree, then exited. The agent escalates to
     the human (Constitution I7). Do not enter a.3, do not merge this
     task branch, do not run `arm merged` for this task.

   Confirmation-green tasks may proceed to a.3 / merge individually.
   Cycle-3 escalated tasks stay out of a.3, `arm merged`, and branch
   merge pending the human (I7). The wave is not stalled on one
   escalation. A non-green confirmation never enters a.3.

The reviewer checks semantic conformance. The auditor checks citation coverage and repo health. Both must pass before story sign-off.

### a.3. Parallel Branch Overlap Audit

Same-wave tasks can semantically revert each other. After all wave tasks are `done`, run `references/overlap-audit.md` before merge.

### b. Check for scope conflicts and merge conflicts

After a.2 and a.3, merge each `task/TASK-ID` into `feat/STORY-ID`. Resolve conflicts. Only then are task commits reachable from story `HEAD`.

### c. Wave Verification Gate

Do not run `arm merged` until `references/wave-verification.md` passes. Failed tasks stay `done`.

### d. Mark completed tasks merged (with violation gate)

Promote `done` → `merged` only after the gate. `arm merged --issue TASK-ID` fails closed on `violation:` entries unless `--force`. Procedure: `references/violation-gate.md`.

```bash
for TASK_ID in $WAVE_TASK_IDS; do
  arm merged --issue TASK_ID
done
```

### e. Check citation coverage

```bash
arm validate
```

Uncited node:

```bash
arm sources link --issue ID --source-id SOURCE-UUID
# or
arm sources accept-citation --issue ID --rationale "No external source; self-citing" --ci
```

### f. Clean up worktrees

Review-then-teardown per task (`references/violation-gate.md`). Then:

```bash
git worktree list
git worktree remove <path> --force
git branch -d <worker-branch>
```

### g. Continue to next wave

```bash
arm ready
```

---

## Story Completion

When `arm ready` is empty and all tasks are `done`:

### 1. Run the Auditor (pre-merge gate)

Dispatch **armature-auditor** (`Skill("armature-auditor")`) before any story transition. Five checks: `arm validate`, `arm sources verify`, outcome quality, `arm validate --strict`, `arm doctor --strict`. Do not proceed until all five are green.

### 2. Transition the story

```bash
arm transition STORY-ID --to done --outcome "brief summary of what was delivered"
```

### 3. Verify armature ops

Ops commit on `_armature` after each command. No manual ops commit.

### 4. Push and open PR

```bash
git push -u origin HEAD
```

One PR per story. Title is the story title. Body lists each ISSUE-ID and its outcome.

Failure table: `references/failure-modes.md`.
