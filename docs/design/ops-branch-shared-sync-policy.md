# Shared `_armature` Sync Policy (`opsync`)

**Status:** Design sketch only. Do not implement from this document until a
task with a Definition of Done cites it. No code in this change set.

**Date:** 2026-09-12

**Candidate:** Arena Candidate B — one policy object owns push, reject-rebase,
and lag. Distinct from “wire `Push` at `low_stakes_push_threshold`.”

---

## 1. Problem

Armature’s coordination truth lives on the `_armature` branch (I1). Clones
only see each other’s ops after that branch is published. Today that publish
path is split, incomplete, and quiet.

### What the code does (2026-09-12 `main`)

| Path | After local commit | On non-fast-forward | On remaining failure |
| --- | --- | --- | --- |
| High-stakes (`appendHighStakesOpIf` in `cmd/armature/helpers.go`) | `Push("_armature")` | one `FetchAndRebase` then a second `Push` with **errors ignored** | command succeeds; other clones never see the op |
| Low-stakes (`appendLowStakesOps`) | increment `pending-push-count`; **reset at threshold without pushing** | n/a | n/a |
| `arm push-ops` | `Push` only | **no rebase retry** | Command Failure `PUSH-OPS-1` |
| Read verbs (`ready`, `list`, `render-context`, …) | no fetch | n/a | clone can be arbitrarily behind origin |
| `arm doctor` | no ahead/behind probe | n/a | silent lag |

`docs/configuration.md` already admits the config lie: `low_stakes_push_threshold`
“does not push `_armature`.” Heartbeats and notes can remain local forever if
the worker never hits a high-stakes verb. High-stakes verbs can also remain
local: the retry lives inline in `cmd/` and both push errors are discarded.

`docs/design/architecture.md` (§ process flow) still describes `git pull` on
every command and a ~5-iteration push retry. That is **not** the
implementation. This sketch does **not** adopt fetch-before-every-read; that
path is out of scope (too invasive for materialize/list latency and depguard
surface).

Related but not this design: `arm sync` / `internal/sync` detect **code-branch
merges** and write `merged` transitions. Unrelated to publishing `_armature`.
G2.2 in `docs/design/narrow-gaps-addendum.md` (reserved doctor **D11**,
`TOPTIER-S12-T2`) is disaster-recovery / missing upstream, not lag counts.

### Failure modes this sketch owns

1. **Low-stakes never publish** — notes, heartbeats, decisions stay on one
   clone.
2. **Sync failures are silent** — high-stakes swallows git errors; workers
   report success with unpublished claims.
3. **Clones fall behind with no warning** — doctor does not report
   ahead/behind; reads do not fetch (and will not after this design).

---

## 2. Design choice in one sentence

Introduce a small **deep module** `internal/opsync` whose `Branch` type is the
only composition of “push `_armature`, on reject fetch-rebase and push again,
optionally report lag.” High-stakes and low-stakes **both** call
`AfterCommit` after a successful ops commit. Doctor calls `Lag` only. Callers
never mention `Push` or `FetchAndRebase`.

### Error policy (normative for this candidate)

| Caller | Network / reject after retries | Local commit |
| --- | --- | --- |
| **High-stakes** `AfterCommit(ctx, High)` | **Return the error** to the CLI port. Map to the **existing command** Failure Code (`CLAIM-1`, `TRANSITION-1`, …), cause naming publish failure, Next Actions including `arm push-ops` and `arm doctor`. Do **not** invent an `arm opsync` verb (ADR 0011: no CLI group without a user-facing module; `arm sync` already means merge-detect). | **Kept.** I2: do not rewrite or delete the op. |
| **Low-stakes** `AfterCommit(ctx, Low)` | **Best-effort.** `AfterCommit` returns `nil`. Persist a worker-local last-attempt record (gitignored `state/`, never committed to `_armature`). Doctor lag/check reads it. Optional one-line stderr warning; not a Command Failure. | **Kept.** |
| **`arm push-ops`** | `Publish` with **Must** semantics (no coalesce, return error) → existing `PUSH-OPS-1`. | n/a (no new op) |
| **`Lag` (doctor)** | Probe errors become a **doctor finding** (successful `arm doctor` report). Not a Command Failure (ADR 0020 §7). | n/a |

**Why split High vs Low:** a failed claim/transition publish is a
cross-clone coordination hazard (double-claim, stale ready set). A failed
heartbeat publish is noise that must not eject a worker from its loop. Both
still **attempt** the same publish algorithm. The split is surfacing, not a
second git strategy.

**Why not roll back the op on High failure:** rolling back JSONL or
`git reset` on `_armature` is T2 / I2. The recovery command is `arm push-ops`,
which is the same `Publish` path.

---

## 3. Usage

### 3.1 After an ops commit (CLI helpers)

Today:

```text
appendHighStakesOpIf → AppendAndCommitIf → Push; if err { FetchAndRebase; Push }
appendLowStakesOps   → AppendAndCommit → Increment; if n >= threshold { Reset }
```

Target:

```text
appendHighStakesOpIf → AppendAndCommitIf → opsync.AfterCommit(ctx, High)
appendLowStakesOps   → AppendAndCommit    → opsync.AfterCommit(ctx, Low)
```

`cmd/armature/helpers.go` stops importing git push verbs. Construction:

```go
sess := opsync.New(opsync.Config{
    Branch:    "_armature", // literal; not configurable in v1 (ADR 0006)
    Remote:    "origin",    // matches adapters.Client today
    Transport: gc,          // *adapters.Client or test fake
    Store:     opsync.FileAttemptStore{Dir: ctx.StateDir},
    Retry:     opsync.DefaultRetry, // cap 5, same number architecture.md already claims
})
```

Nil transport (no ops worktree) → `AfterCommit` is a no-op success. That
replaces `NoTracker` without preserving single-branch as a product mode
(already eliminated).

### 3.2 Explicit publish

`arm push-ops` becomes:

```go
return sess.Publish(ctx) // Must, ignores coalesce (there is none in v1)
```

Post-commit hook remains `arm push-ops 2>/dev/null || true`. Hook-level
best-effort is **outside** this module. Agent-facing `arm push-ops` stays
loud.

### 3.3 Doctor lag probe only

```go
rep, err := sess.Lag(ctx)
// doctor maps LagError / Report into a new check (see §8). No Publish. No rebase.
```

`Lag` may `git fetch origin _armature` (update remote-tracking ref only). It
must **not** rebase or move local `_armature`. Doctor is an explicit
diagnostic, not a read verb.

### 3.4 What callers must not do

- `ready` / `list` / `render-context` / `validate` / materialize **must not**
  call `Publish` or `Lag`. No fetch-before-every-read.
- `internal/ops` **must not** import `opsync`. Append+commit stays a log
  concern; publish stays a branch concern.
- Tests of claim/transition **must not** re-specify rebase retry; they fake
  `opsync.Branch` or inject a fake `Transport`.

### 3.5 Config field `low_stakes_push_threshold`

**v1 of this candidate retires it as a publish trigger.** After both stakes
call `AfterCommit`, every successful commit attempts publish. The field
today does not push; wiring it would be Candidate A.

Disposition (pick one at implementation tasking; sketch preference is A):

- **A (preferred):** keep the JSON key for one release so D10 does not
  churn adopters; document “unused, ignored by `opsync`”; doctor D10 stays
  `> 0` only. Delete in a follow-up subtractive task.
- **B:** delete the key in the same change (breaking strict decode / D10).
- **C (rejected as the primary shape):** use the integer as coalesce-N
  inside `AfterCommit(Low)`. That re-creates “push at threshold” behind a
  new name. Allowed later as an **optional** `Config.CoalesceLow` defaulting
  to 1 (always publish), not as the headline.

---

## 4. Types and signatures

Package: `github.com/scullxbones/armature/internal/opsync`

Name **`opsync`**, not `sync`: `internal/sync` is merge detection.

### 4.1 Transport (git port)

Narrower than `adapters.Client`. Production adapter is a thin wrapper around
the existing client. Tests fake this only.

```go
package opsync

// Transport is the git surface opsync needs. Callers do not use it.
type Transport interface {
    Push(branch string) error
    FetchAndRebase(branch string) error
    // FetchTracking updates origin/<branch> without moving local HEAD.
    FetchTracking(remote, branch string) error
    // AheadBehind is git rev-list --left-right --count A...B
    // (local...tracking). Missing tracking ref → error.
    AheadBehind(local, tracking string) (ahead, behind int, err error)
}
```

`adapters.Client` today has `Push` and `FetchAndRebase` only. Implementation
adds `FetchTracking` and `AheadBehind` on the client **or** a small adapter
type in `opsync` that shells via the client’s repo path. Prefer methods on
`adapters.Client` if they stay dumb one-liners; policy stays in `opsync`.

### 4.2 Attempt store (local diagnostic)

```go
type Attempt struct {
    Unix      int64  `json:"unix"`
    OK        bool   `json:"ok"`
    Stakes    string `json:"stakes"` // "high" | "low" | "must"
    Err       string `json:"error,omitempty"`
    Ahead     int    `json:"ahead,omitempty"`
    Behind    int    `json:"behind,omitempty"`
}

type AttemptStore interface {
    Save(Attempt) error
    Last() (Attempt, bool, error)
}

type FileAttemptStore struct{ Dir string } // writes Dir/opsync-last-attempt.json
```

Worker-local, same class as `pending-push-count` (which this module
**replaces**). Gitignored via existing `.armature` state gitignore. Not an
op. Not origin SoT.

### 4.3 Session / Branch (the deep module)

```go
type Stakes uint8

const (
    High Stakes = iota
    Low
)

type Config struct {
    Branch    string // must be "_armature" in v1; empty → default
    Remote    string // default "origin"
    Transport Transport
    Store     AttemptStore // optional; nil skips persistence
    Retry     Retry        // zero value → DefaultRetry
}

type Retry struct {
    MaxAttempts int // default 5: initial Push + up to 4 reject-rebase cycles
}

var DefaultRetry = Retry{MaxAttempts: 5}

type Branch struct { /* unexported fields from Config */ }

func New(cfg Config) *Branch

// AfterCommit runs Publish. High returns publish errors; Low records and returns nil.
func (b *Branch) AfterCommit(ctx context.Context, s Stakes) error

// Publish is the shared algorithm: push; on reject, fetch-rebase and push;
// repeat until success or Retry.MaxAttempts. Always records Attempt when Store ≠ nil.
func (b *Branch) Publish(ctx context.Context) error

// Lag fetches tracking (not rebase) and returns ahead/behind vs origin/_armature.
func (b *Branch) Lag(ctx context.Context) (Report, error)

type Report struct {
    Branch  string
    Remote  string
    Ahead   int // local commits not on origin
    Behind  int // origin commits not in local _armature
    TrackingMissing bool
    Last    Attempt // from Store, zero if none
}
```

### 4.4 Publish algorithm (hidden)

```text
if Transport == nil: return nil
attempt = 0
loop:
  attempt++
  err = Push(branch)
  if err == nil: Save(ok); return nil
  if attempt >= MaxAttempts: Save(fail); return err
  if !rejectedNonFF(err) && !rebaseWorthRetry(err): Save(fail); return err
  rbErr = FetchAndRebase(branch)
  if rbErr != nil: Save(fail wrapping both); return rbErr
  continue
```

Classify reject vs auth/network: substring/exit on “non-fast-forward” /
“fetch first” vs everything else. Auth and missing remote **do not** rebase.
Context cancel aborts between attempts.

`FetchAndRebase` remains the existing client behavior (fetch origin, rebase
onto `origin/_armature`). I3: rebase succeeds when each worker only appends
its own log file. Conflict → error (dirty scaffolding, hook debris — already
a known bootstrap hazard). No `reset --hard`.

### 4.5 Sentinel errors (domain, not Command Failures)

```go
var (
    ErrNoTransport = errors.New("opsync: no git transport")
    ErrTracking    = errors.New("opsync: missing remote-tracking ref")
)

type PublishError struct { Attempts int; Last error }
func (e *PublishError) Unwrap() error
func (e *PublishError) Error() string
```

`cmd/` maps `*PublishError` into the **caller’s** Failure Code. `opsync`
does not import `internal/errors` or know `CLAIM-1` (ADR 0020: deep modules
return ordinary errors).

---

## 5. Module map

```text
cmd/armature/helpers.go     AfterCommit High|Low after AppendAndCommit*
cmd/armature/push_ops.go    Publish (Must)
cmd/armature/doctor.go      Lag → new doctor check (reservation D12)
internal/opsync             Branch, Transport, AttemptStore, algorithm
internal/adapters           Push, FetchAndRebase, + FetchTracking, AheadBehind
internal/ops                UNCHANGED append/commit; delete PendingPushTracker
internal/ops/tracker.go     DELETE (FilePushTracker / NoTracker)
internal/doctor             finding mapper only; no git verbs
internal/sync               UNCHANGED (merge detect)
```

### Depguard (ADR 0004)

`opsync` is **port-clean**: allow `$gostd` + `internal/adapters` only.
Do **not** fold publish into `internal/ops` (ops allow list would grow a
policy it should not own; GitCommitter stays commit-only).

Do **not** add an `opsync` CLI group. ADR 0011: hyphenated `push-ops` stays
the user-facing publish verb.

`cmd/` may import `opsync`. `claim` / `materialize` / `validate` must not.

### Call graph (target)

```text
claim/transition/assign/unassign/doctor --fix
  → appendHighStakesOpIf → ops.AppendAndCommitIf → opsync.AfterCommit(High)
note/heartbeat/decision
  → appendLowStakesOps → ops.AppendAndCommit → opsync.AfterCommit(Low)
arm push-ops → opsync.Publish
arm doctor  → opsync.Lag          (no AfterCommit)
arm ready   → (no opsync)
```

### Files expected at implementation (not in this PR)

| File | Role |
| --- | --- |
| `internal/opsync/branch.go` | `New`, `AfterCommit`, `Publish`, `Lag` |
| `internal/opsync/transport.go` | interface + reject classification |
| `internal/opsync/attempt.go` | `FileAttemptStore` |
| `internal/opsync/branch_test.go` | fake Transport; table tests |
| `internal/adapters/git.go` | `FetchTracking`, `AheadBehind` |
| `cmd/armature/helpers.go` | replace inline Push/Rebase and tracker |
| `internal/doctor/opsync_lag.go` | D12 mapper |
| `docs/configuration.md` | stop claiming threshold pushes; document ignore/delete |
| `docs/error-contract.md` | Next Actions examples only; no new prefix required |
| `docs/design/doctor-check-ids.md` | reservation row D12 until wired |

---

## 6. Rejected alternatives

| Alternative | Why rejected |
| --- | --- |
| **Candidate A: `if n >= threshold { gc.Push }`** | Does not fix silent high-stakes errors, does not give doctor lag, duplicates retry in `cmd/` (or leaves it there). Threshold is already a documented no-op; “make the counter honest” is a one-line lie-fix, not a module. |
| **Fetch before every read** | Architecture.md fantasy. Touches every command, materialize latency, offline use, and sandbox network. Explicitly out of scope. Lag on doctor + publish after write is the warning channel. |
| **Method soup on `adapters.Client` (`SyncOpsBranch`)** | God client. Retry, attempt persistence, High/Low surfacing, and doctor reports are policy, not git. Tests would need a real repo for policy. |
| **Put policy in `internal/ops`** | Ops is MRDT JSONL + commit. Mixing publish would force every AppendAndCommit test to care about remotes and would violate ops-boundary unless the allow list grows. |
| **Daemon / watch loop pushing `_armature`** | Tripwire **T1**. |
| **`git reset --hard` on reject** | Tripwire **T2** / I2. |
| **Bidirectional sync with a non-git mutable store** | Tripwire **T3**. |
| **Fail low-stakes commands on publish error** | Heartbeat in a flaky sandbox becomes a crash loop; claims still need Low to succeed locally. Doctor + last-attempt is the loud path for Low. |
| **Succeed high-stakes on publish error (status quo)** | The bug. Other clones race the unpublished claim. |
| **New Failure Code family `OPSYNC-1` plus `arm opsync`** | No user-facing group. Collides conceptually with `arm sync`. Port mapping stays on the verb that ran. |
| **Coalesce-N as the headline** | That *is* Candidate A with an extra type. Coalesce may later be `Config.CoalesceLow` default 1. |
| **Reuse reserved D11** | D11 is G2.2 backup / missing upstream (`TOPTIER-S12-T2`). Lag is a different finding. Allocate **D12**. |
| **Commit last-attempt JSON to `_armature`** | Cross-clone noise, extra push, and I3 file-ownership questions. Diagnostic is clone-local. |
| **Push from the post-commit hook only** | Hook is `|| true` and stderr-discarded. Agents never see it. Cannot be the high-stakes contract. |

---

## 7. Red-flag screen (constitution)

Cite, do not re-argue.

| Check | Result |
| --- | --- |
| **I1 Git-native** | Publish is `git push` of `_armature`. Attempt file is local cache, like checkpoint, not SoT. |
| **I2 Append-only** | Failed publish does not delete ops or rewrite history. Rebase of **own** unpublished commits onto origin is the existing reject path, not history rewrite of origin. |
| **I3 Merge-conflict-free** | Unchanged: one worker, one log file. Rebase conflicts remain exceptional (dirty scaffolding). |
| **I4 Agents first** | High-stakes returns a Command Failure with `arm push-ops` / `arm doctor` Next Actions. Low-stakes does not spam heartbeat. Doctor check is agent-readable payload. |
| **I5 Deterministic gates** | Doctor lag is a check, not an LLM merge decision. |
| **I6 `done` ≠ `merged`** | Untouched. `arm sync` (merge detect) unchanged. |
| **I7 Humans accountable** | Offline / no-remote remains possible; doctor says so. |
| **N1 not CI** | No GitHub Action, no required status on `_armature`. |
| **N4 not merge authority** | Does not protect or merge the ops branch. Architecture already: ops branch stays unprotected for low-latency coordination. |
| **T1 no daemon** | Sync runs in-process at commit/doctor/push-ops. |
| **T2 no history rewrite** | No force-push, no reset on `_armature`. |
| **T3 no bidirectional mutable remote** | Fetch from `origin` is one-way git. |

### Residual risks (acceptable if documented)

- **Reads stay stale** until a write-publish or doctor fetch. Workers that
  only `arm list` can miss remote claims. Mitigation: skills already tell
  workers to `claim` / `heartbeat`; both will publish. Optional later:
  document `arm push-ops` after clone; still no per-read fetch.
- **High-stakes command failure after local claim** can confuse a naive
  retry (`already claimed` by self). Next Actions must say `arm push-ops`
  first, not `arm claim` again. Implementation task should add a helpers
  test that the mapped cause mentions unpublished ops.
- **Retry storm:** two clones rejecting each other. I3 makes this rare;
  cap 5 plus classify non-FF vs auth.
- **`FetchAndRebase` dirty tree** (bootstrap comments): still fatal; not
  papered over with reset.

---

## 8. Proof plan (when an implementation task exists)

TDD (`docs/agents/quality-gates.md`). No implementation in this sketch.

### 8.1 Fake-transport unit tests (`internal/opsync`)

| Case | Expect |
| --- | --- |
| `Publish` success on first `Push` | one Push, zero FetchAndRebase, `Attempt.OK` |
| `Push` non-FF then rebase ok then Push ok | Push, FetchAndRebase, Push; success |
| `Push` non-FF forever | `MaxAttempts` Pushes, `MaxAttempts-1` rebases, `*PublishError` |
| `Push` auth failure | no rebase; error immediately |
| `AfterCommit(High)` on `*PublishError` | same error returned |
| `AfterCommit(Low)` on `*PublishError` | `err == nil`, `Last().OK == false` |
| `Lag` | `FetchTracking` only; **no** `Push`, **no** `FetchAndRebase` |
| `Lag` missing tracking | `TrackingMissing` or `ErrTracking`; doctor maps to warn |
| `Transport == nil` | `AfterCommit` nil |
| `ctx` cancelled between attempts | stop; error |

Do **not** require `_REQ_` names until a tasked issue ID exists.

### 8.2 Adapter tests (`internal/adapters`)

Throwaway git repos (existing `git_test.go` style): `AheadBehind` 1/0 after
a local commit not pushed; `FetchTracking` updates `origin/_armature`
without moving worktree HEAD.

### 8.3 CLI / doctor

- High-stakes integration: origin rejects first push (second clone advanced);
  rebase+push lands; command exit 0.
- High-stakes: no remote → non-zero, claim op still in local log, JSON
  error is `CLAIM-1` (or the verb under test) not success envelope.
- Low-stakes: no remote → `arm note` exit 0; doctor D12 not OK.
- `arm push-ops` uses `Publish` (rebase on reject) — regression vs today’s
  Push-only.
- Doctor D12: behind ≥ 1 after fetch of a remote with extra commits; ahead
  ≥ 1 with local unpublished commit.
- `ready` / `list` test spies: Transport methods **not** called.

### 8.4 Tracker removal

Delete `PendingPushTracker` tests or replace with AttemptStore tests.
Config tests: threshold still decodes (disposition A) or strict-decode
rejects unknown/removed key (disposition B). D10 corpus fixtures updated
only if B.

### 8.5 Gates

`make check-fast` during implementation; `make check` at task head. New
package must meet internal coverage. Mutation: retry classifier and
High/Low surfacing are the interesting mutants.

### 8.6 Manual / isolated CLI

Use `.agents/skills/verify-armature` against a throwaway repo: two clones,
A notes (Low) → B `doctor` / `list` sees the note only after publish; A
claim with origin down → loud failure + local op + `push-ops` after origin
returns.

### 8.7 Explicitly not proof

- Fetch on `arm ready`.
- Force-push recovery (G2 / D11).
- Making architecture.md’s “pull every command” true.

---

## 9. Doctor check reservation (D12)

Not implemented here. When tasked:

| ID | Planned |
| --- | --- |
| **D12** | Ops-branch lag: `opsync.Lag`. Warn if `behind > 0` (clone stale), `ahead > 0` (unpublished local), or tracking missing (adjacent to D11; D11 stays backup/DR). Include `Last` attempt if present. |

Do not claim D11. Do not hand-edit live tables until `go generate` /
`UPDATE_CHECK_IDS_DOC` (see `docs/design/doctor-check-ids.md`).

Severity sketch: tracking missing or `behind > 0` → warn; `ahead > 0` after
failed Last attempt → warn; both zero and Last OK → OK. Doctor `--fix`
does **not** push (fix is for other classes); Next Action text points at
`arm push-ops`.

---

## 10. Implementation sequencing (for a future story, not this PR)

1. Fake `Transport` + `Publish` / `AfterCommit` / `Lag` tests (red).
2. `opsync` algorithm (green).
3. Adapter `FetchTracking` + `AheadBehind`.
4. Wire helpers + `push-ops`; delete tracker.
5. Reserve then wire D12.
6. Docs: configuration.md, commands.md doctor section, architecture.md
   process-flow paragraph (align to “publish after write, lag on doctor,”
   delete the false per-command pull).
7. Subtractive follow-up: remove `low_stakes_push_threshold` if disposition A
   was used.

---

## 11. Summary vs Candidate A

| | A (threshold Push) | **B (this sketch)** |
| --- | --- | --- |
| Where retry lives | still `helpers.go` or copy-paste | one `Branch.Publish` |
| Low-stakes | push every N | **every** commit attempts same Publish |
| High-stakes errors | still easy to swallow | **returned** |
| Doctor | maybe a one-off `rev-list` | `Lag` on the same type |
| Callers know | threshold, Push | `AfterCommit` / `Lag` |
| Config honesty | make the lying field true | stop using it for publish |

Complexity is behind one Sync surface. CLI depth stays “commit then
`AfterCommit`.” Git depth stays inside `opsync` + a dumb Transport.
