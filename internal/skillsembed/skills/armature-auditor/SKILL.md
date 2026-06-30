---
name: armature-auditor
description: >
  Use when verifying completed work before story sign-off — checks citation
  coverage, source UUID integrity, outcome quality, and repo health. Runs
  validate, sources verify, render-context, and doctor --strict.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Auditor

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
arm validate
```

Expected output: `COVERAGE: N/N cited` with **zero ERROR lines**.

If `arm validate` reports ERROR lines, read `references/citation-errors.md`.

For CI use (exits non-zero on any error):

```bash
arm validate --ci
```

To validate only a subtree:

```bash
arm validate --scope STORY-ID
```

### Step 2 — Source Freshness

```bash
arm sources verify
```

All sources must show `OK`. Any `MISSING` entry means a source fingerprint is stale or the source was removed after initial registration.

If sources show `MISSING`:

```bash
arm sources sync        # fetch and re-fingerprint all sources
arm sources verify      # re-run until all show OK
```

If a source is gone entirely, re-register it:

```bash
arm sources add <url-or-path>
```

If source content changed and you need to review the delta before accepting it:

```bash
arm stale-review        # interactive review of sources whose content changed
```

### Step 3 — Outcome Quality Review

List all terminal tasks under the story (done, merged, and cancelled):

```bash
arm list --terminal --parent STORY-ID
```

For each completed task, inspect its outcome against its acceptance criteria:

```bash
arm render-context --issue ISSUE-ID
```

`render-context` shows both the `acceptance` criteria array and the recorded `outcome` field side by side. Verify that:

- The outcome is **concrete** — it describes what was built, references specific files, commands, or test results, and cites metrics where applicable.
- The outcome **addresses every acceptance criterion** — if `acceptance` lists "TestDoctorVerbose passes" and the outcome says "done", that is a gap.
- The outcome is **not vague** — flag "Done", "Fixed", "Implemented", "Completed" as insufficient before sign-off.

You can also inspect an issue directly:

```bash
arm show ISSUE-ID      # shows outcome and acceptance criteria side by side
```

**Good outcome example:**
> "Added `--verbose` flag to `arm doctor`; each violation now prints the op file path and affected issue ID; 3 new tests added; make check green at 82% coverage"

**Vague outcome examples (flag these):**
> "Done" / "Fixed" / "Implemented as requested" / "Completed"

### Step 4 — Scope Overlap Resolution

`arm validate` may report `WARNING: scope overlap` lines. These are not errors by default, but **must be resolved before sign-off**.

```bash
# Make overlapping tasks serial
arm link --source ISSUE-A --dep ISSUE-B

# Or treat overlaps as errors to confirm none remain
arm validate --strict
```

If two tasks genuinely do not overlap despite the warning, document the rationale with `arm decision` on one of the issues before proceeding.

### Step 5 — Repo Health

```bash
arm doctor --strict
```

`--strict` promotes all warnings to errors. The command must **exit zero**. Any warning or error must be resolved before approving the story.

For machine-readable output:

```bash
arm doctor --format json
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

## Pre-Merge Gate

Before approving the story transition, all five checks must be green:

| Check | Command | Pass Condition |
|-------|---------|----------------|
| Citation integrity | `arm validate` | Zero ERRORs, `COVERAGE: N/N cited` |
| Source freshness | `arm sources verify` | Zero MISSING |
| Outcome quality | `arm render-context --issue ID` for each done task | All outcomes concrete, all acceptance criteria addressed |
| Scope overlap | `arm validate --strict` | Zero scope overlap warnings |
| Repo health | `arm doctor --strict` | Exit zero (zero ERRORs, zero WARNINGs) |

Only after all five pass should you approve the story for transition and PR.

## Common Failure Modes

| Failure Mode | Why It Happens | Fix |
|---|---|---|
| Trusting D6 alone for citation integrity | `arm doctor` D6 checks field presence only — it will pass even if the source UUID is invalid (E8) | Always run `arm validate` after `arm doctor`; D6 and E8 check different things |
| Accepting vague outcomes ("done", "fixed") | Worker transitions without writing a concrete outcome | Use `arm render-context --issue ID` to cross-check outcome against `acceptance` criteria; require workers to amend before sign-off |
| Skipping `arm sources verify` | Source fingerprints go stale silently when documents are updated after initial registration | Always run `arm sources verify` as step 2; run `arm sources sync` to refresh, then re-verify |
