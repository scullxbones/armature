# Armature Rename Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the project from Trellis to Armature across every layer — OS path, Go module, CLI binary, data directory, branch names, git config keys, environment variables, skills, and all user documentation.

**Architecture:** This is a pure rename with no logic changes. It is structured in four chunks so each chunk leaves the codebase in a buildable, testable state: (1) structural renames that affect compilation, (2) runtime string constants governed by TDD, (3) skill revamp, (4) documentation sweep. Each chunk ends with `make check` green before moving on.

**Tech Stack:** Go, Cobra CLI, Make, git worktrees, bash hook templates, Markdown skills (Claude Code / Gemini CLI).

---

## Rename Map (reference for the whole plan)

| Before | After |
|--------|-------|
| OS dir `/home/brian/development/trellis/` | `/home/brian/development/armature/` |
| Go module `github.com/scullxbones/trellis` | `github.com/scullxbones/armature` |
| Go cmd package `cmd/trellis/` | `cmd/armature/` |
| Binary `trls` / `bin/trls` / `~/.local/bin/trls` | `arm` / `bin/arm` / `~/.local/bin/arm` |
| Root command `Use: "trls"` | `Use: "arm"` |
| Data dir `.issues/` | `.armature/` |
| Git config key `trellis.mode` | `armature.mode` |
| Git config key `trellis.ops-worktree-path` | `armature.ops-worktree-path` |
| Git config key `trellis.worker-id` | `armature.worker-id` |
| Ops branch `_trellis` | `_armature` |
| Ops worktree default path `.trellis/` | `.arm/` |
| Env var `TRLS_LOG_SLOT` | `ARM_LOG_SLOT` |
| Skill `trls` | `armature` |
| Skill `trls-worker` | `armature-worker` |
| Skill `trls-coordinator` | `armature-coordinator` |
| Skill `trls-planner` | `armature-planner` |
| Skill `trls-auditor` | `armature-auditor` |
| Product name "Trellis" | "Armature" |

> **Note on worktree path:** In dual-branch mode, the ops worktree was created at `.trellis/` by `trls init`. After rename, `arm init --dual-branch` will create it at `.arm/`. Data inside the worktree: `.arm/.armature/`. The current repo uses single-branch mode; `.trellis/` is a stale worktree that will be removed in Task 7.

---

## Chunk 1: Structural Renames (Compilation)

These tasks must be done first — nothing compiles until the module path and package directory are consistent.

### Task 1: Rename the OS directory

**Files:** n/a — OS-level rename before any edits.

- [ ] **Step 1: Rename directory**

```bash
mv /home/brian/development/trellis /home/brian/development/armature
cd /home/brian/development/armature
```

- [ ] **Step 2: Verify git is intact**

```bash
git status
git log --oneline -3
```

Expected: clean working tree, no errors. Git follows the directory transparently.

- [ ] **Step 3: Commit nothing yet** — commit after Task 3 when the build is green.

---

### Task 2: Rename cmd package directory

**Files:**
- Rename: `cmd/trellis/` → `cmd/armature/`

- [ ] **Step 1: Rename**

```bash
git mv cmd/trellis cmd/armature
```

- [ ] **Step 2: Verify no broken symlinks**

```bash
ls cmd/armature/
```

Expected: all `.go` files present, no errors.

---

### Task 3: Update Go module path and all imports

**Files:**
- Modify: `go.mod` (line 1)
- Modify: every `*.go` file that imports `github.com/scullxbones/trellis/...`

- [ ] **Step 1: Update go.mod**

Change line 1 of `go.mod`:
```
module github.com/scullxbones/armature
```

- [ ] **Step 2: Bulk-replace all import paths**

```bash
find . -name "*.go" -not -path "./.git/*" -not -path "./.claude/worktrees/*" \
  -exec sed -i 's|github.com/scullxbones/trellis|github.com/scullxbones/armature|g' {} +
```

- [ ] **Step 3: Verify the build**

```bash
go build ./...
```

Expected: exits 0. If any import errors remain, grep for the old path:
```bash
grep -r "scullxbones/trellis" --include="*.go" .
```

- [ ] **Step 4: Run tests (sanity only — full make check after chunk)**

```bash
go test ./... 2>&1 | tail -20
```

Expected: tests run (some may fail on string literals — that's fine, those are fixed in Chunk 2).

- [ ] **Step 5: Commit**

```bash
git add go.mod cmd/armature/ $(git diff --name-only)
git commit -m "refactor: rename Go module and cmd package to armature"
```

---

### Task 4: Rename binary in Makefile and root command

**Files:**
- Modify: `Makefile`
- Modify: `cmd/armature/main.go`

- [ ] **Step 1: Update Makefile**

In `Makefile`, make these replacements:
- `"Trellis Go build targets:"` → `"Armature Go build targets:"`
- `make build      - Build CLI binary to ./bin/trls` → `make build      - Build CLI binary to ./bin/arm`
- `make install    - Build binary and install to ~/.local/bin/trls` → `make install    - Build binary and install to ~/.local/bin/arm`
- `-o bin/trls` → `-o bin/arm`
- `cp bin/trls ~/.local/bin/trls` → `cp bin/arm ~/.local/bin/arm`
- `chmod +x ~/.local/bin/trls` → `chmod +x ~/.local/bin/arm`
- `"Installed trls to ~/.local/bin/trls"` → `"Installed arm to ~/.local/bin/arm"`
- `"Ensure ~/.local/bin is on your PATH"` stays
- `./cmd/trellis` → `./cmd/armature` (in the build target)

- [ ] **Step 2: Update root command in cmd/armature/main.go**

```go
root := &cobra.Command{
    Use:   "arm",
    Short: "Armature — git-native work orchestration",
    ...
}
```

- [ ] **Step 3: Sweep product-name string literals across all cmd/armature/*.go (TDD)**

Many command files contain `Long:` help strings, error messages, and `Short:` descriptions with `trls` or `Trellis` that are user-visible. Do this TDD-style.

First, find every affected test assertion:
```bash
grep -rn '"trls \|"Trellis\|trls version\|trls hook\|trls init\|trls decompose\|trls worker-init' \
  cmd/armature/*_test.go | head -20
```

Update those test assertions first (e.g. `assert.Contains(t, buf.String(), "arm version")`, `assert.Contains(t, content, "_armature")` in hook tests). Run to confirm FAIL:

```bash
go test ./cmd/armature/... 2>&1 | grep -E "FAIL|--- FAIL" | head -20
```

Then bulk-replace in source files:
```bash
sed -i \
  -e 's/\btrls\b/arm/g' \
  -e 's/Trellis/Armature/g' \
  cmd/armature/*.go
```

Verify the build still compiles (sed may affect import paths — check with `go build ./cmd/armature/`). If import paths are corrupted, the module path replacement already handled them; revert only those lines.

Run tests to confirm PASS:
```bash
go test ./cmd/armature/...
```

- [ ] **Step 4: Build and verify binary name**

```bash
make build
./bin/arm --help
```

Expected: `Usage: arm [command]` with Armature in the description, and all subcommand help text shows `arm <subcommand>`.

- [ ] **Step 5: Commit**

```bash
git add Makefile cmd/armature/
git commit -m "refactor: rename CLI binary from trls to arm and update all user-visible strings"
```

---

## Chunk 2: Runtime String Constants (TDD)

These are behavioral changes: the tool reads/writes differently named directories, branches, git config keys, and environment variables. Apply TDD — update the test first, confirm it fails, then update the source, confirm it passes.

### Task 5: `.issues/` → `.armature/` data directory

**Files:**
- Modify: `internal/config/context.go` (lines 102, 108)
- Modify: `internal/config/context_test.go`
- Modify: `cmd/armature/init.go` (multiple `.issues/` string literals in hook scripts and comments)
- Modify: `cmd/armature/secondary_state_test.go`
- Modify: `internal/ops/tracker.go` (comment on line 29)
- Modify: `cmd/armature/helpers.go` (any `.issues/` in comments/strings)
- Modify: `internal/materialize/atsha.go` (comment on line 15)

- [ ] **Step 1: Update tests first**

In `internal/config/context_test.go`: replace all `.issues` path constructions with `.armature`:
```go
// Before:
issuesDir := filepath.Join(repo, ".issues")
// After:
issuesDir := filepath.Join(repo, ".armature")
```

In `cmd/armature/secondary_state_test.go`:
```go
// Before:
stateDir := filepath.Join(repo, ".issues", "state", workerID)
// After:
stateDir := filepath.Join(repo, ".armature", "state", workerID)
```

Search for all test files touching `.issues`:
```bash
grep -rn '\.issues' --include="*_test.go" .
```

Update every occurrence. Key locations in `cmd/armature/main_test.go` to check in addition to the files listed above:
- The `resolveIssuesDir` test helper (lines ~28–33) uses `.trellis/.issues` paths — update to `.arm/.armature/` for dual-branch and `.armature/` for single-branch
- Any `assert.DirExists(t, ".issues")` calls

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/config/... ./cmd/armature/... 2>&1 | grep -E "FAIL|PASS"
```

Expected: FAIL (tests expect `.armature/` but code still creates `.issues/`).

- [ ] **Step 3: Update source — config/context.go**

```go
// single-branch:
issuesDir = filepath.Join(actualRepoPath, ".armature")
// dual-branch:
issuesDir = filepath.Join(worktreePath, ".armature")
```

Also update the comment: `// Context holds resolved paths and config for the current armature session.`

- [ ] **Step 4: Update source — cmd/armature/init.go**

The hook script templates embedded in init.go contain `.issues/` references. Replace all occurrences:
- `.issues/config.json` → `.armature/config.json`
- `.issues/ops/` → `.armature/ops/`
- `.issues/hooks/` → `.armature/hooks/`

Also update the `Short:` description string:
```go
Short: "Initialize Armature in the current repository",
```

And the `--dual-branch` flag help text:
```go
"initialize in dual-branch mode (ops stored on separate _armature branch)"
```

- [ ] **Step 5: Bulk-sweep remaining `.issues` references in non-test Go source**

```bash
grep -rn '"\.issues\|`\.issues\|\.issues/' --include="*.go" . | grep -v "_test.go" | grep -v ".git"
```

Fix any remaining occurrences in source files.

- [ ] **Step 6: Run tests to confirm they pass**

```bash
go test ./internal/config/... ./cmd/armature/...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add -u
git commit -m "refactor: rename data directory from .issues to .armature"
```

---

### Task 6: `_trellis` → `_armature` branch, all `trellis.*` git config keys, and worktree path

**Files:**
- Modify: `cmd/armature/helpers.go` (lines 104, 135, 137, 138 — `_trellis` branch literals)
- Modify: `cmd/armature/init.go` (many `_trellis` occurrences in hook scripts and branch setup code)
- Modify: `cmd/armature/worker_init.go` (error string `"run 'trls worker-init'"`)
- Modify: `internal/config/context.go` (git config key strings on lines 104, 106, 149; error strings on lines 95, 110)
- Modify: `internal/config/context_test.go` (git config key strings on lines 67, 68, 82, 87, 94, 122, 123)
- Modify: `internal/worker/identity.go` (line 11: `gitConfigKey = "trellis.worker-id"`; line 28: error message with `trls worker-init`)
- Modify: `internal/worker/identity_test.go`
- Modify: `internal/git/git_test.go` (all `_trellis` branch name literals, `trellis.ops-worktree-path`)
- Modify: `internal/ops/pusher_test.go` (lines 62, 87 — `Branch: "_trellis"`)
- Modify: `cmd/armature/main_test.go` (hook content assertions on lines ~863, ~877, ~878, ~957, ~964; dual-branch dir-exists assertions on lines ~510–529)

- [ ] **Step 1: Update tests first**

In `internal/git/git_test.go`, replace all `"_trellis"` with `"_armature"`.

In `internal/ops/pusher_test.go`:
```go
// Before:
Branch: "_trellis",
// After:
Branch: "_armature",
```

In `internal/config/context_test.go`:
```go
// Before:
runGit("config", "trellis.mode", "dual-branch")
runGit("config", "trellis.ops-worktree-path", worktreePath)
// After:
runGit("config", "armature.mode", "dual-branch")
runGit("config", "armature.ops-worktree-path", worktreePath)
```

Also update error message assertions:
```go
// Before:
assert.Contains(t, err.Error(), "trellis.ops-worktree-path")
// After:
assert.Contains(t, err.Error(), "armature.ops-worktree-path")
```

In `internal/git/git_test.go`:
```go
// Before:
err := c.SetGitConfig("trellis.ops-worktree-path", "/some/path")
val, err := c.ReadGitConfig("trellis.ops-worktree-path")
// After:
err := c.SetGitConfig("armature.ops-worktree-path", "/some/path")
val, err := c.ReadGitConfig("armature.ops-worktree-path")
```

In `internal/worker/identity_test.go`, replace `trellis.worker-id` with `armature.worker-id`.

In `cmd/armature/main_test.go`, update:
- Hook content assertions (lines ~863, 877, 878): `"arm sync"`, `"arm heartbeat"`, `"arm push-ops"`
- Branch assertions (lines ~957, 964): `"_armature"`
- Dual-branch dir-exists assertions (lines ~510–529): `assert.DirExists(t, ".arm")`, `git config armature.mode`, `git config armature.ops-worktree-path`

- [ ] **Step 2: Run failing tests**

```bash
go test ./internal/config/... ./internal/git/... ./internal/ops/... ./internal/worker/...
```

Expected: FAIL.

- [ ] **Step 3: Update source — config/context.go**

```go
// Line ~95:
return nil, fmt.Errorf("read armature mode: %w", err)
// Line ~104:
worktreePath, err = readGitConfig(actualRepoPath, "armature.ops-worktree-path")
// Line ~106:
return nil, fmt.Errorf("dual-branch mode requires armature.ops-worktree-path to be set: %w", err)
// Line ~110:
return nil, fmt.Errorf("unknown armature mode: %q", mode)
// readGitConfigMode comment:
// readGitConfigMode reads armature.mode from git config. Returns "single-branch" if unset.
// Line ~149:
cmd := nonInteractiveGitCmd(repoPath, "config", "armature.mode")
```

Also update `WorktreePath` field comment:
```go
WorktreePath string // path to .arm/ worktree; empty in single-branch mode
```

- [ ] **Step 4: Update source — cmd/armature/helpers.go**

```go
// Before:
Branch: "_trellis",
// ...
gc2.Push("_trellis")
gc2.FetchAndRebase("_trellis")
// After:
Branch: "_armature",
// ...
gc2.Push("_armature")
gc2.FetchAndRebase("_armature")
```

- [ ] **Step 5: Update source — cmd/armature/init.go**

In the embedded hook scripts, replace all `_trellis` with `_armature`:
```bash
# Before:
if [ "$current_branch" = "_trellis" ]; then
# After:
if [ "$current_branch" = "_armature" ]; then
```

In the Go code:
```go
// Before:
if err := gitClient.CreateOrphanBranch("_trellis"); err != nil {
    return fmt.Errorf("create _trellis branch: %w", err)
}
worktreePath := filepath.Join(repoPath, ".trellis")
if err := gitClient.AddWorktree("_trellis", worktreePath); err != nil {
    return fmt.Errorf("add .trellis worktree: %w", err)
}
if err := gitClient.SetGitConfig("trellis.mode", "dual-branch"); err != nil {
    return fmt.Errorf("set trellis.mode: %w", err)
}
if err := gitClient.SetGitConfig("trellis.ops-worktree-path", worktreePath); err != nil {
    return fmt.Errorf("set trellis.ops-worktree-path: %w", err)
}
// After:
if err := gitClient.CreateOrphanBranch("_armature"); err != nil {
    return fmt.Errorf("create _armature branch: %w", err)
}
worktreePath := filepath.Join(repoPath, ".arm")
if err := gitClient.AddWorktree("_armature", worktreePath); err != nil {
    return fmt.Errorf("add .arm worktree: %w", err)
}
if err := gitClient.SetGitConfig("armature.mode", "dual-branch"); err != nil {
    return fmt.Errorf("set armature.mode: %w", err)
}
if err := gitClient.SetGitConfig("armature.ops-worktree-path", worktreePath); err != nil {
    return fmt.Errorf("set armature.ops-worktree-path: %w", err)
}
```

Also update hook script content: `.armature/config.json` checks for `"mode".*"dual-branch"`, `_armature` branch checks throughout.

- [ ] **Step 6: Update source — internal/worker/identity.go and cmd/armature/worker_init.go**

```go
// internal/worker/identity.go line 11:
const gitConfigKey = "armature.worker-id"
// line 28 error message:
return "", fmt.Errorf("worker ID not configured — run 'arm worker-init': %w", err)

// cmd/armature/worker_init.go — update error string:
"no worker ID configured — run 'arm worker-init'"
```

- [ ] **Step 7: Run tests to confirm pass**

```bash
go test ./internal/config/... ./internal/git/... ./internal/ops/... ./internal/worker/... ./cmd/armature/...
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -u
git commit -m "refactor: rename _trellis branch, all trellis.* git config keys, and .trellis worktree"
```

---

### Task 7: `TRLS_LOG_SLOT` → `ARM_LOG_SLOT`

**Files:**
- Modify: `cmd/armature/hook.go` (line 91)
- Modify: `cmd/armature/helpers.go` (line 80)
- Modify: `cmd/armature/main_test.go` (lines 1974, 2012, 2042, 2049, 2056, 2404, 2407)

- [ ] **Step 1: Update tests first**

In `cmd/armature/main_test.go`, replace all `TRLS_LOG_SLOT` with `ARM_LOG_SLOT` and update any log-slot test descriptions that mention the old name.

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./cmd/armature/... -run TestLogSlot
```

Expected: FAIL (code reads `TRLS_LOG_SLOT`, test sets `ARM_LOG_SLOT`).

- [ ] **Step 3: Update source — hook.go and helpers.go**

```go
// Before:
if slot := os.Getenv("TRLS_LOG_SLOT"); slot != "" {
// After:
if slot := os.Getenv("ARM_LOG_SLOT"); slot != "" {
```

- [ ] **Step 4: Run tests to confirm pass**

```bash
go test ./cmd/armature/... -run TestLogSlot
```

Expected: PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./...
```

Expected: all PASS. Fix any stragglers before continuing.

- [ ] **Step 6: Commit**

```bash
git add -u
git commit -m "refactor: rename TRLS_LOG_SLOT env var to ARM_LOG_SLOT"
```

---

### Task 8: Rename data directory in the repo itself + clean up stale worktree

The repo uses single-branch mode; its data lives in `.issues/`. Rename it to `.armature/` so the self-dogfooding instance matches the updated binary.

**Files:**
- Rename (git): `.issues/` → `.armature/`
- Remove: stale `.trellis/` worktree (from earlier dual-branch experiments)
- Modify: `.vscode/settings.json` (directory filter)

- [ ] **Step 1: Remove stale .trellis worktree**

```bash
git worktree list
git worktree remove .trellis --force 2>/dev/null || true
```

- [ ] **Step 2: Rename .issues to .armature in git**

```bash
git mv .issues .armature
```

- [ ] **Step 3: Update .vscode/settings.json**

Update only the `-.trellis` entry; keep the existing `-.claude/worktrees` filter:

```json
{
  "gopls": {
    "build.directoryFilters": [
      "-/.claude/worktrees",
      "-/.arm"
    ]
  }
}
```

- [ ] **Step 4: Verify binary works with renamed directory**

```bash
make install
arm ready --format agent 2>&1 | head -5
```

Expected: binary runs without errors, finds `.armature/` correctly.

- [ ] **Step 5: Commit**

```bash
git add .armature .vscode/settings.json
git commit -m "refactor: rename .issues to .armature in repo data and remove stale .trellis worktree"
```

---

### Task 9: Chunk 1+2 verification — `make check` green

- [ ] **Step 1: Run full CI check**

```bash
make check
```

Expected: lint ✓, test ✓, coverage ≥80% ✓, mutate ✓.

Fix any failures before proceeding to Chunk 3. Common issues to look for:
- `golangci-lint` flagging leftover `trellis` strings in comments → fix them
- Coverage drop → check if any renamed test files lost their connection to source
- Mutation survivors → no new mutations introduced by rename (if this regresses, the source logic changed accidentally)

- [ ] **Step 2: Commit fix (if needed)**

```bash
git add -u && git commit -m "fix: resolve any lint/test issues after rename"
```

---

## Chunk 3: Skills Revamp

### Task 10: Rename skill directories and update meta.yaml

**Files:**
- Rename: `skills/trls/` → `skills/armature/`
- Rename: `skills/trls-worker/` → `skills/armature-worker/`
- Rename: `skills/trls-coordinator/` → `skills/armature-coordinator/`
- Rename: `skills/trls-planner/` → `skills/armature-planner/`
- Rename: `skills/trls-auditor/` → `skills/armature-auditor/`
- Modify: each `meta.yaml`

- [ ] **Step 1: Rename skill directories**

```bash
git mv skills/trls skills/armature
git mv skills/trls-worker skills/armature-worker
git mv skills/trls-coordinator skills/armature-coordinator
git mv skills/trls-planner skills/armature-planner
git mv skills/trls-auditor skills/armature-auditor
```

- [ ] **Step 2: Update skills/armature/meta.yaml**

```yaml
name: armature
description: >
  Armature task management interface for AI agents. Use when working in an
  armature-managed repo: find actionable work with ready, claim issues, record
  progress with note/decision/heartbeat, complete work with transition.
  Requires arm on PATH (run make install).
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
```

- [ ] **Step 3: Update skills/armature-worker/meta.yaml**

```yaml
name: armature-worker
description: >
  Use when starting work in an armature-managed repository — picks up ready
  issues, claims them, assembles context, and drives implementation. Enforces
  per-task commits and story-level push/PR strategy.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH (run make install).
```

- [ ] **Step 4: Update skills/armature-coordinator/meta.yaml**

```yaml
name: armature-coordinator
description: >
  Use when coordinating parallel AI workers in an armature-managed repository.
  Handles worker dispatch, story tracking, PR strategy, and conflict resolution.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH (run make install).
```

- [ ] **Step 5: Update skills/armature-planner/meta.yaml and skills/armature-auditor/meta.yaml**

Apply the same pattern: `name:` → `armature-planner` / `armature-auditor`, update `description:` and `compatibility:`.

- [ ] **Step 6: Rebuild and rename the skills binary**

The `skills/armature/scripts/` directory contains a compiled ELF binary named `trls` (built from the old source). After the directory rename, it is now at `skills/armature/scripts/trls`. It must be rebuilt under the new binary name:

```bash
make build
cp bin/arm skills/armature/scripts/arm
git rm skills/armature/scripts/trls
git add skills/armature/scripts/arm
```

The `make skill` target copies everything under `skills/*/scripts/` into the deployed `.claude/skills/` and `.gemini/skills/` directories, so the binary name must match what the SKILL.md instructs agents to call.

- [ ] **Step 7: Commit**

```bash
git add skills/
git commit -m "refactor: rename skill directories from trls-* to armature-* and rebuild skills binary"
```

---

### Task 11: Revamp SKILL.md content — armature skill (command reference)

**File:** `skills/armature/SKILL.md`

This skill is a quick command reference. Update all `trls` → `arm`, `.issues/` → `.armature/`, and product name references.

- [ ] **Step 1: Update all command examples**

Replace every occurrence of `trls ` with `arm ` in code blocks and inline references throughout the file.

Key sections to update:
- "During Work" code block: `arm note`, `arm decision`, `arm heartbeat`, `arm transition`
- "Finding and Starting Work": `arm worker-init`, `arm ready`, `arm claim`, `arm render-context`
- Installation line: `make install   # installs to ~/.local/bin/arm`
- `SKILL.md` header canonical comment: `<!-- CANONICAL SOURCE: edit skills/armature/SKILL.md ... -->`

- [ ] **Step 2: Update data directory references**

- `.issues/` → `.armature/` everywhere in the file
- `_trellis` → `_armature`

- [ ] **Step 3: Update product name**

- "Trellis" → "Armature" in the title and any descriptive text

- [ ] **Step 4: Build and verify skill deploys**

```bash
make skill
```

Expected: `.claude/skills/armature/SKILL.md` created.

---

### Task 12: Revamp SKILL.md content — armature-worker

**File:** `skills/armature-worker/SKILL.md`

The worker skill is the most content-rich. Do a thorough pass.

- [ ] **Step 1: Update all trls → arm command references**

```bash
# Quick audit of what needs changing:
grep -n "trls\|Trellis\|\.issues\|_trellis\|TRLS_" skills/armature-worker/SKILL.md
```

Key updates:
- Title: `# Armature Worker`
- Prerequisites section: `arm` instead of `trls`, `~/.local/bin/arm`, `arm worker-init`
- All command examples: `arm note`, `arm decision`, `arm heartbeat`, `arm transition`
- Skill cross-references: `armature-coordinator` instead of `trls-coordinator`
- `.issues/` → `.armature/` in all staging instructions
- `_trellis` → `_armature` in dual-branch section
- `TRLS_LOG_SLOT` → `ARM_LOG_SLOT`
- Error table: update all `.issues/` paths and `_trellis` branch references

- [ ] **Step 2: Commit block — staging instructions**

The single-branch staging instructions are user-critical. Update:
```bash
# Before:
git add <each file from the task scope> .issues/
# After:
git add <each file from the task scope> .armature/
```

And the bundled command:
```bash
# Before:
arm transition ISSUE-ID --to done --outcome "..." && git add <scope files> .issues/ && git commit -m "..."
# After:
arm transition ISSUE-ID --to done --outcome "..." && git add <scope files> .armature/ && git commit -m "..."
```

- [ ] **Step 3: Deploy and verify**

```bash
make skill
grep -c "trls\|Trellis\|\.issues\b\|_trellis\|TRLS_" .claude/skills/armature-worker/SKILL.md
```

Expected: 0 matches (all old names eliminated from deployed copy).

---

### Task 13: Revamp SKILL.md content — armature-coordinator, armature-planner, armature-auditor

**Files:**
- `skills/armature-coordinator/SKILL.md`
- `skills/armature-planner/SKILL.md`
- `skills/armature-auditor/SKILL.md`

Apply the same sweep as Task 12 to each file. The coordinator skill also references `.issues/` staging in mop-up commit instructions and `_trellis` in dual-branch sections.

- [ ] **Step 1: Audit each file**

```bash
for f in skills/armature-coordinator/SKILL.md skills/armature-planner/SKILL.md skills/armature-auditor/SKILL.md; do
  echo "=== $f ==="
  grep -c "trls\|Trellis\|\.issues\b\|_trellis\|TRLS_" "$f"
done
```

- [ ] **Step 2: Update each file**

For coordinator specifically, update:
- `git add .issues/ && git commit -m "chore(STORY-ID): sync trellis state"` →
  `git add .armature/ && git commit -m "chore(STORY-ID): sync armature state"`
- Error table row: `Trellis ops from story/epic transitions` → `Armature ops from story/epic transitions`
- `TRLS_LOG_SLOT` → `ARM_LOG_SLOT` throughout (including the "Before running any arm command" instruction)

- [ ] **Step 3: Deploy all skills and final audit**

```bash
make skill
grep -rn "trls\|Trellis\b\|\.issues\b\|_trellis\|TRLS_" .claude/skills/
```

Expected: 0 matches.

- [ ] **Step 4: Commit**

```bash
git add skills/ .claude/skills/ .gemini/skills/
git commit -m "refactor: revamp all SKILL.md content for armature rename"
```

---

## Chunk 4: Documentation

### Task 14: Core project docs — README.md, CLAUDE.md, AGENTS.md

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `AGENTS.md`

- [ ] **Step 1: Update README.md**

The README needs a thorough rewrite of user-facing sections. Key changes:

```markdown
# Armature

**Git-Native Work Orchestration for Multi-Agent AI Coordination**

> "Context rot is a memory problem. Armature gives your agents memory."
```

Replace throughout:
- Title and tagline: Trellis → Armature
- All `trls` command examples → `arm`
- `_trellis` orphan branch → `_armature`
- `.issues/` → `.armature/`
- `TRLS_LOG_SLOT` → `ARM_LOG_SLOT`
- Clone URL: update to future armature repo URL (use placeholder `https://github.com/scullxbones/armature.git` until GitHub rename is done)
- Binary install instructions: `~/.local/bin/arm`, `arm init`, `arm ready`, etc.
- All prose references to "Trellis" → "Armature"

- [ ] **Step 2: Update CLAUDE.md**

```markdown
# Armature — Claude Code Rules
```

CLAUDE.md is mostly process rules; the only content change is the title and any project-specific references.

- [ ] **Step 3: Update AGENTS.md**

```markdown
# AGENTS.md — Armature Go Development
```

Update the title line. Check for any `trls`/`trellis`-specific references in the body.

```bash
grep -n "trls\|Trellis\|trellis" AGENTS.md
```

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md AGENTS.md
git commit -m "docs: update README, CLAUDE.md, and AGENTS.md for armature rename"
```

---

### Task 15: User documentation — docs/commands.md, docs/getting-started.md, docs/use-cases.md

**Files:**
- Modify: `docs/commands.md`
- Modify: `docs/getting-started.md`
- Modify: `docs/use-cases.md`

These are the primary user-facing docs. Do a thorough rewrite pass, not just a token swap — prose should read naturally with "Armature" as the product name.

- [ ] **Step 1: Audit each file**

```bash
for f in docs/commands.md docs/getting-started.md docs/use-cases.md; do
  echo "=== $f ==="; grep -c "trls\|Trellis\|trellis\|\.issues\|_trellis" "$f"
done
```

- [ ] **Step 2: Update docs/getting-started.md**

Key changes:
- `arm init` instead of `trls init`
- `_armature` orphan branch, `.arm/` worktree
- `.armature/` data directory
- "Armature will detect..." instead of "Trellis will detect..."
- All command examples updated
- `ARM_LOG_SLOT` instead of `TRLS_LOG_SLOT`

- [ ] **Step 3: Update docs/commands.md**

This is a comprehensive command reference. Replace:
- Every `trls <command>` → `arm <command>`
- `--dual-branch` description: `_armature` branch
- `.issues/` path references → `.armature/`
- `TRLS_LOG_SLOT` → `ARM_LOG_SLOT`

- [ ] **Step 4: Update docs/use-cases.md**

Similar sweep. Note the dual-branch use case section describes:
- `_armature` orphan branch
- `.arm/` worktree
- `.armature/` data directory

- [ ] **Step 5: Commit**

```bash
git add docs/commands.md docs/getting-started.md docs/use-cases.md
git commit -m "docs: update user documentation for armature rename"
```

---

### Task 16: Design docs and hook templates — bulk sweep

**Files:**
- Modify: `docs/design/architecture.md`
- Modify: `docs/design/trellis-prd.md`
- Modify: `docs/design/gap-resolutions.md`
- Modify: `docs/design/trellis-rename.md` (update status to "Complete")
- Modify: `.armature/hooks/*.template` (previously `.issues/hooks/*.template`, now in renamed dir)
- Modify: `.claude/settings.local.json`

Design docs are reference material; a token-replacement pass is sufficient without deep prose rewrite.

- [ ] **Step 1: Bulk sed on design docs**

```bash
sed -i \
  -e 's/Trellis/Armature/g' \
  -e 's/trellis/armature/g' \
  -e 's/_trellis/_armature/g' \
  -e 's/trls\b/arm/g' \
  -e 's/\.issues\//\.armature\//g' \
  -e 's/TRLS_LOG_SLOT/ARM_LOG_SLOT/g' \
  docs/design/architecture.md \
  docs/design/trellis-prd.md \
  docs/design/gap-resolutions.md \
  docs/design/dogfooding-learnings.md \
  docs/design/dogfood-ceremony-e1.md
```

Review the diff after — the sed is broad; check for any accidental substitutions in code samples or URLs.

- [ ] **Step 2: Update trellis-rename.md status**

Add a note at the top of `docs/design/trellis-rename.md`:
```markdown
**Status:** Complete — rename executed 2026-04-22. Armature is the name.
```

- [ ] **Step 3: Update hook templates in .armature/hooks/**

The hook template files now live in `.armature/hooks/` (after the data dir rename in Task 8). Their content still references `_trellis`, `.issues/`, and `trls`. Update:

```bash
sed -i \
  -e 's/_trellis/_armature/g' \
  -e 's/\.issues\//.armature\//g' \
  -e 's/\btrls\b/arm/g' \
  -e 's/trellis/armature/g' \
  .armature/hooks/*.template
```

- [ ] **Step 4: Clean up .claude/settings.local.json**

The file has many historical permissions with `trls` and `.trellis` path references that are no longer valid. Replace the relevant allow entries:

Key entries to update:
- `"Bash(trls *)"` → `"Bash(arm *)"`
- `"Bash(git -C /home/brian/development/trellis/.trellis log ...)"` → remove (stale path)
- `"Bash(/dev/null trls:*)"` → `"Bash(/dev/null arm:*)"`
- `"Bash(trls init:*)"` → `"Bash(arm init:*)"`
- `"Bash(trls unlink:*)"` → `"Bash(arm unlink:*)"`
- `"Bash(~/.local/bin/trls *)"` → `"Bash(~/.local/bin/arm *)"`
- `"Bash(export TRLS_LOG_SLOT=*)"` → `"Bash(export ARM_LOG_SLOT=*)"`
- `"Bash(unset TRLS_LOG_SLOT)"` → `"Bash(unset ARM_LOG_SLOT)"`

Remove any permissions that reference `/home/brian/development/trellis/` — those paths no longer exist.

- [ ] **Step 5: Commit**

```bash
git add docs/design/ .armature/hooks/ .claude/settings.local.json
git commit -m "docs: bulk rename in design docs, hook templates, and settings"
```

---

### Task 17: Historical plan/spec docs — mechanical sweep

**Files:** All `docs/superpowers/plans/*.md` and `docs/superpowers/specs/*.md`

These are historical artifacts; no prose rewrite needed. A single sed pass.

- [ ] **Step 1: Bulk sed**

```bash
find docs/superpowers/ -name "*.md" -o -name "*.json" | xargs sed -i \
  -e 's/scullxbones\/trellis/scullxbones\/armature/g' \
  -e 's/\.issues\//\.armature\//g' \
  -e 's/_trellis/_armature/g' \
  -e 's/\btrls\b/arm/g'
```

- [ ] **Step 2: Spot-check**

```bash
grep -rn "scullxbones/trellis\|\.issues/\|_trellis\b\|\btrls\b" docs/superpowers/ | head -10
```

Expected: 0 results. If any remain, fix manually.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/
git commit -m "docs: sweep historical plan/spec docs for armature rename"
```

---

### Task 18: Final verification — `make check` and binary smoke test

- [ ] **Step 1: Run full CI**

```bash
make check
```

Expected: lint ✓, test ✓, coverage ≥80% ✓, mutate ✓.

- [ ] **Step 2: Smoke test the installed binary**

```bash
make install
arm --help
arm ready --format agent
arm version
```

Expected: binary name shows `arm`, description shows "Armature", commands work against the `.armature/` data directory.

- [ ] **Step 3: Audit for any remaining old names in source**

```bash
grep -rn "\btrls\b\|scullxbones/trellis\|\.issues/\|_trellis\b\|TRLS_LOG_SLOT\|trellis\.mode\|trellis\.ops" \
  --include="*.go" --include="*.md" --include="*.yaml" --include="*.json" --include="Makefile" \
  --exclude-dir=".git" --exclude-dir="worktrees" \
  . | grep -v "docs/design/trellis-rename.md"
```

Expected: 0 results outside of the rename archive doc.

- [ ] **Step 4: Final commit if any stragglers were fixed**

```bash
git add -u && git commit -m "fix: final cleanup of remaining trellis/trls references"
```

- [ ] **Step 5: Remind user to rename GitHub repository**

After the local rename is complete: rename the GitHub repo from `scullxbones/trellis` to `scullxbones/armature` via GitHub Settings → Repository name. Then update the remote URL:

```bash
git remote set-url origin https://github.com/scullxbones/armature.git
git remote -v
```
