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
  `GENERAL-1` is a specific reserved code — the temporary expand-step wrap
  for unmapped port errors — not a `GENERAL` prefix family; a future
  `GENERAL-2` must earn a real deep module or top-level Use like any other
  code.
- **No padding.** Codes are `PREFIX` or `PREFIX-N`, not `PREFIX-001`.
- **No reuse.** Retired codes stay on this ledger and are never given a new
  meaning. Bidirectional registry↔ledger tests in `make check` enforce that
  live registry codes match non-retired rows; retired rows are exempt from
  the *current* allowed-prefix check (their module or Use may since have
  been removed or renamed) but still hold their originally recorded shape.
- **Next Actions** are recovery commands, not AOC `help[]`. Empty is allowed
  on `IO` and on `GENERAL-1` while it lives. `--help` is for `USAGE` and for
  `GENERAL-1` (ADR 0020: "`--help` is for `USAGE` / `GENERAL`"), not a
  cop-out on any other specific code.

Graph Findings (`arm validate`) and doctor checks are not Command Failures;
they remain the payload of a successful report. `harness-hook` / `arm hook`
stay on the platform/git protocol.

## Ledger

| Code | Module or Use | Meaning | First shipped | Retired |
| --- | --- | --- | --- | --- |
| `GENERAL-1` | reserved | Expand-step wrap for an unmapped port error. Envelope is uniform; trailing S6 work maps remaining `RunE` sites and retires this code. Empty `next_actions` allowed. | LNGHZN-S6-T1 | |
| `USAGE` | reserved | Invalid invocation (missing required flag, extra args, cobra usage). Exit 2. `--help` is an allowed Next Action. | LNGHZN-S6-T1 | |
| `IO` | reserved | I/O failure at the CLI port. Empty `next_actions` allowed. Reserved even while unused by a mapper. | LNGHZN-S6-T1 | |
| `CLAIM-1` | claim | `arm claim` could not complete (unknown issue, worktree/git miss, already claimed). | LNGHZN-S6-T2 | |
| `TRACEABILITY-1` | traceability | `arm review` could not complete (unknown issue, bundle path/JSON, assessment parse). | LNGHZN-S6-T2 | |
| `READY-1` | ready | `arm ready` could not complete (ops/state load). | LNGHZN-S6-T2 | |
| `TRANSITION-1` | transition | `arm transition` could not complete (invalid status, delivery gate, port error). Use, not a deep module. | LNGHZN-S6-T2 | |
| `RENDER-CONTEXT-1` | render-context | `arm render-context` could not complete (unknown issue). Orphan Use until a module exists. | LNGHZN-S6-T2 | |
