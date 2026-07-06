# Sensitive Environments and Execution Evidence Disclosure

Armature records harness-executed commands and their output to support semantic review.
In sensitive environments (customer infrastructure, air-gapped systems, compliance-critical
contexts), this data may contain credentials, tokens, API keys, internal hostnames, or
other non-public information. This document covers:

1. **The disclosure path:** How execution evidence moves from recording to published report
2. **Citation boundary:** What operators and reviewers see vs. what the public sees
3. **Capture configuration:** Default behavior and how to disable recording
4. **Operational guidance:** When and how to disable capture

## Disclosure Path: Recording → Review → Report → Publication

### Recording (Harness Session)

When a worker runs a command within Armature (`Bash` tool), the harness hook records:
- Command string (exact text executed)
- Exit code
- Truncated stdout/stderr (first 1 KB + last 1 KB if > 2 KB)
- Full output SHA-256 hash
- Worktree HEAD commit
- RFC3339 timestamp

This data is written to a **worktree-local activity log** at `<git-dir>/armature-activity.log`.
The log is **ephemeral and never committed**. It lives only in the worktree's git metadata
directory and is deleted when the worktree is torn down.

### Review (Fresh Evaluator Context)

The Coordinator prepares a Review Bundle containing the delivery diff and optional activity
section. A fresh `armature-reviewer` skill is dispatched with **read-only access** to:
- The delivery diff
- The Review Bundle (including activity digest and entry count, but not the raw log)
- Bounded repository context (surrounding code for diff interpretation)

The Reviewer **does not have direct access to the activity log itself**. It sees:
- The activity section metadata (digest, entry count, HEAD-anchor summary, log path)
- The log's integrity fingerprint for verification

If the Reviewer cites execution evidence, citations reference **raw log entry IDs** only.
Index-based citations are invalid; the activity index is a finding aid, not an evidence source.

### Record (Attestation)

The `arm review record` command:
1. Reconstructs the bundle from the explicit delivery range
2. Re-verifies the activity log digest against the on-disk log, but only when the
   assessment contains activity citations; if it doesn't, this check is skipped
3. Validates all reviewer citations against the raw activity log
4. Creates a compact durable attestation (operator-facing, not public)

At this point:
- **Raw log:** Still worktree-local, unmodified
- **Attestation:** Stored in `.armature/ops/` (operators only)
- **Citation details:** Pre-rendered during record time (entry ID, command, exit status)

### Report (PR Surface)

When the Coordinator publishes a detailed review report to the PR surface, the rendered
output includes only:
- Entry ID (e.g., `entry[42]`)
- Command line (exact text executed)
- Exit status (exit code, success/failure)

The rendered report **excludes:**
- Raw output excerpts (including truncated head/tail)
- Full output hash
- Worktree HEAD commit SHA
- Timestamp
- Any other worktree-local metadata

This is the **citation boundary:** public reports cite evidence without exposing the
raw data that generated it.

## What Published Reports Include and Exclude

### Included in PR Surface Reports

When a semantic review is published to the PR:
- **Green/yellow/red rating** for each criterion
- **Criterion ID** and verbatim text
- **Rationale** (the Reviewer's concise explanation)
- **Citations** to diff hunks (path, line number)
- **Activity citations** (entry ID, command, exit status only)

Example activity citation in a published report:
```
Criterion: "Endpoint returns 401 on invalid credentials"
Status: satisfied
Rationale: Observed test execution confirming error response.
Citations:
  - Diff: src/auth/handler_test.go:42 (test case for invalid credentials)
  - Activity: entry[5] command="go test ./auth -run TestInvalidCredentials"
             exit_status=0 (tests passed)
```

### Excluded from PR Surface Reports

These data are **never published** to the PR surface:
- Raw command output (head/tail excerpts)
- Full output SHA-256 hash
- Worktree HEAD commit SHA at execution time
- Timestamps
- Activity log file path
- Complete truncated output payloads
- Any raw Bash execution details

### Operator-Facing Records (Not Published)

Operators have access to more detailed records via Armature's op log:
- Assessment attestation (compact record: fingerprints, counts, rating)
- Optionally, the full detailed review JSON/Markdown (ephemeral, at Coordinator discretion)

Operators who need to audit detailed execution evidence can access the **original
worktree's activity log** directly before it is torn down. This is the only source
of full output data (head, tail, hash, timestamp) and is never automatically
published to external surfaces.

## Capture Is Default-On: How to Disable

Execution evidence capture is **enabled by default** wherever harness hooks are installed.

### Disable Capture (Git Config — the only kill-switch)

Disable for the entire repository (all worktrees):

```bash
git config --local armature.disable-activity-logging true
```

This setting:
- Applies to all worktrees of the repository
- Persists across sessions
- Affects all future hook invocations

There is deliberately **no environment-variable override**. An earlier version of
Armature supported `ARMATURE_DISABLE_ACTIVITY_LOGGING` as a session-scoped alternative,
but it has been removed: a worker process could `export` it mid-session to suppress
capture around a failing command and `unset` it afterward, curating exactly the
failure-then-success selection bias this capture policy exists to prevent. Disabling
capture is a repo-level Definition-of-Done decision — use the git config kill-switch,
which is visible in repo state and not something a running session can toggle
unilaterally.

### Re-Enable Capture

```bash
git config --local --unset armature.disable-activity-logging
```

## Operational Guidance for Sensitive Environments

**If your repository contains or may produce:**
- Credentials, tokens, API keys
- Internal IP addresses, hostnames, or domain names
- Proprietary algorithms, passwords, or security-relevant data
- Customer data or personally identifiable information (PII)
- Compliance-critical or classified information

**You MUST disable execution evidence capture before running Armature workers:**

```bash
git config --local armature.disable-activity-logging true
arm claim TASK-001 --worktree ./worktree
# then launch harness from the worktree
```

### Before Publishing a Review to Your PR System

If capture was **enabled** during a delivery:

1. **Before merging or publishing,** ensure the worktree has been torn down
   (worktrees are deleted by `arm merged --issue <task-id>`, which happens after
   `arm review record`). The activity log is worktree-local and is automatically
   deleted.

2. **Verify the published report only includes safe citations:**
   - Check the PR that Armature generates; it should show only entry IDs, command
     lines, and exit statuses — never raw output.
   - If raw output appears in the published report, there is a bug; do not merge
     until the issue is resolved.

3. **If you accidentally published raw output,** immediately:
   - Remove the report from your PR system
   - Run `arm doctor` to check for any stale worktrees
   - Contact Armature maintainers if the citation boundary was violated

### Custom Harness Integrations

If you integrate Armature with a custom harness (not Claude Code, Codex, or Devin):

1. **Ensure the harness calls `arm harness-hook`** for PostToolUse events on the bound issue
2. **Document capture behavior** in your harness's own security/privacy documentation, noting
   that the only supported kill-switch is the repo-level `armature.disable-activity-logging`
   git config key (there is no environment-variable override)
3. **Scrub or disable capture** before processing sensitive environments

## FAQ

**Q: Can I inspect the activity log directly?**

A: Yes. The log is at `<worktree-git-dir>/armature-activity.log` (e.g., `.git/armature-activity.log`).
It's a JSONL file (one JSON object per line, one line per command).
Inspect it before the worktree is torn down, or disable capture to prevent it from being created.

**Q: Does disabling capture affect semantic review?**

A: Yes. If capture is disabled, `arm review prepare` will not include an activity section
in the bundle. The Reviewer will have only the delivery diff and repository context.
The review will be more likely to produce `indeterminate` for behavioral criteria that
cannot be established from the diff alone. This is by design: in sensitive environments,
correctness (no false signal) is more important than coverage (fewer indeterminates).

**Q: Can I partially capture (e.g., some commands, not others)?**

A: No. The policy is complete capture with full filtering. You can either:
- Capture all Bash commands (default), or
- Capture none (set the kill-switch)

Per-command filtering is not supported because filtering introduces selection bias.

**Q: If I disable capture, will the review be incomplete?**

A: Potentially. Behavioral criteria that require execution evidence may produce
`indeterminate` if only the diff is available. This is expected and correct: when
evidence is unavailable, the Reviewer must not guess. In sensitive environments,
this is the right trade-off.

**Q: How long is the activity log retained?**

A: The activity log is ephemeral and worktree-local. It is deleted when the
worktree is torn down (by `arm merged --issue <task-id>`). If you need to retain
it for auditing, back it up before the worktree is torn down.

**Q: What if I forget to disable capture in a sensitive environment?**

A: The activity log is worktree-local and ephemeral—it is automatically deleted
when the worktree is torn down. However, if the Coordinator publishes a review
report to your PR system, that report will include activity citations. The report
should only show entry IDs, command lines, and exit statuses. If raw output appears
in the published report, immediately:

1. Remove the report from your PR system
2. Contact Armature maintainers to report a citation boundary violation

**Q: Can I see the activity log after semantic review is complete?**

A: Not without special effort. The activity log is deleted when the worktree is
torn down (which happens as part of `arm merged`). To inspect it:
- Disable this teardown step (Coordinator-specific option, varies by harness)
- Back up the log before the worktree is deleted
- Or, disable capture and never create the log in the first place

**Q: Does the activity log appear in my repository history?**

A: No. The activity log is never committed and never appears in git history. It is
worktree-local and ephemeral. Only the compact durable attestation (fingerprints,
counts, rating) enters Armature's op log.

## See Also

- [ADR-0008: Admit harness-recorded execution evidence into semantic review, upgrade-only](docs/adr/0008-execution-evidence-in-semantic-review.md) — Trust model, mechanics, rejected alternatives
- [docs/harness-hook.md](docs/harness-hook.md) — Activity capture, kill-switch, teardown ordering
- [Semantic Conformance Review Design](docs/superpowers/specs/2026-06-27-semantic-conformance-review-design.md) — Full review protocol, bundle structure, rating derivation
