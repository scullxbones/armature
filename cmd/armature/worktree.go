package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/worktree"
	"github.com/spf13/cobra"
)

func newWorktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage managed worktrees and reconcile against claim state",
	}

	cmd.AddCommand(newWorktreeListCmd())
	cmd.AddCommand(newWorktreeGCCmd())

	return cmd
}

func newWorktreeListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed worktrees with status (bound/orphan/ghost)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)

			// Read all managed worktrees from git worktree list
			worktrees := readManagedWorktrees(ctx.RepoPath)

			// Load all issues from materialized state
			issues, err := materialize.LoadAllIssues(filepath.Join(ctx.IssuesDir, "issues"))
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}

			// Reconcile
			result := worktree.Reconcile(worktrees, issues)

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			if format == "json" || format == "agent" {
				jsonResult := map[string]interface{}{
					"bound":        result.BoundWorktrees,
					"orphans":      result.Orphans,
					"ghosts":       result.Ghosts,
					"gc_ready":     result.GCRemovalSet,
					"unrecognized": result.Unrecognized,
				}
				data, _ := json.MarshalIndent(jsonResult, "", "  ") //nolint:errcheck
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				// Human format
				if len(result.BoundWorktrees) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "BOUND WORKTREES:")
					for _, id := range result.BoundWorktrees {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
					}
				}

				if len(result.Orphans) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "ORPHANS (no live claim):")
					for _, id := range result.Orphans {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
					}
				}

				if len(result.Ghosts) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "GHOSTS (recorded but missing on disk):")
					for _, id := range result.Ghosts {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
					}
				}

				if len(result.BoundWorktrees) == 0 && len(result.Orphans) == 0 && len(result.Ghosts) == 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No worktrees found or all are in order.")
				}
			}

			return nil
		},
	}

	return cmd
}

func newWorktreeGCCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Remove worktrees for merged/cancelled issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := currentCtx(cmd)

			// Read all managed worktrees
			worktrees := readManagedWorktrees(ctx.RepoPath)

			// Load all issues
			issues, err := materialize.LoadAllIssues(filepath.Join(ctx.IssuesDir, "issues"))
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}

			// Reconcile to find what should be removed
			result := worktree.Reconcile(worktrees, issues)

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			if dryRun {
				// Dry run: just report what would be removed
				if format == "json" || format == "agent" {
					jsonResult := map[string]interface{}{
						"dry_run":            true,
						"would_remove":       result.GCRemovalSet,
						"would_remove_count": len(result.GCRemovalSet),
					}
					data, _ := json.MarshalIndent(jsonResult, "", "  ") //nolint:errcheck
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				} else {
					if len(result.GCRemovalSet) > 0 {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would remove %d worktree(s):\n", len(result.GCRemovalSet))
						for _, id := range result.GCRemovalSet {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
						}
					} else {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), "dry-run: no worktrees to remove")
					}
				}
				return nil
			}

			// Actually remove worktrees
			gc := adapters.New(ctx.RepoPath)
			removed := []string{}
			failed := []string{}

			for _, issueID := range result.GCRemovalSet {
				issue, ok := issues[issueID]
				if !ok || issue.WorktreePath == "" {
					continue
				}

				if err := gc.RemoveWorktree(issue.WorktreePath); err != nil {
					failed = append(failed, issueID)
				} else {
					removed = append(removed, issueID)
				}
			}

			// Report results
			if format == "json" || format == "agent" {
				jsonResult := map[string]interface{}{
					"removed":       removed,
					"removed_count": len(removed),
					"failed":        failed,
					"failed_count":  len(failed),
				}
				data, _ := json.MarshalIndent(jsonResult, "", "  ") //nolint:errcheck
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				if len(removed) > 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed %d worktree(s):\n", len(removed))
					for _, id := range removed {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
					}
				}
				if len(failed) > 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Failed to remove %d worktree(s):\n", len(failed))
					for _, id := range failed {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", id)
					}
					return fmt.Errorf("failed to remove %d worktree(s)", len(failed))
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview what would be removed without actually removing")

	return cmd
}

// readManagedWorktrees reads all managed worktrees from git worktree list --porcelain.
// Filters to only worktrees under .worktrees/ directory.
func readManagedWorktrees(repoPath string) []worktree.Meta {
	// #nosec G204 - git binary and arguments are controlled by us, not user input
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		// No worktrees or git command failed; return empty list
		return []worktree.Meta{}
	}

	var worktrees []worktree.Meta
	lines := strings.Split(string(output), "\n")

	var currentPath string
	var currentBranch string

	for _, line := range lines {
		if line == "" {
			continue
		}

		if rest, ok := strings.CutPrefix(line, "worktree "); ok {
			currentPath = rest
		} else if rest, ok := strings.CutPrefix(line, "branch "); ok {
			currentBranch = rest
			// Once we have both path and branch, check if this is a managed worktree
			if isManaged(repoPath, currentPath) {
				worktrees = append(worktrees, worktree.Meta{
					Path:   currentPath,
					Branch: currentBranch,
				})
			}
		} else if rest, ok := strings.CutPrefix(line, "detached"); ok {
			// Detached worktrees under .worktrees/ are also managed
			_ = rest
			if isManaged(repoPath, currentPath) {
				worktrees = append(worktrees, worktree.Meta{
					Path:   currentPath,
					Branch: "detached",
				})
			}
		}
	}

	return worktrees
}

// isManaged reports whether path is a managed worktree: it must live directly
// under this repo's <repoPath>/.worktrees/ directory. A substring test on
// ".worktrees" would misclassify unrelated paths (e.g. a sibling repo that
// merely contains the string) as managed, making them gc-removal candidates.
// Both sides are symlink-normalized so a symlinked repo root still matches.
func isManaged(repoPath, path string) bool {
	managedRoot := worktree.NormalizePath(filepath.Join(repoPath, ".worktrees")) + string(os.PathSeparator)
	return strings.HasPrefix(worktree.NormalizePath(path), managedRoot)
}
