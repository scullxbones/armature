# Trellis Roles

Trellis supports four roles. A single human or agent may occupy multiple roles across a session (e.g., a solo developer is simultaneously Planner, Coordinator, and Worker). On larger teams, roles are typically held by different actors.

Roles are orthogonal to **deployment topology** (solo/team, single-branch/dual-branch). The same four roles apply regardless of topology, team size, or technology stack.

Each role has a corresponding skill that provides a workflow-oriented guide from that role's perspective. The `trls` command reference skill is a shared reference card usable by all roles.

---

## Planner

**Purpose:** Translates objectives into a well-structured DAG of actionable work.

**Key responsibilities:**
- Create epics, stories, and tasks with complete `dod`, `scope`, and `acceptance` fields
- Decompose stories using `trls decompose-apply` with properly formed plan JSON
- Register source documents and link every created issue at creation time (not as a remediation pass)
- Set priorities and `blocked_by` dependencies before workers start
- Promote issues from `draft` to visible using `trls dag-transition`
- Validate the DAG is structurally sound before releasing work (`trls doctor`, `trls validate`)

**Key commands:** `create`, `decompose-apply`, `decompose-apply --dry-run`, `dag-transition`, `sources add`, `sources sync`, `source-link`, `accept-citation`, `validate`, `link`, `doctor`

**Skill:** `trls-planner` _(to be written — highest-priority gap)_

**Common failure modes:**
- Tasks missing `dod`, `scope`, or `acceptance` — workers cannot self-verify completion
- Issues created without source links — citation debt accumulates silently
- Scope overlaps not resolved with `trls link` before work begins — workers collide
- Draft issues not promoted — workers see an empty ready queue

---

## Coordinator

**Purpose:** Manages execution flow — finds ready work, assembles context, dispatches workers, and integrates completed work.

**Key responsibilities:**
- Identify unblocked work with `trls ready`
- Pre-assign tasks to specific workers using `trls assign` when needed (parallel wave isolation, specialist routing)
- Claim issues and assemble task context with `trls render-context`; deliver context to each worker at dispatch time
- Monitor in-flight work; resolve or escalate blocked issues
- After workers complete: verify build integrity, integrate branches, resolve conflicts
- Run `trls validate` once all story tasks are done; open the pull request

**Key commands:** `ready`, `assign`, `unassign`, `claim`, `render-context`, `list`, `validate`, `transition` (story-level), `doctor`

**Skill:** `trls-coordinator` _(to be written — currently embedded in `trls-worker`)_

**Common failure modes:**
- Dispatching parallel agents without unique log slot assignments — agents share one log and per-agent attribution is lost
- Skipping build and integration verification before merging parallel branches
- Transitioning a story while uncited issues remain — `trls validate` will error
- Forgetting to commit op log changes generated between task commits before pushing

---

## Worker

**Purpose:** Executes a single claimed task end-to-end: implements, verifies, cites, and transitions.

**Key responsibilities:**
- Register identity once per clone with `trls worker-init`
- Receive task context from the Coordinator at dispatch time — do not re-derive context independently
- Implement the work satisfying the task's `acceptance` criteria
- Record progress with `trls note` and design decisions with `trls decision`
- Issue `trls heartbeat` periodically on long-running tasks to prevent claim expiry
- Cite every issue touched before completing (`trls source-link` or `trls accept-citation --ci`)
- Transition to `done` with a concrete `--outcome` and commit all changes

**Key commands:** `worker-init`, `note`, `decision`, `heartbeat`, `source-link`, `accept-citation`, `transition` (task-level)

**Skill:** `trls-worker`

**Common failure modes:**
- Leaving issues uncited before returning — citation debt and validate errors
- Skipping heartbeat on long tasks — claim expires, another worker can take it
- Vague outcome on `transition --to done` — Auditor cannot verify completion against acceptance criteria
- Failing to commit op log changes alongside implementation changes

---

## Auditor

**Purpose:** Verifies that completed work is honest and traceable — every issue has a valid cited source, every decision is recorded, and outcomes satisfy acceptance criteria.

**Key responsibilities:**
- Run `trls validate` to check citation coverage and source UUID integrity (E7, E8)
- Run `trls sources verify` to confirm all registered sources are fingerprinted and current
- Inspect `trls render-context` on completed issues to compare `outcome` against `acceptance` criteria
- Verify scope overlap warnings are resolved with `trls link` dependencies (not left as unresolved warnings)
- Flag vague outcomes and missing definitions of done before story sign-off
- Run `trls doctor --strict` as a final repo health check before merge

**Key commands:** `validate`, `sources verify`, `sources sync`, `render-context`, `doctor --strict`, `list --status done`

**Skill:** `trls-auditor` _(to be written)_

**Common failure modes:**
- Trusting `trls doctor` D6 alone — it checks field presence only, not source UUID validity; always run `trls validate` for full citation integrity
- Accepting vague outcomes ("done", "fixed") without cross-checking against `acceptance` criteria in `render-context`
- Skipping `trls sources verify` — source fingerprints can go stale if documents are updated after initial registration

---

## Role-to-Command Quick Reference

| Command | Planner | Coordinator | Worker | Auditor |
|---|:---:|:---:|:---:|:---:|
| `create` | ✓ | | | |
| `decompose-apply` | ✓ | | | |
| `dag-transition` | ✓ | | | |
| `sources add/sync` | ✓ | | | ✓ |
| `source-link` / `accept-citation` | ✓ | | ✓ | |
| `link` | ✓ | ✓ | | |
| `ready` | | ✓ | | |
| `assign` / `unassign` | | ✓ | | |
| `claim` | | ✓ | | |
| `render-context` | | ✓ | | ✓ |
| `worker-init` | | | ✓ | |
| `note` / `decision` | | | ✓ | |
| `heartbeat` | | | ✓ | |
| `transition` (task) | | | ✓ | |
| `transition` (story) | | ✓ | | |
| `doctor` | ✓ | ✓ | | ✓ |
| `validate` | ✓ | ✓ | | ✓ |
| `sources verify` | | | | ✓ |
| `list` | ✓ | ✓ | | ✓ |

---

## Skill Coverage Gap (as of 2026-04-13)

| Role | Skill status |
|---|---|
| Worker | `trls-worker` — complete, actively used |
| Coordinator | Embedded in `trls-worker` — needs extraction into `trls-coordinator` |
| Planner | No skill exists — highest-priority gap; task creation guidance is absent |
| Auditor | No skill exists — second priority; `trls validate` usage is under-documented |
