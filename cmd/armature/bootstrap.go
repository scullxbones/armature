package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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

# Push ops logs after each commit. A failed push (no network, no remote,
# permission denied) must never block or break the commit that already
# happened, so its exit status is explicitly ignored here.
arm push-ops 2>/dev/null || true
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
// Returns (true, backupDirPath, committed) if migration was performed, where committed reports
// whether a git commit was made removing .armature from tracking (so callers can roll back
// precisely if a later step fails); (false, "", false) if no legacy layout was detected;
// or (false, "", false, error) if an error occurred.
func migrateLegacySingleBranchOps(repoPath string) (bool, string, string, bool, error) {
	// Check if .armature/ops exists in the main working tree (legacy single-branch layout)
	legacyArmatureDir := filepath.Join(repoPath, config.StateDirName)
	legacyOpsDir := filepath.Join(legacyArmatureDir, "ops")
	preMigrationSHA := ""

	// A repoPath/.armature that is already a git worktree (its ".git" is a
	// worktree-pointer file, not a directory) is the collapsed-layout ops
	// worktree, not legacy flat data — treating it as legacy would rename
	// the real, current ops history away. Same hazard the .arm-worktree
	// pre-flight check above guards against, for the collapsed case.
	if gitMarker, err := os.Stat(filepath.Join(legacyArmatureDir, ".git")); err == nil && !gitMarker.IsDir() {
		return false, "", "", false, nil
	}

	// If the legacy layout doesn't exist, no migration needed
	info, err := os.Stat(legacyOpsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", "", false, nil
		}
		return false, "", preMigrationSHA, false, fmt.Errorf("check for legacy layout: %w", err)
	}

	// Confirm it's a directory with content
	if !info.IsDir() {
		return false, "", "", false, nil
	}

	entries, err := os.ReadDir(legacyOpsDir)
	if err != nil {
		return false, "", preMigrationSHA, false, fmt.Errorf("read legacy ops directory: %w", err)
	}

	// If the ops dir exists but is empty, no migration needed
	if len(entries) == 0 {
		return false, "", "", false, nil
	}

	// Legacy layout detected: rename .armature to .armature.migrated-<timestamp>.
	// The timestamp is second-resolution, and rolled-back migrations leave their
	// backup behind, so uniquify the name rather than renaming onto an existing dir.
	timestamp := time.Now().Format("20060102150405")
	backupDir := filepath.Join(repoPath, fmt.Sprintf(".armature.migrated-%s", timestamp))
	for i := 2; ; i++ {
		if _, err := os.Lstat(backupDir); os.IsNotExist(err) {
			break
		}
		backupDir = filepath.Join(repoPath, fmt.Sprintf(".armature.migrated-%s-%d", timestamp, i))
	}

	// Before renaming, check if .armature is tracked in git
	gitClient := adapters.New(repoPath)
	isTracked := gitClient.IsTracked(config.StateDirName)
	if isTracked {
		preMigrationSHA, err = gitClient.HeadSHA()
		if err != nil {
			return false, "", preMigrationSHA, false, fmt.Errorf("capture pre-migration HEAD: %w", err)
		}
	}

	// If tracked, remove from index to avoid leaving a dirty working tree after rename
	if isTracked {
		_ = gitClient.RemoveFromIndex(config.StateDirName) //nolint:errcheck // path might not be tracked
	}

	if err := os.Rename(legacyArmatureDir, backupDir); err != nil {
		// If the deletion was already staged, re-stage .armature so the index isn't
		// left pointing at a removal that never happened on disk.
		if isTracked {
			if addErr := gitClient.AddPaths([]string{config.StateDirName}); addErr != nil {
				return false, "", preMigrationSHA, false, fmt.Errorf("backup legacy .armature directory: %w; re-stage .armature after failed rename: %w", err, addErr)
			}
		}
		return false, "", preMigrationSHA, false, fmt.Errorf("backup legacy .armature directory: %w", err)
	}

	// If .armature was tracked, commit the removal to keep the working tree clean.
	// Scoped to the .armature path so it structurally cannot sweep in unrelated staged
	// changes; a real commit failure (not "nothing to commit") is propagated as an error.
	if isTracked {
		if err := gitClient.CommitPathsNoVerify("chore: migrate legacy .armature to dual-branch layout", config.StateDirName); err != nil {
			// Rollback on commit failure: restore the original .armature directory from the backup
			// and restore the index to its original state so the migration is atomic.
			if restoreErr := os.Rename(backupDir, legacyArmatureDir); restoreErr != nil {
				// If restore fails, the repo is in an inconsistent state; return both errors
				// with the backup path so the user can recover .armature manually.
				return false, "", preMigrationSHA, false, fmt.Errorf(
					"commit legacy .armature removal: %w; restore .armature from backup %s: %w",
					err, backupDir, restoreErr,
				)
			}

			// Re-add .armature to the index to restore the tracked state before the failed migration
			if restoreIndexErr := gitClient.AddPaths([]string{config.StateDirName}); restoreIndexErr != nil {
				// Directory is restored (the critical part); still surface the index
				// re-add failure alongside the original commit error.
				return false, "", preMigrationSHA, false, fmt.Errorf(
					"commit legacy .armature removal: %w; re-add .armature to index after rollback: %w",
					err, restoreIndexErr,
				)
			}

			return false, "", preMigrationSHA, false, fmt.Errorf("commit legacy .armature removal: %w", err)
		}
	}

	return true, backupDir, preMigrationSHA, isTracked, nil
}

// rollbackLegacyMigration undoes a legacy migration whose subsequent dual-branch setup
// (orphan branch creation or worktree add) failed, so the repo isn't left with .armature
// removed on disk/committed away while the new layout was never actually created.
//
// If committed is true, the migration made a commit removing .armature from tracking on
// the current branch; that commit is reverted with a hard reset to its parent (safe here
// because bootstrap requires a clean working tree before migration runs, and nothing else
// commits between the migration and this rollback). Otherwise, .armature was never
// git-tracked, so no commit exists to revert and the backup directory is simply renamed
// back into place.
func rollbackLegacyMigration(repoPath, backupDir, preMigrationSHA string, committed bool) error {
	if committed {
		gitClient := adapters.New(repoPath)
		target := preMigrationSHA
		if target == "" {
			target = "HEAD~1"
		}
		if err := gitClient.ResetHard(target); err != nil {
			return fmt.Errorf("revert migration commit: %w", err)
		}
		return nil
	}

	legacyArmatureDir := filepath.Join(repoPath, config.StateDirName)
	if err := os.Rename(backupDir, legacyArmatureDir); err != nil {
		return fmt.Errorf("restore .armature from backup %s: %w", backupDir, err)
	}
	return nil
}

// migrateDualBranchToCollapsed detects and migrates the current dual-branch .arm/.armature/
// layout to the collapsed .armature/ layout. The ops worktree is renamed from .arm/ to
// .armature/, and its inner .armature/ subdirectory contents move up to the root.
// Returns (true, backupDirPath) if migration was performed, (false, "") if no migration
// was needed, or (false, "", error) if an error occurred.
func migrateDualBranchToCollapsed(repoPath string) (bool, string, error) {
	gitClient := adapters.New(repoPath)

	// Check if the dual-branch layout exists (.arm worktree with inner .armature/)
	armWorktreePath := filepath.Join(repoPath, ".arm")
	innerArmaturePath := filepath.Join(armWorktreePath, config.StateDirName)

	// Check if .arm worktree exists. Any error here (missing, or .arm exists
	// but isn't a directory containing a worktree pointer) means there is no
	// dual-branch layout to migrate — leave diagnosing a malformed .arm to
	// whatever step tries to use it next (e.g. AddWorktree), not this check.
	if _, err := os.Stat(filepath.Join(armWorktreePath, ".git")); err != nil {
		return false, "", nil
	}

	// Check if the inner .armature/ directory exists within .arm/
	if _, err := os.Stat(innerArmaturePath); err != nil {
		if os.IsNotExist(err) {
			// Dual-branch layout doesn't exist, no migration needed
			return false, "", nil
		}
		return false, "", fmt.Errorf("check for inner .armature/ directory: %w", err)
	}

	// Check if the _armature worktree has uncommitted changes (reject if it does)
	armtreeGitClient := adapters.New(armWorktreePath)
	dirty, err := armtreeGitClient.IsWorkingTreeDirty()
	if err != nil {
		return false, "", fmt.Errorf("check _armature worktree for uncommitted changes: %w", err)
	}
	if dirty {
		return false, "", fmt.Errorf(
			"_armature worktree has uncommitted changes; please commit or stash them before running bootstrap",
		)
	}

	// Dual-branch layout detected: back up .arm/ to a timestamped directory
	timestamp := time.Now().Format("20060102150405")
	backupDir := filepath.Join(repoPath, fmt.Sprintf(".arm.collapsed-%s", timestamp))
	for i := 2; ; i++ {
		if _, err := os.Lstat(backupDir); os.IsNotExist(err) {
			break
		}
		backupDir = filepath.Join(repoPath, fmt.Sprintf(".arm.collapsed-%s-%d", timestamp, i))
	}

	// Rename .arm/ to the timestamped backup
	if err := os.Rename(armWorktreePath, backupDir); err != nil {
		return false, "", fmt.Errorf("backup .arm worktree to %s: %w", backupDir, err)
	}

	// Remove the old .arm worktree from git's worktree registry
	// The worktree has already been moved, so this may fail, but we try anyway
	_ = gitClient.RemoveWorktree(armWorktreePath) //nolint:errcheck // already moved, may fail

	// Add the new collapsed .armature/ worktree
	newWorktreePath := filepath.Join(repoPath, config.StateDirName)
	if err := gitClient.AddWorktree("_armature", newWorktreePath); err != nil {
		// Rollback: restore .arm/ from backup
		if restoreErr := os.Rename(backupDir, armWorktreePath); restoreErr != nil {
			return false, "", fmt.Errorf(
				"add .armature worktree: %w; restore .arm from backup: %w (backup at %s)",
				err, restoreErr, backupDir,
			)
		}
		// Try to re-register the restored .arm worktree (best effort)
		_ = gitClient.AddWorktree("_armature", armWorktreePath) //nolint:errcheck
		return false, "", fmt.Errorf("add .armature worktree: %w (migration rolled back)", err)
	}

	// Copy contents from backup's inner .armature/ to the new collapsed .armature/
	// The new .armature/ worktree already has its own ops/ structure from the _armature branch,
	// so we merge legacy ops/templates/hooks/review if they exist in the backup
	legacyInnerArmaturePath := filepath.Join(backupDir, config.StateDirName)
	if _, err := os.Stat(legacyInnerArmaturePath); err == nil {
		// Legacy inner .armature/ exists in backup, merge its contents
		skippedCount, err := copyLegacyOpsToNewWorktree(legacyInnerArmaturePath, newWorktreePath)
		if err != nil {
			// Rollback: remove new worktree and restore old one
			if removeErr := gitClient.RemoveWorktree(newWorktreePath); removeErr != nil {
				return false, "", fmt.Errorf(
					"copy legacy ops from backup: %w; remove failed .armature worktree: %w (backup at %s)",
					err, removeErr, backupDir,
				)
			}
			restoreErr := os.Rename(backupDir, armWorktreePath)
			if restoreErr != nil {
				return false, "", fmt.Errorf(
					"copy legacy ops from backup: %w; restore .arm from backup: %w (backup at %s)",
					err, restoreErr, backupDir,
				)
			}
			if reregisterErr := gitClient.AddWorktree("_armature", armWorktreePath); reregisterErr != nil {
				return false, "", fmt.Errorf(
					"copy legacy ops from backup: %w; restore .arm from backup: %w (backup at %s); re-register .arm worktree: %w",
					err, restoreErr, backupDir, reregisterErr,
				)
			}
			return false, "", fmt.Errorf("copy legacy ops from backup: %w (migration rolled back)", err)
		}
		_ = skippedCount // best-effort merge count; not currently surfaced for this migration path

		// config.json lives directly under the legacy inner .armature/, not one
		// of the legacyDirs subdirectories copyLegacyOpsToNewWorktree handles;
		// copy it too so migration doesn't silently reset the user's config.
		legacyConfigPath := filepath.Join(legacyInnerArmaturePath, "config.json")
		if _, err := os.Stat(legacyConfigPath); err == nil {
			if _, err := copyRecursive(legacyConfigPath, filepath.Join(newWorktreePath, "config.json")); err != nil {
				if removeErr := gitClient.RemoveWorktree(newWorktreePath); removeErr != nil {
					return false, "", fmt.Errorf(
						"copy legacy config.json from backup: %w; remove failed .armature worktree: %w (backup at %s)",
						err, removeErr, backupDir,
					)
				}
				restoreErr := os.Rename(backupDir, armWorktreePath)
				if restoreErr != nil {
					return false, "", fmt.Errorf(
						"copy legacy config.json from backup: %w; restore .arm from backup: %w (backup at %s)",
						err, restoreErr, backupDir,
					)
				}
				if reregisterErr := gitClient.AddWorktree("_armature", armWorktreePath); reregisterErr != nil {
					return false, "", fmt.Errorf(
						"copy legacy config.json from backup: %w; restore .arm from backup: %w (backup at %s); re-register .arm worktree: %w",
						err, restoreErr, backupDir, reregisterErr,
					)
				}
				return false, "", fmt.Errorf("copy legacy config.json from backup: %w (migration rolled back)", err)
			}
		}

		// The new worktree's checkout of the _armature branch may still have a nested
		// .armature/ subtree (committed before the collapse, when everything lived under
		// .armature/.armature/...). Its contents were just copied to the worktree root
		// above, so the nested copy is now stale; remove it from both the index and disk
		// and commit the newly-copied root-level files, mirroring the commit pattern the
		// legacy single-branch migration uses after copying its data (see the
		// AddPaths/CommitPathsNoVerify call in runRepoSetup after migrateLegacySingleBranchOps).
		worktreeGitClient := adapters.New(newWorktreePath)
		staleNestedPath := filepath.Join(newWorktreePath, config.StateDirName)
		if _, statErr := os.Stat(staleNestedPath); statErr == nil {
			if err := worktreeGitClient.RemoveTree(config.StateDirName); err != nil {
				if removeErr := gitClient.RemoveWorktree(newWorktreePath); removeErr != nil {
					return false, "", fmt.Errorf(
						"remove stale nested %s subtree: %w; remove failed %s worktree: %w (backup at %s)",
						config.StateDirName, err, config.StateDirName, removeErr, backupDir,
					)
				}
				restoreErr := os.Rename(backupDir, armWorktreePath)
				if restoreErr != nil {
					return false, "", fmt.Errorf(
						"remove stale nested %s subtree: %w; restore .arm from backup: %w (backup at %s)",
						config.StateDirName, err, restoreErr, backupDir,
					)
				}
				if reregisterErr := gitClient.AddWorktree("_armature", armWorktreePath); reregisterErr != nil {
					return false, "", fmt.Errorf(
						"remove stale nested %s subtree: %w; restore .arm from backup: %w (backup at %s); re-register .arm worktree: %w",
						config.StateDirName, err, restoreErr, backupDir, reregisterErr,
					)
				}
				return false, "", fmt.Errorf("remove stale nested %s subtree: %w (migration rolled back)", config.StateDirName, err)
			}
		}

		// Stage whichever root-level directories/files actually exist after copying
		// (git add errors on a pathspec that matches nothing, so only stage what's there).
		candidatePaths := []string{"ops", "templates", "hooks", "review", "sources", "config.json"}
		var filesToStage []string
		for _, p := range candidatePaths {
			if _, err := os.Stat(filepath.Join(newWorktreePath, p)); err == nil {
				filesToStage = append(filesToStage, p)
			}
		}
		if len(filesToStage) > 0 {
			if err := worktreeGitClient.AddPaths(filesToStage); err != nil {
				if removeErr := gitClient.RemoveWorktree(newWorktreePath); removeErr != nil {
					return false, "", fmt.Errorf(
						"stage collapsed layout files: %w; remove failed %s worktree: %w (backup at %s)",
						err, config.StateDirName, removeErr, backupDir,
					)
				}
				restoreErr := os.Rename(backupDir, armWorktreePath)
				if restoreErr != nil {
					return false, "", fmt.Errorf(
						"stage collapsed layout files: %w; restore .arm from backup: %w (backup at %s)",
						err, restoreErr, backupDir,
					)
				}
				if reregisterErr := gitClient.AddWorktree("_armature", armWorktreePath); reregisterErr != nil {
					return false, "", fmt.Errorf(
						"stage collapsed layout files: %w; restore .arm from backup: %w (backup at %s); re-register .arm worktree: %w",
						err, restoreErr, backupDir, reregisterErr,
					)
				}
				return false, "", fmt.Errorf("stage collapsed layout files: %w (migration rolled back)", err)
			}
		}

		// Commit the removal of the stale nested subtree and the newly-staged root-level
		// files to the _armature branch, so the collapsed layout (and its full ops history)
		// is visible to every clone, not just the machine that ran the migration locally.
		if err := worktreeGitClient.CommitPathsNoVerify(
			"chore: collapse .arm/.armature dual-branch layout to single .armature worktree",
			".",
		); err != nil {
			if removeErr := gitClient.RemoveWorktree(newWorktreePath); removeErr != nil {
				return false, "", fmt.Errorf(
					"commit collapsed layout: %w; remove failed %s worktree: %w (backup at %s)",
					err, config.StateDirName, removeErr, backupDir,
				)
			}
			restoreErr := os.Rename(backupDir, armWorktreePath)
			if restoreErr != nil {
				return false, "", fmt.Errorf(
					"commit collapsed layout: %w; restore .arm from backup: %w (backup at %s)",
					err, restoreErr, backupDir,
				)
			}
			if reregisterErr := gitClient.AddWorktree("_armature", armWorktreePath); reregisterErr != nil {
				return false, "", fmt.Errorf(
					"commit collapsed layout: %w; restore .arm from backup: %w (backup at %s); re-register .arm worktree: %w",
					err, restoreErr, backupDir, reregisterErr,
				)
			}
			return false, "", fmt.Errorf("commit collapsed layout: %w (migration rolled back)", err)
		}
	}

	// Update git config to point to the new worktree path
	if err := gitClient.SetGitConfig("armature.ops-worktree-path", newWorktreePath); err != nil {
		// Rollback: remove new worktree and restore old one
		if removeErr := gitClient.RemoveWorktree(newWorktreePath); removeErr != nil {
			return false, "", fmt.Errorf(
				"set git config: %w; remove failed .armature worktree: %w (backup at %s)",
				err, removeErr, backupDir,
			)
		}
		restoreErr := os.Rename(backupDir, armWorktreePath)
		if restoreErr != nil {
			return false, "", fmt.Errorf(
				"set git config: %w; restore .arm from backup: %w (backup at %s)",
				err, restoreErr, backupDir,
			)
		}
		if reregisterErr := gitClient.AddWorktree("_armature", armWorktreePath); reregisterErr != nil {
			return false, "", fmt.Errorf(
				"set git config: %w; restore .arm from backup: %w (backup at %s); re-register .arm worktree: %w",
				err, restoreErr, backupDir, reregisterErr,
			)
		}
		return false, "", fmt.Errorf("set git config: %w (migration rolled back)", err)
	}

	return true, backupDir, nil
}

// copyLegacyOpsToNewWorktree copies the ops/, templates/, hooks/, review/, and sources/
// directory contents from the backup (created during migration) into the corresponding
// directories of the new worktree's .armature/, preserving all legacy data, not just ops/.
// Returns the count of destination files that already existed and were therefore skipped
// (not overwritten), so callers can surface a summary to the user.
func copyLegacyOpsToNewWorktree(backupDir string, newIssuesDir string) (int, error) {
	// Subdirectories under legacy .armature/ that may hold user data worth preserving.
	// "ops" is required (callers only invoke this when legacy ops exist); the rest are
	// copied best-effort if present, since older layouts may not have them.
	legacyDirs := []string{"ops", "templates", "hooks", "review", "sources"}

	skippedCount := 0
	for _, dirName := range legacyDirs {
		legacyDir := filepath.Join(backupDir, dirName)
		entries, err := os.ReadDir(legacyDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue // optional legacy directory not present; nothing to copy
			}
			return skippedCount, fmt.Errorf("read legacy %s directory from backup: %w", dirName, err)
		}

		newDir := filepath.Join(newIssuesDir, dirName)
		if err := os.MkdirAll(newDir, 0o750); err != nil {
			return skippedCount, fmt.Errorf("create %s directory: %w", dirName, err)
		}
		for _, entry := range entries {
			srcPath := filepath.Join(legacyDir, entry.Name())
			dstPath := filepath.Join(newDir, entry.Name())

			if dirName == "ops" && strings.HasSuffix(entry.Name(), ".log") {
				if _, err := os.Stat(dstPath); err == nil {
					appended, err := mergeAppendOnlyLog(srcPath, dstPath)
					if err != nil {
						return skippedCount, fmt.Errorf("merge legacy %s log %s: %w", dirName, entry.Name(), err)
					}
					skippedCount += appended
					continue
				}
			}

			skipped, err := copyRecursive(srcPath, dstPath)
			if err != nil {
				return skippedCount, fmt.Errorf("copy legacy %s file %s: %w", dirName, entry.Name(), err)
			}
			skippedCount += skipped
		}
	}

	return skippedCount, nil
}

// listMigrationBackups returns sorted base names of stranded migration backups.
// It is best-effort and returns nil when the repo cannot be read.
func listMigrationBackups(repoPath string) []string {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil
	}

	var backups []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".armature.migrated-") {
			backups = append(backups, name)
		}
	}
	slices.Sort(backups)
	return backups
}

// mergeAppendOnlyLog appends each non-empty line from src to dst if it is not already present.
// It preserves the order of the source and treats identical logs as a no-op.
func mergeAppendOnlyLog(srcPath, dstPath string) (int, error) {
	srcContent, err := os.ReadFile(srcPath) //nolint:gosec // G304: srcPath is derived from controlled legacy repo paths
	if err != nil {
		return 0, fmt.Errorf("read source log: %w", err)
	}
	dstContent, err := os.ReadFile(dstPath) //nolint:gosec // G304: dstPath is derived from controlled repo paths
	if err != nil {
		return 0, fmt.Errorf("read destination log: %w", err)
	}

	existing := make(map[string]struct{})
	for _, line := range strings.Split(strings.TrimRight(string(dstContent), "\n"), "\n") {
		if line != "" {
			existing[line] = struct{}{}
		}
	}

	var merged []string
	appended := 0
	for _, line := range strings.Split(strings.TrimRight(string(srcContent), "\n"), "\n") {
		if line == "" {
			continue
		}
		if _, ok := existing[line]; ok {
			continue
		}
		existing[line] = struct{}{}
		merged = append(merged, line)
		appended++
	}

	if appended == 0 {
		return 0, nil
	}

	mergedContent := string(dstContent)
	if len(mergedContent) > 0 && !strings.HasSuffix(mergedContent, "\n") {
		mergedContent += "\n"
	}
	mergedContent += strings.Join(merged, "\n")
	if !strings.HasSuffix(mergedContent, "\n") {
		mergedContent += "\n"
	}
	if err := os.WriteFile(dstPath, []byte(mergedContent), 0o600); err != nil { //nolint:gosec // G304: dstPath is derived from controlled repo paths
		return 0, fmt.Errorf("write merged log: %w", err)
	}
	return appended, nil
}

// copyRecursive recursively copies a file or directory from src to dst.
// If a file already exists at the destination, it is NOT overwritten (skip it).
// This preserves any newer or hand-crafted files at the destination.
// Note: uses os.Stat (follows symlinks) rather than os.Lstat, so symlinks in
// the source tree are copied as their target's contents rather than being
// preserved as symlinks. Legacy .armature/ops is not expected to contain
// symlinks in practice.
// Returns the count of files skipped because a destination file already existed.
func copyRecursive(src string, dst string) (int, error) {
	info, err := os.Stat(src)
	if err != nil {
		return 0, fmt.Errorf("stat source: %w", err)
	}

	if info.IsDir() {
		// Create destination directory
		if err := os.MkdirAll(dst, info.Mode()); err != nil {
			return 0, fmt.Errorf("create directory: %w", err)
		}

		// Recursively copy directory contents
		entries, err := os.ReadDir(src)
		if err != nil {
			return 0, fmt.Errorf("read directory: %w", err)
		}

		skippedCount := 0
		for _, entry := range entries {
			srcPath := filepath.Join(src, entry.Name())
			dstPath := filepath.Join(dst, entry.Name())
			skipped, err := copyRecursive(srcPath, dstPath)
			if err != nil {
				return skippedCount, err
			}
			skippedCount += skipped
		}
		return skippedCount, nil
	}

	// Check if destination file already exists
	if _, err := os.Stat(dst); err == nil {
		// File exists at destination, skip it (don't overwrite)
		return 1, nil
	} else if !os.IsNotExist(err) {
		// Some other error checking the destination
		return 0, fmt.Errorf("stat destination: %w", err)
	}

	// Destination file does not exist, safe to copy
	content, err := os.ReadFile(src) //nolint:gosec // G304: src is constructed from legacyOpsDir
	if err != nil {
		return 0, fmt.Errorf("read file: %w", err)
	}

	if err := os.WriteFile(dst, content, info.Mode()); err != nil { //nolint:gosec // dst is constructed from newOpsDir
		return 0, fmt.Errorf("write file: %w", err)
	}

	return 0, nil
}

// excludeArmWorktreeFromGit adds .arm/ to .git/info/exclude so the worktree is not tracked by git.
// This is idempotent: if .arm/ is already in the exclude file, it won't be duplicated.
func excludeArmWorktreeFromGit(repoPath string) error {
	return updateGitExclude(repoPath, ".arm/", "")
}

// updateGitExclude adds an exclude pattern to .git/info/exclude and optionally removes another.
// This is idempotent: if the pattern to add is already present, it won't be duplicated.
// If removePattern is non-empty and present, it will be removed before the new pattern is added.
func updateGitExclude(repoPath string, addPattern, removePattern string) error {
	excludePath := filepath.Join(repoPath, ".git", "info", "exclude")

	// Create the info directory if it doesn't exist
	infoDir := filepath.Dir(excludePath)
	if err := os.MkdirAll(infoDir, 0o750); err != nil {
		return fmt.Errorf("create .git/info directory: %w", err)
	}

	// Read the current exclude file (it may not exist yet)
	var currentContent string
	if data, err := os.ReadFile(excludePath); err == nil { //nolint:gosec // G304: path is constructed from repo/.git/info/exclude
		currentContent = string(data)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read .git/info/exclude: %w", err)
	}

	// Remove the pattern to remove (if specified)
	var newContent string
	if removePattern != "" {
		var filteredLines []string
		for line := range strings.SplitSeq(currentContent, "\n") {
			if strings.TrimSpace(line) != removePattern {
				filteredLines = append(filteredLines, line)
			}
		}
		newContent = strings.Join(filteredLines, "\n")
	} else {
		newContent = currentContent
	}

	// Check if the pattern to add is already in the exclude file
	found := false
	for line := range strings.SplitSeq(newContent, "\n") {
		if strings.TrimSpace(line) == addPattern {
			found = true
			break
		}
	}

	// If not found, append it
	if !found {
		if len(newContent) > 0 && !strings.HasSuffix(newContent, "\n") {
			newContent += "\n"
		}
		newContent += addPattern + "\n"
	}

	if err := os.WriteFile(excludePath, []byte(newContent), 0o600); err != nil { //nolint:gosec // G703: path is constructed from repo/.git/info/exclude
		return fmt.Errorf("write .git/info/exclude: %w", err)
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

	// Pre-flight: refuse to run against a checkout of the ops branch itself (e.g. the
	// .arm worktree). Its .armature/ops would otherwise be mistaken for a legacy
	// single-branch layout and "migrated" — renaming the real dual-branch data away.
	if branch, err := gitClient.CurrentBranch(); err == nil && branch == "_armature" {
		return RepoSetupResult{}, fmt.Errorf(
			"refusing to bootstrap a checkout of the _armature ops branch (path %s): run bootstrap from the main repository instead", repoPath,
		)
	}

	// Pre-flight: refuse to touch anything if the working tree is dirty. This makes
	// bootstrap atomic with respect to this check — either the tree is clean at the
	// start (so migration below cannot sweep in unrelated staged changes) or bootstrap
	// refuses before doing anything, including renaming a legacy .armature directory.
	dirty, err := gitClient.IsWorkingTreeDirty()
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("check working tree: %w", err)
	}
	if dirty {
		return RepoSetupResult{}, fmt.Errorf(
			"working tree is dirty (contains uncommitted changes): please commit or stash your changes before running bootstrap",
		)
	}

	// Attempt to migrate legacy single-branch layout if it exists
	migrated, backupDir, preMigrationSHA, migrationCommitted, err := migrateLegacySingleBranchOps(repoPath)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("migrate legacy single-branch layout: %w", err)
	}
	if migrated {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated legacy single-branch .armature layout to timestamped backup at %s\n", backupDir)
	}

	// Attempt to migrate dual-branch layout to collapsed layout if it exists
	dualMigrated, dualBackupDir, err := migrateDualBranchToCollapsed(repoPath)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("migrate dual-branch layout to collapsed: %w", err)
	}
	if dualMigrated {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated dual-branch .arm/.armature layout to collapsed .armature at timestamped backup %s\n", dualBackupDir)
		// Update git exclude to use .armature/ instead of .arm/
		if err := updateGitExclude(repoPath, config.StateDirName+"/", ".arm/"); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update .git/info/exclude after migration: %v\n", err)
		}
	}

	// Always use dual-branch mode: create orphan branch _armature and .arm worktree
	// Create orphan branch _armature (idempotent)
	if err := gitClient.CreateOrphanBranch("_armature"); err != nil {
		if migrated {
			if rbErr := rollbackLegacyMigration(repoPath, backupDir, preMigrationSHA, migrationCommitted); rbErr != nil {
				return RepoSetupResult{}, fmt.Errorf(
					"create _armature branch: %w; additionally, rollback of legacy migration failed: %w (backup left at %s)",
					err, rbErr, backupDir,
				)
			}
			if migrationCommitted && preMigrationSHA != "" {
				// The reset restored tracked files, but the backup is the only copy of
				// any legacy files that were untracked at migration time.
				return RepoSetupResult{}, fmt.Errorf("create _armature branch: %w (migration rolled back; backup left at %s)", err, backupDir)
			}
		}
		return RepoSetupResult{}, fmt.Errorf("create _armature branch: %w", err)
	}

	// Determine the correct worktree path based on layout
	// After dual-branch to collapsed migration, worktree is at .armature/
	// Otherwise, worktree is at .arm/
	var worktreePath string
	var isCollapsedLayout bool

	// alreadyCollapsedWorktreePath returns whether repoPath/StateDirName is
	// itself the _armature ops worktree (its ".git" is a worktree-pointer file,
	// not a directory). This distinguishes an already-migrated repo (bootstrap
	// must keep using the collapsed path) from a repo that has never had a
	// worktree there (fresh init still defaults to the .arm/ layout below).
	alreadyCollapsed := false
	if gitMarker, statErr := os.Stat(filepath.Join(repoPath, config.StateDirName, ".git")); statErr == nil && !gitMarker.IsDir() {
		alreadyCollapsed = true
	}

	// A repo with neither a legacy .arm/ worktree nor an already-collapsed
	// .armature/ worktree is a fresh init: it goes straight to the collapsed
	// layout, matching the design target that dual-branch is never the resting
	// state for a repo bootstrap creates from scratch (only a pre-existing
	// dual-branch repo transits through it, via the dualMigrated path above).
	hasPreExistingArmWorktree := false
	if gitMarker, statErr := os.Stat(filepath.Join(repoPath, ".arm", ".git")); statErr == nil && !gitMarker.IsDir() {
		hasPreExistingArmWorktree = true
	}

	switch {
	case dualMigrated, alreadyCollapsed:
		worktreePath = filepath.Join(repoPath, config.StateDirName) // .armature/
		isCollapsedLayout = true
	case hasPreExistingArmWorktree:
		worktreePath = filepath.Join(repoPath, ".arm")
		isCollapsedLayout = false
	default:
		worktreePath = filepath.Join(repoPath, config.StateDirName) // .armature/
		isCollapsedLayout = true
	}

	// Create worktree if not already exists (idempotent)
	worktreeLabel := filepath.Base(worktreePath)
	if err := gitClient.AddWorktree("_armature", worktreePath); err != nil {
		if migrated {
			if rbErr := rollbackLegacyMigration(repoPath, backupDir, preMigrationSHA, migrationCommitted); rbErr != nil {
				return RepoSetupResult{}, fmt.Errorf(
					"add %s worktree: %w; additionally, rollback of legacy migration failed: %w (backup left at %s)",
					worktreeLabel, err, rbErr, backupDir,
				)
			}
			if migrationCommitted && preMigrationSHA != "" {
				// The reset restored tracked files, but the backup is the only copy of
				// any legacy files that were untracked at migration time.
				return RepoSetupResult{}, fmt.Errorf("add %s worktree: %w (migration rolled back; backup left at %s)", worktreeLabel, err, backupDir)
			}
		}
		return RepoSetupResult{}, fmt.Errorf("add %s worktree: %w", worktreeLabel, err)
	}

	// Exclude worktree from git tracking.
	// The dualMigrated path already updated the exclude file (.arm/ -> .armature/)
	// below; this covers the fresh-init and pre-existing-.arm cases.
	if !dualMigrated {
		if isCollapsedLayout {
			if err := updateGitExclude(repoPath, config.StateDirName+"/", ""); err != nil {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: failed to exclude %s/ from git tracking: %v\n", config.StateDirName, err)
			}
		} else {
			if err := excludeArmWorktreeFromGit(repoPath); err != nil {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: failed to exclude .arm/ from git tracking: %v\n", err)
			}
		}
	}

	// Set git config keys for current layout
	// Note: armature.mode is intentionally not written here; nothing reads it anymore
	// (dual-branch is the only mode for now; collapsed is T3+), so it would be dead legacy-compat state.
	if err := gitClient.SetGitConfig("armature.ops-worktree-path", worktreePath); err != nil {
		return RepoSetupResult{}, fmt.Errorf("set armature.ops-worktree-path: %w", err)
	}

	// Determine issuesDir based on layout
	// In collapsed layout, WorktreePath == IssuesDir
	// In dual-branch layout, IssuesDir == WorktreePath/.armature/
	var issuesDir string
	if isCollapsedLayout {
		issuesDir = worktreePath
	} else {
		issuesDir = filepath.Join(worktreePath, config.StateDirName)
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
			return RepoSetupResult{}, fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Load and prepare config early, before committing migrated data.
	// This ensures custom config (if migrated) is included in the bootstrap commit.
	configPath := filepath.Join(issuesDir, "config.json")
	var configToWrite *config.Config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		var cfg config.Config

		// If migration happened, try to load legacy config from backup
		if migrated && backupDir != "" {
			legacyConfigPath := filepath.Join(backupDir, "config.json")
			if legacyConfig, err := config.LoadConfig(legacyConfigPath); err == nil {
				// Legacy config loaded successfully, use it
				cfg = legacyConfig
			} else {
				// Absent legacy config is normal; anything else means the user HAD a
				// config that is being replaced — say so instead of silently defaulting.
				if !errors.Is(err, os.ErrNotExist) {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"Warning: legacy config.json could not be loaded (%v); using default config (original preserved at %s)\n",
						err, legacyConfigPath)
				}
				projectType := config.DetectProjectType(repoPath)
				cfg = config.DefaultConfig(projectType)
			}
		} else {
			// No migration, detect project type and use defaults
			projectType := config.DetectProjectType(repoPath)
			cfg = config.DefaultConfig(projectType)
		}

		configToWrite = &cfg
	}

	// Copy legacy ops data from backup if migration happened
	if migrated && backupDir != "" {
		skippedCount, err := copyLegacyOpsToNewWorktree(backupDir, issuesDir)
		if err != nil {
			return RepoSetupResult{}, fmt.Errorf("copy legacy ops data (preserved at %s): %w", backupDir, err)
		}
		if skippedCount > 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%d legacy file(s) already present in new worktree, not overwritten\n", skippedCount)
		}

		// Write config before committing, so it's included in the migration commit
		if configToWrite != nil {
			if err := config.WriteConfig(configPath, *configToWrite); err != nil {
				return RepoSetupResult{}, fmt.Errorf("write config: %w", err)
			}
		}

		// Commit the migrated ops files and config to the _armature branch so they're preserved for other clones.
		// Use a gitClient scoped to the worktree to commit within that working tree.
		worktreeGitClient := adapters.New(worktreePath)

		// In collapsed layout, worktreePath == issuesDir, so paths are relative
		// to the worktree root directly. In dual-branch layout, issuesDir is the
		// inner config.StateDirName subdirectory of worktreePath, so paths need
		// that prefix.
		stagePrefix := ""
		commitScope := "."
		if !isCollapsedLayout {
			stagePrefix = config.StateDirName + "/"
			commitScope = config.StateDirName
		}

		// Stage the copied ops/templates/hooks/review files and config
		filesToStage := []string{
			stagePrefix + "ops",
			stagePrefix + "templates",
			stagePrefix + "hooks",
			stagePrefix + "review",
		}
		if configToWrite != nil {
			filesToStage = append(filesToStage, stagePrefix+"config.json")
		}
		if err := worktreeGitClient.AddPaths(filesToStage); err != nil {
			return RepoSetupResult{}, fmt.Errorf("stage migrated data (legacy data preserved at %s): %w", backupDir, err)
		}

		// Commit the staged files (scoped to cover both ops and config.json)
		if err := worktreeGitClient.CommitPathsNoVerify(
			"chore: commit migrated legacy ops and config from single-branch layout",
			commitScope,
		); err != nil {
			return RepoSetupResult{}, fmt.Errorf("commit migrated data to _armature branch (legacy data preserved at %s): %w", backupDir, err)
		}
	}

	// Write .gitignore to prevent state/ from being committed
	gitignorePath := filepath.Join(issuesDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(issuesGitignore), 0o600); err != nil {
		return RepoSetupResult{}, fmt.Errorf("write %s/.gitignore: %w", config.StateDirName, err)
	}

	// Write SCHEMA file
	schemaPath := filepath.Join(issuesDir, "ops", "SCHEMA")
	if err := os.WriteFile(schemaPath, []byte(ops.GenerateSchema()), 0o600); err != nil {
		return RepoSetupResult{}, fmt.Errorf("write SCHEMA: %w", err)
	}

	if backups := listMigrationBackups(repoPath); len(backups) > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note: stranded migration backups remain: %s\n", strings.Join(backups, ", "))
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

	// Write config if not already written during migration
	// (configPath was defined earlier before migration block, so it's in scope throughout)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		var cfg config.Config

		// For non-migration case, detect project type and use defaults
		// (For migration case, config was already prepared and written above)
		projectType := config.DetectProjectType(repoPath)
		cfg = config.DefaultConfig(projectType)

		if err := config.WriteConfig(configPath, cfg); err != nil {
			return RepoSetupResult{}, fmt.Errorf("write config: %w", err)
		}

		// Commit the generated config to the _armature branch so it's preserved in git
		// history and pushed to other clones. Gated on config.json having just been
		// written here (not on freshInit): a repo can be non-fresh (e.g. _armature was
		// adopted from a remote with ops/ but no config.json) yet still need this new
		// config.json committed, or it's silently unpreserved outside this worktree.
		worktreeGitClient := adapters.New(worktreePath)

		// In collapsed layout, worktreePath == issuesDir, so config.json lives
		// at the worktree root. In dual-branch layout it's nested a level down.
		configRelPath := "config.json"
		commitScope := "."
		if !isCollapsedLayout {
			configRelPath = config.StateDirName + "/config.json"
			commitScope = config.StateDirName
		}

		// Stage the config file
		if err := worktreeGitClient.AddPaths([]string{configRelPath}); err != nil {
			return RepoSetupResult{}, fmt.Errorf("stage config: %w", err)
		}

		// Commit the config to _armature branch
		if err := worktreeGitClient.CommitPathsNoVerify(
			"chore: init armature config",
			commitScope,
		); err != nil {
			return RepoSetupResult{}, fmt.Errorf("commit config to _armature branch: %w", err)
		}
	}

	// Init worker if not already configured
	if ok, _ := worker.CheckWorkerID(repoPath); !ok {
		if _, err := worker.InitWorker(repoPath); err != nil {
			return RepoSetupResult{}, fmt.Errorf("init worker: %w", err)
		}
	}

	// If the repo is still on the dual-branch .arm/.armature/ layout at this point,
	// immediately migrate it to collapsed layout in the same call, so a single
	// `arm bootstrap` invocation always converges to the collapsed layout regardless
	// of which legacy state the repo started in (LNGHZN-S1-T3). This covers not just
	// the legacy-single-branch-migration case (migrated == true) but also a
	// pre-existing .arm/ worktree that had no inner .armature/ yet: the setup above
	// just created that inner directory structure for the first time, so
	// migrateDualBranchToCollapsed could not have found it on the earlier call above
	// but will find it now.
	if !dualMigrated && !isCollapsedLayout {
		chainedDualMigrated, chainedDualBackupDir, err := migrateDualBranchToCollapsed(repoPath)
		if err != nil {
			return RepoSetupResult{}, fmt.Errorf("chain dual-branch to collapsed migration after legacy migration: %w", err)
		}
		if chainedDualMigrated {
			// Update tracking variables and paths after successful migration
			dualMigrated = true
			isCollapsedLayout = true
			worktreePath = filepath.Join(repoPath, config.StateDirName)
			issuesDir = worktreePath // In collapsed layout, issuesDir == worktreePath
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated dual-branch .arm/.armature layout to collapsed .armature at timestamped backup %s\n", chainedDualBackupDir)
			// Update git config with the new collapsed worktree path
			if err := gitClient.SetGitConfig("armature.ops-worktree-path", worktreePath); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update git config after migration: %v\n", err)
			}
			// Update git exclude to use .armature/ instead of .arm/
			if err := updateGitExclude(repoPath, config.StateDirName+"/", ".arm/"); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update .git/info/exclude after migration: %v\n", err)
			}
			// Recreate state directories in the new collapsed worktree (they were created in .arm/ before migration)
			stateDir := filepath.Join(issuesDir, "state")
			if err := os.MkdirAll(filepath.Join(stateDir, "issues"), 0o750); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to recreate state directories after migration: %v\n", err)
			}
		}
	}

	var status string
	if freshInit {
		status = "initialized"
		if isCollapsedLayout {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized Armature in collapsed layout at %s\n", issuesDir)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized Armature in dual-branch mode at %s\n", issuesDir)
		}
	} else {
		status = "already_initialized"
		if isCollapsedLayout {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Armature already initialized in collapsed layout at %s\n", issuesDir)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Armature already initialized in dual-branch mode at %s\n", issuesDir)
		}
	}

	result := RepoSetupResult{
		Status:       status,
		SkippedHooks: skippedHooks,
	}
	return result, nil
}
