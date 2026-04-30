# Release Pipeline Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a production-grade release pipeline: goreleaser config, GitHub Actions release workflow, cross-platform builds, shell completion commands, and automatic changelog/checksums on tag push.

**Architecture:** goreleaser drives all release artifacts from a single `.goreleaser.yaml`. The release GitHub Actions workflow triggers on `v*.*.*` tag push, skips `make check` (CI already enforces it), and publishes binaries + checksums + changelog to GitHub Releases. Shell completion subcommands (`arm completion bash|zsh|fish`) are added to the CLI for human users; they are passive (no side effects on normal `arm` invocations) so agent workflows are unaffected.

**Tech Stack:** Go, goreleaser v2, GitHub Actions, cobra (verify CLI framework in Task 1)

**Spec:** `docs/superpowers/specs/2026-04-26-readme-positioning-design.md` (context only — no spec for this task; decisions are captured below)

**Key decisions:**
- Cross-build targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` — no 32-bit
- Changelog: goreleaser built-in, grouped by Conventional Commit type
- Checksums: goreleaser auto-generates `checksums.txt` (SHA256)
- `make check` NOT run in release workflow (redundant with CI)
- Shell completion: only add `arm completion` subcommand — does not affect agent operation
- Copyright: `scullxbones`

---

## Chunk 1: goreleaser config and release workflow

### Task 1: Check CLI framework and binary entry point

**Files:**
- Read: `cmd/armature/main.go` (or equivalent entry point)
- Read: `go.mod` for cobra/urfave-cli dependency

- [ ] **Step 1: Identify CLI framework**

```bash
grep -r "cobra\|urfave/cli\|kingpin\|flag\." go.mod cmd/
```

Expected: identify whether the project uses cobra, urfave/cli, or stdlib `flag`. Record the result — Task 4 (shell completion) depends on it.

- [ ] **Step 2: Identify binary name and main package path**

```bash
grep -r "func main" cmd/
```

Expected: `cmd/armature/main.go` (or similar). The binary is named `arm` per the Makefile (`-o bin/arm`). Confirm the package path for goreleaser's `main` field.

---

### Task 2: Create `.goreleaser.yaml`

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Write goreleaser config**

Create `.goreleaser.yaml` at the repo root with this content (adjust `main` path if Task 1 reveals a different entry point):

```yaml
version: 2

project_name: armature

before:
  hooks:
    - go mod tidy

builds:
  - id: arm
    binary: arm
    main: ./cmd/armature
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w -X main.Version={{.Version}}
    ignore:
      - goos: windows
        goarch: arm64

archives:
  - id: arm
    name_template: "arm_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    formats:
      - tar.gz
    format_overrides:
      - goos: windows
        formats:
          - zip
    files:
      - LICENSE
      - README.md

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

changelog:
  use: github
  sort: asc
  groups:
    - title: "New Features"
      regexp: "^feat"
      order: 0
    - title: "Bug Fixes"
      regexp: "^fix"
      order: 1
    - title: "Documentation"
      regexp: "^docs"
      order: 2
    - title: "Other Changes"
      order: 999
  filters:
    exclude:
      - "^chore"
      - "^test"
      - "^ci"
      - "Merge pull request"

release:
  github:
    owner: scullxbones
    name: armature
  draft: false
  prerelease: auto
  name_template: "{{.ProjectName}} v{{.Version}}"
```

Note on `windows/arm64`: excluded because the Go toolchain supports it but it has essentially zero real-world demand and goreleaser cross-compilation for that target can be flaky.

- [ ] **Step 2: Verify goreleaser config syntax**

```bash
goreleaser check 2>&1 || echo "goreleaser not installed locally — syntax check skipped, will validate in CI"
```

If goreleaser is installed: expected `config is valid`. If not installed, that's fine — CI will catch any errors.

- [ ] **Step 3: Commit**

```bash
git add .goreleaser.yaml LICENSE
git commit -m "chore: add goreleaser config with cross-platform builds and changelog"
```

---

### Task 3: Create GitHub Actions release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Write release workflow**

```yaml
name: Release

on:
  push:
    tags:
      - 'v*.*.*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26.1'

      - name: Run goreleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Key points:
- `fetch-depth: 0` — goreleaser needs full git history to generate the changelog between tags
- `permissions: contents: write` — required to create GitHub Releases and upload assets
- No `make check` step — CI workflow already enforces this on every push/PR
- `prerelease: auto` in goreleaser config means tags like `v1.0.0-rc.1` auto-publish as pre-releases

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add GitHub Actions release workflow triggered on v*.*.* tags"
```

---

## Chunk 2: Shell completion

### Task 4: Add `arm completion` subcommand

**Files depend on CLI framework identified in Task 1.**

#### If using cobra:

Cobra has built-in completion generation. Add a `completion` command:

**Files:**
- Create: `cmd/armature/cmd_completion.go` (or wherever other command files live — check existing structure)

- [ ] **Step 1: Write failing test**

```go
// In the appropriate test file, e.g. cmd/armature/cmd_completion_test.go
func TestCompletionCommand_Bash(t *testing.T) {
    output, err := runCLI(t, "completion", "bash")
    require.NoError(t, err)
    assert.Contains(t, output, "bash")
    assert.Contains(t, output, "arm")
}

func TestCompletionCommand_Zsh(t *testing.T) {
    output, err := runCLI(t, "completion", "zsh")
    require.NoError(t, err)
    assert.Contains(t, output, "zsh")
}

func TestCompletionCommand_Fish(t *testing.T) {
    output, err := runCLI(t, "completion", "fish")
    require.NoError(t, err)
    assert.Contains(t, output, "fish")
}
```

(Adjust `runCLI` helper to match existing test patterns in the codebase.)

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./cmd/... -run TestCompletion -v
```
Expected: FAIL — completion command not found.

- [ ] **Step 3: Implement completion command**

```go
// cmd/armature/cmd_completion.go
package main

import (
    "github.com/spf13/cobra"
    "os"
)

func newCompletionCmd(rootCmd *cobra.Command) *cobra.Command {
    return &cobra.Command{
        Use:   "completion [bash|zsh|fish|powershell]",
        Short: "Generate shell completion scripts",
        Long: `Generate shell completion scripts for arm.

To load completions (examples):

  bash:   source <(arm completion bash)
  zsh:    arm completion zsh > "${fpath[1]}/_arm"
  fish:   arm completion fish | source`,
        ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
        Args:                  cobra.ExactArgs(1),
        DisableFlagsInUseLine: true,
        RunE: func(cmd *cobra.Command, args []string) error {
            switch args[0] {
            case "bash":
                return rootCmd.GenBashCompletion(os.Stdout)
            case "zsh":
                return rootCmd.GenZshCompletion(os.Stdout)
            case "fish":
                return rootCmd.GenFishCompletion(os.Stdout, true)
            case "powershell":
                return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
            }
            return nil
        },
    }
}
```

Register it in the root command setup (wherever other subcommands are added).

#### If NOT using cobra (stdlib flag or urfave/cli):

Implement completion manually or use the appropriate library's completion mechanism. The test structure above still applies — verify `arm completion bash` exits 0 and outputs non-empty shell script content.

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./cmd/... -run TestCompletion -v
```
Expected: PASS

- [ ] **Step 5: Smoke test manually**

```bash
go run ./cmd/armature completion bash | head -5
go run ./cmd/armature completion zsh | head -5
go run ./cmd/armature completion fish | head -5
```
Expected: non-empty shell script output for each, no errors.

- [ ] **Step 6: Verify make check passes**

```bash
make check
```
Expected: all stages green.

- [ ] **Step 7: Commit**

```bash
git add cmd/
git commit -m "feat: add arm completion subcommand for bash, zsh, fish, powershell"
```

---

## Chunk 3: Snapshot test and final verification

### Task 5: Snapshot build verification

**Files:** none (read-only verification)

- [ ] **Step 1: Run goreleaser snapshot build**

```bash
goreleaser build --snapshot --clean 2>&1
```

If goreleaser is not installed locally:
```bash
go install github.com/goreleaser/goreleaser/v2@latest
goreleaser build --snapshot --clean
```

Expected: `dist/` directory populated with binaries for all 5 targets:
- `arm_linux_amd64/arm`
- `arm_linux_arm64/arm`
- `arm_darwin_amd64/arm`
- `arm_darwin_arm64/arm`
- `arm_windows_amd64/arm.exe`

- [ ] **Step 2: Verify binary runs**

```bash
./dist/arm_linux_amd64_v1/arm --version 2>/dev/null || ./dist/arm_linux_amd64/arm --version
```
Expected: version string printed (e.g. `arm vX.X.X-SNAPSHOT-<sha>`).

- [ ] **Step 3: Confirm dist/ is gitignored**

```bash
grep "dist/" .gitignore 2>/dev/null || echo "dist/ not in .gitignore — add it"
```
If missing, add `dist/` to `.gitignore`.

- [ ] **Step 4: Final commit if needed**

```bash
git add .gitignore   # only if Step 3 required a change
git commit -m "chore: add dist/ to gitignore"
```

---

## Release process (for the human, after this plan is implemented)

To cut a release:

```bash
# Ensure main is clean and CI is green
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions release workflow fires automatically. The release appears at `github.com/scullxbones/armature/releases` with:
- Binaries for all 5 targets (tar.gz / zip)
- `checksums.txt` (SHA256)
- Changelog auto-generated from commits since previous tag

Pre-releases: use tags like `v0.1.0-rc.1` — goreleaser marks them as pre-release automatically.
