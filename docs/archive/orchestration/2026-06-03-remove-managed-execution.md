# Remove Managed Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove `arm orchestrate`, `arm worker run`, and their managed-execution internals while preserving Armature's DAG workflow and the planned harness-native policy hook direction.

**Architecture:** Armature remains the source of task truth, claims, scope, context, lifecycle, validation, and audit records. External coordinator and worker skills invoke explicit Armature commands and dispatch harnesses themselves. Future enforcement enters through harness-native hooks that call Armature policy commands; Armature does not launch, supervise, retry, verify completion for, or commit on behalf of harnesses.

**Tech Stack:** Go, Cobra, JSONL operation logs, embedded Markdown skills, Armature source manifest and DAG commands.

---

## File And Commit Map

1. Migrate active local DAG intent and remove obsolete execution records while the old CLI can still read them.
2. Remove the public commands, managed-execution packages, config, and op vocabulary.
3. Rewrite embedded skills around explicit coordinator/worker workflows.
4. Archive obsolete design history, rewrite current docs, and update source registrations.
5. Run repository, source, DAG, and stale-reference verification.

The migration is deliberately first. Once the removed op constants disappear, any remaining managed-execution records would make materialization fail.
The `.arm/` directory is intentionally gitignored, so Tasks 1, 2, and 8 are
verified local-state migration checkpoints rather than commits.

### Task 1: Re-scope HKREFACT And Retire Active Managed-Execution Work

**Files:**
- Modify: `.arm/.armature/ops/codex-worker.log` through `arm amend` and `arm transition`
- Reference: `docs/superpowers/specs/2026-06-03-remove-managed-execution-design.md`
- Reference: `docs/superpowers/specs/2026-05-26-harness-hooks-guardrail-refactor-design.md`
- Reference: `docs/superpowers/plans/2026-05-26-harness-hooks-guardrail-refactor.md`

- [ ] **Step 1: Confirm the graph state before mutation**

Run:

```bash
go run ./cmd/armature show ORCRUN --format json
go run ./cmd/armature show story-1778637796 --format json
go run ./cmd/armature list --parent HKREFACT --format json
```

Expected:
- `ORCRUN` is `in-progress`.
- `story-1778637796` and all its direct children are historical `merged` work.
- `HKREFACT-T01` through `T12` are open except unrelated `T14`; `T13`, `T18`, and `T19` are historical.

- [ ] **Step 2: Cancel only the still-active ORCRUN epic**

Run:

```bash
go run ./cmd/armature transition ORCRUN --to cancelled --outcome "Managed execution was intentionally removed. Coordinator and worker skills now use explicit Armature commands and external harness dispatch."
```

Expected: `ORCRUN` becomes `cancelled`; completed and cancelled descendants retain their existing history.

- [ ] **Step 3: Amend the retained HKREFACT tasks to describe harness-native enforcement**

The current `arm amend` command does not rename titles. Keep existing graph titles as
historical labels; make the rewritten HKREFACT design and plan the canonical names for
T02, T07, T08, and T11.

Run:

```bash
go run ./cmd/armature amend HKREFACT-T02 --scope internal/verification --dod "A reusable verification service is available to explicit commands and harness hooks, but does not infer task completion or supervise harness execution."
go run ./cmd/armature amend HKREFACT-T07 --scope internal/harnesshook,cmd/armature/harness_hook.go --dod "Armature can generate or install native harness hook configuration without launching or supervising a harness."
go run ./cmd/armature amend HKREFACT-T08 --scope internal/harnesspolicy,internal/hooks --dod "Harness-native hooks and existing git pre-commit hooks share scope enforcement without a zero-trust commit flow."
go run ./cmd/armature amend HKREFACT-T11 --dod "Documentation and smoke tests demonstrate native harness hooks invoking Armature policy enforcement."
```

Expected: the four tasks no longer require `internal/orchestrate`, managed harness launch, engine completion inference, or zero-trust commit.

- [ ] **Step 4: Cancel the two HKREFACT tasks that require managed execution**

Run:

```bash
go run ./cmd/armature transition HKREFACT-T09 --to cancelled --outcome "Cancelled because Armature no longer owns a managed execution engine or completion inference."
go run ./cmd/armature transition HKREFACT-T10 --to cancelled --outcome "Cancelled because arm orchestrate and arm worker run are being removed."
```

Expected: `HKREFACT-T09` and `HKREFACT-T10` are `cancelled`.

- [ ] **Step 5: Verify retained HKREFACT scopes do not reference deleted modules**

Run:

```bash
go run ./cmd/armature list --parent HKREFACT --format json
go run ./cmd/armature validate --ci
```

Expected: retained open HKREFACT work describes policy, hook adapters, installation, docs, and verification without requiring managed execution. Validation may still report source-path findings that Task 5 will resolve, but must not report invalid hierarchy or lifecycle state.

- [ ] **Step 6: Record the graph migration checkpoint**

```bash
git check-ignore -v .arm/.armature/ops/codex-worker.log
git status --short
```

Expected: `.arm/` is ignored and no repository commit is created for this local state migration.

### Task 2: Remove Managed-Execution Records From Dogfood State

**Local state:**
- Modify: `.arm/.armature/ops/codex-worker.log`
- Modify: `.arm/.armature/ops/34dafe5f-6881-4bca-b209-c98c906e3550.log`
- Regenerate: `.arm/.armature/state/`

- [ ] **Step 1: Record exact pre-migration counts**

Run:

```bash
rg -c '^\["orchestrate-(start|dispatch|dispatch-complete|verify-fail|retry|escalate|complete|check-result)"|^\["worker-runtime-decision"' .arm/.armature/ops/*.log
```

Expected: only the two named logs contain removed operation types. Save the counts in the commit message or migration note.

- [ ] **Step 2: Remove only obsolete operation lines**

Use a structured line filter over only the two known logs:

```bash
perl -ni -e 'print unless /^\["(?:orchestrate-(?:start|dispatch|dispatch-complete|verify-fail|retry|escalate|complete|check-result)|worker-runtime-decision)"/' .arm/.armature/ops/codex-worker.log
perl -ni -e 'print unless /^\["(?:orchestrate-(?:start|dispatch|dispatch-complete|verify-fail|retry|escalate|complete|check-result)|worker-runtime-decision)"/' .arm/.armature/ops/34dafe5f-6881-4bca-b209-c98c906e3550.log
```

Expected: create, amend, claim, note, decision, source-link, citation, and ordinary transition records remain unchanged.

- [ ] **Step 3: Prove all removed records are gone**

Run:

```bash
rg -n '"(orchestrate-(start|dispatch|dispatch-complete|verify-fail|retry|escalate|complete|check-result)|worker-runtime-decision)"' .arm/.armature/ops
```

Expected: no matches.

- [ ] **Step 4: Re-materialize and verify historical tasks remain**

Run:

```bash
go run ./cmd/armature materialize
go run ./cmd/armature show ORCRUN --format json
go run ./cmd/armature show story-1778637796 --format json
go run ./cmd/armature validate --ci
go run ./cmd/armature doctor
```

Expected:
- Materialization succeeds.
- `ORCRUN` is cancelled.
- `story-1778637796` remains merged.
- No history was deleted except managed-execution operation records.

- [ ] **Step 5: Record the dogfood migration checkpoint**

```bash
git check-ignore -v .arm/.armature/ops/codex-worker.log .arm/.armature/state/issues.json
git status --short
```

Expected: migrated dogfood state remains local and ignored; no repository commit is created.

### Task 3: Remove Public Managed-Execution Commands

**Files:**
- Modify: `cmd/armature/main.go`
- Modify: `cmd/armature/main_test.go`
- Delete: `cmd/armature/orchestrate.go`
- Delete: `cmd/armature/orchestrate_test.go`
- Delete: `cmd/armature/orchestrate_delegate_test.go`
- Delete: `cmd/armature/worker.go`
- Delete: `cmd/armature/worker_run.go`
- Delete: `cmd/armature/worker_run_test.go`
- Retain: `cmd/armature/worker_init.go`

- [ ] **Step 1: Add command-removal contract tests**

Add table-driven cases to `cmd/armature/main_test.go`:

```go
func TestManagedExecutionCommandsAreNotRegistered(t *testing.T) {
	for _, args := range [][]string{{"orchestrate"}, {"worker"}, {"worker", "run"}} {
		root := newRootCmd()
		cmd, _, err := root.Find(args)
		require.Error(t, err)
		assert.Equal(t, root, cmd)
	}
}
```

- [ ] **Step 2: Run the contract test and observe failure**

Run:

```bash
go test ./cmd/armature -run TestManagedExecutionCommandsAreNotRegistered -count=1
```

Expected: FAIL because both commands are still registered.

- [ ] **Step 3: Remove command registration and implementation**

Delete the `newOrchestrateCmd()` and `newWorkerCmd()` registration blocks from `cmd/armature/main.go`. Delete the six managed-execution command and test files listed above. Keep the top-level `worker-init` command unchanged.

- [ ] **Step 4: Run command tests**

Run:

```bash
go test ./cmd/armature -count=1
go run ./cmd/armature orchestrate
go run ./cmd/armature worker run
go run ./cmd/armature worker-init --check
```

Expected:
- Package tests pass.
- The first two CLI invocations return unknown-command errors.
- `worker-init` remains registered and executes its existing check behavior.

- [ ] **Step 5: Commit the public API deletion**

```bash
git add cmd/armature
git commit -m "refactor(cli): remove managed execution commands"
```

### Task 4: Delete Managed-Execution Internals And Configuration

**Files:**
- Delete: `internal/orchestrate/`
- Delete: `internal/workerruntime/`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Remove obsolete config tests**

Delete `TestOrchestratorConfigRoundTrip` and `TestOrchestratorConfigAbsentKeyIsZeroValue` from `internal/config/config_test.go`. Do not add backward-compatibility parsing for the removed config because this is an intentional hard break.

- [ ] **Step 2: Add a regression test that default config has no orchestrator section**

Add to `internal/config/config_test.go`:

```go
func TestDefaultConfigHasNoOrchestratorSection(t *testing.T) {
	data, err := json.Marshal(DefaultConfig("go"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"orchestrator"`)
}
```

- [ ] **Step 3: Remove managed-execution config types**

From `internal/config/config.go`, remove:

```go
Orchestrator OrchestratorConfig `json:"orchestrator,omitempty"`
```

Also remove `OrchestratorConfig`, `AuthConfig`, and `AdapterCommands`. Keep `Hooks []HookConfig`; it is part of the retained repository hook model.

- [ ] **Step 4: Delete the managed-execution packages**

Delete every file under `internal/orchestrate/` and `internal/workerruntime/`.

- [ ] **Step 5: Verify no production Go code imports deleted packages**

Run:

```bash
rg -n 'internal/(orchestrate|workerruntime)|OrchestratorConfig|AuthConfig|AdapterCommands' --glob '*.go'
go test ./internal/config ./cmd/armature -count=1
go test ./... -count=1
```

Expected: no stale Go references and all tests pass.

- [ ] **Step 6: Commit internal deletion**

```bash
git add internal cmd/armature
git commit -m "refactor(core): delete managed execution runtime"
```

### Task 5: Remove Managed-Execution Operation Vocabulary

**Files:**
- Modify: `internal/ops/types.go`
- Modify: `internal/ops/types_test.go`
- Modify: `internal/materialize/engine.go`
- Modify: `internal/materialize/engine_test.go`

- [ ] **Step 1: Add regression coverage that removed operation types are invalid**

Add a table-driven test to `internal/ops/types_test.go`:

```go
func TestManagedExecutionOperationTypesAreInvalid(t *testing.T) {
	for _, opType := range []string{
		"orchestrate-start",
		"orchestrate-dispatch",
		"orchestrate-dispatch-complete",
		"orchestrate-verify-fail",
		"orchestrate-retry",
		"orchestrate-escalate",
		"orchestrate-complete",
		"orchestrate-check-result",
		"worker-runtime-decision",
	} {
		assert.False(t, ValidOpTypes[opType], opType)
	}
}
```

Add an engine test that applies one removed op string and asserts `unknown op type`.

- [ ] **Step 2: Run the tests and observe failure**

Run:

```bash
go test ./internal/ops ./internal/materialize -run 'ManagedExecution|UnknownOp' -count=1
```

Expected: FAIL because the old operations are still accepted or ignored.

- [ ] **Step 3: Delete constants, valid-op registrations, payload fields, and no-op materialization**

In `internal/ops/types.go`, delete:
- All nine managed-execution constants.
- Their `ValidOpTypes` entries.
- `FailureRecord` if `rg` confirms it has no retained caller.
- Payload fields used only by removed operations: `worktree_path`, `pre_dispatch_ref`, `retry_budget`, `run`, `correlation_id`, `causation_id`, and `decision_class`.

In `internal/materialize/engine.go`, delete the case that silently accepts managed-execution operations.

Do not remove unrelated `WorktreePath` fields from config/context types.

- [ ] **Step 4: Run focused and full tests**

Run:

```bash
go test ./internal/ops ./internal/materialize -count=1
go test ./... -count=1
go run ./cmd/armature materialize
```

Expected: tests and repository materialization pass, proving Task 2 removed all incompatible dogfood records.

- [ ] **Step 5: Commit vocabulary removal**

```bash
git add internal/ops internal/materialize
git commit -m "refactor(ops): remove managed execution vocabulary"
```

### Task 6: Rewrite Embedded Skills Around Explicit Workflows

**Files:**
- Modify: `internal/skillsembed/skills/armature/SKILL.md`
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md`
- Modify: `internal/skillsembed/skills/armature-worker/SKILL.md`
- Delete: `internal/skillsembed/skills/armature-orchestrator/SKILL.md`
- Modify: `cmd/armature/install_skills_test.go`

- [ ] **Step 1: Update installation tests for the new skill set**

Change skill installation assertions so:
- `armature`, `armature-coordinator`, and `armature-worker` are installed.
- `armature-orchestrator` is absent.
- Installed content contains no `arm orchestrate` or `arm worker run`.

- [ ] **Step 2: Run the focused test and observe failure**

Run:

```bash
go test ./cmd/armature -run TestInstallSkills -count=1
```

Expected: FAIL because the old orchestrator skill and managed-execution text still exist.

- [ ] **Step 3: Rewrite the general Armature skill**

Make `internal/skillsembed/skills/armature/SKILL.md` teach explicit operations only:

```text
arm ready
arm claim ID
arm render-context ID
arm note ID --message "..."
arm decision ID --message "..."
arm heartbeat ID
arm transition ID done --outcome "..."
arm validate --ci
arm doctor
```

State that harness execution is external and that native harness hooks may invoke Armature policy commands.

- [ ] **Step 4: Rewrite coordinator and worker skills**

The coordinator skill must:
- Inspect `arm ready`.
- Claim and render context explicitly.
- Dispatch an external worker or harness.
- Optionally activate native harness hooks.
- Never supervise via an Armature runtime loop.

The worker skill must:
- Treat explicit implementation, notes/decisions/heartbeats, repo verification, transition, and commit as the default workflow.
- Avoid “fallback” language.
- Explain that hook denials are policy enforcement from the DAG, not managed execution.

Delete the `armature-orchestrator` skill.

- [ ] **Step 5: Verify skill content and installation**

Run:

```bash
go test ./cmd/armature -run TestInstallSkills -count=1
rg -n 'arm orchestrate|arm worker run|armature-orchestrator|managed execution' internal/skillsembed
```

Expected: tests pass; stale workflow references have no matches except deliberate historical wording if a test fixture requires it.

- [ ] **Step 6: Commit the skill rewrite**

```bash
git add internal/skillsembed cmd/armature/install_skills_test.go
git commit -m "docs(skills): teach explicit coordinator and worker flows"
```

### Task 7: Archive Managed-Execution Designs And Rewrite Current Documentation

**Files:**
- Create: `docs/archive/orchestration/README.md`
- Move: obsolete managed-execution files from `docs/design/` to `docs/archive/orchestration/design/`
- Move: obsolete managed-execution files from `docs/superpowers/specs/` to `docs/archive/orchestration/specs/`
- Move: obsolete managed-execution files from `docs/superpowers/plans/` to `docs/archive/orchestration/plans/`
- Modify: `README.md`
- Modify: `docs/commands.md`
- Modify: `docs/getting-started.md`
- Modify: `docs/use-cases.md`
- Modify: `docs/provider-smoke-tests.md`
- Modify: `docs/adr/adr-context-files.md`
- Modify: `docs/superpowers/specs/2026-05-26-harness-hooks-guardrail-refactor-design.md`
- Modify: `docs/superpowers/plans/2026-05-26-harness-hooks-guardrail-refactor.md`
- Retain: `docs/superpowers/specs/2026-06-03-remove-managed-execution-design.md`
- Retain: `docs/superpowers/plans/2026-06-03-remove-managed-execution.md`

- [ ] **Step 1: Move obsolete managed-execution documents into the archive**

Create the three archive subdirectories and use `git mv` for these exact files.

Move to `docs/archive/orchestration/design/`:

```text
docs/design/orchestrate-audit-model.md
docs/design/orchestrate-exception-taxonomy.md
docs/design/orchestrate-policy-subworkflow-specification.md
docs/design/orchestrate-policy-subworkflows.md
docs/design/orchestrate-readiness-and-executability.md
docs/design/orchestrate-runtime-decisions.md
docs/design/orchestrate-runtime-direction.md
docs/design/orchestrate-runtime-gap-analysis.md
docs/design/orchestrate-runtime-index.md
docs/design/orchestrate-runtime-oss-review.md
docs/design/orchestrate-runtime-policy-model.md
docs/design/orchestrate-runtime-roadmap.md
docs/design/orchestrate-runtime-v1-scope.md
docs/design/orchestrate-worker-runtime-state-machine.md
docs/design/orchestration-run-contract.md
```

Move to `docs/archive/orchestration/specs/`:

```text
docs/superpowers/specs/2026-05-05-orchestrator-design.md
docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md
docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-decision-record-design.md
docs/superpowers/specs/2026-05-10-orchestrate-runtime-phase2-control-model-design.md
```

Move to `docs/archive/orchestration/plans/`:

```text
docs/superpowers/plans/2026-05-06-e7-orchestrate.json
docs/superpowers/plans/2026-05-06-orchestrator.md
docs/superpowers/plans/2026-05-09-orchestrate-runtime-phase1-boundary-note.md
docs/superpowers/plans/2026-05-09-orchestrate-runtime-phase1-decision-record.md
docs/superpowers/plans/2026-05-11-orchestrate-runtime-phase3-reconciliation.md
docs/superpowers/plans/2026-05-11-orchestrate-runtime-phase4-v1-slice.md
docs/superpowers/plans/2026-05-11-orchestrate-runtime-v1.md
docs/superpowers/plans/2026-05-29-orchestrate-auth-disclosure.md
docs/superpowers/plans/2026-06-02-orchestration-run-contract-plan.json
```

Keep the HKREFACT hook design and plan, the current removal design and plan, and
unrelated historical plans in place.

- [ ] **Step 2: Add the archive boundary README**

`docs/archive/orchestration/README.md` must state:
- These documents describe abandoned Armature-managed harness execution.
- They are retained as historical design evidence, not current behavior.
- Current direction is explicit skill-driven workflow plus optional harness-native policy hooks.
- Armature does not launch, supervise, retry, infer completion for, or commit on behalf of harnesses.

- [ ] **Step 3: Rewrite current user documentation**

Update README, command reference, getting started, and use cases to show:

```text
arm ready
arm claim <id>
arm render-context <id>
# dispatch the external worker/harness
arm note|decision|heartbeat ...
# run repository-owned verification
arm transition <id> done --outcome "..."
arm validate --ci
arm doctor
```

Remove managed-execution flags, auth/network disclosure, retries, queue draining, zero-trust commit, and completion inference from current docs.

- [ ] **Step 4: Rewrite hook-facing documents without closing HKREFACT**

Rewrite the HKREFACT design and plan in place around:
- Shared scope policy.
- Generic hook request/decision types.
- Platform-native hook adapters.
- Task policy resolution from the DAG.
- `arm harness-hook`.
- Harness hook installation/configuration.
- Shared enforcement with existing git hooks.
- Explicit verification service that does not infer completion.

Rewrite `docs/provider-smoke-tests.md` as a harness-hook smoke-test runbook, retaining the path if HKREFACT source links or scopes already reference it. Update `docs/adr/adr-context-files.md` only where it treats managed execution as current behavior.

- [ ] **Step 5: Scan current docs for stale claims**

Run:

```bash
rg -n 'arm orchestrate|arm worker run|internal/orchestrate|internal/workerruntime|zero-trust commit|runtime-owned execution' README.md docs --glob '!docs/archive/**'
```

Expected: no stale current-behavior claims. Matches in the approved removal design/plan may remain when they describe what is being deleted.

- [ ] **Step 6: Commit archive and documentation changes**

```bash
git add README.md docs
git commit -m "docs: archive managed execution direction"
```

### Task 8: Update Source Registrations For Archived Documents

**Local state:**
- Modify: `.arm/.armature/sources/manifest.json`
- Modify: `.arm/.armature/sources/`
- Modify: `.arm/.armature/ops/codex-worker.log` and historical logs containing moved source links

- [ ] **Step 1: Update retained source URLs without changing source IDs**

Edit `.arm/.armature/sources/manifest.json` entries for moved documents. Preserve
the IDs and replace each URL with this exact value:

```text
44fecba4-af85-4ad0-ae8c-1c41bc8a0765  docs/archive/orchestration/plans/2026-05-29-orchestrate-auth-disclosure.md
7a92f2d9-c184-4acd-9538-113368e39692  docs/archive/orchestration/specs/2026-05-05-orchestrator-design.md
87d9e29e-2063-46cc-b86d-089332ba8538  docs/archive/orchestration/plans/2026-05-06-e7-orchestrate.json
9542f8e2-ee3f-4bfb-b59d-dfa853fbf412  docs/archive/orchestration/design/orchestration-run-contract.md
b027c51e-3bb7-4af3-b424-d84fd9828666  docs/archive/orchestration/plans/2026-05-11-orchestrate-runtime-v1.md
```

Normalize the absolute `b027...` URL to the repository-relative archive path. Keep HKREFACT source `48bf4795-4493-49ae-b534-bbf4f29d3532` pointing to the rewritten in-place hook plan.

- [ ] **Step 2: Update retained source-link payload URLs**

In `.arm/.armature/ops/*.log`, replace only the `source_url` values associated
with those five source IDs using the same exact mapping above. Do not change
source IDs, targets, timestamps, workers, or any non-source-link records.

Run:

```bash
rg -n '"source_url":"(docs/(design|superpowers)/(plans|specs)|/home/brian/development/armature/docs/superpowers/plans)/[^"]*(orchestrat|worker-runtime)' .arm/.armature/ops
```

Expected: no source-link payload points at a moved pre-archive path.

- [ ] **Step 3: Synchronize and verify registered sources**

Run:

```bash
go run ./cmd/armature sources sync
go run ./cmd/armature sources verify
```

Expected: all retained source IDs resolve and fingerprints match their current or archived files.

- [ ] **Step 4: Verify task source links still resolve**

Run:

```bash
go run ./cmd/armature validate --ci
go run ./cmd/armature doctor
```

Expected: no missing-source or stale-source-path findings.

- [ ] **Step 5: Record the source migration checkpoint**

```bash
git check-ignore -v .arm/.armature/sources/manifest.json .arm/.armature/ops/codex-worker.log
git status --short
```

Expected: source registration and source-link migration remains local and ignored; no repository commit is created.

### Task 9: Full Verification And Stale-Surface Audit

**Files:**
- Modify only files required to resolve findings from these checks.

- [ ] **Step 1: Prove deleted Go surfaces are absent**

Run:

```bash
test ! -e cmd/armature/orchestrate.go
test ! -e cmd/armature/worker_run.go
test ! -d internal/orchestrate
test ! -d internal/workerruntime
rg -n 'newOrchestrateCmd|newWorkerRunCmd|OpOrchestrate|OpWorkerRuntimeDecision|OrchestratorConfig|AuthConfig|AdapterCommands' --glob '*.go'
```

Expected: all `test` commands pass and `rg` has no matches.

- [ ] **Step 2: Prove removed CLI and op vocabulary are absent from current surfaces**

Run:

```bash
rg -n 'arm orchestrate|arm worker run|armature-orchestrator' README.md docs internal/skillsembed --glob '!docs/archive/**' --glob '!docs/superpowers/specs/2026-06-03-remove-managed-execution-design.md' --glob '!docs/superpowers/plans/2026-06-03-remove-managed-execution.md'
rg -n '"(orchestrate-(start|dispatch|dispatch-complete|verify-fail|retry|escalate|complete|check-result)|worker-runtime-decision)"' .arm/.armature/ops
```

Expected: no matches.

- [ ] **Step 3: Run repository verification**

Run:

```bash
gofmt -w cmd/armature/main.go cmd/armature/main_test.go internal/config/config.go internal/config/config_test.go internal/ops/types.go internal/ops/types_test.go internal/materialize/engine.go internal/materialize/engine_test.go
go test ./... -count=1
make check
```

Expected: all tests and checks pass.

- [ ] **Step 4: Run focused retained-workflow verification**

Run:

```bash
go test ./cmd/armature -run 'Test(ReadyCommand|ClaimCommand|RenderContextCommand|HeartbeatCommand|NoteCommand|DecisionCommand|TransitionCommand|Merged_)' -count=1
go test ./cmd/armature -run 'TestHookRun' -count=1
```

Expected: explicit DAG workflow and retained git hook tests pass after managed-execution deletion.

- [ ] **Step 5: Run Armature verification**

Run:

```bash
go run ./cmd/armature materialize
go run ./cmd/armature sources verify
go run ./cmd/armature validate --ci
go run ./cmd/armature doctor
```

Expected: all commands exit zero. `arm doctor` must not report lifecycle/commit mismatches introduced by this transition.

- [ ] **Step 6: Review final diff against the approved boundary**

Run:

```bash
git status --short
git diff --stat 7cbd509..HEAD
git diff 7cbd509..HEAD -- docs/superpowers/specs/2026-06-03-remove-managed-execution-design.md
```

Confirm:
- Armature does not execute or supervise harnesses.
- The explicit coordinator/worker workflow remains usable.
- HKREFACT retains a path to harness-native enforcement.
- Historical tasks and archived documents remain available.
