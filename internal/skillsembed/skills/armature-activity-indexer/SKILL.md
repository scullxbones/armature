---
name: armature-activity-indexer
description: >
  Use when preparing a cheap fresh-context Activity Index from a worktree's 
  activity log. Reads the log, emits a schema-constrained JSON index that routes
  the Reviewer to raw entries. The index is a finding aid only, never citable.
  Requires arm on PATH.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Activity Indexer

The Activity Indexer reads a worktree's activity log and emits a schema-constrained
JSON Activity Index carrying the log digest and an ordered entry summary. The index
is a **finding aid only** — it routes reviewers to raw log entries for behavioral
evidence but is **never citable**. Citations against the index are rejected by
`arm review record`.

## Prerequisites

If `arm` is not found, stop and resolve this before proceeding.

The Activity Indexer does not require `arm worker-init`. It receives log metadata
(path and digest) from the Coordinator as part of the delivery bundle.

---

## The Indexing Workflow

```
Coordinator passes: log path + digest (from ReviewBundle.Activity)
    ↓
Read and validate activity log
    ↓
Parse each JSONL entry
    ↓
Assign entry IDs (0-based physical line number in the log file)
    ↓
Classify command into category (build/test/lint/run/other)
    ↓
Determine HEAD-anchor flag (this entry's head_sha == delivery HEAD)
    ↓
Emit Activity Index JSON
    ↓
Return index to Coordinator
    ↓
Coordinator passes index + bundle to Reviewer
```

---

## Input: Activity Log Metadata

The Coordinator provides:
- **Log path** — path to the worktree's `armature-activity.log` file (relative or absolute)
- **Log digest** — SHA-256 digest of the log file content (for verification)
- **DeliveryHeadCount** — number of entries at the delivery HEAD commit (from ReviewBundle.Activity)
- **EarlierCount** — number of entries at commits before delivery HEAD (from ReviewBundle.Activity)

The log file is JSONL format (one JSON entry per line).

### Activity Log Entry Format

Each line in the activity log is a JSON entry with fields:
- `timestamp` — RFC3339 UTC timestamp
- `command` — the executed command (one-liner string, may include pipes/redirects)
- `exit_code` — integer exit status (0 = success); only meaningful when `exit_code_known` is `true`
- `exit_code_known` — boolean; `false` means the harness did not report an exit code for this
  command (e.g. a pre-execution event, or a harness that omits it). Treat an entry with
  `exit_code_known: false` as **not** verified successful — never report it as `exit_status: 0`
  in the index, and never let it back a `satisfied` verdict.
- `head_sha` — the worktree HEAD commit SHA at execution time
- `output_hash` — SHA-256 of the full command output (for integrity)
- `output_head` / `output_tail` — truncated output (first 1KB / last 1KB if the full output
  exceeded 2KB; `output_tail` is absent/empty when the output was short enough to keep in full)

Example:
```json
{
  "timestamp": "2026-07-06T12:34:56Z",
  "command": "go test ./...",
  "exit_code": 0,
  "exit_code_known": true,
  "head_sha": "abc1234567890",
  "output_hash": "deadbeef...",
  "output_head": "PASS\nok\tmodule/pkg\t1.234s\n"
}
```

Entry IDs are **not** stored in the log file itself — they are the 0-based physical line
number of the entry within the log (see "Assign Entry IDs" below). A malformed or blank
line consumes its line number but produces no entry, so IDs never shift when such lines
are skipped.

---

## Output: Activity Index

Emit a JSON Activity Index with the following schema:

```json
{
  "schema_version": 1,
  "log_path": "<path to armature-activity.log>",
  "log_digest": "<SHA-256 of log file content>",
  "entry_count": <total entries>,
  "delivery_head_count": <entries at delivery HEAD>,
  "earlier_count": <entries at earlier commits>,
  "entries": [
    {
      "id": "0",
      "command": "make build",
      "exit_status": 0,
      "head_anchor": true,
      "category": "build",
      "log_pointer": "0"
    },
    {
      "id": "1",
      "command": "go test ./...",
      "exit_status": 0,
      "head_anchor": true,
      "category": "test",
      "log_pointer": "1"
    }
  ]
}
```

### Field Definitions

- **id** — entry ID: the 0-based physical line number of the entry in the activity log file
  (a raw, plain integer as a string, e.g. `"0"`, `"1"`, `"2"`; **not** zero-padded, and
  **not** a 1-based sequential count — a skipped malformed line means IDs are not
  necessarily contiguous). This must match exactly what `arm review record` accepts for
  `activity_entry_id` citations.
- **command** — first 100 characters of the executed command; include pipes and redirects if present
- **exit_status** — integer exit code (0 = success, non-zero = failure); use the string
  `"unknown"` instead of an integer when the entry's `exit_code_known` is `false` — never
  report an unknown exit code as `0`
- **head_anchor** — boolean flag: `true` if this entry's `head_sha` equals the delivery HEAD
  commit SHA, `false` otherwise. Compare `head_sha` directly, per entry — do not assume the
  first `delivery_head_count` entries are the head-anchored ones (entry order in the log is
  chronological, not grouped by commit).
- **category** — string classification of the command:
  - `"build"` — contains `build`, `make`, `go build`, `cargo build`, etc.
  - `"test"` — contains `test`, `go test`, `pytest`, `cargo test`, etc.
  - `"lint"` — contains `lint`, `fmt`, `golangci-lint`, `clippy`, etc.
  - `"run"` — contains `run`, `exec`, etc.
  - `"other"` — catch-all for commands that don't fit other categories
- **log_pointer** — reference to locate the entry in the raw log; set to the entry ID
- **log_path** — (top level) path to the activity log file
- **log_digest** — (top level) SHA-256 digest of the entire log file content (for verification)
- **entry_count** — (top level) total number of entries in the log
- **delivery_head_count** — (top level) number of entries at the delivery HEAD commit
- **earlier_count** — (top level) number of entries at commits before delivery HEAD

### Category Classification Algorithm

```
command = (first 200 chars of the command)
if "build" in lowercase(command) or "go build" in command:
  category = "build"
else if "test" in lowercase(command) or "go test" in command:
  category = "test"
else if "lint" in lowercase(command) or "fmt" in lowercase(command):
  category = "lint"
else if "run" in lowercase(command) or "exec" in lowercase(command):
  category = "run"
else:
  category = "other"
```

---

## Step-by-Step

### 1. Receive Input

From the Coordinator, receive:
```
Log path: (passed as argument or environment variable)
Log digest: (from ReviewBundle.Activity.Digest)
DeliveryHeadCount: (from ReviewBundle.Activity.DeliveryHeadCount)
EarlierCount: (from ReviewBundle.Activity.EarlierCount)
```

### 2. Read and Validate the Activity Log

1. Open the log file at the provided path
2. Compute SHA-256 digest of the entire file content
3. Verify the computed digest matches the passed digest — if mismatch, report error and stop
4. Parse each line as JSON and extract fields
5. Count total entries and verify counts match the passed DeliveryHeadCount + EarlierCount

### 3. Classify Each Entry

For each physical line in the log file, in order (oldest to newest):
1. Assign the entry ID as the 0-based physical line number (skip and do not assign an
   entry for blank or malformed/non-JSON lines — but still count their line number so
   later entries keep their correct ID)
2. Extract the `command` field and truncate to first 100 characters
3. Extract `exit_code` and `exit_code_known`; if `exit_code_known` is `false`, report
   `exit_status: "unknown"` rather than an integer
4. Extract the `head_sha` field
5. Determine `head_anchor`: `true` if this entry's `head_sha` equals the delivery HEAD SHA
   (from `ReviewBundle.Delivery.HeadSHA`), `false` otherwise
6. Classify `category` using the algorithm above

### 4. Build the Index

Assemble all entries into the Activity Index JSON structure:
- Top-level fields: `schema_version`, `log_path`, `log_digest`, `entry_count`, `delivery_head_count`, `earlier_count`
- `entries` array: ordered list of classified entries

### 5. Validate and Return

1. Confirm the index is valid JSON
2. Confirm entry count matches the log
3. Confirm exactly the entries whose `head_sha` equals the delivery HEAD SHA have `head_anchor: true`
4. Confirm all other entries have `head_anchor: false`
5. Return the index JSON to the Coordinator

---

## Critical Rules: The Index is Never Citable

**The Activity Index is a finding aid only.** It summarizes the raw log to route reviewers to specific entry IDs for behavioral evidence.

### Citable vs Non-Citable

**Citable (OK in review citations):**
- Raw activity log entry IDs (e.g., `"activity_entry_id": "001"`)
- Diff citations (file paths, line numbers)

**Not citable (rejected by `arm review record`):**
- References to the index as a whole
- Summarized or aggregated counts from the index
- "Entry X in the index" — must cite the raw entry ID instead

### Why?

If a reviewer reads only the Activity Index without the raw log, they cannot verify entry details (full command, output hash, full output availability). An index/log digest mismatch is detectable at record time, but an index-only citation cannot be verified against the raw entry. For safety and auditability, citations must reference raw log entry IDs, which can be verified by the harness.

---

## Common Patterns

### Pattern 1: All Entries at Delivery HEAD

All commands run between the base and delivery HEAD commits:
```json
{
  "entry_count": 5,
  "delivery_head_count": 5,
  "earlier_count": 0,
  "entries": [
    {"id": "0", "head_anchor": true, ...},
    {"id": "1", "head_anchor": true, ...},
    ...
  ]
}
```

### Pattern 2: Mixed Entries (Some Earlier)

Some commands ran on earlier commits before delivery HEAD was reached. head_anchor is
determined per entry by comparing head_sha to the delivery HEAD — entries are not
necessarily grouped chronologically by commit:
```json
{
  "entry_count": 10,
  "delivery_head_count": 7,
  "earlier_count": 3,
  "entries": [
    {"id": "0", "head_anchor": false, ...},  // head_sha != delivery HEAD
    {"id": "1", "head_anchor": false, ...},  // head_sha != delivery HEAD
    {"id": "2", "head_anchor": false, ...},  // head_sha != delivery HEAD
    {"id": "3", "head_anchor": true, ...},   // head_sha == delivery HEAD
    ...
  ]
}
```

### Pattern 3: Failed Build (High Exit Status)

```json
{
  "entries": [
    {"id": "0", "command": "make lint", "exit_status": 0, "category": "lint", ...},
    {"id": "1", "command": "go build ./...", "exit_status": 1, "category": "build", ...},
    {"id": "2", "command": "go test ./...", "exit_status": 0, "category": "test", ...}
  ]
}
```

### Pattern 4: Unknown Exit Code

```json
{
  "entries": [
    {"id": "0", "command": "go test ./...", "exit_status": "unknown", "category": "test", ...}
  ]
}
```

---

## Error Handling

### Log File Not Found
Report error: "Activity log not found at [path]"

### Digest Mismatch
Report error: "Activity log digest mismatch: computed [actual], expected [provided]"

### Invalid JSON Entry
Report error: "Failed to parse activity log entry [line number]: [error]"

### Count Mismatch
Report error: "Entry count mismatch: log contains [N], but DeliveryHeadCount + EarlierCount = [M]"

---

## Validation Checklist

Before returning the index:

1. **File digest** — computed digest matches passed digest exactly
2. **Entry counts** — `entry_count == delivery_head_count + earlier_count`
3. **HEAD anchors** — exactly the entries whose `head_sha` equals the delivery HEAD SHA have `head_anchor: true`
4. **Entry IDs** — 0-based physical line numbers (plain integers as strings, e.g. "0", "1"); not zero-padded, not necessarily contiguous
5. **Categories** — all entries have valid categories (build/test/lint/run/other)
6. **Commands** — all commands are non-empty strings
7. **Exit statuses** — either an integer, or the string `"unknown"` when the entry's `exit_code_known` is `false`
8. **JSON schema** — valid JSON, no missing required fields
9. **No raw output in index** — the index carries only command summaries and entry IDs, never raw output excerpts or truncated output fields

---

## Returning Results to the Coordinator

After producing the Activity Index JSON, return it to the Coordinator. The Coordinator
will pass both the index and the ReviewBundle to the Reviewer.

```json
{
  "schema_version": 1,
  "log_path": "...",
  "log_digest": "...",
  "entry_count": 5,
  "delivery_head_count": 5,
  "earlier_count": 0,
  "entries": [...]
}
```

The Coordinator is responsible for:
1. Capturing the returned index JSON
2. Passing it to the Reviewer agent along with the ReviewBundle
3. Recording the assessment via `arm review record`

---

## Command Reference

The indexer does not use `arm` commands directly — it reads the log file and emits JSON.

The Coordinator manages the log location and provides the path/digest:

```bash
# Coordinator: prepare bundle. `arm review prepare` has no --activity-log or
# --activity-digest flags — it discovers the activity log itself (from the
# delivery worktree's own git dir) and attaches an Activity section to the
# bundle automatically when a log is present. No indexer-specific flags exist.
arm review prepare --issue TASK-ID --base "$BASE_SHA" --head "$HEAD_SHA" --output "$BUNDLE_FILE"

# Coordinator: dispatch the activity indexer as a subagent, passing it the
# ReviewBundle (or at minimum bundle.activity.log_path and bundle.activity.digest,
# read out of $BUNDLE_FILE) so it knows what log to index and what digest to verify.

# Coordinator: capture the indexer's returned Activity Index JSON and pass both
# it and the ReviewBundle to the Reviewer agent.

# Coordinator: record assessment
arm review record --issue TASK-ID --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE"
```

