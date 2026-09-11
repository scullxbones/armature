# The Paved Road

Generated from cobra command metadata in `cmd/armature/main.go`. Do not edit by hand.
Regenerate: `UPDATE_PAVED_ROAD=1 go test ./cmd/armature -run TestPavedRoadHelpClassification_REQ_NXTTN_S4_T1`.

## What this is

The paved road is the one blessed end-to-end pipeline: bootstrap, plan/decompose, wave dispatch, work, review, sync.
Agents should follow that pipeline. Everything else is an escape hatch.
Escape-hatch commands stay in their existing `--help` groups. They are marked `[escape hatch]` inline.
There is no separate escape-hatch group.

## Pipeline

### 1. Bootstrap

Initialize the repo, register this clone as a worker, and confirm the binary.

- `arm bootstrap`
- `arm worker-init`
- `arm version`

### 2. Plan / decompose

Register sources, render plan context, apply the plan, promote drafts, and add dependency edges.

- `arm sources add`
- `arm sources sync`
- `arm dag context`
- `arm dag apply`
- `arm dag transition`
- `arm link`

### 3. Wave dispatch

Survey the graph, take the ready queue, claim with a worktree, and render the worker spec.

- `arm list`
- `arm doctor`
- `arm ready`
- `arm claim`
- `arm render-context`
- `arm worktree list`
- `arm show`

### 4. Work

Record progress, keep the graph green, run the configured gate, and mark the issue done.

- `arm note`
- `arm decision`
- `arm validate`
- `arm gate run`
- `arm transition`

### 5. Review

Prepare the bundle, record the assessment, and list delivery commits.

- `arm review prepare`
- `arm review record`
- `arm review commits`

### 6. Sync

Publish ops, then let merged PRs promote issues.

- `arm push-ops`
- `arm sync`

## Command classification

Every cobra command, including leaf subcommands, is paved or escape-hatch.
Zero unclassified leaf commands is a gate.

### Paved

- `arm bootstrap` — Bootstrap
- `arm claim` — Wave dispatch
- `arm dag`
- `arm dag apply` — Plan / decompose
- `arm dag context` — Plan / decompose
- `arm dag transition` — Plan / decompose
- `arm decision` — Work
- `arm doctor` — Wave dispatch
- `arm gate`
- `arm gate run` — Work
- `arm help`
- `arm link` — Plan / decompose
- `arm list` — Wave dispatch
- `arm note` — Work
- `arm push-ops` — Sync
- `arm ready` — Wave dispatch
- `arm render-context` — Wave dispatch
- `arm review`
- `arm review commits` — Review
- `arm review prepare` — Review
- `arm review record` — Review
- `arm show` — Wave dispatch
- `arm sources`
- `arm sources add` — Plan / decompose
- `arm sources sync` — Plan / decompose
- `arm sync` — Sync
- `arm transition` — Work
- `arm validate` — Work
- `arm version` — Bootstrap
- `arm worker-init` — Bootstrap
- `arm worktree`
- `arm worktree list` — Wave dispatch

### Escape hatch

- `arm amend` — Correct fields after create or apply. Prefer getting the plan right.
- `arm assign` — Soft assignment without a claim. Dispatch uses claim --worktree.
- `arm completion` — Shell completion script. Not an orchestration verb.
- `arm confirm` — Interactive promotion. The road uses dag transition after validate.
- `arm context-history` — Scan git history for context changes. Diagnostic only.
- `arm context-report` — Diagnostic meter for static agent-facing artifacts. Not an orchestration verb.
- `arm create` — Mint one issue by flags. Prefer dag apply.
- `arm dag override-release` — Human Plan Release that skipped validate. Never a green release.
- `arm dag revert` — Undo a plan apply. Not part of the forward pipeline.
- `arm dag summary` — Interactive draft survey. Prefer list and dag transition.
- `arm harness-hook` — Internal harness entrypoint. Hidden from --help groups.
- `arm heartbeat` — The harness hook heartbeats on tool use. Manual heartbeat is for long stretches with no tools.
- `arm hook` — Git hook management. Bootstrap --with-hooks installs them.
- `arm hook run` — Invoke a named git hook. Git and the harness call this, not agents.
- `arm import` — Bulk create from CSV/JSON. Prefer dag apply from a plan.
- `arm log` — Ops audit log. Use when diagnosing, not when dispatching.
- `arm materialize` — Replay ops to rebuild state. Recovery, not the daily loop.
- `arm merged` — Manual merged promotion. Prefer sync after the PR lands.
- `arm reopen` — Rework after done. The road completes once, then syncs.
- `arm reparent` — Move an issue in the hierarchy after the fact.
- `arm review validate` — Advisory assessment check. Record remains the enforcement gate.
- `arm scope-delete` — Drop a scope glob. Prefer amending the plan.
- `arm scope-rename` — Rewrite a scope glob across issues. Planning should not need this.
- `arm sources accept-citation` — Record citation acceptance. Grounding at plan time is the road.
- `arm sources link` — Attach a source after the fact. Prefer --source at create/apply.
- `arm sources stale-review` — Review sources whose cache drifted. Not the daily loop.
- `arm sources verify` — Re-check cached sources. Add and sync are the road.
- `arm stats` — Derived ops-log cost view. Use when diagnosing spend, not when dispatching.
- `arm tui` — Interactive kanban. Agents use list, ready, and show.
- `arm unassign` — Claim is the dispatch reservation. Unassign is recovery.
- `arm unlink` — Remove a dependency. The road adds blocked_by edges; it does not routinely delete them.
- `arm validate doc-examples` — Hidden make-check helper. Not an agent verb.
- `arm workers` — Worker activity dump. Ready and list cover the dispatch loop.
- `arm worktree gc` — Remove worktrees after merged/cancelled. Sync/merged teardown is the road.

### Unclassified

None. Every registered command is classified.

## Defaults audit

These flags exist because the road was not always the default.
Skip and force flags stay listed here. They are not the paved-road invocation.

| Flag | Command | Why it exists |
| --- | --- | --- |
| `--skip-delivery-gate` | `arm transition` | Bypasses the delivery gate on --to done. The paved road runs the gate. |
| `--force` | `arm transition` | Bypasses branch and PR discipline on --to done. |
| `--force` | `arm claim` | Bypasses claim overlap planning. |
| `--force` | `arm merged` | Bypasses hook-violation refusal at merge. |
| `--force` | `arm sources accept-citation` | Skips the confirmation prompt. The paved road cites sources at plan time. |
| `--strict=false` | `arm validate` | Keeps warnings as warnings. The paved road is fail-closed (strict by default). |
| `--global` | `arm bootstrap` | Deploys skills outside the repo. The paved road deploys locally. |
| `--raw` | `arm render-context` | Skips the token budget. The paved road truncates to budget. |
| `--approve-all` | `arm dag summary` | Bulk-approves drafts. Plan release is dag transition after validate. |

