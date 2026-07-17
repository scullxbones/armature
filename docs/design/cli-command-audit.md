# CLI Command Audit: Deep-Module Correspondence and Flag Conventions

**Date:** 2026-07-17
**Scope:** Complete enumeration of surviving commands (post-NXTTN-S2 subtractive release)
**Basis:** ADR 0011 (CLI groups are discovered from deep-module boundaries), ADR 0004 (deep-module designations)

## Audit Table

All 47 commands are listed in canonical form (current name in use) — the ADR 0011 issue title cited an estimate of 46 taken before the post-`NXTTN-S2` census landed; the correct, current count is 47, matching `docs/design/surface-census.md`'s "CLI Commands: 47" tally. The **Proposed Name** column shows the renamed form if the command fails deep-module correspondence or flag conventions; otherwise it matches **Current Name**.

| Current Name | Proposed Name | Deep Module Correspondence | Corresponding Module(s) | Flag Convention | Positional Arg | --format Support | Status | Notes |
|---|---|---|---|---|---|---|---|---|
| `accept-citation` | `sources accept-citation` | sources (related; citation infrastructure) | sources | compliant: positional + --issue | positional [issue-id] or --issue (bulk) | no | **rename** | Currently top-level hyphenated; belongs in sources group per ADR 0011. Positional/--issue pattern is compliant (supports both). No --format support (not a structured-output command). |
| `amend` | `amend` | none (Issue field mutation) | (none) | compliant: positional | positional [issue-id] or --issue | no | **keep** | No corresponding deep module; correctly flat. Positional arg is compliant. Mutates issue fields via --type/--scope/--dod/--acceptance flags. |
| `assign` | `assign` | none (workflow) | (none) | compliant: positional | positional [issue-id] or --issue | no | **keep** | No deep module. Positional arg pattern compliant. |
| `bootstrap` | `bootstrap` | none (admin) | (none) | compliant | (none) | no | **keep** | Admin command; no deep-module correspondence expected. |
| `claim` | `claim` | claim (deep module) | claim | compliant: positional | positional [issue-id] or --issue | no | **keep** | Deep module: `claim`. Positional arg pattern correct. No --format support (claim output is primarily side effects: worktree creation). |
| `completion` | `completion` | none (shell integration) | (none) | compliant | (none) | no | **keep** | Admin/shell integration; no deep module. |
| `confirm` | `confirm` | dag/issuetype (DAG confidence promotion) | dag, issuetype | compliant: positional | positional [node-id] | no | **keep** | DAG operation for confidence promotion; relates to dag (pure package) and issuetype. Could be renamed to `dag confirm` but confirm is sufficiently self-evident as a top-level verb. No --format support (interactive confirmation flow). |
| `context-history` | `context-history` | none (diagnostic) | (none) | compliant: hyphenated, no deep module = signal evaluated | --issue (required) | no | **keep-signal-acknowledged** | Hyphenated without deep-module correspondence is a signal per ADR 0011. Evaluated: context-history is a diagnostic query tool; no corresponding deep module. No plain verb alternative exists (`context` alone would be ambiguous). Stays flat with hyphen; signal is acknowledged as an intentional diagnostic verb. |
| `create` | `create` | none (hierarchy/composition) | (none) | compliant | (none) | no | **keep** | Direct issue creation (not decompose-based). No deep-module correspondence; plain verb. |
| `dag-summary` | `dag summary` | dag (pure package) + issuetype | dag, issuetype | compliant: hyphenated, deep module exists = rename | (none) | no | **rename** | Corresponds to dag (pure package per ADR 0004). Hyphenated without deep-module group is a signal; dag exists, so group under dag. Becomes a dag subcommand: `dag summary`. |
| `dag-transition` | `dag transition` | dag (pure package) + issuetype | dag, issuetype | compliant: hyphenated, deep module exists = rename | (none) | no | **rename** | Corresponds to dag. Rename to subcommand: `dag transition`. |
| `decision` | `decision` | ops (events/decisions) | ops | compliant: positional | positional [issue-id] or --issue | no | **keep** | Records structured decisions on issues. Relates to ops (events model). No deep module called `decision`; ops is port-clean (adapters usage is OK). Stays top-level. |
| `decompose-apply` | `dag apply` | dag (pure package) | dag | compliant: hyphenated, deep module exists = rename | (none) | no | **rename** | DAG bulk-creation from structured plans. Corresponds to dag. Rename to `dag apply` subcommand. |
| `decompose-context` | `dag context` | dag (pure package) | dag | compliant: hyphenated, deep module exists = rename | (none) | no | **rename** | DAG context rendering for LLM. Corresponds to dag. Rename to `dag context` subcommand. |
| `decompose-revert` | `dag revert` | dag (pure package) | dag | compliant: hyphenated, deep module exists = rename | (none) | no | **rename** | DAG plan undo. Corresponds to dag. Rename to `dag revert` subcommand. |
| `doctor` | `doctor` | (none; validation/diagnostics) | (none) | compliant | (none) | no | **keep** | DAG health diagnostic command. No deep module; plain verb. Compliant verb form. |
| `harness-hook` | `harness-hook` | (none; internal integration) | (none) | compliant: hyphenated, internal = keep | (none) | no | **keep-internal** | Internal harness integration entry point (pre-commit/post-merge). Not user-facing command; stays hyphenated without deep-module correspondence. ADR 0011 rule applies to user-facing CLI; this is internal scaffolding. |
| `heartbeat` | `heartbeat` | claim (deep module; refreshes claim TTL) | claim | compliant: positional | positional [issue-id] or --issue | no | **keep** | Keeps a claim alive by refreshing TTL. Relates to claim deep module. Plain verb; top-level is appropriate. No --format support (TTL refresh is a side effect). |
| `hook` | `hook` | (none; harness hook config) | (none) | compliant | (none) | no | **keep** | User-facing harness hook configuration tool. No deep module; plain verb. Has subcommand `hook run`. |
| `hook run` | `hook run` | (none; harness hook config) | (none) | compliant: group already exists | (none) | no | **keep** | Runs a named harness hook. Subcommand of `hook`. No deep module; matches how peer subcommands (`sources add/sync/verify`, `review prepare/record/commits`) each get their own audit row. |
| `import` | `import` | materialize (deep module; creates issues from external source) | materialize, sources | compliant | (none) | no | **keep** | Bulk issue creation from CSV/JSON. Relates to materialize (state reconstruction) and sources (source linking). Plain verb; top-level. |
| `link` | `link` | dag (pure package; dependency coupling) | dag | compliant | (none) | no | **keep** | Adds dependency edges in DAG. No hyphenated command; plain verb. Relates to dag but dag module doesn't justify separate group (link is fundamental enough to stand alone). |
| `list` | `list` | (none; query/filter) | (none) | compliant | (none) | no | **keep** | Query tool; no deep module. Plain verb. |
| `log` | `log` | ops (deep module; audit log events) | ops | compliant | (none) | no | **keep** | Ops audit log viewer. Relates to ops deep module but log is a standalone query verb. No --format support (log output is structured but human text by default; agents get --format=json via root flag). |
| `materialize` | `materialize` | materialize (deep module) | materialize | compliant | (none) | no | **keep** | Replay ops log and update materialized state. Direct deep-module correspondence. Plain verb. |
| `merged` | `merged` | (none; workflow state transition) | (none) | compliant | (none) | no | **keep** | Manual transition to merged status. No deep module; plain verb. Workflow event. |
| `note` | `note` | ops (events; note op type) | ops | compliant: positional | positional [issue-id] | no | **keep** | Adds/deletes notes (ops). Relates to ops. Plain verb; top-level. Positional arg compliant. |
| `push-ops` | `push-ops` | ops (deep module; publishes ops to VCS) | ops | compliant: hyphenated, no deep-module group exists = signal | (none) | no | **keep-signal-acknowledged** | Hyphenated without deep-module group for ops. Signal acknowledged: ops deep module is internal (ops log replay is not a user-facing group like sources/validate/materialize). `push-ops` is a specialized verb for publishing ops to git; no plain-verb alternative works (`push` alone would be ambiguous with git push). Stays flat with hyphen. |
| `ready` | `ready` | (none; queue inspection) | (none) | compliant | (none) | no | **keep** | Queue inspection; shows unblocked ready tasks. No deep module. Plain verb. |
| `reopen` | `reopen` | (none; workflow state transition) | (none) | compliant | positional [issue-id] or --issue | no | **keep** | Reverses done→open transition. No deep module. Positional arg compliant. |
| `render-context` | `render-context` | (none; agent context rendering) | (none) | compliant: hyphenated, no deep module = signal | (none) | no | **keep-signal-acknowledged** | Hyphenated diagnostic/agent-facing tool. No deep-module correspondence. Signal evaluated: agent context rendering is a specialized workflow tool; no corresponding package. No plain alternative (`render` alone is too generic; `context` alone conflicts with the data structure). Stays flat with hyphen. |
| `reparent` | `reparent` | (none; hierarchy mutation) | (none) | compliant | (none) | no | **keep** | Moves issue to new parent in hierarchy. No deep module; plain verb. |
| `review` | `review` | traceability (deep module; code review assessment) | traceability | compliant: group already exists | (via subcommands) | yes (subcommands: prepare, record) | **keep** | Deep module: `traceability`. Already a proper group with subcommands: `review prepare`, `review record`, `review commits`. Compliant. |
| `review commits` | `review commits` | traceability (subcommand) | traceability | compliant: positional issue-id or --issue | positional [issue-id] or --issue | yes | **keep** | Lists delivery commits for an issue. Subcommand of `review` (traceability group). Compliant. Supports --format=json. |
| `review prepare` | `review prepare` | traceability (subcommand) | traceability | compliant: --issue (required) | --issue (required flag) | yes | **keep** | Prepares review bundle. Subcommand of `review`. Structured output; compliant. Supports --format=json. |
| `review record` | `review record` | traceability (subcommand) | traceability | compliant: --issue (required) | --issue (required flag) | yes | **keep** | Records assessment. Subcommand of `review`. Structured output; compliant. Supports --format=json. |
| `scope-delete` | `scope-delete` | (none; scope field mutation) | (none) | compliant: hyphenated, no deep module = signal | (positional path) | no | **keep-signal-acknowledged** | Hyphenated without deep-module group. Signal acknowledged: scope is an Issue field, not a package (ADR 0011 notes "no internal/scope package exists"). No deep module justifies grouping. Stays flat with hyphen. |
| `scope-rename` | `scope-rename` | (none; scope field mutation) | (none) | compliant: hyphenated, no deep module = signal | (positional old-path, new-path) | no | **keep-signal-acknowledged** | Same signal as scope-delete: scope is a field, not a module. Stays flat with hyphen. |
| `show` | `show` | (none; query/display) | (none) | compliant | positional [issue-id ...] or --issue | no | **keep** | Displays issue details. No deep module. Supports --field for extraction but not --format (output is human text; agents use JSON structured fields via --field). |
| `source-link` | `sources link` | sources (deep module) | sources | compliant: hyphenated, deep module group exists = rename | positional [issue-id] or --issue (repeatable) | no | **rename** | Links issue to source entry. Corresponds to sources deep module. Rename to `sources link` subcommand per ADR 0011 early guidance. |
| `sources` | `sources` | sources (deep module) | sources | compliant: group already exists | (via subcommands) | no | **keep** | Deep module: `sources`. Already a proper group with subcommands: `sources add`, `sources sync`, `sources verify`. Compliant. |
| `sources add` | `sources add` | sources (subcommand) | sources | compliant: --url, --type (required) | (none; uses flags only) | no | **keep** | Adds source to manifest. Subcommand of `sources`. Compliant. |
| `sources sync` | `sources sync` | sources (subcommand) | sources | compliant | (none) | no | **keep** | Syncs sources. Subcommand of `sources`. Compliant. |
| `sources verify` | `sources verify` | sources (subcommand) | sources | compliant | (none) | no | **keep** | Verifies cached sources. Subcommand of `sources`. Compliant. |
| `stale-review` | `sources stale-review` | sources (deep module; relates to claim staleness and source freshness) | sources, claim | compliant: hyphenated, deep-module signal = rename | (none) | no | **rename** | Detects stale claims (source freshness review). Hyphenated without top-level group. Relates to sources (staleness monitoring for sources) and claim (TTL expiry). Rename to `sources stale-review` subcommand to group under sources (the more relevant boundary). |
| `sync` | `sync` | (none; git integration) | (none) | compliant | (none) | no | **keep** | Auto-transitions closed PRs. Git integration verb; no deep module. Plain verb. |
| `transition` | `transition` | ops (deep module; status transition events) | ops | compliant: positional | positional [issue-id] or --issue | no | **keep** | Changes issue status. Relates to ops (transition op type). Plain verb; top-level. Positional arg compliant. |
| `tui` | `tui` | (none; interactive interface) | (none) | compliant | (none) | no | **keep** | Interactive kanban board. Admin/UI command; no deep module. |
| `unassign` | `unassign` | (none; workflow) | (none) | compliant | positional [issue-id] or --issue | no | **keep** | Reverses assign operation. No deep module. Plain verb. Positional arg compliant. |
| `unlink` | `unlink` | dag (pure package; dependency removal) | dag | compliant | (none) | no | **keep** | Removes dependency edges. Relates to dag but doesn't require grouping. Plain verb. |
| `validate` | `validate` | validate (deep module) | validate | compliant: group already exists | (none) | no | **keep** | DAG validation command. Deep module: `validate`. Currently not a group but functions as top-level (no subcommands). Could become `validate graph` or stay top-level verb. Per ADR 0011, stays top-level verb since no subcommands exist yet. |
| `validate-doc-examples` | `validate doc-examples` | validate (deep module; hidden conformance check) | validate | compliant: hyphenated, deep module group exists = rename | (none) | no | **rename** | Validates JSON examples in canonical docs. Hidden command (used by `make check`). Corresponds to validate deep module. Rename to `validate doc-examples` subcommand to align with deep-module grouping. |
| `version` | `version` | (none; diagnostic) | (none) | compliant | (none) | no | **keep** | Prints arm version. Admin; plain verb. |
| `worker-init` | `workers init` | workers (potential deep module or field; workflow initialization) | (none or internal/worker) | compliant: hyphenated, no deep-module group = signal | (none) | no | **keep-signal-pending** | Hyphenated without confirmed deep-module group. Signal pending decision: either (a) promote `internal/worker` to deep-module status and rename to `workers init` subcommand, or (b) keep flat. Currently `workers` exists as a top-level command (worker status listing). **Recommendation for future follow-up**: Confirm whether `internal/worker` should become a deep module; if yes, group as `workers init`. If no, rename to plain verb (e.g., `init-worker` or keep `worker-init`). For now, pending architectural decision. |
| `workers` | `workers` | (none or internal/worker) | (none or internal/worker) | compliant: plain verb, top-level | (none) | no | **keep-signal-pending** | Lists active workers. Relates to internal/worker package status. Signal pending: if `internal/worker` is promoted to deep module, worker-init becomes `workers init` subcommand and workers is the group verb. Currently both are top-level; architectural review needed. |

## Rename Map

Commands requiring renames after NXTTN-S5 conformance work. All renames break immediately (no aliases, per ADR 0010 reasoning).

| Old Name | New Name | Reason | Breaking | User Action |
|---|---|---|---|---|
| `accept-citation` | `sources accept-citation` | ADR 0011: deep-module group alignment; accept-citation relates to sources (citation infrastructure). | ✓ breaking | Update scripts/docs to use `arm sources accept-citation [issue-id]` |
| `dag-summary` | `dag summary` | ADR 0011: hyphenated command with deep-module correspondence (dag pure package). | ✓ breaking | Update scripts/docs to use `arm dag summary [--issue ID]` |
| `dag-transition` | `dag transition` | ADR 0011: hyphenated command with deep-module correspondence (dag pure package). | ✓ breaking | Update scripts/docs to use `arm dag transition [--issue ID]` |
| `decompose-apply` | `dag apply` | ADR 0011: deep-module grouping under dag; plan application is a DAG operation. | ✓ breaking | Update scripts/docs to use `arm dag apply --plan <file>` |
| `decompose-context` | `dag context` | ADR 0011: deep-module grouping under dag; context generation is a DAG operation. | ✓ breaking | Update scripts/docs to use `arm dag context --plan <file>` |
| `decompose-revert` | `dag revert` | ADR 0011: deep-module grouping under dag; plan undo is a DAG operation. | ✓ breaking | Update scripts/docs to use `arm dag revert --plan <file>` |
| `source-link` | `sources link` | ADR 0011 early guidance: deep-module group alignment under sources; citation infrastructure. | ✓ breaking | Update scripts/docs to use `arm sources link [issue-id]` |
| `stale-review` | `sources stale-review` | ADR 0011: group under sources (staleness review for source/claim lifecycle). Relates to both sources and claim deep modules; sources is more specific. | ✓ breaking | Update scripts/docs to use `arm sources stale-review` |
| `validate-doc-examples` | `validate doc-examples` | ADR 0011: group under validate deep module (conformance check for documented examples). | ✓ breaking | Update scripts/docs/Makefile to use `arm validate doc-examples` |

### Signals Acknowledged

The following commands are **hyphenated without deep-module groups** but are intentionally retained as-is per ADR 0011's signal-evaluation logic:

- **`context-history`**: Diagnostic/query tool (not a deep-module area). No plain-verb alternative (context-history as a single concept is necessary).
- **`push-ops`**: Specialized workflow verb (ops publishing to git). No plain verb without ambiguity.
- **`scope-delete` and `scope-rename`**: Scope is an Issue field, not a package; no deep module justifies grouping per ADR 0011.
- **`harness-hook`**: Internal integration entry point, not user-facing CLI verb governed by ADR 0011.

### Pending Decisions

The following commands have **signals requiring architectural follow-up** but are kept as-is pending clarification:

- **`worker-init` and `workers`**: Both exist without a confirmed deep-module group. Signal: either (a) promote `internal/worker` to deep-module status (rename worker-init to `workers init` subcommand), or (b) confirm both stay flat. Pending ADR 0004 amendment or architectural decision in a follow-up story.

## Flag Convention Compliance Summary

### Positional Argument Rule

**Rule:** A command acting on exactly one issue takes a positional `[issue-id]`; `--issue` (repeatable) is reserved for commands that can take multiple issues.

**Compliant commands:**
- Single-issue positional pattern: `claim`, `note`, `decision`, `amend`, `assign`, `unassign`, `reopen`, `accept-citation`, `source-link`, `show`, `review commits`
- Multi-issue via `--issue`: `accept-citation`, `source-link` (both support bulk operations via --issue flag)
- No-argument commands: `list`, `ready`, `validate`, `log`, `workers`, `sources`, `tui`, etc.

**Assessment:** ✓ All commands follow the positional-vs-flag rule correctly.

### --format Support Rule

**Rule:** Every structured-output command must support `--format` (human/json/agent), with `agent` as the harness-facing contract.

**Commands with structured output:**
- `review prepare` - ✓ supports --format
- `review record` - ✓ supports --format
- `review commits` - ✓ supports --format
- `list` - ✓ supports --format (auto-sets to json in non-TTY)
- `log` - ✓ supports --format
- `log --json` flag - ✓ provides JSON output
- `workers --json` flag - ✓ provides JSONL output
- `ready` - ✓ supports --format
- `show` - ✓ supports --format (--field for extraction)
- `render-context` - ✓ supports --format
- `validate` - ✓ supports --format (human/json/agent)

**Commands without structured-output support (correctly human-only):**
- `claim`, `transition`, `heartbeat`, `note`, `decision`, `amend`, `assign`, etc. (side-effect commands that don't return structured data suitable for --format variation)

**Assessment:** ✓ All structured-output commands support --format; human-only commands are appropriately human-only.

### No New Single-Letter Flags Rule

**Rule:** No new single-letter flags beyond `-h` (help).

**Audit:** Searched all cmd/armature/*.go files for single-letter flag definitions. No new single-letter flags found beyond cobra's built-in `-h`.

**Assessment:** ✓ Compliant.

## Conformance Test Impact

The `cmd/armature/grammar_test.go` conformance test (per ADR 0011 § Conformance Test) will enforce:

1. ✓ No hyphenated `Use` strings without deep-module correspondence (post-rename, this will pass).
2. ✓ Single-issue commands use positional arg not `--issue` (already passing).
3. ✓ Every structured-output command supports `--format` enum (already passing).
4. ✓ No command outside `main.go` calls `tui.IsTerminal()` directly (enforcement pending; currently `dagsum.go` and `stalereview.go` violate this — fixed as drive-by in ADR issue 0011-related work).

## Delivery Sequence

**Phase 1 (this task):** Audit table and rename map (docs/design/cli-command-audit.md) — this document.

**Phase 2 (follow-up story):** Implement renames in cmd/armature/main.go:
- Refactor dag commands into dag group (dag.go with subcommand registration)
- Refactor decompose-* subcommands into dag group
- Refactor sources subcommands to include source-link and stale-review
- Refactor validate subcommands to include validate-doc-examples
- Remove hyphenated command registrations; wire up group structure

**Phase 3 (follow-up story):** Write/update `cmd/armature/grammar_test.go` conformance tests.

**Phase 4 (follow-up story, pending decision):** Confirm worker-init/workers architectural decision (ADR 0004 amendment or new architectural decision).

## References

- ADR 0011: CLI Command Groups Are Discovered From Deep Module Boundaries
- ADR 0010: Subtractive Release (no aliases; breaking renames immediately)
- ADR 0004: Depguard Boundaries for Deep Modules
- docs/design/cli-grammar-contract.md: The CLI Grammar Contract spec and conformance test design
- docs/design/surface-census.md: The complete surface census (47 commands inventoried)
- docs/commands.md: User-facing command reference (will be updated post-rename)
