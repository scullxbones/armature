# Citation Error Remediation

## Citation Integrity (E7 + E8)

### E7 — Uncited Node

```
ERROR: uncited node: ISSUE-ID
```

An issue has neither a `source-link` nor an `accept-citation`. It is completely untraced.

**Fix:**

```bash
# Link to a source document
arm source-link ISSUE-ID

# Or accept the citation risk explicitly (for issues with no recoverable source)
arm accept-citation --ci ISSUE-ID
```

### E8 — Unknown Source

```
ERROR: unknown source: UUID in citation for ISSUE-ID
```

An issue's `source-link` points to a UUID that no longer exists in the sources manifest. This happens when a source was registered, used in a citation, then deleted from the manifest.

**Fix:**

```bash
arm sources sync          # refresh manifest; re-fingerprint all sources
arm sources verify        # confirm all show OK
arm validate              # re-run — E8 should be gone if the source was re-found
```

If the source is gone permanently, register a replacement and re-link:

```bash
arm sources add <replacement-url-or-path>
arm source-link ISSUE-ID  # link to the new source UUID
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

Source fingerprints go stale when the underlying document changes after initial registration. `arm sources verify` detects this; `arm sources sync` re-fetches and re-fingerprints all sources.

Workflow when sources are stale:

```bash
arm sources verify        # identify MISSING or changed sources
arm sources sync          # re-fingerprint
arm sources verify        # confirm all OK
arm stale-review          # if content changed, review delta before accepting
arm validate              # confirm no new E8 errors from stale UUIDs
```

Sources can also go stale silently between the time a worker registers them and the time the auditor runs. Always run `arm sources verify` as step 2 of the audit — do not assume sources registered during implementation are still current.
