# ADR 0017: Agent Output Contract

## Status

Accepted

## Principles touched

I4, I5

## Context

`--format` already accepts `human` / `json` / `agent`, and the CLI grammar
reserves `agent` as the harness-facing contract. There is no contract. Each
command invents a shape:

- `arm list` emits a bare JSON array, including `outcome` on every row.
- `arm workers --json` emits JSONL.
- `arm ready` writes expired claims as a second JSON array on stderr, so
  `2>/dev/null` silently drops them.
- `--group` is documented as human-only and is ignored in structured output.
- An empty match is sometimes a bare `[]`, sometimes no output.

That mix is a tax on the primary user (I4). Measured on this repo, `arm list`
is ~86k estimated tokens; the same inventory under a four-field default row
is ~22k. Agents cannot compile against a shape that is not written down, and
a later deterministic lint (I5) cannot exist until the shape is named.

`arm harness-hook` is a different kind of command: it speaks a
platform-native allow/block protocol on stdin/stdout with the host harness,
not an inventory of Armature issues. Folding it into the same envelope would
break hook integration.

## Decision

Agent-facing structured output follows one **Agent Output Contract**:

1. **Single channel.** The result is one JSON object on stdout. Result data
   does not appear on stderr. stderr is not a parallel structured channel.
2. **Uniform envelope.** That object is `{count, <payload>[], help[]}`.
   `count` is the true length of the command's payload array. The payload
   key is named for its contents (`issues`, `workers`, …), not the literal
   key `payload`. `help` is always an array of strings.
3. **Protocol Output carve-out.** A command classified Protocol Output does
   not emit this envelope. Classification is explicit. The default is
   agent-facing, so a new command cannot silently opt out. The sole Protocol
   Output command is `harness-hook`.

The normative rules — envelope members, default list schema, `help[]`,
empty-state, channel, and classification — live in
[`docs/design/agent-output-contract.md`](../design/agent-output-contract.md).
That document is the lint target. This ADR records the decision; it does not
duplicate the rule list.

`--format=json` and `--format=agent` emit the same envelope. `--format=human`
is unconstrained. Failure/error payloads are not this contract; they belong
to the agent-grade error work (`LNGHZN-S6`).

This is the expand half of an expand-contract sequence. Existing writers stay
until later stories migrate and then delete them.

## Consequences

- Downstream tasks can add envelope helpers and migrate commands against a
  written shape instead of inventing one per vertical slice.
- A cobra-enumerated shape lint can fail a new agent-facing command that
  ships without a conforming fixture.
- Token cost of inventory commands is bounded by the default list row, with
  `help[]` pointing at `arm show` for detail.
- `harness-hook` stays on its platform protocol. Exemption is by
  classification, never by name, so a renamed hook does not accidentally
  become agent-facing, and a new command cannot hide from the lint by
  copying the hook's name pattern.
- Alternate encodings (TOON and others) are out of this ADR; parking or
  adopting them is a later decision.
