# Gate Efficiency — Two-Tier Gates, Evidence-Based Acceptance, Bounded Review

Status: ratified in grilling session 2026-08-14 (human decisions by Brian
Scully). Source handoff: S9 coordination cost analysis. This document is the
citable spec for the gate-efficiency story.

## Problem

Coordinating `LNGHZN-S9` showed two dominant cost drivers:

- **Wall clock**: repeated full `make check` runs (~10 min each; mutate 153s,
  coverage-check 140s, test 133s, crosscompile 69s) triggered by small
  remediations were an estimated 60–80% of elapsed time.
- **Tokens**: serial defect discovery (review → fix → review → fix), prose-only
  gate reports forcing reruns, and unrelated `arm validate` warning noise were
  an estimated 45–60% of token spend.

Evidence (do not re-derive):

- `docs/dogfood/findings/raw/2026-08-14T2021Z-5207ee28-coordination-sequential-doc-task-blocks-code-gate.md`
- `docs/dogfood/findings/raw/2026-08-14T2130Z-5207ee28-coordination-worker-green-gate-not-reproducible.md`
- `docs/dogfood/findings/raw/2026-08-14T1933Z-5207ee28-skills-coordinator-custom-worktree-syntax-stale.md`
- `docs/dogfood/findings/raw/2026-08-14T1931Z-5207ee28-coordination-doctor-ok-with-coordinator-blocking-warnings.md`

## Ratified decisions

### D1 — Two-tier gate model (normative)

Two gate profiles with distinct roles:

- **Fast gate** (`make check-fast`): deterministic, diff-routed; used during
  implementation and remediation. Workers MUST NOT run the full gate on
  intermediate remediations.
- **Publish gate** (`make check`): scope unchanged; mandatory exactly
  twice per task lifecycle — once at the final task head, once cumulatively at
  story integration. Its scope is unchanged by this document; ADR 0015
  (docs/adr/0015-recalibrate-mutation-and-coverage-gates.md) subsequently
  recalibrated the publish gate's coverage and mutant-coverage thresholds —
  the gate still runs the same checks, at different numbers.

A green fast gate is sufficient to iterate; only a publish gate confers
delivery. Constitution I5 is preserved: the publish gate still decides.

### D2 — check-fast routing

A routing script computes changed files against `merge-base HEAD origin/main`
(overridable via `BASE=`) and maps surfaces to steps:

| Changed surface | Steps |
|---|---|
| `**/*.go` | lint + build + `go test` on changed packages **plus reverse importers** (`go list`) |
| `skills/**`, `docs/skills/**` | validate-skills, validate-doc-examples |
| `cmd/**`, `docs/design/surface-census.md`, `docs/commands.md` | census-drift-check |
| docs only | adr-principles lint only |

Acceptance anchors: a documentation-only remediation runs no mutation,
coverage, or crosscompile; a CLI flag change runs census plus `cmd` package
tests.

### D3 — Full-gate single-run test/coverage

`check` currently runs the unit suite twice (`test`, then `coverage-check`
re-runs it). The full gate runs the suite once with `-coverprofile`; the
threshold check reads the profile. Saves ~140s per full gate with no rigor
change.

### D4 — Gate evidence (config-declared, wrapper-recorded)

- `.armature/config.json` gains a `gates` map alongside `hooks`:
  `{"fast": {"command": [...]}, "full": {"command": [...]}}`. The profile name
  `full` is **reserved**: it is the publish profile acceptance keys on.
  Armature carries no knowledge of make/Go; unconfigured repos get an error
  from `arm gate run` and today's behavior otherwise (opt-in feature).
- `arm gate run <profile>` executes the configured command, streams output to a
  log file, and appends an **evidence op** to the worker's own log (I3):
  `{profile, command, head SHA, start, end, exit}`. A dirty tree is executed
  but recorded `uncommitted` — citable as nothing. Self-reported gate results
  never count as evidence.
- ReviewBundle includes gate evidence. Acceptance rule: a reviewer or
  coordinator may treat a behavioral gate criterion as satisfied **iff** an
  evidence op exists with `exit=0`, `profile=full`, and SHA equal to the bundle
  head. Older SHA, fast profile, or no op ⇒ rerun required — never
  "indeterminate".

### D5 — Bounded, consolidated review

Normative coordinator/reviewer guidance:

1. One comprehensive initial review (all findings, not first-found).
2. Independent perspectives, if used, run **in parallel** and aggregate into
   one findings list.
3. One consolidated remediation request.
4. One narrow confirmation review, **hard-scoped** to the remediated findings;
   out-of-scope findings are recorded but block only at critical severity.
5. Cap: **3 remediation cycles** per task, then stop and escalate to the human
   (I7).

Reviewer chat responses contain only rating and actionable findings; full
schema-valid assessment JSON is written under `.armature/review/`, referenced
by path.

### D6 — Effort and context hygiene

- Reasoning effort defaults to **medium** for workers and task reviews. High
  effort is explicitly assigned at planning time (concurrency, security,
  cross-cutting refactors) or **auto-escalated when a task enters remediation
  cycle 2**. Story-level final audits remain high.
- Dispatch workers with the rendered task spec and relevant file paths only —
  never an inherited transcript. Reviewers get bundle paths, not inlined
  content. Remediation dispatches state what changed; unchanged skills and
  bundles are not re-read.

### D7 — Strict validation, enforced at introduction

Three doors, three jobs. Partial/`--scope` audit is still rejected.

- **Audit** (`arm validate`): strict **by default** — warnings fail the run,
  green means a single summary line. No scoping flags; the audit validates
  the whole graph or not at all. Rules that fire on intentional states get
  fixed or deleted, not waived. JSON keeps findings in their native
  `errors` / `warnings` / `infos` buckets; `Strict` drives only `OK` and
  the exit code.
- **Introduction (now):** `dag transition` to `verified` requires
  validate-green so a planner cannot release a dirty plan. Keep this
  whole-graph: the planner is about to add work to the union and can still
  stop. A recorded `--skip-validate-gate` exists for humans (I7); happy-path
  errors name the finding, not the escape.
- **Introduction (follow-up, T6):** write-time refusal on `dag apply` /
  `create` / `amend` / `link` so a node that already fails the rules cannot
  land. Fail only on findings that cite IDs the command touched. Default
  fail-closed; recorded override; do not advertise the override in the
  error. That is how birth defects die before plan release, without
  conscripting the next feature worker as janitor.
- **Integration:** whole-graph `arm validate --ci` belongs at **story
  close** and in CI (`make validate-graph`). The per-task publish gate
  (`make check`) does **not** run it. Wiring the union graph into every
  worker's `make check` couples delivery to everyone else's in-flight
  nodes (I3 in spirit) and trains agents onto `check-fast`.
- Rollout includes burning down all existing warnings.

### D8 — Vertical-slice planning validation

Plan-release validation fails when one task's scope touches a censused surface
(`cmd/**`) while a different task in the same story owns the documentation or
census files that surface's drift check reads (`docs/commands.md`,
`docs/design/surface-census.md`). Remedy is co-location: the flag's census row
and command documentation belong to the task adding the flag. Stories deliver
vertical slices, not horizontal layers.

**Delivered (LNGHZN-S10-T5):** `internal/validate.checkE13VerticalSliceCoupling`
groups sibling tasks by parent story, then for each censused surface glob
(currently `cmd/**`) asks a per-task question: does this task's scope touch
that surface without owning any of the census/doc files the surface's drift
check reads, while a sibling in the same story owns them? A match raises an
`E13` error-severity Finding. The error message names the offending task, the
surface glob, every implicated sibling, and the specific coupled file(s), and
states the co-location remedy.

`error` severity means E13 fails **every** `Validate` call, not only strict
ones: `Validate` computes `ok := len(errors) == 0` unconditionally and `Strict`
only additionally folds in warnings. In practice E13 therefore fails
`arm validate` and `arm dag transition --to verified` (D7's plan-release gate).
It is deliberately **exempt from the write-time introduction check**
(`validate.CheckIntroduction`, which every `create` / `amend` / `link` runs), in
the same manner as the E7/E8 cite-after carve-out: decomposition adds tasks one
at a time and the graph is transiently ill-shaped between writes, so refusing
the write would forbid ever reaching an intermediate state a planner must pass
through. E13 judges a plan being released, not a plan being built.

A task whose own scope covers both the code and the census/doc lines is exempt
by construction: same-task ownership is co-location, not coupling, so two
siblings that each carry their own code and doc lines — the vertical slice this
rule exists to reward — raise nothing. `scopeTouchesSurface` asks whether a
scope entry *definitely lands inside* the surface (`scopematch.Allows`), not
whether the two globs could conceivably intersect (`scopematch.Overlaps`, which
documents itself as an over-approximation calibrated for a warning with a
`--force` escape), so a repo-wide task — a lint sweep, a dependency bump — is
not read as a phantom `cmd/**` code task. Only tasks in a non-terminal status
(not `merged`, `done`, or `cancelled`) are considered, mirroring the W1
scope-overlap check, so E13 does not fire against already-merged history.
Findings are one per offending task, citing every implicated sibling, rather
than one per (code task, doc task) pair.

The censused surfaces and the doc files each one reads are authoritative in the
Censused Surfaces table in `docs/design/surface-census.md`;
`internal/validate`'s `censusedSurfaces` map restates it and
`TestCensusedSurfacesMatchesCensusDoc_REQ_LNGHZN_S10_T5` fails if the two drift
apart. E13 is catalogued in `docs/validation-codes.md`.

## Story shape

| Task | Scope theme | Depends on |
|---|---|---|
| T1 | Workflow/skill guidance (D1 normative text, D5, D6) | — |
| T2 | `make check-fast` routing + single-run test/coverage (D2, D3) | — |
| T3 | Gate evidence end-to-end (D4) | — |
| T4 | Strict validate default + burn-down + plan-release enforcement (D7) | — |
| T5 | Vertical-slice plan validation (D8) | T4 |

T1–T4 are parallelizable. T1 is text-only and pays off on the next story even
before tooling lands.

## TUI seam extraction (LNGHZN-S6-T4)

Interactive terminal-entry code in `cmd/armature` lives in `*_tui.go` files
(`ready_tui.go`, `stalereview_tui.go`, `dagsum_tui.go`, `tui_tui.go`): program
construction, model wiring, `tea.NewProgram`. Non-interactive logic and
post-TUI side effects (claim/note/dag-transition ops) stay in the host files
and remain covered. `.gremlins.yaml` excludes `_tui\\.go$` on the same
precedent as `_windows.go`, so mutation no longer treats unreachable
interactive sites as test-quality misses.

`make coverage-check` still counts `*_tui.go` statements (that script is
outside this task's scope). Extracting 0%-covered seam files therefore
slightly lowers the cmd aggregate even as the host files rise.

Measured on `task/LNGHZN-S6-T4` immediately before and after the extract
(same worktree; statement coverage via `make coverage` / `scripts/coverage-check.sh`;
cmd mutant-coverage via `gremlins unleash ./cmd`).

| Metric | Before | After |
|---|---|---|
| cmd statement coverage | 83.86% | 83.56% |
| internal statement coverage | 87.11% | 87.12% |
| cmd mutant-coverage | 95.15% (nearest prior `./cmd` gremlins report: killed 1274, lived 0, not_covered 65; not re-run on this HEAD while the seam tests were red) | 95.35% (this HEAD after extract+exclude: killed 1292, lived 0, not_covered 63) |
| cmd efficacy | 100.00% | 100.00% |

Host-file statement coverage (the non-interactive remainder):

| File | Before | After |
|---|---|---|
| `cmd/armature/ready.go` (`newReadyCmd`) | 62.6% | 66.0% |
| `cmd/armature/stalereview.go` (`newStaleReviewCmd`) | 65.4% | 68.0% |
| `cmd/armature/dagsum.go` (`newDAGSummaryCmd`) | 64.6% | 67.1% |
| `cmd/armature/tui.go` (`newTUICmd`) | 68.2% | 78.9% |

Seam files `runReadyTUI`, `runStaleReviewTUI`, `runDAGSummaryTUI`, and
`runBoardTUI` measure 0.0% statement coverage — they are the excluded
mutation boundary, not a coverage-check exclusion.
