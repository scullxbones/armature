# N1 host settings checklist (Brian)

Operators flip these **after** N1 (#480–#483) is on `main`. Agents and PRs
**must not** change branch protection or Actions permissions.

Source: ADR 0026 N1 design; Phase 0 closeout issue #483.

## 1. Required check: `promotion-ready`

- [x] In branch protection for `main` (and any other protected integration
      branch that must enforce N1 poka-yoke), mark the GitHub Actions check
      named **`promotion-ready`** as **required**.
- [x] Match the check identity to the job `name:` (`promotion-ready` in
      `.github/workflows/ci.yml`), not the workflow filename.
- [x] Enable only after that job has run on `main` and appears in the
      branch-protection UI. Enabling earlier sticks every open PR.
- [ ] Spot-check: a PR without a green `promotion-ready` cannot merge.

**Precondition before relying on the required check for real R2/R3/R4:**
satisfied. On `pull_request` the job checks out
`github.event.pull_request.head.sha` and exits non-zero if `HEAD` is any
other commit. `github.sha` / `GITHUB_SHA` on that event is the synthetic
merge commit, not the delivery tip. Gate evidence and assessments are keyed
by the head SHA, so a merge SHA is not R2 or R3 for the delivery. On `push`
to `main` (and any other non-`pull_request` run) the job logs
`promotion-ready: N/A` and exits 0. That records the check name on `main`
without treating a push as a delivery. `GITHUB_SHA` is not evaluated there.

The job is verify-only: no model call, no secret, `permissions: contents:
read`, `persist-credentials: false`. It fetches `_armature` read-only
(`arm bootstrap` uses `networkCmd`, then a fast-forward refresh) and runs
`arm bootstrap --global` so harness files stay out of the delivery tree.
`arm promotion-ready scope` derives `--issue` and `--base` from ops state
only. A done leaf with no children is in the delivery when its recorded tip
is an ancestor of the PR head (including the head) and is not an ancestor of
`github.event.pull_request.base.sha`. A tip object missing from the clone is
outside the range. `--base` is that leaf's recorded delivery base; leaves
that share a base are one delivery. The PR body is not read. For each
delivery the job runs `arm gate run full` (`make check` from `HEAD:gates.json`)
on that clean checkout with `origin/main` fetched, then `arm promotion-ready`
with `--actions-run-id` set to `GITHUB_RUN_ID`. The job passes only when
jidoka, R2, R4, and R3 all hold. `ci=true` is recorded only when
`GITHUB_ACTIONS=true` and both `GITHUB_RUN_ID` and `GITHUB_REPOSITORY` are
non-empty. `PromotionReadyEvidence` is unchanged (citable, `ci`,
`profile=full`, matching head). The run-id bind means a pre-seeded op from
another run does not satisfy this job. check-core is not reused: that job
builds the merge commit, a different SHA from the delivery tip.

Normative order: the head commit exists, `arm review record` writes the
assessment for that exact `(base, head)`, `arm push-ops` publishes it, then
the branch is pushed. The job refetches `_armature` until every derived
delivery is assessed or `PROMOTION_OPS_WAIT_SECONDS` (default 45) elapses,
then stops refetching. It does not refetch after the gate. A workflow rerun
is not the sequence. Hermetic `REQ_480` / `REQ_482` / `REQ_483` fixtures stay
on the `n1-proof` job (`scripts/n1-proof.sh`). This job also runs
`scripts/promotion-ready-pr-proof.sh`.

No ID-scoped delivery (docs-only, routine simplify, no-comments) logs
`promotion-scope: kind=no-scope` and exits 0. That pass is not a bypass of
R2/R3/R4: `arm land` promotes only done leaves whose tip is in this range,
and there are none. An agent cannot drop an ID from the PR body to skip the
check. Stacked PRs, whose base ref is not the integration branch, still
finish. Pass/fail is the same R2/R3/R4 check on each recorded delivery base
at this head. A parent-branch tip is an ancestor of the PR base SHA and is
not this PR's delivery.

Caveats: the check sees `_armature` as of its last refresh before the gate.
A done tip appended only after that refresh is not evaluated on this run.
Land still will not promote it without an assessment keyed to its tip; R2
is not re-checked post-merge. `ci=true` is the wrapper's observation of the
Actions environment this job's runner sets. A process that exports the same
variables can set the flag locally, and that local op still fails this job's
`--actions-run-id` match unless this job produced it. No-scope does not run
the full gate. Branch protection remains the operator step above.

## 2. `_armature` workflow permissions (post-merge land)

The `Arm land` workflow (`.github/workflows/arm-land.yml`) must be allowed to
fast-forward-push `_armature` using the workflow’s own `GITHUB_TOKEN` (no PAT,
no extra secrets).

- [x] Repo / org Actions permissions allow `contents: write` for this
      workflow’s token (workflow already declares it).
- [x] Confirm the Action can FF-push `_armature` without a branch rule that
      blocks FF-only updates to that ref. Prefer minimum contents write; do
      not mint PATs.
- [x] Concurrency group `armature-land` stays single-writer
      (`cancel-in-progress: false`).
- [x] No force-push to `_armature` from the Action (`arm push-ops` only).
- [x] `issues: write` remains available so jidoka can open one andon issue.

## 3. Optional

- [x] Scheduled sweep on `Arm land` stays enabled (not disabled by repo
      Actions policy). Cron: hourly at minute 17.

## 4. Proof commands (after settings)

- [x] `bash scripts/n1-proof.sh` green (full fixture proof bar).
- [x] `arm land --pr 464 --dry-run` → already promoted, no new ops
      (`{"count":0,"issues":[],…}`; ops byte count unchanged).
- [x] `actionlint` clean on N1 workflows (`.github/workflows/ci.yml`,
      `.github/workflows/arm-land.yml`).
