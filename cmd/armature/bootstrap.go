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
	"github.com/scullxbones/armature/internal/worker"
	"github.com/spf13/cobra"
)

// RepoSetupResult captures the outcome of repository initialization.
type RepoSetupResult struct {
	Status       string   `json:"status"`
	SkippedHooks []string `json:"skipped_hooks,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// BootstrapResult is the complete output of a bootstrap operation.
type BootstrapResult struct {
	RepoSetup    RepoSetupResult                   `json:"repo_setup"`
	HarnessSetup []bootstrap.HarnessArtifactResult `json:"harness_setup"`
}

func silenceHumanStdoutWhenStructured(cmd *cobra.Command, format string) *cobra.Command {
	if format == "json" || format == "agent" {
		silentCmd := &cobra.Command{}
		silentCmd.SetOut(io.Discard)
		return silentCmd
	}
	return cmd
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			applyTTYDetectionPolicy(cmd.Root())
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}

			absRepoPath, err := filepath.Abs(repoPath)
			if err != nil {
				return fmt.Errorf("resolve repo path: %w", err)
			}
			repoPath = absRepoPath

			format, _ := cmd.Root().PersistentFlags().GetString("format")

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

			repoSetupResult, err := runRepoSetup(silenceHumanStdoutWhenStructured(cmd, format), repoPath)
			if err != nil {
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
					return skipCommandFailure(fmt.Errorf("repo setup failed: %w", err))
				}
				return fmt.Errorf("repo setup failed: %w", err)
			}

			harnessResults, err := executeHarnessSetup(silenceHumanStdoutWhenStructured(cmd, format), plan, repoPath, global)
			if err != nil {
				if (format == "json" || format == "agent") && len(harnessResults) > 0 {
					result := BootstrapResult{
						RepoSetup:    repoSetupResult,
						HarnessSetup: harnessResults,
					}
					if data, merr := json.MarshalIndent(result, "", "  "); merr == nil {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
					}
					return skipCommandFailure(fmt.Errorf("harness setup failed: %w", err))
				}
				return fmt.Errorf("harness setup failed: %w", err)
			}

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

func recordArtifactAction(results *[]bootstrap.HarnessArtifactResult, platformName, artifactName string, action bootstrap.ActionKind) {
	switch action {
	case bootstrap.ActionInstall:
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

func executeHarnessSetup(cmd *cobra.Command, plan bootstrap.Plan, repoPath string, global bool) ([]bootstrap.HarnessArtifactResult, error) {
	var results []bootstrap.HarnessArtifactResult

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

	for _, row := range plan.Rows {
		platformName := string(row.Platform)

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

		if row.PluginMetadata == bootstrap.ActionInstall {
			pluginName, err := pluginNameFromSkillsFS(skillsembed.SkillsFS)
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
# Armature post-commit hook: delegate to native arm hook run.
# Git exports GIT_DIR / GIT_INDEX_FILE / … for this hook; nested git against
# the ops worktree must not inherit them (index would mis-resolve).
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR
arm hook run post-commit
`

const preCommitHookTemplate = `#!/bin/sh
# armature:managed
# Armature pre-commit hook: delegate to native arm hook run.
# Git exports GIT_DIR / GIT_INDEX_FILE / … for this hook; nested git against
# the ops worktree must not inherit them (index would mis-resolve).
# Fail-loud on code branches (I3) when arm is present. Skip _armature so
# ops commits do not require config.json. If arm is not on PATH, do not
# block the commit (test sandboxes and incomplete PATH); the Go path still
# prints the dual-branch remediation line when it refuses.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR
current_branch=$(git symbolic-ref --short HEAD 2>/dev/null)
if [ "$current_branch" = "_armature" ]; then
  exit 0
fi
command -v arm >/dev/null 2>&1 || exit 0
arm hook run pre-commit
`

func installHooks(repoPath string, issuesDir string) ([]string, error) {
	hooksDir := filepath.Join(issuesDir, "hooks")
	gitHooksDir := filepath.Join(repoPath, ".git", "hooks")

	if err := os.MkdirAll(gitHooksDir, 0o750); err != nil {
		return nil, fmt.Errorf("create .git/hooks directory: %w", err)
	}

	hooks := []string{"pre-commit", "post-commit", "post-merge"}
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

		if existing, readErr := os.ReadFile(hookPath); readErr == nil { //nolint:gosec // G304: internal hooks path
			if !isArmatureManagedHook(string(existing)) {
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

	if err := removeObsoleteHooks(gitHooksDir, hooksDir); err != nil {
		return skipped, err
	}

	return skipped, nil
}

func isArmatureManagedHook(content string) bool {
	if strings.Contains(content, "# armature:managed") {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(content), "#!/bin/sh") &&
		strings.Contains(content, "\n# Armature ")
}

var obsoleteHooks = []string{"prepare-commit-msg"}

func removeObsoleteHooks(gitHooksDir, hooksDir string) error {
	for _, hook := range obsoleteHooks {
		templatePath := filepath.Join(hooksDir, hook+".sh.template")
		if err := os.Remove(templatePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove obsolete hook template %s: %w", hook, err)
		}

		hookPath := filepath.Join(gitHooksDir, hook)
		existing, err := os.ReadFile(hookPath) //nolint:gosec // G304: internal hooks path
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("check obsolete hook %s: %w", hook, err)
		}
		if !isArmatureManagedHook(string(existing)) {
			continue
		}
		if err := os.Remove(hookPath); err != nil {
			return fmt.Errorf("remove obsolete hook %s: %w", hook, err)
		}
	}
	return nil
}

func commitOpsScaffolding(worktreePath string, isCollapsedLayout bool) error {
	client := adapters.New(worktreePath)
	prefix := ""
	if !isCollapsedLayout {
		prefix = config.StateDirName + "/"
	}
	paths := []string{
		prefix + ".gitignore",
		prefix + "ops/SCHEMA",
	}
	if err := untrackLocalOnlyPaths(client, prefix); err != nil {
		return err
	}
	if err := client.AddPaths(paths); err != nil {
		return fmt.Errorf("stage ops scaffolding: %w", err)
	}
	if err := client.CommitPathsNoVerify("chore: refresh ops scaffolding", paths...); err != nil {
		return fmt.Errorf("commit ops scaffolding: %w", err)
	}
	return nil
}

func untrackLocalOnlyPaths(client *adapters.Client, prefix string) error {
	sidecars := []string{prefix + "gates", prefix + "review", prefix + "hooks/*.sh.template"}
	tracked := false
	for _, sidecar := range sidecars {
		if client.IsTracked(sidecar) {
			tracked = true
			break
		}
	}
	staged, err := client.StagedPaths()
	if err != nil {
		return fmt.Errorf("inspect ops index before untracking sidecars: %w", err)
	}
	pending, unrelated := partitionLocalOnlyStaged(staged, prefix)
	if len(unrelated) > 0 && (tracked || len(pending) > 0) {
		return fmt.Errorf(
			"refusing to untrack local-only ops paths: unrelated staged changes in the ops worktree would be swept into the cleanup commit: %s",
			strings.Join(unrelated, ", "),
		)
	}
	if !tracked && len(pending) == 0 {
		return nil
	}

	removedPaths := append([]string(nil), pending...)
	if tracked {
		for _, sidecar := range sidecars {
			if !client.IsTracked(sidecar) {
				continue
			}
			if err := client.RemoveFromIndex(sidecar); err != nil {
				return restoreIndexKeeping(client, removedPaths, fmt.Errorf("untrack sidecar %s: %w", sidecar, err))
			}
		}
		stagedAfter, stagedErr := client.StagedPaths()
		if stagedErr != nil {
			return restoreIndexKeeping(client, removedPaths, fmt.Errorf("inspect ops index after untracking sidecars: %w", stagedErr))
		}
		removedPaths = stagedAfter
	}
	if len(removedPaths) == 0 {
		return nil
	}
	if err := client.CommitIndexNoVerify("chore: untrack local-only ops scaffolding"); err != nil {
		return restoreIndexKeeping(client, removedPaths, fmt.Errorf("commit local-only untracking: %w", err))
	}
	return nil
}

func restoreIndexKeeping(client *adapters.Client, paths []string, primary error) error {
	if restoreErr := client.RestoreIndexFromHEAD(paths); restoreErr != nil {
		return fmt.Errorf("%w (also restore staged removals: %v)", primary, restoreErr)
	}
	return primary
}

func partitionLocalOnlyStaged(staged []string, prefix string) (pending, unrelated []string) {
	for _, p := range staged {
		if isLocalOnlyUntrackPath(p, prefix) {
			pending = append(pending, p)
			continue
		}
		unrelated = append(unrelated, p)
	}
	return pending, unrelated
}

func isLocalOnlyUntrackPath(path, prefix string) bool {
	switch {
	case path == prefix+"gates" || strings.HasPrefix(path, prefix+"gates/"):
		return true
	case path == prefix+"review" || strings.HasPrefix(path, prefix+"review/"):
		return true
	case strings.HasPrefix(path, prefix+"hooks/") && strings.HasSuffix(path, ".sh.template"):
		return true
	default:
		return false
	}
}

func writeScaffoldingMonotonic(path, label string, content []byte, warn io.Writer) error {
	existing, err := os.ReadFile(path) //nolint:gosec // G304: path is derived from controlled repo paths
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if tracked, ok := ops.ParseScaffoldingVersion(string(existing)); ok && tracked > ops.ScaffoldingVersion {
		_, _ = fmt.Fprintf(warn,
			"Warning: %s was written by a newer arm (scaffolding version %d > %d); leaving it alone. Upgrade arm to regenerate it.\n",
			path, tracked, ops.ScaffoldingVersion)
		return nil
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", label, err)
	}
	return nil
}

func writeGitignoreMonotonic(gitignorePath string, warn io.Writer) error {
	return writeScaffoldingMonotonic(gitignorePath, ".gitignore", []byte(ops.GenerateOpsGitignore()), warn)
}

func writeSchemaMonotonic(schemaPath string, warn io.Writer) error {
	return writeScaffoldingMonotonic(schemaPath, "SCHEMA", []byte(ops.GenerateSchema()), warn)
}

func commitObsoleteHookTemplateRemovals(worktreePath string, isCollapsedLayout bool) error {
	client := adapters.New(worktreePath)
	prefix := "hooks/"
	if !isCollapsedLayout {
		prefix = config.StateDirName + "/hooks/"
	}
	var paths []string
	for _, hook := range obsoleteHooks {
		rel := prefix + hook + ".sh.template"
		if !client.IsTracked(rel) {
			continue
		}
		if err := client.RemoveTree(rel); err != nil {
			return fmt.Errorf("remove tracked obsolete hook template %s: %w", rel, err)
		}
		paths = append(paths, rel)
	}
	if len(paths) == 0 {
		return nil
	}
	if err := client.CommitPathsNoVerify("chore: remove obsolete hook templates", paths...); err != nil {
		return fmt.Errorf("commit obsolete hook template removal: %w", err)
	}
	return nil
}

func migrateLegacySingleBranchOps(repoPath string) (bool, string, string, bool, error) {
	legacyArmatureDir := filepath.Join(repoPath, config.StateDirName)
	legacyOpsDir := filepath.Join(legacyArmatureDir, "ops")
	preMigrationSHA := ""

	if gitMarker, err := os.Stat(filepath.Join(legacyArmatureDir, ".git")); err == nil && !gitMarker.IsDir() {
		return false, "", "", false, nil
	}

	info, err := os.Stat(legacyOpsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", "", false, nil
		}
		return false, "", preMigrationSHA, false, fmt.Errorf("check for legacy layout: %w", err)
	}

	if !info.IsDir() {
		return false, "", "", false, nil
	}

	entries, err := os.ReadDir(legacyOpsDir)
	if err != nil {
		return false, "", preMigrationSHA, false, fmt.Errorf("read legacy ops directory: %w", err)
	}

	if len(entries) == 0 {
		return false, "", "", false, nil
	}

	timestamp := time.Now().Format("20060102150405")
	backupDir := filepath.Join(repoPath, fmt.Sprintf(".armature.migrated-%s", timestamp))
	for i := 2; ; i++ {
		if _, err := os.Lstat(backupDir); os.IsNotExist(err) {
			break
		}
		backupDir = filepath.Join(repoPath, fmt.Sprintf(".armature.migrated-%s-%d", timestamp, i))
	}

	gitClient := adapters.New(repoPath)
	isTracked := gitClient.IsTracked(config.StateDirName)
	if isTracked {
		preMigrationSHA, err = gitClient.HeadSHA()
		if err != nil {
			return false, "", preMigrationSHA, false, fmt.Errorf("capture pre-migration HEAD: %w", err)
		}
	}

	if isTracked {
		swallowErr(gitClient.RemoveFromIndex(config.StateDirName))
	}

	if err := os.Rename(legacyArmatureDir, backupDir); err != nil {
		if isTracked {
			if addErr := gitClient.AddPaths([]string{config.StateDirName}); addErr != nil {
				return false, "", preMigrationSHA, false, fmt.Errorf("backup legacy .armature directory: %w; re-stage .armature after failed rename: %w", err, addErr)
			}
		}
		return false, "", preMigrationSHA, false, fmt.Errorf("backup legacy .armature directory: %w", err)
	}

	if isTracked {
		if err := gitClient.CommitPathsNoVerify("chore: migrate legacy .armature to dual-branch layout", config.StateDirName); err != nil {
			if restoreErr := os.Rename(backupDir, legacyArmatureDir); restoreErr != nil {
				return false, "", preMigrationSHA, false, fmt.Errorf(
					"commit legacy .armature removal: %w; restore .armature from backup %s: %w",
					err, backupDir, restoreErr,
				)
			}

			if restoreIndexErr := gitClient.AddPaths([]string{config.StateDirName}); restoreIndexErr != nil {
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

func rollbackLegacyMigration(repoPath, backupDir, preMigrationSHA string, committed bool) error {
	if committed {
		gitClient := adapters.New(repoPath)
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

func isPreB1SourcesDebrisPath(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) != 3 || parts[0] != config.StateDirName || parts[1] != "sources" {
		return false
	}
	name := parts[2]
	return name == "manifest.json" || strings.HasSuffix(name, ".cache")
}

func migrateDualBranchToCollapsed(repoPath string) (bool, string, error) {
	gitClient := adapters.New(repoPath)

	armWorktreePath := filepath.Join(repoPath, ".arm")
	innerArmaturePath := filepath.Join(armWorktreePath, config.StateDirName)

	gitInfo, err := os.Stat(filepath.Join(armWorktreePath, ".git"))
	if err != nil || gitInfo.IsDir() {
		return false, "", nil
	}

	if _, err := os.Stat(innerArmaturePath); err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("check for inner .armature/ directory: %w", err)
	}

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
		if entry.Untracked || entry.Ignored {
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

	if err := gitClient.MoveWorktree(armWorktreePath, newWorktreePath); err != nil {
		return false, "", fmt.Errorf(
			"move .arm worktree to %s: %w (backup at %s, .arm untouched)", newWorktreePath, err, backupDir,
		)
	}

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

	collapseCommitted := false

	legacyInnerArmaturePath := filepath.Join(newWorktreePath, config.StateDirName)
	if _, err := os.Stat(legacyInnerArmaturePath); err == nil {
		skippedCount, err := copyLegacyOpsToNewWorktree(legacyInnerArmaturePath, newWorktreePath)
		if err != nil {
			return false, "", rollback(fmt.Errorf("copy legacy ops to worktree root: %w", err))
		}
		_ = skippedCount

		legacyConfigPath := filepath.Join(legacyInnerArmaturePath, "config.json")
		if _, err := os.Stat(legacyConfigPath); err == nil {
			if _, err := copyRecursive(legacyConfigPath, filepath.Join(newWorktreePath, "config.json")); err != nil {
				return false, "", rollback(fmt.Errorf("copy legacy config.json to worktree root: %w", err))
			}
		}

		worktreeGitClient := adapters.New(newWorktreePath)
		if err := worktreeGitClient.RemoveTree(config.StateDirName); err != nil {
			return false, "", rollback(fmt.Errorf("remove stale nested %s subtree: %w", config.StateDirName, err))
		}
		if err := os.RemoveAll(legacyInnerArmaturePath); err != nil {
			return false, "", rollback(fmt.Errorf("remove untracked stale nested %s subtree: %w", config.StateDirName, err))
		}

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

		if err := worktreeGitClient.CommitPathsNoVerify(
			"chore: collapse .arm/.armature dual-branch layout to single .armature worktree",
			".",
		); err != nil {
			return false, "", rollback(fmt.Errorf("commit collapsed layout: %w", err))
		}
		collapseCommitted = true
	}

	if err := gitClient.SetGitConfig("armature.ops-worktree-path", newWorktreePath); err != nil {
		if collapseCommitted {
			return false, "", fmt.Errorf(
				"collapse migration committed successfully but set git config failed: %w; "+
					"run 'git config armature.ops-worktree-path %s' to complete migration (backup at %s)",
				err, newWorktreePath, backupDir,
			)
		}
		return false, "", rollback(fmt.Errorf("set git config: %w", err))
	}

	swallowErr(gitClient.UnsetGitConfig("armature.mode"))

	return true, backupDir, nil
}

func copyLegacyOpsToNewWorktree(backupDir string, newIssuesDir string) (int, error) {
	legacyDirs := []string{"ops", "templates", "hooks", "review", "sources"}

	skippedCount := 0
	for _, dirName := range legacyDirs {
		legacyDir := filepath.Join(backupDir, dirName)
		entries, err := os.ReadDir(legacyDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
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

func copyRecursive(src string, dst string) (int, error) {
	info, err := os.Stat(src)
	if err != nil {
		return 0, fmt.Errorf("stat source: %w", err)
	}

	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode()); err != nil {
			return 0, fmt.Errorf("create directory: %w", err)
		}

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

	if _, err := os.Stat(dst); err == nil {
		return 1, nil
	} else if !os.IsNotExist(err) {
		return 0, fmt.Errorf("stat destination: %w", err)
	}

	content, err := os.ReadFile(src) //nolint:gosec // G304: src is constructed from legacyOpsDir
	if err != nil {
		return 0, fmt.Errorf("read file: %w", err)
	}

	if err := os.WriteFile(dst, content, info.Mode()); err != nil { //nolint:gosec // dst is constructed from newOpsDir
		return 0, fmt.Errorf("write file: %w", err)
	}

	return 0, nil
}

func excludeArmWorktreeFromGit(repoPath string) error {
	return updateGitExclude(repoPath, ".arm/", "")
}

func printCollapseMigrationBackupGuidance(cmd *cobra.Command, backupDir string) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(),
		"safety snapshot of the pre-migration ops worktree left at %s; "+
			"its contents are committed on the _armature branch — safe to delete once you've verified the collapsed layout\n",
		backupDir)
}

func updateGitExclude(repoPath string, addPattern, removePattern string) error {
	_, err := updateGitExcludeTracked(repoPath, addPattern, removePattern)
	return err
}

func updateGitExcludeTracked(repoPath string, addPattern, removePattern string) (bool, error) {
	flock, err := acquireBlockingGitExcludeFlock(repoPath)
	if err != nil {
		return false, err
	}
	defer flock.Release()
	return updateGitExcludeTrackedLocked(repoPath, addPattern, removePattern)
}

func updateGitExcludeTrackedLocked(repoPath, addPattern, removePattern string) (bool, error) {
	gitDir, err := resolveCommonGitDir(repoPath)
	if err != nil {
		return false, err
	}
	excludePath := filepath.Join(gitDir, "info", "exclude")

	infoDir := filepath.Dir(excludePath)
	if err := os.MkdirAll(infoDir, 0o750); err != nil {
		return false, fmt.Errorf("create .git/info directory: %w", err)
	}

	var currentContent string
	if data, err := os.ReadFile(excludePath); err == nil { //nolint:gosec // G304: path is constructed from repo/.git/info/exclude
		currentContent = string(data)
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("read .git/info/exclude: %w", err)
	}

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

	found := addPattern == ""
	if addPattern != "" {
		for line := range strings.SplitSeq(newContent, "\n") {
			if strings.TrimSpace(line) == addPattern {
				found = true
				break
			}
		}
	}

	added := !found
	if added {
		if len(newContent) > 0 && !strings.HasSuffix(newContent, "\n") {
			newContent += "\n"
		}
		newContent += addPattern + "\n"
	}

	if err := os.WriteFile(excludePath, []byte(newContent), 0o600); err != nil { //nolint:gosec // G703: path is constructed from repo/.git/info/exclude
		return false, fmt.Errorf("write .git/info/exclude: %w", err)
	}

	return added, nil
}

func runRepoSetup(cmd *cobra.Command, repoPath string) (RepoSetupResult, error) {
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("resolve repo path: %w", err)
	}
	repoPath = absRepoPath

	gitClient := adapters.New(repoPath)

	if branch, err := gitClient.CurrentBranch(); err == nil && branch == "_armature" {
		return RepoSetupResult{}, fmt.Errorf(
			"refusing to bootstrap a checkout of the _armature ops branch (path %s): run bootstrap from the main repository instead", repoPath,
		)
	}

	dirty, err := gitClient.IsWorkingTreeDirty()
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("check working tree: %w", err)
	}
	if dirty {
		return RepoSetupResult{}, fmt.Errorf(
			"working tree is dirty (contains uncommitted changes): please commit or stash your changes before running bootstrap",
		)
	}

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

	migrated, backupDir, preMigrationSHA, migrationCommitted, err := migrateLegacySingleBranchOps(repoPath)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("migrate legacy single-branch layout: %w", err)
	}
	if migrated {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated legacy single-branch .armature layout to timestamped backup at %s\n", backupDir)
	}

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
				return RepoSetupResult{}, fmt.Errorf("migrate dual-branch layout to collapsed: %w (legacy migration rolled back; backup left at %s)", err, backupDir)
			}
		}
		return RepoSetupResult{}, fmt.Errorf("migrate dual-branch layout to collapsed: %w", err)
	}
	if dualMigrated {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated dual-branch .arm/.armature layout to collapsed .armature at timestamped backup %s\n", dualBackupDir)
		printCollapseMigrationBackupGuidance(cmd, dualBackupDir)
		if err := updateGitExclude(repoPath, config.StateDirName+"/", ".arm/"); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update .git/info/exclude after migration: %v\n", err)
		}
	}

	if err := gitClient.CreateOrphanBranch("_armature"); err != nil {
		if migrated {
			if rbErr := rollbackLegacyMigration(repoPath, backupDir, preMigrationSHA, migrationCommitted); rbErr != nil {
				return RepoSetupResult{}, fmt.Errorf(
					"create _armature branch: %w; additionally, rollback of legacy migration failed: %w (backup left at %s)",
					err, rbErr, backupDir,
				)
			}
			if migrationCommitted && preMigrationSHA != "" {
				return RepoSetupResult{}, fmt.Errorf("create _armature branch: %w (migration rolled back; backup left at %s)", err, backupDir)
			}
		}
		return RepoSetupResult{}, fmt.Errorf("create _armature branch: %w", err)
	}

	var worktreePath string
	var isCollapsedLayout bool

	alreadyCollapsed := false
	if gitMarker, statErr := os.Stat(filepath.Join(repoPath, config.StateDirName, ".git")); statErr == nil && !gitMarker.IsDir() {
		alreadyCollapsed = true
	}

	hasPreExistingArmWorktree := false
	if gitMarker, statErr := os.Stat(filepath.Join(repoPath, ".arm", ".git")); statErr == nil && !gitMarker.IsDir() {
		hasPreExistingArmWorktree = true
	}

	switch {
	case customCollapsedWorktreePath != "":
		worktreePath = customCollapsedWorktreePath
		isCollapsedLayout = true
	case dualMigrated, alreadyCollapsed:
		worktreePath = filepath.Join(repoPath, config.StateDirName)
		isCollapsedLayout = true
	case hasPreExistingArmWorktree:
		worktreePath = filepath.Join(repoPath, ".arm")
		isCollapsedLayout = false
	default:
		worktreePath = filepath.Join(repoPath, config.StateDirName)
		isCollapsedLayout = true
	}

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
				return RepoSetupResult{}, fmt.Errorf("add %s worktree: %w (migration rolled back; backup left at %s)", worktreeLabel, err, backupDir)
			}
		}
		return RepoSetupResult{}, fmt.Errorf("add %s worktree: %w", worktreeLabel, err)
	}

	if !dualMigrated {
		if isCollapsedLayout {
			if customCollapsedWorktreePath != "" {
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

	if err := updateGitExclude(repoPath, ".worktrees/", ""); err != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: failed to exclude .worktrees/ from git tracking: %v\n", err)
	}

	if err := gitClient.SetGitConfig("armature.ops-worktree-path", worktreePath); err != nil {
		return RepoSetupResult{}, fmt.Errorf("set armature.ops-worktree-path: %w", err)
	}

	var issuesDir string
	if isCollapsedLayout {
		issuesDir = worktreePath
	} else {
		issuesDir = filepath.Join(worktreePath, config.StateDirName)
	}

	opsDir := filepath.Join(issuesDir, "ops")
	freshInit := true
	if entries, err := os.ReadDir(opsDir); err == nil && len(entries) > 0 {
		freshInit = false
	}

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

	configPath := filepath.Join(issuesDir, "config.json")
	var configToWrite *config.Config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		var cfg config.Config

		if migrated && backupDir != "" {
			legacyConfigPath := filepath.Join(backupDir, "config.json")
			if legacyConfig, err := config.LoadConfig(legacyConfigPath); err == nil {
				cfg = legacyConfig
			} else {
				if !errors.Is(err, os.ErrNotExist) {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"Warning: legacy config.json could not be loaded (%v); using default config (original preserved at %s)\n",
						err, legacyConfigPath)
				}
				projectType := config.DetectProjectType(repoPath)
				cfg = config.DefaultConfig(projectType)
			}
		} else {
			projectType := config.DetectProjectType(repoPath)
			cfg = config.DefaultConfig(projectType)
		}

		configToWrite = &cfg
	}

	if migrated && backupDir != "" {
		skippedCount, err := copyLegacyOpsToNewWorktree(backupDir, issuesDir)
		if err != nil {
			return RepoSetupResult{}, fmt.Errorf("copy legacy ops data (preserved at %s): %w", backupDir, err)
		}
		if skippedCount > 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%d legacy file(s) already present in new worktree, not overwritten\n", skippedCount)
		}

		if configToWrite != nil {
			if err := config.WriteConfig(configPath, *configToWrite); err != nil {
				return RepoSetupResult{}, fmt.Errorf("write config: %w", err)
			}
		}

		worktreeGitClient := adapters.New(worktreePath)

		stagePrefix := ""
		commitScope := "."
		if !isCollapsedLayout {
			stagePrefix = config.StateDirName + "/"
			commitScope = config.StateDirName
		}

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

		if err := worktreeGitClient.CommitPathsNoVerify(
			"chore: commit migrated legacy ops and config from single-branch layout",
			commitScope,
		); err != nil {
			return RepoSetupResult{}, fmt.Errorf("commit migrated data to _armature branch (legacy data preserved at %s): %w", backupDir, err)
		}
	}

	gitignorePath := filepath.Join(issuesDir, ".gitignore")
	if err := writeGitignoreMonotonic(gitignorePath, cmd.ErrOrStderr()); err != nil {
		return RepoSetupResult{}, fmt.Errorf("write %s/.gitignore: %w", config.StateDirName, err)
	}

	schemaPath := filepath.Join(issuesDir, "ops", "SCHEMA")
	if err := writeSchemaMonotonic(schemaPath, cmd.ErrOrStderr()); err != nil {
		return RepoSetupResult{}, err
	}

	if backups := listMigrationBackups(repoPath); len(backups) > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Note: stranded migration backups remain: %s\n", strings.Join(backups, ", "))
	}

	hookTemplates := map[string]string{
		"post-merge.sh.template":  postMergeHookTemplate,
		"post-commit.sh.template": postCommitHookTemplate,
		"pre-commit.sh.template":  preCommitHookTemplate,
	}

	for hookName, hookContent := range hookTemplates {
		hookTemplatePath := filepath.Join(issuesDir, "hooks", hookName)
		if err := os.WriteFile(hookTemplatePath, []byte(hookContent), 0o600); err != nil {
			return RepoSetupResult{}, fmt.Errorf("write hook template %s: %w", hookName, err)
		}
	}

	skippedHooks, err := installHooks(repoPath, issuesDir)
	if err != nil {
		return RepoSetupResult{}, fmt.Errorf("install hooks: %w", err)
	}

	for _, hookName := range skippedHooks {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: skipping git hook %s (not Armature-managed)\n", hookName)
	}

	if err := commitObsoleteHookTemplateRemovals(worktreePath, isCollapsedLayout); err != nil {
		return RepoSetupResult{}, err
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		var cfg config.Config

		projectType := config.DetectProjectType(repoPath)
		cfg = config.DefaultConfig(projectType)

		if err := config.WriteConfig(configPath, cfg); err != nil {
			return RepoSetupResult{}, fmt.Errorf("write config: %w", err)
		}

		worktreeGitClient := adapters.New(worktreePath)

		configRelPath := "config.json"
		commitScope := "."
		if !isCollapsedLayout {
			configRelPath = config.StateDirName + "/config.json"
			commitScope = config.StateDirName
		}

		if err := worktreeGitClient.AddPaths([]string{configRelPath}); err != nil {
			return RepoSetupResult{}, fmt.Errorf("stage config: %w", err)
		}

		if err := worktreeGitClient.CommitPathsNoVerify(
			"chore: init armature config",
			commitScope,
		); err != nil {
			return RepoSetupResult{}, fmt.Errorf("commit config to _armature branch: %w", err)
		}
	}

	if ok, _ := worker.CheckWorkerID(repoPath); !ok {
		if _, err := worker.InitWorker(repoPath); err != nil {
			return RepoSetupResult{}, fmt.Errorf("init worker: %w", err)
		}
	}

	if !dualMigrated && !isCollapsedLayout {
		chainedDualMigrated, chainedDualBackupDir, err := migrateDualBranchToCollapsed(repoPath)
		if err != nil {
			return RepoSetupResult{}, fmt.Errorf("chain dual-branch to collapsed migration after legacy migration: %w", err)
		}
		if chainedDualMigrated {
			isCollapsedLayout = true
			worktreePath = filepath.Join(repoPath, config.StateDirName)
			issuesDir = worktreePath
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated dual-branch .arm/.armature layout to collapsed .armature at timestamped backup %s\n", chainedDualBackupDir)
			printCollapseMigrationBackupGuidance(cmd, chainedDualBackupDir)
			if err := gitClient.SetGitConfig("armature.ops-worktree-path", worktreePath); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update git config after migration: %v\n", err)
			}
			if err := updateGitExclude(repoPath, config.StateDirName+"/", ".arm/"); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update .git/info/exclude after migration: %v\n", err)
			}
			stateDir := filepath.Join(issuesDir, "state")
			if err := os.MkdirAll(filepath.Join(stateDir, "issues"), 0o750); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to recreate state directories after migration: %v\n", err)
			}
			if err := writeGitignoreMonotonic(filepath.Join(issuesDir, ".gitignore"), cmd.ErrOrStderr()); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to write .gitignore after migration: %v\n", err)
			}
		}
	}

	if err := commitOpsScaffolding(worktreePath, isCollapsedLayout); err != nil {
		return RepoSetupResult{}, err
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
