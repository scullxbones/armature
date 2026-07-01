package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/bootstrap"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/skillsembed"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

// RepoSetupResult captures the outcome of repository initialization.
type RepoSetupResult struct {
	Status       string   `json:"status"`                  // "initialized", "already_initialized", "error"
	SkippedHooks []string `json:"skipped_hooks,omitempty"` // hook names skipped (unmanaged)
	Error        string   `json:"error,omitempty"`
}

// BootstrapResult is the complete output of a bootstrap operation.
type BootstrapResult struct {
	RepoSetup    RepoSetupResult                   `json:"repo_setup"`
	HarnessSetup []bootstrap.HarnessArtifactResult `json:"harness_setup"`
}

// runRepoSetupWithFormat calls runRepoSetup, silencing human output when format is "json".
func runRepoSetupWithFormat(cmd *cobra.Command, repoPath string, format string) (RepoSetupResult, error) {
	if format == "json" || format == "agent" {
		silentCmd := &cobra.Command{}
		silentCmd.SetOut(io.Discard)
		return runRepoSetup(silentCmd, repoPath)
	}
	return runRepoSetup(cmd, repoPath)
}

// executeHarnessSetupWithFormat calls executeHarnessSetup, silencing human output when format is "json" or "agent".
func executeHarnessSetupWithFormat(
	cmd *cobra.Command, plan bootstrap.Plan, repoPath string, global bool, format string,
) ([]bootstrap.HarnessArtifactResult, error) {
	if format == "json" || format == "agent" {
		silentCmd := &cobra.Command{}
		silentCmd.SetOut(io.Discard)
		return executeHarnessSetup(silentCmd, plan, repoPath, global)
	}
	return executeHarnessSetup(cmd, plan, repoPath, global)
}

func newBootstrapCmd() *cobra.Command {
	var global bool
	var withHooks bool
	var platforms []string

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
			// Read repo path from the root persistent flag
			repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}

			// Resolve to absolute path
			absRepoPath, err := filepath.Abs(repoPath)
			if err != nil {
				return fmt.Errorf("resolve repo path: %w", err)
			}
			repoPath = absRepoPath

			// Read format from the root persistent flag
			format, _ := cmd.Root().PersistentFlags().GetString("format")

			// Detect non-TTY and auto-set format to JSON when --format is not explicitly set
			if !cmd.Root().PersistentFlags().Changed("format") && format == "human" && !tui.IsTerminal() {
				format = "json"
				_ = cmd.Root().PersistentFlags().Set("format", "json")
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

			// If a platform was explicitly requested, verify it has at least one actionable artifact.
			// Do not error for auto-detected platforms (when user didn't specify --platform).
			if len(platforms) > 0 {
				for _, row := range plan.Rows {
					allUnsupported := row.Skills == bootstrap.ActionUnsupported &&
						row.PluginMetadata == bootstrap.ActionUnsupported &&
						(!withHooks || row.HarnessHookConfig == bootstrap.ActionUnsupported)
					if allUnsupported {
						return fmt.Errorf("platform %s has no supported requested artifacts", string(row.Platform))
					}
				}
			}

			// Run repo setup and collect results (pass format flag for silent mode in JSON)
			repoSetupResult, err := runRepoSetupWithFormat(cmd, repoPath, format)
			if err != nil {
				// Emit error in JSON format before returning (for json/agent format)
				if format == "json" || format == "agent" {
					repoSetupResult.Status = "error"
					repoSetupResult.Error = err.Error()
					result := BootstrapResult{
						RepoSetup:    repoSetupResult,
						HarnessSetup: []bootstrap.HarnessArtifactResult{},
					}
					if data, merr := json.MarshalIndent(result, "", "  "); merr == nil {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
					}
				}
				return fmt.Errorf("repo setup failed: %w", err)
			}

			// Execute harness setup and collect results (pass format flag for silent mode in JSON)
			harnessResults, err := executeHarnessSetupWithFormat(cmd, plan, repoPath, global, format)
			if err != nil {
				// Emit partial JSON results before returning error (for json/agent format)
				if (format == "json" || format == "agent") && len(harnessResults) > 0 {
					result := BootstrapResult{
						RepoSetup:    repoSetupResult,
						HarnessSetup: harnessResults,
					}
					if data, merr := json.MarshalIndent(result, "", "  "); merr == nil {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
					}
				}
				return fmt.Errorf("harness setup failed: %w", err)
			}

			// Determine output format
			if format == "json" || format == "agent" {
				result := BootstrapResult{
					RepoSetup:    repoSetupResult,
					HarnessSetup: harnessResults,
				}
				data, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal JSON: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				// Text output - report skipped/unsupported artifacts
				for _, r := range harnessResults {
					if r.Status == "unsupported" || r.Status == "skipped" {
						msg := r.Status
						if r.Note != "" {
							msg = r.Note
						}
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s (%s): %s\n", r.Artifact, r.Platform, msg)
					}
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Bootstrap complete.\n")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "deploy to ~/.claude/ instead of .claude/")
	cmd.Flags().BoolVar(&withHooks, "with-hooks", false, "also write harness hook configuration")
	cmd.Flags().StringSliceVar(&platforms, "platform", nil, "restrict to specific platform(s) (can be repeated)")
	return cmd
}

// recordArtifactAction records an artifact deployment action to the results slice based on action type.
func recordArtifactAction(results *[]bootstrap.HarnessArtifactResult, platformName, artifactName string, action bootstrap.ActionKind) {
	switch action {
	case bootstrap.ActionInstall:
		// ActionInstall results are recorded after successful deployment
		return
	case bootstrap.ActionUnsupported:
		*results = append(*results, bootstrap.HarnessArtifactResult{
			Platform: platformName,
			Artifact: artifactName,
			Status:   "unsupported",
			Action:   string(action),
		})
	case bootstrap.ActionSkip:
		*results = append(*results, bootstrap.HarnessArtifactResult{
			Platform: platformName,
			Artifact: artifactName,
			Status:   "skipped",
			Action:   string(action),
		})
	}
}

// executeHarnessSetup executes the harness setup plan: deploys skills, plugin metadata, and hook configs.
// Returns a slice of HarnessArtifactResult for each artifact processed.
// On error, returns the results collected so far along with the error, allowing structured output
// to report partial results.
func executeHarnessSetup(cmd *cobra.Command, plan bootstrap.Plan, repoPath string, global bool) ([]bootstrap.HarnessArtifactResult, error) {
	var results []bootstrap.HarnessArtifactResult

	// Determine deployment target base
	var destBase string
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return results, fmt.Errorf("resolve home directory: %w", err)
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
				results = append(results, bootstrap.HarnessArtifactResult{
					Platform: platformName,
					Artifact: "skills",
					Status:   "error",
					Action:   string(bootstrap.ActionInstall),
					Error:    err.Error(),
				})
				return results, fmt.Errorf("deploy skills for %s: %w", platformName, err)
			}

			// Deploy flat skill files
			if err := deployFlatSkills(skillsembed.SkillsFS, skillsDest); err != nil {
				results = append(results, bootstrap.HarnessArtifactResult{
					Platform: platformName,
					Artifact: "skills",
					Status:   "error",
					Action:   string(bootstrap.ActionInstall),
					Error:    err.Error(),
				})
				return results, fmt.Errorf("deploy flat skills for %s: %w", platformName, err)
			}

			results = append(results, bootstrap.HarnessArtifactResult{
				Platform: platformName,
				Artifact: "skills",
				Status:   "ok",
				Action:   string(bootstrap.ActionInstall),
				Note:     fmt.Sprintf("Deployed to %s", skillsDest),
			})
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deployed skills to %s for %s\n", skillsDest, platformName)
		} else {
			recordArtifactAction(&results, platformName, "skills", row.Skills)
		}

		// Deploy plugin metadata if requested
		if row.PluginMetadata == bootstrap.ActionInstall {
			pluginName, err := getPluginNameFromFS(skillsembed.SkillsFS)
			if err != nil {
				return results, fmt.Errorf("extract plugin name: %w", err)
			}

			pluginsDest := filepath.Join(destBase, ".claude", "plugins", pluginName)
			if err := deployPlugin(skillsembed.SkillsFS, pluginsDest); err != nil {
				results = append(results, bootstrap.HarnessArtifactResult{
					Platform: platformName,
					Artifact: "plugin_metadata",
					Status:   "error",
					Action:   string(bootstrap.ActionInstall),
					Error:    err.Error(),
				})
				return results, fmt.Errorf("deploy plugin metadata for %s: %w", platformName, err)
			}

			results = append(results, bootstrap.HarnessArtifactResult{
				Platform: platformName,
				Artifact: "plugin_metadata",
				Status:   "ok",
				Action:   string(bootstrap.ActionInstall),
				Note:     fmt.Sprintf("Deployed to %s", pluginsDest),
			})
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deployed plugin configuration to %s for %s\n", pluginsDest, platformName)
		} else {
			recordArtifactAction(&results, platformName, "plugin_metadata", row.PluginMetadata)
		}

		// Deploy harness hook config if requested
		if row.HarnessHookConfig == bootstrap.ActionInstall {
			adapter, err := harnesshook.NewAdapterForPlatform(platformName)
			if err != nil {
				results = append(results, bootstrap.HarnessArtifactResult{
					Platform: platformName,
					Artifact: "harness_hook_config",
					Status:   "error",
					Action:   string(bootstrap.ActionInstall),
					Error:    err.Error(),
				})
				return results, fmt.Errorf("create adapter for %s: %w", platformName, err)
			}

			owned, err := adapter.OwnsConfig(destBase)
			if err != nil {
				return results, fmt.Errorf("check config ownership for %s: %w", platformName, err)
			}
			if !owned {
				results = append(results, bootstrap.HarnessArtifactResult{
					Platform: platformName,
					Artifact: "harness_hook_config",
					Status:   "skipped",
					Action:   string(bootstrap.ActionInstall),
					Note:     "existing config not managed by Armature",
				})
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skipped harness hook config for %s (not managed by Armature)\n", platformName)
			} else {
				if err := adapter.WriteConfig(destBase); err != nil {
					results = append(results, bootstrap.HarnessArtifactResult{
						Platform: platformName,
						Artifact: "harness_hook_config",
						Status:   "error",
						Action:   string(bootstrap.ActionInstall),
						Error:    err.Error(),
					})
					return results, fmt.Errorf("write harness hook config for %s: %w", platformName, err)
				}

				results = append(results, bootstrap.HarnessArtifactResult{
					Platform: platformName,
					Artifact: "harness_hook_config",
					Status:   "ok",
					Action:   string(bootstrap.ActionInstall),
				})
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deployed harness hook config for %s\n", platformName)
			}
		} else {
			recordArtifactAction(&results, platformName, "harness_hook_config", row.HarnessHookConfig)
		}
	}

	return results, nil
}

const issuesGitignore = `# Materialized state — derived from ops logs, regenerated locally by each worker.
# Never commit. See architecture.md §2 (Directory Structure).
state/
`

const postMergeHookTemplate = `#!/bin/sh
# armature:managed
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
# armature:managed
# Armature post-commit hook: emit heartbeat and push ops.
# Branch-aware: skips on _armature since ops are committed directly there.
# To activate: cp this file to .git/hooks/post-commit && chmod +x .git/hooks/post-commit

# Skip on _armature branch where ops logs are committed directly
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

# Send heartbeat for active claim (if any)
arm heartbeat 2>/dev/null

# Push ops logs after each commit
arm push-ops 2>/dev/null
`

const prepareCommitMsgHookTemplate = `#!/bin/sh
# armature:managed
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
# armature:managed
# Armature pre-commit hook: block ops log commits on code branches.
# Ops live on _armature — never on a code branch.
# To activate: cp this file to .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit
#
# This is defense-in-depth; .armature/.gitignore also blocks ops/ from being staged.

# Allow commits on _armature — that's exactly where ops belong.
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi

# Only block additions/modifications — deletions are allowed (cleanup commits).
if git diff --cached --name-only --diff-filter=AM | grep -q '\.armature/ops/'; then
  echo "ERROR: Refusing to commit .armature/ops/ changes on a code branch."
  echo "Ops are written directly to the _armature branch."
  exit 1
fi
`

// installHooks copies hook templates from .armature/hooks/ to .git/hooks/ and makes them executable.
// Existing hooks are skipped (and returned as skipped) only if they lack both the "# armature:managed"
// marker and the legacy Armature signature (#!/bin/sh shebang + "# Armature " header); legacy hooks
// are migrated/overwritten. Returns a list of skipped hook names.
// In dual-branch mode, the templates are in the worktree's .armature/hooks/.
func installHooks(repoPath string, issuesDir string) ([]string, error) {
	hooksDir := filepath.Join(issuesDir, "hooks")
	gitHooksDir := filepath.Join(repoPath, ".git", "hooks")

	if err := os.MkdirAll(gitHooksDir, 0o750); err != nil {
		return nil, fmt.Errorf("create .git/hooks directory: %w", err)
	}

	hooks := []string{"pre-commit", "post-commit", "post-merge", "prepare-commit-msg"}
	var skipped []string

	for _, hook := range hooks {
		templatePath := filepath.Join(hooksDir, hook+".sh.template")
		hookPath := filepath.Join(gitHooksDir, hook)

		content, err := os.ReadFile(templatePath) //nolint:gosec // G304: path constructed from internal hooks dir
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return skipped, fmt.Errorf("read hook template %s: %w", hook, err)
		}

		// Skip hooks that exist but were not written by Armature.
		if existing, readErr := os.ReadFile(hookPath); readErr == nil { //nolint:gosec // G304: internal hooks path
			isArmatureManaged := strings.Contains(string(existing), "# armature:managed")
			isLegacyArmatureHook := strings.HasPrefix(strings.TrimSpace(string(existing)), "#!/bin/sh") &&
				strings.Contains(string(existing), "\n# Armature ")
			if !isArmatureManaged && !isLegacyArmatureHook {
				skipped = append(skipped, hook)
				continue
			}
		} else if !os.IsNotExist(readErr) {
			return skipped, fmt.Errorf("check hook %s: %w", hook, readErr)
		}

		if err := os.WriteFile(hookPath, content, 0o755); err != nil { //nolint:gosec // git hooks require executable bit
			return skipped, fmt.Errorf("install hook %s: %w", hook, err)
		}
	}

	return skipped, nil
}

// migrateLegacySingleBranchOps detects and migrates a pre-existing single-branch .armature/ops layout
// to the new dual-branch layout. If legacy ops exist in repoPath/.armature/ops, they are moved to
// a timestamped backup directory .armature.migrated-<timestamp>, and the new dual-branch structure
// is set up on the _armature branch in the .arm worktree.
// Returns (true, backupDirPath) if migration was performed, (false, "") if no legacy layout was detected,
// or (false, error) if an error occurred.
func migrateLegacySingleBranchOps(repoPath string) (bool, string, error) {
	// Check if .armature/ops exists in the main working tree (legacy single-branch layout)
	legacyArmatureDir := filepath.Join(repoPath, ".armature")
	legacyOpsDir := filepath.Join(legacyArmatureDir, "ops")

	// If the legacy layout doesn't exist, no migration needed
	info, err := os.Stat(legacyOpsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("check for legacy layout: %w", err)
	}

	// Confirm it's a directory with content
	if !info.IsDir() {
		return false, "", nil
	}

	entries, err := os.ReadDir(legacyOpsDir)
	if err != nil {
		return false, "", fmt.Errorf("read legacy ops directory: %w", err)
	}

	// If the ops dir exists but is empty, no migration needed
	if len(entries) == 0 {
		return false, "", nil
	}

	// Legacy layout detected: rename .armature to .armature.migrated-<timestamp>
	timestamp := time.Now().Format("20060102150405")
	backupDir := filepath.Join(repoPath, fmt.Sprintf(".armature.migrated-%s", timestamp))

	if err := os.Rename(legacyArmatureDir, backupDir); err != nil {
		return false, "", fmt.Errorf("backup legacy .armature directory: %w", err)
	}

	return true, backupDir, nil
}

// copyLegacyOpsToNewWorktree copies the ops directory contents from the backup (created during migration)
// to the new worktree's .armature/ops directory. This preserves legacy issue data.
func copyLegacyOpsToNewWorktree(backupDir string, newOpsDir string) error {
	legacyOpsDir := filepath.Join(backupDir, "ops")

	// Read the legacy ops directory
	entries, err := os.ReadDir(legacyOpsDir)
	if err != nil {
		return fmt.Errorf("read legacy ops directory from backup: %w", err)
	}

	// Recursively copy all entries from legacy ops to new ops directory
	for _, entry := range entries {
		srcPath := filepath.Join(legacyOpsDir, entry.Name())
		dstPath := filepath.Join(newOpsDir, entry.Name())

		if err := copyRecursive(srcPath, dstPath); err != nil {
			return fmt.Errorf("copy legacy ops file %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// copyRecursive recursively copies a file or directory from src to dst.
// Note: uses os.Stat (follows symlinks) rather than os.Lstat, so symlinks in
// the source tree are copied as their target's contents rather than being
// preserved as symlinks. Legacy .armature/ops is not expected to contain
// symlinks in practice.
func copyRecursive(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if info.IsDir() {
		// Create destination directory
		if err := os.MkdirAll(dst, info.Mode()); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}

		// Recursively copy directory contents
		entries, err := os.ReadDir(src)
		if err != nil {
			return fmt.Errorf("read directory: %w", err)
		}

		for _, entry := range entries {
			srcPath := filepath.Join(src, entry.Name())
			dstPath := filepath.Join(dst, entry.Name())
			if err := copyRecursive(srcPath, dstPath); err != nil {
				return err
			}
		}
	} else {
		// Copy file
		content, err := os.ReadFile(src) //nolint:gosec // G304: src is constructed from legacyOpsDir
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}

		if err := os.WriteFile(dst, content, info.Mode()); err != nil { //nolint:gosec // dst is constructed from newOpsDir
			return fmt.Errorf("write file: %w", err)
		}
	}

	return nil
}

// runRepoSetup initializes the repository structure for Armature in dual-branch mode.
// Returns RepoSetupResult with status and any skipped hooks.
func runRepoSetup(cmd *cobra.Command, repoPath string) (RepoSetupResult, error) {
	// Resolve repoPath to an absolute path so stored paths are never relative.
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("resolve repo path: %w", err)
	}
	repoPath = absRepoPath

	gitClient := adapters.New(repoPath)

	// Attempt to migrate legacy single-branch layout if it exists
	migrated, backupDir, err := migrateLegacySingleBranchOps(repoPath)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("migrate legacy single-branch layout: %w", err)
	}
	if migrated {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated legacy single-branch .armature layout to timestamped backup\n")
	}

	// Always use dual-branch mode: create orphan branch _armature and .arm worktree
	// Create orphan branch _armature (idempotent)
	if err := gitClient.CreateOrphanBranch("_armature"); err != nil {
		return RepoSetupResult{}, fmt.Errorf("create _armature branch: %w", err)
	}

	// Create .arm/ worktree (idempotent)
	worktreePath := filepath.Join(repoPath, ".arm")
	if err := gitClient.AddWorktree("_armature", worktreePath); err != nil {
		return RepoSetupResult{}, fmt.Errorf("add .arm worktree: %w", err)
	}

	// Set git config keys for dual-branch mode
	if err := gitClient.SetGitConfig("armature.mode", "dual-branch"); err != nil {
		return RepoSetupResult{}, fmt.Errorf("set armature.mode: %w", err)
	}
	if err := gitClient.SetGitConfig("armature.ops-worktree-path", worktreePath); err != nil {
		return RepoSetupResult{}, fmt.Errorf("set armature.ops-worktree-path: %w", err)
	}

	issuesDir := filepath.Join(worktreePath, ".armature")

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
			return RepoSetupResult{}, fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Copy legacy ops data from backup if migration happened
	if migrated && backupDir != "" {
		if err := copyLegacyOpsToNewWorktree(backupDir, opsDir); err != nil {
			return RepoSetupResult{}, fmt.Errorf("copy legacy ops data: %w", err)
		}
	}

	// Write .gitignore to prevent state/ from being committed
	gitignorePath := filepath.Join(issuesDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(issuesGitignore), 0o600); err != nil {
		return RepoSetupResult{}, fmt.Errorf("write .armature/.gitignore: %w", err)
	}

	// Write SCHEMA file
	schemaPath := filepath.Join(issuesDir, "ops", "SCHEMA")
	if err := os.WriteFile(schemaPath, []byte(ops.GenerateSchema()), 0o600); err != nil {
		return RepoSetupResult{}, fmt.Errorf("write SCHEMA: %w", err)
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
			return RepoSetupResult{}, fmt.Errorf("write hook template %s: %w", hookName, err)
		}
	}

	// Install hooks from templates to .git/hooks/
	skippedHooks, err := installHooks(repoPath, issuesDir)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("install hooks: %w", err)
	}

	// Print warnings for skipped hooks to stderr
	for _, hookName := range skippedHooks {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: skipping git hook %s (not Armature-managed)\n", hookName)
	}

	// Detect project type and write config
	configPath := filepath.Join(issuesDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		projectType := config.DetectProjectType(repoPath)
		cfg := config.DefaultConfig(projectType)
		if err := config.WriteConfig(configPath, cfg); err != nil {
			return RepoSetupResult{}, fmt.Errorf("write config: %w", err)
		}
	}

	// Init worker if not already configured
	if ok, _ := worker.CheckWorkerID(repoPath); !ok {
		if _, err := worker.InitWorker(repoPath); err != nil {
			return RepoSetupResult{}, fmt.Errorf("init worker: %w", err)
		}
	}

	var status string
	if freshInit {
		status = "initialized"
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized Armature in dual-branch mode at %s\n", issuesDir)
	} else {
		status = "already_initialized"
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Armature already initialized in dual-branch mode at %s\n", issuesDir)
	}

	result := RepoSetupResult{
		Status:       status,
		SkippedHooks: skippedHooks,
	}
	return result, nil
}
