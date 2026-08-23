# Claim command decision seams

Citable spec for `ARCHIMP-S20`. Workers implement this document, not discarded
`ARCHIMP-S15` branches or out-of-repo handoffs.

The claim *command* file mixes three decision seams. Each seam is a pure
in-process plan; `cmd/armature` adapts facts and performs I/O (stderr, Op
appends, git, filesystem).

| Seam | Function | Package | Command I/O |
|---|---|---|---|
| Overlap | `PlanClaim` | `internal/claim` | stderr, note Ops, then Claim Op |
| Compensation | `PlanCompensation` | `internal/claim` | ownership reload, compensating Transition, exclude cleanup |
| Provision | `PlanProvision` | `internal/worktree` | git worktree, binding, exclude markers, branch-point writes, `--from` |

Keep `internal/claim` independent of `internal/materialize`. Add no writer port,
rule interface, or hypothetical adapter. Git and filesystem stay in `cmd/`.

---

## Glossary (workers add to `CONTEXT.md`)

**T1** adds:

**Claim Plan**: The overlap decision for a proposed Claim, produced before any
Op is appended. It names block reasons, warnings, and note intents. Distinct
from the Claim Op itself.
_Avoid_: Resolver, overlap check, claim result

**Claim Compensation**: The planned Transition that restores or releases a
Claim after a failed post-Claim step, produced before any compensating Op is
appended. Distinct from the Claim Op and from git rollback.
_Avoid_: Rollback, resolver, claim result

**T5** adds:

**Worktree Provision Plan**: The decision for a proposed Managed Worktree
destination, produced before any git or filesystem mutation. Outcomes are
refuse, adopt, already-at-destination, or provision-fresh. Distinct from
Adoption (one outcome) and from the Claim Op.
_Avoid_: Resolver, worktree create, checkout

---

## 1. Overlap — `PlanClaim`

### Problem

`arm claim` decides overlapping-scope Claims inline: block a foreign-Worker
overlap, dismiss a same-Worker overlap, or force-override. The command loop
ranges a Go map, returns on the first foreign overlap, can append a same-Worker
note then fail later, and ignores note-append errors.

### Interface

```go
type IssueFacts struct {
    Type      string
    Status    string
    ClaimedBy string
    Title     string
    Scope     []string
}

type PlanInput struct {
    TargetID    string
    TargetScope []string
    WorkerID    string
    Force       bool
    Issues      map[string]IssueFacts
    Graph       HierarchyGraph
    PriorOps    []ops.Op
}

type NoteIntent struct {
    IssueID string
    Message string
}

type ClaimPlan struct {
    BlockReasons []string
    Warnings     []string
    Notes        []NoteIntent
}

func PlanClaim(PlanInput) (ClaimPlan, error)
```

`HierarchyGraph` is the existing `Descendants` interface in this package.
Inputs are borrowed and read-only.

### Behavior

1. Empty `TargetID`, empty `WorkerID`, or a nil `Graph` is an input error.
   Never panic. A nil `Issues` map or nil `PriorOps` is a valid empty
   collection and yields an empty plan when the rest of the input is valid.
2. Sort candidate Issue IDs before evaluation. Map insertion order must not
   affect diagnostics or note order.
3. Ignore the target itself.
4. Only another Task in `claimed` or `in-progress` competes.
5. Overlap uses `ScopesOverlapEx` so ancestor and descendant pairs remain
   excluded in both directions.
6. Diagnostics name the conflicting Issue ID, title, type, status, and Claimed
   By. Empty Claimed By renders as `unknown`.
7. Exact same-Worker overlap is a dismissal, including when `Force` is set.
8. Deduplicate dismissal evidence by target Issue, conflicting Issue, and
   current Worker identity.
9. Without `Force`, return every sorted foreign-Worker block reason. A blocked
   plan is atomic: empty `Warnings` and `Notes`.
10. With `Force`, return a warning and two reciprocal note intents for every
    foreign-Worker overlap, in candidate-ID order. Same-Worker overlaps remain
    dismissals.
11. Canonical messages, preserved:
    - dismissal: `Serial claim: scope overlap with {id} (same worker, dismissed)`
    - force notes: `Scope overlap with {id} detected at claim time` on each Issue
    - reason: `scope overlap with {id} ({title}), a {type} {status} held by {holder}`

### Command adapter (T2)

Map Materialized State into `PlanInput` using Claimed By. Persist `Notes` with
`appendLowStakesOps` (errors returned). A blocked plan writes nothing: no Claim
Op, no worktree. Do not change lock ordering, worktree I/O, claim-token
rollback, or parent auto-advance.

### Tests

`package claim_test`, `_REQ_ARCHIMP_S20_T1`. Cover invalid input, nil
collections, competitor filters, ancestor exclusion, dismissal/dedup, unknown
Claimed By, collect-all blocks, mixed atomicity, force notes, same-Worker under
force, map-order invariance, input immutability.

T2: thin Cobra tests for adaptation, stderr labels, Op targets/order,
evidence-write failure, no worktree or Claim Op on block (`_REQ_ARCHIMP_S20_T2`).

---

## 2. Compensation — `PlanCompensation`

### Problem

After a won Claim, worktree setup can fail. The command then either restores
the prior live same-Worker lease or releases to `open`. That restore-vs-release
kernel is tested only through git-failure Cobra paths. Ownership reload is I/O
and stays in `cmd/`.

### Interface

```go
type LeaseFacts struct {
    Status                 string
    ClaimedBy              string
    ClaimedAt              int64
    LastHeartbeat          int64
    ClaimTTL               int
    ClaimingWorkerActivity int64
    WorktreePath           string
    ClaimToken             string
}

type CompensationInput struct {
    Prior      LeaseFacts
    WorkerID   string
    Now        int64
    ClaimToken string // IfClaimToken of the Claim this compensates
}

func PlanCompensation(CompensationInput) (ops.Payload, error)
```

Inputs borrowed and read-only. Empty `WorkerID` or empty `ClaimToken` is an
input error. Never panic. The command calls this only after it has confirmed
the Claim still holds; this function does not return a skip.

### Behavior

Reuse `IsClaimStale` on the prior lease.

- Live same-Worker (`Prior.ClaimedBy == WorkerID` and not stale): `To` is the
  prior status; `RestoreClaim` true; copy all restore-lease fields including
  `RestoreClaimToken` from prior.
- Stale same-Worker or different Claimed By: `To` is `open`; `RestoreClaim`
  true; leave restore-lease fields zero (clears the lease).
- Always set `IfClaimToken` to `ClaimToken`.
- If `Prior.WorktreePath` is non-empty, set `Payload.WorktreePath` to it;
  otherwise set `ClearWorktreePath`.

### Command adapter (T4)

`rollbackClaim` keeps store reload / `ClaimHeldBy` / exclude cleanup /
`appendHighStakesOp`. When still owned, build the compensating Op from
`PlanCompensation`. Do not change lock or exclude behavior.

### Tests

T3 `package claim_test`, `_REQ_ARCHIMP_S20_T3`: restore live same-Worker,
release stale, release foreign, empty path clears, non-empty path restores,
`IfClaimToken` set, invalid input errors, inputs unchanged.

T4: existing rollback Cobra tests stay green; thin `_REQ_ARCHIMP_S20_T4` that
restore-vs-release still holds on the worktree-failure path.

---

## 3. Provision — `PlanProvision`

### Problem

Destination legality, Adoption, Ambiguous Binding, and provenance-required
refuse live inside `createWorktreeAndBranch*`. Git I/O belongs in `cmd/`. The
decision belongs in `internal/worktree` next to inventory and reconcile.

### Interface

```go
type InventoryRow struct {
    Path    string
    Branch  string
    Binding string
}

type ProvisionInput struct {
    IssueID        string
    Dest           string
    ExpectedBranch string
    Inventory      []InventoryRow
    DestExists     bool
    InRepo         bool
    UnderCanonical bool
    NestedUnder    string // registered path if dest is nested, else empty
    ProvenanceOK   bool   // trusted branch-point on the adopt candidate
}

type ProvisionAction string

const (
    ProvisionRefuse       ProvisionAction = "refuse"
    ProvisionAdopt        ProvisionAction = "adopt"
    ProvisionAlreadyAtDest ProvisionAction = "already_at_dest"
    ProvisionFresh        ProvisionAction = "fresh"
)

type ProvisionPlan struct {
    Action       ProvisionAction
    AdoptFrom    string
    RefuseReason string
}

func PlanProvision(ProvisionInput) (ProvisionPlan, error)
```

Inputs borrowed and read-only. Empty `IssueID`, `Dest`, or `ExpectedBranch` is
an input error. Never panic. No git. `internal/worktree` does not import
`issueid`, `adapters`, `deliverygate`, or `cmd`. Issue-ID validation stays in
`cmd/` before this is called.

### Behavior

Evaluate in this order. Inventory order must not affect the result; sort bound
paths when naming them.

1. If `NestedUnder` is non-empty: refuse. Preserve phrasing:
   `custom worktree destination %s is nested inside registered worktree %s`.
2. If `InRepo && !UnderCanonical`: refuse. Preserve phrasing:
   `custom worktree destination %s is inside the repository; explicit destinations must be outside the repository or under canonical .worktrees`.
3. Collect inventory rows whose `Binding` equals `IssueID`.
4. If more than one: refuse Ambiguous Binding. Preserve phrasing:
   `issue %s is bound to %d worktrees (%s); remove the armature-issue-id binding from the ones you do not want before claiming`
   with bound paths sorted.
5. If exactly one bound row:
   - If its path is the dest: `already_at_dest`.
   - If its `Branch` is not `refs/heads/`+`ExpectedBranch` (empty branch means
     detached): refuse. Preserve phrasing:
     `worktree at %s is bound to %s but is on %s, not %s; finish or abandon the in-progress git operation there and check out %s before claiming`.
   - If `!ProvenanceOK`: refuse. Preserve phrasing:
     `adopted worktree has no recorded branch-point provenance; re-claim it from a managed worktree or use --skip-delivery-gate only with an explicit override`.
   - Otherwise: `adopt` with `AdoptFrom` set to that path.
6. Otherwise: `fresh`.

`--from` tip/branch revalidation, `stillOwns` before destructive cleanup,
exclude markers, binding writes, and branch-point writes are cmd I/O, not this
plan. `merged.go` keeps using existing cmd helpers; this seam does not move
them.

### Command adapter (T6)

Gather inventory and dest facts, call `PlanProvision`, then existing git:
refuse returns the reason; adopt moves; already-at-dest rebinds; fresh
detaches and checks out. Do not change Claim Op ordering relative to
provision (still Claim first, then provision, then rollback on failure).

### Tests

T5 `package worktree_test`, `_REQ_ARCHIMP_S20_T5`: each action above, sorted
ambiguous paths, inventory-order invariance, invalid input, inputs unchanged.

T6: existing adoption/destination Cobra tests stay green; thin
`_REQ_ARCHIMP_S20_T6` that refuse/adopt/fresh still produce the same
user-visible errors. Do not duplicate T5 tables in Cobra.

---

## Split

| Task | What | Blocked by |
|---|---|---|
| T1 | `PlanClaim` + tests; `CONTEXT.md` Claim Plan and Claim Compensation | — |
| T2 | Overlap command adapter | T1, `LNGHZN-S6-T2` |
| T3 | `PlanCompensation` + tests (parallel with T1) | — |
| T4 | `rollbackClaim` calls `PlanCompensation` | T2, T3 |
| T5 | `PlanProvision` + tests; `CONTEXT.md` Worktree Provision Plan | T1 |
| T6 | Command calls `PlanProvision`, then existing git | T2, T4, T5 |

T2 does not wire compensation or provision. T4 does not extract provision.
T6 does not edit `merged.go`.

---

## Out of scope

- Recreating `ARCHIMP-S15` IDs or citing discarded branches
- Importing `internal/materialize` into `internal/claim`
- Widening `claim-boundary` or `worktree-boundary` for this story
- Putting git inside `internal/worktree` or `internal/claim`
- Splitting staleness, heartbeat, or race-winner out of `internal/claim`
- Promotion redesign
- Moving exclude-marker or parent-branch teardown helpers into `merged.go`'s
  files

---

## How `ARCHIMP` closes

`ARCHIMP-S15` stays `cancelled`. Rollup promotes a parent only when every
child is `merged`. Cancelled children count as unmerged. `ARCHIMP` stays open
until an explicit later human transition — not a side effect of this story.
