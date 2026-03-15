# Trellis Epic Decomposition Design

**Date:** 2026-03-14
**Scope:** Break down the Trellis PRD into epics for parallel multi-agent development, with a dogfood cut-over gate at the end of Epic 1.

---

## Goals

1. Decompose the Trellis PRD + architecture doc into epics and stories sized for 3-4 parallel agents.
2. Identify the minimum viable solo workflow (E1) that enables dogfooding Trellis on itself.
3. Define a clean dogfood transition ceremony: E1 ends by loading E2/E3/E4 into Trellis via `trls decompose-apply`.
4. Produce both a design doc (this file) and a `plan.json` for import into the system.

---

## Decisions

- **Epic structure:** Technical domain epics (Approach 2) — one critical-path bootstrap epic, three parallel post-bootstrap tracks.
- **Bootstrap scope:** Full solo workflow (C) — includes `render-context` and `decompose-apply` so the hand-off to dogfooding is a single clean ceremony.
- **Parallelism target:** 3-4 agents post-bootstrap.
- **Output:** Design doc + `plan.json` (both artifacts, for dogfooding ASAP).

---

## Dependency Graph

```
E1: Bootstrap Solo Workflow          [critical path — dogfood gate]
         │
         ├──────────────────────────────────────────┐
         │                                          │
         ↓                                          ↓
E2: Dual-Branch & Multi-Agent        E3: TUI + Governance + Sources
         │                                          │
         └──────────┬───────────────────────────────┘
                    ↓
         E4: Distribution + Ops
```

E2 and E3 share no internal dependencies — they write to different subsystems and run simultaneously with zero coordination. E4 depends loosely on both being mostly complete.

---

## E1: Bootstrap Solo Workflow

**Exit criterion / dogfood gate:** `trls decompose-apply plan.json` successfully loads the E2/E3/E4 plan, `trls ready` returns tasks, and at least one task is claimed and completed using only the `trls` CLI.

### Internal Story Dependencies

```
S1: Data Model & Op Log Engine       [no deps — start here]
S2: Materialization Engine           [depends on S1]
S3: Ready Task Computation           [depends on S2]
S4: Claim System                     [depends on S3]
S5: Context Assembly                 [depends on S2 — parallel with S3/S4]
S6: Status Transitions & Core Ops    [depends on S1 — parallel with S2-S5]
S7: trls init (single-branch)        [depends on S1]
S8: Decomposition Workflow           [depends on S1, S2]
S9: Pre-Transition Hooks             [depends on S4, S6]
S10: Structured Error Diagnostics    [cross-cutting — parallel with S7-S9]
S11: SKILL.md                        [depends on S3-S9 being stable]
```

**Parallelization within E1 (2 agents possible):**
- Agent A: S1 → S2 → S3 → S4 → S9 (state machine path)
- Agent B: S1 → S5 → S8 (context & decomposition path), S6/S7/S10 interleaved

### Stories

| ID | Title | Key Deliverables |
|---|---|---|
| E1-S1 | Data Model & Op Log Engine | JSONL positional array format, all 10 op types (`create`, `claim`, `heartbeat`, `transition`, `note`, `link`, `source-link`, `source-fingerprint`, `dag-transition`, `decision`), per-worker log files, SCHEMA file, filename-worker-ID validation during materialization, CLI-side rate limiters (heartbeats 1/min/issue, creates 500/commit) |
| E1-S2 | Materialization Engine | Incremental replay O(new ops), `checkpoint.json` with SHA + byte offsets, state files (`index.json`, `ready.json`, `issues/*.json`, `traceability.json`, `sources-fingerprint.json`), cold-start full replay with progress indicator, bottom-up rollup (story/epic auto-promote), DFS cycle detection, state files local-only (never committed) |
| E1-S3 | Ready Task Computation | 4-rule gate (open + blockers merged + parent in-progress + claim available), priority sort (explicit > depth > unblock count > age), `ready.json` format, `requires_confirmation` flag for `inferred` nodes |
| E1-S4 | Claim System | Timestamp-based race resolution, lexicographic tiebreaker, TTL + heartbeat protocol, post-claim pull-and-verify (claim → push → pull → re-check), stale claim detection, claim-time scope overlap advisory + auto `note` ops on both tasks |
| E1-S5 | Context Assembly | 7-layer algorithm (core spec, snippets, blocker outcomes, parent chain, open decisions, prior notes, sibling outcomes), fixed vs truncatable sections, advisory token budget (`chars/4`), truncation by priority (lowest first), `--raw` bypass, deterministic output, agent JSON schema, basic human text format (Glamour rendering deferred to E3) |
| E1-S6 | Status Transitions & Core Ops | All status transitions + reverse (`reopen` done→open, blocked→open, claimed→open), `transition` op with optional `branch`/`pr` fields, `create`, `note`, `link`, `decision`, `heartbeat` CLI commands with JSON output |
| E1-S7 | `trls init` (single-branch) | Branch protection detection, single-branch mode selection, `config.json` generation, worker UUID via `worker-init`, project type auto-detection (package.json/go.mod/pyproject.toml/Cargo.toml/Makefile), verification hook proposal (no install yet — deferred to E2) |
| E1-S8 | Decomposition Workflow | `decompose-apply` (E1-E12 validation rules, atomicity guarantee via single file write, idempotency protection via batch ID + plan SHA, `--dry-run`, `--generate-ids`, `--strict`), `decompose-context` (template interpolation with `{{SOURCES}}`/`{{EXISTING_DAG}}`/`{{PLAN_SCHEMA}}`/`{{CONSTRAINTS}}`, basic source stub without provider fetching), `decompose-revert` (double-entry cancellation ops), plan file format v1 |
| E1-S9 | Pre-Transition Hooks | Config-driven verification commands in `config.json`, required vs optional distinction, exit code semantics (0=pass, 1=actionable failure, other=environment error), `{scope}` interpolation, strict phase separation (verify in code worktree, record in ops worktree) |
| E1-S10 | Structured Error Diagnostics | Structured error format (message + relevant state + hint), `--debug` flag (dumps materialized issue, raw log entries, git status, checkpoint state), ops-worktree-specific error table (`ops branch not found`, `ops worktree desync`, `stale worktree`, `materialization failed`) |
| E1-S11 | SKILL.md | Complete AI worker interface: setup (2 commands), work loop (7 steps: find work, get context, claim, execute, verify, complete ops, push code), error recovery (blocked, wrong task, PR rejected), rules and constraints |

---

## E2: Dual-Branch & Multi-Agent Coordination

Runs in parallel with E3 and E4 after E1 ships. Owned by one agent.

### Internal Story Dependencies

```
S1: _trellis Orphan Branch & Ops Worktree   [no deps]
S2: Worker Identity & Registry              [depends on S1]
S3: MRDT Claim Races                        [depends on S1, S2]
S4: Two-Phase Completion                    [depends on S1]
S5: Git Hook Automation                     [depends on S1 — parallel with S3/S4]
S6: trls sync                               [depends on S1 — parallel with S3/S4]
```

### Stories

| ID | Title | Key Deliverables |
|---|---|---|
| E2-S1 | `_trellis` Orphan Branch & Ops Worktree | Orphan branch creation (`git checkout --orphan`), `git worktree add --no-checkout`, sparse checkout (`ops/` + `state/`), `.trellis/` directory, `.gitignore` entry on all branches, `trls init --repair` (delete + recreate), dual-branch detection at init, `trellis.ops-worktree-path` git config |
| E2-S2 | Worker Identity & Registry | UUID generation, `git config --local trellis.worker-id`, uniqueness validation on first push vs existing log filenames, optional `worker-registry.json` (worker UUID → git committer identity cross-reference), multi-machine worker guidance |
| E2-S3 | MRDT Claim Races | Multi-worker timestamp resolution confirmed at read time, lexicographic tiebreaker for identical timestamps, losing-worker recovery path (return to ready queue), push retry loop (`git push` → `git pull --rebase` → retry, cap ~5) |
| E2-S4 | Two-Phase Completion | `done` → `merged` semantic distinction, 4-layer merge detection (commit-message scan via `git log --grep`, branch-name check via `git branch --merged main`, scope-file heuristic, `trls merged` manual fallback), story/epic auto-rollup to `merged` when all children `merged`, abandoned PR staleness check |
| E2-S5 | Git Hook Automation | `post-commit` (auto-heartbeat if active claim, push to ops), `prepare-commit-msg` (prepend active claim issue ID), `post-merge` (run `trls materialize`), hook installation from `.issues/hooks/` templates, `trls init --no-hooks` flag, coexistence pattern for husky/pre-commit |
| E2-S6 | `trls sync` | Pull ops worktree + incremental materialize + change summary output, `--quiet`, `--code`, `--check` flags, exit codes (0=success, 1=ops branch not found, 2=network error) |

---

## E3: TUI + Governance + Sources

Runs in parallel with E2 and E4 after E1 ships. Heaviest track — owned by one agent. S8 (brownfield import) and S9 (stale-review) are most deferrable if needed.

### Internal Story Dependencies

```
S1: Charm TUI Foundation             [no deps — start here]
S2: render-context Human Format      [depends on S1]
S3: trls validate                    [depends on E1 materialization]
S4: Source Document Management       [no TUI dep — parallel with S1-S3]
S5: decompose-context Full Impl      [depends on S4]
S6: dag-summary Interactive TUI      [depends on S1, S7]
S7: Traceability                     [depends on E1 materialization]
S8: Brownfield Import                [depends on E1 decompose-apply]
S9: stale-review TUI                 [depends on S1, S4, S6]
```

### Stories

| ID | Title | Key Deliverables |
|---|---|---|
| E3-S1 | Charm TUI Foundation | Bubble Tea event loop, Lip Gloss declarative styling, Glamour markdown rendering, Bubbles components (spinner, progress bar, table, viewport), semantic color palette (`Critical`/`Warning`/`Advisory`/`OK`/`Info`/`Muted`/`ActionRequired`), cross-platform (Windows Terminal, iTerm2, xterm), TTY detection, `--format` flag plumbing |
| E3-S2 | `render-context` Human Format | Full Glamour-rendered markdown with semantic color mapping, header border (`Info`), scope paths (`Muted`), acceptance checkmarks (`OK`), budget progress bar (`OK`/`Warning`/`Critical` thresholds at 80%/100%), replaces E1-S5's basic text format |
| E3-S3 | `trls validate` | All W1-W11 warning checks, E1-E12 error checks on existing DAG, `--scope <node-id>` subtree validation, `--ci` machine-parseable output, coverage metrics (`cited_nodes / total_nodes`), exit codes (0=clean, 1=errors) |
| E3-S4 | Source Document Management | `sources add/sync/verify` commands, Confluence provider (base_url + space_key + page_id, `version.number` signal), SharePoint provider (site_url + drive_id + item_id, eTag signal), local filesystem provider, normalization to markdown + Mermaid sidecar extraction, `manifest.json`, aggressive cache-on-ops-branch, `source-fingerprint` ops, credential management via `~/.config/trls/providers.toml` + env vars, MCP integration for provider communication |
| E3-S5 | `decompose-context` Full Implementation | Full prompt template interpolation (`{{SOURCES}}`, `{{EXISTING_DAG}}`, `{{PLAN_SCHEMA}}`, `{{CONSTRAINTS}}`), `--sources`, `--existing-dag`, `--template`, `--format`, `--output` flags, source content from cache (expand sparse checkout on demand), exit codes (0=success, 1=no sources, 2=cache missing) |
| E3-S6 | `dag-summary` Interactive TUI | Per-item sign-off checklist (mechanically impossible to bulk-approve), `dag-transition` op per item with signing worker attribution, draft→verified confidence promotion, coverage metrics display, uncited node surfacing (named individually, reviewer acknowledges each by name), `dag-summary.md` audit artifact committed to ops branch, cross-document consistency warning (PRD newer than arch doc by >7 days) |
| E3-S7 | Traceability | `traceability.json` materialization from `source-link` ops, `source-link` op processing in materializer, coverage % computation, surfacing in `dag-summary` and `trls validate` |
| E3-S8 | Brownfield Import | `trls import <file>` (CSV + JSON baseline formats, Jira/Linear as future providers), `provenance.method=imported`, `provenance.confidence=inferred`, `requires_confirmation: true` in ready queue, `trls confirm <node-id>` per-node promotion (inferred→verified), bulk confirmation not supported, `--source` attachment flag, `--dry-run` |
| E3-S9 | `stale-review` Interactive TUI | Per-node flow for source changes: source change summary + node citation + full `render-context` output, confirm/flag/skip options (each recorded as log op), triggered after `trls sources sync` detects fingerprint change |

---

## E4: Distribution + Ops

Runs in parallel with E2 and E3 after E1 ships. Mostly independent features. Owned by one agent.

### Internal Story Dependencies

```
S1: Binary Packaging                 [no deps — start here]
S2: trls status                      [depends on E1 materialization]
S3: trls metrics                     [depends on E1 materialization]
S4: Time Travel & Forensics          [depends on E1 materialization]
S5: Sprint Bookmarks                 [no deps beyond E1 git ops]
S6: SKILL.md as Skill Folder         [depends on S1 stable binary]
S7: Log Compaction Design            [design only — no deps]
```

All of S2–S7 are parallel after S1.

### Stories

| ID | Title | Key Deliverables |
|---|---|---|
| E4-S1 | Binary Packaging | `CGO_ENABLED=0` static build, platform matrix (linux/macOS/Windows × amd64/arm64), GitHub Actions release workflow, binary size target (8–12MB uncompressed / 4–6MB compressed), Makefile `build` and `dist` targets, per-platform binary naming |
| E4-S2 | `trls status` | Dashboard view of all workers (UUID, last heartbeat, active claim), active claim details (issue ID, title, claimed_at, TTL remaining), staleness warnings (claims approaching TTL, tasks stuck at `done` beyond N days), `--worker <id>` filter |
| E4-S3 | `trls metrics` | DAG completion statistics (total/open/in-progress/done/merged by type), velocity tracking (tasks merged per day/week), context budget utilization (avg tokens per render-context call), output as JSON or Glamour table |
| E4-S4 | Time Travel & Forensics | `render-context --at <sha>` (reconstruct exact context at ops branch commit), `trls context-history <issue-id>` (show context changes over time with op attribution), `trls materialize --exclude-worker <id>` (selective replay for corruption diagnosis) |
| E4-S5 | Sprint Bookmarks | `git tag sprint-<N> _trellis` convention, `trls validate --at sprint-<N>` support, sprint boundary state reconstruction, documentation for sprint close ceremony |
| E4-S6 | SKILL.md as Skill Folder | `agentskills.io` open standard packaging, `scripts/trls` binary placement, SKILL.md frontmatter (name, description, version), `references/` subdirectory with architecture summary, `schema.json` structured tool definitions (function calling schemas, parameter types, return types for all agent-invocable commands) |
| E4-S7 | Log Compaction Design | `trls compact` command specification (not implemented), trigger heuristic options (N ops / N days / explicit), rewrite algorithm (one synthetic op per issue → current state), history preservation in git log, documentation as deferred design artifact |

---

## Dogfood Transition Ceremony

At E1 completion:

1. Generate `plan.json` containing E2, E3, E4 epics with all stories, acceptance criteria, scope globs, and source citations pointing to PRD + architecture doc sections.
2. Run `trls init` in the trellis repo (single-branch mode).
3. Register `docs/trellis-prd.md` and `docs/architecture.md` as local filesystem sources.
4. Run `trls decompose-apply plan.json`.
5. Run `trls dag-summary` — human reviews and signs off on the DAG structure.
6. From this point, all remaining development tracked via `trls`.

The `plan.json` is a deliverable of E1-S11 (alongside SKILL.md), generated as part of the E1 exit ceremony.

---

## Parallelism Summary

| Phase | Agents | Tracks |
|---|---|---|
| Bootstrap | 1–2 | E1 (two internal agent paths possible) |
| Post-bootstrap | 3 | E2 (dual-branch + MRDT), E3 (TUI + governance), E4 (distribution + ops) |

### E3 Deferral Options

If E3 falls behind and threatens the overall schedule, the following stories are deferrable to a follow-on cycle with no downstream blocking:
- E3-S8: Brownfield Import (P1, no dependents)
- E3-S9: Stale-Review TUI (P2, no dependents)

---

## Plan.json Artifacts

Two `plan.json` files will be produced:

| File | Contents | When Generated |
|---|---|---|
| `docs/plan-bootstrap.json` | E1 only (all 11 stories as tasks under one epic) | Now — to track bootstrap work before dogfooding |
| `docs/plan-post-bootstrap.json` | E2 + E3 + E4 (3 epics, all stories) | At E1 completion — loaded into Trellis via `trls decompose-apply` |

`plan-bootstrap.json` may be tracked manually or via a minimal task list until E1 is operational.
