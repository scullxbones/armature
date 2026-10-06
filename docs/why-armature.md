# Why Armature

Armature is git-native work orchestration for AI coding agents: a typed task DAG,
append-only ops in git, and structural citations from every claim and transition
back to a source document. No database, no server, no daemon.

This page is for adopters comparing options after `v0.1.0`. It distills the
internal market analysis and the first foreign-repo dogfood (a clean
`query-guardian` clone with the release binary). Positioning written before that
dogfood would have been hopeful. This one names the friction that actually
showed up.

## What you are choosing between

| Approach | What it is good at | What breaks at agent scale |
| --- | --- | --- |
| **GitHub Issues + prompts** | Human PM visibility, labels, notifications | No typed DAG for agents; no structural citations; state lives outside the repo agents edit; prompts restate context every session |
| **Plain markdown task lists** | Zero setup, greppable, PR-reviewable | Ambiguous blockers; no claim TTL; no ready queue; agents invent status by rewriting the same file |
| **Beads / Beans-style trackers** | Agent-oriented graphs, ready queries, session memory | Extra persistence (SQLite/Dolt or companion processes); weaker structural requirement citations; not “git alone” |
| **Spec kits / OpenSpec-style SDD** | Upfront alignment before code | Strong on specs, light on multi-worker claims, MRDT ops, and merge-safe coordination |
| **Swarm / persona frameworks** (Gastown, Metaswarm, BMAD, CrewAI, …) | Role playbooks, parallel fan-out | Often assume servers, cloud, or framework runtimes; coordination state is not append-only git ops |
| **Armature** | Zero infra, MRDT single-writer logs, deterministic context, citations, delivery gates | Steeper first hour than a markdown list; draft nodes stay off `ready` until cited and verified |

Competitive detail in the market analysis is point-in-time (March 2026) and
lives in `docs/archive/ai-context-management-market-analysis.md`. Prefer the
architectural columns above over star counts when the landscape moves.

## Where Armature is the fit

Choose Armature when you need **all** of:

1. **Traceability** — every issue cites a source; reviews evaluate delivery against that contract.
2. **Multi-agent coordination without merge fights on coordination state** — each worker appends only to its own ops log.
3. **Offline / repo-local operation** — clone + `arm` binary; no hosted control plane.
4. **Deterministic gates** — `ready`, validate, doctor, and the delivery gate decide; LLM judgment stays advisory.

Skip it when a single human plus a checklist is enough, or when you already
standardized on a hosted tracker and only need agents to comment on tickets.

## Paved road vs escape-hatch `create`

Dogfood on a foreign repo made the two entry paths vivid.

**Paved road** (what the README quickstart and `docs/getting-started.md` teach):

```text
bootstrap → worker-init → sources add/sync/verify
  → dag apply (plan with citations) → dag transition (draft → verified)
  → ready → claim --worktree → render-context → work → transition done
```

**Escape hatch** — `arm create` for a story/task without a plan file. Useful for
ad-hoc nodes. It is not a shortcut past grounding:

- Birth confidence is always **`draft`**.
- Tasks need `--acceptance` (JSON array). Synopsis docs that omit it will fail
  Graph Finding on create.
- `create --source` is not the same flag as `sources link --source-id`.
- Until you **cite** (`sources link` or accept-citation) and **`arm dag
  transition`** the draft subgraph to verified, `arm ready` stays empty even
  though `arm list` shows open issues and `arm claim <id>` can still succeed.

Lead with the paved road. Treat `create` as an escape hatch that still owes
cite + verify before the ready queue is meaningful.

Any committed requirements-like document works as a source (the dogfood repo
used `docs/product-requirements-doc-v2.md`, not only `docs/requirements.md`).

## Draft ≠ ready

This was the sharpest foreign-repo footgun.

| Signal | Draft (birth) | After cite + `dag transition` |
| --- | --- | --- |
| `arm list` | Shows open issues | Shows open issues |
| `arm ready` | Empty | Lists claimable verified leaves |
| `arm ready --explain` | Can report that nothing is excluded while drafts are skipped | Explains real blockers for open verified issues |
| `arm claim <id>` | Often still works by id | Normal ready → claim path |

`ComputeReady` skips `provenance.confidence == draft`. Empty ready plus empty
explain feels like a broken install. It is usually “nodes are still draft.”
Fix: sources → link → `arm dag transition --issue <story-or-node>`, then
`arm ready` again.

Related constitution invariants: append-only ops (**I2**), merge-conflict-free
logs (**I3**), deterministic gates (**I5**), and `done` ≠ `merged` (**I6**).

## Install and bootstrap notes from dogfood

- Prefer the **`v0.1.0` release asset** (or another pinned tag) and verify
  checksums. An older `arm` earlier on `PATH` wins silently.
- **`go install …@latest`** tracks tip; pin `@v0.1.0` when you want the cut.
- If `arm bootstrap` fails with GPG / pinentry on a non-TTY agent session,
  orphan commits on `_armature` cannot sign interactively. Workaround: local
  `git config commit.gpgsign false` for that clone (or a signing agent), then
  retry bootstrap. See also the install section in the README.

## Worked example and demo

- Inspectable artifact trail (no need to run anything): [`examples/completed-story/`](../examples/completed-story/)
- End-to-end demo recording: [`docs/assets/`](assets/) (linked from the README when present)

## Related docs

- [Getting started](getting-started.md)
- [Core concepts](concepts.md) — confidence levels, citations, claims
- [Commands](commands.md)
- [Constitution](../CONSTITUTION.md)
