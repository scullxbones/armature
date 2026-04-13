# Architecture TOC Gap Resolutions

Resolves the 5 gaps identified during TOC development. These decisions feed into the architecture document.

---

## Gap 1: trls sync — Recommendation

### Background: Where Does Sync Currently Live?

The concept of "pull before operating" is scattered across four documents with inconsistent descriptions:

**DESIGN.md (line 68):** Step 1 of the process flow is `git pull`. Assumes single branch, one worktree.

**DESIGN-BRANCHING.md (line 47):** Revised process flow step 1 is `cd ops-worktree && git pull`. Now targets the ops worktree specifically, but this is described as an internal implementation detail of the CLI, not a user-facing command.

**DESIGN-WORKFLOWS.md SKILL.md skeleton (line 757):** AI workers are told to run `git pull` as step 1 of the work loop. This is the agent-facing contract, and it currently describes the wrong operation — it would pull the code branch, not the ops branch.

**DESIGN-BRANCHING.md hooks:** The `post-merge` hook triggers materialization after `git pull` on the code branch brings new commits to main. This is a different sync path — it syncs code, then triggers ops materialization as a side effect.

The problem: there is no single command that does "make my local ops state current." The concept exists implicitly in every CLI command's internal flow, but the SKILL.md and human operator need an explicit way to say "sync me" without knowing about worktrees.

### Recommendation: Implicit Sync With Explicit Override

**Every read or write command implicitly syncs the ops worktree before executing.** This means `trls ready`, `trls claim`, `trls render-context`, `trls transition` — all of them pull the ops branch as their first internal step. The developer and the AI agent never need to think about syncing ops.

**`trls sync` exists as an explicit command** for two use cases:

1. **Diagnostic/verification:** "Is my ops state current?" A developer troubleshooting a stale claim or unexpected ready-task list can run `trls sync` and see what changed.

2. **Batch operations:** If a script or CI job needs to ensure fresh state before a sequence of read-only operations (e.g., generating a report), a single `trls sync` at the top is cheaper than implicit sync on each command.

The command specification:

```
trls sync [flags]

Behavior:
  1. Pull ops worktree (_trellis branch)
  2. Incremental materialize (process new ops since checkpoint)
  3. Report summary of changes

Flags:
  --quiet          Suppress output (for scripting)
  --code           Also pull the code branch (convenience wrapper)
  --check          Report sync status without pulling (is local behind remote?)

Output:
  synced: 3 new ops from 2 workers
  materialized: 1 task claimed, 1 task merged, 0 new tasks

Exit codes:
  0  success (or already current)
  1  ops branch not found (run trls init)
  2  network error (offline — local state is stale)
```

**SKILL.md contract change:** Replace `git pull` in step 1 with nothing — the agent just runs `trls ready` and sync happens automatically. If we want to be explicit in the SKILL.md for clarity, the step becomes `trls sync` followed by `trls ready`, but the sync is redundant since `ready` syncs internally. Recommend keeping it explicit in the SKILL.md for readability even though it is technically unnecessary.

**Impact on the process flow:** The 6-step flow from DESIGN-BRANCHING.md is now internal to every CLI write command. Read-only commands (ready, render-context, validate) do steps 1-2 only (pull + materialize, no write/push). This is the consolidated specification that the architecture doc should describe once.

### Document Sprawl Note

The process flow is currently described three times with three different levels of accuracy. The architecture document should have ONE canonical process flow description with two variants (read-only and read-write), and the SKILL.md should reference it by behavior, not by internal steps. The design docs become historical only.

---

## Gap 2: Epic/Story Merged Rollup — Decision

**Decision (from Brian):** An epic is `done` when all its child stories have status `merged`. At that point, the epic itself is also marked `merged`.

### Implications

**Rollup rules by type:**

| Type | Becomes `done` when | Becomes `merged` when |
|---|---|---|
| Task | Worker transitions it | CLI detects code on main |
| Story | All child tasks are `merged` | Same moment (auto-set with `done`) |
| Epic | All child stories are `merged` | Same moment (auto-set with `done`) |

For stories and epics, `done` and `merged` are set simultaneously because there is no separate code artifact to review. The two-phase gap (done but not yet merged) only exists for tasks, which are the leaf nodes that produce code.

**Implementation:** During materialization, after processing merged-status promotions for tasks, run a bottom-up rollup pass: for each story, check if all children are `merged`. If so, emit a synthetic transition op marking the story `done` and `merged`. Repeat upward for epics.

**Ready-task computation:** No change needed beyond what was already decided. Tasks check blockers for `merged`. Stories and epics are never directly claimed, so their status only matters for the parent-is-in-progress gate and for reporting.

**The parent-is-in-progress gate for tasks:** A task is ready when its parent story is `in-progress`. A story becomes `in-progress` when at least one child task is `claimed` or `in-progress`. This is unchanged. The merged rollup only affects when stories and epics reach their terminal state.

---

## Gap 3: Verification Phase Worktree Targeting — Recommendation

### The Problem

`trls transition <id> done --verify` runs acceptance hooks (test runners, linters, type checkers) that operate on code files. These files are in the developer's code worktree, not the ops worktree. The CLI needs to know where the code worktree is so it can run hooks in the right directory.

Additionally, the hook commands themselves are project-specific. A Node.js project runs `npm test`. A Go project runs `go test ./...`. A Python project runs `pytest`. The CLI cannot assume any particular toolchain.

### Recommendation: Configure at Init, Store in Repo Config

**During `trls init`, the CLI detects and prompts for verification configuration.** This is stored in `.issues/config.json` on the ops branch, so all workers share the same hook definitions.

**Auto-detection at init time:**

The CLI checks for well-known files in the repository root and proposes defaults:

| Detected File | Proposed test_cmd | Proposed lint_cmd |
|---|---|---|
| package.json | `npm test` | `npx eslint {scope}` |
| go.mod | `go test ./...` | `golangci-lint run {scope}` |
| pyproject.toml | `pytest` | `ruff check {scope}` |
| Cargo.toml | `cargo test` | `cargo clippy -- {scope}` |
| Makefile (with test target) | `make test` | (none) |

The developer confirms or overrides. If nothing is detected, the CLI prompts for manual entry.

**Config format in `.issues/config.json`:**

```json
{
  "pre_transition_hooks": [
    {
      "cmd": "npm test -- --testPathPattern={scope}",
      "label": "tests",
      "required": true,
      "exit_codes": {
        "0": "pass",
        "1": "test_failure",
        "*": "environment_error"
      }
    },
    {
      "cmd": "npx tsc --noEmit",
      "label": "typecheck",
      "required": true
    },
    {
      "cmd": "npx eslint {scope}",
      "label": "lint",
      "required": false
    }
  ]
}
```

**`{scope}` interpolation:** Replaced at runtime with the task's `scope.modify` paths (space-separated for CLI args, or glob patterns depending on the tool). This is the bridge between the ops data model and the code verification tools.

**Required vs optional:** Required hooks block the transition — if they fail, the CLI refuses to emit the `done` op. Optional hooks report results but do not block. This distinction lets teams include aspirational checks (lint) without gating on them.

**Exit code semantics:** `0` = pass, `1` = test/lint failure (actionable), anything else = environment error (not actionable by the worker — dependency missing, build broken by another change, etc.). This distinction matters for AI agents: a test failure means "fix your code," an environment error means "report the issue and move on."

**Code worktree path:** The CLI does not need explicit configuration for the code worktree path. It discovers it by walking up from `cwd` to find the `.git` directory, which is the standard git behavior. The developer runs `trls transition` from within their code worktree (where they have been working). The CLI runs hooks in `cwd`, then switches to the ops worktree internally to record the transition. This matches the verify-then-record phase separation from the adversarial review.

**Single-branch mode:** In single-branch mode, there is no worktree distinction — hooks run in the same directory as ops. The config format is identical.

**Init flow:**

```
$ trls init
  Detected: package.json (Node.js)
  Proposed test command: npm test
  Proposed lint command: npx eslint {scope}
  Accept? [Y/n/edit]
  
  ✓ Config written to .issues/config.json
  ✓ Hooks installed to .git/hooks/
  ✓ Worker ID: a1b2c3d4
  ✓ Ops worktree created at .trellis/
```

---

## Gap 4: Offline Mode — Documentation

### Behavior

Workers without network access can continue appending ops to their local ops worktree. They cannot push. When network is restored, they push accumulated ops normally.

### What Happens on Reconnection

**Stale claims:** A worker claims a task while offline. Meanwhile, another (online) worker claims the same task. When the offline worker pushes, both claims land in the log. Normal timestamp-based resolution applies — first claim by timestamp wins. If the offline worker's clock was accurate, they win only if they claimed first. If they lose, they discover it on their next `trls sync` and their work may be wasted. This is the same behavior as the online race condition, just with a longer window.

**Stale done transitions:** A worker transitions a task to `done` while offline. The transition op sits in their local log. When they push, the `done` status propagates to all workers. The two-phase model handles this correctly — `done` does not unblock downstream work, only `merged` does. The code PR still needs to be submitted and merged. No correctness issue.

**Stale heartbeats:** A worker emits heartbeats offline but cannot push them. Other workers may see the claim as expired (no heartbeats within TTL) and reclaim the task. When the offline worker reconnects, their heartbeats land in the log but with old timestamps — they do not revive an expired claim. The reclaiming worker's claim takes precedence by normal timestamp ordering.

### Summary

Offline mode requires no special handling. The existing timestamp-based conflict resolution, two-phase completion, and heartbeat TTL mechanics all produce correct (if sometimes wasteful) outcomes. The only cost is potential duplicated effort if another worker reclaims a task during the offline period. This is acceptable and consistent with the design philosophy of advisory coordination over strict locking.

**Documentation requirement:** The architecture doc should include a paragraph in the Failure Modes section noting that offline operation is supported, with the caveat that claims may be lost during the offline window due to TTL expiry.

---

## Gap 5: Multi-Repo — Opened Discussion

### Personas and Their Repository Topology

**Persona 1 — Freelance solo developer.** Single repo, no branch protection, likely no team. Uses single-branch mode. Multi-repo is irrelevant — if they have multiple repos, they run separate trellis instances per repo. No coordination needed across repos because there is only one worker.

**Persona 2 — Enterprise solo developer.** Single repo, protected main branch, uses dual-branch mode. Same as persona 1 for multi-repo: separate trellis instances per repo. Branch protection drives the dual-branch model, but multi-repo coordination is not a concern because one developer does not face claim races with themselves.

**Persona 3 — Enterprise/freelance developer team.** This is the persona where multi-repo becomes a real question. Multiple workers (human and AI) collaborate on a project. If the project spans multiple repositories, work items in one repo may depend on work items in another.

### The Real Question

Multi-repo is not about "can trellis run in multiple repos?" — it already can. Each repo gets its own `trls init`, its own ops branch, its own DAG. The question is: **can a task in repo A declare a dependency on a task in repo B, such that the ready-task computation in repo A knows when the blocker in repo B is `merged`?**

### Options

**Option A: Separate instances, manual coordination.**

Each repo has its own independent trellis instance. Cross-repo dependencies are tracked by humans or by convention (e.g., a note on the task: "blocked by repo-B task-xyz — verify manually"). The CLI has no awareness of other repos.

Pros: No new architecture. Zero complexity. Works today.

Cons: Cross-repo blockers are not enforced. A developer might start work on repo-A task-01 without realizing repo-B task-02 (its blocker) is still in progress. For AI workers following the SKILL.md strictly, this is fine — they only see what `trls ready` shows, and cross-repo blockers would need to be modeled differently. But for humans, it is a process gap.

Best for: Teams where cross-repo dependencies are rare (fewer than 10% of tasks). Most monorepo-first organizations. Persona 1 and 2 exclusively. Persona 3 when using a monorepo.

**Option B: Ops hub repo.**

A dedicated repository (e.g., `acme/trellis-ops`) contains all trellis ops for the entire organization or project. No code lives here — it is purely a coordination repo. Individual code repos reference this hub for task context. Workers in any code repo push ops to the hub.

Implementation: `trls init --ops-repo=acme/trellis-ops` configures the ops worktree to point at the external hub repo instead of creating a local ops branch. All CLI commands operate against the hub. The code worktree is the current repo.

Pros: Single DAG across all repos. Cross-repo dependencies work natively. Ready-task computation, claim races, and merged detection all operate normally because ops are centralized.

Cons: Adds a repo dependency — every worker needs clone access to the hub repo. The hub repo is a single point of coordination (though still distributed via git). Merged detection becomes harder: the CLI needs to check main in the task's target code repo, not the hub repo. Task scope must include a repo identifier (e.g., `scope: {"repo": "acme/backend", "modify": ["src/auth.ts"]}`).

Best for: Multi-repo microservices architectures with frequent cross-repo dependencies. Enterprise teams with 3+ repos that share a project DAG.

**Option C: Federated ops with cross-repo links.**

Each repo has its own ops branch. A new link type (`cross-repo-blocked-by`) references a task in another repo by a qualified identifier: `repo:acme/backend#task-xyz`. The CLI resolves these links by cloning (or fetching) the remote repo's ops branch and checking the referenced task's status.

Pros: No central hub repo. Each repo is self-contained with optional cross-repo awareness.

Cons: Significant implementation complexity. The CLI needs to fetch remote ops branches, cache them, handle authentication across repos, and deal with staleness. Cross-repo merged detection requires checking main in a different repo. The failure modes multiply (remote repo unavailable, auth expired, inconsistent state between repos).

Best for: Loosely coupled teams that want optional cross-repo visibility without a centralized coordination point. I don't think this is a v1 concern.

### Recommendation: Option A for v1, Design for Option B

**v1: Ship with separate instances per repo (Option A).** This covers persona 1, persona 2, and persona 3 when using a monorepo or when cross-repo dependencies are rare. No architectural changes needed.

**Design the data model to be Option B-compatible.** This means:

1. Task scope should include an optional `repo` field from the start. In single-repo mode it is omitted (default: current repo). This is a schema addition, not a behavioral change.

2. The ops worktree path should be configurable (already recommended for other reasons), so that a future `--ops-repo` flag can point it at an external repo.

3. Task IDs should be globally unique (UUIDs — already the case). If two repos' trellis instances are later merged into a hub, there are no ID collisions.

4. The merged detection algorithm should not assume the code branch is in the same repo as the ops branch. Today it does (it runs `git log main` in the local repo). Making the code-repo a configurable per-task property is a small abstraction that pays off later.

**When to implement Option B:** When a paying customer or significant user cohort requests cross-repo coordination. The design-for signals above ensure it is a feature addition, not an architectural rewrite.

### Persona-Driven Feature Matrix

| Feature | Solo Freelance | Solo Enterprise | Team (Monorepo) | Team (Multi-Repo) |
|---|---|---|---|---|
| Branch mode | Single-branch | Dual-branch | Dual-branch | Dual-branch |
| Ops location | .issues/ on main | _trellis branch | _trellis branch | Hub repo (future) |
| Claim races | N/A (one worker) | N/A (one worker) | Full MRDT | Full MRDT |
| Two-phase completion | Optional (no PR gate) | Yes | Yes | Yes |
| Merge detection | Immediate (direct push) | Commit-message scan | Commit-message scan | Cross-repo scan (future) |
| Cross-repo deps | N/A | N/A | N/A (monorepo) | Manual (v1), Hub (future) |
| Hooks | Optional | Recommended | Recommended | Required (hub config) |
| Workers per repo | 1 | 1 | Many | Many across repos |

### Risks of the Recommendation

**Risk:** A team adopts trellis for a monorepo, then splits into multiple repos (common microservices evolution). Their DAG is now split across repos with broken cross-repo links.

**Mitigation:** Migration path from single-repo to hub-repo is straightforward: create the hub, copy the ops branch from the original repo, update worker configs. The data model does not change. Document this migration path even before implementing Option B.

**Risk:** Solo enterprise developers find the dual-branch model unnecessarily complex for their use case (one worker, no claim races to resolve).

**Mitigation:** The single-branch fallback already handles this if they do not have branch protection. If they do have branch protection, dual-branch is genuinely necessary even for solo developers — they cannot push ops to protected main. The complexity is inherent to the constraint, not the tool.