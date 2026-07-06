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
Assign entry IDs (sequential: 001, 002, …)
    ↓
Classify command into category (build/test/lint/run/other)
    ↓
Determine HEAD-anchor flag (at delivery HEAD vs earlier)
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
- `exit_code` — integer exit status (0 = success)
- `head_sha` — the worktree HEAD commit SHA at execution time
- `output_hash` — SHA-256 of the full command output (for integrity)
- `output` or `output_head`/`output_tail` — truncated output (first 1KB / last 1KB if large)

Example:
```json
{
  "timestamp": "2026-07-06T12:34:56Z",
  "command": "go test ./...",
  "exit_code": 0,
  "head_sha": "abc1234567890",
  "output_hash": "deadbeef...",
  "output": "PASS\nok\tmodule/pkg\t1.234s\n"
}
```

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
      "id": "001",
      "command": "make build",
      "exit_status": 0,
      "head_anchor": true,
      "category": "build",
      "log_pointer": "001"
    },
    {
      "id": "002",
      "command": "go test ./...",
      "exit_status": 0,
      "head_anchor": true,
      "category": "test",
      "log_pointer": "002"
    }
  ]
}
```

### Field Definitions

- **id** — entry ID, zero-padded sequential number (001, 002, …, 999, 1000, …)
- **command** — first 100 characters of the executed command; include pipes and redirects if present
- **exit_status** — integer exit code (0 = success, non-zero = failure)
- **head_anchor** — boolean flag: `true` if this entry is at the delivery HEAD commit (first `delivery_head_count` entries), `false` if earlier
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

For each entry in order (oldest to newest):
1. Extract the `command` field and truncate to first 100 characters
2. Extract the `exit_code` field (or `exit_status` if already present in log)
3. Extract the `head_sha` field
4. Assign sequential entry ID: "001", "002", etc.
5. Determine `head_anchor`:
   - If this is one of the first `delivery_head_count` entries → `true`
   - Otherwise → `false`
6. Classify `category` using the algorithm above

### 4. Build the Index

Assemble all entries into the Activity Index JSON structure:
- Top-level fields: `schema_version`, `log_path`, `log_digest`, `entry_count`, `delivery_head_count`, `earlier_count`
- `entries` array: ordered list of classified entries

### 5. Validate and Return

1. Confirm the index is valid JSON
2. Confirm entry count matches the log
3. Confirm first `delivery_head_count` entries have `head_anchor: true`
4. Confirm remaining entries have `head_anchor: false`
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
    {"id": "001", "head_anchor": true, ...},
    {"id": "002", "head_anchor": true, ...},
    ...
  ]
}
```

### Pattern 2: Mixed Entries (Some Earlier)

Some commands ran on earlier commits before delivery HEAD was reached:
```json
{
  "entry_count": 10,
  "delivery_head_count": 7,
  "earlier_count": 3,
  "entries": [
    {"id": "001", "head_anchor": false, ...},  // earlier commit
    {"id": "002", "head_anchor": false, ...},  // earlier commit
    {"id": "003", "head_anchor": false, ...},  // earlier commit
    {"id": "004", "head_anchor": true, ...},   // at delivery HEAD
    ...
  ]
}
```

### Pattern 3: Failed Build (High Exit Status)

```json
{
  "entries": [
    {"id": "001", "command": "make lint", "exit_status": 0, "category": "lint", ...},
    {"id": "002", "command": "go build ./...", "exit_status": 1, "category": "build", ...},
    {"id": "003", "command": "go test ./...", "exit_status": 0, "category": "test", ...}
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
3. **HEAD anchors** — exactly the first `delivery_head_count` entries have `head_anchor: true`
4. **Entry IDs** — sequential, zero-padded, start at "001"
5. **Categories** — all entries have valid categories (build/test/lint/run/other)
6. **Commands** — all commands are non-empty strings
7. **Exit statuses** — all exit statuses are integers
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
# Coordinator: prepare bundle with activity section
arm review prepare --issue TASK-ID ... --activity-log <path> --activity-digest <digest>

# Coordinator: dispatch activity indexer with log path
# (pass $BUNDLE_FILE path to the indexer agent)

# Coordinator: capture returned index and dispatch reviewer
# (pass both index JSON and bundle to reviewer)

# Coordinator: record assessment
arm review record --issue TASK-ID --assessment "$RESULT_FILE" --bundle "$BUNDLE_FILE"
```

