<!-- CANONICAL SOURCE: edit this file, not .claude/skills/trls-auditor/SKILL.md — run `make skill` to regenerate the deployed copy -->

# Trellis Auditor

The Auditor verifies that completed work is honest and traceable before story sign-off. Every issue must have a valid cited source, every decision must be recorded, and outcomes must satisfy acceptance criteria.

## When to Run

The Auditor runs **after** workers report tasks done and **before** story transition and PR. It is a gate, not a monitor.

```
Workers complete tasks → Auditor runs → Story transitions to done → PR opened
```

Do not approve a story transition until all audit checks pass. If any check fails, report the specific issues back to the workers for remediation before re-auditing.

## The Audit Checklist

Work through these steps in order. Each step must pass before proceeding to the next.

### Step 1 — Citation Integrity

```bash
trls validate
```

Expected output: `COVERAGE: N/N cited` with **zero ERROR lines**.

If you see `ERROR: uncited node` or `ERROR: unknown source`, see the [Citation Integrity](#citation-integrity-e7--e8) section below for fixes.

For CI use (exits non-zero on any error):

```bash
trls validate --ci
```

To validate only a subtree:

```bash
trls validate --scope STORY-ID
```

### Step 2 — Source Freshness

```bash
trls sources verify
```

All sources must show `OK`. Any `MISSING` entry means a source fingerprint is stale or the source was removed after initial registration.

If sources show `MISSING`:

```bash
trls sources sync        # fetch and re-fingerprint all sources
trls sources verify      # re-run until all show OK
```

If a source is gone entirely, re-register it:

```bash
trls sources add <url-or-path>
```

If source content changed and you need to review the delta before accepting it:

```bash
trls stale-review        # interactive review of sources whose content changed
```

### Step 3 — Outcome Quality Review

List all completed tasks under the story:

```bash
trls list --status done --parent STORY-ID
```

For each completed task, inspect its outcome against its acceptance criteria:

```bash
trls render-context --issue ISSUE-ID
```

`render-context` shows both the `acceptance` criteria array and the recorded `outcome` field side by side. Verify that:

- The outcome is **concrete** — it describes what was built, references specific files, commands, or test results, and cites metrics where applicable.
- The outcome **addresses every acceptance criterion** — if `acceptance` lists "TestDoctorVerbose passes" and the outcome says "done", that is a gap.
- The outcome is **not vague** — flag "Done", "Fixed", "Implemented", "Completed" as insufficient before sign-off.

You can also inspect an issue directly:

```bash
trls show ISSUE-ID      # shows outcome and acceptance criteria side by side
```

**Good outcome example:**
> "Added `--verbose` flag to `trls doctor`; each violation now prints the op file path and affected issue ID; 3 new tests added; make check green at 82% coverage"

**Vague outcome examples (flag these):**
> "Done" / "Fixed" / "Implemented as requested" / "Completed"

### Step 4 — Scope Overlap Resolution

`trls validate` may report `WARNING: scope overlap` lines. These are not errors by default, but **must be resolved before sign-off**.

```bash
# Make overlapping tasks serial
trls link --source ISSUE-A --dep ISSUE-B

# Or treat overlaps as errors to confirm none remain
trls validate --strict
```

If two tasks genuinely do not overlap despite the warning, document the rationale with `trls decision` on one of the issues before proceeding.

### Step 5 — Repo Health

```bash
trls doctor --strict
```

`--strict` promotes all warnings to errors. The command must **exit zero**. Any warning or error must be resolved before approving the story.

For machine-readable output:

```bash
trls doctor --format json
```

Doctor checks reference:

| Code | Severity | Check |
|------|----------|-------|
| D1 | warning | Git commits reference non-done issues |
| D2 | warning | Claimed issues with expired TTL (stale claims) |
| D3 | error | Op files reference issue IDs not in graph |
| D4 | error | Issues whose parent points to non-existent ID |
| D5 | error | `blocked_by` chains forming a dependency cycle |
| D6 | warning | Issues without `source-link` or `accept-citation` (field-presence only — see caveat below) |

## Citation Integrity (E7 + E8)

### E7 — Uncited Node

```
ERROR: uncited node: ISSUE-ID
```

An issue has neither a `source-link` nor an `accept-citation`. It is completely untraced.

**Fix:**

```bash
# Link to a source document
trls source-link ISSUE-ID

# Or accept the citation risk explicitly (for issues with no recoverable source)
trls accept-citation --ci ISSUE-ID
```

### E8 — Unknown Source

```
ERROR: unknown source: UUID in citation for ISSUE-ID
```

An issue's `source-link` points to a UUID that no longer exists in the sources manifest. This happens when a source was registered, used in a citation, then deleted from the manifest.

**Fix:**

```bash
trls sources sync          # refresh manifest; re-fingerprint all sources
trls sources verify        # confirm all show OK
trls validate              # re-run — E8 should be gone if the source was re-found
```

If the source is gone permanently, register a replacement and re-link:

```bash
trls sources add <replacement-url-or-path>
trls source-link ISSUE-ID  # link to the new source UUID
trls validate              # confirm E8 is resolved
```

### CRITICAL: D6 Does Not Catch E8

> **WARNING: `trls doctor` D6 checks field presence only.** It verifies that `source_link` or `citation_acceptance` fields exist on an issue — but it does **not** verify that the source UUID actually exists in the manifest.
>
> **An issue can pass D6 while still failing E8 in `trls validate`.**
>
> Always run both:
> - `trls doctor` — structural health (field presence, parent refs, dependency cycles)
> - `trls validate` — semantic citation validity (UUID integrity, coverage)
>
> Never rely on D6 alone as proof of citation integrity.

## Source Freshness

Source fingerprints go stale when the underlying document changes after initial registration. `trls sources verify` detects this; `trls sources sync` re-fetches and re-fingerprints all sources.

Workflow when sources are stale:

```bash
trls sources verify        # identify MISSING or changed sources
trls sources sync          # re-fingerprint
trls sources verify        # confirm all OK
trls stale-review          # if content changed, review delta before accepting
trls validate              # confirm no new E8 errors from stale UUIDs
```

Sources can also go stale silently between the time a worker registers them and the time the auditor runs. Always run `trls sources verify` as step 2 of the audit — do not assume sources registered during implementation are still current.

## Pre-Merge Gate

Before approving the story transition, all five checks must be green:

| Check | Command | Pass Condition |
|-------|---------|----------------|
| Citation integrity | `trls validate` | Zero ERRORs, `COVERAGE: N/N cited` |
| Source freshness | `trls sources verify` | Zero MISSING |
| Outcome quality | `trls render-context --issue ID` for each done task | All outcomes concrete, all acceptance criteria addressed |
| Scope overlap | `trls validate --strict` | Zero scope overlap warnings |
| Repo health | `trls doctor --strict` | Exit zero (zero ERRORs, zero WARNINGs) |

Only after all five pass should you approve the story for transition and PR.

## Common Failure Modes

| Failure Mode | Why It Happens | Fix |
|---|---|---|
| Trusting D6 alone for citation integrity | `trls doctor` D6 checks field presence only — it will pass even if the source UUID is invalid (E8) | Always run `trls validate` after `trls doctor`; D6 and E8 check different things |
| Accepting vague outcomes ("done", "fixed") | Worker transitions without writing a concrete outcome | Use `trls render-context --issue ID` to cross-check outcome against `acceptance` criteria; require workers to amend before sign-off |
| Skipping `trls sources verify` | Source fingerprints go stale silently when documents are updated after initial registration | Always run `trls sources verify` as step 2; run `trls sources sync` to refresh, then re-verify |
