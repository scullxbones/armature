package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSkillLint_REQ_TOPTIER-S1-T1 verifies that skill-lint validates arm commands in fenced code blocks.
func TestSkillLint_REQ_TOPTIER_S1_T1(t *testing.T) {
	// Get the project root by looking for the scripts directory
	projectRoot, err := os.Getwd()
	require.NoError(t, err)
	// If we're in cmd/armature, go up two levels to get to the project root
	for !fileExists(filepath.Join(projectRoot, "scripts", "skill_lint.py")) {
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Skip("skill_lint.py not found in project")
		}
		projectRoot = parent
	}
	scriptPath := filepath.Join(projectRoot, "scripts", "skill_lint.py")

	// Point skill_lint.py at a real, freshly-built arm binary instead of
	// relying on PATH: CI doesn't build/install arm before running go test,
	// so `arm` may not exist on PATH there even though it does on a dev
	// machine with `make install` run previously.
	armBin := os.Getenv("ARM_BIN")
	if armBin == "" {
		armBin = filepath.Join(projectRoot, "bin", "arm")
	}
	require.FileExists(t, armBin, "expected arm binary to be built at %s (run `make build` first)", armBin)

	pythonBin := os.Getenv("PYTHON")
	if pythonBin == "" {
		pythonBin = "python3"
	}

	// Test 1: Verify that valid arm commands pass
	t.Run("ValidCommandsPasses", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

		// Write a valid skill file with correct arm commands
		skillMD := `---
name: test-skill
description: Test skill
---

# Test Skill

## Usage

Run the following commands:

` + "```bash" + `
arm worker-init --check || arm worker-init
arm claim TASK-01 --worktree /tmp/wt
arm note TASK-01 --msg "Testing"
arm transition TASK-01 --to done --outcome "Complete"
arm doctor
arm validate
` + "```" + `

More info.
`
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		// Run skill-lint
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		if err != nil {
			t.Logf("skill-lint failed with: %v\nStdout: %s\nStderr: %s", err, output.String(), errOutput.String())
		}
		require.NoError(t, err, "skill-lint should pass for valid commands")
	})

	// Test 2: Verify that missing mandatory flags fail
	t.Run("MissingMandatoryFlagFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

		// Write a skill file with invalid arm commands (missing --worktree on claim)
		skillMD := `---
name: test-skill
description: Test skill
---

# Test Skill

This command is missing the mandatory --worktree flag:

` + "```bash" + `
arm claim TASK-01
` + "```" + `

More info.
`
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		// Run skill-lint
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		// Should fail, and fail for the right reason
		require.Error(t, err, "skill-lint should fail when mandatory --worktree flag is missing from claim command")
		require.Contains(t, errOutput.String(), "missing mandatory flags: --worktree",
			"failure should be attributed to the missing --worktree flag")
	})

	// References are shipped with their parent skill and must be linted too.
	t.Run("ReferenceMarkdownIsLinted", func(t *testing.T) {
		tmpDir := t.TempDir()
		referenceDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill", "references")
		require.NoError(t, os.MkdirAll(referenceDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(referenceDir, "commands.md"), []byte("```bash\narm invalid-reference-command\n```\n"), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.Error(t, err)
		require.Contains(t, output.String(), "invalid-reference-command")
	})

	// A continued command is one shell command, so a mandatory flag supplied
	// only on the continuation line must satisfy the mandatory-flag check.
	// This discriminates joined-vs-unjoined continuations: `arm claim
	// TASK-01` alone would already be missing --worktree, so a test that
	// only checks for that failure can't prove the join happened. Here the
	// continuation line supplies the mandatory flag, so the command must
	// PASS lint -- which only happens if the physical lines were actually
	// joined before validation.
	t.Run("ContinuedCommandSuppliesMandatoryFlagOnContinuationLine", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("```bash\narm claim TASK-01 \\\n  --worktree /tmp/wt\n```\n"), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.NoError(t, err, "stderr: %s", output.String())
	})

	// The continuation line's flags must actually be parsed and validated,
	// not just the first physical line -- an invalid flag on the
	// continuation line must be caught.
	t.Run("ContinuedCommandValidatesFlagsOnContinuationLine", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("```bash\narm claim TASK-01 --worktree /tmp/wt \\\n  --not-a-real-flag\n```\n"), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.Error(t, err)
		require.Contains(t, output.String(), "invalid flags")
		require.Contains(t, output.String(), "--not-a-real-flag")
	})

	// `arm` commands in command substitutions are real invocations, not prose.
	t.Run("CommandSubstitutionIsLinted", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("```bash\nFILES=$(arm ready --not-a-real-flag | jq -r '.')\n```\n"), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.Error(t, err)
		require.Contains(t, output.String(), "--not-a-real-flag")
	})

	// Test 3: Verify that invalid subcommands fail
	t.Run("InvalidSubcommandFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

		// Write a skill file with invalid subcommand
		skillMD := `---
name: test-skill
description: Test skill
---

# Test Skill

This is an invalid subcommand:

` + "```bash" + `
arm invalid-subcommand --some-flag value
` + "```" + `

More info.
`
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		// Run skill-lint
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		// Should fail, and fail for the right reason
		require.Error(t, err, "skill-lint should fail for invalid subcommands")
		require.Contains(t, errOutput.String(), "Unknown subcommand 'invalid-subcommand'",
			"failure should be attributed to the unknown subcommand")
	})

	// Test for the extract_code_blocks fence-pairing bug: a non-bash fenced
	// block (e.g. ```json) appearing before a ```bash block must not cause
	// the bash block's commands to be silently skipped.
	t.Run("BashBlockAfterNonBashBlockIsExtracted", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

		skillMD := `---
name: test-skill
description: Test skill
---

# Test Skill

Here's some JSON:

` + "```json" + `
{"key": "value"}
` + "```" + `

Now an arm command with an invalid subcommand, so it must be caught:

` + "```bash" + `
arm invalid-subcommand-after-json --some-flag value
` + "```" + `
`
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		// If the bash block after the json block were silently skipped (the
		// bug), skill-lint would report no errors here. It must be caught.
		require.Error(t, err, "the bash block after a non-bash fenced block must still be extracted and linted")
		require.Contains(t, errOutput.String(), "Unknown subcommand 'invalid-subcommand-after-json'",
			"the invalid command inside the bash block after the json block should have been found")
	})

	// Regression check for the concrete case the reviewer flagged: before the
	// fence-parser fix, armature-reviewer/SKILL.md's `arm review ...`
	// commands (which follow ```json blocks in the same file) were silently
	// never extracted, so skill-lint validated nothing in that file's bash
	// blocks.
	t.Run("ArmatureReviewerSkillCommandsAreExtracted", func(t *testing.T) {
		skillPath := filepath.Join(projectRoot, "internal", "skillsembed", "skills", "armature-reviewer", "SKILL.md")
		require.FileExists(t, skillPath)

		script := `
import sys
sys.path.insert(0, "scripts")
import skill_lint

with open("` + skillPath + `", encoding="utf-8") as f:
    content = f.read()

blocks = skill_lint.extract_code_blocks(content)
commands = [c for b in blocks for c in skill_lint.extract_arm_commands(b)]
assert any("arm review commits" in c for c in commands), commands
`
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmd := exec.CommandContext(ctx, pythonBin, "-c", script) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Dir = projectRoot
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()
		if err != nil {
			t.Logf("stdout: %s\nstderr: %s", output.String(), errOutput.String())
		}
		require.NoError(t, err, "arm review show/record commands should be extracted from armature-reviewer/SKILL.md")
	})

	// Test 4: Verify that invalid flags fail
	t.Run("InvalidFlagFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

		// Write a skill file with invalid flag
		skillMD := `---
name: test-skill
description: Test skill
---

# Test Skill

This command has an invalid flag:

` + "```bash" + `
arm doctor --invalid-flag
` + "```" + `

More info.
`
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		// Run skill-lint
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		// Should fail, and fail for the right reason
		require.Error(t, err, "skill-lint should fail for invalid flags")
		require.Contains(t, errOutput.String(), "invalid flags: --invalid-flag", "failure should be attributed to the invalid flag, not some other cause")
	})
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
