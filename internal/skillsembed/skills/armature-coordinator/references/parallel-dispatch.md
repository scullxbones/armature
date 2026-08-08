# Parallel Dispatch

## Wave Planning with `arm ready --waves`

Before attempting manual scope analysis, use `arm ready --waves` to compute scope-disjoint wave partitions automatically:

```bash
arm ready --waves --format json
```

This produces a JSON output grouping ready-eligible issues into waves based on file-level scope disjointness (respecting priority-tier boundaries and excluding ancestor/descendant pairs):

```json
{
  "waves": [
    [{"issue": "STORY-S1-T1", "title": "...", "scope": [...]}, ...],
    [{"issue": "STORY-S1-T2", "title": "...", "scope": [...]}, ...],
    ...
  ]
}
```

Each wave is a suggested grouping of tasks that can proceed in parallel without file-level scope conflicts. Use this output to identify candidate dispatch batches instead of manually eyeballing scope overlap yourself.

### Important Limitations and Guarantees

**File-level scope disjointness only:** `--waves` guarantees that tasks in the same wave touch no overlapping files. However, it does NOT guarantee freedom from **shared-contract drift** — two tasks can touch completely different files while both depending on a shared interface, struct field, or protocol definition, and if both try to evolve that contract differently, they will conflict semantically even though no files overlap. See ADR 0012 for details.

**Advisory guidance only:** The `--waves` output is pre-dispatch planning guidance computed at query time. It is NOT a persisted record and does NOT replace the Coordinator's existing wave manifest step (described below). If issues are claimed or transitioned between the `--waves` query and actual dispatch, the output can diverge from what actually gets dispatched.

**Post-wave overlap audit is mandatory:** Even when using `--waves` to plan a wave, the Coordinator must continue to run its post-wave Parallel Branch Overlap Audit (documented in the "Record Wave Manifest" step below) after the wave completes. This audit catches semantic conflicts and shared-contract drift that file-level partitioning cannot see.

---

## 4. Parallel Dispatch (independent tasks in one wave)

Use parallel dispatch for tasks with no dependencies between them.

**a. Assign log slots and pre-assign workers (optional but recommended):**
```bash
arm assign --issue T1-ID --worker WORKER-A
arm assign --issue T2-ID --worker WORKER-B
```

**b. Claim all tasks in the wave:**
```bash
arm claim --issue T1-ID --worktree
arm claim --issue T2-ID --worktree
```

**c. Render context for each:**
```bash
arm render-context --issue T1-ID --budget 4000
arm render-context --issue T2-ID --budget 4000
```

**d. Dispatch all workers concurrently** — include the slot and full context in
each prompt (see Dispatch Protocol in the main skill and Log Slots below).

**e. Wait for all workers to return before proceeding.**

**f. Record wave manifest** — Before proceeding to integration, record the actual tasks dispatched in this wave by capturing `WAVE_TASK_IDS` and `WAVE_BASE_SHA` as described in the "Record Wave Manifest" step elsewhere in the Coordinator skill. This prose-recorded manifest is unchanged and required, even if you used `arm ready --waves` to plan the wave. The wave manifest remains the authoritative source of truth for what was actually dispatched.

**g. Verify and integrate** (see After Workers Return in the main skill).

---

## Log Slots for Parallel Dispatch

When multiple agents run concurrently, they each write ops to `.armature/`.
Without log slots, all agents write to the same log file, causing races and
losing per-agent attribution.

**How slots work:**

- Each agent sets `ARM_LOG_SLOT` before its first `arm` command.
- Ops go to `<worker-id>~<slot>.log` instead of `<worker-id>.log`.
- The coordinator's own shell must have `ARM_LOG_SLOT` **unset** so its ops
  (claims, story transitions) land in the plain `<worker-id>.log`.

**Assigning slots:**

Use the short task ID or a single letter as the slot:

| Agent | Task | Slot |
|---|---|---|
| Worker A | T1-ID | `t1` |
| Worker B | T2-ID | `t2` |
| Worker C | T3-ID | `t3` |

**Critical:** When dispatching via an AI platform's native agent tool (not a
shell subprocess), each agent runs in its own isolated shell. The coordinator's
`export ARM_LOG_SLOT=...` is never inherited. The slot **must** be embedded
verbatim as the first instruction in each agent's prompt:

```
Before running any arm command, run: export ARM_LOG_SLOT=t1
```

**Rules:**
- Coordinator always runs with `ARM_LOG_SLOT` unset.
- Each parallel agent sets a distinct slot before any `arm` call.
- Slot names must be unique within a batch — reusing a slot defeats the purpose.
- Slot log files are committed alongside code, just like the plain log.
