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
# To activate: cp this file to .git/hooks/post-merge && chmod +x .git/hooks/post-merge
trls sync
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

	// Write post-merge hook template
	hookTemplatePath := filepath.Join(issuesDir, "hooks", "post-merge.sh.template")
	if err := os.WriteFile(hookTemplatePath, []byte(postMergeHookTemplate), 0644); err != nil {
		return fmt.Errorf("write post-merge hook template: %w", err)
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
