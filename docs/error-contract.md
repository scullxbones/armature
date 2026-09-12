# Failure Code ledger

Agent-facing `arm` failures are a port-level Command Failure (ADR 0020), not a
Graph Finding and not an Agent Output Contract success envelope. Failed
structured invocations write exactly one JSON object to stdout:

```json
{"error":{"code":"CLAIM-1","cause":"issue missing","next_actions":["arm ready","arm list"],"exit_code":1}}
```

The object is `{error:{code,cause,next_actions,exit_code}}`. It is never nested
in `{count, payload[], help[]}`. Human output is `Error [CODE]: cause` plus
`Try:` lines. `json` and `agent` are the same envelope.

## Rules

- **Prefix.** A Failure Code prefix names the originating deep module (CLI
  group per ADR 0011), as `ToUpper(module basename)`: `CLAIM-1`. Subcommands
  share the parent. Commands with no module use `ToUpper(top-level Use)` until
  a module exists (`RENDER-CONTEXT-1`). Reserved prefixes: `USAGE`, `IO`.
  `GENERAL-1` is a retired reserved code — the expand-step wrap for unmapped
  port errors, now unused. It is not a `GENERAL` prefix family; a future
  `GENERAL-2` must earn a real deep module or top-level Use like any other
  code.
- **No padding.** Codes are `PREFIX` or `PREFIX-N`, not `PREFIX-001`.
- **No reuse.** Retired codes stay on this ledger and are never given a new
  meaning. Bidirectional registry↔ledger tests in `make check` enforce that
  live registry codes match non-retired rows; retired rows are exempt from
  the *current* allowed-prefix check (their module or Use may since have
  been removed or renamed) but still hold their originally recorded shape.
- **Next Actions** are recovery commands, not AOC `help[]`. Empty is allowed
  on `IO`. `--help` is for `USAGE` only (ADR 0020: "`--help` is for `USAGE` /
  `GENERAL`" — `GENERAL-1` is retired), not a cop-out on any other specific
  code.

Graph Findings (`arm validate`) and doctor checks are not Command Failures;
they remain the payload of a successful report. `harness-hook` / `arm hook`
stay on the platform/git protocol.

## Ledger

| Code | Module or Use | Meaning | First shipped | Retired |
| --- | --- | --- | --- | --- |
| `GENERAL-1` | reserved | Expand-step wrap for an unmapped port error. Retired once remaining agent-facing `RunE` sites mapped to specific prefixes. Empty `next_actions` was allowed while it lived. | LNGHZN-S6-T1 | unused |
| `USAGE` | reserved | Invalid invocation (missing required flag, extra args, cobra usage). Exit 2. `--help` is an allowed Next Action. | LNGHZN-S6-T1 | |
| `IO` | reserved | I/O failure at the CLI port, or a port error with no command context. Empty `next_actions` allowed. | LNGHZN-S6-T1 | |
| `CLAIM-1` | claim | `arm claim` could not complete (unknown issue, worktree/git miss, already claimed). | LNGHZN-S6-T2 | |
| `TRACEABILITY-1` | traceability | `arm review` could not complete (unknown issue, bundle path/JSON, assessment parse). | LNGHZN-S6-T2 | |
| `READY-1` | ready | `arm ready` could not complete (ops/state load). | LNGHZN-S6-T2 | |
| `TRANSITION-1` | transition | `arm transition` could not complete (invalid status, delivery gate, port error). Use, not a deep module. | LNGHZN-S6-T2 | |
| `RENDER-CONTEXT-1` | render-context | `arm render-context` could not complete (unknown issue). Orphan Use until a module exists. | LNGHZN-S6-T2 | |
| `AMEND-1` | amend | `arm amend` could not complete (unknown issue, invalid field, port error). | LNGHZN-S6-T5 | |
| `ASSIGN-1` | assign | `arm assign` could not complete. | LNGHZN-S6-T5 | |
| `BOOTSTRAP-1` | bootstrap | `arm bootstrap` could not complete when the failure is not already a BootstrapResult protocol payload. | LNGHZN-S6-T5 | |
| `COMPLETION-1` | completion | `arm completion` could not complete (unsupported shell or port error). | LNGHZN-S6-T5 | |
| `CONFIRM-1` | confirm | `arm confirm` could not complete (unknown issue, port error). | LNGHZN-S6-T5 | |
| `CONTEXT-HISTORY-1` | context-history | `arm context-history` could not complete. | LNGHZN-S6-T5 | |
| `CONTEXT-REPORT-1` | context-report | `arm context-report` could not complete. | LNGHZN-S6-T5 | |
| `CREATE-1` | create | `arm create` could not complete (invalid type, introduction check, port error). | LNGHZN-S6-T5 | |
| `DAG-1` | dag | `arm dag` subcommands could not complete (apply/context/transition/revert/summary/override-release). | LNGHZN-S6-T5 | |
| `DECISION-1` | decision | `arm decision` could not complete. | LNGHZN-S6-T5 | |
| `DOCTOR-1` | doctor | `arm doctor` could not complete as a Command Failure (layout/load/fix-apply). Doctor checks remain a successful report, not this code. | LNGHZN-S6-T5 | |
| `GATE-1` | gate | `arm gate run` could not complete when the failure is not already a gate protocol payload. | LNGHZN-S6-T5 | |
| `HEARTBEAT-1` | heartbeat | `arm heartbeat` could not complete. | LNGHZN-S6-T5 | |
| `IMPORT-1` | import | `arm import` could not complete. | LNGHZN-S6-T5 | |
| `LINK-1` | link | `arm link` could not complete. | LNGHZN-S6-T5 | |
| `LIST-1` | list | `arm list` could not complete (ops/state load). | LNGHZN-S6-T5 | |
| `LOG-1` | log | `arm log` could not complete. | LNGHZN-S6-T5 | |
| `MATERIALIZE-1` | materialize | `arm materialize` could not complete. | LNGHZN-S6-T5 | |
| `MERGED-1` | merged | `arm merged` could not complete. | LNGHZN-S6-T5 | |
| `NOTE-1` | note | `arm note` could not complete. | LNGHZN-S6-T5 | |
| `PUSH-OPS-1` | push-ops | `arm push-ops` could not complete. | LNGHZN-S6-T5 | |
| `REOPEN-1` | reopen | `arm reopen` could not complete. | LNGHZN-S6-T5 | |
| `REPARENT-1` | reparent | `arm reparent` could not complete. | LNGHZN-S6-T5 | |
| `SCOPE-DELETE-1` | scope-delete | `arm scope-delete` could not complete. | LNGHZN-S6-T5 | |
| `SCOPE-RENAME-1` | scope-rename | `arm scope-rename` could not complete. | LNGHZN-S6-T5 | |
| `SHOW-1` | show | `arm show` could not complete (unknown issue, snapshot load). | LNGHZN-S6-T5 | |
| `SOURCES-1` | sources | `arm sources` subcommands could not complete. | LNGHZN-S6-T5 | |
| `STATS-1` | stats | `arm stats` could not complete. | LNGHZN-S6-T5 | |
| `SYNC-1` | sync | `arm sync` could not complete. | LNGHZN-S6-T5 | |
| `TUI-1` | tui | `arm tui` could not complete (non-interactive snapshot load). | LNGHZN-S6-T5 | |
| `UNASSIGN-1` | unassign | `arm unassign` could not complete. | LNGHZN-S6-T5 | |
| `UNLINK-1` | unlink | `arm unlink` could not complete. | LNGHZN-S6-T5 | |
| `VALIDATE-1` | validate | `arm validate` could not complete as a Command Failure (load/contradictory flags). Graph Findings remain a successful report, not this code. | LNGHZN-S6-T5 | |
| `VERSION-1` | version | `arm version` could not complete. | LNGHZN-S6-T5 | |
| `WORKER-INIT-1` | worker-init | `arm worker-init` could not complete. | LNGHZN-S6-T5 | |
| `WORKERS-1` | workers | `arm workers` could not complete. | LNGHZN-S6-T5 | |
| `WORKTREE-1` | worktree | `arm worktree` subcommands could not complete when the failure is not already a gc protocol report. | LNGHZN-S6-T5 | |
