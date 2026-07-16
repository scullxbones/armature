# CI Validation Pipeline Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `make check` wrapper target that runs all validation steps (lint, test, mutate, coverage-check), wire it into a GitHub Actions workflow on push, and document it in AGENTS.md so agents run it automatically.

**Architecture:** A single `check` Makefile phony target chains the existing validation targets in fail-fast order. A GitHub Actions workflow installs required tools (golangci-lint, gremlins) and runs `make check` on every push and pull request. AGENTS.md gains a "Before You Commit" section pointing to `make check`.

**Tech Stack:** GNU Make, GitHub Actions, Go 1.26.1, golangci-lint, gremlins

---

## Chunk 1: Makefile `check` target and AGENTS.md update

### Task 1: Add `check` make target

**Files:**
- Modify: `Makefile`

The `check` target must run steps in this order (fail-fast is the default for make):
1. `lint` — fast, catches issues before running slow steps
2. `test` — runs all unit/integration/property tests
3. `coverage-check` — enforces the 80% threshold (re-runs tests internally; acceptable duplication for clarity)
4. `mutate` — slowest; runs last to avoid wasting time when earlier checks fail

> **Why `check` and not `validate`?** The CLI binary already has a `arm validate` subcommand. Naming the Makefile target `validate` would cause confusion. `check` is idiomatic in Go toolchains (e.g., `go vet`, `staticcheck`).

- [ ] **Step 1: Open the Makefile and locate the `.PHONY` line and `help` target**

  Read `Makefile` to confirm current content before editing.

- [ ] **Step 2: Add `check` to `.PHONY` and insert the target**

  Edit `Makefile`: append `check` to the `.PHONY` line, add to `help`, and insert the target after the `mutate` block:

  ```makefile
  .PHONY: test coverage coverage-check lint clean mutate check help skill install
  ```

  In the `help` target, add:
  ```
  	@echo "  make check      - Run all validation: lint, test, coverage-check, mutate"
  ```

  New target (insert after the `mutate` block, before `clean`):
  ```makefile
  check: lint test coverage-check mutate
  ```

  That's it — Make's dependency syntax handles sequencing and fail-fast automatically.

- [ ] **Step 3: Verify the target is wired correctly (dry run)**

  Run:
  ```bash
  make --dry-run check
  ```

  Expected output shows the commands for `lint`, then `test`, then `coverage-check`, then `mutate` — in that order — without actually executing them.

- [ ] **Step 4: Commit**

  ```bash
  git add Makefile
  git commit -m "feat: add make check target to run full validation pipeline"
  ```

---

### Task 2: Update AGENTS.md

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: Read AGENTS.md to understand current structure**

- [ ] **Step 2: Add a "Before You Commit" section**

  Insert a new section between "Key Commands" and "File Organization":

  ```markdown
  ## Before You Commit

  Always run the full validation pipeline before committing or submitting work:

  ```bash
  make check
  ```

  This runs, in order:
  1. `make lint` — static analysis via golangci-lint
  2. `make test` — all unit, property, and integration tests
  3. `make coverage-check` — enforces 80% coverage threshold
  4. `make mutate` — mutation testing via gremlins on `./internal` and `./cmd`

  **If any step fails, fix it before proceeding.** Do not skip or comment out failing checks.
  ```

- [ ] **Step 3: Commit**

  ```bash
  git add AGENTS.md
  git commit -m "docs: document make check in AGENTS.md as pre-commit requirement"
  ```

---

## Chunk 2: GitHub Actions workflow

### Task 3: Create the GitHub Actions workflow

**Files:**
- Create: `.github/workflows/ci.yml`

The workflow needs to:
1. Check out code
2. Set up Go (version from `go.mod`)
3. Cache Go module download cache
4. Install `golangci-lint` (via the official action — faster than `go install`, uses caching)
5. Install `gremlins` (no official action; use `go install`)
6. Run `make check`

Trigger on: `push` to any branch, `pull_request` to any branch.

- [ ] **Step 1: Create the `.github/workflows/` directory structure**

  ```bash
  mkdir -p .github/workflows
  ```

- [ ] **Step 2: Write the workflow file**

  Create `.github/workflows/ci.yml`:

  ```yaml
  name: CI

  on:
    push:
    pull_request:

  jobs:
    check:
      name: Validate
      runs-on: ubuntu-latest
      timeout-minutes: 30

      steps:
        - name: Checkout
          uses: actions/checkout@v4

        - name: Set up Go
          uses: actions/setup-go@v5
          with:
            go-version-file: 'go.mod'
            cache: true

        - name: Install gremlins
          run: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest

        - name: Install golangci-lint
          run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

        - name: Run make check
          run: make check
  ```

  > **Note on golangci-lint:** Using `go install` keeps the workflow simple and avoids coupling to the `golangci-lint-action` interface. The tradeoff is slower installs (no action-level caching), but `actions/setup-go` with `cache: true` already caches the Go module download cache, which covers most of the install time.
  >
  > **Optional upgrade:** If install time becomes a bottleneck, replace the `go install` step with:
  > ```yaml
  >       - name: Install golangci-lint
  >         uses: golangci/golangci-lint-action@v6
  >         with:
  >           install-mode: binary
  >           args: --version  # install only; make lint runs it
  > ```

- [ ] **Step 3: Validate the YAML syntax locally**

  ```bash
  python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "YAML valid"
  ```

  Expected: `YAML valid`

- [ ] **Step 4: Commit**

  ```bash
  git add .github/workflows/ci.yml
  git commit -m "ci: add GitHub Actions workflow to run make check on push"
  ```

---

## Final verification

- [ ] Push to GitHub and confirm the Actions tab shows the `CI / Validate` job running and passing.
- [ ] Confirm the workflow appears under the "Actions" tab for both push and pull_request events.
- [ ] If `make check` consistently exceeds 30 minutes (gremlins on a large codebase can be slow), increase `timeout-minutes` in `.github/workflows/ci.yml` and consider running `mutate` in a separate nightly workflow rather than on every push.
