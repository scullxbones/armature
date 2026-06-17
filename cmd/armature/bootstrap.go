package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/spf13/cobra"
)

func newBootstrapCmd() *cobra.Command {
	var global bool
	var withHooks bool
	var platforms []string
	var repoPath string

	cmd := &cobra.Command{
		Use:   "bootstrap [--global] [--with-hooks] [--platform <name>]",
		Short: "Bootstrap Armature: initialize repo and deploy harness artifacts",
		Long: `Initialize a repository for Armature coordination and optionally deploy harness artifacts
(skills, plugin metadata, harness hook configs).

By default, artifacts deploy to .claude/ (local). Use --global to deploy to ~/.claude/ instead.
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

			// Step 1: Run init for repo setup
			if err := runInit(cmd, repoPath, false); err != nil {
				return fmt.Errorf("repo setup failed: %w", err)
			}

			// Step 2: Build the plan
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

			// Step 3: Execute the plan
			if err := executeHarnessSetup(cmd, plan, repoPath, global, withHooks); err != nil {
				return fmt.Errorf("harness setup failed: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Bootstrap complete.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")
	cmd.Flags().BoolVar(&global, "global", false, "deploy to ~/.claude/ instead of .claude/")
	cmd.Flags().BoolVar(&withHooks, "with-hooks", false, "also write harness hook configuration")
	cmd.Flags().StringSliceVar(&platforms, "platform", nil, "restrict to specific platform(s) (can be repeated)")
	return cmd
}

// executeHarnessSetup executes the harness setup plan: deploys skills, plugin metadata, and hook configs.
func executeHarnessSetup(cmd *cobra.Command, plan bootstrap.Plan, repoPath string, global bool, withHooks bool) error {
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
		if row.Skills == bootstrap.ActionInstall {
			skillsDest := filepath.Join(destBase, ".claude", "skills")
			if err := deploySkills(skillsembed.SkillsFS, skillsDest); err != nil {
				return fmt.Errorf("deploy skills for %s: %w", platformName, err)
			}

			// Deploy flat skill files
			if err := deployFlatSkills(skillsembed.SkillsFS, skillsDest); err != nil {
				return fmt.Errorf("deploy flat skills for %s: %w", platformName, err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deployed skills to %s for %s\n", skillsDest, platformName)
		}

		// Deploy plugin metadata if requested
		if row.PluginMetadata == bootstrap.ActionInstall {
			pluginsDest := filepath.Join(destBase, ".claude", "plugins", platformName)
			if err := deployPlugin(skillsembed.SkillsFS, pluginsDest); err != nil {
				return fmt.Errorf("deploy plugin metadata for %s: %w", platformName, err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deployed plugin configuration to %s for %s\n", pluginsDest, platformName)
		}

		// Deploy harness hook config if requested
		if row.HarnessHookConfig == bootstrap.ActionInstall && withHooks {
			adapter, err := harnesshook.NewAdapterForPlatform(platformName)
			if err != nil {
				return fmt.Errorf("create adapter for %s: %w", platformName, err)
			}

			if err := adapter.WriteConfig(destBase); err != nil {
				return fmt.Errorf("write harness hook config for %s: %w", platformName, err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deployed harness hook config for %s\n", platformName)
		}
	}

	return nil
}
