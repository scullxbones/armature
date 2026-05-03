# Parallel Dispatch

## 4. Parallel Dispatch (independent tasks in one wave)

Use parallel dispatch for tasks with no dependencies between them.

**a. Assign log slots and pre-assign workers (optional but recommended):**
```bash
arm assign --issue T1-ID --worker WORKER-A
arm assign --issue T2-ID --worker WORKER-B
```

**b. Claim all tasks in the wave:**
```bash
arm claim --issue T1-ID
arm claim --issue T2-ID
```

**c. Render context for each:**
```bash
arm render-context --issue T1-ID --budget 4000
arm render-context --issue T2-ID --budget 4000
```

**d. Dispatch all workers concurrently** — include the slot and full context in
each prompt (see Dispatch Protocol in the main skill and Log Slots below).

**e. Wait for all workers to return before proceeding.**

**f. Verify and integrate** (see After Workers Return in the main skill).

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
