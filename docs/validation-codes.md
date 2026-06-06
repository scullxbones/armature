# Validation & Doctor Codes Reference

This document describes all diagnostic codes emitted by `arm validate` (errors E2–E10, E12 and warnings W1–W8, W10–W11) and `arm doctor` (checks D1–D6).

## Validation Codes (arm validate)

The `arm validate` command checks the issue graph for semantic and structural consistency. It reports **errors** that prevent execution and **warnings** that indicate potential issues.

**Basic usage:**
```bash
# Check full graph
arm validate

# Treat warnings as errors
arm validate --strict

# Exit non-zero if errors found (CI mode)
arm validate --ci

# Check only a subtree
arm validate --scope EPIC-001

# Suppress INFO lines (e.g., phantom-scope notices)
arm validate --quiet
```

**Output format:**
```
ERROR: <error-code>: <message>
WARNING: <warning-code>: <message>
INFO: <info-code>: <message>          # only if --quiet not set
COVERAGE: <cited>/<total> cited
OK: no issues found
```

---

### Errors (E2–E10, E12)

#### E2: Unresolved Parent Link

**Trigger:** A task or story references a parent issue ID that does not exist in the graph.

**Message:** `unresolved parent: <parent-id> for node <issue-id>`

**Fix:**
1. Verify the parent ID is spelled correctly: `arm show <parent-id>`
2. If parent was deleted, remove the parent link: `arm amend <issue-id> --parent ""`
3. If parent should exist but doesn't, recreate it: `arm create --id <parent-id> --title "..." --type story`

**Example:**
```bash
# Task TASK-001 refers to non-existent parent STORY-999
ERROR: unresolved parent: STORY-999 for node TASK-001

# Fix: amend the task to remove the bad parent link
arm amend TASK-001 --parent ""
```

---

#### E3: Unresolved Link Target

**Trigger:** An issue references a blocker (dependency) that does not exist in the graph.

**Message:** `unresolved link target: <blocker-id> from <issue-id>`

**Fix:**
1. Check if the blocker ID exists: `arm show <blocker-id>`
2. If the blocker was deleted or renamed, update the link: `arm link --source <issue-id> --rel blocked_by --dep <correct-id>`
3. If the blocker should not exist, remove it: `arm link --source <issue-id> --rel blocked_by --dep <blocker-id>` (re-run to remove)

**Example:**
```bash
# Task TASK-003 is blocked by non-existent TASK-002
ERROR: unresolved link target: TASK-002 from TASK-003

# Fix: link to the correct blocker
arm link --source TASK-003 --rel blocked_by --dep TASK-002-CORRECT
```

---

#### E4: Cycle Detected

**Trigger:** Two or more issues form a circular dependency (A blocks B, B blocks A, etc.).

**Message:** `cycle detected: <issue-a> → ... → <issue-a>`

**Fix:**
1. Identify the cycle: `arm show <issue-a>` and check `BlockedBy` field
2. Remove one link in the cycle to break it
3. Test: `arm validate` should pass

**Example:**
```bash
# TASK-001 blocks TASK-002 blocks TASK-001 (cycle)
ERROR: cycle detected: TASK-001 → ... → TASK-001

# Fix: examine the chain
arm show TASK-001
# BlockedBy: [TASK-002]

arm show TASK-002
# BlockedBy: [TASK-001]

# Remove one link to break the cycle
arm link --source TASK-002 --rel blocked_by --dep TASK-001  # toggle to remove
```

---

#### E5: Invalid Type Hierarchy

**Trigger:** A task or story has a child of an invalid type (e.g., epic cannot be a child of task).

**Message:** `invalid hierarchy: <parent-type> <parent-id> cannot parent <child-type> <child-id>`

**Valid hierarchies:**
- Epic → Story, Task, Bug
- Story → Task, Bug
- Task, Bug → (no children; terminal)

**Fix:**
1. Identify the invalid parent-child pair from the error message
2. Promote the parent to a valid type, or demote the child
3. Typical fix: change Task to Story if it has children: `arm amend <task-id> --type story`

**Example:**
```bash
# Bug TASK-001 has Story STORY-001 as child (invalid)
ERROR: invalid hierarchy: bug TASK-001 cannot parent story STORY-001

# Fix: promote the bug to a story
arm amend TASK-001 --type story
```

---

#### E6: Missing Required Field

**Trigger:** A non-terminal task is missing one of: `scope`, `acceptance`, or `definition_of_done`.

**Message:** `missing required field: <field> on task <issue-id>`

**Required fields for tasks (open/in-progress/blocked):**
- `scope`: File globs the task will modify (e.g., `["cmd/app/*.go", "internal/core/"]`)
- `acceptance`: JSON array of acceptance criteria
- `definition_of_done`: Plain-text definition of done (1–500 chars)

**Fix:**
```bash
# Add missing scope
arm amend TASK-001 --scope 'cmd/app/*.go' --scope 'internal/*.go'

# Add missing acceptance criteria (as JSON)
arm amend TASK-001 --acceptance '[{"type": "test_passes", "description": "all tests pass"}]'

# Add missing definition of done
arm amend TASK-001 --dod "Code is merged to main and deployed to staging"
```

---

#### E9: Definition of Done Exceeds Max Length

**Trigger:** An issue's `definition_of_done` field exceeds 500 characters.

**Message:** `definition_of_done exceeds 500 chars on <issue-id>`

**Fix:**
1. Shorten the definition of done to ≤500 chars
2. Move detailed context to a note or decision
```bash
# Shorten the DoD
arm amend TASK-001 --dod "Brief, clear definition of done"

# Add detailed context as a note if needed
arm note TASK-001 --msg "Additional context: ..."
```

---

#### E10: Invalid Glob Pattern

**Trigger:** An issue's `scope` contains an invalid glob pattern that cannot be parsed.

**Message:** `invalid glob: <glob> on <issue-id>`

**Valid glob patterns:**
- Literal paths: `cmd/app/main.go`
- Directory globs: `cmd/**/*.go`, `internal/*.go`, `**/*.md`
- Wildcard characters: `*` (any chars in one level), `**` (recursive), `?` (single char), `[a-z]` (char class)

**Fix:**
```bash
# Remove the invalid glob and add a correct one
arm amend TASK-001 --scope 'cmd/**/*.go'  # fixes invalid patterns
```

---

#### E12: Unknown Source in Citation

**Trigger:** An issue references a source ID that does not exist in the source manifest.

**Message:** `unknown source: <source-id> in citation for <issue-id>` or `uncited node: <issue-id>`

**Fix:**
1. Verify the source exists: `arm sources list` or check `.armature/sources/manifest.json`
2. If source was deleted, remove the citation: `arm source-link <issue-id>` (no --source-id)
3. If you need to cite a source, add it first: `arm sources add --url <url> --type <type>`
4. Then link: `arm source-link <issue-id> --source-id <source-id>`

---

### Warnings (W1–W8, W10–W11)

Warnings highlight potential issues but do not prevent execution. Use `--strict` to treat them as errors.

#### W1: Scope Overlap Between Siblings

**Trigger:** Two sibling tasks (same parent, not in terminal status) modify overlapping files, and neither blocks the other.

**Message:** `scope overlap: <task-a> and <task-b> both modify <file-list>`

**Context:** Scope overlap is expected if tasks execute serially (one blocks the other). This warning indicates potential parallel-execution conflicts.

**Fix:**
1. If tasks should run in sequence, add a dependency: `arm link --source <task-b> --rel blocked_by --dep <task-a>`
2. If scope truly overlaps and can be parallelized, narrow one task's scope: `arm amend <task-b> --scope '...'`
3. If this is intentional (e.g., refactoring one file in parallel), add a note explaining: `arm note <task-a> --msg "Intentional scope overlap with <task-b>"`

---

#### W2: No Test Criteria

**Trigger:** A task has no acceptance criteria with `type: "test_passes"` or `type: "manual_review"`.

**Message:** `no test criteria on <issue-id>`

**Fix:**
```bash
# Add test acceptance criteria
arm amend TASK-001 --acceptance '[
  {"type": "test_passes", "description": "unit tests pass"},
  {"type": "manual_review", "description": "code reviewed by peer"}
]'
```

---

#### W3: Budget Exceeded

**Trigger:** A task's estimated token consumption (DoD length + title + context) exceeds 4000 tokens (~16k chars).

**Message:** `budget advisory: <issue-id> est. <tokens> tokens > 4000`

**Fix:**
1. Shorten the definition of done or context
2. Add a note with detailed requirements instead of embedding them in DoD
3. Split the task into smaller subtasks: `arm create --parent <task-id> --title "..."`

---

#### W4: Broad Scope

**Trigger:** A non-terminal task's scope is `**/*`, `**`, or `.` (entire repository).

**Message:** `broad scope: <issue-id> scope covers entire tree`

**Fix:**
```bash
# Narrow the scope to specific directories or file patterns
arm amend TASK-001 --scope 'cmd/**/*.go' --scope 'internal/**/*.go'
```

---

#### W5: Missing Context Files

**Trigger:** A non-terminal task has ≥3 distinct directories in its scope but no `context_files` specified.

**Message:** `missing context_files on <issue-id> with broad scope — split the task into smaller pieces or narrow scope via: arm amend <issue-id> --scope <glob>`

**Context:** `context_files` helps workers understand the codebase. Large multi-directory scopes without context files can be hard for AI to navigate.

**Fix:**
1. Specify context files: `arm amend TASK-001 --context-files 'README.md' --context-files 'docs/architecture.md'`
2. Or split the task: `arm create --parent <epic> --title "..." --scope 'cmd/**/*.go'` (smaller scope)
3. Or narrow scope: `arm amend TASK-001 --scope 'internal/**/*.go'` (fewer directories)

---

#### W6: Complexity Mismatch

**Trigger:** A task's `est_complexity` field doesn't match the scope size.

**Message:** `complexity mismatch: <issue-id> has <n> files but marked <complexity>`

**Thresholds:**
- `small`: ≤5 files
- `large`: ≥2 files (any size is "large")

**Fix:**
```bash
# Amend the estimated complexity to match scope
arm amend TASK-001 --est-complexity small  # if scope has ≤5 files
arm amend TASK-002 --est-complexity large  # if scope has ≥2 files
```

---

#### W7: Vague Definition of Done

**Trigger:** A task's DoD contains vague words: "properly", "correctly", "good", "well", "appropriate", "suitable".

**Message:** `vague DoD: <issue-id> contains "<vague-word>"`

**Fix:**
```bash
# Replace vague language with measurable criteria
# Before: "Code is properly tested"
# After: "All unit and integration tests pass with ≥80% coverage"

arm amend TASK-001 --dod "All unit tests pass (go test ./...), integration tests pass, ≥80% code coverage"
```

---

#### W8: Conflicting Decisions

**Trigger:** An issue records multiple conflicting decisions on the same topic.

**Message:** `conflicting decisions: topic "<topic>" has <n> choices: <choice-list> on <issue-id>`

**Fix:**
1. Identify the topic and the conflicting choices
2. Keep the chosen option and remove the others
3. Record the decision outcome as a note
```bash
# Example: issue has decisions "Use PostgreSQL" and "Use MongoDB" on topic "Database"
# Remove one: (use decision command to overwrite, or note the resolution)
arm note TASK-001 --msg "Resolved database choice: PostgreSQL selected for ACID compliance"
```

---

#### W10: Phantom Scope (Informational)

**Trigger:** A task declares a scope entry that doesn't match any file in the repository.

**Message:** `INFO: phantom scope: <path> on <issue-id> does not match any file`

**Context:** Phantom scope typically indicates:
- A typo in the glob pattern
- A planned file marked `(new)` that hasn't been created yet
- A file that was deleted after the task was created

**Note:** Planned files ending with ` (new)` are not treated as phantom.

**Fix:**
1. If it's a typo, fix the glob: `arm amend TASK-001 --scope 'correct/path/**/*.go'`
2. If it's a planned file, the ` (new)` suffix should be present: `internal/newmodule/main.go (new)`
3. If the file was deleted, remove it from scope: `arm amend TASK-001 --scope 'cmd/**/*.go'` (without the deleted file)

---

#### W11: Vague Outcome

**Trigger:** A done/merged issue has an outcome that is either <20 chars or is one of: "done", "completed", "finished", "ok", "fixed".

**Message:** `vague outcome: <issue-id> outcome is <n> chars`

**Fix:**
```bash
# Outcome must be ≥20 chars and descriptive
# Before: "done"
# After: "Implemented feature and merged PR #123 to main"

arm transition TASK-001 --to done --outcome "Implemented feature, all tests pass, merged PR #123 to production"
```

---

## Doctor Codes (arm doctor)

The `arm doctor` command runs structural health checks on the repository. It detects broken references, stale data, and synchronization issues.

**Basic usage:**
```bash
# Run all doctor checks
arm doctor

# Promote warnings to errors
arm doctor --strict

# Show file:line context for D3 violations and uncited issue IDs for D6
arm doctor --verbose

# Output as JSON
arm doctor --format json
```

**Output format:**
```
✓ D1: <message>       # OK
⚠ D2: <message>       # Warning
✗ D3: <message>       # Error
    - <item>
    - <item>
```

---

### D1: Git/Armature Divergence

**Check:** Scans recent git commits for issue IDs that are not in `done`/`merged` state.

**Trigger:** Code has been committed referencing an issue that is still `open`, `in-progress`, or `blocked`.

**Message:** `Git commits reference issues not in done/merged state`

**Items:** `<issue-id> (<current-status>)`

**Context:** This indicates the issue graph and git history are out of sync—git shows work done, but Armature shows the task incomplete.

**Fix:**
1. Transition the issue to `done` or `merged`: `arm transition <issue-id> --to done`
2. Or create a new issue for the work referenced in git: `arm create --title "..." --type task`
3. Link the git commit to the issue: `git commit --amend -m "message ISSUE-ID"` if needed

---

### D2: Stale Claims

**Check:** Identifies issues with active claims whose TTL (time-to-live) has expired.

**Trigger:** A worker claimed a task with a TTL (default 60 minutes), and the TTL has passed without a heartbeat or completion.

**Message:** `Claimed issues with expired TTL`

**Items:** `<issue-id>`

**Context:** Stale claims can block other workers. Worker should have sent a heartbeat: `arm heartbeat <issue-id>` before TTL expired.

**Fix:**
1. If worker is still active, send a heartbeat: `arm heartbeat <issue-id>`
2. If worker abandoned the task, unclaim it: `arm reopen <issue-id>` (resets status to open)
3. Review claim TTL in `arm claim` if workers need longer windows: `arm claim <issue-id> --ttl 120` (2 hours)

---

### D3: Orphaned Ops

**Check:** Finds operation log entries (ops) that reference issue IDs not in the graph.

**Trigger:** An op file contains a create/update for an issue ID that doesn't appear in the materialized graph (e.g., due to a revert or op file corruption).

**Message:** `Op files reference issue IDs not in the graph`

**Items:** `<issue-id>`

**Verbose output:** `<issue-id> (workerid.log:5, workerid.log:10)` shows which log files and line numbers

**Context:** Orphaned ops indicate either:
- A worker created an issue but it was later reverted
- Op log corruption (e.g., truncated file)
- A revert operation that removed an issue from the graph

**Fix:**
1. If the issue should exist, check if it was reverted: `arm log --issue <issue-id>`
2. If it should not exist, this is safe to ignore (the op is dead)
3. If log corruption is suspected, check file integrity: `ls -la .armature/ops/`
4. Contact a maintainer if op logs are corrupted

---

### D4: Broken Parent References

**Check:** Finds issues whose `parent` field points to a non-existent issue ID.

**Trigger:** A parent issue was deleted, but child issues still reference it.

**Message:** `Issues with broken parent references`

**Items:** `<child-id> -> <parent-id>`

**Context:** This is a data consistency error. Children reference a parent that no longer exists.

**Fix:**
1. Create the missing parent: `arm create --id <parent-id> --title "..." --type story`
2. Or remove the broken link: `arm amend <child-id> --parent ""`
3. Or re-parent to a different issue: `arm amend <child-id> --parent <correct-parent-id>`

---

### D5: Dependency Cycles

**Check:** Detects cycles in the `blocked_by` dependency graph.

**Trigger:** A chain of dependencies forms a loop: A blocks B, B blocks C, C blocks A.

**Message:** `Dependency cycles detected in blocked_by chains`

**Items:** `<issue-a> -> <issue-b>` (edges in the cycle)

**Context:** Cycles prevent task scheduling. All issues in the cycle will be permanently blocked.

**Fix:**
1. Identify all issues in the cycle from the items list
2. Remove one `blocked_by` link to break the cycle
3. Example:
   ```bash
   arm show TASK-A       # shows BlockedBy: [TASK-B]
   arm show TASK-B       # shows BlockedBy: [TASK-C]
   arm show TASK-C       # shows BlockedBy: [TASK-A]  <- remove this link
   
   arm link --source TASK-C --rel blocked_by --dep TASK-A  # toggle to remove
   ```

---

### D6: Uncited Issues

**Check:** Finds issues without source-link or accept-citation records.

**Trigger:** An issue exists but has no traceability to external documentation (no sources linked, no citations accepted).

**Message:** `Issues without source-link or accept-citation`

**Items:** `<issue-id>`

**Verbose output:** Names uncited issue IDs explicitly

**Context:** Uncited issues have no documented origin, which reduces traceability. They may be undocumented work or exploratory.

**Fix:**
1. Link to a source: `arm source-link <issue-id> --source-id <source-uuid>`
   - First add the source: `arm sources add --url <url> --type filesystem`
   - Then link: `arm source-link <issue-id> --source-id <source-uuid>`
2. Or accept citation risk: `arm accept-citation <issue-id> --rationale "Exploratory work; no external source available."`

---

## Integration with Commands

### validate command flags

```bash
arm validate [flags]

--ci              Exit non-zero if errors found
--scope string    Validate only this subtree (issue ID)
--strict          Treat warnings as errors
--quiet           Suppress INFO lines; still print COVERAGE and OK
--format string   Output format: human, json, agent
```

### doctor command flags

```bash
arm doctor [flags]

--strict          Promote warnings to errors
--verbose         Show file:line context for D3; name uncited IDs for D6
--format string   Output format: human, json, agent
```

---

## Quick Reference Table

| Code | Type | Severity | Category |
|------|------|----------|----------|
| E2 | Error | Critical | Graph structure (parent) |
| E3 | Error | Critical | Graph structure (blocker) |
| E4 | Error | Critical | Graph structure (cycles) |
| E5 | Error | Critical | Type hierarchy |
| E6 | Error | Critical | Required fields |
| E9 | Error | Critical | DoD length |
| E10 | Error | Critical | Scope syntax |
| E12 | Error | Critical | Citation/sources |
| W1 | Warning | Medium | Scope conflict |
| W2 | Warning | Medium | Testing |
| W3 | Warning | Medium | Token budget |
| W4 | Warning | Medium | Scope breadth |
| W5 | Warning | Medium | Context files |
| W6 | Warning | Low | Complexity mismatch |
| W7 | Warning | Low | Vague language (DoD) |
| W8 | Warning | Medium | Conflicting decisions |
| W10 | Info | Low | Phantom files |
| W11 | Warning | Medium | Vague outcome |
| D1 | Doctor | Warning | Git/Armature sync |
| D2 | Doctor | Warning | Stale claims |
| D3 | Doctor | Error | Orphaned ops |
| D4 | Doctor | Error | Broken parent refs |
| D5 | Doctor | Error | Dependency cycles |
| D6 | Doctor | Warning | Uncited issues |

---

## See Also

- [Command Reference](./commands.md) — Full syntax for `arm validate` and `arm doctor`
- [Getting Started](./getting-started.md) — First steps with Armature
- [Design Docs](./design/) — Architectural decisions and data model
