# Agent-friendly defaults plan

This program makes the cheap Armature edit the correct one for agents. It encodes Brian's steering from [PR 270](https://github.com/scullxbones/armature/pull/270) onward as types, tests, and `make check` holds. It does not add a mock-library ban or an exported-symbol count limit. Those belong to another agent. The operator lands AF-P1 through AF-P6. Nothing merges in this program until that explicit go.

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The program runs `pstack/skills/poteto-mode/playbooks/autopilot-stack.md`. The operator lands every PR. AF-P3, AF-P5, and AF-P6 wait for chat review because they change agent-visible `arm` behavior.

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

## Program checklist

### Arm the program

- [ ] State the protocol and this plan to the operator, then stop. Start execution only on the operator's explicit go.
- [ ] Read these from trunk at program start. Re-read them at every tick.
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/autopilot-stack.md` (pstack lives in the Cursor plugin, not this repo. If that path is missing, read the plugin copy of the same playbook.)
  - [ ] `git show origin/main:pstack/skills/swarm/SKILL.md`
  - [ ] `git show origin/main:CONSTITUTION.md`
  - [ ] `git show origin/main:CONTEXT.md`
  - [ ] `git show origin/main:docs/conventions.md`
  - [ ] `git show origin/main:.golangci.yml`
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/opening-a-pr.md`
  - [ ] `git show origin/main:pstack/skills/correct/SKILL.md`
- [ ] On the operator's go, arm the audit tick as `/loop 1h` with the tick prompt below. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk. Audit the operation against it and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then post a short status message to the operator in chat only when the audit found a tracked change that no earlier status message reported, such as a PR opened, a code-ready head, a round launched or closed, a verdict, a merge, a stuck agent and the action taken, a blocker added or cleared, or a decision only the operator can make. Name every such change and nothing else. Do not repeat a table, the merged list, or an unchanged blocker. If the audit found none, end the turn with no reply text. Either way, log this tick's row in your decision trail. The row names the items reported, or none."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.

### Spawn owners

- [ ] Spawn one owner per PR with the full lifecycle the execution playbook names.
- [ ] Follow this dependency graph. Start dependent work only after its parent merges, or base it on the parent branch when the execution playbook stacks.
  - [ ] `AF-P1` and `AF-P4` are independent and first. Both branch from `main`.
  - [ ] `AF-P2` after `AF-P1`.
  - [ ] `AF-P3` after `AF-P1`.
  - [ ] `AF-P5` after `AF-P2`.
  - [ ] `AF-P6` after `AF-P3`.
- [ ] Hold the file boundaries. `AF-P1` touches only commit-subject checking. `AF-P2` touches ops parse and load. `AF-P3` touches validate findings goldens. `AF-P4` touches worktree identity resolution. `AF-P5` touches publish retry validation. `AF-P6` touches `unlink` and `link` graph-edit preview. No PR widens a depguard allow-list. ADR 0023 already forbids that.
- [ ] Hold the review gate. `AF-P3`, `AF-P5`, and `AF-P6` change an interaction. They wait for the operator's review in chat with screenshots and a video before merge.

### PR mechanics, for every PR

- [ ] Resolve the forge once. Default to `gh`; if `command -v origin` succeeds and Origin can resolve the repository, use `origin pr` for every PR operation. Record any fallback to `gh`. Never require `gt`.
- [ ] Open the PR ready, never draft, per **Opening a PR**. Use the run's built-in PR tool when it has one, else `origin pr create --status open --base <base-branch>` or `gh pr create --base <base-branch>` according to the resolved forge. A stack child targets its parent branch.
- [ ] Run the repo's lint and typecheck once before the PR-facing push. Push with hooks on.
- [ ] Run `/deslop` before each commit and `/no-comments` before review.
- [ ] Triage every Bugbot and security-reviewer comment per `../references/bugbot-triage.md`.
- [ ] Rebase onto current trunk before the code-ready report and babysit. Keep that merge base in fix rounds. Rebase again only at merge prep, on a `git merge-tree` conflict with trunk, or on a CI failure that comes from a change on trunk.

### Verdict and merge, for every PR

- [ ] A full swarm runs once, at the code-ready head. Another full swarm runs only for new behavior, a redesign, or a large new diff. A rebase, a merge-conflict resolution, or a minor fix gets a single verifier at the new head. Run each full swarm per `pstack/skills/swarm/SKILL.md`. One gates lane. The ten live lanes from the PR's **Verify, live** block. The perf lane from its **Verify, perf** block. Two or more audit lanes, each with its own focus, that read the diff and the receipts and distrust the PR body. The root audits the receipts in the merge-ready report before the verdict.
- [ ] Clean only when every lane is `PASS`. Findings go back to the owner, including a defect that a lane filed as a note. A new head gets a fresh verdict, except for results that stay valid under the patch-id rule in `playbooks/shipping.md`.
- [ ] Owners report STACK-READY and never merge. The root appends each clean PR onto the base-branch stack. The operator lands the contiguous verified run bottom-up. Patch-id must match after any rebase, per `playbooks/shipping.md`.

### Boot recipe, for every live lane

Each live lane runs on its own cloud VM at the PR head. Drive through `control-cli` from `cursor-team-kit` when that skill is present. If it is missing, drive the same commands in the VM shell and still save the named screenshot.

- [ ] `git fetch origin <head-branch> && git checkout <head SHA>`.
- [ ] Install with `make install` when the PR ships a binary change. Wait until `arm version` prints.
- [ ] Deliver input only through argv and stdin. Read-only diagnostics are `git log -1 --format=%s`, `make check-fast`, and `arm --help`.
- [ ] Save every screenshot to `/tmp/swarm-<pr-id>/worker-<n>/<slug>.png` and return the paths with the report.

## Gate commit subjects with commitref (AF-P1)

**Depends on.** None.

**Files.**

- [ ] Edit `internal/commitref/pattern.go`.
- [ ] Create `scripts/check-commit-subjects.sh`.
- [ ] Edit `Makefile`.
- [ ] Edit `docs/conventions.md` only to name the new check and to add `ci` if the prototype still requires it.

**Build.**

- [ ] Add `SubjectHasIssueScope(subject string, parentCount int) bool` next to `IsValidReference` so a PR range can be checked without an issue id. Wire `make check` and `make check-fast` to `scripts/check-commit-subjects.sh` on `origin/main..HEAD`. Do not restore `prepare-commit-msg`. Architecture.md already records that the old hook broke `internal/commitref`.

**You see.**

- [ ] `make check-fast` fails on a branch whose tip subject is `refactor: overnight simplify production Go (no behavior change)` and passes on `refactor(GNHF): shrink validate parse helpers`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `internal/commitref/pattern_test.go` gains `TestSubjectHasIssueScope`. Run `go test ./internal/commitref`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `scripts/check-commit-subjects.sh` at trunk and head on a throwaway commit with subject `fix: no scope`. If trunk lacks the script, record that and gate head failure plus a green scoped commit. Save `af-p1-regression.png`. Pass when head fails the unscoped commit and trunk either lacks the check or also fails.
- [ ] Lane 2. Scoped `fix(D12): surface fetch failure` passes. Save `af-p1-scoped-fix.png`. Pass when the script exits 0.
- [ ] Lane 3. `merge: GNHF integrate nightly` with two parents passes. Save `af-p1-merge.png`. Pass when the script exits 0.
- [ ] Lane 4. `feat(PARCLI) 2/3: thread Env` fails. Save `af-p1-parcli-space.png`. Pass when the script exits non-zero.
- [ ] Lane 5. `ci(TOPTIER-S6): fan out check jobs` matches the documented type list after the conventions edit. Save `af-p1-ci-type.png`. Pass when the script and `docs/conventions.md` agree.
- [ ] Lane 6. GitHub squash suffix ` (#271)` is stripped before the match. Save `af-p1-squash-suffix.png`. Pass when the scoped subject still matches.
- [ ] Lane 7. `chore(AGTFND): ignore me` fails unless `chore` is added on purpose. Save `af-p1-chore.png`. Pass when the outcome matches `CommitTypes`.
- [ ] Lane 8. Empty `origin/main..HEAD` on a branch with no new commits prints a skip line and exits 0. Save `af-p1-empty-range.png`. Pass when exit is 0.
- [ ] Lane 9. `make check-fast` on a docs-only scoped commit still runs the subject check. Save `af-p1-check-fast.png`. Pass when the script is in the log.
- [ ] Lane 10. Replay of the 31 unscoped subjects from the prototype file fails at least the overnight `refactor:` rows. Save `af-p1-replay.png`. Pass when those subjects are rejected.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of `scripts/check-commit-subjects.sh` on `origin/main..HEAD` with about 80 commits, at trunk and at head. If trunk lacks the script, also name script runtime at head and the wait until `make check-fast` returns.
- [ ] Probe. `hyperfine --warmup 2 'scripts/check-commit-subjects.sh'` at trunk (or `true` if missing) and at head, interleaved.
- [ ] Baseline. Record the trunk value first. Prototype guess is under 200ms of git-log plus grep. Measure, do not keep the guess.
- [ ] Rule. Head must stay under 2s on that range. If trunk has no script, fail head above 2s rather than inventing a ratio.

**Review gate.** None. AF-P1 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends it to the base-branch stack and the operator lands it bottom-up.

## Fail loud on ops parse skips (AF-P2)

**Depends on.** AF-P1.

**Files.**

- [ ] Edit `internal/ops/log.go`.
- [ ] Edit `internal/ops/opstream.go`.
- [ ] Edit `internal/ops/parse.go`.
- [ ] Create `internal/ops/parse_fail_loud_test.go`.

**Build.**

- [ ] Change `ReadLogFromOffset` so a `ParseLine` error returns instead of `continue`. Keep newer-schema errors as errors in `opstream.go`. Add a typed parse result so callers cannot ignore `error`. Corrupt lines stay visible as load failure, not a warning that materialize drops.

**You see.**

- [ ] Loading a log with one newer-schema line fails the load. A v1 golden corpus still materializes.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `TestParseLineNewerSchemaFailsLoad_REQ_AGTFND` and `TestCorruptLineFailsReadLogFromOffset_REQ_AGTFND`. Run `go test ./internal/ops`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Replay `internal/materialize/testdata/v1` at trunk and head. Save `af-p2-v1-golden.png`. Pass when both produce the same golden state.
- [ ] Lane 2. Inject a v2 schema line into a temp log and run the loader at head. Save `af-p2-v2-fail.png`. Pass when the loader returns the parse error.
- [ ] Lane 3. Inject `not json` into a temp log. Save `af-p2-corrupt.png`. Pass when `ReadLogFromOffset` returns error.
- [ ] Lane 4. Worker-id mismatch still warns without dropping later valid lines if that remains the documented rule. Save `af-p2-mismatch.png`. Pass when the documented rule holds at head.
- [ ] Lane 5. `arm doctor` on a fixture with a corrupt log prints a Command Failure, not a silent D-check skip. Save `af-p2-doctor.png`. Pass when stdout matches ADR 0020 envelope in agent mode.
- [ ] Lane 6. Empty log still loads. Save `af-p2-empty.png`. Pass when zero ops and nil error.
- [ ] Lane 7. Legacy 5-element v1 line still parses. Save `af-p2-legacy-v1.png`. Pass when schema v1 is accepted.
- [ ] Lane 8. Two valid lines around a corrupt line fail the whole load (no partial fold). Save `af-p2-no-partial.png`. Pass when error is set and items are unused.
- [ ] Lane 9. `make check-fast` on the ops packages is green. Save `af-p2-check-fast.png`. Pass when the command exits 0.
- [ ] Lane 10. Confirm I2. Local log bytes are unchanged after a failed load. Save `af-p2-append-only.png`. Pass when `cmp` on the jsonl is silent.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. `LoadFromDirValidated` time on the v1 corpus at trunk and head.
- [ ] Probe. `go test -bench=LoadFromDir -count=5 ./internal/ops` at both SHAs, interleaved.
- [ ] Baseline. Record trunk ns/op first.
- [ ] Rule. Head must not exceed trunk by 20 percent on that bench. Fail if head is slower by more than that.

**Review gate.** The operator reviews before merge.

- [ ] Copy lane 2 screenshots into `media/af-p2-review-v2-fail.png`.
- [ ] Record a 30 to 60 second video of the change on a lane VM. Save it as `media/af-p2-review.mp4`.
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends it to the base-branch stack and the operator lands it bottom-up.

## Golden-test validate findings (AF-P3)

**Depends on.** AF-P1.

**Files.**

- [ ] Edit `internal/validate/validate.go` only if a golden proves a live skip.
- [ ] Create `internal/validate/testdata/findings/`.
- [ ] Edit `internal/validate/validate_extra_test.go`.

**Build.**

- [ ] Commit JSON fixtures for E7 parse failures (`{"entries":{"src":42}}` and garbage `last_synced`) and for E10 malformed globs. Assert rule id, severity, and message prefix. Graph Findings stay validate payload per ADR 0020 decision 7. Command Failures stay at `cmd/`.

**You see.**

- [ ] `go test ./internal/validate` fails if `parseManifestData` decodes entries as `json.RawMessage` again.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `TestParseManifestMalformedEntriesStillE7_REQ_AGTFND` and `TestMalformedGlobStillE10_REQ_AGTFND`. Run `go test ./internal/validate`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `arm validate --ci` on a tiny fixture repo at trunk and head. Save `af-p3-regression.png`. Pass when E7 still fires on the two malformed manifests.
- [ ] Lane 2. Agent JSON mode prints findings in the success envelope, not `{error}`. Save `af-p3-json-findings.png`. Pass when `error` is absent and a finding lists `E7`.
- [ ] Lane 3. Human mode still prints the E7 sentence with `cannot parse source manifest`. Save `af-p3-human-e7.png`. Pass when the text matches the golden prefix.
- [ ] Lane 4. E10 glob stays a finding, not a `USAGE` Command Failure. Save `af-p3-e10.png`. Pass when rule `E10` is present.
- [ ] Lane 5. Valid manifest with a real source id produces no E7 parse finding. Save `af-p3-valid.png`. Pass when that rule is absent.
- [ ] Lane 6. `internal/validate` still does not import `sources`. Save `af-p3-depguard.png`. Pass when `make lint` is green.
- [ ] Lane 7. Uncited node still emits E7 uncited, distinct from parse skip. Save `af-p3-uncited.png`. Pass when both messages stay distinct.
- [ ] Lane 8. Empty `entries` object is valid. Save `af-p3-empty-entries.png`. Pass when parse succeeds.
- [ ] Lane 9. `entries` as a JSON array still E7-parses. Save `af-p3-entries-array.png`. Pass when the parse finding fires.
- [ ] Lane 10. `make check-fast` green. Save `af-p3-check-fast.png`. Pass when exit is 0.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. `go test ./internal/validate` duration at trunk and head.
- [ ] Probe. `go test -count=3 ./internal/validate` interleaved.
- [ ] Baseline. Record trunk duration first.
- [ ] Rule. Head must stay within 30 percent of trunk. Fail above that.

**Review gate.** The operator reviews before merge.

- [ ] Copy lane 2 screenshots into `media/af-p3-review-json.png`.
- [ ] Record a 30 to 60 second video of the change on a lane VM. Save it as `media/af-p3-review.mp4`.
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends it to the base-branch stack and the operator lands it bottom-up.

## Bind identity to the invoking worktree (AF-P4)

**Depends on.** None.

**Files.**

- [ ] Edit `internal/config/context.go`.
- [ ] Edit `cmd/armature/helpers.go`.
- [ ] Create `internal/config/worktree_identity_test.go`.

**Build.**

- [ ] Introduce distinct types for the invoking worktree path and the main git dir so `ResolveContext` cannot silently substitute the main checkout for worker-id and `ARM_LOG_SLOT`. Keep git-dir lookup for repo-global git, but read `armature.worker-id` from the invoking worktree config.

**You see.**

- [ ] `arm worker-init` inside a linked worktree writes that worktree's local config. A second worktree does not inherit the first UUID.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `TestResolveContextKeepsInvokingWorktree_REQ_AGTFND`. Run `go test ./internal/config ./cmd/armature -run Worktree`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Create two linked worktrees and run `arm worker-init --check` at trunk and head. Save `af-p4-regression.png`. Pass when head keeps per-worktree ids. Record trunk if it still collapses to the main path.
- [ ] Lane 2. Invalid `ARM_LOG_SLOT` fails closed in harness-hook. Save `af-p4-log-slot.png`. Pass when the hook returns the slot error instead of discarding it.
- [ ] Lane 3. Shared UUID with two slots cannot steal the other slot's claim. Save `af-p4-slot-claim.png`. Pass when hook matching uses the exact slot.
- [ ] Lane 4. `git config --local` in the worktree is what `GetWorkerID` reads. Save `af-p4-local-config.png`. Pass when the values match.
- [ ] Lane 5. Main checkout identity is unchanged. Save `af-p4-main.png`. Pass when main still has its own id.
- [ ] Lane 6. `docs/concepts.md` no longer tells users that worker-id is clone-only if the type is worktree-scoped. Save `af-p4-docs.png`. Pass when the doc matches the type.
- [ ] Lane 7. Windows remains unsupported. Save `af-p4-windows.png`. Pass when the existing skip still holds.
- [ ] Lane 8. `make git-test-hermetic-check` stays green. Save `af-p4-hermetic.png`. Pass when exit is 0.
- [ ] Lane 9. Parallel CLI tests still pass (`PARCLI`). Save `af-p4-parcli.png`. Pass when `go test ./cmd/armature -count=1` exits 0.
- [ ] Lane 10. `make check-fast` green. Save `af-p4-check-fast.png`. Pass when exit is 0.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. `arm worker-init --check` wall time in a linked worktree at trunk and head.
- [ ] Probe. `/usr/bin/time -f %e arm worker-init --check` interleaved.
- [ ] Baseline. Record trunk seconds first.
- [ ] Rule. Head must not add more than 200ms over trunk on that command.

**Review gate.** None. AF-P4 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends it to the base-branch stack and the operator lands it bottom-up.

## Require revalidate on publish retry (AF-P5)

**Depends on.** AF-P2.

**Files.**

- [ ] Edit `cmd/armature/push_ops.go`.
- [ ] Edit `cmd/armature/publish_validate.go`.
- [ ] Edit `cmd/armature/helpers.go`.
- [ ] Create `cmd/armature/publish_retry_validate_test.go`.

**Build.**

- [ ] Make `pushOpsBranchAfter` take a non-optional after-integrate callback type. A nil callback must not compile. Overlay remote `sources/manifest.json` into the candidate graph. Re-run CI validation after every rebase unless a recorded override is active.

**You see.**

- [ ] A first non-fast-forward followed by a rebase that introduces a validate finding refuses the retry push.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `TestPublishRetryRevalidates_REQ_PUBLISH_VALIDATE` and `TestOverlayIncludesRemoteSources_REQ_PUBLISH_VALIDATE`. Run `go test ./cmd/armature -run Publish`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Happy-path `arm push-ops` on a clean isolated pair of clones at trunk and head. Save `af-p5-regression.png`. Pass when both succeed.
- [ ] Lane 2. Simulated non-ff then invalid graph refuses retry. Save `af-p5-retry-fail.png`. Pass when the command exits non-zero and the remote has no new commit.
- [ ] Lane 3. Recorded override still skips validate and records the reason. Save `af-p5-override.png`. Pass when the override op is present.
- [ ] Lane 4. Remote-only source UUID is known after overlay. Save `af-p5-remote-source.png`. Pass when validate does not emit unknown source.
- [ ] Lane 5. Auth failure stays a classified publish Command Failure from [PR 272](https://github.com/scullxbones/armature/pull/272). Save `af-p5-auth.png`. Pass when the code prefix is the publish class, not `GENERAL-1`.
- [ ] Lane 6. Local ops commit remains after failed push (I2). Save `af-p5-local-keep.png`. Pass when `git log` in the ops worktree still has the commit.
- [ ] Lane 7. Foreign child after rebase is rechecked ([PR 290](https://github.com/scullxbones/armature/pull/290)). Save `af-p5-foreign-child.png`. Pass when revert or decompose does not publish a stale snapshot.
- [ ] Lane 8. `requirePublishedOwner` after integrate ([PR 279](https://github.com/scullxbones/armature/pull/279)). Save `af-p5-owner.png`. Pass when a lost owner cannot heartbeat.
- [ ] Lane 9. `make test-e2eharness` subset for publish if present. Save `af-p5-e2e.png`. Pass when that subset exits 0.
- [ ] Lane 10. `make check-fast` green. Save `af-p5-check-fast.png`. Pass when exit is 0.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Happy-path `arm push-ops` wall time at trunk and head on a one-op fixture.
- [ ] Probe. Three interleaved runs of that command at each SHA.
- [ ] Baseline. Record trunk seconds first.
- [ ] Rule. Extra validate on retry may add cost only on the retry path. Happy-path head must stay within 25 percent of trunk.

**Review gate.** The operator reviews before merge.

- [ ] Copy lane 2 screenshots into `media/af-p5-review-retry-fail.png`.
- [ ] Record a 30 to 60 second video of the change on a lane VM. Save it as `media/af-p5-review.mp4`.
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends it to the base-branch stack and the operator lands it bottom-up.

## Preview graph edits before unlink (AF-P6)

**Depends on.** AF-P3.

**Files.**

- [ ] Edit `cmd/armature/unlink.go`.
- [ ] Edit `cmd/armature/link.go`.
- [ ] Create `cmd/armature/graph_edit_preview_test.go`.

**Build.**

- [ ] Default `arm unlink` and `arm link` to compute the post-edit Graph Finding set and refuse when the edit would introduce findings. Keep a recorded human override that is not `--skip-validate-gate`. Print the cascade (the next overlapping pair) in the Command Failure `cause`.

**You see.**

- [ ] Unlinking `TOPTIER-S6-T3` from `TOPTIER-S15-T2` while README overlap remains prints `UNLINK-1` with the next pair, and writes no op.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `TestUnlinkRefusesIntroducedOverlap_REQ_AGTFND` and `TestUnlinkCauseNamesNextPair_REQ_AGTFND`. Run `go test ./cmd/armature -run Unlink`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Safe unlink of an unrelated dep at trunk and head. Save `af-p6-regression.png`. Pass when both write the unlink op.
- [ ] Lane 2. Overlap cascade fixture matching the Oct 4 Project session. Save `af-p6-cascade.png`. Pass when no op is written and `cause` names the next pair.
- [ ] Lane 3. Agent JSON stdout is `{error:{code,cause,next_actions,exit_code}}`. Save `af-p6-json.png`. Pass when the object matches ADR 0020.
- [ ] Lane 4. `Try` lines include `arm link` or scope-narrow, not `arm merged`. Save `af-p6-next-actions.png`. Pass when `merged` is absent.
- [ ] Lane 5. Human override records reason and still does not claim green. Save `af-p6-override.png`. Pass when the ledger grows and validate stays red.
- [ ] Lane 6. `arm link` that would create overlap also refuses. Save `af-p6-link.png`. Pass when no op is written.
- [ ] Lane 7. I2. Failed unlink leaves worker log bytes unchanged. Save `af-p6-append-only.png`. Pass when `cmp` is silent.
- [ ] Lane 8. Ready computation still treats `merged` as stronger than `done` (I6). Save `af-p6-i6.png`. Pass when a done-only blocker does not ready the dependent.
- [ ] Lane 9. `arm doctor` after a refused unlink shows no new finding. Save `af-p6-doctor.png`. Pass when the finding set is unchanged.
- [ ] Lane 10. `make check-fast` green. Save `af-p6-check-fast.png`. Pass when exit is 0.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. `arm unlink` wall time on a 50-issue fixture at trunk and head.
- [ ] Probe. Three interleaved runs at each SHA.
- [ ] Baseline. Record trunk seconds first.
- [ ] Rule. Head may run validate once extra. Fail if head is more than 2x trunk on that fixture.

**Review gate.** The operator reviews before merge.

- [ ] Copy lane 2 screenshots into `media/af-p6-review-cascade.png`.
- [ ] Record a 30 to 60 second video of the change on a lane VM. Save it as `media/af-p6-review.mp4`.
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends it to the base-branch stack and the operator lands it bottom-up.

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with the report the execution playbook names.

## Appendix A. Prototype evidence

Throwaway measurements. No prototype branch is part of the plan PR.

**Commit-subject gate.** On `origin/main` after [PR 270](https://github.com/scullxbones/armature/pull/270) (`e029883715`), `git log` has 81 commits. `internal/commitref` typed-scope matching accepts 50 and rejects 31. Nineteen of the 31 are unscoped `refactor:` overnight subjects. `ci:` is not in `CommitTypes`, so [PR 340](https://github.com/scullxbones/armature/pull/340) and [PR 341](https://github.com/scullxbones/armature/pull/341) would fail a naive copy of `CommitTypes`. `feat(PARCLI) 2/3:` fails because the scope contains a space. Restoring `prepare-commit-msg` is rejected. Architecture.md already says that hook broke delivery-gate patterns. Local prototype log is `commit-scope-prototype.txt` under the planning agent's artifacts directory.

**Ops parse skip.** `internal/ops/log.go` `ReadLogFromOffset` still `continue`s on `ParseLine` error. `opstream.go` returns newer-schema errors and continues on other corrupt lines. A contract test that loads a v2 line would have caught [PR 305](https://github.com/scullxbones/armature/pull/305) Codex P1. Unproven on this machine. Full `make mutate` was not run for this prototype.

**E7 decode.** Head `parseManifestData` already decodes `sourceEntry`. The [PR 333](https://github.com/scullxbones/armature/pull/333) follow-up in transcript `bc-48c50487-3aa9-5f54-bbae-b261a92e8774` is the proof that a golden was missing at review time. Unproven whether current `validate_extra_test.go` covers `{"entries":{"src":42}}` until AF-P3 adds it.

**Clock.** `forbidigo` already bans `time.Now` outside tests, `internal/clock/`, and `internal/tui/app/`. `cmd/` is excluded. Production `internal/decompose` no longer calls `time.Now`. `docs/design/quality-controls.md` C6 is stale. Remaining cmd wall-clock sites are a later hold, not AF-P1 through AF-P6.

## Appendix B. Alternatives rejected

**More AGENTS.md and skill prose.** Brian's overnight prompts already repeat CONTEXT.md, CONSTITUTION.md, and "never widen depguard". Agents still ship unscoped commits and silent skips. Encode Lessons in Structure. Prose lost.

**Mock-library ban and exported-symbol count limit.** Out of scope. Another agent owns those.

**Re-add prepare-commit-msg.** It prefixed issue ids and broke `^`-anchored `commitref` matching. AF-P1 is a read-only range check.

**Fail the whole CI graph on any Codex P1.** N1 and N4. Codex stays advisory (I5). Deterministic checks copy only the classes that repeated.

**Lease or lock rewrite for claims.** Transcripts `bc-25734276-8d2f-502b-b594-30f3e93e94d7` and `bc-8a1b7494-6c92-572e-ae73-db8fcc29fe84` are design-only. [PR 276](https://github.com/scullxbones/armature/pull/276) through [PR 282](https://github.com/scullxbones/armature/pull/282) already landed ownership. This program does not reopen TTL shape.

**Widen depguard to "fix" validate importing sources.** [PR 333](https://github.com/scullxbones/armature/pull/333) follow-up forbade that. Duplicate the `SourceEntry` fields locally, as head already does.

## Appendix C. Risks

**`control-cli` may be absent in this repo.** Live lanes still run argv and save screenshots. Named in AF-P1 through AF-P6 boot.

**AF-P2 fail-loud can break dogfood clones with corrupt lines that trunk skipped.** Owner watches v1 golden and a counted corrupt-line fixture. I2 forbids rewriting those logs. Operators repair with a new op, not a rewrite (T2).

**AF-P1 will fail current overnight automation until prompts use `refactor(GNHF):`.** That is the point. Coordinate with the GNHF dispatcher before landing.

**AF-P6 default refuse may block legitimate unlinks that today require a human cascade of extra `arm link`s.** The Command Failure must name the next pair. Do not recommend `arm merged` (I6, [PR 283](https://github.com/scullxbones/armature/pull/283)).

**Stale C6 docs.** Do not spend a PR on clock injection until forbidigo cmd exclusions are measured again.

## Appendix D. Links and reading list

Read before editing. `CONSTITUTION.md` I1 through I7 and T1 through T3. `CONTEXT.md` for Issue, Ready, Done, Merged, Graph Finding, Command Failure, Next Action. `docs/conventions.md`. ADR 0004, ADR 0020, ADR 0023. `docs/design/quality-controls.md`. `docs/dogfood/findings/themes/quality-gap-crosswalk.md`.

`AF-P2` and `AF-P5` get `pstack/skills/how/SKILL.md` and `pstack/skills/interrogate/SKILL.md` because they change load and publish failure behavior.

Decision trail per `pstack/skills/show-me-your-work/SKILL.md` lives with the execution owner, not in this file.

## Appendix E. Ranked pitfalls

Score is frequency times cost. Cost is review churn, revert risk, or invariant miss. Every class has at least two PRs or commits. Every fix names the check that would have caught it.

**1. Unscoped conventional commits.** Codex P1 on [PR 271](https://github.com/scullxbones/armature/pull/271), [PR 292](https://github.com/scullxbones/armature/pull/292), [PR 296](https://github.com/scullxbones/armature/pull/296), [PR 297](https://github.com/scullxbones/armature/pull/297), [PR 300](https://github.com/scullxbones/armature/pull/300), [PR 309](https://github.com/scullxbones/armature/pull/309), [PR 317](https://github.com/scullxbones/armature/pull/317). Delivery gate already uses `commitref` per issue. PR range check does not. **AF-P1.** Check `scripts/check-commit-subjects.sh` in `make check`.

**2. Silent skip on parse and publish paths.** Codex P1 on [PR 305](https://github.com/scullxbones/armature/pull/305) (newer schema skipped in the stream), [PR 271](https://github.com/scullxbones/armature/pull/271) (D12 fetch discarded), [PR 290](https://github.com/scullxbones/armature/pull/290) (revert retry), [PR 309](https://github.com/scullxbones/armature/pull/309) (legacy shim drop). Overnight prompt in `bc-1e08701b-5bae-5b8c-8383-8b86876f81c1` still says no behavior change while loaders `continue`. **AF-P2.** Check `TestCorruptLineFailsReadLogFromOffset_REQ_AGTFND`.

**3. Validate behavior change under a simplify PR.** Transcript `bc-48c50487-3aa9-5f54-bbae-b261a92e8774` on [PR 333](https://github.com/scullxbones/armature/pull/333) (`parseManifestData` as `RawMessage`, E7 silent). [PR 273](https://github.com/scullxbones/armature/pull/273) Codex P1 (E10 became a Command Failure). **AF-P3.** Check the E7 and E10 goldens.

**4. Publish retry without revalidate, incomplete overlay.** Codex P1 on [PR 285](https://github.com/scullxbones/armature/pull/285) (two comments), [PR 279](https://github.com/scullxbones/armature/pull/279), [PR 307](https://github.com/scullxbones/armature/pull/307). Transcript `bc-2a5b580f-6771-5133-8646-abec346ca88c` had to patch #285 after the fact. **AF-P5.** Check `TestPublishRetryRevalidates_REQ_PUBLISH_VALIDATE`.

**5. Worktree identity collapsed to the main checkout.** Codex P1 on [PR 282](https://github.com/scullxbones/armature/pull/282) and [PR 281](https://github.com/scullxbones/armature/pull/281). Dogfood theme `worktree-scoped-worker-id-overrides-clone-local`. PARCLI [PR 345](https://github.com/scullxbones/armature/pull/345) and [PR 346](https://github.com/scullxbones/armature/pull/346) started threading env and dir. **AF-P4.** Check `TestResolveContextKeepsInvokingWorktree_REQ_AGTFND`.

**6. Graph edits that invent overlap, plus I6 confusion.** Project transcript `bc-495167c1-f4c1-46b6-85d9-8cbeaa2b3a3b` (two `UNLINK-1` cascades on Oct 4). Dogfood `scope-overlap-validation-gaps`. Codex P1 on [PR 283](https://github.com/scullxbones/armature/pull/283) (`arm merged` while the GitHub PR was still open). **AF-P6.** Check `TestUnlinkRefusesIntroducedOverlap_REQ_AGTFND`. Next Actions must not suggest premature `arm merged`.

**Not in this stack.** Leftover `cmd/` `time.Now` (forbidigo hole). Skills split is already covered by `cmd/armature/bootstrap_test.go` after [PR 283](https://github.com/scullxbones/armature/pull/283). Comment-strip waves (`bc-c5027dc5-dc6d-5c0b-9c9a-366a7096fdac`, `bc-e44199db-a2c4-5922-99e9-14fb21bda10e`) stay overnight process, not a new linter.

## Appendix F. Recall brief

Scope. All 33 JSONL files under `/cursor/stores/bot/armature-transcripts/` from 2026-09-27 through 2026-10-16. INDEX.md listed 11. Recall used every jsonl. Transcripts stay out of git.

**Capsule.**

- Brian steers for 0.1.0 shipped, then leftovers, with Ready PRs handed to PJ. Project thread `bc-495167c1-f4c1-46b6-85d9-8cbeaa2b3a3b`.
- Overnight GNHF must shrink deep modules, never widen depguard, never change CLI or ops schema, and keep `make check` green. Many agent ids share that prompt.
- Comment Sicko fences to the last 24h of merged PR files. Meat comments die. Structural MUST KILLs become renames, not new comments.
- Codex-fix agents are told exact discussion ids, no force-push, merge `origin/main` rather than rebase, and leave GitHub threads untouched.
- Claim and TTL design (`bc-25734276-8d2f-502b-b594-30f3e93e94d7`, `bc-8a1b7494-6c92-572e-ae73-db8fcc29fe84`) stays design-only until Brian is convinced.

**Threads.**

- `[merged #276]` claim ownership and TTL 0. Transcript `bc-d5d5f9b4-ccde-5804-bba4-189bdb2658cc`.
- `[merged #285]` publish validate. Transcript `bc-2a5b580f-6771-5133-8646-abec346ca88c`.
- `[merged #333]` validate simplify plus E7 regression fix. Transcript `bc-48c50487-3aa9-5f54-bbae-b261a92e8774`.
- `[merged #340]` macOS `cmd/armature` git-spawn cost. Transcript `bc-089d0675-b2b1-5468-b141-1e10c8a326fd`.
- `[merged #345]` PARCLI invocation env. Transcript `bc-500998b0-8aab-5951-8226-1c539f34aba1` fence.
- `[planned, not started]` AF-P1 through AF-P6 in this file.

**Problems.**

- Unscoped commits survive review until Codex nags.
- Simplify PRs silently change parse and finding shape.
- Publish retry and worktree identity need types, not comments.
- Unlink overlap cascades burn human loops (`UNLINK-1`).
- Agents still reach for `arm merged` to unblock stacks.

**Next move.** Land this plan PR ready. On Brian's go, run AF-P1.
