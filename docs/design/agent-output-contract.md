# The Agent Output Contract

Decision record for `arm`'s structured output surface. Settled 2026-08-23.
Ratified by [ADR 0017](../adr/0017-agent-output-contract.md).

This file is the lint target. A later shape lint walks the cobra command
tree and checks a golden fixture per agent-facing command against **Normative
spec** only. A rule that is not in that section is not a lint rule.

The contract is the expand-contract *target*. Current CLI writers are not
required to conform until they are migrated.

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
10. **Two carve-outs, both cited.** `arm harness-hook` is **Protocol Output**: it
    writes decisions to stdout that a runtime parses without judgment, in a shape
    dictated by Claude Code and Codex. Separately, some commands emit a **canonical
    artifact** on stdout — `review prepare` writes a ReviewBundle whose schema
    requires its own top-level fields, `completion` writes a shell script, and
    `dag apply --schema|--example` writes a JSON Schema document. Enveloping those
    would break the documented redirect-to-file flows, so they are **Artifact
    Output**. Because two of the three qualify only under particular flags, the
    class attaches to a command *mode*, and admission requires citing the governing
    schema or consumer. Both carve-outs are explicitly classified in code and
    censused; neither is available by name or by omission.
11. **Content first, repo-gated.** Bare `arm` in a non-TTY shows the ready queue when
    the cwd is an Armature repo; otherwise a definitive empty state, not a manual.
12. **One envelope everywhere**: `{count, <payload>[], help[]}`, including single-item
    responses. Per-command envelope shapes are what produced the `show`-ignores-`agent`
    bug.
13. **Named**: `Agent Output Contract`, `Protocol Output` and `Artifact Output` enter
    the glossary, and `Surface` is amended to include output shape — which brings the
    envelope under the subtractive-release census.
14. **Two gates, sequenced.** Size (byte budgets, extending `internal/contextreport`)
    then shape (a lint). A byte budget catches neither an ignored flag nor data on
    stderr.
15. **New epic**, four stories.
16. **The fixture corpus is walked from the cobra command tree**, never hand-listed.
    A gate over commands somebody remembered to fixture is not a guardrail. The
    three-way classification is explicit in code and censused, and a carve-out must
    cite the harness protocol or governing schema that makes the envelope impossible,
    so neither "mark it Protocol" nor "mark it Artifact" can become an escape hatch.
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

## How the work is decomposed

Epic `AOC`, four stories, 15 tasks. `arm dag apply` and `arm validate` are both green
against this shape; the plan is checked in at `docs/design/agent-output-contract-plan.json`
and regenerated from the graph so the two cannot drift silently.

Two adjustments came out of applying it, and both are worth keeping visible because
they were forced by the repo's own rules rather than chosen:

**The S2 migrations run as a chain, not a parallel wave.** The first draft put every
command's `docs/commands.md` update in one doc-sweep task so the code tasks would stay
scope-disjoint. `arm validate` rejected that with E13 — "stories deliver vertical
slices, not horizontal layers" — and E13 is right: the doc belongs with the code that
changes it. But only one task may own a file at a time, so co-locating forces
`AOC-S2-T2 → T1 → T3 → T4 → T5` into a sequence. The parallelism was never real; it
was purchased by deferring documentation, which is the trade E13 exists to refuse.

**The error-channel move folded into `LNGHZN-S6-T1`.** It was drafted as its own task
(`AOC-S2-T6`, now cancelled) on the reasoning that chaining to `LNGHZN-S6` would stall
behind an unrelated queue. Two facts corrected that: `LNGHZN-S10-T12` is already
merged so `LNGHZN-S6-T1` is ready now, and the two tasks edit the same four files to
do overlapping work. `LNGHZN-S6-T1` now carries the channel move in its definition of
done and is blocked by `AOC-S1-T2` so the envelope exists before the error type nests
into it.

`AOC` also has 17 dependency edges into the existing backlog — `LNGHZN-S6`, `NXTTN-S4-T1`,
`TOPTIER-S11`, `TOPTIER-S15`, `ARCHIMP-S20-T2`. That density is inherent: the contract
touches every command, and open work already exists on most of them. `doctor`, `dagsum`
and `render-context` are deliberately excluded from `AOC-S2-T4` for this reason and
migrate after their owners land, caught by the `AOC-S3-T3` lint rather than scheduled
by hand.

## Normative spec

Keywords MUST / MUST NOT / SHOULD / MAY are used as in RFC 2119.

### N1. Applicability

1. The contract applies to every **agent-facing** command mode when
   `--format=json` or `--format=agent` is in effect, including when that
   format is implied (`--non-interactive`, non-TTY auto-detect). Modes
   classified Protocol Output (N7) or Artifact Output (N8) are outside it.
2. `--format=json` and `--format=agent` MUST emit the same envelope object:
   same required keys, types, and semantics. Whitespace MAY differ.
3. `--format=human` is unconstrained by this spec.
4. Failure reporting (non-zero exit, error object) is out of scope here.
   Result data still MUST NOT be moved to stderr because an error path
   exists.

### N2. Envelope

On a successful structured invocation, stdout MUST contain exactly one JSON
value: a single object, then one terminating newline. That object MUST
contain:

| Key | Type | Rule |
|---|---|---|
| `count` | JSON number | Integer ≥ 0. MUST equal the length of the payload array. MUST be the true total; the payload MUST NOT be silently capped or paginated. |
| *payload key* | JSON array | Command-declared plural name for the contents (`issues`, `workers`, `worktrees`, …). MUST NOT be the literal key `payload` unless the contents are themselves named payload. MUST be present. MUST NOT be `null`. |
| `help` | JSON array of strings | MUST be present. MUST NOT be `null`. Each element MUST be a non-empty string. |

The payload key is per-command. Issue-inventory commands (`list`, `ready`,
and any new issue list) declare `issues`. `show` is a detail view of one
issue: same envelope, payload key `issues`, `count` 1. A missing issue is
an error, not an empty state.

Additional members MAY exist (adjuncts). An adjunct MUST NOT replace
`count`, the payload array, or `help`, and MUST NOT be the only place a
result row is represented. Adjuncts are how `--waves` and expired claims
fold into the same object (see N3, examples).

JSONL, a top-level array, a top-level string, or more than one JSON value
on stdout MUST NOT be used for agent-facing structured output.

### N3. Channel

1. The envelope MUST be written to stdout.
2. Result data MUST NOT be written to stderr. stderr is not a second
   structured results channel.
3. Diagnostics that are part of the result (expired claims, wave grouping,
   truncation notices that describe the payload) MUST appear as envelope
   members, not as a sibling JSON value on stderr.

### N4. Default list schema

The default issue-inventory row (`arm list`) MUST be a JSON object with
exactly these keys, all strings:

- `id`
- `type`
- `status`
- `title`

`outcome` MUST NOT appear on a list row. Truncating it is not a substitute
for omitting it. Detail lives on `arm show`.

Additional row keys MUST NOT appear on the default `arm list` row. Other
commands that use the `issues` payload key MUST include those four keys and
MAY extend the row with command-specific keys documented in the fixture the
shape lint will check. They MUST NOT add `outcome` to a list row.

### N5. `help[]`

1. `help` trails the payload. It is next-action text, not a second copy of
   the rows.
2. Elements SHOULD be one line each and SHOULD name a concrete command when
   a next step exists.
3. Issue-list commands MUST include an element that points at `arm show`
   for detail.
4. `help` MUST NOT embed payload rows, stack traces, or debug dumps.
5. Order is stable: index 0 is the most actionable hint.

### N6. Empty state

A successful match of zero items is still the envelope:

- `count` MUST be `0`.
- The payload array MUST be present and empty (`[]`), not omitted, not
  `null`.
- `help` MUST be non-empty and MUST name the reason the result is empty
  (filter, queue, or environment). That reason MUST NOT live only on
  stderr or only in a human-prose line.

A bare `[]`, an omitted payload key, a missing envelope, or exit 0 with no
stdout MUST NOT represent emptiness. A failed invocation is an error, not
an empty state.

### N7. Protocol Output carve-out

A command classified **Protocol Output** MUST NOT emit this envelope. It
speaks its own bidirectional stdin/stdout protocol with the host harness,
and the wire shape is dictated by that harness rather than by us.

Exemption from envelope lint is by that classification, never by command
name, path, or `Use` string.

The sole Protocol Output command is `harness-hook`.

### N8. Artifact Output carve-out

A command mode classified **Artifact Output** MUST NOT emit this envelope.
Its stdout *is* a canonical artifact: a document whose shape is fixed by a
schema or by a consumer that is not this contract, and which a downstream
tool reads verbatim.

A mode qualifies as Artifact Output only if all three hold:

1. **Named consumer.** Something other than a general-purpose agent reads
   it — a JSON Schema validator, a shell, another `arm` subcommand.
2. **Foreign shape.** A required top-level shape already governs the output,
   and wrapping it in `{count, <payload>[], help[]}` would break that shape.
   The governing schema or consumer MUST be cited in the classification.
3. **Verbatim redirect is a documented flow.** The output is meant to be
   redirected to a file or piped, not read as a result set.

Prose that is merely long, or a result set that happens to be large, does
NOT qualify. Neither does a shape this contract could own — those are
migration targets, not exemptions. `render-context` is the worked negative
example: its `--format=agent` shape is ours, so it stays agent-facing and
migrates on schedule.

Because two of the three current members produce an artifact only under
particular flags, Artifact Output attaches to a **command mode**, not to a
command. The classification MUST name the selecting flags, and every other
structured invocation of the same command MUST conform to the envelope.

The Artifact Output modes are:

| Command mode | Consumer | Governing shape |
|---|---|---|
| `review prepare` with `--output` unset | the reviewer skill | `docs/schemas/review-bundle.schema.json` — requires top-level `schema_version`, `bundle_id`, `issue`, `contract`, `delivery`, `fingerprints` |
| `completion <shell>` (all modes) | bash / zsh / fish / powershell | the shell's own completion-script grammar; not JSON at all |
| `dag apply --schema`, `dag apply --example` | JSON Schema validators, plan authors | a JSON Schema document / a plan instance, each governed by the plan format |

`review prepare --output <file>` and `dag apply` without those flags are
agent-facing and MUST conform.

### N9. Command classification

Every CLI command mode is **agent-facing**, **Protocol Output**, or
**Artifact Output**.

1. The default is agent-facing.
2. Protocol Output and Artifact Output MUST each be an explicit
   classification in code, and MUST appear in the surface census.
3. A new command MUST NOT enter either carve-out by omission, by copying a
   name pattern, or by skipping a fixture.
4. An Artifact Output classification MUST cite the governing schema or
   consumer that makes the envelope impossible (N8.2). A classification
   that cites nothing is not admissible.
5. Where a command has both artifact and result modes, the classification
   MUST name the selecting flags, and the shape lint MUST fixture both.

## Worked examples

Successful list (default row, `help[]` points at detail):

```json
{
  "count": 2,
  "issues": [
    {"id": "AOC-S1-T1", "type": "task", "status": "claimed", "title": "Contract definition: ADR 0017 and the normative output spec"},
    {"id": "AOC-S1-T2", "type": "task", "status": "open", "title": "Envelope and channel helpers, added alongside existing output"}
  ],
  "help": ["arm show <id> for outcome, scope, and acceptance"]
}
```

Definitive empty ready queue:

```json
{
  "count": 0,
  "issues": [],
  "help": ["no issues are ready to claim; blockers are unmerged or claims are active"]
}
```

Ready queue with adjuncts (waves and expired claims stay on stdout):

```json
{
  "count": 2,
  "issues": [
    {"id": "AOC-S2-T2", "type": "task", "status": "open", "title": "arm list conforms to the contract"},
    {"id": "AOC-S2-T1", "type": "task", "status": "open", "title": "arm ready conforms to the contract"}
  ],
  "waves": [
    ["AOC-S2-T2"],
    ["AOC-S2-T1"]
  ],
  "expired_claims": [
    {"id": "AOC-S9-T1", "status": "claimed", "claimed_by": "worker-1"}
  ],
  "help": [
    "dispatch one wave at a time",
    "arm show <id> for detail"
  ]
}
```

Non-conforming shapes (lint MUST reject for agent-facing commands):

- `[{"id":"X","title":"…"}]` — top-level array, no envelope, no `count`/`help`.
- `{"count":1,"issues":[]}` — `count` ≠ payload length; `help` omitted.
- `{"count":0}` — payload key omitted; empty state is not definitive.
- Two JSON values, stdout array plus stderr array — second channel.

## Sequencing

- **AOC-S1-T2** adds envelope constructors and classification beside existing
  writers. No command migrates there.
- **AOC-S1-T3** adds the cited, mode-sensitive Artifact Output classification
  omitted from T2. Existing output remains byte-identical.
- **AOC-S2** migrates agent-facing commands onto this envelope.
- **AOC-S3** deletes the legacy writers and installs the cobra-enumerated
  shape lint against this document.
- Alternate encodings are out of this spec.
