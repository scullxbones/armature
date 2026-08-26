# ADR 0020: Agent-grade error contract

Agent-facing `arm` failures are a port-level Command Failure, not a domain
type and not an AOC success envelope. Origin stays in `internal/`; `cmd/`
maps to a stdout object `{error:{code,cause,next_actions,exit_code}}`. The
migration is expand-then-contract so the envelope is uniform before every
site is coded.

## Status

Accepted

## Principles touched

I4, I5

## Context

LH C3 (`LNGHZN-S6`): agents recover from errors only as well as the error
text allows. Today's path is `fmt.Errorf` plus substring `classifyError`,
JSON `{"error","code","exit_code"}` on **stderr** where `code` is an exit
label (`not_found`), and a dead `ArmatureError` taxonomy nobody calls.

ADR 0017 (Agent Output Contract, in flight as `AOC-S1`) already decided
json ≡ agent, results on stdout, and that **failure payloads are not the
success envelope** `{count, payload[], help[]}`. Nesting `{error}` inside
that object collides with empty-success (`count: 0`). Numbering: 0017–0019
are the AOC cluster; this ADR is 0020 so the two PRs do not collide.

`cmd/` is the CLI adapter (ADR 0004); deep modules and pure packages must
not import a presentation type. `next_actions` names `arm` invocations —
CLI, not domain. ADR 0011: a CLI group exists iff a deep module exists, so
a Failure Code prefix can name both.

Considered and rejected: two live code namespaces (`ArmatureError` + C3);
nesting failures in the AOC envelope; constructing Command Failures inside
deep modules; a shrink-only `classifyError` allowlist; memorizing exit
labels in agent JSON.

## Decision

1. **Command Failure is a port type.** Core returns ordinary errors /
   sentinels. `cmd/` maps them: Failure Code, cause, Next Actions, exit
   code, envelope. Pure packages stay pure. S6 does not deepen `claim.go`.
2. **Wire.** Failed structured invocations write exactly one JSON object to
   stdout: `{error:{code,cause,next_actions,exit_code}}`. Not nested in
   AOC. Drop the exit label. Human is the same fields (`Error [CODE]:
   cause` plus `Try:` lines). Root `SilenceErrors`. `--debug` may dump on
   stderr. json ≡ agent.
3. **Failure Code.** Prefix names the originating deep module (which ADR
   0011 says is the CLI group): `CLAIM-1`. No padding. Subcommands share
   the parent. Orphans with no module use `Use` (`RENDER-CONTEXT-1`) until
   a module exists. Reserved: `USAGE`, `IO`. Guardrail: non-reserved
   prefixes ∈ `ToUpper(module basename)` ∪ `ToUpper(top-level Use)`.
4. **Ledger.** `docs/error-contract.md` lists code, module or Use, meaning,
   first shipped. Retired codes stay on the ledger and are never reused.
   Bidirectional registry↔ledger test runs in `make check`.
5. **Next Actions** are recovery commands, not AOC `help[]`. Light hand:
   exact argv is not required. Cap ~3. Empty is allowed on `IO` (and on
   `GENERAL-1` while it lives). `--help` is for `USAGE` / `GENERAL`, not a
   cop-out on a specific code.
6. **Scope.** Agent-facing `RunE` and cobra usage are in. Panics are out.
   `harness-hook` and `arm hook` are Command Failures conceptually; on the
   wire they stay the platform/git protocol (`docs/harness-hook.md`).
   `adapterExitError` keeps the platform integer. Multi-harness mapping is
   Next-Ten №08, citing this contract — not S6.
7. **Graph Findings and doctor checks are not Command Failures.** They
   remain payload of a successful report (`E2`, `D8`).
8. **Expand then contract.** Early S6 task: delete `classifyError` and
   `ArmatureError`; wrap unmapped port errors as `GENERAL-1` so the
   envelope is uniform. High-traffic mapping is a middle task. Trailing
   task maps every remaining agent-facing `RunE` and **deletes the wrap**.
   The gate is at the port (`RunE` returns a non-`GENERAL` Command
   Failure), not an AST walk of `internal/`. No shrink-allowlist. Alpha:
   no compatibility tax on the old stderr JSON.

## Consequences

- Agents always parse the same failure object; skills match on prefix +
  cause/next_actions, not memorized integers.
- Deep modules do not grow a CLI vocabulary. `internal/errors` is imported
  by `cmd/`, not by depguard-bounded packages.
- `GENERAL-1` exists only during the expand step and remains on the ledger
  as unused/retired afterward.
- AOC success rendering and this failure object share a channel and
  nothing else. Hook stdout is unchanged.
- Follow-up: `LNGHZN-S6` task refresh (T1 wrap, T2 high-traffic, T3
  ledger+tests, T5 trailing migration). T4 is unrelated TUI-seam work
  housed under the same story.
