# E5-S1 Dogfooding UX Fixes — T9–T13

Friction points surfaced during E5-S1 implementation. Five targeted fixes to `arm` CLI, `Makefile`, and skill documentation.

---

## T9 — Fix `arm show` to return human-readable output

**Issue:** `arm show --issue ISSUE-ID` returned empty output during E5-S1 implementation. Workers had to `grep` op files directly to inspect issue state.

**Scope:** `cmd/trellis/status.go` (or new `show.go`), `cmd/trellis/main_test.go`

**Definition of Done:** `arm show --issue ISSUE-ID` prints a human-readable summary of the materialized issue state (ID, title, type, status, parent, claimed_by, DoD, notes); `--format json` outputs structured data; `TestShowCmd` passes; make check green.

---

## T10 — `arm doctor` command

**Issue:** No command exists to surface structural repo health problems — git/trellis divergence (commits referencing issues that aren't `done`), stale claims, orphaned ops, broken parent refs, dependency cycles, and uncited issues.

**Scope:** `cmd/trellis/doctor.go`, `internal/doctor/` package, `cmd/trellis/main_test.go`, `docs/SKILL.md`, `docs/arm-worker-SKILL.md`

**Definition of Done:** `arm doctor` runs six checks (D1–D6 below) and exits non-zero on any error-severity failure; `--strict` promotes warnings to errors; `--format json` outputs structured results; integrated into `docs/SKILL.md` (Repo Health section) and `docs/arm-worker-SKILL.md` (worker-init step); tests pass; make check green.

### Checks

| ID | Severity | Description |
|----|----------|-------------|
| D1 | warning | **Git/trellis divergence** — scan `git log` for `(ISSUE-ID)` patterns; warn for any referenced issue not in `done`/`merged` |
| D2 | warning | **Stale claims** — issues in `claimed` state with expired TTL |
| D3 | error   | **Orphaned ops** — op files referencing issue IDs not in the graph |
| D4 | error   | **Broken parent refs** — issues whose `parent` field points to a non-existent ID |
| D5 | error   | **Dependency cycles** — `blocked_by` chains that form a cycle |
| D6 | warning | **Uncited issues** — issues without `source-link` or `accept-citation` |

### Output format (human)

```
arm doctor
  ✓ git/trellis sync
  ✗ stale claims (2):
      E5-S2-T1  claimed 47m ago, TTL=60m (worker: abc123)
  ✓ orphaned ops
  ✓ broken parent refs
  ✓ dependency cycles
  ⚠ uncited issues (3): E5-S2-T4  E5-S2-T5  E5-S2-T6
```

### Skill integration

- **`docs/SKILL.md`** — add "Repo Health" section documenting `arm doctor`
- **`docs/arm-worker-SKILL.md`** — add `arm doctor` call to worker-init step; workers should stop and surface errors before claiming work

---

## T11 — `--parent` filter on `arm ready`

**Issue:** `arm ready --story E5-S1` silently returned the full queue. Workers focused on a specific story have no way to scope the ready queue.

**Scope:** `cmd/trellis/ready.go`, `cmd/trellis/cmd_extra_test.go`

**Definition of Done:** `arm ready --parent ISSUE-ID` returns only tasks that are descendants of the given issue; `TestReadyParentFilter` passes; make check green.

---

## T12 — Add `make skill` to `make check`

**Issue:** After updating `docs/SKILL.md` in T7, `make skill` had to be run manually to regenerate `.claude/skills/arm/SKILL.md`. The deployed skill can silently drift from the source docs. The `.claude/skills/` build output is intentionally gitignored (dogfooding only), so the fix is to ensure `make check` always regenerates skills so any drift is caught during local development.

**Scope:** `Makefile`

**Definition of Done:** `make check` runs `make skill` as a step (after `build`, before or after `lint`); the deployed skill is always fresh after any `make check` run; make check green.

---

## T13 — Heartbeat instructions in arm-worker subagent prompts

**Issue:** The arm-worker SKILL.md dispatches subagents but doesn't instruct them to call `arm heartbeat`. Long-running subagents (T3 took 28m, T8 took 13m) risk TTL expiry and claim theft by other workers.

**Scope:** `docs/arm-worker-SKILL.md`

**Definition of Done:** The "Dispatch Subagent" section of `docs/arm-worker-SKILL.md` explicitly instructs subagents to call `arm heartbeat --issue ID` for long-running work (with a note on the max-once-per-minute constraint); `make skill` regenerates the deployed skill; make check green.
