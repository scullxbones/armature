package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/trellis/internal/config"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/worker"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var repoPath string

	cmd := &cobra.Command{
		Use:               "init",
		Short:             "Initialize Trellis in the current repository",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			return runInit(cmd, repoPath)
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")
	return cmd
}

func runInit(cmd *cobra.Command, repoPath string) error {
	issuesDir := filepath.Join(repoPath, ".issues")

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

	fmt.Fprintf(cmd.OutOrStdout(), "Initialized Trellis in single-branch mode at %s\n", issuesDir)
	return nil
}
