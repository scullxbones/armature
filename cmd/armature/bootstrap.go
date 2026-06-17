package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

func newBootstrapCmd() *cobra.Command {
	var global bool
	var withHooks bool
	var dualBranch bool
	var platforms []string
	var repoPath string

	cmd := &cobra.Command{
		Use:   "bootstrap [--global] [--with-hooks] [--dual-branch] [--platform <name>]",
		Short: "Bootstrap Armature: initialize repo and deploy harness artifacts",
		Long: `Initialize a repository for Armature coordination and optionally deploy harness artifacts
(skills, plugin metadata, harness hook configs).

By default, artifacts deploy to .claude/ (local). Use --global to deploy to ~/.claude/ instead.
Use --dual-branch to initialize in dual-branch mode (issues stored on separate _armature branch).
Use --with-hooks to also write harness hook configuration (both require --platform support).
Use --platform to restrict bootstrap to specific platforms (can be repeated); default is all verified platforms.

The command is idempotent: running it multiple times has the same effect as running it once.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse repo path from flag or default to current directory
			if repoPath == "" {
				repoPath = "."
			}

			// Resolve to absolute path
			absRepoPath, err := filepath.Abs(repoPath)
			if err != nil {
				return fmt.Errorf("resolve repo path: %w", err)
			}
			repoPath = absRepoPath

			if err := runRepoSetup(cmd, repoPath, dualBranch); err != nil {
				return fmt.Errorf("repo setup failed: %w", err)
			}

			platformList := bootstrap.DefaultPlatforms()
			if len(platforms) > 0 {
				platformList = nil
				for _, p := range platforms {
					platformList = append(platformList, bootstrap.Platform(p))
				}
			}

			target := "local"
			if global {
				target = "global"
			}

			req := bootstrap.PlanRequest{
				Platforms: platformList,
				Target:    target,
				WithHooks: withHooks,
			}

			plan, err := bootstrap.BuildPlan(req)
			if err != nil {
				return fmt.Errorf("build harness setup plan: %w", err)
			}

			if err := executeHarnessSetup(cmd, plan, repoPath, global); err != nil {
				return fmt.Errorf("harness setup failed: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Bootstrap complete.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")
	cmd.Flags().BoolVar(&global, "global", false, "deploy to ~/.claude/ instead of .claude/")
	cmd.Flags().BoolVar(&dualBranch, "dual-branch", false, "initialize in dual-branch mode (issues stored on separate _armature branch)")
	cmd.Flags().BoolVar(&withHooks, "with-hooks", false, "also write harness hook configuration")
	cmd.Flags().StringSliceVar(&platforms, "platform", nil, "restrict to specific platform(s) (can be repeated)")
	return cmd
}

// executeHarnessSetup executes the harness setup plan: deploys skills, plugin metadata, and hook configs.
func executeHarnessSetup(cmd *cobra.Command, plan bootstrap.Plan, repoPath string, global bool) error {
	// Determine deployment target base
	var destBase string
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		destBase = home
	} else {
		destBase = repoPath
	}

	// Process each platform row in the plan
	for _, row := range plan.Rows {
		platformName := string(row.Platform)

		// Deploy skills if requested
		switch row.Skills {
		case bootstrap.ActionInstall:
			skillsDest := filepath.Join(destBase, ".claude", "skills")
			if err := deploySkills(skillsembed.SkillsFS, skillsDest); err != nil {
				return fmt.Errorf("deploy skills for %s: %w", platformName, err)
			}

			// Deploy flat skill files
			if err := deployFlatSkills(skillsembed.SkillsFS, skillsDest); err != nil {
				return fmt.Errorf("deploy flat skills for %s: %w", platformName, err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deployed skills to %s for %s\n", skillsDest, platformName)
		case bootstrap.ActionUnsupported:
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: skills not supported for %s, skipping\n", platformName)
		}

		// Deploy plugin metadata if requested
		switch row.PluginMetadata {
		case bootstrap.ActionInstall:
			pluginName, err := getPluginNameFromFS(skillsembed.SkillsFS)
			if err != nil {
				return fmt.Errorf("extract plugin name: %w", err)
			}

			pluginsDest := filepath.Join(destBase, ".claude", "plugins", pluginName)
			if err := deployPlugin(skillsembed.SkillsFS, pluginsDest); err != nil {
				return fmt.Errorf("deploy plugin metadata for %s: %w", platformName, err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deployed plugin configuration to %s for %s\n", pluginsDest, platformName)
		case bootstrap.ActionUnsupported:
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: plugin metadata not supported for %s, skipping\n", platformName)
		}

		// Deploy harness hook config if requested
		switch row.HarnessHookConfig {
		case bootstrap.ActionInstall:
			adapter, err := harnesshook.NewAdapterForPlatform(platformName)
			if err != nil {
				return fmt.Errorf("create adapter for %s: %w", platformName, err)
			}

			if err := adapter.WriteConfig(destBase); err != nil {
				return fmt.Errorf("write harness hook config for %s: %w", platformName, err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deployed harness hook config for %s\n", platformName)
		case bootstrap.ActionUnsupported:
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: harness hook config not supported for %s, skipping\n", platformName)
		}
	}

	return nil
}

const issuesGitignore = `# Materialized state — derived from ops logs, regenerated locally by each worker.
# Never commit. See architecture.md §2 (Directory Structure).
state/
`

const postMergeHookTemplate = `#!/bin/sh
# Armature post-merge hook: auto-detect merged branches and transition done issues to merged.
# Branch-aware: skips on _armature since ops are committed directly there.
# To activate: cp this file to .git/hooks/post-merge && chmod +x .git/hooks/post-merge

# Skip on _armature branch where ops logs are committed directly
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

arm sync
`

const postCommitHookTemplate = `#!/bin/sh
# Armature post-commit hook: emit heartbeat and push ops in dual-branch mode.
# Branch-aware: skips on _armature since ops are committed directly there.
# To activate: cp this file to .git/hooks/post-commit && chmod +x .git/hooks/post-commit

# Skip on _armature branch where ops logs are committed directly
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

# Send heartbeat for active claim (if any)
arm heartbeat 2>/dev/null

# In dual-branch mode, push ops logs after each commit
if grep -q '"mode".*"dual-branch"' .armature/config.json 2>/dev/null; then
  arm push-ops 2>/dev/null
fi
`

const prepareCommitMsgHookTemplate = `#!/bin/sh
# Armature prepare-commit-msg hook: prepend active claim ID to commit message.
# Branch-aware: skips on _armature since ops logs use automated messages.
# To activate: cp this file to .git/hooks/prepare-commit-msg && chmod +x .git/hooks/prepare-commit-msg

# Skip on _armature branch where ops logs use automated messages
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

# Get the active claim ID
claim_id=$(arm show active-claim --field id 2>/dev/null)

# If there's an active claim, prepend it to the commit message
if [ -n "$claim_id" ]; then
  commit_msg_file=$1
  original_msg=$(cat "$commit_msg_file")
  echo "$claim_id: $original_msg" > "$commit_msg_file"
fi
`

const preCommitHookTemplate = `#!/bin/sh
# Armature pre-commit hook: block ops log commits on code branches in dual-branch mode.
# In dual-branch mode, ops live on _armature — never on a code branch.
# To activate: cp this file to .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit
#
# This is defense-in-depth; .armature/.gitignore also blocks ops/ from being staged.

# Allow commits on _armature — that's exactly where ops belong.
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

# Only block in dual-branch mode; check if config says dual-branch
if ! grep -q '"mode".*"dual-branch"' .armature/config.json 2>/dev/null; then
  # Single-branch mode allows ops/ commits
  exit 0
fi

# Only block additions/modifications — deletions are allowed (cleanup commits).
if git diff --cached --name-only --diff-filter=AM | grep -q '\.armature/ops/'; then
  echo "ERROR: Refusing to commit .armature/ops/ changes on a code branch."
  echo "In dual-branch mode, ops are written directly to the _armature branch."
  echo "If you are migrating to dual-branch mode, run: arm bootstrap --dual-branch"
  exit 1
fi
`

// installHooks copies hook templates from .armature/hooks/ to .git/hooks/ and makes them executable.
// In dual-branch mode, the templates are in the worktree's .armature/hooks/.
func installHooks(repoPath string, issuesDir string) error {
	hooksDir := filepath.Join(issuesDir, "hooks")
	gitHooksDir := filepath.Join(repoPath, ".git", "hooks")

	// Create .git/hooks directory if it doesn't exist
	if err := os.MkdirAll(gitHooksDir, 0o750); err != nil {
		return fmt.Errorf("create .git/hooks directory: %w", err)
	}

	// List of hooks to install
	hooks := []string{"pre-commit", "post-commit", "post-merge", "prepare-commit-msg"}

	for _, hook := range hooks {
		templatePath := filepath.Join(hooksDir, hook+".sh.template")
		hookPath := filepath.Join(gitHooksDir, hook)

		// Read template content
		content, err := os.ReadFile(templatePath) //nolint:gosec // G304: path constructed from internal hooks dir
		if err != nil {
			// If template doesn't exist, skip (it might not be needed for this mode)
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read hook template %s: %w", hook, err)
		}

		// Write hook to .git/hooks/ with executable permissions
		if err := os.WriteFile(hookPath, content, 0o755); err != nil { //nolint:gosec // git hooks require executable bit
			return fmt.Errorf("install hook %s: %w", hook, err)
		}
	}

	return nil
}

// runRepoSetup initializes the repository structure for Armature.
func runRepoSetup(cmd *cobra.Command, repoPath string, dualBranch bool) error {
	// Resolve repoPath to an absolute path so stored paths are never relative.
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo path: %w", err)
	}
	repoPath = absRepoPath

	gitClient := adapters.New(repoPath)

	var issuesDir string
	if dualBranch {
		// Create orphan branch _armature (idempotent)
		if err := gitClient.CreateOrphanBranch("_armature"); err != nil {
			return fmt.Errorf("create _armature branch: %w", err)
		}

		// Create .arm/ worktree (idempotent)
		worktreePath := filepath.Join(repoPath, ".arm")
		if err := gitClient.AddWorktree("_armature", worktreePath); err != nil {
			return fmt.Errorf("add .arm worktree: %w", err)
		}

		// Set git config keys
		if err := gitClient.SetGitConfig("armature.mode", "dual-branch"); err != nil {
			return fmt.Errorf("set armature.mode: %w", err)
		}
		if err := gitClient.SetGitConfig("armature.ops-worktree-path", worktreePath); err != nil {
			return fmt.Errorf("set armature.ops-worktree-path: %w", err)
		}

		issuesDir = filepath.Join(worktreePath, ".armature")
	} else {
		issuesDir = filepath.Join(repoPath, ".armature")
	}

	// Detect whether this is a fresh init or an idempotent re-run before writing anything.
	opsDir := filepath.Join(issuesDir, "ops")
	freshInit := true
	if entries, err := os.ReadDir(opsDir); err == nil && len(entries) > 0 {
		freshInit = false
	}

	// Create directory structure
	dirs := []string{
		opsDir,
		filepath.Join(issuesDir, "state"),
		filepath.Join(issuesDir, "state", "issues"),
		filepath.Join(issuesDir, "templates"),
		filepath.Join(issuesDir, "hooks"),
		filepath.Join(issuesDir, "review"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Write .gitignore to prevent state/ from being committed
	gitignorePath := filepath.Join(issuesDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(issuesGitignore), 0o600); err != nil {
		return fmt.Errorf("write .armature/.gitignore: %w", err)
	}

	// Write SCHEMA file
	schemaPath := filepath.Join(issuesDir, "ops", "SCHEMA")
	if err := os.WriteFile(schemaPath, []byte(ops.GenerateSchema()), 0o600); err != nil {
		return fmt.Errorf("write SCHEMA: %w", err)
	}

	// Write hook templates to .armature/hooks/
	hookTemplates := map[string]string{
		"post-merge.sh.template":         postMergeHookTemplate,
		"post-commit.sh.template":        postCommitHookTemplate,
		"prepare-commit-msg.sh.template": prepareCommitMsgHookTemplate,
		"pre-commit.sh.template":         preCommitHookTemplate,
	}

	for hookName, hookContent := range hookTemplates {
		hookTemplatePath := filepath.Join(issuesDir, "hooks", hookName)
		if err := os.WriteFile(hookTemplatePath, []byte(hookContent), 0o600); err != nil {
			return fmt.Errorf("write hook template %s: %w", hookName, err)
		}
	}

	// Install hooks from templates to .git/hooks/
	if err := installHooks(repoPath, issuesDir); err != nil {
		return fmt.Errorf("install hooks: %w", err)
	}

	// Detect project type and write config
	configPath := filepath.Join(issuesDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		projectType := config.DetectProjectType(repoPath)
		cfg := config.DefaultConfig(projectType)
		if dualBranch {
			cfg.Mode = "dual-branch"
		}
		if err := config.WriteConfig(configPath, cfg); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	// Init worker if not already configured
	if ok, _ := worker.CheckWorkerID(repoPath); !ok {
		if _, err := worker.InitWorker(repoPath); err != nil {
			return fmt.Errorf("init worker: %w", err)
		}
	}

	mode := "single-branch"
	if dualBranch {
		mode = "dual-branch"
	}

	if freshInit {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized Armature in %s mode at %s\n", mode, issuesDir)
	} else {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Armature already initialized in %s mode at %s\n", mode, issuesDir)
	}
	return nil
}
