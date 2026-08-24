# The Agent Output Contract

Decision record for `arm`'s structured output surface. Settled 2026-08-23.

## Why

I4 says agents are the primary users. Nothing in the CLI enforced that for output:
`--format agent` was a flag with no contract behind it, and the largest agent-facing
payload in the repo had never been measured.

[AXI](https://axi.md/) (`kunchenguid/axi`) is cited here as **prior art**, not as a
standard we conform to. Its ten principles are a good checklist; its SDK is JavaScript
and its `principles.yaml` is a third-party file with no stability guarantee, so binding
a `make check` gate to it would put an external dependency inside a deterministic gate
(I1, I5). We take the ideas and own the contract under our own names.

## Measured state before the change

Taken from this repo at `v0.0.2-237-g80dee97c`, 738 issues, using the bytes/4 token
convention that `NXTTN-S3-T1` standardises.

| payload | bytes | ~tokens |
| --- | ---: | ---: |
| `arm list` (non-TTY default) | 342,560 | 85,640 |
| same, minimal 4-field schema, pretty JSON | 111,223 | 27,805 |
| same, compact JSON | 89,082 | 22,270 |
| same, TOON | 61,880 | 15,470 |

`outcome` prose accounted for 173,371 of the 342,560 bytes — 638 issues carry one,
mean 271 chars, max 2,132. **Dropping it from list views is 68% of the available
saving; TOON is the last 30% of what remains.**

Three defects found while measuring:

- **`arm list --group` is silently inert in non-TTY.** Byte-identical output with and
  without it. `internal/skillsembed/skills/armature-coordinator/SKILL.md` and
  `armature-planner/SKILL.md` both instruct agents to run it. They receive ungrouped
  data and believe it is grouped.
- **Agent-facing data is split across stdout and stderr by design.** `arm ready`
  prints expired claims to stderr; structured errors go to stderr with stdout empty.
  An agent running `2>/dev/null` loses them silently.
- **`--format agent` is not honoured uniformly.** `arm show` falls through to human
  prose; `arm workers` emits JSONL where `arm list` emits a JSON array; `arm ready`
  emits bytes identical to `--format human`.

Separately, replaying all 9,172 ops in `.armature/ops/` found 43 same-status
transitions, splitting into **29 amendments** (same status, richer payload — e.g.
`LNGHZN-S9-T2` reached `done` seven times, each with a corrected outcome) and
**14 true duplicates** (byte-identical payload — `ORCH-RUNTIME-V1-T3` fired six
empty-outcome `done`s in 15 minutes). True duplicates are 0.153% of the log.

## Decisions

1. **Driver.** Token cost and agent reliability, weighted equally. External
   positioning is not a goal.
2. **AXI is prior art.** Cited, not conformed to. No external file inside a gate.
3. **One contract.** `--format agent` and `--format json` both resolve to a single
   structured-output contract. The non-TTY default is the real surface — deployed
   skills never pass a format flag.
4. **One channel.** Everything an agent reads goes to stdout, errors included.
   stderr is diagnostics-only.
5. **TOON is parked**, not rejected, behind a measurement gate with a written
   re-entry criterion (ADR 0019, park-not-purge per ADR 0010).
6. **List rows carry `id`, `type`, `status`, `title`.** `outcome` is omitted from
   lists — not truncated — because the detail view already exists and a truncated
   preview × 638 rows is the worst of both. `help[]` trails the list, pointing at
   `arm show`.
7. **No default cap, mandatory total count.** A silent cap trades bounded cost for
   unbounded correctness risk in the one command whose job is completeness. Revisit
   at 5,000 issues.
8. **Fail loud.** Unknown or inapplicable flags exit 2 with the valid set listed.
   `--group` gains a structured meaning rather than staying inert.
9. **Idempotency is keyed on payload, not status.** Identical payload is a no-op at
   exit 0 that appends nothing; changed payload appends as an amendment at exit 0.
   An op that cannot change materialized state was never history, so declining to
   append it does not rewrite anything (I2). Suppressed duplicates leave no trace —
   worker thrash is a harness defect belonging in the dogfood corpus, and a counter
   on an existing op would both rewrite history and race under I3.
10. **Protocol Output is exempt.** `arm harness-hook` writes decisions to stdout that
    a runtime parses without judgment; its shape is dictated by Claude Code and Codex,
    not by us. Commands are explicitly classified in code.
11. **Content first, repo-gated.** Bare `arm` in a non-TTY shows the ready queue when
    the cwd is an Armature repo; otherwise a definitive empty state, not a manual.
12. **One envelope everywhere**: `{count, <payload>[], help[]}`, including single-item
    responses. Per-command envelope shapes are what produced the `show`-ignores-`agent`
    bug.
13. **Named**: `Agent Output Contract` and `Protocol Output` enter the glossary, and
    `Surface` is amended to include output shape — which brings the envelope under the
    subtractive-release census.
14. **Two gates, sequenced.** Size (byte budgets, extending `internal/contextreport`)
    then shape (a lint). A byte budget catches neither an ignored flag nor data on
    stderr.
15. **New epic**, four stories.
16. **The fixture corpus is walked from the cobra command tree**, never hand-listed.
    A gate over commands somebody remembered to fixture is not a guardrail. The
    agent-facing/Protocol classification is explicit in code and censused, so
    "mark it Protocol" cannot become an escape hatch.
17. **Three ADRs**: 0017 the contract, 0018 payload-keyed idempotency, 0019 the TOON
    park. The park gets its own file so its re-entry criterion stays findable.
18. **Deliberately unversioned pre-1.0.** No `contract`/`v` field. `TOPTIER-S6-T3`
    (cut v0.1.0) is the freeze point.

## Deliberate deviations from AXI

- **TOON (§1)** — parked, see decision 5.
- **Truncation in lists (§3)** — AXI says never omit a large field entirely. That is
  written for detail views and we follow it there. In a list we omit, per decision 6.
- **Default limits (§2)** — we do not cap, per decision 7.
- **`--fields` (§2)** — not added. It is a surface that must be censused and
  documented forever; add it when something asks for it.

## What the guardrail does and does not cover

`scripts/census-drift-check.sh` already compares CLI commands and flags against
`docs/design/surface-census.md` **bidirectionally**, so a new command or flag added
without a census row already fails `make check`. That is enumeration completeness,
and it is the hard half.

What it does not do is prove *conformance*: the census has no column for output shape,
so a command could be added, censused, pass drift-check, and still emit a bare array
with data on stderr. Decision 13 gives the census jurisdiction; decision 16 makes the
shape lint enumerate from the same source. Both are required — either alone leaves the
hole open one level down.
