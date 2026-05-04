# Skill Optimization: Progressive Disclosure + Metadata Merge — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge `meta.yaml` into `SKILL.md` frontmatter, move `skills/` into `internal/skillsembed/`, add `arm install-skills` subcommand, trim skill bodies with progressive disclosure to references files.

**Architecture:** Five skills live in `internal/skillsembed/skills/` (embedded via `//go:embed`). The `arm install-skills` command walks the embedded FS and writes skills to `.claude/skills/` and `.gemini/skills/` locally or globally. Skills are restructured with a slim core body plus `references/` files loaded on demand.

**Tech Stack:** Go 1.26 / Cobra / `embed.FS`, Makefile, Markdown SKILL.md files

**Spec:** `docs/superpowers/specs/2026-05-02-skill-optimization-design.md`

---

## File Map

### New Files
- `internal/skillsembed/embed.go` — `//go:embed skills` directive exposing `SkillsFS`
- `internal/skillsembed/skills/` — moved from `skills/` (all five skill directories)
- `cmd/armature/install_skills.go` — `arm install-skills` cobra command
- `cmd/armature/install_skills_test.go` — TDD tests for install-skills
- `internal/skillsembed/skills/armature-coordinator/references/parallel-dispatch.md`
- `internal/skillsembed/skills/armature-coordinator/references/commands.md`
- `internal/skillsembed/skills/armature-worker/references/dual-branch.md`
- `internal/skillsembed/skills/armature-worker/references/batch-strategy.md`
- `internal/skillsembed/skills/armature-planner/references/decompose-apply.md`
- `internal/skillsembed/skills/armature-planner/references/dependency-management.md`
- `internal/skillsembed/skills/armature-auditor/references/citation-errors.md`

### Modified Files
- `internal/skillsembed/skills/*/SKILL.md` — frontmatter added, descriptions cleaned, `<!-- CANONICAL SOURCE -->` removed
- `internal/skillsembed/skills/*/meta.yaml` → deleted (content merged into SKILL.md frontmatter)
- `cmd/armature/main.go` — wire `installSkillsCmd` into `newRootCmd()`
- `Makefile` — update `skill` target paths, add `deploy-skills`, update `dist-skills` help echo
- `AGENTS.md` — populate with setup instructions

---

## Chunk 1: Wave 1 Infrastructure

### Task 1A: Frontmatter Merge + Skills Directory Move + Makefile Update

**Files:**
- Move: `skills/` → `internal/skillsembed/skills/`
- Modify: `internal/skillsembed/skills/*/SKILL.md` (all 5) — add frontmatter, remove CANONICAL SOURCE comment
- Delete: `internal/skillsembed/skills/*/meta.yaml` (all 5)
- Modify: `Makefile`

---

- [ ] **Step 1: Create the embed package directory and move skills/**

```bash
mkdir -p internal/skillsembed
git mv skills internal/skillsembed/skills
```

Verify:
```bash
ls internal/skillsembed/skills/
# Expected: armature  armature-auditor  armature-coordinator  armature-planner  armature-worker
```

- [ ] **Step 2: Add frontmatter to armature/SKILL.md and delete its meta.yaml**

Open `internal/skillsembed/skills/armature/SKILL.md`. Replace the `<!-- CANONICAL SOURCE ... -->` comment at the top with the merged frontmatter. The description is trimmed (fix "a armature" → "an armature"; remove `(run make install)` from compatibility):

```markdown
---
name: armature
description: >
  Quick reference for arm command syntax and flags. Use when you know
  your role and need the right command — for role-specific workflows,
  use armature-planner, armature-coordinator, armature-worker, or
  armature-auditor instead.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---
```

Then delete the file:
```bash
git rm internal/skillsembed/skills/armature/meta.yaml
```

- [ ] **Step 3: Add frontmatter to armature-auditor/SKILL.md and delete its meta.yaml**

Remove the `<!-- CANONICAL SOURCE ... -->` line (line 1). Add frontmatter (description trimmed — remove command list "Runs validate, sources verify..."):

```markdown
---
name: armature-auditor
description: >
  Use when verifying completed work before story sign-off — checks citation
  coverage, source UUID integrity, outcome quality, and repo health.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---
```

```bash
git rm internal/skillsembed/skills/armature-auditor/meta.yaml
```

- [ ] **Step 4: Add frontmatter to armature-coordinator/SKILL.md and delete its meta.yaml**

Remove `<!-- CANONICAL SOURCE ... -->` line. Add frontmatter (description trimmed — remove "finds unblocked tasks..." summary; fix "a armature" → "an armature"; remove `(run make install)` is already absent):

```markdown
---
name: armature-coordinator
description: >
  Use when orchestrating work in an armature-managed repository.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---
```

```bash
git rm internal/skillsembed/skills/armature-coordinator/meta.yaml
```

- [ ] **Step 5: Add frontmatter to armature-planner/SKILL.md and delete its meta.yaml**

Remove `<!-- CANONICAL SOURCE ... -->` line. Add frontmatter (description trimmed — remove "covers decompose-apply..." detail; fix "a armature" if present; remove `(run make install)`):

```markdown
---
name: armature-planner
description: >
  Use when creating a new story or epic in an armature-managed repository.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---
```

```bash
git rm internal/skillsembed/skills/armature-planner/meta.yaml
```

- [ ] **Step 6: Add frontmatter to armature-worker/SKILL.md and delete its meta.yaml**

Remove `<!-- CANONICAL SOURCE ... -->` line. Add frontmatter (description trimmed — remove "picks up ready issues..." workflow; fix "a armature" → "an armature"; remove `(run make install)`):

```markdown
---
name: armature-worker
description: >
  Use when starting work in an armature-managed repository.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---
```

Also scan the body text for "make install" references and replace with "arm must be on your PATH" (per spec §6 — `make install` references are removed from skill bodies; setup lives in `AGENTS.md`). In `armature-worker/SKILL.md` lines 10-15 there is a `make install` code block — remove those lines entirely, replacing with:

```
If `arm` is not found, stop and resolve this before proceeding.
```

```bash
git rm internal/skillsembed/skills/armature-worker/meta.yaml
```

- [ ] **Step 7: Update the Makefile**

The `skill` target needs its loop path updated from `skills/*/` to `internal/skillsembed/skills/*/`. The assembly step (concatenating meta.yaml + banner + SKILL.md) becomes a straight copy of SKILL.md. Also add the new `deploy-skills` target and update the `dist-skills` help echo. Add `deploy-skills` to the `.PHONY` line.

Replace the `skill` target (lines 75–92) with:

```makefile
SKILLS_DIR := internal/skillsembed/skills

skill: build
	@for name in $(SKILLS_DIR)/*/; do \
		name=$$(basename "$$name"); \
		[ -f "$(SKILLS_DIR)/$$name/SKILL.md" ] || continue; \
		for harness in claude gemini; do \
			mkdir -p ".$$harness/skills/$$name"; \
			cp "$(SKILLS_DIR)/$$name/SKILL.md" ".$$harness/skills/$$name/SKILL.md"; \
			if [ -d "$(SKILLS_DIR)/$$name/scripts" ]; then \
				mkdir -p ".$$harness/skills/$$name/scripts"; \
				cp "$(SKILLS_DIR)/$$name/scripts/"* ".$$harness/skills/$$name/scripts/"; \
				chmod +x ".$$harness/skills/$$name/scripts/"*; \
			fi; \
			if [ -d "$(SKILLS_DIR)/$$name/references" ]; then \
				mkdir -p ".$$harness/skills/$$name/references"; \
				cp "$(SKILLS_DIR)/$$name/references/"* ".$$harness/skills/$$name/references/"; \
			fi; \
		done; \
	done
	@echo "Deployed skills to .claude/skills/ and .gemini/skills/"

deploy-skills:
	@for name in $(SKILLS_DIR)/*/; do \
		name=$$(basename "$$name"); \
		[ -f "$(SKILLS_DIR)/$$name/SKILL.md" ] || continue; \
		for harness in claude gemini; do \
			mkdir -p ".$$harness/skills/$$name"; \
			cp "$(SKILLS_DIR)/$$name/SKILL.md" ".$$harness/skills/$$name/SKILL.md"; \
			if [ -d "$(SKILLS_DIR)/$$name/scripts" ]; then \
				mkdir -p ".$$harness/skills/$$name/scripts"; \
				cp "$(SKILLS_DIR)/$$name/scripts/"* ".$$harness/skills/$$name/scripts/"; \
				chmod +x ".$$harness/skills/$$name/scripts/"*; \
			fi; \
			if [ -d "$(SKILLS_DIR)/$$name/references" ]; then \
				mkdir -p ".$$harness/skills/$$name/references"; \
				cp "$(SKILLS_DIR)/$$name/references/"* ".$$harness/skills/$$name/references/"; \
			fi; \
		done; \
	done
	@echo "Deployed skills to .claude/skills/ and .gemini/skills/"
```

Update the `dist-skills` help echo line (in the `help` target) from:
```
  make dist-skills - Package skills for distribution (no binaries) into dist/
```
to:
```
  make dist-skills - DEPRECATED: use arm install-skills instead
```

Read lines 94–109 of the Makefile to see exactly what path `dist-skills` iterates. If its loop walks `.$$harness/skills` (the deployed harness directory), no path change is needed. If it walks `skills/*/` directly (the source directory), update it to `$(SKILLS_DIR)/*/`. Make the change only if the source path requires it — do not guess.

Add `deploy-skills` to the `.PHONY` line at the top:
```makefile
.PHONY: test coverage coverage-check lint clean mutate check help skill deploy-skills dist-skills install
```

- [ ] **Step 8: Verify make skill runs cleanly**

```bash
make skill
```

Expected: skills deployed to `.claude/skills/` and `.gemini/skills/` from the new path. Inspect one:

```bash
head -10 .claude/skills/armature-worker/SKILL.md
```

Expected: starts with `---` frontmatter block, no `<!-- CANONICAL SOURCE -->` line.

- [ ] **Step 9: Run make check**

```bash
make check
```

Expected: all stages green (lint, test, coverage-check, mutate, skill). The `skill` stage now uses the new Makefile target.

- [ ] **Step 10: Commit**

```bash
git add internal/skillsembed/skills/ Makefile
git commit -m "feat: move skills to internal/skillsembed, merge meta.yaml frontmatter, update Makefile"
```

---

### Task 1B: `arm install-skills` Subcommand (TDD)

**Files:**
- Create: `internal/skillsembed/embed.go`
- Create: `cmd/armature/install_skills.go`
- Create: `cmd/armature/install_skills_test.go`
- Modify: `cmd/armature/main.go` (wire command)

> **Note on parallelism:** This task runs in parallel with Task 1A on a separate branch. Create a minimal placeholder for `internal/skillsembed/skills/test-skill/SKILL.md` in this branch so the embed compiles. The coordinator merges 1A (which has real skill content) on top. **After merging 1A, the coordinator must delete `internal/skillsembed/skills/test-skill/` before releasing to wave 2** — it must not ship in the binary.

---

- [ ] **Step 1: Create the embed package with a placeholder skills directory**

Create `internal/skillsembed/embed.go`:

```go
package skillsembed

import "embed"

//go:embed skills
var SkillsFS embed.FS
```

Create a minimal placeholder so `//go:embed skills` compiles:

```bash
mkdir -p internal/skillsembed/skills/test-skill
```

Create `internal/skillsembed/skills/test-skill/SKILL.md`:
```markdown
---
name: test-skill
description: Placeholder for embed compilation.
compatibility: n/a
---

# Test Skill

Placeholder content.
```

- [ ] **Step 2: Write failing tests**

Create `cmd/armature/install_skills_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/skillsembed"
)

func TestInstallSkillsDeploysSKILLMD(t *testing.T) {
	tmpDir := t.TempDir()

	err := deploySkills(skillsembed.SkillsFS, tmpDir)
	if err != nil {
		t.Fatalf("deploySkills returned error: %v", err)
	}

	// Verify at least one SKILL.md was deployed
	entries, err := os.ReadDir(filepath.Join(tmpDir, ".claude", "skills"))
	if err != nil {
		t.Fatalf("expected .claude/skills/ to exist: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one skill directory in .claude/skills/")
	}

	// Verify SKILL.md content matches embedded source for the test-skill
	deployedPath := filepath.Join(tmpDir, ".claude", "skills", "test-skill", "SKILL.md")
	deployedBytes, err := os.ReadFile(deployedPath)
	if err != nil {
		t.Fatalf("expected deployed SKILL.md at %s: %v", deployedPath, err)
	}

	embeddedBytes, err := skillsembed.SkillsFS.ReadFile("skills/test-skill/SKILL.md")
	if err != nil {
		t.Fatalf("failed to read embedded source: %v", err)
	}

	if string(deployedBytes) != string(embeddedBytes) {
		t.Errorf("deployed content does not match embedded source")
	}
}

func TestInstallSkillsDeploysReferences(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a skill with references/ in the embedded FS by using a sub-FS fixture
	// We'll create a temporary embedded FS via os.DirFS for this test
	testSkillsDir := t.TempDir()
	refDir := filepath.Join(testSkillsDir, "skills", "ref-skill", "references")
	if err := os.MkdirAll(refDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testSkillsDir, "skills", "ref-skill", "SKILL.md"), []byte("# Ref Skill\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refDir, "extra.md"), []byte("# Extra\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := deploySkills(os.DirFS(testSkillsDir), tmpDir)
	if err != nil {
		t.Fatalf("deploySkills returned error: %v", err)
	}

	deployedRef := filepath.Join(tmpDir, ".claude", "skills", "ref-skill", "references", "extra.md")
	if _, err := os.Stat(deployedRef); os.IsNotExist(err) {
		t.Errorf("expected references/extra.md to be deployed at %s", deployedRef)
	}
}

func TestInstallSkillsScriptsGetExecutablePermissions(t *testing.T) {
	testSkillsDir := t.TempDir()
	scriptDir := filepath.Join(testSkillsDir, "skills", "script-skill", "scripts")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testSkillsDir, "skills", "script-skill", "SKILL.md"), []byte("# Script Skill\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptDir, "run.sh"), []byte("#!/bin/sh\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	err := deploySkills(os.DirFS(testSkillsDir), tmpDir)
	if err != nil {
		t.Fatalf("deploySkills returned error: %v", err)
	}

	deployedScript := filepath.Join(tmpDir, ".claude", "skills", "script-skill", "scripts", "run.sh")
	info, err := os.Stat(deployedScript)
	if err != nil {
		t.Fatalf("expected deployed script at %s: %v", deployedScript, err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("expected script to have executable bits, got %o", info.Mode())
	}
}

func TestInstallSkillsGlobalFlag(t *testing.T) {
	// Tests the deploySkills helper with a pre-resolved home dir as baseDir.
	tmpHome := t.TempDir()

	testSkillsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(testSkillsDir, "skills", "g-skill"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testSkillsDir, "skills", "g-skill", "SKILL.md"), []byte("# G\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := deploySkills(os.DirFS(testSkillsDir), tmpHome)
	if err != nil {
		t.Fatalf("deploySkills returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpHome, ".claude", "skills", "g-skill", "SKILL.md")); os.IsNotExist(err) {
		t.Error("expected global deploy to ~/.claude/skills/")
	}
	if _, err := os.Stat(filepath.Join(tmpHome, ".gemini", "skills", "g-skill", "SKILL.md")); os.IsNotExist(err) {
		t.Error("expected global deploy to ~/.gemini/skills/")
	}
}

func TestInstallSkillsGlobalFlagUsesUserHomeDir(t *testing.T) {
	// Tests that the cobra command's --global flag resolves os.UserHomeDir() correctly.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cmd := newInstallSkillsCmd()
	cmd.SetArgs([]string{"--global"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install-skills --global returned error: %v", err)
	}

	// At least one skill should appear under the fake home
	entries, err := os.ReadDir(filepath.Join(tmpHome, ".claude", "skills"))
	if err != nil {
		t.Fatalf("expected ~/.claude/skills/ to exist: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one skill deployed to ~/.claude/skills/")
	}
}

func TestInstallSkillsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	err := deploySkills(skillsembed.SkillsFS, tmpDir)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}

	// Run again — must not error
	err = deploySkills(skillsembed.SkillsFS, tmpDir)
	if err != nil {
		t.Fatalf("second deploy (idempotent) failed: %v", err)
	}
}
```

- [ ] **Step 3: Run failing tests to verify they fail**

```bash
go test ./cmd/armature/... -run TestInstallSkills -v
```

Expected: compilation error — `deploySkills` not defined.

- [ ] **Step 4: Create install_skills.go with deploySkills implementation**

Create `cmd/armature/install_skills.go`:

```go
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/spf13/cobra"
)

func newInstallSkillsCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:   "install-skills",
		Short: "Deploy bundled agent skills to harness directories",
		Long: `Deploy armature's bundled skills to .claude/skills/ and .gemini/skills/.

By default deploys to the current working directory. Use --global to deploy
to ~/.claude/skills/ and ~/.gemini/skills/ instead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var baseDir string
			if global {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("resolving home directory: %w", err)
				}
				baseDir = home
			} else {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("resolving working directory: %w", err)
				}
				baseDir = wd
			}

			return deploySkills(skillsembed.SkillsFS, baseDir)
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "Deploy to ~/.claude/skills/ and ~/.gemini/skills/ instead of project-local")
	return cmd
}

func deploySkills(fsys fs.FS, baseDir string) error {
	harnesses := []string{"claude", "gemini"}

	skillsRoot := "skills"
	entries, err := fs.ReadDir(fsys, skillsRoot)
	if err != nil {
		return fmt.Errorf("reading embedded skills: %w", err)
	}

	counts := map[string]int{}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		skillPath := skillsRoot + "/" + skillName

		for _, harness := range harnesses {
			destBase := filepath.Join(baseDir, "."+harness, "skills", skillName)
			if err := os.MkdirAll(destBase, 0755); err != nil {
				return err
			}

			// Walk all files under this skill
			err := fs.WalkDir(fsys, skillPath, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}

				// Relative path from the skill root
				rel, err := filepath.Rel(skillPath, path)
				if err != nil {
					return err
				}

				destPath := filepath.Join(destBase, rel)
				if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
					return err
				}

				data, err := fs.ReadFile(fsys, path)
				if err != nil {
					return err
				}

				perm := fs.FileMode(0644)
				// files under scripts/ get executable bits
				if strings.HasPrefix(rel, "scripts/") {
					perm = 0755
				}

				return os.WriteFile(destPath, data, perm)
			})
			if err != nil {
				return err
			}
			counts[harness]++
		}
	}

	for _, harness := range harnesses {
		dest := filepath.Join(baseDir, "."+harness, "skills")
		fmt.Printf("Deployed %d skills to %s\n", counts[harness], dest)
	}

	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./cmd/armature/... -run TestInstallSkills -v
```

Expected: all 5 `TestInstallSkills*` tests PASS.

- [ ] **Step 6: Wire the command into main.go**

In `cmd/armature/main.go`, add after the `contextHistoryCmd` block (before `return root`):

```go
	installSkillsCmd := newInstallSkillsCmd()
	installSkillsCmd.GroupID = "admin"
	root.AddCommand(installSkillsCmd)
```

- [ ] **Step 7: Run make check**

```bash
make check
```

Expected: all stages green.

- [ ] **Step 8: Commit**

```bash
git add internal/skillsembed/ cmd/armature/install_skills.go cmd/armature/install_skills_test.go cmd/armature/main.go
git commit -m "feat: add arm install-skills subcommand with embedded skills FS"
```

---

## Chunk 2: Wave 2 Content Cleanup

> Wave 2 tasks are parallelizable. Run 2A, 2B, and 2C on separate branches simultaneously. Each starts from the merged wave-1 branch.

### Task 2A: armature Quick Reference Card + Description Cleanup (All 5 Skills)

**Files:**
- Modify: `internal/skillsembed/skills/armature/SKILL.md` — trim to ~50-line command reference
- Modify: descriptions already updated in Task 1A; this task verifies all five are correct

**Note:** The descriptions were updated during Task 1A (steps 2–6). This task focuses on trimming the `armature` skill body to ~50 lines and verifying all five description fields are correct.

---

- [ ] **Step 1: Read the current armature/SKILL.md body**

```bash
cat internal/skillsembed/skills/armature/SKILL.md
```

Identify all workflow prose, prerequisites, rate limit tables, and extended explanations. The target is grouped command blocks only — one line per command with key flags, no paragraphs.

- [ ] **Step 2: Trim armature/SKILL.md body to grouped command blocks only**

Working from the file you read in Step 1, delete all workflow prose, prerequisites, rate limit tables, and extended explanations. Do **not** use a pre-written template — derive the output from the actual source so no existing commands are accidentally dropped.

Rules for what stays vs. goes:
- **Keep:** every `arm <subcommand>` line and its key flags, grouped under section headings
- **Remove:** all paragraph text, callout boxes, "when to use" guidance, prerequisite blocks, rate limit tables, and any multi-sentence explanation

Target: ~50 lines in the body (after frontmatter). One line per command with a brief inline comment if the command's purpose is not obvious from its name.

- [ ] **Step 3: Verify all five skill descriptions match spec §6**

Check each skill's frontmatter:

| Skill | Expected description start |
|-------|---------------------------|
| `armature` | "Quick reference for arm command syntax and flags." |
| `armature-auditor` | "Use when verifying completed work before story sign-off — checks citation coverage, source UUID integrity, outcome quality, and repo health." (no command list) |
| `armature-coordinator` | "Use when orchestrating work in an armature-managed repository." |
| `armature-planner` | "Use when creating a new story or epic in an armature-managed repository." |
| `armature-worker` | "Use when starting work in an armature-managed repository." |

All compatibility fields: `Designed for Claude Code and Gemini CLI. Requires arm on PATH.` (no `(run make install)`).

Grep to confirm no "a armature" typos remain:
```bash
grep -r "a armature" internal/skillsembed/skills/
```
Expected: no output.

- [ ] **Step 4: Run make deploy-skills**

```bash
make deploy-skills
```

Expected: "Deployed skills to .claude/skills/ and .gemini/skills/"

Verify `.claude/skills/armature/SKILL.md` is under 60 lines:
```bash
wc -l .claude/skills/armature/SKILL.md
```

- [ ] **Step 5: Run make check**

```bash
make check
```

Expected: all stages green.

- [ ] **Step 6: Commit**

```bash
git add internal/skillsembed/skills/armature/SKILL.md
git commit -m "feat: trim armature skill to command reference card (~50 lines)"
```

---

### Task 2B: AGENTS.md Setup Content

**Files:**
- Modify: `AGENTS.md` (currently empty)

---

- [ ] **Step 1: Write AGENTS.md content per spec**

```markdown
## Setup

1. Install arm (see releases)
2. Run `arm install-skills` once to deploy agent skills
3. Run `arm worker-init` before claiming your first task
```

- [ ] **Step 2: Verify AGENTS.md is non-empty**

```bash
cat AGENTS.md
```

- [ ] **Step 3: Run make check**

```bash
make check
```

Expected: all stages green.

- [ ] **Step 4: Commit**

```bash
git add AGENTS.md
git commit -m "docs: populate AGENTS.md with arm install-skills setup instructions"
```

---

### Task 2C: armature-auditor Progressive Disclosure References

**Files:**
- Modify: `internal/skillsembed/skills/armature-auditor/SKILL.md` — remove remediation prose, add trigger pointer
- Create: `internal/skillsembed/skills/armature-auditor/references/citation-errors.md`

---

- [ ] **Step 1: Read the current armature-auditor/SKILL.md**

```bash
cat internal/skillsembed/skills/armature-auditor/SKILL.md
```

Identify the "Citation Integrity (E7 + E8)" full remediation guide and extended "Source Freshness" workflow sections.

- [ ] **Step 2: Create references/citation-errors.md**

```bash
mkdir -p internal/skillsembed/skills/armature-auditor/references
```

Extract the "Citation Integrity (E7 + E8)" remediation guide + extended "Source Freshness" workflow into `internal/skillsembed/skills/armature-auditor/references/citation-errors.md`. Keep all content verbatim — this is a move, not a rewrite.

The file should open with a heading:
```markdown
# Citation Error Remediation (E7 + E8)
```

followed by the extracted sections.

- [ ] **Step 3: Trim armature-auditor/SKILL.md core body**

Remove the extracted sections from the core body. In their place, add the trigger pointer (per spec §7):

```
If `arm validate` reports ERROR lines,
read `references/citation-errors.md`.
```

Core retains: when to run, all five checklist steps (commands + expected output, no remediation prose), D6/E8 caveat note, pre-merge gate table, common failure modes.

Verify the resulting core is under 500 lines:
```bash
wc -l internal/skillsembed/skills/armature-auditor/SKILL.md
```

Expected: well under 500 (spec target ~150 lines).

- [ ] **Step 4: Run make deploy-skills**

```bash
make deploy-skills
```

Verify the references file is deployed:
```bash
ls .claude/skills/armature-auditor/references/
# Expected: citation-errors.md
```

- [ ] **Step 5: Run make check**

```bash
make check
```

Expected: all stages green.

- [ ] **Step 6: Commit**

```bash
git add internal/skillsembed/skills/armature-auditor/
git commit -m "feat: extract armature-auditor citation-errors to references/ (progressive disclosure)"
```

---

## Chunk 3: Wave 3 — armature-worker References

> Wave 3 is a single task (no parallelism). Must land before wave 4 so wave 4 workers exercise the improved skill.

### Task 3: armature-worker Progressive Disclosure References

**Files:**
- Modify: `internal/skillsembed/skills/armature-worker/SKILL.md` — remove dual-branch callouts, batch strategy section, add trigger pointers
- Create: `internal/skillsembed/skills/armature-worker/references/dual-branch.md`
- Create: `internal/skillsembed/skills/armature-worker/references/batch-strategy.md`

---

- [ ] **Step 1: Read the current armature-worker/SKILL.md**

```bash
cat internal/skillsembed/skills/armature-worker/SKILL.md
```

Identify:
- All dual-branch mode callouts scattered through the body (look for `dual-branch` mentions, conditional blocks for `armature.mode = dual-branch`)
- The "Batch Strategy (Advanced)" section

- [ ] **Step 2: Create references/dual-branch.md**

```bash
mkdir -p internal/skillsembed/skills/armature-worker/references
```

Consolidate all dual-branch mode callouts from the skill body into `internal/skillsembed/skills/armature-worker/references/dual-branch.md`. Open with:

```markdown
# Dual-Branch Mode

> Load this file when `git config --local armature.mode` returns `dual-branch`,
> before any `git add` or `git commit`.
```

Append all extracted dual-branch content.

- [ ] **Step 3: Create references/batch-strategy.md**

Move the "Batch Strategy (Advanced)" section verbatim to `internal/skillsembed/skills/armature-worker/references/batch-strategy.md`. Open with:

```markdown
# Batch Strategy (Advanced)

> Load this file when your task involves 10 or more files.
```

- [ ] **Step 4: Trim armature-worker/SKILL.md core body**

Remove the extracted dual-branch callout blocks and the batch-strategy section. Insert trigger pointers **inline at each removal site** — not appended at the end. Replace dual-branch callout rows in the common mistakes table with a single pointer row. The two trigger pointers from spec §7 are:

```
If `git config --local armature.mode` returns `dual-branch`,
read `references/dual-branch.md` before any git add or commit.

If your task involves 10 or more files,
read `references/batch-strategy.md`.
```

Core retains: flow flowchart, prerequisites (without `make install` block), full step-by-step with trigger pointers replacing dual-branch callouts, log slot section, valid transition targets, common mistakes table.

Verify:
```bash
wc -l internal/skillsembed/skills/armature-worker/SKILL.md
```
Expected: ~140 lines (under 500 spec limit).

- [ ] **Step 5: Run make deploy-skills**

```bash
make deploy-skills
```

Verify:
```bash
ls .claude/skills/armature-worker/references/
# Expected: batch-strategy.md  dual-branch.md
```

- [ ] **Step 6: Run make check**

```bash
make check
```

Expected: all stages green.

- [ ] **Step 7: Commit**

```bash
git add internal/skillsembed/skills/armature-worker/
git commit -m "feat: extract armature-worker dual-branch and batch-strategy to references/ (progressive disclosure)"
```

---

## Chunk 4: Wave 4 — Planner + Coordinator References

> Wave 4 tasks are parallelizable. Run 4A and 4B on separate branches simultaneously. Both start from the merged wave-3 branch.

### Task 4A: armature-planner Progressive Disclosure References

**Files:**
- Modify: `internal/skillsembed/skills/armature-planner/SKILL.md` — remove decompose-apply walkthrough, dependency management deep-dive, add trigger pointers
- Create: `internal/skillsembed/skills/armature-planner/references/decompose-apply.md`
- Create: `internal/skillsembed/skills/armature-planner/references/dependency-management.md`

---

- [ ] **Step 1: Read the current armature-planner/SKILL.md**

```bash
cat internal/skillsembed/skills/armature-planner/SKILL.md
```

Identify:
- The full "Decompose-Apply Workflow" walkthrough (schema inspection, writing plan.json, dry-run, apply, promote)
- The "Dependency Management" deep-dive (`arm link`, checking/resolving scope overlaps)

- [ ] **Step 2: Create references/decompose-apply.md**

```bash
mkdir -p internal/skillsembed/skills/armature-planner/references
```

Move the full "Decompose-Apply Workflow" section to `internal/skillsembed/skills/armature-planner/references/decompose-apply.md`. Open with:

```markdown
# Decompose-Apply Workflow

> Load this file for multi-task stories before running arm decompose.
```

- [ ] **Step 3: Create references/dependency-management.md**

Move the "Dependency Management" deep-dive to `internal/skillsembed/skills/armature-planner/references/dependency-management.md`. Open with:

```markdown
# Dependency Management

> Load this file if `arm validate` reports scope overlap WARNINGs.
```

- [ ] **Step 4: Trim armature-planner/SKILL.md core body**

Remove the extracted sections. Insert trigger pointers **inline at each removal site** — not appended at the end. The two trigger pointers from spec §7 are:

```
For multi-task stories, read `references/decompose-apply.md`
for the full decompose-apply workflow.

If `arm validate` reports scope overlap WARNINGs,
read `references/dependency-management.md`.
```

Core retains: planner loop flowchart, prerequisites, step-by-step summary, "Writing Good Plan JSON" section, source registration paths A and B, release checklist, common failure modes.

Verify:
```bash
wc -l internal/skillsembed/skills/armature-planner/SKILL.md
```
Expected: ~265 lines (under 500).

- [ ] **Step 5: Run make deploy-skills and make check**

```bash
make deploy-skills && make check
```

Expected: references deployed, all check stages green.

- [ ] **Step 6: Commit**

```bash
git add internal/skillsembed/skills/armature-planner/
git commit -m "feat: extract armature-planner decompose-apply and dependency-management to references/"
```

---

### Task 4B: armature-coordinator Progressive Disclosure References

**Files:**
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md` — remove parallel dispatch section, log slots for parallel dispatch, command reference, querying JSON sections, add trigger pointers
- Create: `internal/skillsembed/skills/armature-coordinator/references/parallel-dispatch.md`
- Create: `internal/skillsembed/skills/armature-coordinator/references/commands.md`

---

- [ ] **Step 1: Read the current armature-coordinator/SKILL.md**

```bash
cat internal/skillsembed/skills/armature-coordinator/SKILL.md
```

Identify:
- "Parallel Dispatch" section
- "Log Slots for Parallel Dispatch" section
- "Querying JSON Output" section
- "Command Reference" section

- [ ] **Step 2: Create references/parallel-dispatch.md**

```bash
mkdir -p internal/skillsembed/skills/armature-coordinator/references
```

Move "Parallel Dispatch" + "Log Slots for Parallel Dispatch" sections to `internal/skillsembed/skills/armature-coordinator/references/parallel-dispatch.md`. Open with:

```markdown
# Parallel Dispatch

> Load this file when the story has tasks with no blocking dependencies between them.
```

- [ ] **Step 3: Create references/commands.md**

Move "Querying JSON Output" + "Command Reference" sections to `internal/skillsembed/skills/armature-coordinator/references/commands.md`. Open with:

```markdown
# Command Reference

> Full command reference for coordinator operations.
```

- [ ] **Step 4: Trim armature-coordinator/SKILL.md core body**

Remove the extracted sections. Insert trigger pointers **inline at each removal site** — not appended at the end. The two trigger pointers from spec §7 are:

```
If the story has tasks with no blocking dependencies between them,
read `references/parallel-dispatch.md` before dispatching.

For a full command reference, see `references/commands.md`.
```

Core retains: loop flowchart, survey + branch creation, find ready work, sequential dispatch, after workers return, story completion, common failure modes.

Verify:
```bash
wc -l internal/skillsembed/skills/armature-coordinator/SKILL.md
```
Expected: ~255 lines (under 500).

- [ ] **Step 5: Run make deploy-skills and make check**

```bash
make deploy-skills && make check
```

Expected: references deployed, all check stages green.

- [ ] **Step 6: Commit**

```bash
git add internal/skillsembed/skills/armature-coordinator/
git commit -m "feat: extract armature-coordinator parallel-dispatch and commands to references/"
```

---

## Completion Checklist

After all waves are merged by the coordinator:

- [ ] `make check` green on the integrated branch
- [ ] No `meta.yaml` files remain: `find internal/skillsembed/skills -name meta.yaml` → no output
- [ ] No `<!-- CANONICAL SOURCE -->` lines remain: `grep -r "CANONICAL SOURCE" internal/skillsembed/skills/` → no output
- [ ] No `make install` references in skill bodies: `grep -r "make install" internal/skillsembed/skills/` → no output
- [ ] No `a armature` typos: `grep -r "a armature" internal/skillsembed/skills/` → no output
- [ ] `arm install-skills` deploys all 5 skills (post wave-1 merge): `arm install-skills 2>&1 | grep "Deployed 5 skills"`
- [ ] Each core skill body under 500 lines: `wc -l internal/skillsembed/skills/*/SKILL.md`
- [ ] `AGENTS.md` non-empty
- [ ] `make deploy-skills` runs without `build` dependency
- [ ] All 7 reference files exist: `find internal/skillsembed/skills/*/references -name "*.md" | sort` → 7 lines (auditor: 1, coordinator: 2, planner: 2, worker: 2)
