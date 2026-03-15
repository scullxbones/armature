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
- **Bootstrap scope:** Full solo workflow — includes `render-context` and `decompose-apply` so the hand-off to dogfooding is a single clean ceremony.
- **Parallelism target:** 3-4 agents post-bootstrap; E2, E3, and E4 run in parallel starting from E1 completion.
- **Output:** Design doc + `plan.json` (both artifacts, for dogfooding ASAP).

---

## Dependency Graph

```
E1: Bootstrap Solo Workflow          [critical path — dogfood gate]
         │
         ├──────────────────────────────────────────┐──────────────┐
         │                                          │              │
         ↓                                          ↓              ↓
E2: Dual-Branch & Multi-Agent        E3: TUI + Governance + Sources    E4: Distribution + Ops
```

E2, E3, and E4 all start immediately after E1 and run fully in parallel — they write to different subsystems with zero cross-track dependencies. E4 can begin as soon as E1 ships; no E2 or E3 feature is required for any E4 story.

---

## E1: Bootstrap Solo Workflow

**Exit criterion / dogfood gate:** `trls decompose-apply plan.json` successfully loads the E2/E3/E4 plan, `trls ready --format=json` returns tasks, and at least one task is claimed and completed using only the `trls` CLI.

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
S11: SKILL.md + plan.json            [depends on S3-S9 being stable]
```

**Parallelization within E1 (2 agents possible):**
- Agent A: S1 → S2 → S3 → S4 → S9 (state machine path)
- Agent B: S1 → S5 → S8 (context & decomposition path), S6/S7/S10 interleaved

### Stories

| ID | Title | Key Deliverables |
|---|---|---|
| E1-S1 | Data Model & Op Log Engine | JSONL positional array format, all 10 op types (`create`, `claim`, `heartbeat`, `transition`, `note`, `link`, `source-link`, `source-fingerprint`, `dag-transition`, `decision`), per-worker log files, SCHEMA file, filename-worker-ID validation during materialization, CLI-side rate limiters (heartbeats 1/min/issue, creates 500/commit) |
| E1-S2 | Materialization Engine | Incremental replay O(new ops), `checkpoint.json` with SHA + byte offsets, state files (`index.json`, `ready.json`, `issues/*.json`, `traceability.json`, `sources-fingerprint.json`), cold-start full replay with progress indicator, bottom-up rollup (story/epic auto-promote), DFS cycle detection, state files local-only (never committed). **`trls materialize` must be a standalone callable CLI command** (not just an internal function) with exit codes: 0=success, 1=materialization error, 2=ops branch not found. **Single-branch mode:** When a task transitions to `done` in single-branch mode, the materializer must also immediately set `merged` on the same task — this is required because the ready-task gate requires `blocked_by` issues to have `status == "merged"`, and there is no separate PR merge step in single-branch mode. The full 4-layer merge detection algorithm is deferred to E2-S4. |
| E1-S3 | Ready Task Computation | 4-rule gate (open + blockers merged + parent in-progress + claim available), priority sort (explicit > depth > unblock count > age), `ready.json` format, `requires_confirmation` flag for `inferred` nodes (surfaced in output; `trls confirm` command to promote them is deferred to E3-S8 — in E1, inferred nodes are blocked from claiming with a clear error message) |
| E1-S4 | Claim System | Timestamp-based race resolution, lexicographic tiebreaker, TTL + heartbeat protocol, post-claim pull-and-verify (claim → push → pull → re-check), stale claim detection, claim-time scope overlap advisory + auto `note` ops on both tasks |
| E1-S5 | Context Assembly | 7-layer algorithm (core spec, snippets, blocker outcomes, parent chain, open decisions, prior notes, sibling outcomes), fixed vs truncatable sections, advisory token budget (`chars/4`), truncation by priority (lowest first), `--raw` bypass, deterministic output, agent JSON schema (`--format=agent`), basic plain-text human format for `--format=human` (Glamour rendering upgraded in E3-S2; the output schema for both `agent` and `human` formats must be stable at E1 — E3-S2 will enhance rendering only, not change the data fields) |
| E1-S6 | Status Transitions & Core Ops | All status transitions + reverse (`reopen` done→open, blocked→open, claimed→open), `transition` op with optional `branch`/`pr` fields, `create`, `note`, `link`, `decision`, `heartbeat` CLI commands with JSON output. **`trls merged <issue-id>` must be implemented as a stub command** that emits a transition op with `{"to": "merged"}` — it is a no-op in single-branch mode (auto-handled by materializer) but the CLI command must exist because E2-S4 activates it as the manual fallback for dual-branch mode, and SKILL.md error recovery references it. **`trls confirm` has no E1 stub** — it does not exist until E3-S8. SKILL.md (E1-S11) must not reference `trls confirm` in any error recovery or work loop steps; the only reference to inferred nodes in SKILL.md is that they cannot be claimed (error message directs user to wait for a human to confirm them). |
| E1-S7 | `trls init` (single-branch) + Worker Identity | Branch protection detection, single-branch mode selection, `config.json` generation, project type auto-detection (package.json/go.mod/pyproject.toml/Cargo.toml/Makefile), verification hook proposal (acceptance/rejection written to config; hook file installation deferred to E2-S5). **`trls worker-init` must be a standalone, independently invocable command** (not only wired through `init`) that generates worker UUID and writes to `git config --local trellis.worker-id`; **`trls worker-init --check`** verifies an existing worker ID is configured without modifying state. **`trls version`** must be implemented (prints module version string); this is the first command called in the SKILL.md setup sequence. |
| E1-S8 | Decomposition Workflow + Minimal Validate | `decompose-apply` (all E1-E12 validation rules, atomicity guarantee via single file write, idempotency protection via batch ID + plan SHA, `--dry-run`, `--generate-ids`, `--strict`), `decompose-revert` (double-entry cancellation ops), plan file format v1. **`decompose-context` stub:** must accept `--sources`, `--template`, `--format`, `--output` flags; **local filesystem sources must work in the stub** (reads file content directly from the given path, no change detection — that is deferred to E3-S4); produces valid JSON with `prompt_template`, `sources` (populated if local files are registered, empty array otherwise), `existing_dag` (when `--existing-dag`), `constraints`, and `plan_schema` fields — output schema identical to E3-S5 so no consumers break. `{{SOURCES}}` interpolates to file content for local sources. **`trls validate --ci` (minimal):** must ship in E1 with structural-only checks (DFS cycle detection, orphan parent references, E6 task required fields — scope/acceptance/definition_of_done). Outputs JSON `{"errors": [...], "warnings": [...]}`, exits 1 on errors. Does NOT include traceability coverage metrics (deferred to E3-S3/S7) — E3-S3 extends this command with the full W1-W11/E1-E12 suite. |
| E1-S9 | Pre-Transition Hooks | Config-driven verification commands in `config.json`, required vs optional distinction, exit code semantics (0=pass, 1=actionable failure, other=environment error), `{scope}` interpolation, strict phase separation (verify in code worktree, record in ops worktree) |
| E1-S10 | Structured Error Diagnostics | Structured error format (message + relevant state + hint), `--debug` flag (dumps materialized issue, raw log entries, git status, checkpoint state). Ops-worktree-specific errors: `ops branch not found` and `ops worktree desync` must return informative stub messages in E1 single-branch mode (e.g., "dual-branch mode not active — run `trls init` in a repo with branch protection, or after E2 is complete"); these errors are fully implemented in E2-S1. `materialization failed` (corrupt log line, skip + warn) is fully implemented in E1. |
| E1-S11 | SKILL.md + plan.json Generation | Complete AI worker interface: setup (2 commands: `trls version`, `trls worker-init --check`), work loop (7 steps), error recovery (blocked, wrong task, PR rejected), rules and constraints. **`plan-post-bootstrap.json` is a first-class deliverable of this story** — not a side artifact. The implementing agent must generate a complete, valid `plan.json` containing E2, E3, and E4 as epics with all stories, acceptance criteria, source citations pointing to `docs/trellis-prd.md` and `docs/architecture.md` sections, and scope globs. This file is the input to the dogfood ceremony. Plan.json generation is a non-trivial task comparable in effort to writing SKILL.md itself. |

---

## Dogfood Transition Ceremony

At E1 completion, using only E1-delivered capabilities:

1. Generate `plan-post-bootstrap.json` (this is E1-S11's primary deliverable).
2. Run `trls init` in the trellis repo (auto-detects single-branch mode).
3. Register `docs/trellis-prd.md` and `docs/architecture.md` as **local filesystem sources** using the E1-S8 stub (no external provider needed; filesystem provider reads local files directly).
4. Run `trls decompose-apply plan-post-bootstrap.json --dry-run` to validate.
5. Run `trls decompose-apply plan-post-bootstrap.json` to create all nodes.
6. Run `trls validate --ci` to confirm DAG structural integrity (minimal E1 version — cycle detection, orphan parents, required fields; delivered by E1-S8). The full interactive `dag-summary` governance gate (E3-S6) is the proper sign-off but requires the Charm TUI (E3-S1). Human reviews `trls validate` output manually before proceeding.
7. Run `trls ready --format=json` to confirm tasks are available.
8. Claim and complete one task using the `trls` work loop.

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
| E2-S1 | `_trellis` Orphan Branch & Ops Worktree | Orphan branch creation (`git checkout --orphan`), `git worktree add --no-checkout`, sparse checkout (`ops/` + `state/`), `.trellis/` directory, `.gitignore` entry on all branches, `trls init --repair` (delete + recreate), dual-branch detection at init, `trellis.ops-worktree-path` git config. Fully implements the `ops branch not found` and `ops worktree desync` error paths stubbed in E1-S10. |
| E2-S2 | Worker Identity & Registry | UUID generation (already in E1-S7; this story adds multi-worker uniqueness validation on first push vs existing log filenames), optional `worker-registry.json` (worker UUID → git committer identity cross-reference), multi-machine worker documentation |
| E2-S3 | MRDT Claim Races | Multi-worker timestamp resolution confirmed at read time, lexicographic tiebreaker for identical timestamps, losing-worker recovery path (return to ready queue), push retry loop (`git push` → `git pull --rebase` → retry, cap ~5) |
| E2-S4 | Two-Phase Completion | Full `done` → `merged` semantic distinction for dual-branch mode (in E1 single-branch mode, materializer auto-sets `merged` at `done` transition time — see E1-S2). **4-layer merge detection for dual-branch mode:** (1) commit-message scan via `git log --grep`, (2) branch-name check via `git branch --merged main`, (3) scope-file heuristic (files in scope changed on main post-transition), (4) `trls merged` manual fallback (command already stubbed in E1-S6 — E2-S4 activates its full implementation). Story/epic auto-rollup to `merged` when all children `merged`. Abandoned PR staleness check (task stuck at `done` > N days). |
| E2-S5 | Git Hook Automation | `post-commit` (auto-heartbeat if active claim, push to ops), `prepare-commit-msg` (prepend active claim issue ID), `post-merge` (run `trls materialize` — command exists as E1-S2 deliverable), hook installation from `.issues/hooks/` templates, `trls init --no-hooks` flag, coexistence pattern for husky/pre-commit |
| E2-S6 | `trls sync` | Pull ops worktree + incremental materialize + change summary output, `--quiet`, `--code`, `--check` flags, exit codes (0=success, 1=ops branch not found, 2=network error) |

---

## E3: TUI + Governance + Sources

Runs in parallel with E2 and E4 after E1 ships. Heaviest track — owned by one agent. S8 (brownfield import) and S9 (stale-review) are most deferrable if needed.

### Internal Story Dependencies

```
S1: Charm TUI Foundation             [no deps — start here]
S2: render-context Human Format      [depends on S1]
S3: trls validate                    [depends on E1 materialization, S7]
S4: Source Document Management       [no TUI dep — parallel with S1-S2]
S5: decompose-context Full Impl      [depends on S4]
S7: Traceability                     [depends on E1 materialization]
S6: dag-summary Interactive TUI      [depends on S1, S7]
S8: Brownfield Import                [depends on E1 decompose-apply]
S9: stale-review TUI                 [depends on S1, S4, S6]
```

Note: **S7 must complete before S6** — `dag-summary` needs `traceability.json` for coverage metrics. The table below orders them accordingly.

### Stories

| ID | Title | Key Deliverables |
|---|---|---|
| E3-S1 | Charm TUI Foundation | Bubble Tea event loop, Lip Gloss declarative styling, Glamour markdown rendering, Bubbles components (spinner, progress bar, table, viewport), semantic color palette (`Critical`/`Warning`/`Advisory`/`OK`/`Info`/`Muted`/`ActionRequired`), cross-platform (Windows Terminal, iTerm2, xterm), TTY detection, `--format` flag plumbing |
| E3-S2 | `render-context` Human Format | Full Glamour-rendered markdown output with semantic color mapping (header border=`Info`, scope paths=`Muted`, acceptance checkmarks=`OK`, budget bar=`OK`/`Warning`/`Critical`). **Extends** the existing `--format=human` path from E1-S5 — upgrades the plain text fallback to Glamour rendering. Does not change the `--format=agent` JSON schema or add new data fields. |
| E3-S4 | Source Document Management | `sources add/sync/verify` commands, Confluence provider (base_url + space_key + page_id, `version.number` signal), SharePoint provider (site_url + drive_id + item_id, eTag signal), local filesystem provider (full implementation; E1-S8 stub handles filesystem reads, this story adds change detection and `sources sync` for local files), normalization to markdown + Mermaid sidecar extraction, `manifest.json`, aggressive cache-on-ops-branch, `source-fingerprint` ops, credential management via `~/.config/trls/providers.toml` + env vars, MCP integration for provider communication |
| E3-S7 | Traceability | `traceability.json` materialization from `source-link` ops, `source-link` op processing in materializer, coverage % computation (cited_nodes / total_nodes), surfacing in `dag-summary` and `trls validate` |
| E3-S3 | `trls validate` (full) | **Extends E1-S8's minimal `trls validate --ci`** with the full check suite: all W1-W11 warning checks, all E1-E12 error checks (E7/citation coverage checks require `traceability.json` from E3-S7 — **E3-S7 must complete before E3-S3**), `--scope <node-id>` subtree validation, coverage metrics (`cited_nodes / total_nodes`), exit codes (0=clean, 1=errors, 0 for warnings-only unless `--ci`) |
| E3-S5 | `decompose-context` Full Implementation | Full prompt template interpolation (`{{SOURCES}}`, `{{EXISTING_DAG}}`, `{{PLAN_SCHEMA}}`, `{{CONSTRAINTS}}`), `--sources`, `--existing-dag`, `--template`, `--format`, `--output` flags, source content from cache (expand sparse checkout on demand). **Replaces E1-S8's stub implementation** using the same output JSON schema — no consumer changes needed on upgrade. Exit codes (0=success, 1=no sources, 2=cache missing). |
| E3-S6 | `dag-summary` Interactive TUI | Per-item sign-off checklist (mechanically impossible to bulk-approve), `dag-transition` op per item with signing worker attribution, draft→verified confidence promotion, coverage metrics display (requires E3-S7 `traceability.json`), uncited node surfacing (named individually, reviewer acknowledges each by name), `dag-summary.md` audit artifact committed to ops branch, cross-document consistency warning (PRD newer than arch doc by >7 days). **This completes the governance gate** that `trls validate --ci` substituted during the bootstrap ceremony. |
| E3-S8 | Brownfield Import | `trls import <file>` (CSV + JSON baseline formats, Jira/Linear as future providers), `provenance.method=imported`, `provenance.confidence=inferred`, `requires_confirmation: true` in ready queue. **`trls confirm <node-id>`** — fully implements the command stubbed in E1-S3 (promotes inferred→verified via `dag-transition` op to ops worktree; bulk confirmation not supported). `--source` attachment flag, `--dry-run`. |
| E3-S9 | `stale-review` Interactive TUI | Per-node flow for source changes: source change summary + node citation + full `render-context` output, confirm/flag/skip options (each recorded as log op), triggered after `trls sources sync` detects fingerprint change |

---

## E4: Distribution + Ops

Runs in parallel with E2 and E3 after E1 ships. All E4 stories depend only on E1 deliverables — no E2 or E3 feature is required. Owned by one agent.

### Internal Story Dependencies

```
S1: Binary Packaging                 [no deps — start here]
S2: trls status                      [depends on E1 materialization]
S3: trls metrics                     [depends on E1 materialization]
S4: Time Travel & Forensics          [depends on E1 materialization]
S5: Sprint Bookmarks                 [depends on E1 git ops]
S6: SKILL.md as Skill Folder         [depends on S1, and E1-S11 SKILL.md content]
S7: Log Compaction Design            [design only — no deps]
```

S2–S7 are all parallel after S1.

### Stories

| ID | Title | Key Deliverables |
|---|---|---|
| E4-S1 | Binary Packaging | `CGO_ENABLED=0` static build, platform matrix (linux/macOS/Windows × amd64/arm64), GitHub Actions release workflow, binary size target (8–12MB uncompressed / 4–6MB compressed), Makefile `build` and `dist` targets, per-platform binary naming |
| E4-S2 | `trls status` | Dashboard view of all workers (UUID, last heartbeat, active claim), active claim details (issue ID, title, claimed_at, TTL remaining), staleness warnings (claims approaching TTL, tasks stuck at `done` beyond N days), `--worker <id>` filter |
| E4-S3 | `trls metrics` | DAG completion statistics (total/open/in-progress/done/merged by type), velocity tracking (tasks merged per day/week), context budget utilization (avg tokens per render-context call), output as JSON or Glamour table |
| E4-S4 | Time Travel & Forensics | `render-context --at <sha>` (reconstruct exact context at ops branch commit), `trls context-history <issue-id>` (show context changes over time with op attribution), `trls materialize --exclude-worker <id>` (selective replay for corruption diagnosis) |
| E4-S5 | Sprint Bookmarks | `git tag sprint-<N> _trellis` convention, `trls validate --at sprint-<N>` support, sprint boundary state reconstruction, documentation for sprint close ceremony |
| E4-S6 | SKILL.md as Skill Folder | **Depends on E1-S11 SKILL.md content** — packages the existing SKILL.md (not re-authors it) into `agentskills.io` open standard format: `scripts/trls` binary placement, SKILL.md frontmatter (name, description, version), `references/` subdirectory with architecture summary, `schema.json` structured tool definitions (function calling schemas, parameter types, return types for all agent-invocable commands) |
| E4-S7 | Log Compaction Design | `trls compact` command specification (not implemented), trigger heuristic options (N ops / N days / explicit), rewrite algorithm (one synthetic op per issue → current state), history preservation in git log, documented as deferred design artifact |

---

## Parallelism Summary

| Phase | Agents | Tracks |
|---|---|---|
| Bootstrap | 1–2 | E1 (two internal agent paths possible within E1) |
| Post-bootstrap | 3 | E2 (dual-branch + MRDT), E3 (TUI + governance), E4 (distribution + ops) |

### E3 Deferral Options

If E3 falls behind, the following stories are deferrable with no downstream blocking:
- E3-S8: Brownfield Import (P1, no dependents within E3)
- E3-S9: Stale-Review TUI (P2, no dependents)

---

## Plan.json Artifacts

Two `plan.json` files are produced:

| File | Contents | When Generated |
|---|---|---|
| `docs/plan-bootstrap.json` | E1 only (all 11 stories as tasks under one epic) | Now — to track bootstrap work before dogfooding |
| `docs/plan-post-bootstrap.json` | E2 + E3 + E4 (3 epics, all stories) | Deliverable of E1-S11 — loaded into Trellis via `trls decompose-apply` at the dogfood ceremony |

`plan-bootstrap.json` is tracked manually until E1 is operational.
