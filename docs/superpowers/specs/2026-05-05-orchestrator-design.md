# Orchestrator Design Spec

**Date:** 2026-05-05
**Status:** Draft

---

## Overview

Armature gains a deterministic orchestration layer: `arm orchestrate`. It wraps agent harness invocations inside a state machine that enforces scope boundaries, test existence, build health, lint, code coverage, and (optionally) mutation testing — all verified externally, without trusting the agent's self-reporting. All orchestration state is persisted as new op types in the existing JSONL event log, giving crash-resilient resume and a full audit trail in git.

---

## Motivation

Existing Armature skills (coordinator, worker) rely on LLM cooperation. Agents do not always follow instructions: they commit when told not to, modify files outside declared scope, skip tests, or produce untestable code. A cooperative protocol is only as good as agent compliance, which is unreliable. The orchestrator enforces constraints deterministically — the agent has no vote on whether its work passes.

---

## Prerequisites

This feature requires the following additions to existing types before implementation:

- `Payload.PreferredModel string` (`json:"preferred_model,omitempty"`) — added to `internal/ops/types.go` `Payload` struct
- `Issue.PreferredModel string` — added to the materialized `Issue` struct, populated from the `create` op payload
- `Config.Orchestrator OrchestratorConfig` — new sub-struct added to the config, populated from `.arm/config.json`
- Citations check — the orchestrator implements a stricter variant than the existing `validate.go` `checkE7E8E12Citations`. The existing check verifies at least one source link or citation exists and that source entry IDs resolve in the manifest. The orchestrator's citations check additionally verifies that a `citation-accepted` op exists for every source linked via `source-link` to the task. This stricter check lives in the orchestrator's verification pipeline, not in `validate.go`. To support per-source correlation, `CitationAcceptance` (in `internal/materialize/engine.go`) must be extended with a `SourceEntryID string` field, and the `citation-accepted` op payload must include `"source_entry_id"`. This is a required schema addition before implementing the citations check.

---

## Command Interface

```
arm orchestrate <task-id> \
  --harness <claude|codex|devin> \
  [--model <model-id>] \
  [--retries N]          # default: 3 \
  [--timeout <duration>] # default: 10m, per-dispatch
```

Exits zero on success, non-zero on escalation or preflight rejection.

---

## State Machine

Nine named states. `staged` and `rollback` are transitional states between `dispatched` and `verifying`; they do not have their own event log ops but are described here for clarity. States with event log ops are: `idle`, `dispatched`, `verifying`, `correcting`, `escalated`, and `done`.

```
preflight → idle → dispatched → staged → verifying → done
          ↘          ↘ rollback ↗        ↘ correcting → dispatched (retry)
          rejected                        ↘ escalated
```

### State Descriptions

**preflight** — validate the task is executable before any agent is invoked:
- Scope paths declared on the task exist in the repo
- Acceptance criteria are present and in structured assertion format (see Acceptance Criteria)
- Source citations resolve against the manifest
- Rendered context is within token budget

Failure writes a `note` op with a structured rejection reason and re-opens the task. No orchestration ops are written.

**idle** — `arm claim` internal logic is called (not shelled out) to write the `claim` op and enforce branch discipline. Worktree created at `.worktrees/<task-id>`. `orchestrate-start` written.

**dispatched** — the pre-dispatch HEAD SHA of the worktree is recorded as `pre-dispatch-ref`. Harness process launched in the worktree. `orchestrate-dispatch` written (with `run` = current 1-indexed run number). Orchestrator blocks on process exit or timeout. When the process exits, `orchestrate-dispatch-complete` is written immediately before any further state transitions.

**staged** — transitional. After `orchestrate-dispatch-complete` is written: `git diff <pre-dispatch-ref>` captures the full accumulated diff; `git reset --hard <pre-dispatch-ref>` resets the worktree; diff is re-applied as unstaged changes. If the reset or re-apply fails, transitions to `rollback`.

**rollback** — transitional. The worktree is in an inconsistent state (reset or diff re-apply failed). The worktree is removed and re-created from the pre-dispatch ref. Treated as a verification failure for retry purposes — `orchestrate-verify-fail` is written with `check: "rollback"`, then follows the normal correcting path.

**verifying** — verification pipeline runs against the staged diff and worktree. Each failing check writes `orchestrate-verify-fail`. If all checks pass, transitions to `done`.

**correcting** — one or more checks failed. Structured feedback assembled. `orchestrate-retry` written with `run` = the upcoming dispatch run number. Re-enters `dispatched`.

**escalated** — retry budget exhausted. `orchestrate-escalate` written. Task transitioned to `blocked` with structured outcome note. Orchestrator exits non-zero.

**done** — all checks passed. The orchestrator is operating inside the task's worktree (on the task branch), so branch discipline checks in `arm transition` pass without `--force`. Sequence: (1) stage and commit enriched message, (2) call `arm transition <task-id> --to done` with outcome note, (3) write `orchestrate-complete` to the log. Writing `orchestrate-complete` last means a crash between step 1 and step 3 leaves the log without `orchestrate-complete` — on resume the last op is `orchestrate-verify-fail` or similar, so the orchestrator will re-enter verifying. Since the diff was already committed, the re-applied diff will produce an empty or no-op diff, which passes all checks and completes cleanly. Orchestrator exits zero.

### Crash Resume

State is derived by replaying the task's event log to the last orchestration op. The `run` field in all orchestration ops is 1-indexed and represents the current run in progress at the time the op was written (except `orchestrate-retry`, where `run` is the upcoming run number to be dispatched next).

| Last op | Resume state |
|---|---|
| `orchestrate-start` | Re-dispatch run 1 (process assumed dead) |
| `orchestrate-dispatch` | Re-dispatch same run (process assumed dead) |
| `orchestrate-dispatch-complete` | Re-run staged → verifying |
| `orchestrate-verify-fail` | Verify worktree exists at path in `orchestrate-start` (re-create from `pre-dispatch-ref` if absent), then assemble feedback and enter correcting |
| `orchestrate-retry` | Re-dispatch at `run` number from op |
| `orchestrate-escalate` | Terminal — surface to human, no re-entry |
| `orchestrate-complete` | Terminal — already done |

---

## Harness Adapter Interface

```go
type HarnessAdapter interface {
    Name() string
    Invoke(ctx context.Context, workdir string, prompt string) (InvocationResult, error)
}

type InvocationResult struct {
    ExitStatus ExitStatus
    Stdout     string
    Stderr     string
    DurationMs int64
}

type ExitStatus int
const (
    ExitClean   ExitStatus = iota
    ExitTimeout
    ExitError
)

type HarnessConfig struct {
    Adapter string        // "claude" | "codex" | "devin"
    Model   string        // optional model override
    Timeout time.Duration
}
```

### Sandbox-Based Permission Model

All three harnesses support OS-level sandboxing (Seatbelt on macOS, bubblewrap on Linux/WSL2). The orchestrator runs each dispatch inside a sandbox configured to restrict writes to the worktree path. This allows `--dangerouslySkipPermissions` (or equivalent) without risk — the OS enforces the actual boundaries; the agent cannot exceed them regardless of what it attempts.

The sandbox doubles as a second scope enforcement layer: the OS physically prevents writes outside the configured paths, complementing the verification pipeline's `scope-boundary` check. Both are kept — the sandbox prevents violations; the check generates precise feedback when violations are attempted.

Before each dispatch, the orchestrator writes harness-specific config into the worktree:

**Claude Code** — writes `.claude/settings.json`:
```json
{
  "sandbox": {
    "enabled": true,
    "failIfUnavailable": true,
    "filesystem": {
      "allowWrite": ["<scope-paths>"],
      "denyWrite": ["../"]
    }
  }
}
```
Invoked as: `claude --dangerouslySkipPermissions --model <model> -p "<prompt>"`

**Codex** — writes `config.toml` in worktree:
```toml
sandbox_mode = "workspace-write"
approval_policy = "never"
[permissions.default.filesystem]
writable_roots = ["<scope-paths>"]
```
Invoked as: `codex --model <model> "<prompt>"`

**Devin** — writes `.devin/config.json` in worktree pre-granting write access to declared scope paths (required because Devin's `edit`/`write` tools run in the CLI process, not the OS sandbox, so they still require explicit permission grants):
```json
{
  "permissions": [
    { "allow": "Write(<scope-glob>)" }
  ]
}
```
Invoked as: `devin --sandbox --permission-mode autonomous -- "<prompt>"` (model selection not yet supported by Devin CLI — `Model` field is ignored and a warning is logged)

Sandbox availability is checked at preflight (`failIfUnavailable: true` equivalent). If the sandbox cannot be initialized (e.g., bubblewrap not installed, WSL1), `arm orchestrate` fails at preflight with a clear error. Running without a sandbox is not supported.

### Invocation per Harness

| Harness | Command |
|---|---|
| Claude Code | `claude --dangerouslySkipPermissions --model <model> -p "<prompt>"` |
| Codex | `codex --model <model> "<prompt>"` |
| Devin | `devin --sandbox --permission-mode autonomous -- "<prompt>"` |

The adapter's sole responsibility: write harness config into worktree, spawn the process in `workdir`, stream stdout/stderr to `.arm/orchestration/<task-id>/run-<N>.log`, return a normalized `InvocationResult`. Prompt assembly is handled entirely by the orchestrator before the adapter is called.

### Prompt Assembly

The orchestrator assembles the full prompt before handing it to the adapter:

1. Rendered task context from `arm render-context`
2. Explicit scope constraints (declared scope file paths)
3. Retry feedback block, written to a temp file and passed via stdin where the harness supports it — not inline in the shell invocation, to avoid Unicode encoding issues in shell arguments (the feedback block uses `✓`/`✗` characters)
4. No-commit instruction (best-effort; enforcement is structural via zero-trust commit handling, not cooperative)

### Zero Trust Commit Enforcement

The orchestrator never trusts the agent's commit behavior. After every dispatch:

1. `git diff <pre-dispatch-ref>` captures the full accumulated diff regardless of how many commits the agent made
2. `git reset --hard <pre-dispatch-ref>` resets the worktree to the known-good baseline
3. Diff is re-applied as unstaged changes (staged state)
4. After verification passes, the orchestrator stages and commits with the enriched message

The agent's commit history is always discarded. The diff is the only trusted artifact from each dispatch.

### Model Selection

Resolved in priority order:

1. `--model` flag on `arm orchestrate`
2. `PreferredModel` field on the task's `Issue` struct (populated from `preferred_model` in the `create` op payload at decompose time)
3. `OrchestratorConfig.DefaultModel` from `.arm/config.json`

Well-scoped tasks with deterministic acceptance criteria are good candidates for cheaper/faster models (e.g., `claude-haiku-4-5`, Codex no-reasoning). The orchestrator's guardrails compensate for reduced reasoning capability.

---

## Verification Pipeline

An ordered sequence of checks run against the staged diff and worktree after each dispatch. Each check produces `pass | warn | fail` plus a structured reason used in feedback injection. Hard fails stop the pipeline and trigger correcting state. Warnings accumulate in the report but do not block.

```
scope-boundary → build → lint → test-existence → coverage → mutation (optional) → acceptance-criteria → citations
```

### Checks

**scope-boundary** — `git diff --name-only` vs declared scope paths. Any modified file outside scope is a hard fail.

**build** — language adapter runs build command on worktree. Exit code is the signal. Hard fail.

**lint** — language adapter runs linter. Fail mode is configurable: `"hard"` (default) or `"warn"`, set via `OrchestratorConfig.LintFailMode`.

**test-existence** — for each modified source file, at least one matching test file must be added or modified in the same diff. Test file patterns are declared per language in the config as globs:

```json
"test_patterns": {
  "go": ["**/*_test.go"],
  "python": ["**/test_*.py", "**/*_test.py"],
  "javascript": ["**/*.spec.js", "**/*.test.js"]
}
```

Hard fail if no matching test file found for any modified source file.

**coverage** — language adapter runs coverage tool scoped to in-scope packages, parses output, compares against threshold. Threshold is resolved in priority order: (1) `coverage-gte` acceptance criterion on the task, (2) `OrchestratorConfig.CoverageThreshold` global default. Hard fail on all runs — no advisory grace period.

**mutation testing** — optional. Enabled via `OrchestratorConfig.MutationTesting: true`. Language adapter runs mutation tool (e.g., `gremlins` for Go, `stryker` for JS) scoped to in-scope files. Fail mode configurable via `OrchestratorConfig.MutationFailMode`: `"warn"` (default when enabled) or `"hard"`. Recommended for agent-generated test suites where hollow assertions are common.

**acceptance-criteria** — structured assertions evaluated externally. Supported assertion types:

| Type | Example |
|---|---|
| `file-exists` | `internal/dag/validate.go` |
| `test-passes` | `TestDAGCycleDetection` |
| `coverage-gte` | `internal/dag >= 85%` (also overrides global coverage threshold) |
| `lint-clean` | `internal/dag/...` |
| `no-scope-violation` | implicit, always present |

Tasks with prose-only acceptance criteria are rejected at preflight. Tasks with a mix may mark prose items as `unverifiable` — these pass through as reviewer notes in the commit message but do not gate the pipeline.

**citations** — "required citations" are all sources linked to the task via `source-link` ops. The check replays the event log and hard fails if any `source-link`-associated source lacks a corresponding `citation-accepted` op (correlated by `source_entry_id`). This is stricter than the existing `validate.go` check, which only requires at least one citation to exist — see Prerequisites for the schema addition required to support per-source correlation.

### Language Adapters

Build, lint, coverage, and mutation checks are language-specific. Each is a command string in the config, run against the worktree path:

```json
"adapters": {
  "build": "go build ./...",
  "lint": "golangci-lint run ./...",
  "coverage": "go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out",
  "mutation": "gremlins unleash ./..."
}
```

---

## Orchestrator-Controlled Commits

After all checks pass, the orchestrator makes the single canonical commit. Commit message format:

```
<type>(<task-id>): <task-title>

Spec: <link to source spec document from task citations>

Context:
<rendered context summary from arm render-context — requirements, acceptance criteria, scope>

Checks passed: scope-boundary, build, lint, test-existence, coverage, acceptance-criteria, citations

Co-Authored-By: <harness-name> (<model-id>)
```

Agent-generated commits in the worktree are discarded by the zero-trust reset. The orchestrator's commit is the only record. Run logs are committed to `.arm/orchestration/<task-id>/` alongside the task changes so reviewers can inspect exactly what the agent produced.

---

## Event Log Schema

Eight new op types. All carry `TargetID` (task ID), `WorkerID`, and `Timestamp` on the outer `Op` struct per the existing pattern. The `Payload` fields shown below are the only additions; the outer fields are unchanged.

The `run` field in `orchestrate-dispatch`, `orchestrate-dispatch-complete`, and `orchestrate-verify-fail` is the 1-indexed current run number. In `orchestrate-retry`, `run` is the upcoming (next) dispatch run number.

```jsonc
// orchestrate-start
{ "harness": "claude", "model": "claude-haiku-4-5",
  "retry_budget": 3, "worktree": ".worktrees/task-1234" }

// orchestrate-dispatch
{ "run": 1, "pre_dispatch_ref": "abc123", "prompt_hash": "sha256:..." }

// orchestrate-dispatch-complete
{ "run": 1, "exit_status": "clean", "duration_ms": 42000,
  "log_path": ".arm/orchestration/task-1234/run-1.log" }

// orchestrate-verify-fail
{ "run": 1, "check": "scope-boundary", "severity": "fail",
  "reason": "internal/auth/token.go modified, outside declared scope internal/dag/" }

// orchestrate-retry
{ "run": 2, "feedback_summary": "scope violation on run 1: ..." }

// orchestrate-escalate
{ "total_runs": 3,
  "failures": [
    { "run": 1, "check": "scope-boundary", "reason": "..." },
    { "run": 2, "check": "test-existence", "reason": "..." },
    { "run": 3, "check": "test-existence", "reason": "..." }
  ]}

// orchestrate-complete — written after arm transition --to done succeeds
{ "run": 2, "commit_sha": "def456",
  "checks_passed": ["scope-boundary","build","lint","test-existence","coverage","acceptance-criteria","citations"] }

// orchestrate-verify-fail (rollback variant)
{ "run": 1, "check": "rollback", "severity": "fail",
  "reason": "git reset --hard failed: <error detail>" }
```

---

## Retry & Escalation Logic

### Retry Budget

Default 3, overridable via `--retries N`. Each dispatch consumes one slot regardless of how many checks fail in that run.

### Feedback Assembly

When checks fail, the orchestrator assembles a structured feedback block written to a temp file and passed to the next harness invocation. Example:

```
CORRECTION REQUIRED (run 1 of 3 failed):

  ✗ scope-boundary: internal/auth/token.go was modified — declared scope is internal/dag/ only
  ✗ test-existence: no test file found covering internal/dag/validate.go
  ✓ build: passed
  ✓ lint: passed

Correct only what is listed above. Do not modify files outside internal/dag/.
Required test pattern for this repo: **/*_test.go
```

Prior runs are summarized in one line ("run 1: scope violation") to avoid prompt bloat. The feedback block is regenerated fresh from the current run's failures each retry — prior full failure reports are dropped.

### Progressive Constraint Tightening

Each retry adds one additional explicit constraint:

| Retry | Additional constraint |
|---|---|
| 1 | Failure report only |
| 2 | Explicit negative: "do not modify files outside `<scope>`" |
| 3 | Named file list: "the following files require test coverage: `<paths>`" |

### Escalation

When retry budget is exhausted:

1. `orchestrate-escalate` written to log with full failure history across all runs
2. `arm transition <task-id> --to blocked` called with structured outcome note containing all run summaries
3. Orchestrator exits non-zero so calling processes (CI, coordinator) can detect and route

The escalation note is human-readable: all run summaries, which checks failed, what feedback was injected each time.

---

## Escalation Runbook

When a task reaches `blocked` via orchestrator escalation, the structured escalation note in the event log identifies the root cause. Read it with `arm show <task-id>`. Common patterns and recovery actions:

### scope-boundary failures (all runs)

**What it means:** The agent repeatedly modified files outside the declared scope, even after explicit negative constraints were injected.

**Look for:** Is the scope too narrow for the actual work required? Does the task have implicit dependencies on shared utilities outside its declared scope?

**Recovery:** Re-open the task and use `arm amend` to widen the scope. Alternatively, decompose — create a prerequisite task scoped to the shared file, then re-queue this task as a dependent.

### test-existence failures (all runs)

**What it means:** The agent produced no test files matching the configured patterns for modified source files.

**Look for:** Check `test_patterns` in `.arm/config.json` — are they correct for this language/framework? Check run logs at `.arm/orchestration/<task-id>/run-N.log` to see if the agent attempted tests under a different naming convention.

**Recovery:** If patterns are wrong, fix the config and re-queue. If the task acceptance criteria are underspecified, add explicit `test-passes` assertions naming the required test functions.

### coverage failures (all runs)

**What it means:** Test files exist but coverage of in-scope packages fell below the configured threshold.

**Look for:** Is the threshold realistic for the scope of change? Check run logs to see if the agent's tests are exercising the right code paths.

**Recovery:** If the global threshold is too strict for this task, add a `coverage-gte` acceptance criterion with a task-specific threshold (this overrides the global default). If tests appear hollow, enable mutation testing to gate future runs. If the code is genuinely hard to cover, the task may need decomposition.

### build or lint failures (all runs)

**What it means:** The agent produced code that does not compile or fails the linter across all retries.

**Look for:** Run logs contain full build/lint output. Is this a complex refactor at the edge of the model's capability?

**Recovery:** Retry with a more capable model via `arm orchestrate --model <model>`. Decompose the task into smaller units. If a lint rule is legitimately inapplicable, evaluate whether to suppress it with justification or adjust the task scope.

### acceptance-criteria failures

**What it means:** One or more structured assertions were not satisfied after all retries.

**Look for:** Are the assertions correct? Did a `file-exists` assertion name a file that legitimately will not exist after this change? Use `arm render-context <task-id>` to inspect the full acceptance criteria.

**Recovery:** Use `arm amend` to correct malformed assertions. If the task is too large to satisfy all criteria in one pass, decompose.

### mixed failures across runs

**What it means:** Different checks fail on different runs — the agent is partially correcting but introducing new violations.

**Look for:** Is the scope too large for the model tier? Is the task decomposed at the right granularity?

**Recovery:** Increase `--retries` budget or switch to a more capable model. If the task has been retried at multiple model tiers and still fails, decompose — the task may require more context than the harness can handle in a single pass.

---

## Configuration Reference

All orchestrator configuration lives under an `"orchestrator"` key in `.arm/config.json`, extending the existing `Config` struct with an `Orchestrator OrchestratorConfig` sub-struct.

```json
{
  "orchestrator": {
    "default_harness": "claude",
    "default_model": "claude-haiku-4-5",
    "default_retries": 3,
    "default_timeout": "10m",

    "language": "go",
    "coverage_threshold": 80,
    "lint_fail_mode": "hard",

    "mutation_testing": false,
    "mutation_fail_mode": "warn",

    "adapters": {
      "build": "go build ./...",
      "lint": "golangci-lint run ./...",
      "coverage": "go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out",
      "mutation": "gremlins unleash ./..."
    },

    "test_patterns": {
      "go": ["**/*_test.go"],
      "python": ["**/test_*.py", "**/*_test.py"],
      "javascript": ["**/*.spec.js", "**/*.test.js"]
    }
  }
}
```

---

## Resolved Design Decisions

1. **`--dry-run` flag** — yes, supported. Runs preflight and prints the assembled prompt without dispatching the harness. Useful for inspecting what the agent would receive before committing a run.

2. **Escalation target** — always escalates to human. No automatic re-queue to a different harness. The structured escalation note provides enough signal for a human to decide the right recovery action.

3. **Run logs** — local-only. `.arm/orchestration/<task-id>/` is gitignored. Logs are available for inspection during and after a run but are not committed. This avoids bloating the repo with potentially large agent output.

4. **`preferred_model` field** — set at task creation time only (`arm decompose-apply`). Not settable via `arm amend`. To override per-invocation, use `arm orchestrate --model <model>`.

5. **Framework auto-detection** — the orchestrator probes the repo for well-known framework indicators (`go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, etc.) and pre-populates test patterns and adapter commands from a built-in registry. `.arm/config.json` is an override layer, not a requirement. If a framework is detected and no config override exists, the built-in defaults apply. If the framework cannot be detected, `arm orchestrate` fails at preflight with a clear message asking the user to add an `orchestrator` block to `.arm/config.json`.

---

## Relationship to Existing Skills

### armature-worker

The orchestrator fully replaces the `armature-worker` skill for task execution. The worker skill instructs an LLM how to claim, implement, and transition a task. The orchestrator does all of this deterministically — no LLM judgment is involved in driving the task lifecycle. The worker skill is deprecated for repos using the orchestrator.

### armature-coordinator

The coordinator skill is not immediately replaced but becomes optional for standard workflows. The coordinator currently uses LLM judgment for: finding ready tasks, determining parallelism, dispatching workers, integrating work, and closing stories with PRs. All of these are deterministic given the existing DAG.

#### Pull model replaces push dispatch

The coordinator skill uses a **push model**: an LLM surveys the DAG, decides which tasks to run, and assigns them to subagents. The orchestrator enables a **pull model** instead: multiple `arm orchestrate` instances run concurrently, each polling `arm ready` for the next available task, claiming it, executing it, and immediately pulling the next one. No central coordinator needed.

The pull model is architecturally superior for this workload:

- **Backpressure resilience** — orchestrators naturally idle when no tasks are ready (waiting on dependencies) rather than requiring a coordinator to track and schedule the dependency-unblocking sequence
- **Crash resilience** — a crashed orchestrator releases no work; unclaimed tasks remain in the queue and are picked up by the next orchestrator to poll
- **Horizontal scaling** — throughput scales by adding orchestrator processes; shedding load is removing them
- **No central bottleneck** — the coordinator LLM is a single point of failure and requires reasoning about DAG state that the DAG already encodes

Parallel wave execution happens naturally: when task A completes and unblocks B, C, and D, the next three orchestrators to call `arm ready` claim them immediately. No coordinator needed to plan the wave.

**Claim collision** is handled by the existing MRDT claim mechanism: two orchestrators polling simultaneously both see the same task as ready, but the first `claim` op written wins. The second detects the conflict on log replay and polls again for the next available task.

**Model routing** is handled per-task via `preferred_model`, not by a coordinator assigning tasks to agents by capability. Each task carries its own routing preference; the orchestrator reads it at claim time.

A future `arm coordinate <story-id>` command encapsulates the pull model: start N orchestrator processes pointed at a story, let them pull until all tasks are done, then open a PR from the story outcomes. The coordinator skill is retained as an escape hatch for edge cases requiring LLM judgment (unusual DAG shapes, partial failures needing narrative summaries). It is no longer the default execution path for repos using the orchestrator.
