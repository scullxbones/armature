# N1 host settings checklist (Brian)

Operators flip these **after** N1 (#480–#483) is on `main`. Agents and PRs
**must not** change branch protection or Actions permissions.

Source: ADR 0026 N1 design; Phase 0 closeout issue #483.

## 1. Required check: `promotion-ready`

- [ ] In branch protection for `main` (and any other protected integration
      branch that must enforce N1 poka-yoke), mark the GitHub Actions check
      named **`promotion-ready`** as **required**.
- [ ] Match the check identity to the job `name:` (`promotion-ready` in
      `.github/workflows/ci.yml`), not the workflow filename.
- [ ] Enable only after that job has run on `main` and appears in the
      branch-protection UI. Enabling earlier sticks every open PR.
- [ ] Spot-check: a PR without a green `promotion-ready` cannot merge.

**Precondition before relying on the required check for real R2/R3/R4:** the
CI job currently runs hermetic `REQ_480` / `REQ_482` fixtures. Wire
`arm promotion-ready` against the PR under test at `GITHUB_SHA` (verify-only;
no model/secret) before treating the required check as live host poka-yoke
for production deliveries. Until then, fixtures prove the verify path; the
required-check flip is still the operator gate for the job name.

## 2. `_armature` workflow permissions (post-merge land)

The `Arm land` workflow (`.github/workflows/arm-land.yml`) must be allowed to
fast-forward-push `_armature` using the workflow’s own `GITHUB_TOKEN` (no PAT,
no extra secrets).

- [ ] Repo / org Actions permissions allow `contents: write` for this
      workflow’s token (workflow already declares it).
- [ ] Confirm the Action can FF-push `_armature` without a branch rule that
      blocks FF-only updates to that ref. Prefer minimum contents write; do
      not mint PATs.
- [ ] Concurrency group `armature-land` stays single-writer
      (`cancel-in-progress: false`).
- [ ] No force-push to `_armature` from the Action (`arm push-ops` only).
- [ ] `issues: write` remains available so jidoka can open one andon issue.

## 3. Optional

- [ ] Scheduled sweep on `Arm land` stays enabled (not disabled by repo
      Actions policy). Cron: hourly at minute 17.

## 4. Proof commands (after settings)

- [ ] `bash scripts/n1-proof.sh` green (full fixture proof bar).
- [ ] `arm land --pr 464 --dry-run` → already promoted, no new ops
      (`{"count":0,"issues":[],…}`; ops byte count unchanged).
- [ ] `actionlint` clean on N1 workflows (`.github/workflows/ci.yml`,
      `.github/workflows/arm-land.yml`).
