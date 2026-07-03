# Migration-Path Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Retire the residual defect sources in the `runRepoSetup` legacy-migration path: lossy ops-log copying, fragile `HEAD~1` rollback, locale-sensitive git output parsing, user-hook interference with internal commits, silent stranded backups — and lock the whole path down with a failure-injection permutation test.

**Architecture:** All product changes live in `cmd/armature/bootstrap.go` (migration orchestration) and `internal/adapters/git.go` (git boundary adapter). No new packages. Each task is a vertical TDD slice; the final task is test-only (invariant matrix over migration failure modes).

**Tech Stack:** Go, testify (`assert`/`require`), real git repos in `t.TempDir()` via existing helpers `initTempRepo(t)` and `run(t, dir, "git", ...)` in `cmd/armature/main_test.go`.

## Global Constraints

- Build/test via `make` targets; final gate is `make check` (lint + tests + coverage ≥ threshold + mutation testing). Never raw `go build`.
- TDD is mandatory (docs/agents/quality-gates.md): failing test first, then minimal code.
- Follow existing commit-message style: `feat:`/`fix:`/`test:`/`chore:` prefixes.
- Migration invariant (applies to every task): **legacy data must never be silently lost** — after any outcome (success, failure, rollback), every legacy file's content must exist either in the repo/worktree or in a backup directory that the error/output names.
- Do not change the public CLI surface; all changes are internal to bootstrap/adapters.

## Context for the implementer (read first)

Armature migrates a legacy single-branch layout (`<repo>/.armature/ops/...` on the code branch) to a dual-branch layout (ops live in `<repo>/.arm`, a git worktree checked out on orphan branch `_armature`). The migration in `cmd/armature/bootstrap.go`:

1. `migrateLegacySingleBranchOps` renames `.armature` → `.armature.migrated-<timestamp>` (backup); if `.armature` was git-tracked it also commits its removal on the current branch.
2. `runRepoSetup` then creates the `_armature` orphan branch and `.arm` worktree; on failure of either it calls `rollbackLegacyMigration`.
3. `copyLegacyOpsToNewWorktree` copies `ops/`, `templates/`, `hooks/`, `review/` from the backup into `.arm/.armature/`, skipping files that already exist at the destination; then the migrated data is committed on `_armature`.

Ops logs are append-only JSONL files named `<worker-uuid>.log` under `ops/` (see `internal/adapters/files.go` `ListLogFiles`). `internal/adapters/git.go` `Client` is the only way bootstrap talks to git.

---

### Task 1: Locale-stable git output parsing

`Client.CommitPaths` (internal/adapters/git.go:547) treats a failed commit as success when stderr contains the literal English string `"nothing to commit"`. Under a non-English locale (`LC_ALL=fr_FR.UTF-8` etc.) git localizes that message, turning a benign no-op into a hard failure mid-migration. Fix at the adapter root: force the C locale for every git invocation, so all output parsing in the client is stable.

**Files:**
- Modify: `internal/adapters/git.go:37-48` (`cmdContext`)
- Test: `internal/adapters/git_internal_test.go`

**Interfaces:**
- Consumes: existing `Client.cmdContext(ctx, args...)`.
- Produces: no signature changes; every `*exec.Cmd` built by `Client` now carries `LC_ALL=C` and `LANG=C` in its env. Later tasks rely on this for exact-string matching of git output.

- [ ] **Step 1: Write the failing test**

Append to `internal/adapters/git_internal_test.go`:

```go
func TestCmdContextForcesCLocale_REQ_MIGH_T1(t *testing.T) {
	c := New(t.TempDir())
	cmd := c.cmd("status")

	env := cmd.Env
	assert.Contains(t, env, "LC_ALL=C",
		"git commands must run under LC_ALL=C so output parsing (e.g. \"nothing to commit\") is locale-stable")
	assert.Contains(t, env, "LANG=C")
}
```

If the file does not already import `github.com/stretchr/testify/assert`, add it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/ -run TestCmdContextForcesCLocale_REQ_MIGH_T1 -count=1`
Expected: FAIL — env does not contain `LC_ALL=C`.

- [ ] **Step 3: Write minimal implementation**

In `internal/adapters/git.go` `cmdContext`, change the env line:

```go
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_ASKPASS=true",
		// Force the C locale so git's human-readable output is byte-stable and
		// callers that match on strings (e.g. CommitPaths' "nothing to commit",
		// isBenignEmptyRepoRmError, isGitContentionError) work under any user locale.
		"LC_ALL=C", "LANG=C",
	)
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/adapters/ -count=1`
Expected: PASS (whole package — the env change must not break other adapter tests).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/git.go internal/adapters/git_internal_test.go
git commit -m "fix: force C locale for git commands so output parsing is locale-stable"
```

---

### Task 2: Internal migration commits bypass user git hooks

Migration makes internal chore commits (legacy `.armature` removal on the code branch; migrated data and config on `_armature`; the orphan branch's empty init commit). A user's pre-existing `pre-commit`/`prepare-commit-msg` hook can reject or mutate these, aborting migration after the destructive rename has happened. Internal bookkeeping commits should not be subject to user hooks.

**Files:**
- Modify: `internal/adapters/git.go` (add `CommitPathsNoVerify`; add `--no-verify` to the orphan init commit in `CreateOrphanBranch`, currently ~line 205)
- Modify: `cmd/armature/bootstrap.go` (switch the three migration/bootstrap commit call sites from `CommitPaths` to `CommitPathsNoVerify`)
- Test: `cmd/armature/bootstrap_test.go`

**Interfaces:**
- Consumes: existing `CommitPaths(message string, paths ...string) error`.
- Produces: `func (c *Client) CommitPathsNoVerify(message string, paths ...string) error` — identical contract to `CommitPaths` (scoped pathspec commit; nil on "nothing to commit") but passes `--no-verify`. Task 6's matrix test relies on migration succeeding under a hostile hook.

- [ ] **Step 1: Write the failing test**

Append to `cmd/armature/bootstrap_test.go`:

```go
// TestRunRepoSetupMigrationSurvivesHostileUserHooks_REQ_MIGH_T2 verifies that a user's
// pre-existing pre-commit hook cannot abort migration: internal bookkeeping
// commits (legacy removal, orphan init, migrated data, config) run --no-verify.
func TestRunRepoSetupMigrationSurvivesHostileUserHooks_REQ_MIGH_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Tracked legacy layout, so migration also commits on the code branch.
	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	opsContent := []byte(`{"op":"x"}`)
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "w1.log"), opsContent, 0o600))
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy setup")

	// A pre-commit hook that rejects every commit. Worktrees share .git/hooks,
	// so this fires for code-branch AND _armature commits alike.
	hooksDir := filepath.Join(repo, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o755))

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err, "migration must not be blockable by user hooks")

	migrated, readErr := os.ReadFile(filepath.Join(repo, ".arm", ".armature", "ops", "w1.log"))
	require.NoError(t, readErr)
	assert.Equal(t, opsContent, migrated)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/armature/ -run TestRunRepoSetupMigrationSurvivesHostileUserHooks_REQ_MIGH_T2 -count=1`
Expected: FAIL — runRepoSetup errors (the hook rejects either the legacy-removal commit or the orphan init commit).

- [ ] **Step 3: Write minimal implementation**

In `internal/adapters/git.go`, refactor `CommitPaths` into a shared helper and add the no-verify variant (place next to `CommitPaths`, ~line 547):

```go
// CommitPaths creates a commit scoped to the given pathspecs, so it structurally cannot
// sweep in unrelated staged changes outside those paths. If there is nothing staged for
// the given paths, this is a no-op (returns nil) rather than an error. Any other commit
// failure (hook rejection, missing git identity, etc.) is returned as an error.
func (c *Client) CommitPaths(message string, paths ...string) error {
	return c.commitPaths(message, false, paths...)
}

// CommitPathsNoVerify is CommitPaths with --no-verify: user pre-commit and
// commit-msg hooks are bypassed. Use only for Armature's internal bookkeeping
// commits (e.g. legacy migration), which must not be blockable by user hooks.
func (c *Client) CommitPathsNoVerify(message string, paths ...string) error {
	return c.commitPaths(message, true, paths...)
}

func (c *Client) commitPaths(message string, noVerify bool, paths ...string) error {
	args := []string{"commit", "-m", message}
	if noVerify {
		args = append(args, "--no-verify")
	}
	args = append(append(args, "--"), paths...)
	cmd := c.cmd(args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if strings.Contains(string(out), "nothing to commit") {
		return nil
	}
	return fmt.Errorf("git commit: %w\n%s", err, out)
}
```

In `CreateOrphanBranch` (~line 205), add `--no-verify` to the init commit:

```go
	commitCmd := c.cmd("commit", "--allow-empty", "--no-verify", "-m", "chore: init armature issues branch")
```

In `cmd/armature/bootstrap.go`, switch these call sites from `CommitPaths` to `CommitPathsNoVerify` (grep for `CommitPaths(` — there are exactly three in this file):
1. In `migrateLegacySingleBranchOps`: the `"chore: migrate legacy .armature to dual-branch layout"` commit.
2. In `runRepoSetup`'s migration block: the `"chore: commit migrated legacy ops and config from single-branch layout"` commit.
3. In `runRepoSetup`'s fresh-config block: the `"chore: init armature config"` commit.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./cmd/armature/ -run 'TestRunRepoSetupMigration|TestRunRepoSetup' -count=1 && go test ./internal/adapters/ -count=1`
Expected: PASS, except `TestRunRepoSetupMigrationCommitFailureNamesBackupDir_P1` (existing), whose failure injection is a pre-commit hook that rejects staged `.armature/ops` commits — with `--no-verify` that trigger no longer fires. Rewrite that test: replace its hook-setup block with a bogus-GPG config that makes *every* commit fail regardless of `--no-verify`:

```go
	// Force commits to fail: signing with a nonexistent gpg program fails
	// regardless of --no-verify.
	run(t, repo, "git", "config", "commit.gpgsign", "true")
	run(t, repo, "git", "config", "user.signingkey", "NONEXISTENT-KEY")
	run(t, repo, "git", "config", "gpg.program", "/nonexistent-gpg")
```

With this trigger the first failing commit is the orphan-branch init commit (the test's legacy dir is untracked, so no code-branch commit precedes it), and that failure path rolls the un-committed rename back. So the test now covers rollback-on-early-commit-failure instead of backup naming: replace the two final assertions with:

```go
	require.Error(t, err)
	// With --no-verify commits, the first failure is the orphan-branch init commit;
	// the un-committed rename must have been rolled back so no data is stranded.
	assert.DirExists(t, filepath.Join(repo, ".armature"), "legacy dir restored by rollback")
	content, rerr := os.ReadFile(filepath.Join(repo, ".armature", "ops", "log.jsonl"))
	require.NoError(t, rerr)
	assert.Equal(t, []byte(`{"op":"x"}`), content)
```

and rename the test to `TestRunRepoSetupCommitFailureRollsBackUncommittedMigration_REQ_MIGH_T2` (update the doc comment to match: commit failures before the worktree exists roll the rename back).

- [ ] **Step 5: Run the full package**

Run: `go test ./cmd/armature/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/git.go cmd/armature/bootstrap.go cmd/armature/bootstrap_test.go
git commit -m "fix: internal migration commits bypass user git hooks (--no-verify)"
```

---

### Task 3: SHA-based migration rollback (replace HEAD~1 reset)

`rollbackLegacyMigration` reverts a committed migration with `ResetHard("HEAD~1")`. That silently assumes exactly one commit landed between migration and rollback — true today only by convention. If `CommitPaths` no-op'd (nothing was actually committed) or any future code commits in between, `HEAD~1` destroys the wrong commit. Capture the pre-migration HEAD SHA and reset to *it*.

**Files:**
- Modify: `cmd/armature/bootstrap.go` — `migrateLegacySingleBranchOps`, `rollbackLegacyMigration`, and their call sites in `runRepoSetup`
- Test: `cmd/armature/bootstrap_test.go`

**Interfaces:**
- Consumes: existing `Client.HeadSHA() (string, error)` (internal/adapters/git.go:437) and `Client.ResetHard(ref string) error`.
- Produces (signature changes — update every caller):
  - `migrateLegacySingleBranchOps(repoPath string) (migrated bool, backupDir string, preMigrationSHA string, err error)` — `preMigrationSHA` is non-empty **iff** a commit was actually made (HEAD moved), replacing the old `committed bool`.
  - `rollbackLegacyMigration(repoPath, backupDir, preMigrationSHA string) error` — resets to `preMigrationSHA` when non-empty; renames the backup back when empty.

- [ ] **Step 1: Write the failing test**

Append to `cmd/armature/bootstrap_test.go`:

```go
// TestRollbackLegacyMigrationResetsToPreMigrationSHA_REQ_MIGH_T3 verifies rollback restores
// exactly the pre-migration commit, even if extra commits exist above the migration
// commit — a HEAD~1 reset would destroy the wrong commit.
func TestRollbackLegacyMigrationResetsToPreMigrationSHA_REQ_MIGH_T3(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Tracked legacy layout.
	legacyOpsPath := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOpsPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(legacyOpsPath, "w1.log"), []byte(`{"op":"a"}`), 0o600))
	run(t, repo, "git", "add", ".armature")
	run(t, repo, "git", "commit", "-m", "legacy setup")

	gitClient := adapters.New(repo)
	preSHA, err := gitClient.HeadSHA()
	require.NoError(t, err)

	migrated, backupDir, gotPreSHA, err := migrateLegacySingleBranchOps(repo)
	require.NoError(t, err)
	require.True(t, migrated)
	assert.Equal(t, preSHA, gotPreSHA, "migration must report the pre-migration HEAD SHA")

	// Simulate an unrelated commit landing between migration and rollback.
	run(t, repo, "git", "commit", "--allow-empty", "--no-verify", "-m", "interloper")

	require.NoError(t, rollbackLegacyMigration(repo, backupDir, gotPreSHA))

	headSHA, err := gitClient.HeadSHA()
	require.NoError(t, err)
	assert.Equal(t, preSHA, headSHA, "rollback must land on the pre-migration commit, not HEAD~1")
	assert.DirExists(t, filepath.Join(repo, ".armature", "ops"), ".armature restored by reset")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/armature/ -run TestRollbackLegacyMigrationResetsToPreMigrationSHA_REQ_MIGH_T3 -count=1`
Expected: FAIL — compile error first (`migrateLegacySingleBranchOps` returns `(bool, string, bool, error)`); after the signature change lands the behavioral assertion is what the implementation must satisfy.

- [ ] **Step 3: Write minimal implementation**

In `migrateLegacySingleBranchOps`:
- Change the signature to `func migrateLegacySingleBranchOps(repoPath string) (bool, string, string, error)` and every `return false, "", false, err` to `return false, "", "", err`.
- Before the `isTracked` commit block, capture the SHA; after committing, verify HEAD moved:

```go
	// If .armature was tracked, commit the removal to keep the working tree clean.
	// Capture HEAD first so a rollback can reset to exactly this commit — and so a
	// commit that turns out to be a no-op ("nothing to commit") is not later
	// "rolled back" by destroying an unrelated commit.
	preMigrationSHA := ""
	if isTracked {
		beforeSHA, shaErr := gitClient.HeadSHA()
		if shaErr != nil {
			// Can't record a rollback point; restore the rename and abort.
			if restoreErr := os.Rename(backupDir, legacyArmatureDir); restoreErr != nil {
				return false, "", "", fmt.Errorf("read HEAD before migration commit: %w; restore .armature from backup %s: %w", shaErr, backupDir, restoreErr)
			}
			return false, "", "", fmt.Errorf("read HEAD before migration commit: %w", shaErr)
		}
		if err := gitClient.CommitPathsNoVerify("chore: migrate legacy .armature to dual-branch layout", ".armature"); err != nil {
			// ... keep the existing rollback-on-commit-failure block unchanged,
			// but its returns become four-value: return false, "", "", fmt.Errorf(...)
		}
		if afterSHA, shaErr := gitClient.HeadSHA(); shaErr == nil && afterSHA != beforeSHA {
			preMigrationSHA = beforeSHA
		}
	}

	return true, backupDir, preMigrationSHA, nil
```

In `rollbackLegacyMigration`, replace the `committed bool` parameter and the `HEAD~1` reset:

```go
// rollbackLegacyMigration undoes a legacy migration whose subsequent dual-branch setup
// (orphan branch creation or worktree add) failed, so the repo isn't left with .armature
// removed on disk/committed away while the new layout was never actually created.
//
// If preMigrationSHA is non-empty, the migration made a commit removing .armature from
// tracking; reset hard to that recorded SHA (not HEAD~1, which would assume exactly one
// commit landed since). Otherwise no commit exists to revert and the backup directory is
// simply renamed back into place.
func rollbackLegacyMigration(repoPath, backupDir, preMigrationSHA string) error {
	if preMigrationSHA != "" {
		gitClient := adapters.New(repoPath)
		if err := gitClient.ResetHard(preMigrationSHA); err != nil {
			return fmt.Errorf("revert migration commit (reset to %s): %w", preMigrationSHA, err)
		}
		return nil
	}

	legacyArmatureDir := filepath.Join(repoPath, ".armature")
	if err := os.Rename(backupDir, legacyArmatureDir); err != nil {
		return fmt.Errorf("restore .armature from backup %s: %w", backupDir, err)
	}
	return nil
}
```

In `runRepoSetup`, rename the local `migrationCommitted` to `preMigrationSHA` (type `string`), pass it through both rollback call sites, and change both `if migrationCommitted {` guards (the "backup left at" error wraps) to `if preMigrationSHA != "" {`.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./cmd/armature/ -run 'TestRollbackLegacyMigration|TestRunRepoSetup' -count=1`
Expected: PASS, including the existing rollback tests (`TestRunRepoSetupRollsBackMigrationWhenWorktreeAddFails_P1`, `TestMigrateLegacySingleBranchOpsRollsBackOnCommitFailure_P1`, `TestRunRepoSetupCommittedRollbackNamesLeftoverBackup_P2`). If `TestMigrateLegacySingleBranchOpsRollsBackOnCommitFailure_P1` calls `migrateLegacySingleBranchOps` directly, update its call to the four-value signature.

- [ ] **Step 5: Commit**

```bash
git add cmd/armature/bootstrap.go cmd/armature/bootstrap_test.go
git commit -m "fix: roll back migration to recorded pre-migration SHA instead of HEAD~1"
```

---

### Task 4: Merge divergent ops logs on migration instead of skipping

`copyLegacyOpsToNewWorktree` skips any file that already exists at the destination. When `_armature` was adopted from a remote that already holds the same per-worker `*.log` file, divergent legacy lines are silently dropped. Ops logs are append-only JSONL, so the safe semantics is a line-level union: keep the destination file, append legacy lines it doesn't contain, preserving order.

**Files:**
- Modify: `cmd/armature/bootstrap.go` — `copyLegacyOpsToNewWorktree`; new helper `mergeAppendOnlyLog`
- Test: `cmd/armature/bootstrap_test.go`

**Interfaces:**
- Consumes: `copyRecursive(src, dst string) (int, error)` (unchanged, still used for non-log files).
- Produces: `func mergeAppendOnlyLog(srcPath, dstPath string) (appended int, err error)` — appends each non-empty line of `srcPath` not already present (exact byte match) in `dstPath`, in source order, to the end of `dstPath`; returns how many lines were appended. `copyLegacyOpsToNewWorktree` gains no signature change, but its skip counter no longer counts merged `.log` files.

- [ ] **Step 1: Write the failing test**

Append to `cmd/armature/bootstrap_test.go`:

```go
// TestMigrationMergesDivergentOpsLogs_REQ_MIGH_T4 verifies that when the destination worktree
// already has the same per-worker ops log (e.g. _armature adopted from a remote),
// legacy lines missing from the destination are appended rather than dropped.
func TestMigrationMergesDivergentOpsLogs_REQ_MIGH_T4(t *testing.T) {
	backup := t.TempDir()
	worktree := t.TempDir()

	// Legacy log: two shared lines plus one line the destination doesn't have.
	legacyOps := filepath.Join(backup, "ops")
	require.NoError(t, os.MkdirAll(legacyOps, 0o750))
	legacy := "{\"op\":\"a\"}\n{\"op\":\"b\"}\n{\"op\":\"legacy-only\"}\n"
	require.NoError(t, os.WriteFile(filepath.Join(legacyOps, "w1.log"), []byte(legacy), 0o600))

	// Destination log: the shared lines plus one newer line of its own.
	issuesDir := filepath.Join(worktree, ".armature")
	dstOps := filepath.Join(issuesDir, "ops")
	require.NoError(t, os.MkdirAll(dstOps, 0o750))
	dst := "{\"op\":\"a\"}\n{\"op\":\"b\"}\n{\"op\":\"remote-newer\"}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dstOps, "w1.log"), []byte(dst), 0o600))

	_, err := copyLegacyOpsToNewWorktree(backup, issuesDir)
	require.NoError(t, err)

	merged, err := os.ReadFile(filepath.Join(dstOps, "w1.log"))
	require.NoError(t, err)
	want := "{\"op\":\"a\"}\n{\"op\":\"b\"}\n{\"op\":\"remote-newer\"}\n{\"op\":\"legacy-only\"}\n"
	assert.Equal(t, want, string(merged),
		"destination order preserved; missing legacy lines appended; no duplicates")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/armature/ -run TestMigrationMergesDivergentOpsLogs_REQ_MIGH_T4 -count=1`
Expected: FAIL — destination file untouched (skip-on-exists), `legacy-only` line missing.

- [ ] **Step 3: Write minimal implementation**

Add the helper next to `copyRecursive` in `cmd/armature/bootstrap.go`:

```go
// mergeAppendOnlyLog appends each non-empty line of srcPath that is not already
// present in dstPath (exact byte match) to the end of dstPath, preserving both
// files' line order. Ops logs are append-only JSONL, so a line-level union is the
// lossless way to reconcile a legacy log with one adopted from a remote; skipping
// the whole file (as copyRecursive does) would silently drop divergent entries.
// Returns the number of lines appended.
func mergeAppendOnlyLog(srcPath, dstPath string) (int, error) {
	srcData, err := os.ReadFile(srcPath) //nolint:gosec // G304: internal migration paths
	if err != nil {
		return 0, fmt.Errorf("read legacy log: %w", err)
	}
	dstData, err := os.ReadFile(dstPath) //nolint:gosec // G304: internal migration paths
	if err != nil {
		return 0, fmt.Errorf("read destination log: %w", err)
	}

	existing := make(map[string]struct{})
	for line := range strings.SplitSeq(string(dstData), "\n") {
		if line != "" {
			existing[line] = struct{}{}
		}
	}

	var missing []string
	for line := range strings.SplitSeq(string(srcData), "\n") {
		if line == "" {
			continue
		}
		if _, ok := existing[line]; !ok {
			missing = append(missing, line)
		}
	}
	if len(missing) == 0 {
		return 0, nil
	}

	out := string(dstData)
	if len(out) > 0 && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += strings.Join(missing, "\n") + "\n"

	info, err := os.Stat(dstPath)
	if err != nil {
		return 0, fmt.Errorf("stat destination log: %w", err)
	}
	if err := os.WriteFile(dstPath, []byte(out), info.Mode()); err != nil {
		return 0, fmt.Errorf("write merged log: %w", err)
	}
	return len(missing), nil
}
```

In `copyLegacyOpsToNewWorktree`, inside the `for _, entry := range entries` loop, route ops logs with an existing destination to the merge instead of `copyRecursive`:

```go
		for _, entry := range entries {
			srcPath := filepath.Join(legacyDir, entry.Name())
			dstPath := filepath.Join(newDir, entry.Name())

			// Ops logs are append-only JSONL; if the destination already has this
			// worker's log (e.g. _armature adopted from a remote), merge line-wise
			// instead of skipping, so divergent legacy entries are not lost.
			if dirName == "ops" && !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
				if _, statErr := os.Stat(dstPath); statErr == nil {
					if _, mergeErr := mergeAppendOnlyLog(srcPath, dstPath); mergeErr != nil {
						return skippedCount, fmt.Errorf("merge legacy ops log %s: %w", entry.Name(), mergeErr)
					}
					continue
				} else if !os.IsNotExist(statErr) {
					return skippedCount, fmt.Errorf("stat destination ops log %s: %w", entry.Name(), statErr)
				}
			}

			skipped, err := copyRecursive(srcPath, dstPath)
			if err != nil {
				return skippedCount, fmt.Errorf("copy legacy %s file %s: %w", dirName, entry.Name(), err)
			}
			skippedCount += skipped
		}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./cmd/armature/ -run 'TestMigration|TestCopyLegacy|TestCopyRecursive|TestRunRepoSetupMigration' -count=1`
Expected: PASS. Non-`.log` files (SCHEMA, templates, hooks, review) keep skip-on-exists semantics; `TestCopyLegacyOpsToNewWorktreeReportsSkippedCount_P3` must still pass — if that test uses `.log`-named files with existing destinations, its expected skip count changes; verify what it creates and, if it counts a now-merged log as "skipped", update the test's expectation and name it for merge semantics.

- [ ] **Step 5: Add the edge-case test (identical logs are a no-op)**

```go
// TestMergeAppendOnlyLogIdenticalIsNoOp_REQ_MIGH_T4 verifies merging identical logs neither
// duplicates lines nor rewrites the destination content.
func TestMergeAppendOnlyLogIdenticalIsNoOp_REQ_MIGH_T4(t *testing.T) {
	dir := t.TempDir()
	content := "{\"op\":\"a\"}\n{\"op\":\"b\"}\n"
	src := filepath.Join(dir, "src.log")
	dst := filepath.Join(dir, "dst.log")
	require.NoError(t, os.WriteFile(src, []byte(content), 0o600))
	require.NoError(t, os.WriteFile(dst, []byte(content), 0o600))

	appended, err := mergeAppendOnlyLog(src, dst)
	require.NoError(t, err)
	assert.Equal(t, 0, appended)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, string(got))
}
```

Run: `go test ./cmd/armature/ -run TestMergeAppendOnlyLog -count=1`
Expected: PASS (if it fails, fix `mergeAppendOnlyLog`, not the test).

- [ ] **Step 6: Commit**

```bash
git add cmd/armature/bootstrap.go cmd/armature/bootstrap_test.go
git commit -m "fix: merge divergent ops logs line-wise during migration instead of skipping"
```

---

### Task 5: Surface stranded migration backups on every bootstrap run

If a past migration failed after the rename, its `.armature.migrated-*` backup sits in the repo root and nothing ever mentions it again. Make every bootstrap run list existing backups so the user knows recovery material exists (informational only — never an error, never auto-deleted).

**Files:**
- Modify: `cmd/armature/bootstrap.go` — `runRepoSetup` (after the migration attempt, before the status output); new helper `listMigrationBackups`
- Test: `cmd/armature/bootstrap_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `func listMigrationBackups(repoPath string) []string` — sorted base names of `.armature.migrated-*` directories in `repoPath`; best-effort (returns nil on read errors).

- [ ] **Step 1: Write the failing test**

```go
// TestRunRepoSetupReportsStrandedBackups_REQ_MIGH_T5 verifies bootstrap mentions pre-existing
// migration backup dirs, so data stranded by an earlier failed run stays discoverable.
func TestRunRepoSetupReportsStrandedBackups_REQ_MIGH_T5(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	staleBackup := filepath.Join(repo, ".armature.migrated-20250101000000")
	require.NoError(t, os.MkdirAll(filepath.Join(staleBackup, "ops"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(staleBackup, "ops", "w1.log"), []byte(`{"op":"x"}`), 0o600))

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	_, err := runRepoSetup(cmd, repo)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), ".armature.migrated-20250101000000",
		"bootstrap should point at stranded migration backups")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/armature/ -run TestRunRepoSetupReportsStrandedBackups_REQ_MIGH_T5 -count=1`
Expected: FAIL — output does not mention the backup.

- [ ] **Step 3: Write minimal implementation**

Helper (place near `migrateLegacySingleBranchOps`):

```go
// listMigrationBackups returns the sorted base names of .armature.migrated-* backup
// directories in repoPath. Best-effort: read errors yield nil (this feeds an
// informational notice only).
func listMigrationBackups(repoPath string) []string {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil
	}
	var backups []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), ".armature.migrated-") {
			backups = append(backups, e.Name())
		}
	}
	sort.Strings(backups)
	return backups
}
```

Add `"sort"` to the imports of `cmd/armature/bootstrap.go`.

In `runRepoSetup`, right after the migration attempt's `if migrated { ... }` output block:

```go
	// Point at any migration backups (from this run or stranded by earlier failed
	// runs) so legacy data stays discoverable. Informational only.
	if backups := listMigrationBackups(repoPath); len(backups) > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Note: legacy migration backup(s) present: %s (safe to archive or delete once verified)\n",
			strings.Join(backups, ", "))
	}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./cmd/armature/ -run 'TestRunRepoSetup' -count=1`
Expected: PASS. Existing migration tests assert `Contains` (not exact output), so the extra Note line is compatible; if any test asserts full output equality, update it.

- [ ] **Step 5: Commit**

```bash
git add cmd/armature/bootstrap.go cmd/armature/bootstrap_test.go
git commit -m "feat: bootstrap reports stranded legacy migration backups"
```

---

### Task 6: Failure-injection permutation test (invariant matrix)

Test-only task. One table-driven test sweeping the migration state space, asserting the story's core invariant after every outcome: **no legacy byte is lost or unfindable** — content is in the worktree, or in the restored `.armature`, or in a backup whose path the error/output names — and a re-run of bootstrap after the failure either succeeds or fails with the backup named.

**Files:**
- Test (create): `cmd/armature/bootstrap_migration_matrix_test.go`

**Interfaces:**
- Consumes: `runRepoSetup(cmd, repoPath)`, helpers `initTempRepo`, `run`, `pathExists` from existing test files (same package `main`).
- Produces: nothing (tests only).

- [ ] **Step 1: Write the matrix test**

Create `cmd/armature/bootstrap_migration_matrix_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationInvariantMatrix_REQ_MIGH_T6 sweeps migration permutations and asserts the
// no-silent-data-loss invariant after each: every legacy file's content must be
// findable in the worktree, the restored .armature, or a backup directory named
// by the error or output; and a follow-up bootstrap run must converge.
func TestMigrationInvariantMatrix_REQ_MIGH_T6(t *testing.T) {
	type opsFile struct{ name, content string }
	legacyFiles := []opsFile{
		{"ops/w1.log", "{\"op\":\"tracked-1\"}\n"},
		{"ops/w2.log", "{\"op\":\"tracked-2\"}\n"},
		{"templates/story.md", "story template\n"},
	}
	untrackedFile := opsFile{"ops/untracked.log", "{\"op\":\"untracked\"}\n"}

	cases := []struct {
		name        string
		tracked     bool   // legacy .armature committed on the code branch
		addUntracked bool  // an extra legacy file not committed (clean-tree check ignores it)
		failure     string // "", "worktree-add"
	}{
		{"untracked_success", false, false, ""},
		{"tracked_success", true, false, ""},
		{"tracked_with_untracked_success", true, true, ""},
		{"untracked_worktree_add_fails", false, true, "worktree-add"},
		{"tracked_worktree_add_fails", true, true, "worktree-add"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initTempRepo(t)
			run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

			files := append([]opsFile{}, legacyFiles...)
			for _, f := range files {
				p := filepath.Join(repo, ".armature", f.name)
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
				require.NoError(t, os.WriteFile(p, []byte(f.content), 0o600))
			}
			if tc.tracked {
				run(t, repo, "git", "add", ".armature")
				run(t, repo, "git", "commit", "-m", "legacy setup")
			}
			if tc.addUntracked {
				p := filepath.Join(repo, ".armature", untrackedFile.name)
				require.NoError(t, os.WriteFile(p, []byte(untrackedFile.content), 0o600))
				files = append(files, untrackedFile)
			}

			if tc.failure == "worktree-add" {
				// .arm as a plain file makes `git worktree add` fail deterministically.
				require.NoError(t, os.WriteFile(filepath.Join(repo, ".arm"), []byte("x"), 0o600))
			}

			buf := new(bytes.Buffer)
			cmd := newRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			_, err := runRepoSetup(cmd, repo)

			if tc.failure == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			// Invariant: every legacy file's content is findable somewhere legitimate.
			for _, f := range files {
				assertLegacyContentFindable(t, repo, f.name, f.content, err, buf.String())
			}

			// Convergence: clear the injected failure and re-run; it must succeed and
			// end with all legacy content in the worktree or a named backup.
			if tc.failure == "worktree-add" {
				require.NoError(t, os.Remove(filepath.Join(repo, ".arm")))
			}
			buf2 := new(bytes.Buffer)
			cmd2 := newRootCmd()
			cmd2.SetOut(buf2)
			cmd2.SetErr(buf2)
			_, err2 := runRepoSetup(cmd2, repo)
			require.NoError(t, err2, "bootstrap must converge after the failure cause is removed")
			for _, f := range files {
				assertLegacyContentFindable(t, repo, f.name, f.content, nil, buf2.String())
			}
		})
	}
}

// assertLegacyContentFindable checks the file content exists in the worktree copy,
// the restored legacy dir, or a backup directory — and that if it is ONLY in a
// backup, that backup is named in the error or the command output.
func assertLegacyContentFindable(t *testing.T, repo, relName, content string, runErr error, output string) {
	t.Helper()

	candidates := []string{
		filepath.Join(repo, ".arm", ".armature", relName),
		filepath.Join(repo, ".armature", relName),
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil && strings.Contains(string(data), strings.TrimSpace(content)) {
			return
		}
	}

	// Fall back to backup dirs; content there is acceptable only if discoverable.
	entries, err := os.ReadDir(repo)
	require.NoError(t, err)
	mentioned := output
	if runErr != nil {
		mentioned += runErr.Error()
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".armature.migrated-") {
			continue
		}
		p := filepath.Join(repo, e.Name(), relName)
		if data, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(data), strings.TrimSpace(content)) {
			assert.Contains(t, mentioned, e.Name(),
				"legacy file %s survives only in backup %s, which must be named in error or output", relName, e.Name())
			return
		}
	}
	t.Errorf("legacy file %s content not found in worktree, legacy dir, or any backup", relName)
}
```

- [ ] **Step 2: Run the matrix**

Run: `go test ./cmd/armature/ -run TestMigrationInvariantMatrix_REQ_MIGH_T6 -count=1 -v`
Expected: PASS for all cases (Tasks 1–5 landed the behavior). Any failing cell is a real residual defect: **fix the product code, not the invariant.** If a cell exposes a defect outside this plan's scope, capture it as a dogfood finding per docs/agents/dogfood-findings.md and fix it in a follow-up commit within this story.

- [ ] **Step 3: Commit**

```bash
git add cmd/armature/bootstrap_migration_matrix_test.go
git commit -m "test: migration failure-injection matrix enforcing no-silent-data-loss invariant"
```

---

### Task 7: Full quality gate

- [ ] **Step 1: Run the repo gate**

Run: `make check`
Expected: lint clean, all tests pass, coverage and mutation thresholds met (baseline before this story: coverage 86.3%, mutation efficacy 100% — do not regress).

- [ ] **Step 2: Fix anything the gate flags, re-run until green, then commit any fixes**

```bash
git add -A
git commit -m "chore: quality-gate fixes for migration hardening story"
```

(Skip the commit if the gate was green with nothing to fix.)
