package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/trellis/internal/config"
	"github.com/scullxbones/trellis/internal/git"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/worker"
	"github.com/spf13/cobra"
)

const issuesGitignore = `# Materialized state — derived from ops logs, regenerated locally by each worker.
# Never commit. See architecture.md §2 (Directory Structure).
state/
`

const postMergeHookTemplate = `#!/bin/sh
# Trellis post-merge hook: auto-detect merged branches and transition done issues to merged.
# Branch-aware: skips on _trellis since ops are committed directly there.
# To activate: cp this file to .git/hooks/post-merge && chmod +x .git/hooks/post-merge

# Skip on _trellis branch where ops logs are committed directly
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_trellis" ]; then
  exit 0
fi

trls sync
`

const postCommitHookTemplate = `#!/bin/sh
# Trellis post-commit hook: emit heartbeat and push ops in dual-branch mode.
# Branch-aware: skips on _trellis since ops are committed directly there.
# To activate: cp this file to .git/hooks/post-commit && chmod +x .git/hooks/post-commit

# Skip on _trellis branch where ops logs are committed directly
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_trellis" ]; then
  exit 0
fi

# Send heartbeat for active claim (if any)
trls heartbeat 2>/dev/null

# In dual-branch mode, push ops logs after each commit
if grep -q '"mode".*"dual-branch"' .issues/config.json 2>/dev/null; then
  trls push-ops 2>/dev/null
fi
`

const prepareCommitMsgHookTemplate = `#!/bin/sh
# Trellis prepare-commit-msg hook: prepend active claim ID to commit message.
# Branch-aware: skips on _trellis since ops logs use automated messages.
# To activate: cp this file to .git/hooks/prepare-commit-msg && chmod +x .git/hooks/prepare-commit-msg

# Skip on _trellis branch where ops logs use automated messages
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_trellis" ]; then
  exit 0
fi

# Get the active claim ID
claim_id=$(trls show active-claim --field id 2>/dev/null)

# If there's an active claim, prepend it to the commit message
if [ -n "$claim_id" ]; then
  commit_msg_file=$1
  original_msg=$(cat "$commit_msg_file")
  echo "$claim_id: $original_msg" > "$commit_msg_file"
fi
`

const preCommitHookTemplate = `#!/bin/sh
# Trellis pre-commit hook: block ops log commits on code branches in dual-branch mode.
# In dual-branch mode, ops live on _trellis — never on a code branch.
# To activate: cp this file to .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit
#
# This is defense-in-depth; .issues/.gitignore also blocks ops/ from being staged.

# Allow commits on _trellis — that's exactly where ops belong.
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_trellis" ]; then
  exit 0
fi

# Only block in dual-branch mode; check if config says dual-branch
if ! grep -q '"mode".*"dual-branch"' .issues/config.json 2>/dev/null; then
  # Single-branch mode allows ops/ commits
  exit 0
fi

# Only block additions/modifications — deletions are allowed (cleanup commits).
if git diff --cached --name-only --diff-filter=AM | grep -q '\.issues/ops/'; then
  echo "ERROR: Refusing to commit .issues/ops/ changes on a code branch."
  echo "In dual-branch mode, ops are written directly to the _trellis branch."
  echo "If you are migrating to dual-branch mode, run: trls init --dual-branch"
  exit 1
fi
`

func newInitCmd() *cobra.Command {
	var repoPath string
	var dualBranch bool

	cmd := &cobra.Command{
		Use:               "init",
		Short:             "Initialize Trellis in the current repository",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			return runInit(cmd, repoPath, dualBranch)
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")
	cmd.Flags().BoolVar(&dualBranch, "dual-branch", false, "initialize in dual-branch mode (issues stored on separate _trellis branch)")
	return cmd
}

// installHooks copies hook templates from .issues/hooks/ to .git/hooks/ and makes them executable.
// In dual-branch mode, the templates are in the worktree's .issues/hooks/.
func installHooks(repoPath string, issuesDir string) error {
	hooksDir := filepath.Join(issuesDir, "hooks")
	gitHooksDir := filepath.Join(repoPath, ".git", "hooks")

	// Create .git/hooks directory if it doesn't exist
	if err := os.MkdirAll(gitHooksDir, 0755); err != nil {
		return fmt.Errorf("create .git/hooks directory: %w", err)
	}

	// List of hooks to install
	hooks := []string{"pre-commit", "post-commit", "post-merge", "prepare-commit-msg"}

	for _, hook := range hooks {
		templatePath := filepath.Join(hooksDir, hook+".sh.template")
		hookPath := filepath.Join(gitHooksDir, hook)

		// Read template content
		content, err := os.ReadFile(templatePath)
		if err != nil {
			// If template doesn't exist, skip (it might not be needed for this mode)
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read hook template %s: %w", hook, err)
		}

		// Write hook to .git/hooks/ with executable permissions
		if err := os.WriteFile(hookPath, content, 0755); err != nil {
			return fmt.Errorf("install hook %s: %w", hook, err)
		}
	}

	return nil
}

func runInit(cmd *cobra.Command, repoPath string, dualBranch bool) error {
	// Resolve repoPath to an absolute path so stored paths are never relative.
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo path: %w", err)
	}
	repoPath = absRepoPath

	gitClient := git.New(repoPath)

	var issuesDir string
	if dualBranch {
		// Create orphan branch _trellis (idempotent)
		if err := gitClient.CreateOrphanBranch("_trellis"); err != nil {
			return fmt.Errorf("create _trellis branch: %w", err)
		}

		// Create .trellis/ worktree (idempotent)
		worktreePath := filepath.Join(repoPath, ".trellis")
		if err := gitClient.AddWorktree("_trellis", worktreePath); err != nil {
			return fmt.Errorf("add .trellis worktree: %w", err)
		}

		// Set git config keys
		if err := gitClient.SetGitConfig("trellis.mode", "dual-branch"); err != nil {
			return fmt.Errorf("set trellis.mode: %w", err)
		}
		if err := gitClient.SetGitConfig("trellis.ops-worktree-path", worktreePath); err != nil {
			return fmt.Errorf("set trellis.ops-worktree-path: %w", err)
		}

		issuesDir = filepath.Join(worktreePath, ".issues")
	} else {
		issuesDir = filepath.Join(repoPath, ".issues")
	}

	// Create directory structure
	dirs := []string{
		filepath.Join(issuesDir, "ops"),
		filepath.Join(issuesDir, "state"),
		filepath.Join(issuesDir, "state", "issues"),
		filepath.Join(issuesDir, "templates"),
		filepath.Join(issuesDir, "hooks"),
		filepath.Join(issuesDir, "review"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Write .gitignore to prevent state/ from being committed
	gitignorePath := filepath.Join(issuesDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(issuesGitignore), 0644); err != nil {
		return fmt.Errorf("write .issues/.gitignore: %w", err)
	}

	// Write SCHEMA file
	schemaPath := filepath.Join(issuesDir, "ops", "SCHEMA")
	if err := os.WriteFile(schemaPath, []byte(ops.GenerateSchema()), 0644); err != nil {
		return fmt.Errorf("write SCHEMA: %w", err)
	}

	// Write hook templates to .issues/hooks/
	hookTemplates := map[string]string{
		"post-merge.sh.template":         postMergeHookTemplate,
		"post-commit.sh.template":        postCommitHookTemplate,
		"prepare-commit-msg.sh.template": prepareCommitMsgHookTemplate,
		"pre-commit.sh.template":         preCommitHookTemplate,
	}

	for hookName, hookContent := range hookTemplates {
		hookTemplatePath := filepath.Join(issuesDir, "hooks", hookName)
		if err := os.WriteFile(hookTemplatePath, []byte(hookContent), 0644); err != nil {
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

	// Detect whether this was a fresh init or an idempotent re-run by checking
	// if the ops directory already existed before we ran MkdirAll.
	opsDir := filepath.Join(issuesDir, "ops")
	if entries, err := os.ReadDir(opsDir); err == nil && len(entries) > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Trellis already initialized in %s mode at %s\n", mode, issuesDir)
	} else {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized Trellis in %s mode at %s\n", mode, issuesDir)
	}
	return nil
}
