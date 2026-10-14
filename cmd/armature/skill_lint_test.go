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

func TestSkillLint_REQ_TOPTIER_S1_T1(t *testing.T) {
	projectRoot, err := os.Getwd()
	require.NoError(t, err)
	for !fileExists(filepath.Join(projectRoot, "scripts", "skill_lint.py")) {
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Skip("skill_lint.py not found in project")
		}
		projectRoot = parent
	}
	scriptPath := filepath.Join(projectRoot, "scripts", "skill_lint.py")

	armBin := os.Getenv("ARM_BIN")
	if armBin == "" {
		armBin = filepath.Join(projectRoot, "bin", "arm")
	}
	require.FileExists(t, armBin, "expected arm binary to be built at %s (run `make build` first)", armBin)

	pythonBin := os.Getenv("PYTHON")
	if pythonBin == "" {
		pythonBin = "python3"
	}

	t.Run("ArmatureQuickReferenceUsesCopyableCommands", func(t *testing.T) {
		skillPath := filepath.Join(projectRoot, "internal", "skillsembed", "skills", "armature", "SKILL.md")
		content, err := os.ReadFile(skillPath)
		require.NoError(t, err)
		require.NotContains(t, string(content), "arm claim ID --worktree /path/to/wt [--ttl 60]")
		require.NotContains(t, string(content), "arm render-context ID [--budget 4000]")
	})

	t.Run("ValidCommandsPasses", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

		skillMD := `---
name: test-skill
description: Test skill
---

# Test Skill

## Usage

Run the following commands:

` + "```bash" + `
arm worker-init --check || arm worker-init
arm claim TASK-01 --worktree
arm note TASK-01 --msg "Testing"
arm transition TASK-01 --to done --outcome "Complete"
arm doctor
arm validate
` + "```" + `

More info.
`
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		ctx := t.Context()
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

	t.Run("MissingMandatoryFlagFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

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

		ctx := t.Context()
		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		require.Error(t, err, "skill-lint should fail when mandatory --worktree flag is missing from claim command")
		require.Contains(t, errOutput.String(), "missing mandatory flags: --worktree",
			"failure should be attributed to the missing --worktree flag")
	})

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

	t.Run("CanonicalDocumentationRejectsObsoleteCommandButArchiveIsExcluded", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "docs", "archive"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("```bash\narm removed-command\n```\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "docs", "archive", "old-workflow.md"), []byte("```bash\narm historic-command\n```\n"), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()

		require.Error(t, err)
		require.Contains(t, output.String(), "README.md")
		require.Contains(t, output.String(), "removed-command")
		require.Contains(t, output.String(), "Unknown subcommand 'removed-command'")
		require.NotContains(t, output.String(), "historic-command")
	})

	t.Run("ValueTakingWorktreePasses", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("```bash\narm claim TASK-01 --worktree /tmp/wt\n```\n"), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		require.NoError(t, cmd.Run(), output.String())
	})

	t.Run("BracketedOptionalFlagSyntaxFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "```bash\narm claim --issue TASK-01 --worktree [--ttl 120]\n```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.Error(t, err, "bracketed optional flags are not valid shell syntax")
		require.Contains(t, output.String(), "bracketed synopsis syntax")
	})

	t.Run("ContinuedCommandSuppliesMandatoryFlagOnContinuationLine", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("```bash\narm claim TASK-01 \\\n  --worktree\n```\n"), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.NoError(t, err, "stderr: %s", output.String())
	})

	t.Run("ContinuedCommandValidatesFlagsOnContinuationLine", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "```bash\narm claim TASK-01 --worktree \\\n  --not-a-real-flag\n```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.Error(t, err)
		require.Contains(t, output.String(), "invalid flags")
		require.Contains(t, output.String(), "--not-a-real-flag")
	})

	t.Run("CommandSubstitutionArmPrefixIsNotFlagged", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("```bash\nSTATUS=$(armature-cli status)\n```\n"), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.NoError(t, err, "stderr: %s", output.String())
	})

	t.Run("SemicolonSeparatedAndRedirectedCommandsAreLinted", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("```bash\narm doctor; arm validate\narm ready > out.json\n```\n"), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.NoError(t, err, "stderr: %s", output.String())
	})

	t.Run("SingleQuotedArgumentIsNotMistakenForFlags", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "```bash\narm note TASK-01 --msg 'contains --not-a-real-flag inside a string'\n```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.NoError(t, err, "stderr: %s", output.String())
	})

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

	t.Run("InvalidSubcommandFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

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

		ctx := t.Context()
		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		require.Error(t, err, "skill-lint should fail for invalid subcommands")
		require.Contains(t, errOutput.String(), "Unknown subcommand 'invalid-subcommand'",
			"failure should be attributed to the unknown subcommand")
	})

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

		ctx := t.Context()
		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		require.Error(t, err, "the bash block after a non-bash fenced block must still be extracted and linted")
		require.Contains(t, errOutput.String(), "Unknown subcommand 'invalid-subcommand-after-json'",
			"the invalid command inside the bash block after the json block should have been found")
	})

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
		ctx := t.Context()
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

	t.Run("SourcesAddWithPlaceholderArgMissingMandatoryFlagsFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "```bash\narm sources add SOME-PLACEHOLDER-URL-OR-PATH\n```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.Error(t, err, "sources add missing --url/--type must be flagged even with a placeholder positional arg")
		require.Contains(t, output.String(), "missing mandatory flags")
		require.Contains(t, output.String(), "--url")
		require.Contains(t, output.String(), "--type")
	})

	t.Run("AngleBracketPlaceholderSyntaxFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "```bash\narm scope-rename <old-path> <new-path>\n```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.Error(t, err, "angle-bracket synopsis placeholders are not valid shell syntax")
		require.Contains(t, output.String(), "angle-bracket synopsis syntax")
	})

	t.Run("FusedBracketedFlagSyntaxFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "```bash\narm claim TASK-01 --worktree --ttl[=60]\n```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.Error(t, err, "a fused flag+bracket token is not valid shell syntax")
		require.Contains(t, output.String(), "bracketed synopsis syntax")
	})

	t.Run("IndentedFencedBlockUnderListItemIsLinted", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "1. Claim the task:\n   ```bash\n   arm claim TASK-01 --worktree\n   ```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.NoError(t, err, "a valid arm command inside an indented fenced block should pass lint; stderr: %s", output.String())
	})

	t.Run("IndentedFencedBlockUnderListItemMissingMandatoryFlagFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "1. Claim the task:\n   ```bash\n   arm claim TASK-01\n   ```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		cmd.Stderr = output
		err := cmd.Run()
		require.Error(t, err, "the indented block's content must still be validated, not just its presence")
		require.Contains(t, output.String(), "missing mandatory flags: --worktree")
	})

	t.Run("InvalidFlagFails", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))

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

		ctx := t.Context()
		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled, not attacker input
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		output := new(bytes.Buffer)
		errOutput := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = errOutput
		err := cmd.Run()

		require.Error(t, err, "skill-lint should fail for invalid flags")
		require.Contains(t, errOutput.String(), "invalid flags: --invalid-flag", "failure should be attributed to the invalid flag, not some other cause")
	})

	t.Run("ShowCycleRecoveryRejectsTopLevelNotesParser_REQ_AOC_S2_T3", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "```bash\nCYCLE=$(arm show TASK-01 --format json | jq -r '[.notes // [] | .[] | strings]')\n```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		errOutput := new(bytes.Buffer)
		cmd.Stderr = errOutput
		err := cmd.Run()
		require.Error(t, err, "a top-level .notes parser must fail skill-lint")
		require.Contains(t, errOutput.String(), ".issues[0].notes")
	})

	t.Run("ShowCycleRecoveryAcceptsEnvelopeNotesParser_REQ_AOC_S2_T3", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "internal", "skillsembed", "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		skillMD := "```bash\nCYCLE=$(arm show TASK-01 --format json | jq -r '[.issues[0].notes // [] | .[] | strings]')\n```\n"
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644))

		cmd := exec.CommandContext(context.Background(), pythonBin, scriptPath, tmpDir) //nolint:gosec // pythonBin: test-controlled
		cmd.Env = append(os.Environ(), "ARM_BIN="+armBin)
		errOutput := new(bytes.Buffer)
		cmd.Stderr = errOutput
		err := cmd.Run()
		require.NoError(t, err, "envelope .issues[0].notes parser must pass; stderr: %s", errOutput.String())
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
