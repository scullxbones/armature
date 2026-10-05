---
curated_by: cursor
date: 2026-10-05
---

# Quality-gap crosswalk: dogfood themes vs C1–C9

Maps open dogfood themes and raw findings to the controls in [`docs/design/quality-controls.md`](../../../design/quality-controls.md). Priority is **high / medium / low / none-yet**, grounded in the cited evidence. This file is a curated brief, not a scorecard: it does not assign status fields to themes.

**Doc vs live code:** `quality-controls.md` C7 is **ACTIVE** (`depguard` in `.golangci.yml` is the modularity map; widening an allow-list is ADR-class per ADR 0023). **C6** is still labeled GAP: leftover domain-clock injection sites remain even though `forbidigo` already bans `time.Now()` outside `cmd/`, tests, `internal/clock/`, and `internal/tui/app/`. Remaining architecture work is shrinking fences or proving a new one green — not “depguard is missing.”

Themes are relative links from this file (`./<theme>/README.md`). Raw findings use `../raw/<file>.md`.

---

## C1 — Lint (R3, R4, R5)

**quality-controls.md:** PARTIAL (summary table) / ACTIVE body; gaps for R3 purity and R1 mock ban. Live lint is broader than the C1 table (`depguard`, `forbidigo`, `gosec`, `revive`, …).

**Dogfood:** Lint as a *trust* problem more than a missing linter. Workers skip `make lint` and still report success; one `errcheck` false fail trains agents to treat C1 as noise; clock/mtime bugs are not the kind `govet`/`staticcheck` catch.

**Priority: medium** — keep C1 on; the dogfood gap is worker bypass of the full gate and a few false positives, not an absent linter.

**Evidence:**

- [`../raw/2026-08-08-validation-workers-skip-full-make-check.md`](../raw/2026-08-08-validation-workers-skip-full-make-check.md) — wave `make check` found 12 golangci-lint issues behind “Build/Lint/Tests pass.” Theme: [unreliable-worker-self-report](./unreliable-worker-self-report/README.md).
- [`../raw/2026-09-27T2355Z-cursor-tooling-errcheck-remove-after-git-reset.md`](../raw/2026-09-27T2355Z-cursor-tooling-errcheck-remove-after-git-reset.md) — `errcheck` on `os.Remove` after git reset. Theme: [tooling-integration-gaps](./tooling-integration-gaps/README.md).
- Clock-related findings are C6, not more C1 rules: [timestamp-domain-and-clock-purity](./timestamp-domain-and-clock-purity/README.md).

**C1 mock-ban gap (R1):** see [Mock-library ban](#mock-library-ban-r1--c1-gap) below. **no dogfood evidence yet** of `gomock` / `testify/mock` in this repo (`go.mod` has neither).

---

## C2 — Tests + fakes (R1, R2, R7)

**quality-controls.md:** PARTIAL. Gaps: no shared fake/real contract suite (R2); no CI hermeticity beyond convention (R7). Live `make git-test-hermetic-check` covers **raw `git init` + TestMain isolation** only.

**Dogfood:** Strong. False-green suites, hollow assertions, fixtures that are not git worktrees, local vs CI disagreement, workers skipping tests that would fail mutation.

**Priority: high** — this is the largest pile that a stronger C2 (contracts + hermeticity beyond git fixtures) would have caught or reduced.

**Evidence (themes):** [test-coverage-gaps](./test-coverage-gaps/README.md), [locateops-skips-on-disk-jsonl](./locateops-skips-on-disk-jsonl/README.md), [timestamp-domain-and-clock-purity](./timestamp-domain-and-clock-purity/README.md), [local-ci-ops-graph-divergence](./local-ci-ops-graph-divergence/README.md), [unreliable-worker-self-report](./unreliable-worker-self-report/README.md).

**Evidence (raw):**

- [`../raw/2026-06-28T2200Z-claude-workflow-json-string-int-mismatch-hidden-by-tests.md`](../raw/2026-06-28T2200Z-claude-workflow-json-string-int-mismatch-hidden-by-tests.md)
- [`../raw/2026-06-29T0001Z-5207ee28-validation-eval-corpus-divergence.md`](../raw/2026-06-29T0001Z-5207ee28-validation-eval-corpus-divergence.md)
- [`../raw/2026-08-08T1900Z-claude-validation-hollow-tests-masked-dead-worktree-path.md`](../raw/2026-08-08T1900Z-claude-validation-hollow-tests-masked-dead-worktree-path.md)
- [`../raw/2026-07-25T2340Z-claude-workflow-hard-enforcing-gate-broke-existing-tests.md`](../raw/2026-07-25T2340Z-claude-workflow-hard-enforcing-gate-broke-existing-tests.md)
- [`../raw/2026-09-27T1209Z-cursor-tooling-test-layout-armature-not-git-worktree.md`](../raw/2026-09-27T1209Z-cursor-tooling-test-layout-armature-not-git-worktree.md)
- [`../raw/2026-09-27T1211Z-cursor-tooling-locateops-snapshot-load-drops-uncommitted-ops.md`](../raw/2026-09-27T1211Z-cursor-tooling-locateops-snapshot-load-drops-uncommitted-ops.md)
- [`../raw/2026-09-02T1045Z-claude-validation-delivery-gate-tests-fail-locally-pass-in-ci.md`](../raw/2026-09-02T1045Z-claude-validation-delivery-gate-tests-fail-locally-pass-in-ci.md)
- [`../raw/2026-06-28T1700Z-claude-workflow-test-strengthening-mtime-unreliable.md`](../raw/2026-06-28T1700Z-claude-workflow-test-strengthening-mtime-unreliable.md)

See also [Hermeticity beyond git-fixture](#hermeticity-beyond-the-git-fixture-check-r7).

---

## C3 — Coverage (R6)

**quality-controls.md:** ACTIVE (cmd ≥83%, internal ≥86%). Doc already says this is a floor, not a quality signal.

**Dogfood:** High per-task coverage / 100% mutation still missed cross-task interface drift and hollow tests. Workers skip coverage by running `go test` only.

**Priority: low** — raising the percentage would not have caught the documented failures; C4 and C2 contracts would.

**Evidence:**

- [`../raw/2026-07-06T1257Z-claude-workflow-cross-task-format-drift-missed-by-per-task-review.md`](../raw/2026-07-06T1257Z-claude-workflow-cross-task-format-drift-missed-by-per-task-review.md) — 100% mutation efficacy per task, story-level defects remained. Theme: [test-coverage-gaps](./test-coverage-gaps/README.md).
- [`../raw/2026-08-08-validation-workers-skip-full-make-check.md`](../raw/2026-08-08-validation-workers-skip-full-make-check.md)
- [`../raw/2026-07-01T1330Z-claude-validation-green-ci-missed-p0-regression.md`](../raw/2026-07-01T1330Z-claude-validation-green-ci-missed-p0-regression.md)

---

## C4 — Mutation testing (R6)

**quality-controls.md:** ACTIVE (gremlins, 92% mutant-coverage / 99% efficacy ratchets).

**Dogfood:** Mutation caught some skipped-gate work (uncovered mutants on reinvented stdlib). It did **not** catch hollow assertions, JSON-vs-struct fixtures, or cross-task format drift. That matches the doc: high coverage + low efficacy is the fingerprint of tautological tests; here efficacy was high while contracts were still wrong.

**Priority: medium** — keep the ratchet; do not treat C4 as a substitute for C2 contracts or story-level integration tests.

**Evidence:**

- [`../raw/2026-08-08-validation-workers-skip-full-make-check.md`](../raw/2026-08-08-validation-workers-skip-full-make-check.md) — T3 failed mutant-coverage after claiming lint/tests pass.
- [`../raw/2026-07-06T1257Z-claude-workflow-cross-task-format-drift-missed-by-per-task-review.md`](../raw/2026-07-06T1257Z-claude-workflow-cross-task-format-drift-missed-by-per-task-review.md)
- [`../raw/2026-08-08T1900Z-claude-validation-hollow-tests-masked-dead-worktree-path.md`](../raw/2026-08-08T1900Z-claude-validation-hollow-tests-masked-dead-worktree-path.md)
- [`../raw/2026-07-23T2210Z-claude-validation-e2e-tests-caught-silent-path-bug.md`](../raw/2026-07-23T2210Z-claude-validation-e2e-tests-caught-silent-path-bug.md) — named e2e/acceptance, not mutants, caught the silent path bug. Theme: [effective-patterns](./effective-patterns/README.md).

---

## C5 — Build / type system (R5)

**quality-controls.md:** ACTIVE. Gap: Task/worker/story IDs are still `string`.

**Dogfood:** No findings that ID cross-assignment of `string` IDs caused a production bug. Build-green while behavior was wrong is C2/C3, not missing newtypes.

**Priority: none-yet** for wrapping IDs in named types from dogfood; **low** for treating `go build` as a quality signal (it already is, and it did not catch the pile).

**Evidence:** no dogfood evidence yet of ID mix-ups. Adjacent green-build misses: [`../raw/2026-07-01T1330Z-claude-validation-green-ci-missed-p0-regression.md`](../raw/2026-07-01T1330Z-claude-validation-green-ci-missed-p0-regression.md).

---

## C6 — Clock / randomness purity in domain code (R3)

**quality-controls.md:** GAP (inject `Clock`, ban `time.Now()`). **Live:** `forbidigo` already bans `time.Now()` in domain packages (exclusions: `cmd/`, `_test.go`, `internal/clock/`, `internal/tui/app/`). Remaining friction is mixed timestamp **domains** in tests and lease logic, not “anyone can call `time.Now()`.”

**Dogfood:** Yes — lease invert, whole-log backdate, mtime rematerialization.

**Priority: high** — localized, already partially linted, and CLAIMORD-W12 showed a real order invert.

**Evidence:** theme [timestamp-domain-and-clock-purity](./timestamp-domain-and-clock-purity/README.md)

- [`../raw/2026-09-27T1210Z-cursor-validation-leaselive-timestamp-domain.md`](../raw/2026-09-27T1210Z-cursor-validation-leaselive-timestamp-domain.md)
- [`../raw/2026-07-23T2215Z-claude-tooling-backdating-ops-log-for-stale-claim-test.md`](../raw/2026-07-23T2215Z-claude-tooling-backdating-ops-log-for-stale-claim-test.md)
- [`../raw/2026-06-28T1700Z-claude-workflow-test-strengthening-mtime-unreliable.md`](../raw/2026-06-28T1700Z-claude-workflow-test-strengthening-mtime-unreliable.md)

---

## C7 — Architecture conformance (R4)

**quality-controls.md:** ACTIVE — `depguard` is the modularity map (ADR 0004 fences; ADR 0023: widening an allow-list or removing a deny is ADR-class). **Live:** strict allow lists for `ops`, `claim`, `traceability`, `materialize`, `sources`, `validate`, `output`, `worktree`; pure `dag`/`issuetype`/`issueref`; `internal` must not import `cmd`.

**Dogfood:** **no dogfood evidence yet** of domain importing adapters against those rules. Closest findings are duplicated composition-root patterns and silent error discard, which depguard does not catch.

**Priority: none-yet** for “turn depguard on” (already on; C7 docs match). Remaining architecture work is shrinking fences or proving a new one green, plus duplicated `PersistentPreRunE` / silent `_, _` error classes as review/convention, not import-graph.

**Evidence:**

- [`../raw/2026-07-01T1330Z-claude-tooling-legacy-fallback-persistentprerun-pattern-undocumented.md`](../raw/2026-07-01T1330Z-claude-tooling-legacy-fallback-persistentprerun-pattern-undocumented.md) — duplicated seam, not an import violation. Theme: [documentation-gaps](./documentation-gaps/README.md).
- [`../raw/2026-07-05T0000Z-claude-validation-s18-silent-error-class.md`](../raw/2026-07-05T0000Z-claude-validation-s18-silent-error-class.md) — discarded errors. Theme: [test-coverage-gaps](./test-coverage-gaps/README.md).

---

## C8 — Contract verification for fakes (R2)

**quality-controls.md:** GAP. Shared `ContractTest(t, impl Port)` for `GitCommitter` / `MergeChecker` (and peers).

**Dogfood:** Does not name those ports, but the failure mode is the same: tests trust a stand-in (Go struct, empty reconcile, non-git `.armature` dir) that does not match production.

**Priority: high** — would have reduced JSON/struct mismatch, hollow worktree GC tests, and LocateOps snapshot tests that skip on-disk JSONL.

**Evidence:**

- [`../raw/2026-06-28T2200Z-claude-workflow-json-string-int-mismatch-hidden-by-tests.md`](../raw/2026-06-28T2200Z-claude-workflow-json-string-int-mismatch-hidden-by-tests.md)
- [`../raw/2026-08-08T1900Z-claude-validation-hollow-tests-masked-dead-worktree-path.md`](../raw/2026-08-08T1900Z-claude-validation-hollow-tests-masked-dead-worktree-path.md)
- [`../raw/2026-09-27T1209Z-cursor-tooling-test-layout-armature-not-git-worktree.md`](../raw/2026-09-27T1209Z-cursor-tooling-test-layout-armature-not-git-worktree.md)
- [`../raw/2026-09-27T1211Z-cursor-tooling-locateops-snapshot-load-drops-uncommitted-ops.md`](../raw/2026-09-27T1211Z-cursor-tooling-locateops-snapshot-load-drops-uncommitted-ops.md)
- [`../raw/2026-07-23T2210Z-claude-validation-e2e-tests-caught-silent-path-bug.md`](../raw/2026-07-23T2210Z-claude-validation-e2e-tests-caught-silent-path-bug.md) — positive control: a real end-to-end path found what unit fakes missed.

Themes: [test-coverage-gaps](./test-coverage-gaps/README.md), [locateops-skips-on-disk-jsonl](./locateops-skips-on-disk-jsonl/README.md).

---

## C9 — Spec traceability (R8)

**quality-controls.md:** GAP, blocked on armature requirement-traceability. Interim: `_REQ_` test names + a small CI orphan check.

**Dogfood:** Yes — undocumented convention, plan-name conflict, workers invent near-miss names, evidence rules graded before the evidence command exists.

**Priority: medium** — document and gate the interim naming now; full `arm validate` REQ graph is still blocked on product work.

**Evidence:** theme [req-naming-traceability-friction](./req-naming-traceability-friction/README.md)

- [`../raw/2026-06-29T1900Z-5207ee28-documentation-req-suffix-naming-convention-undocumented.md`](../raw/2026-06-29T1900Z-5207ee28-documentation-req-suffix-naming-convention-undocumented.md)
- [`../raw/2026-07-02T0000Z-claude-workflow-req-naming-conflicts-with-superpowers-plans.md`](../raw/2026-07-02T0000Z-claude-workflow-req-naming-conflicts-with-superpowers-plans.md)
- [`../raw/2026-07-24T1104Z-claude-workflow-contract-test-naming-mismatch.md`](../raw/2026-07-24T1104Z-claude-workflow-contract-test-naming-mismatch.md)
- [`../raw/2026-08-14T2338Z-5207ee28-coordination-normative-rule-lands-before-its-evidence-mechanism.md`](../raw/2026-08-14T2338Z-5207ee28-coordination-normative-rule-lands-before-its-evidence-mechanism.md)

---

## Adjacent enforcement gaps

### Required CI / branch-protection (red jobs merging when checks aren't required)

Dogfood shows **local vs CI graph disagreement** and **green jobs that still miss P0**, not a measured “required checks off, red CI merged.” Ops-push 403 blocked *publish*, which is the inverse (cannot merge because origin never saw the graph).

**Priority: high** for making publish run the same `validate --ci` as CI; **none-yet** for changing GitHub required-check rules from this pile alone (no finding that a red required job was optional and merged).

**Evidence:** [local-ci-ops-graph-divergence](./local-ci-ops-graph-divergence/README.md)

- [`../raw/2026-09-27T2320Z-cursor-validation-ci-validate-graph-uses-remote-ops.md`](../raw/2026-09-27T2320Z-cursor-validation-ci-validate-graph-uses-remote-ops.md)
- [`../raw/2026-09-27T2345Z-cursor-validation-planner-push-missed-remote-w1.md`](../raw/2026-09-27T2345Z-cursor-validation-planner-push-missed-remote-w1.md)
- [`../raw/2026-09-21T1651Z-loops-workflow-stale-origin-armature-ref.md`](../raw/2026-09-21T1651Z-loops-workflow-stale-origin-armature-ref.md)
- [`../raw/2026-07-01T1330Z-claude-validation-green-ci-missed-p0-regression.md`](../raw/2026-07-01T1330Z-claude-validation-green-ci-missed-p0-regression.md)
- [`../raw/2026-09-21T1648Z-loops-permissions-armature-ops-push-403.md`](../raw/2026-09-21T1648Z-loops-permissions-armature-ops-push-403.md) — cannot push `_armature` at all.

### Soft D1 / GateEvidence not enforced by `arm transition` / `arm merged`

`arm gate` appends `gate-evidence` ops. `cmd/armature/transition.go` and `merged.go` do not require a citable GateEvidence op. Delivery can be skipped (`--skip-delivery-gate`); story gates can skip from the wrong checkout; `merged` can be recorded for work never on main. Doctor D1 is a warning that can name unrelated in-progress issues; planner skill still says clean D1 first.

**Priority: high** — I5/I6 dogfood is exactly “self-report and promotion without durable evidence.”

**Evidence:** themes [unknown-recorded-as-answered](./unknown-recorded-as-answered/README.md), [i6-promotion-agent-owned](./i6-promotion-agent-owned/README.md), [merge-evidence-not-durable](./merge-evidence-not-durable/README.md), [sandbox-environment-vs-gates](./sandbox-environment-vs-gates/README.md), [unreliable-worker-self-report](./unreliable-worker-self-report/README.md)

- [`../raw/2026-09-27T1204Z-cursor-validation-doctor-d1-unrelated-opsclean.md`](../raw/2026-09-27T1204Z-cursor-validation-doctor-d1-unrelated-opsclean.md)
- [`../raw/2026-08-23T1612Z-claude-validation-dag-records-merged-for-work-never-on-main.md`](../raw/2026-08-23T1612Z-claude-validation-dag-records-merged-for-work-never-on-main.md)
- [`../raw/2026-08-02T1600Z-claude-workflow-story-gate-bypass-via-wrong-checkout.md`](../raw/2026-08-02T1600Z-claude-workflow-story-gate-bypass-via-wrong-checkout.md)
- [`../raw/2026-08-12T0204Z-claude-tooling-arm-merged-reads-stale-snapshot-after-transition.md`](../raw/2026-08-12T0204Z-claude-tooling-arm-merged-reads-stale-snapshot-after-transition.md)
- [`../raw/2026-08-08-validation-workers-skip-full-make-check.md`](../raw/2026-08-08-validation-workers-skip-full-make-check.md)
- [`../raw/2026-09-01T1156Z-claude-workflow-story-level-commits-erase-task-evidence.md`](../raw/2026-09-01T1156Z-claude-workflow-story-level-commits-erase-task-evidence.md)

`rg GateEvidence cmd/armature/transition.go cmd/armature/merged.go` is empty as of this curation.

### Hermeticity beyond the git-fixture check (R7)

`scripts/check-git-test-hermetic.sh` (wired as `make git-test-hermetic-check`) fails raw `git init` outside `internal/gittest` and requires TestMain isolation for packages that exec git. It does **not** ban network, `time.Sleep`, or real filesystem outside temp dirs.

**Priority: medium** — git-fixture check is real; remaining R7 gaps match mtime, ambient git config, and non-git test `.armature` layouts.

**Evidence:**

- [`../raw/2026-06-28T1700Z-claude-workflow-test-strengthening-mtime-unreliable.md`](../raw/2026-06-28T1700Z-claude-workflow-test-strengthening-mtime-unreliable.md)
- [`../raw/2026-09-02T1045Z-claude-validation-delivery-gate-tests-fail-locally-pass-in-ci.md`](../raw/2026-09-02T1045Z-claude-validation-delivery-gate-tests-fail-locally-pass-in-ci.md)
- [`../raw/2026-09-27T1209Z-cursor-tooling-test-layout-armature-not-git-worktree.md`](../raw/2026-09-27T1209Z-cursor-tooling-test-layout-armature-not-git-worktree.md)
- [`../raw/2026-06-30T2205Z-claude-tooling-arm-log-slot-breaks-worker-identity-tests.md`](../raw/2026-06-30T2205Z-claude-tooling-arm-log-slot-breaks-worker-identity-tests.md) — env leakage across tests.

No dogfood evidence yet of unit tests making live network calls.

### Mock-library ban (R1 / C1 gap)

Convention only. `go.mod` has no `gomock` / `testify/mock`. Dogfood “false green” is struct fixtures and existence-only asserts, not mock frameworks.

**Priority: low** — add `depguard`/`no-restricted-imports` when a mock package appears; not blocking on current evidence.

**Evidence:** no dogfood evidence yet. Related false-green: [test-coverage-gaps](./test-coverage-gaps/README.md).

---

## Suggested gap order (dogfood-weighted)

This reorders `quality-controls.md` “Adoption order for gaps” using the pile, without implementing anything:

1. **C8 fake/real contracts + remaining C2 hermeticity** — largest false-green cluster.
2. **Enforce GateEvidence / stop recording `merged` without git evidence; tighten D1** — I5/I6 promotion pile.
3. **C6 leftover timestamp domains** (forbidigo is already on).
4. **Publish-path `validate --ci` against `origin/_armature`** (required-CI adjacent).
5. **C9 interim `_REQ_` docs + name check.**
6. **C1 mock ban** if a mock library appears.
7. **C3/C5** — already ACTIVE; do not raise as a response to this pile.

C7 docs accuracy (depguard live) is done; it was never a new linter.

Product themes that quality-controls.md does not own (scope overlap, worktree bypass, render-context, show/list DTOs, sandbox D8) stay out of C1–C9 implementation; they remain in their theme READMEs.
