# ADR 0024: An `arm mcp` stdio session is not a T1 long-running process

A harness-spawned `arm mcp` session over stdio that lives only as long as
the parent's stdio pipes and exits on EOF is not a long-running process
for constitution tripwire T1. Constitution version becomes 2: the T1 bullet
names this exemption in the ratified text.

## Status

Accepted — amends ADR-0009's T1 (needs a long-running process) with this
narrow session-stdio reading. T2 and T3 are unchanged. Bumps
`CONSTITUTION.md` to version 2.

## Principles touched

T1, I1 (reading only), N2 (unchanged)

## Context

ADR 0009 ratified tripwire **T1**: a proposal that needs a long-running
process dies on contact. `CONSTITUTION.md` and `AGENTS.md` stated T1 with
no exceptions. PR #401 ranked item 28b (`MCPSTDIO-S1`, `arm mcp`) on a
claimed "scoped constitution T1 exemption (not a daemon)" that no ADR
granted. Codex review called that conflict a P1 and asked to keep 28b
out of the ranked backlog until the constitution's amendment process
resolved it. Brian chose to amend T1 narrowly and cite the amending ADR
from that garden, rather than drop 28b.

T1 exists to kill daemons, listeners, and other processes that outlive
the invoking command. A child whose lifetime is the parent's stdio pipes
is the opposite of that: the harness owns the session, and EOF is the
stop signal.

## Decision

ADR 0009's T1 still kills proposals that need a long-running process.
This ADR amends only what counts as one, and lands that reading in the
constitution text itself (version 2):

An `arm mcp` session that a harness spawns over stdio, lives only as
long as the parent's stdio pipes, and exits on EOF is not a long-running
process for T1. The T1 bullet names the exemption: one client, no socket,
one in-process `arm` command per request, exit when that client closes
stdin or signals.

The amendment stays inside these boundaries. Crossing any of them is
still T1:

- no listening socket or port
- no detaching or daemonizing
- no surviving the parent
- no background work after EOF

This is not a general MCP exemption, not a blanket stdio exemption, and
not a redefinition of I1's "no daemon". Anything that is not this
harness-spawned `arm mcp` stdio session remains under unmodified T1.

### Inferred I1 reading (server / daemon)

I1 still says "no database, no server, no daemon." The reading used here
is **inferred**, not a finding in the records: a *server* means a process
with a listen address another client can reach; a *daemon* means a process
that outlives the client that started it. `arm mcp` has neither. The word
"server" in I1 does not settle a client-tied stdio child. N2 is unchanged:
Armature is not an agent framework; the session exposes one tool named
`arm`, and code mode stays in the host.

### Protocol Output inventory

[ADR 0017](0017-agent-output-contract.md) remains the historical decision
that introduced Protocol Output with `harness-hook` as the sole member.
This ADR records the inventory change required by the session: the
Protocol Output commands are `harness-hook` and `mcp`. `mcp` is Hidden so
`arm --help` does not invite a person to occupy stdio. Hidden does not
remove it from the census. Artifact Output is unchanged.

## Consequences

- Item 28b (`MCPSTDIO-S1`) may stay on the ranked backlog when it cites
  this ADR and stays inside the boundaries above.
- Reviewers reject `arm mcp` shapes that listen, detach, outlive the
  parent, or keep working after EOF; those still die on T1.
- Boundary check today is review against this ADR (stdio child, pipes
  tied to the parent, exit on EOF, the four "no"s). No existing lint,
  doctor check, or `make check` step enforces T1. A mechanical hook is
  follow-up work; this ADR does not add one.
- `CONSTITUTION.md` is version 2: T1's bullet carries the exemption
  sentence. `AGENTS.md` names that exemption and cites this ADR.
- N7 / surface census list Protocol Output as `harness-hook` and `mcp`.
  ADR 0017 stays historical; do not rewrite it.
