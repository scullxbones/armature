# arm bootstrap Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `arm init` and `arm install-skills` with a single idempotent `arm bootstrap` command driven by a pure functional harness planner.

**Architecture:** A new no-I/O `internal/bootstrap` planner module returns a declarative per-platform matrix of install/skip/unsupported decisions. The `arm bootstrap` command executes those decisions, covering repo setup (init logic) and harness setup (skills, plugin metadata, harness hook config). The old `arm init` and `arm install-skills` commands are deleted outright — no compatibility shim.

**Tech Stack:** Go 1.26, Cobra CLI, `github.com/scullxbones/armature` module, `internal/harnesshook` platform adapters, `internal/skillsembed` embedded FS.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/bootstrap/planner.go` | Pure harness setup planner — no I/O |
| Create | `internal/bootstrap/planner_test.go` | Planner unit tests |
| Create | `cmd/armature/bootstrap.go` | `arm bootstrap` command + repo setup + harness execution + JSON output |
| Create | `cmd/armature/bootstrap_deploy.go` | Deploy helpers moved from install_skills.go |
| Create | `cmd/armature/bootstrap_test.go` | Command integration tests |
| Modify | `internal/harnesshook/types.go` | Add `OwnsConfig` to `PlatformAdapter` interface |
| Modify | `internal/harnesshook/platform_claude.go` | Implement `OwnsConfig` (always true — key-merge) |
| Modify | `internal/harnesshook/platform_codex.go` | Implement `OwnsConfig` + add `# armature:managed` marker to WriteConfig |
| Modify | `internal/harnesshook/platform_devin.go` | Implement `OwnsConfig` + add `"_armature":"managed"` key to WriteConfig |
| Modify | `internal/harnesshook/platform_test.go` | Tests for OwnsConfig and markers |
| Modify | `cmd/armature/main.go` | Register bootstrap; remove init + install-skills |
| Delete | `cmd/armature/init.go` | Replaced by bootstrap.go |
| Delete | `cmd/armature/install_skills.go` | Replaced by bootstrap_deploy.go |
| Delete | `cmd/armature/install_skills_test.go` | Replaced by bootstrap_test.go |
| Modify | `docs/commands.md` | Add bootstrap section; remove init + install-skills |
| Modify | `CLAUDE.md` | Update `arm install-skills` → `arm bootstrap` |
| Modify | `AGENTS.md` | Update `arm install-skills` → `arm bootstrap` |
| Modify | `internal/skillsembed/skills/armature/SKILL.md` | Update Setup section |

---

## Chunk 1: Harness Setup Planner

### Task 1: Define planner types and DefaultPlatforms

**Files:**
- Create: `internal/bootstrap/planner.go`
- Create: `internal/bootstrap/planner_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/bootstrap/planner_test.go
package bootstrap_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/stretchr/testify/assert"
)

func TestDefaultPlatformsIncludesClaude(t *testing.T) {
	platforms := bootstrap.DefaultPlatforms()
	assert.Contains(t, platforms, bootstrap.PlatformClaude)
}

func TestDefaultPlatformsDoesNotIncludeUnverified(t *testing.T) {
	platforms := bootstrap.DefaultPlatforms()
	assert.NotContains(t, platforms, bootstrap.PlatformAntigravity)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/bootstrap/... -run TestDefaultPlatforms -v
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Create `internal/bootstrap/planner.go` with types and DefaultPlatforms**

```go
// Package bootstrap provides the harness setup planner for arm bootstrap.
// The planner is a pure functional module with no I/O.
package bootstrap

// Platform identifies a supported AI harness.
type Platform string

const (
	PlatformClaude      Platform = "claude"
	PlatformCodex       Platform = "codex"
	PlatformAntigravity Platform = "antigravity"
	PlatformDevin       Platform = "devin"
)

// ArtifactKind identifies a bootstrap artifact category.
type ArtifactKind string

const (
	ArtifactSkills            ArtifactKind = "skills"
	ArtifactPluginMetadata    ArtifactKind = "plugin_metadata"
	ArtifactHarnessHookConfig ArtifactKind = "harness_hook_config"
)

// ActionKind is the per-cell plan decision.
type ActionKind string

const (
	ActionInstall     ActionKind = "install"
	ActionSkip        ActionKind = "skip"
	ActionUnsupported ActionKind = "unsupported"
)

// PlatformRow is one row in the plan matrix.
type PlatformRow struct {
	Platform          Platform
	Skills            ActionKind
	PluginMetadata    ActionKind
	HarnessHookConfig ActionKind
}

// Plan is the full declarative harness setup plan.
type Plan struct {
	Target string // "local" or "global"
	Rows   []PlatformRow
}

// PlanRequest holds the inputs to BuildPlan.
type PlanRequest struct {
	Platforms []Platform // empty = DefaultPlatforms()
	Target    string     // "local" or "global"; defaults to "local"
	WithHooks bool
}

// allKnownPlatforms is the exhaustive set of platforms Armature recognises.
var allKnownPlatforms = []Platform{
	PlatformClaude, PlatformCodex, PlatformAntigravity, PlatformDevin,
}

// Verification contract: a platform/artifact is "verified" and may appear in a
// verified* map only when ALL of the following are true:
//
//  1. A writer function for that artifact exists in arm and targets the correct
//     platform-specific path.
//  2. An integration test exercises `arm bootstrap [--platform <p>]` and asserts
//     that the artifact appears at the correct path.
//  3. For harness_hook_config: additionally, ownership tests exist for both the
//     managed-file and unmanaged-file cases.
//
// To add a platform, implement the writer, add the integration test, and update
// the map below. Do not add a platform entry based on future intent alone.

// verifiedSkills lists platforms with a verified arm-level skills+flat deploy path.
var verifiedSkills = map[Platform]bool{
	PlatformClaude: true,
}

// verifiedPluginMetadata lists platforms with a verified plugin metadata deploy path.
var verifiedPluginMetadata = map[Platform]bool{
	PlatformClaude: true,
}

// verifiedHarnessHookConfig lists platforms whose WriteConfig and OwnsConfig are
// implemented and tested in harnesshook.
var verifiedHarnessHookConfig = map[Platform]bool{
	PlatformClaude: true,
	PlatformCodex:  true,
	PlatformDevin:  true,
}

// DefaultPlatforms returns the verified default platform set.
// A platform is included if it has verified skills or plugin_metadata support.
func DefaultPlatforms() []Platform {
	var result []Platform
	for _, p := range allKnownPlatforms {
		if verifiedSkills[p] || verifiedPluginMetadata[p] {
			result = append(result, p)
		}
	}
	return result
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/bootstrap/... -run TestDefaultPlatforms -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bootstrap/planner.go internal/bootstrap/planner_test.go
git commit -m "feat: add bootstrap planner types and DefaultPlatforms"
```

---

### Task 2: Implement BuildPlan

**Files:**
- Modify: `internal/bootstrap/planner.go`
- Modify: `internal/bootstrap/planner_test.go`

- [ ] **Step 1: Write the failing tests**

Add `"github.com/stretchr/testify/require"` to the import block in `planner_test.go`, then append these functions:

```go
// Add to internal/bootstrap/planner_test.go

func findRow(t *testing.T, plan bootstrap.Plan, p bootstrap.Platform) bootstrap.PlatformRow {
	t.Helper()
	for _, row := range plan.Rows {
		if row.Platform == p {
			return row
		}
	}
	t.Fatalf("platform %q not found in plan", p)
	return bootstrap.PlatformRow{}
}

func TestBuildPlanDefaultInstallsSkillsAndPlugin(t *testing.T) {
	plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{})
	require.NoError(t, err)
	row := findRow(t, plan, bootstrap.PlatformClaude)
	assert.Equal(t, bootstrap.ActionInstall, row.Skills)
	assert.Equal(t, bootstrap.ActionInstall, row.PluginMetadata)
}

func TestBuildPlanDefaultSkipsHookConfig(t *testing.T) {
	plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{})
	require.NoError(t, err)
	row := findRow(t, plan, bootstrap.PlatformClaude)
	assert.Equal(t, bootstrap.ActionSkip, row.HarnessHookConfig)
}

func TestBuildPlanWithHooksInstallsHookConfig(t *testing.T) {
	plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{WithHooks: true})
	require.NoError(t, err)
	row := findRow(t, plan, bootstrap.PlatformClaude)
	assert.Equal(t, bootstrap.ActionInstall, row.HarnessHookConfig)
}

func TestBuildPlanTargetDefaultsToLocal(t *testing.T) {
	plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{})
	require.NoError(t, err)
	assert.Equal(t, "local", plan.Target)
}

func TestBuildPlanGlobalTarget(t *testing.T) {
	plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{Target: "global"})
	require.NoError(t, err)
	assert.Equal(t, "global", plan.Target)
}

func TestBuildPlanExplicitPlatformSubset(t *testing.T) {
	plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{bootstrap.PlatformClaude},
	})
	require.NoError(t, err)
	assert.Len(t, plan.Rows, 1)
	assert.Equal(t, bootstrap.PlatformClaude, plan.Rows[0].Platform)
}

func TestBuildPlanRejectsUnknownPlatform(t *testing.T) {
	_, err := bootstrap.BuildPlan(bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{"unknown-harness"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown platform")
}

func TestBuildPlanUnsupportedPlatformSkillsIsUnsupported(t *testing.T) {
	// Antigravity is known but unverified for skills
	plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{bootstrap.PlatformAntigravity},
	})
	require.NoError(t, err)
	row := findRow(t, plan, bootstrap.PlatformAntigravity)
	assert.Equal(t, bootstrap.ActionUnsupported, row.Skills)
}

func TestBuildPlanUnsupportedPlatformHookIsUnsupportedWhenRequested(t *testing.T) {
	// Antigravity has no hook adapter
	plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{
		Platforms: []bootstrap.Platform{bootstrap.PlatformAntigravity},
		WithHooks: true,
	})
	require.NoError(t, err)
	row := findRow(t, plan, bootstrap.PlatformAntigravity)
	assert.Equal(t, bootstrap.ActionUnsupported, row.HarnessHookConfig)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/bootstrap/... -v
```

Expected: FAIL — BuildPlan is not defined.

- [ ] **Step 3: Implement BuildPlan in `internal/bootstrap/planner.go`**

Add this import at the top of planner.go:

```go
import "fmt"
```

Then add the function:

```go
// BuildPlan validates the request and returns a declarative harness plan.
// Returns an error if any explicitly requested platform is not known to Armature.
func BuildPlan(req PlanRequest) (Plan, error) {
	platforms := req.Platforms
	if len(platforms) == 0 {
		platforms = DefaultPlatforms()
	} else {
		known := make(map[Platform]bool, len(allKnownPlatforms))
		for _, p := range allKnownPlatforms {
			known[p] = true
		}
		for _, p := range platforms {
			if !known[p] {
				return Plan{}, fmt.Errorf("unknown platform %q: known platforms are claude, codex, antigravity, devin", p)
			}
		}
	}

	target := req.Target
	if target == "" {
		target = "local"
	}

	rows := make([]PlatformRow, 0, len(platforms))
	for _, p := range platforms {
		row := PlatformRow{Platform: p}

		if verifiedSkills[p] {
			row.Skills = ActionInstall
		} else {
			row.Skills = ActionUnsupported
		}

		if verifiedPluginMetadata[p] {
			row.PluginMetadata = ActionInstall
		} else {
			row.PluginMetadata = ActionUnsupported
		}

		if req.WithHooks {
			if verifiedHarnessHookConfig[p] {
				row.HarnessHookConfig = ActionInstall
			} else {
				row.HarnessHookConfig = ActionUnsupported
			}
		} else {
			row.HarnessHookConfig = ActionSkip
		}

		rows = append(rows, row)
	}

	return Plan{Target: target, Rows: rows}, nil
}
```

- [ ] **Step 4: Run all planner tests**

```bash
go test ./internal/bootstrap/... -v
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bootstrap/planner.go internal/bootstrap/planner_test.go
git commit -m "feat: implement BuildPlan in harness setup planner"
```

---

## Chunk 2: Deploy Helpers

### Task 3: Create bootstrap_deploy.go

Move `deploySkills`, `deployFlatSkills`, `copySkillWithRewrittenRefs`, `deployPlugin`, and `copyFile` from `install_skills.go` into a new file. Do not delete `install_skills.go` yet — that happens in Task 5. Both files will temporarily coexist; duplicate symbols will cause a compile error, so the move must be done in a single atomic step.

**Files:**
- Create: `cmd/armature/bootstrap_deploy.go`

- [ ] **Step 1: Write tests for the deploy helpers in bootstrap_test.go**

```go
// cmd/armature/bootstrap_test.go
package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeBootstrapTestFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"skills/demo-skill/SKILL.md": {Data: []byte("# demo-skill\nA demo skill.\n")},
		"plugin.json":                {Data: []byte(`{"name":"armature"}`)},
	}
}

func TestBootstrapDeploySkillsCopiesFiles(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := t.TempDir()
	require.NoError(t, deploySkills(src, dest))
	content, err := os.ReadFile(filepath.Join(dest, "demo-skill", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "demo-skill")
}

func TestBootstrapDeploySkillsIdempotent(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := t.TempDir()
	require.NoError(t, deploySkills(src, dest))
	require.NoError(t, deploySkills(src, dest))
}

func TestBootstrapDeployFlatSkillsWritesFlatMD(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := t.TempDir()
	require.NoError(t, deploySkills(src, dest))
	require.NoError(t, deployFlatSkills(src, dest))
	content, err := os.ReadFile(filepath.Join(dest, "demo-skill.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "demo-skill")
}

func TestBootstrapDeployPluginCreatesPluginJSON(t *testing.T) {
	src := makeBootstrapTestFS(t)
	dest := t.TempDir()
	require.NoError(t, deployPlugin(src, dest))
	_, err := os.Stat(filepath.Join(dest, "plugin.json"))
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/armature/... -run TestBootstrapDeploy -v
```

Expected: FAIL — bootstrap_deploy.go does not exist.

- [ ] **Step 3: Create `cmd/armature/bootstrap_deploy.go`**

Copy the body of `deploySkills`, `deployFlatSkills`, `copySkillWithRewrittenRefs`, `deployPlugin`, and `copyFile` from `install_skills.go`. The package declaration and imports should be identical to the existing file.

```go
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// deploySkills copies all skills from src (rooted at the "skills" directory)
// into dest, creating subdirectories as needed. Idempotent — existing files
// are overwritten.
func deploySkills(src fs.FS, dest string) error {
	const skillsRoot = "skills"
	return fs.WalkDir(src, skillsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, skillsRoot)
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", target, err)
		}
		return copyFile(src, path, target)
	})
}

// deployFlatSkills writes a flat <name>.md file alongside each skill directory
// so the Skill tool can load skills by name.
func deployFlatSkills(src fs.FS, dest string) error {
	const skillsRoot = "skills"
	entries, err := fs.ReadDir(src, skillsRoot)
	if err != nil {
		return fmt.Errorf("read skills root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		skillFile := skillsRoot + "/" + name + "/SKILL.md"
		target := filepath.Join(dest, name+".md")
		if err := copySkillWithRewrittenRefs(src, skillFile, name, target); err != nil {
			return fmt.Errorf("deploy flat skill %s: %w", name, err)
		}
	}
	return nil
}

func copySkillWithRewrittenRefs(src fs.FS, srcPath, skillName, destPath string) error {
	content, err := fs.ReadFile(src, srcPath)
	if err != nil {
		return fmt.Errorf("read source %s: %w", srcPath, err)
	}
	rewritten := strings.ReplaceAll(string(content), "references/", skillName+"/references/")
	if err := os.WriteFile(destPath, []byte(rewritten), 0o600); err != nil {
		return fmt.Errorf("write dest %s: %w", destPath, err)
	}
	return nil
}

func deployPlugin(src fs.FS, dest string) error {
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("create plugin directory %s: %w", dest, err)
	}
	return copyFile(src, "plugin.json", filepath.Join(dest, "plugin.json"))
}

func copyFile(src fs.FS, srcPath, destPath string) error {
	in, err := src.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source %s: %w", srcPath, err)
	}
	defer in.Close() //nolint:errcheck

	out, err := os.Create(destPath) //nolint:gosec // G304: destPath constructed from internal skills dir
	if err != nil {
		return fmt.Errorf("create dest %s: %w", destPath, err)
	}
	defer out.Close() //nolint:errcheck

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s: %w", srcPath, err)
	}
	return nil
}
```

- [ ] **Step 4: Verify the package compiles (duplicate symbols will be an error)**

```bash
go build ./cmd/armature/...
```

Expected: FAIL with "already declared" errors — because `install_skills.go` still declares the same functions.

- [ ] **Step 5: Remove the duplicated functions from `install_skills.go`**

Delete the bodies of `deploySkills`, `deployFlatSkills`, `copySkillWithRewrittenRefs`, `deployPlugin`, and `copyFile` from `install_skills.go`. The `newInstallSkillsCmd` function must stay (it still uses these helpers). The helpers it calls will now be resolved from `bootstrap_deploy.go`.

After editing, `install_skills.go` should contain only the import block and `newInstallSkillsCmd`. Remove any imports that are no longer needed in that file (fmt, io, io/fs, os, path/filepath, strings — all used by the helpers that moved).

The remaining imports in `install_skills.go` after the move will be:
```go
import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 6: Verify build and tests pass**

```bash
go build ./cmd/armature/... && go test ./cmd/armature/... -run TestBootstrapDeploy -v
```

Expected: build succeeds, tests PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/armature/bootstrap_deploy.go cmd/armature/install_skills.go cmd/armature/bootstrap_test.go
git commit -m "refactor: extract deploy helpers into bootstrap_deploy.go"
```

---

## Chunk 3: Bootstrap Command

### Task 4: Bootstrap command — repo setup

**Files:**
- Create: `cmd/armature/bootstrap.go`

- [ ] **Step 1: Write failing tests for repo setup in `cmd/armature/bootstrap_test.go`**

```go
// Add to bootstrap_test.go

func TestBootstrapCommandInitializesRepo(t *testing.T) {
	repo := initTempRepo(t)
	out, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	assert.Contains(t, out, "Bootstrap")

	// Repo setup: .armature directory should exist
	_, statErr := os.Stat(filepath.Join(repo, ".armature", "ops"))
	require.NoError(t, statErr)
}

func TestBootstrapCommandIsIdempotent(t *testing.T) {
	repo := initTempRepo(t)
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
}

func TestBootstrapCommandCreatesWorkerIdentity(t *testing.T) {
	repo := initTempRepo(t)
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	workerPath := filepath.Join(repo, ".armature", "worker.json")
	_, statErr := os.Stat(workerPath)
	require.NoError(t, statErr)
}

func TestBootstrapCommandDeploysSkills(t *testing.T) {
	repo := initTempRepo(t)
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// At least one skill directory should exist
	skillsDir := filepath.Join(repo, ".claude", "skills")
	entries, readErr := os.ReadDir(skillsDir)
	require.NoError(t, readErr)
	assert.NotEmpty(t, entries)
}

func TestBootstrapCommandDeploysPluginMetadata(t *testing.T) {
	repo := initTempRepo(t)
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	pluginPath := filepath.Join(repo, ".claude", "plugins", "armature", "plugin.json")
	_, statErr := os.Stat(pluginPath)
	require.NoError(t, statErr)
}

func TestBootstrapCommandGlobalDeploysToHome(t *testing.T) {
	repo := initTempRepo(t)
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	_, err := runTrls(t, repo, "bootstrap", "--global")
	require.NoError(t, err)

	skillsDir := filepath.Join(fakeHome, ".claude", "skills")
	_, statErr := os.Stat(skillsDir)
	require.NoError(t, statErr)
}

func TestBootstrapWithHooksDeploysHookConfig(t *testing.T) {
	repo := initTempRepo(t)
	_, err := runTrls(t, repo, "bootstrap", "--with-hooks")
	require.NoError(t, err)

	settingsPath := filepath.Join(repo, ".claude", "settings.json")
	_, statErr := os.Stat(settingsPath)
	require.NoError(t, statErr)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/armature/... -run "TestBootstrapCommand|TestBootstrapWithHooks|TestBootstrapCommandGlobal" -v
```

Expected: FAIL — `bootstrap` subcommand does not exist.

- [ ] **Step 3: Wire `bootstrap` into `cmd/armature/main.go`**

In `newRootCmd()`, add before the `return root` line:

```go
bootstrapCmd := newBootstrapCmd()
bootstrapCmd.GroupID = "admin"
root.AddCommand(bootstrapCmd)
```

`newBootstrapCmd` does not exist yet — this will cause a compile error until Step 4 creates it. That is expected; do both steps before building.

- [ ] **Step 4: Create `cmd/armature/bootstrap.go`**

`init.go` still exists and must not be touched here. `bootstrap.go` calls `runInit` (still in `init.go`) as a temporary delegation point. Task 5 will inline the logic and delete `init.go`.

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/spf13/cobra"
)

// HarnessArtifactResult is the outcome for one platform/artifact cell.
type HarnessArtifactResult struct {
	Platform string
	Artifact string
	Action   string
	Err      error
}

func newBootstrapCmd() *cobra.Command {
	var dualBranch bool
	var global bool
	var platforms []string
	var withHooks bool

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap Armature in the current repository (replaces arm init and arm install-skills)",
		Long: "Idempotently initialises repository structure, worker identity, and git hooks," +
			" then deploys bundled skills and plugin metadata for each supported platform." +
			" Pass --with-hooks to also write harness hook configuration." +
			" Pass --global to deploy harness artifacts to ~/.claude/ instead of .claude/.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, _ := cmd.Flags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}
			absRepo, err := filepath.Abs(repoPath)
			if err != nil {
				return fmt.Errorf("resolve repo path: %w", err)
			}

			// Repo setup (always runs, uses local path)
			if err := runInit(cmd, absRepo, dualBranch); err != nil {
				return fmt.Errorf("repo setup: %w", err)
			}

			// Resolve harness deploy base
			var destBase string
			if global {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("resolve home directory: %w", err)
				}
				destBase = home
			} else {
				destBase = absRepo
			}

			// Build harness plan
			var requestedPlatforms []bootstrap.Platform
			for _, p := range platforms {
				requestedPlatforms = append(requestedPlatforms, bootstrap.Platform(p))
			}
			target := "local"
			if global {
				target = "global"
			}
			plan, err := bootstrap.BuildPlan(bootstrap.PlanRequest{
				Platforms: requestedPlatforms,
				Target:    target,
				WithHooks: withHooks,
			})
			if err != nil {
				return fmt.Errorf("build bootstrap plan: %w", err)
			}

			// Execute harness setup
			results := executeHarnessSetup(plan, destBase)

			// Report
			return reportBootstrap(cmd, results)
		},
	}

	cmd.Flags().BoolVar(&dualBranch, "dual-branch", false, "initialise in dual-branch mode")
	cmd.Flags().BoolVar(&global, "global", false, "deploy harness artifacts to user home (~/.claude/) instead of .claude/")
	cmd.Flags().StringSliceVar(&platforms, "platform", nil, "restrict to specific platform(s): claude, codex, antigravity, devin")
	cmd.Flags().BoolVar(&withHooks, "with-hooks", false, "also install harness hook configuration (opt-in)")
	return cmd
}

// executeHarnessSetup walks the plan matrix and executes each install action.
func executeHarnessSetup(plan bootstrap.Plan, destBase string) []HarnessArtifactResult {
	var results []HarnessArtifactResult

	skillsDest := filepath.Join(destBase, ".claude", "skills")
	pluginDest := filepath.Join(destBase, ".claude", "plugins", "armature")

	for _, row := range plan.Rows {
		p := string(row.Platform)

		// skills
		results = append(results, executeSkills(p, row.Skills, skillsDest))

		// plugin_metadata
		results = append(results, executePluginMetadata(p, row.PluginMetadata, pluginDest))

		// harness_hook_config
		results = append(results, executeHarnessHookConfig(p, row.HarnessHookConfig, destBase))
	}
	return results
}

func executeSkills(platform string, action bootstrap.ActionKind, skillsDest string) HarnessArtifactResult {
	r := HarnessArtifactResult{Platform: platform, Artifact: "skills", Action: string(action)}
	if action != bootstrap.ActionInstall {
		return r
	}
	if err := deploySkills(skillsembed.SkillsFS, skillsDest); err != nil {
		r.Err = err
		return r
	}
	r.Err = deployFlatSkills(skillsembed.SkillsFS, skillsDest)
	return r
}

func executePluginMetadata(platform string, action bootstrap.ActionKind, pluginDest string) HarnessArtifactResult {
	r := HarnessArtifactResult{Platform: platform, Artifact: "plugin_metadata", Action: string(action)}
	if action != bootstrap.ActionInstall {
		return r
	}
	r.Err = deployPlugin(skillsembed.SkillsFS, pluginDest)
	return r
}

func executeHarnessHookConfig(platform string, action bootstrap.ActionKind, destBase string) HarnessArtifactResult {
	r := HarnessArtifactResult{Platform: platform, Artifact: "harness_hook_config", Action: string(action)}
	if action != bootstrap.ActionInstall {
		return r
	}
	adapter, err := harnesshook.NewAdapterForPlatform(platform)
	if err != nil {
		r.Err = err
		return r
	}
	r.Err = adapter.WriteConfig(destBase)
	return r
}

// reportBootstrap writes terse success output or a grouped failure summary.
func reportBootstrap(cmd *cobra.Command, results []HarnessArtifactResult) error {
	var errs []string
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, fmt.Sprintf("  %s/%s: %v", r.Platform, r.Artifact, r.Err))
		}
	}
	if len(errs) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Bootstrap complete.")
		return nil
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Bootstrap completed with errors:\n\nharness_setup:")
	for _, e := range errs {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), e)
	}
	return fmt.Errorf("bootstrap: %s", strings.Join(errs, "; "))
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./cmd/armature/... -run "TestBootstrapCommand|TestBootstrapWithHooks|TestBootstrapCommandGlobal" -v
```

Expected: all PASS

- [ ] **Step 6: Run full test suite to check for regressions**

```bash
make test
```

Expected: all tests pass (init and install-skills are still registered).

- [ ] **Step 7: Commit**

```bash
git add cmd/armature/bootstrap.go cmd/armature/bootstrap_test.go cmd/armature/main.go
git commit -m "feat: add arm bootstrap command with repo and harness setup"
```

---

## Chunk 4: Remove Old Commands

### Task 5: Remove old commands — inline init logic and delete old files

This task makes the cutover: inline `init.go` logic into `bootstrap.go`, then delete `init.go` and `install_skills.go`, and remove their registrations from `main.go`.

**Files:**
- Delete: `cmd/armature/init.go`
- Delete: `cmd/armature/install_skills.go`
- Delete: `cmd/armature/install_skills_test.go`
- Modify: `cmd/armature/main.go`
- Modify: `cmd/armature/bootstrap.go`

- [ ] **Step 1: Write a regression test that confirms bootstrap replaces init behaviour**

Add to `bootstrap_test.go`:

```go
func TestBootstrapCommandWritesGitignore(t *testing.T) {
	repo := initTempRepo(t)
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	content, readErr := os.ReadFile(filepath.Join(repo, ".armature", ".gitignore"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "state/")
}

func TestBootstrapCommandWritesHookTemplates(t *testing.T) {
	repo := initTempRepo(t)
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(repo, ".armature", "hooks", "post-merge.sh.template"))
	require.NoError(t, statErr)
}
```

- [ ] **Step 2: Run tests to make sure they pass before the deletion**

```bash
go test ./cmd/armature/... -run "TestBootstrapCommandWritesGitignore|TestBootstrapCommandWritesHookTemplates" -v
```

Expected: PASS (bootstrap.go delegates to runInit which is still in init.go).

- [ ] **Step 3: Inline init.go logic into bootstrap.go**

In `bootstrap.go`:
1. Copy the hook template constants (`issuesGitignore`, `postMergeHookTemplate`, `postCommitHookTemplate`, `prepareCommitMsgHookTemplate`, `preCommitHookTemplate`) from `init.go` into `bootstrap.go`.
2. Copy the `installHooks` function from `init.go` into `bootstrap.go`.
3. Copy the body of `runInit` from `init.go` into `bootstrap.go`, renaming it `runRepoSetup`.
4. In `newBootstrapCmd`, replace the call to `runInit(cmd, absRepo, dualBranch)` with `runRepoSetup(io.Discard, absRepo, dualBranch)`.

Update the import block in `bootstrap.go` to add `"io"` and the new internal packages. The final import block should be:

```go
import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)
```

The `runRepoSetup` function output message should say "Bootstrap complete." instead of "Initialized Armature in..." — but only the harness setup output is needed; suppress the `runRepoSetup` output by passing a `io.Discard` writer. The bootstrap command owns the success message, not the sub-functions.

> Tip: In `runRepoSetup`, replace `cmd.OutOrStdout()` with a `io.Writer` parameter. Pass `io.Discard` from `newBootstrapCmd` so the repo setup step is silent on success.

Updated `runRepoSetup` signature:
```go
func runRepoSetup(out io.Writer, repoPath string, dualBranch bool) error
```

- [ ] **Step 4: Build to verify no duplicate symbol errors**

```bash
go build ./cmd/armature/...
```

Expected: succeeds — `runRepoSetup` is new, `runInit` still exists in `init.go`; no duplicate symbols yet.

- [ ] **Step 5: Delete `cmd/armature/init.go`**

```bash
rm cmd/armature/init.go
```

- [ ] **Step 6: Build to verify init.go removal didn't break anything**

```bash
go build ./cmd/armature/...
```

Expected: succeeds (bootstrap.go now has all the init logic).

- [ ] **Step 7: Remove `init` and `install-skills` from `cmd/armature/main.go`**

The `bootstrap` command was already registered in Task 4. Now remove the old registrations:

Find and delete these blocks from `newRootCmd()`:
```go
initCmd := newInitCmd()
initCmd.GroupID = "admin"
root.AddCommand(initCmd)
```
and:
```go
installSkillsCmd := newInstallSkillsCmd()
installSkillsCmd.GroupID = "admin"
root.AddCommand(installSkillsCmd)
```

- [ ] **Step 8: Delete `cmd/armature/install_skills.go` and `cmd/armature/install_skills_test.go`**

```bash
rm cmd/armature/install_skills.go cmd/armature/install_skills_test.go
```

- [ ] **Step 9: Run make check**

```bash
make check
```

Expected: green. The `arm init` and `arm install-skills` tests are gone; `arm bootstrap` tests cover the equivalent behaviour. Fix any lint or coverage failures before proceeding.

- [ ] **Step 10: Commit**

```bash
git add cmd/armature/bootstrap.go cmd/armature/bootstrap_test.go cmd/armature/main.go
git rm cmd/armature/init.go cmd/armature/install_skills.go cmd/armature/install_skills_test.go
git commit -m "feat: replace arm init and arm install-skills with arm bootstrap"
```

---

## Chunk 4b: Ownership Markers

Armature must not overwrite harness hook config or git hooks it does not own. This chunk adds:
- `OwnsConfig(workdir string) (bool, error)` to the `PlatformAdapter` interface
- Per-platform ownership detection and write markers
- Ownership-aware `installHooks` for git hooks
- Updated `executeHarnessHookConfig` in bootstrap.go to call `OwnsConfig` before `WriteConfig`
- A richer `HarnessArtifactResult` with `Status` and `Note` fields

**Ownership strategy:**
- **Claude** (`.claude/settings.json`): key-based merge — Armature only touches the `hooks` key. `OwnsConfig` always returns `true`.
- **Codex** (`codex.toml`): file-level ownership. `WriteConfig` prepends `# armature:managed`. `OwnsConfig` checks for that marker.
- **Devin** (`.devin/hooks.json`): file-level ownership. `WriteConfig` adds `"_armature": "managed"` key. `OwnsConfig` checks for that key.
- **Git hooks** (`.git/hooks/*`): file-level ownership. Hook templates include `# armature:managed` on the second line. `installHooks` skips files that exist without the marker.

### Task 5b: Add OwnsConfig to PlatformAdapter interface and implement per platform

**Files:**
- Modify: `internal/harnesshook/types.go`
- Modify: `internal/harnesshook/platform_claude.go`
- Modify: `internal/harnesshook/platform_codex.go`
- Modify: `internal/harnesshook/platform_devin.go`
- Modify: `internal/harnesshook/platform_test.go`

- [ ] **Step 1: Write failing tests for OwnsConfig**

```go
// Add to internal/harnesshook/platform_test.go

func TestClaudeAdapterOwnsConfigAlwaysTrue(t *testing.T) {
	t.Parallel()
	adapter := NewClaudeAdapter()
	dir := t.TempDir()

	owned, err := adapter.OwnsConfig(dir)
	require.NoError(t, err)
	assert.True(t, owned, "Claude adapter owns config via key-merge — always safe to write")
}

func TestCodexAdapterOwnsConfigTrueWhenFileAbsent(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	owned, err := adapter.OwnsConfig(t.TempDir())
	require.NoError(t, err)
	assert.True(t, owned)
}

func TestCodexAdapterOwnsConfigTrueAfterWriteConfig(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	dir := t.TempDir()
	require.NoError(t, adapter.WriteConfig(dir))

	owned, err := adapter.OwnsConfig(dir)
	require.NoError(t, err)
	assert.True(t, owned)
}

func TestCodexAdapterOwnsConfigFalseForUserManagedFile(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "codex.toml"), []byte("[settings]\nmodel = \"gpt-4\"\n"), 0o600))

	owned, err := adapter.OwnsConfig(dir)
	require.NoError(t, err)
	assert.False(t, owned)
}

func TestDevinAdapterOwnsConfigTrueWhenFileAbsent(t *testing.T) {
	t.Parallel()
	adapter := NewDevinAdapter()
	owned, err := adapter.OwnsConfig(t.TempDir())
	require.NoError(t, err)
	assert.True(t, owned)
}

func TestDevinAdapterOwnsConfigTrueAfterWriteConfig(t *testing.T) {
	t.Parallel()
	adapter := NewDevinAdapter()
	dir := t.TempDir()
	require.NoError(t, adapter.WriteConfig(dir))

	owned, err := adapter.OwnsConfig(dir)
	require.NoError(t, err)
	assert.True(t, owned)
}

func TestDevinAdapterOwnsConfigFalseForUserManagedFile(t *testing.T) {
	t.Parallel()
	adapter := NewDevinAdapter()
	dir := t.TempDir()
	devinDir := filepath.Join(dir, ".devin")
	require.NoError(t, os.MkdirAll(devinDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(devinDir, "hooks.json"),
		[]byte(`{"hooks":{"PreToolUse":[{"command":"my-hook"}]}}`), 0o600))

	owned, err := adapter.OwnsConfig(dir)
	require.NoError(t, err)
	assert.False(t, owned)
}

func TestCodexAdapterWriteConfigIncludesManagedMarker(t *testing.T) {
	t.Parallel()
	adapter := NewCodexAdapter()
	dir := t.TempDir()
	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, "codex.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "# armature:managed")
}

func TestDevinAdapterWriteConfigIncludesManagedKey(t *testing.T) {
	t.Parallel()
	adapter := NewDevinAdapter()
	dir := t.TempDir()
	require.NoError(t, adapter.WriteConfig(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".devin", "hooks.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"_armature"`)
	assert.Contains(t, string(data), `"managed"`)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/harnesshook/... -run "TestClaudeAdapterOwnsConfig|TestCodexAdapterOwnsConfig|TestDevinAdapterOwnsConfig|TestCodexAdapterWriteConfigIncludesManagedMarker|TestDevinAdapterWriteConfigIncludesManagedKey" -v
```

Expected: FAIL — `OwnsConfig` not defined.

- [ ] **Step 3: Add `OwnsConfig` to the `PlatformAdapter` interface in `internal/harnesshook/types.go`**

```go
// OwnsConfig reports whether the config at workdir is absent or was created by
// Armature. If true, WriteConfig may safely overwrite it. If false, the file
// exists with user-managed content and must not be touched.
OwnsConfig(workdir string) (bool, error)
```

Add this method to the `PlatformAdapter` interface after `WriteConfig`.

- [ ] **Step 4: Implement `OwnsConfig` in Claude adapter (`internal/harnesshook/platform_claude.go`)**

```go
// OwnsConfig always returns true for Claude because WriteConfig only merges the
// "hooks" key into the existing JSON, leaving all other keys untouched.
func (a *ClaudeAdapter) OwnsConfig(_ string) (bool, error) {
	return true, nil
}
```

- [ ] **Step 5: Implement `OwnsConfig` in Codex adapter and update `WriteConfig` (`internal/harnesshook/platform_codex.go`)**

Add `"strings"` and `"os"` to imports (they may already be there). Then add:

```go
const codexManagedMarker = "# armature:managed"

// OwnsConfig returns true if codex.toml is absent or contains the armature marker.
func (a *CodexAdapter) OwnsConfig(workdir string) (bool, error) {
	content, err := os.ReadFile(filepath.Join(workdir, "codex.toml")) //nolint:gosec // internal path
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(string(content), codexManagedMarker), nil
}
```

Update `WriteConfig` to prepend the marker:

```go
func (a *CodexAdapter) WriteConfig(workdir string) error {
	content := codexManagedMarker + "\n[hooks]\npre_tool_use = \"arm harness-hook\"\nstop = \"arm harness-hook\"\n"
	return os.WriteFile(filepath.Join(workdir, "codex.toml"), []byte(content), 0o600)
}
```

- [ ] **Step 6: Implement `OwnsConfig` in Devin adapter and update `WriteConfig` (`internal/harnesshook/platform_devin.go`)**

```go
// OwnsConfig returns true if .devin/hooks.json is absent or contains the armature managed key.
func (a *DevinAdapter) OwnsConfig(workdir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(workdir, ".devin", "hooks.json")) //nolint:gosec // internal path
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false, nil // unparseable JSON is treated as user-managed
	}
	managed, _ := cfg["_armature"].(string)
	return managed == "managed", nil
}
```

Update `WriteConfig` to add the managed key:

```go
func (a *DevinAdapter) WriteConfig(workdir string) error {
	dir := filepath.Join(workdir, ".devin")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	cfg := map[string]any{
		"_armature": "managed",
		"hooks": map[string]any{
			"PreToolUse": []any{map[string]any{
				"matcher": "edit|exec",
				"command": "arm harness-hook",
			}},
			"Stop": []any{map[string]any{
				"command": "arm harness-hook",
			}},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "hooks.json"), data, 0o600)
}
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/harnesshook/... -v
```

Expected: all PASS. The existing `TestClaudeAdapterWriteConfigPreservesExistingSettings` must still pass — it tests that non-hooks keys survive a `WriteConfig` call, which is unchanged.

- [ ] **Step 8: Commit**

```bash
git add internal/harnesshook/types.go internal/harnesshook/platform_claude.go internal/harnesshook/platform_codex.go internal/harnesshook/platform_devin.go internal/harnesshook/platform_test.go
git commit -m "feat: add OwnsConfig to PlatformAdapter; write ownership markers in codex and devin configs"
```

---

### Task 5c: Ownership-aware installHooks and executeHarnessHookConfig

**Files:**
- Modify: `cmd/armature/bootstrap.go`

- [ ] **Step 1: Write failing tests**

```go
// Add to cmd/armature/bootstrap_test.go

func TestBootstrapWithHooksSkipsUserManagedCodexConfig(t *testing.T) {
	repo := initTempRepo(t)
	// Pre-create a user-managed codex.toml (no armature marker)
	require.NoError(t, os.WriteFile(
		filepath.Join(repo, "codex.toml"),
		[]byte("[settings]\nmodel = \"gpt-4\"\n"),
		0o600,
	))

	out, err := runTrls(t, repo, "bootstrap", "--with-hooks", "--platform", "claude,codex")
	require.NoError(t, err)
	// codex.toml must not have been overwritten
	content, _ := os.ReadFile(filepath.Join(repo, "codex.toml"))
	assert.NotContains(t, string(content), "arm harness-hook")
	_ = out
}

func TestBootstrapGitHookSkippedIfUserManaged(t *testing.T) {
	repo := initTempRepo(t)
	// Write a git hook without the armature marker
	hooksDir := filepath.Join(repo, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(hooksDir, "post-merge"),
		[]byte("#!/bin/sh\necho user hook\n"),
		0o755,
	))

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	// User's hook must be untouched
	content, _ := os.ReadFile(filepath.Join(hooksDir, "post-merge"))
	assert.Contains(t, string(content), "user hook")
	assert.NotContains(t, string(content), "armature")
}

func TestBootstrapGitHookOverwrittenIfArmatureManaged(t *testing.T) {
	repo := initTempRepo(t)
	// First bootstrap installs hooks with armature:managed marker
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)

	// Verify the marker is present
	content, _ := os.ReadFile(filepath.Join(repo, ".git", "hooks", "post-merge"))
	require.Contains(t, string(content), "armature:managed")

	// Second bootstrap must succeed and hooks still have marker
	_, err = runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	content2, _ := os.ReadFile(filepath.Join(repo, ".git", "hooks", "post-merge"))
	assert.Contains(t, string(content2), "armature:managed")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/armature/... -run "TestBootstrapWithHooksSkipsUserManagedCodexConfig|TestBootstrapGitHook" -v
```

Expected: `TestBootstrapGitHookSkippedIfUserManaged` FAIL (bootstrap currently overwrites unconditionally); others may vary.

- [ ] **Step 3: Update `HarnessArtifactResult` in `bootstrap.go` to add `Status` and `Note`**

```go
// HarnessArtifactResult is the outcome for one platform/artifact cell.
type HarnessArtifactResult struct {
	Platform string
	Artifact string
	Action   string // planner decision: install | skip | unsupported
	Status   string // actual outcome: ok | skipped | unsupported | error
	Note     string // human-readable reason when status is skipped or unsupported
	Err      error  // set only when status is error
}
```

- [ ] **Step 4: Add `actionToStatus` helper and update all execute* functions to set `Status`**

```go
func actionToStatus(action bootstrap.ActionKind) string {
	switch action {
	case bootstrap.ActionSkip:
		return "skipped"
	case bootstrap.ActionUnsupported:
		return "unsupported"
	default:
		return "ok"
	}
}
```

Update `executeSkills`:
```go
func executeSkills(platform string, action bootstrap.ActionKind, skillsDest string) HarnessArtifactResult {
	r := HarnessArtifactResult{Platform: platform, Artifact: "skills", Action: string(action)}
	if action != bootstrap.ActionInstall {
		r.Status = actionToStatus(action)
		return r
	}
	if err := deploySkills(skillsembed.SkillsFS, skillsDest); err != nil {
		r.Status = "error"
		r.Err = err
		return r
	}
	if err := deployFlatSkills(skillsembed.SkillsFS, skillsDest); err != nil {
		r.Status = "error"
		r.Err = err
		return r
	}
	r.Status = "ok"
	return r
}
```

Update `executePluginMetadata`:
```go
func executePluginMetadata(platform string, action bootstrap.ActionKind, pluginDest string) HarnessArtifactResult {
	r := HarnessArtifactResult{Platform: platform, Artifact: "plugin_metadata", Action: string(action)}
	if action != bootstrap.ActionInstall {
		r.Status = actionToStatus(action)
		return r
	}
	if err := deployPlugin(skillsembed.SkillsFS, pluginDest); err != nil {
		r.Status = "error"
		r.Err = err
		return r
	}
	r.Status = "ok"
	return r
}
```

Update `executeHarnessHookConfig` to call `OwnsConfig`:
```go
func executeHarnessHookConfig(platform string, action bootstrap.ActionKind, destBase string) HarnessArtifactResult {
	r := HarnessArtifactResult{Platform: platform, Artifact: "harness_hook_config", Action: string(action)}
	if action != bootstrap.ActionInstall {
		r.Status = actionToStatus(action)
		return r
	}
	adapter, err := harnesshook.NewAdapterForPlatform(platform)
	if err != nil {
		r.Status = "error"
		r.Err = err
		return r
	}
	owned, err := adapter.OwnsConfig(destBase)
	if err != nil {
		r.Status = "error"
		r.Err = fmt.Errorf("check config ownership: %w", err)
		return r
	}
	if !owned {
		r.Status = "skipped"
		r.Note = "existing config not managed by Armature"
		return r
	}
	if err := adapter.WriteConfig(destBase); err != nil {
		r.Status = "error"
		r.Err = err
		return r
	}
	r.Status = "ok"
	return r
}
```

- [ ] **Step 5: Update hook templates to include `# armature:managed` and update `installHooks`**

In `bootstrap.go`, update each hook template constant to include `# armature:managed` as the second line:

```go
const postMergeHookTemplate = `#!/bin/sh
# armature:managed
# Armature post-merge hook: ...
```

Apply the same change to `postCommitHookTemplate`, `prepareCommitMsgHookTemplate`, and `preCommitHookTemplate`.

Update `installHooks` to check for the marker before overwriting:

```go
func installHooks(repoPath string, issuesDir string) (skippedHooks []string, err error) {
	hooksDir := filepath.Join(issuesDir, "hooks")
	gitHooksDir := filepath.Join(repoPath, ".git", "hooks")

	if err := os.MkdirAll(gitHooksDir, 0o750); err != nil {
		return nil, fmt.Errorf("create .git/hooks directory: %w", err)
	}

	hooks := []string{"pre-commit", "post-commit", "post-merge", "prepare-commit-msg"}

	for _, hook := range hooks {
		templatePath := filepath.Join(hooksDir, hook+".sh.template")
		hookPath := filepath.Join(gitHooksDir, hook)

		content, err := os.ReadFile(templatePath) //nolint:gosec // internal hooks dir
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return skippedHooks, fmt.Errorf("read hook template %s: %w", hook, err)
		}

		// Ownership check: skip if file exists without the armature marker.
		if existing, readErr := os.ReadFile(hookPath); readErr == nil { //nolint:gosec // internal path
			if !strings.Contains(string(existing), "# armature:managed") {
				skippedHooks = append(skippedHooks, hook)
				continue
			}
		} else if !os.IsNotExist(readErr) {
			return skippedHooks, fmt.Errorf("check hook %s: %w", hook, readErr)
		}

		if err := os.WriteFile(hookPath, content, 0o755); err != nil { //nolint:gosec // git hooks require executable bit
			return skippedHooks, fmt.Errorf("install hook %s: %w", hook, err)
		}
	}
	return skippedHooks, nil
}
```

Update `runRepoSetup` to propagate skipped hooks. Change its return type:

```go
type RepoSetupResult struct {
	SkippedHooks []string // hook names skipped due to user-managed content
}

func runRepoSetup(out io.Writer, repoPath string, dualBranch bool) (RepoSetupResult, error) {
```

Inside `runRepoSetup`, replace the `installHooks` call with:
```go
skipped, err := installHooks(repoPath, issuesDir)
if err != nil {
    return RepoSetupResult{}, fmt.Errorf("install hooks: %w", err)
}
result := RepoSetupResult{SkippedHooks: skipped}
```

Update the call site in `newBootstrapCmd`:
```go
repoResult, err := runRepoSetup(io.Discard, absRepo, dualBranch)
if err != nil {
    return fmt.Errorf("repo setup: %w", err)
}
```

Pass `repoResult` through to `reportBootstrap`:
```go
return reportBootstrap(cmd, repoResult, results)
```

Update `reportBootstrap` signature to accept `repoResult`:
```go
func reportBootstrap(cmd *cobra.Command, repoResult RepoSetupResult, results []HarnessArtifactResult) error {
```

For now, human output for skipped hooks: emit a note line per skipped hook.

- [ ] **Step 6: Run all bootstrap and harnesshook tests**

```bash
go test ./internal/harnesshook/... ./cmd/armature/... -v 2>&1 | tail -30
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/armature/bootstrap.go cmd/armature/bootstrap_test.go
git commit -m "feat: ownership-aware installHooks and executeHarnessHookConfig"
```

---

## Chunk 4c: JSON Result Output

### Task 5d: JSON output for arm bootstrap

The bootstrap command must emit JSON when `--format=json` is passed or when stdout is not a terminal (Armature convention). Since bootstrap bypasses root `PersistentPreRunE`, it checks the format flag directly.

**Files:**
- Modify: `cmd/armature/bootstrap.go`

**JSON schema:**

```json
{
  "repo_setup": {
    "status": "ok",
    "skipped_hooks": ["post-merge"]
  },
  "harness_setup": [
    {
      "platform": "claude",
      "artifact": "skills",
      "action": "install",
      "status": "ok"
    },
    {
      "platform": "claude",
      "artifact": "harness_hook_config",
      "action": "install",
      "status": "skipped",
      "note": "existing config not managed by Armature"
    }
  ]
}
```

Exit code: 0 on full success or partial-skip; non-zero only when any `status == "error"`.

- [ ] **Step 1: Write failing tests**

```go
// Add to cmd/armature/bootstrap_test.go

func TestBootstrapEmitsJSONWhenFormatFlagSet(t *testing.T) {
	repo := initTempRepo(t)
	out, err := runTrls(t, repo, "bootstrap", "--format", "json")
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Contains(t, result, "repo_setup")
	assert.Contains(t, result, "harness_setup")
}

func TestBootstrapJSONRepoSetupStatusOK(t *testing.T) {
	repo := initTempRepo(t)
	out, err := runTrls(t, repo, "bootstrap", "--format", "json")
	require.NoError(t, err)

	var result struct {
		RepoSetup struct {
			Status string `json:"status"`
		} `json:"repo_setup"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, "ok", result.RepoSetup.Status)
}

func TestBootstrapJSONHarnessSetupContainsClaudeSkills(t *testing.T) {
	repo := initTempRepo(t)
	out, err := runTrls(t, repo, "bootstrap", "--format", "json")
	require.NoError(t, err)

	var result struct {
		HarnessSetup []struct {
			Platform string `json:"platform"`
			Artifact string `json:"artifact"`
			Status   string `json:"status"`
		} `json:"harness_setup"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	found := false
	for _, row := range result.HarnessSetup {
		if row.Platform == "claude" && row.Artifact == "skills" {
			assert.Equal(t, "ok", row.Status)
			found = true
		}
	}
	assert.True(t, found, "expected claude/skills row in harness_setup")
}

func TestBootstrapJSONSkippedHookReflectedInRepoSetup(t *testing.T) {
	repo := initTempRepo(t)
	hooksDir := filepath.Join(repo, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(hooksDir, "post-merge"),
		[]byte("#!/bin/sh\necho user hook\n"),
		0o755,
	))

	out, err := runTrls(t, repo, "bootstrap", "--format", "json")
	require.NoError(t, err)

	var result struct {
		RepoSetup struct {
			Status       string   `json:"status"`
			SkippedHooks []string `json:"skipped_hooks"`
		} `json:"repo_setup"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, "ok", result.RepoSetup.Status)
	assert.Contains(t, result.RepoSetup.SkippedHooks, "post-merge")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/armature/... -run "TestBootstrapEmitsJSON|TestBootstrapJSON" -v
```

Expected: FAIL — bootstrap emits human text, not JSON.

- [ ] **Step 3: Add JSON output types and update `reportBootstrap` in `bootstrap.go`**

Add the JSON types:

```go
// bootstrapJSONOutput is the machine-readable result of arm bootstrap.
type bootstrapJSONOutput struct {
	RepoSetup    repoSetupJSON    `json:"repo_setup"`
	HarnessSetup []harnessJSON    `json:"harness_setup"`
}

type repoSetupJSON struct {
	Status       string   `json:"status"`
	SkippedHooks []string `json:"skipped_hooks,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type harnessJSON struct {
	Platform string `json:"platform"`
	Artifact string `json:"artifact"`
	Action   string `json:"action"`
	Status   string `json:"status"`
	Note     string `json:"note,omitempty"`
	Error    string `json:"error,omitempty"`
}
```

Update `reportBootstrap` to check format and branch:

```go
func reportBootstrap(cmd *cobra.Command, repoResult RepoSetupResult, results []HarnessArtifactResult) error {
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if format == "" || (format == "human" && !tui.IsTerminal()) {
		format = "json"
	}
	if format == "json" || format == "agent" {
		return reportBootstrapJSON(cmd, repoResult, results)
	}
	return reportBootstrapHuman(cmd, repoResult, results)
}

func reportBootstrapJSON(cmd *cobra.Command, repoResult RepoSetupResult, results []HarnessArtifactResult) error {
	repoStatus := "ok"
	if repoResult.Error != "" {
		repoStatus = "error"
	}

	harness := make([]harnessJSON, 0, len(results))
	hasError := false
	for _, r := range results {
		h := harnessJSON{
			Platform: r.Platform,
			Artifact: r.Artifact,
			Action:   r.Action,
			Status:   r.Status,
			Note:     r.Note,
		}
		if r.Err != nil {
			h.Error = r.Err.Error()
			hasError = true
		}
		harness = append(harness, h)
	}

	out := bootstrapJSONOutput{
		RepoSetup: repoSetupJSON{
			Status:       repoStatus,
			SkippedHooks: repoResult.SkippedHooks,
			Error:        repoResult.Error,
		},
		HarnessSetup: harness,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bootstrap result: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	if hasError || repoStatus == "error" {
		return fmt.Errorf("bootstrap completed with errors")
	}
	return nil
}

func reportBootstrapHuman(cmd *cobra.Command, repoResult RepoSetupResult, results []HarnessArtifactResult) error {
	for _, hook := range repoResult.SkippedHooks {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "note: git hook %s skipped (not managed by Armature)\n", hook)
	}
	var errs []string
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, fmt.Sprintf("  %s/%s: %v", r.Platform, r.Artifact, r.Err))
		} else if r.Status == "skipped" && r.Note != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "note: %s/%s skipped — %s\n", r.Platform, r.Artifact, r.Note)
		}
	}
	if len(errs) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Bootstrap complete.")
		return nil
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Bootstrap completed with errors:\n\nharness_setup:")
	for _, e := range errs {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), e)
	}
	return fmt.Errorf("bootstrap: %s", strings.Join(errs, "; "))
}
```

Also update `RepoSetupResult` to include an `Error` field for the JSON path:

```go
type RepoSetupResult struct {
	SkippedHooks []string
	Error        string // non-empty if repo setup failed (used for JSON output)
}
```

Add `"encoding/json"` to bootstrap.go imports, and add `"github.com/scullxbones/armature/internal/tui"`.

- [ ] **Step 4: Run JSON tests**

```bash
go test ./cmd/armature/... -run "TestBootstrapEmitsJSON|TestBootstrapJSON" -v
```

Expected: all PASS.

- [ ] **Step 5: Run full make check**

```bash
make check
```

Expected: green.

- [ ] **Step 6: Commit**

```bash
git add cmd/armature/bootstrap.go cmd/armature/bootstrap_test.go
git commit -m "feat: JSON output for arm bootstrap (--format json or non-TTY)"
```

---

## Chunk 5: Documentation

### Task 6: Update docs/commands.md

**Files:**
- Modify: `docs/commands.md`

- [ ] **Step 1: Remove the `## init` section from `docs/commands.md`**

Find and delete the entire `## init` section (synopsis, flags, description).

- [ ] **Step 2: Remove the `## install-skills` section from `docs/commands.md`** (if present — check first)

- [ ] **Step 3: Add a `## bootstrap` section to `docs/commands.md`**

Add under the Admin Commands area (after `## doctor`, for example). The section content:

---

**`## bootstrap`**

Bootstrap Armature in the current repository. Replaces `arm init` and `arm install-skills`.

**Synopsis:** `arm bootstrap [flags]`

**Flags:**
- `--dual-branch`: Initialise in dual-branch mode (ops on separate `_armature` branch).
- `--global`: Deploy harness artifacts to `~/.claude/` instead of `.claude/`.
- `--platform strings`: Restrict bootstrap to specific platform(s): `claude`, `codex`, `antigravity`, `devin`.
- `--with-hooks`: Also install harness hook configuration (opt-in; see Harness Hook Integration Guide before enabling).

**Default behaviour:**
- Initialises `.armature/` directory structure and worker identity.
- Installs git hook templates to `.armature/hooks/`.
- Deploys bundled skills and plugin metadata to `.claude/`.
- Skips harness hook configuration unless `--with-hooks` is passed.

**Examples:**

```bash
arm bootstrap
arm bootstrap --global
arm bootstrap --platform claude --with-hooks
```

**See Also:** See `docs/harness-hook.md` for hook setup details.

---

- [ ] **Step 4: Verify docs compile (no broken internal links)**

```bash
grep -rn "arm init\b\|arm install-skills" docs/
```

Expected: no matches.

- [ ] **Step 5: Commit**

```bash
git add docs/commands.md
git commit -m "docs: add arm bootstrap command reference; remove arm init and arm install-skills"
```

---

### Task 7: Update CLAUDE.md, AGENTS.md, and embedded SKILL.md

**Files:**
- Modify: `CLAUDE.md`
- Modify: `AGENTS.md`
- Modify: `internal/skillsembed/skills/armature/SKILL.md`

- [ ] **Step 1: Update `CLAUDE.md`**

Find:
```
Bundled skills are deployed via `arm install-skills` or `make skill` to local
agent directories.
```

Replace with:
```
Bundled skills are deployed via `arm bootstrap` or `make skill` to local
agent directories.
```

- [ ] **Step 2: Update `AGENTS.md`**

```bash
grep -n "install-skills\|arm init" AGENTS.md
```

Replace each occurrence with `arm bootstrap`.

- [ ] **Step 3: Update `internal/skillsembed/skills/armature/SKILL.md`**

Find:
```
arm install-skills                                      # deploy bundled skills to .claude/skills/
```

Replace with:
```
arm bootstrap                                           # bootstrap repo + deploy bundled skills
```

- [ ] **Step 4: Rebuild and redeploy skills**

```bash
make skill
```

This rebuilds the binary and redeploys the updated embedded skills to `.claude/skills/`.

- [ ] **Step 5: Run validate-skills to confirm no banned references remain**

```bash
make validate-skills
```

Expected: PASS

- [ ] **Step 6: Run full make check**

```bash
make check
```

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add CLAUDE.md AGENTS.md internal/skillsembed/skills/armature/SKILL.md
git commit -m "docs: update CLAUDE.md, AGENTS.md, and armature skill to reference arm bootstrap"
```

---

## Previously Deferred — Now Defined

All three items deferred in `docs/design/bootstrap-agent-integration.md` are addressed in this plan:

1. **Ownership markers** — Implemented in Chunks 4b (Task 5b–5c). Git hooks use `# armature:managed` comment. Codex uses the same comment in `codex.toml`. Devin uses `"_armature": "managed"` JSON key. Claude uses key-based merge (always safe). The `OwnsConfig` method on `PlatformAdapter` encapsulates detection.

2. **Verified-contract test thresholds** — Defined in the planner's verification contract comment (Task 1). A platform/artifact is verified when a writer function exists, an integration test exercises `arm bootstrap` and checks the correct output path, and (for hook config) ownership tests cover both managed and unmanaged cases. The planner's `verified*` maps enforce this: no entry without tests.

3. **JSON result schema** — Defined and implemented in Chunk 4c (Task 5d). Schema: `repo_setup` (status, skipped_hooks, error) + `harness_setup` array (platform, artifact, action, status, note, error). Emitted when `--format=json` or when stdout is not a terminal.
