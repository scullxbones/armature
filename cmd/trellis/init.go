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

	// Write SCHEMA file
	schemaPath := filepath.Join(issuesDir, "ops", "SCHEMA")
	if err := os.WriteFile(schemaPath, []byte(ops.GenerateSchema()), 0644); err != nil {
		return fmt.Errorf("write SCHEMA: %w", err)
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
	fmt.Fprintf(cmd.OutOrStdout(), "Initialized Trellis in %s mode at %s\n", mode, issuesDir)
	return nil
}
