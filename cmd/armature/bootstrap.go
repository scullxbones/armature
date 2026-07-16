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
		// CreateOrphanBranch can fail after `git checkout --orphan _armature` if its
		// own restore-checkout also fails, leaving HEAD parked on the unborn
		// _armature branch. Resetting --hard in that state would point _armature at
		// the code branch's SHA, corrupting the ops branch (AGENTS.md I2/T2:
		// _armature history must never be rewritten). Refuse instead and point at
		// the backup dir for manual recovery.
		currentBranch, err := gitClient.CurrentBranch()
		if err != nil {
			return fmt.Errorf("determine current branch before rollback reset: %w (backup left at %s)", err, backupDir)
		}
		if currentBranch == "_armature" {
			return fmt.Errorf(
				"refusing to reset --hard while HEAD is on _armature (would corrupt ops branch history); "+
					"backup left at %s for manual recovery", backupDir,
			)
		}
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

// isPreB1SourcesDebrisPath reports whether path (a repo-relative path as
// reported by `git status --porcelain` inside an ops worktree) is exactly
// armature-owned sources state: <StateDirName>/sources/<name>, where name is
// "manifest.json" or ends in ".cache". Before the LNGHZN-B1 fix (commit
// 217022ea), `arm sources add/sync` wrote these files directly into the ops
// worktree without committing them (no FileCommitter wired), leaving
// pre-fix clones with permanently uncommitted debris under sources/. The fix
// itself was forward-only: it auto-commits future writes but never reconciles
// debris already on disk. migrateDualBranchToCollapsed uses this to recognize
// that debris and reconcile it instead of refusing to migrate forever.
func isPreB1SourcesDebrisPath(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) != 3 || parts[0] != config.StateDirName || parts[1] != "sources" {
		return false
	}
	name := parts[2]
	return name == "manifest.json" || strings.HasSuffix(name, ".cache")
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

	// Check if .arm worktree exists. A linked worktree's .git is a pointer
	// *file* (containing "gitdir: ..."), not a directory — a real nested repo
	// (e.g. a submodule or accidental nested clone) has .git as a directory
	// and must not be mistaken for the legacy layout and moved aside. Any
	// other stat error (missing, etc.) means there is no dual-branch layout
	// to migrate — leave diagnosing a malformed .arm to whatever step tries
	// to use it next (e.g. AddWorktree), not this check.
	gitInfo, err := os.Stat(filepath.Join(armWorktreePath, ".git"))
	if err != nil || gitInfo.IsDir() {
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

	// Check if the _armature worktree has uncommitted changes (reject if it does),
	// with one carve-out: pre-LNGHZN-B1 builds of `arm sources add/sync` left
	// .armature/sources/*.cache and manifest.json uncommitted in this worktree
	// (see isPreB1SourcesDebrisPath). If every dirty path is either that kind of
	// armature-owned sources debris, or an untracked file outside sources/ (the
	// same tolerance IsWorkingTreeDirty already grants elsewhere in this flow —
	// e.g. runRepoSetup's chained migration call runs after writing fresh,
	// not-yet-committed .gitignore/SCHEMA/hook-template scaffolding into this
	// same worktree, which must not itself block convergence), reconcile the
	// sources debris with a single commit and proceed. Any tracked (modified or
	// staged) dirty path outside sources/ still refuses exactly as before.
	armtreeGitClient := adapters.New(armWorktreePath)
	dirtyEntries, err := armtreeGitClient.DirtyEntries()
	if err != nil {
		return false, "", fmt.Errorf("check _armature worktree for uncommitted changes: %w", err)
	}
	var sourcesDebrisPaths []string
	for _, entry := range dirtyEntries {
		if isPreB1SourcesDebrisPath(entry.Path) {
			sourcesDebrisPaths = append(sourcesDebrisPaths, entry.Path)
			continue
		}
		if entry.Untracked {
			continue
		}
		return false, "", fmt.Errorf(
			"_armature worktree has uncommitted changes; please commit or stash them before running bootstrap",
		)
	}
	if len(sourcesDebrisPaths) > 0 {
		if err := armtreeGitClient.AddPaths(sourcesDebrisPaths); err != nil {
			return false, "", fmt.Errorf("stage pre-LNGHZN-B1 sources debris for reconciliation: %w", err)
		}
		if err := armtreeGitClient.CommitWithMessage("sources: reconcile pre-LNGHZN-B1 uncommitted sources state"); err != nil {
			return false, "", fmt.Errorf("commit pre-LNGHZN-B1 sources debris: %w", err)
		}
	}
	preCollapseSHA, err := armtreeGitClient.HeadSHA()
	if err != nil {
		return false, "", fmt.Errorf("snapshot _armature worktree before collapse: %w", err)
	}

	// Dual-branch layout detected. Snapshot a timestamped backup copy of .arm/
	// before mutating anything, purely for user-visible recovery: the live
	// worktree relocation below uses `git worktree move`, which keeps git's
	// registration correct atomically, so this backup is never needed for
	// rollback (rollback is just moving the worktree back to its old path).
	timestamp := time.Now().Format("20060102150405")
	backupDir := filepath.Join(repoPath, fmt.Sprintf(".arm.collapsed-%s", timestamp))
	for i := 2; ; i++ {
		if _, err := os.Lstat(backupDir); os.IsNotExist(err) {
			break
		}
		backupDir = filepath.Join(repoPath, fmt.Sprintf(".arm.collapsed-%s-%d", timestamp, i))
	}
	if _, err := copyRecursive(armWorktreePath, backupDir); err != nil {
		return false, "", fmt.Errorf("snapshot .arm worktree to backup %s: %w", backupDir, err)
	}

	newWorktreePath := filepath.Join(repoPath, config.StateDirName)

	// Move the worktree's directory and its git registration together
	// atomically. Unlike the manual rename + RemoveWorktree + AddWorktree dance
	// this replaces, `git worktree move` can never leave a partially-registered
	// worktree behind: if it fails, .arm is untouched; if it succeeds, .armature
	// is a fully valid worktree and .arm no longer exists at all (so there is
	// nothing to "re-register" on later rollback — moving back is sufficient).
	if err := gitClient.MoveWorktree(armWorktreePath, newWorktreePath); err != nil {
		return false, "", fmt.Errorf(
			"move .arm worktree to %s: %w (backup at %s, .arm untouched)", newWorktreePath, err, backupDir,
		)
	}

	// rollback restores the moved checkout before putting it back at .arm. The
	// flattening step can have staged removals and created root-level copies, so
	// moving the worktree alone would leave the recovered legacy checkout dirty.
	//
	// ResetHard restores tracked paths, but it does not remove untracked files
	// created by flattening. We must remove those copies. Do not simply remove
	// every candidate root path, though: a legacy worktree may already have had
	// valid root-level ops, templates, or config.json. The backup was captured
	// before any mutation, so use it to put precisely those original paths back.
	migrationRootPaths := []string{"ops", "templates", "hooks", "review", "sources", "config.json"}
	rollback := func(cause error) error {
		worktreeGitClient := adapters.New(newWorktreePath)
		if resetErr := worktreeGitClient.ResetHard(preCollapseSHA); resetErr != nil {
			return fmt.Errorf("%w; additionally, reset collapsed worktree failed: %w (backup at %s)", cause, resetErr, backupDir)
		}
		for _, path := range migrationRootPaths {
			if removeErr := os.RemoveAll(filepath.Join(newWorktreePath, path)); removeErr != nil {
				return fmt.Errorf("%w; additionally, remove flattened %s during rollback: %w (backup at %s)", cause, path, removeErr, backupDir)
			}
		}
		for _, path := range migrationRootPaths {
			snapshotPath := filepath.Join(backupDir, path)
			if _, statErr := os.Lstat(snapshotPath); statErr != nil {
				if os.IsNotExist(statErr) {
					continue
				}
				return fmt.Errorf("%w; additionally, inspect pre-migration %s during rollback: %w (backup at %s)", cause, path, statErr, backupDir)
			}
			if _, copyErr := copyRecursive(snapshotPath, filepath.Join(newWorktreePath, path)); copyErr != nil {
				return fmt.Errorf("%w; additionally, restore pre-migration %s during rollback: %w (backup at %s)", cause, path, copyErr, backupDir)
			}
		}
		if moveErr := gitClient.MoveWorktree(newWorktreePath, armWorktreePath); moveErr != nil {
			return fmt.Errorf(
				"%w; additionally, restore .arm worktree failed: %w (backup at %s)", cause, moveErr, backupDir,
			)
		}
		return fmt.Errorf("%w (migration rolled back; backup at %s)", cause, backupDir)
	}

	// The moved worktree's checkout of the _armature branch may still have a nested
	// .armature/ subtree (committed before the collapse, when everything lived under
	// .armature/.armature/...). Flatten it: copy its contents up to the worktree root,
	// then remove the now-stale nested copy and commit, mirroring the commit pattern the
	// legacy single-branch migration uses after copying its data (see the
	// AddPaths/CommitPathsNoVerify call in runRepoSetup after migrateLegacySingleBranchOps).
	// Tracks whether the flatten-and-commit block below has already committed to the
	// _armature branch. Once true, rollback must never reset --hard past that commit:
	// doing so would move the branch backward and drop already-visible history, violating
	// the append-only invariant (AGENTS.md I2/T2). Any failure after that point is reported
	// without rewriting history instead of routed through rollback.
	collapseCommitted := false

	legacyInnerArmaturePath := filepath.Join(newWorktreePath, config.StateDirName)
	if _, err := os.Stat(legacyInnerArmaturePath); err == nil {
		skippedCount, err := copyLegacyOpsToNewWorktree(legacyInnerArmaturePath, newWorktreePath)
		if err != nil {
			return false, "", rollback(fmt.Errorf("copy legacy ops to worktree root: %w", err))
		}
		_ = skippedCount // best-effort merge count; not currently surfaced for this migration path

		// config.json lives directly under the legacy inner .armature/, not one
		// of the legacyDirs subdirectories copyLegacyOpsToNewWorktree handles;
		// copy it too so migration doesn't silently reset the user's config.
		legacyConfigPath := filepath.Join(legacyInnerArmaturePath, "config.json")
		if _, err := os.Stat(legacyConfigPath); err == nil {
			if _, err := copyRecursive(legacyConfigPath, filepath.Join(newWorktreePath, "config.json")); err != nil {
				return false, "", rollback(fmt.Errorf("copy legacy config.json to worktree root: %w", err))
			}
		}

		// The nested .armature/ copy is now stale; remove it from both the index
		// and disk.
		worktreeGitClient := adapters.New(newWorktreePath)
		if err := worktreeGitClient.RemoveTree(config.StateDirName); err != nil {
			return false, "", rollback(fmt.Errorf("remove stale nested %s subtree: %w", config.StateDirName, err))
		}
		if err := os.RemoveAll(legacyInnerArmaturePath); err != nil {
			return false, "", rollback(fmt.Errorf("remove untracked stale nested %s subtree: %w", config.StateDirName, err))
		}

		// Stage whichever root-level directories/files actually exist after copying
		// (git add errors on a pathspec that matches nothing, so only stage what's there).
		var filesToStage []string
		for _, p := range migrationRootPaths {
			if _, err := os.Stat(filepath.Join(newWorktreePath, p)); err == nil {
				filesToStage = append(filesToStage, p)
			}
		}
		if len(filesToStage) > 0 {
			if err := worktreeGitClient.AddPaths(filesToStage); err != nil {
				return false, "", rollback(fmt.Errorf("stage collapsed layout files: %w", err))
			}
		}

		// Commit the removal of the stale nested subtree and the newly-staged root-level
		// files to the _armature branch, so the collapsed layout (and its full ops history)
		// is visible to every clone, not just the machine that ran the migration locally.
		if err := worktreeGitClient.CommitPathsNoVerify(
			"chore: collapse .arm/.armature dual-branch layout to single .armature worktree",
			".",
		); err != nil {
			return false, "", rollback(fmt.Errorf("commit collapsed layout: %w", err))
		}
		collapseCommitted = true
	}

	// Update git config to point to the new worktree path
	if err := gitClient.SetGitConfig("armature.ops-worktree-path", newWorktreePath); err != nil {
		if collapseCommitted {
			// The collapse commit already landed on the _armature branch; rolling back
			// via reset --hard here would rewind the branch and lose that commit, which
			// AGENTS.md I2 (append-only) forbids. The migration itself succeeded, so
			// leave the worktree at its new location and surface a manual remediation
			// instead of mutating history.
			return false, "", fmt.Errorf(
				"collapse migration committed successfully but set git config failed: %w; "+
					"run 'git config armature.ops-worktree-path %s' to complete migration (backup at %s)",
				err, newWorktreePath, backupDir,
			)
		}
		return false, "", rollback(fmt.Errorf("set git config: %w", err))
	}

	// armature.mode is dead legacy-compat state: nothing in the current codebase
	// reads it (see the "Set git config keys for current layout" comment in
	// runRepoSetup), but older builds wrote it as "dual-branch" and nothing ever
	// cleared it, so real pre-collapse repos can carry it forward indefinitely
	// even once they're on the collapsed layout. Best-effort clear it here;
	// failure is not fatal since the migration itself has already succeeded and
	// nothing depends on this key being absent.
	_ = gitClient.UnsetGitConfig("armature.mode") //nolint:errcheck // best-effort cleanup of dead legacy-compat state; migration already succeeded

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
		if strings.HasPrefix(name, ".armature.migrated-") || strings.HasPrefix(name, ".arm.collapsed-") {
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

// printCollapseMigrationBackupGuidance explains the .arm.collapsed-<timestamp> backup
// directory left behind by a successful dual-branch->collapsed migration: by design it is
// never needed for rollback (the git worktree move is atomic), so users are otherwise left
// with an unexplained directory and no indication of whether it's safe to remove.
func printCollapseMigrationBackupGuidance(cmd *cobra.Command, backupDir string) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(),
		"safety snapshot of the pre-migration ops worktree left at %s; "+
			"its contents are committed on the _armature branch — safe to delete once you've verified the collapsed layout\n",
		backupDir)
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

	// The built-in migration can relocate only the historical .arm worktree.
	// Refuse a configured custom legacy worktree rather than creating a new
	// collapsed worktree and silently leaving its nested ops history behind.
	// If the configured worktree is instead already a valid collapsed layout at a
	// custom path (e.g. armature.ops-worktree-path pointing at .ops), remember it so
	// the layout switch below reuses it instead of falling through to the .arm/.armature
	// defaults and trying to add a worktree where one is already checked out.
	var customCollapsedWorktreePath string
	if existingCtx, resolveErr := config.ResolveContext(repoPath); resolveErr == nil {
		base := filepath.Base(existingCtx.WorktreePath)
		if config.DetectUnmigratedLayout(existingCtx.WorktreePath, existingCtx.IssuesDir) {
			if base != ".arm" {
				return RepoSetupResult{}, fmt.Errorf(
					"repo uses a pre-collapse custom ops worktree at %s; automatic migration supports only .arm. "+
						"Move it to .arm or migrate it manually before running `arm bootstrap`",
					existingCtx.WorktreePath,
				)
			}
		} else if base != ".arm" && base != config.StateDirName {
			customCollapsedWorktreePath = existingCtx.WorktreePath
		}
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
		if migrated {
			if rbErr := rollbackLegacyMigration(repoPath, backupDir, preMigrationSHA, migrationCommitted); rbErr != nil {
				return RepoSetupResult{}, fmt.Errorf(
					"migrate dual-branch layout to collapsed: %w; additionally, rollback of legacy migration failed: %w (backup left at %s)",
					err, rbErr, backupDir,
				)
			}
			if migrationCommitted && preMigrationSHA != "" {
				// The reset restored tracked files, but the backup is the only copy of
				// any legacy files that were untracked at migration time.
				return RepoSetupResult{}, fmt.Errorf("migrate dual-branch layout to collapsed: %w (legacy migration rolled back; backup left at %s)", err, backupDir)
			}
		}
		return RepoSetupResult{}, fmt.Errorf("migrate dual-branch layout to collapsed: %w", err)
	}
	if dualMigrated {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated dual-branch .arm/.armature layout to collapsed .armature at timestamped backup %s\n", dualBackupDir)
		printCollapseMigrationBackupGuidance(cmd, dualBackupDir)
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
	case customCollapsedWorktreePath != "":
		worktreePath = customCollapsedWorktreePath
		isCollapsedLayout = true
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
			if customCollapsedWorktreePath != "" {
				// A custom collapsed worktree path (e.g. .ops) must be excluded by its
				// own basename, not the hardcoded .armature/ constant, or it shows up
				// as untracked. Only add an exclude entry when the worktree actually
				// lives under repoPath — a worktree outside the repo has nothing to
				// exclude here.
				if rel, relErr := filepath.Rel(repoPath, customCollapsedWorktreePath); relErr == nil && !strings.HasPrefix(rel, "..") {
					excludeName := filepath.Base(customCollapsedWorktreePath) + "/"
					if err := updateGitExclude(repoPath, excludeName, ""); err != nil {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: failed to exclude %s from git tracking: %v\n", excludeName, err)
					}
				}
			} else if err := updateGitExclude(repoPath, config.StateDirName+"/", ""); err != nil {
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
			isCollapsedLayout = true
			worktreePath = filepath.Join(repoPath, config.StateDirName)
			issuesDir = worktreePath // In collapsed layout, issuesDir == worktreePath
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated dual-branch .arm/.armature layout to collapsed .armature at timestamped backup %s\n", chainedDualBackupDir)
			printCollapseMigrationBackupGuidance(cmd, chainedDualBackupDir)
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
			// The .gitignore written earlier in this call targeted the old nested
			// location (.arm/.armature/.gitignore); the migration's copy list doesn't
			// include it, so the final collapsed worktree would otherwise have no
			// protection against state/ (per-worker derived data that must never be
			// committed) being swept up by a broad `git add .`. Re-write it here.
			if err := os.WriteFile(filepath.Join(issuesDir, ".gitignore"), []byte(issuesGitignore), 0o600); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to write .gitignore after migration: %v\n", err)
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
