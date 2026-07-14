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
		cmd := exec.CommandContext(ctx, "python3", scriptPath, tmpDir)
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
		cmd := exec.CommandContext(ctx, "python3", scriptPath, tmpDir)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		// Should fail
		require.Error(t, err, "skill-lint should fail when mandatory --worktree flag is missing from claim command")
		t.Logf("Expected failure output: %s", errOutput.String())
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
		cmd := exec.CommandContext(ctx, "python3", scriptPath, tmpDir)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		// Should fail
		require.Error(t, err, "skill-lint should fail for invalid subcommands")
		t.Logf("Expected failure output: %s", errOutput.String())
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
		cmd := exec.CommandContext(ctx, "python3", scriptPath, tmpDir)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		// Should fail
		require.Error(t, err, "skill-lint should fail for invalid flags")
		t.Logf("Expected failure output: %s", errOutput.String())
	})
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
