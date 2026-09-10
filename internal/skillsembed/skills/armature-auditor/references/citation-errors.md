# Citation Error Remediation

## Citation Integrity (E7 + E8)

### E7 — Uncited Node

```
ERROR: uncited node: ISSUE-ID
```

An issue has neither a `sources link` nor an `sources accept-citation`. It is completely untraced.

**Fix:**

```bash
# Link to a source document
# [escape hatch] Prefer --source at apply.
arm sources link ISSUE-ID --source-id SOURCE-UUID

# Or accept the citation risk explicitly (for issues with no recoverable source)
arm sources accept-citation --ci ISSUE-ID --rationale "No recoverable source remains"
```

### E8 — Unknown Source

```
ERROR: unknown source: UUID in citation for ISSUE-ID
```

An issue's `sources link` points to a UUID that no longer exists in the sources manifest. This happens when a source was registered, used in a citation, then deleted from the manifest.

**Fix:**

```bash
arm sources sync          # refresh manifest; re-fingerprint all sources
arm sources verify        # [escape hatch] confirm all show OK
arm validate              # re-run — E8 should be gone if the source was re-found
```

If the source is gone permanently, register a replacement and re-link:

```bash
arm sources add --url /path/to/replacement --type filesystem
arm sources link ISSUE-ID --source-id SOURCE-UUID  # [escape hatch] link to the new source UUID
arm validate              # confirm E8 is resolved
```

### CRITICAL: D6 Does Not Catch E8

> **WARNING: `arm doctor` D6 checks field presence only.** It verifies that `source_link` or `citation_acceptance` fields exist on an issue — but it does **not** verify that the source UUID actually exists in the manifest.
>
> **An issue can pass D6 while still failing E8 in `arm validate`.**
>
> Always run both:
> - `arm doctor` — structural health (field presence, parent refs, dependency cycles)
> - `arm validate` — semantic citation validity (UUID integrity, coverage)
>
> Never rely on D6 alone as proof of citation integrity.

## Source Freshness

`arm sources verify` `[escape hatch]` detects stale fingerprints; `arm sources sync` refreshes them.

Workflow when sources are stale:

```bash
arm sources verify        # [escape hatch] identify MISSING or changed sources
arm sources sync          # re-fingerprint
arm sources verify        # [escape hatch] confirm all OK
arm sources stale-review  # [escape hatch] if content changed, review delta before accepting
arm validate              # confirm no new E8 errors from stale UUIDs
```

Always run `arm sources verify` `[escape hatch]` as audit step 2.
