package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/config"
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

			// Load current-truth issues via the snapshot store, exactly as
			// `arm list` does: it materializes from the op log against the
			// clone-local state dir (stateDirFor(ctx, workerID)), so we read the
			// same issues production read paths see. Reading raw JSON from
			// ctx.IssuesDir/issues pointed at a directory that never exists
			// (.armature/issues), yielding zero issues and a silent no-op.
			issues, err := loadIssuesForReconcile(ctx)
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}

			// Reconcile, scoping ghost detection to worktrees this clone owns so a
			// live claim held by a remote clone (whose absolute WorktreePath can
			// never match this clone's git worktree list) is not a false ghost.
			result := worktree.Reconcile(worktrees, issues, managedWorktreeRoot(ctx.RepoPath))

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

				if len(result.GCRemovalSet) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "GC-READY (merged/cancelled with worktree):")
					for _, id := range result.GCRemovalSet {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
					}
				}

				if len(result.Unrecognized) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "UNRECOGNIZED (worktree maps to no known issue):")
					for _, path := range result.Unrecognized {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", path)
					}
				}

				if len(result.BoundWorktrees) == 0 && len(result.Orphans) == 0 &&
					len(result.Ghosts) == 0 && len(result.GCRemovalSet) == 0 &&
					len(result.Unrecognized) == 0 {
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

			// Load current-truth issues via the snapshot store (see list command).
			issues, err := loadIssuesForReconcile(ctx)
			if err != nil {
				return fmt.Errorf("load issues: %w", err)
			}

			// Reconcile to find what should be removed, scoping ghost detection to
			// worktrees this clone owns (see list command for rationale).
			result := worktree.Reconcile(worktrees, issues, managedWorktreeRoot(ctx.RepoPath))

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

			// Actually remove worktrees. Route each removal through the
			// binding-verified teardown path shared with `arm merged`
			// (removeWorktreeForIssue): it locates the worktree by branch in THIS
			// clone, verifies the clone-local armature-issue-id binding before
			// removing, and cleans up branch-point metadata. This replaces a naive
			// force-remove of issue.WorktreePath — a git-replicated absolute path
			// that may point at a reused or foreign worktree — collapsing the two
			// divergent teardown paths into one.
			removed := []string{}
			failed := []string{}
			skipped := []string{}

			for _, issueID := range result.GCRemovalSet {
				issue, ok := issues[issueID]
				if !ok {
					continue
				}

				outcome, err := removeWorktreeForIssueTracked(ctx.RepoPath, *issue, cmd.ErrOrStderr())
				switch {
				case err != nil:
					failed = append(failed, issueID)
				case outcome == worktreeRemoved:
					removed = append(removed, issueID)
				default:
					// worktreeSkipped: not found by branch, or binding mismatch.
					// Nothing was removed, so it must not be reported as removed.
					skipped = append(skipped, issueID)
				}
			}

			// Report results
			if format == "json" || format == "agent" {
				jsonResult := map[string]interface{}{
					"removed":       removed,
					"removed_count": len(removed),
					"skipped":       skipped,
					"skipped_count": len(skipped),
					"failed":        failed,
					"failed_count":  len(failed),
				}
				data, _ := json.MarshalIndent(jsonResult, "", "  ") //nolint:errcheck
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				// Consistency with the human branch below: a removal failure is a
				// non-zero exit regardless of output format.
				if len(failed) > 0 {
					return fmt.Errorf("failed to remove %d worktree(s)", len(failed))
				}
			} else {
				if len(removed) > 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed %d worktree(s):\n", len(removed))
					for _, id := range removed {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
					}
				}
				if len(skipped) > 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Skipped %d worktree(s) (not found or binding mismatch):\n", len(skipped))
					for _, id := range skipped {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", id)
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
	return strings.HasPrefix(worktree.NormalizePath(path), managedWorktreeRoot(repoPath))
}

// managedWorktreeRoot returns this clone's managed worktree directory as a
// normalized, trailing-separator prefix (<repoPath>/.worktrees/). Used both to
// classify managed worktrees and to scope Reconcile's ghost detection to
// locally-owned claims.
func managedWorktreeRoot(repoPath string) string {
	// Resolve repoPath to an absolute path first. In production ctx.RepoPath
	// defaults to "." (the cwd) when --repo is not passed; git worktree list
	// always emits absolute paths, so a relative prefix here would never match
	// and every managed worktree would be misclassified as not-managed,
	// silently emptying reconcile.
	abs := repoPath
	if resolved, err := filepath.Abs(repoPath); err == nil {
		abs = resolved
	}
	return worktree.NormalizePath(filepath.Join(abs, ".worktrees")) + string(os.PathSeparator)
}

// loadIssuesForReconcile loads current-truth issues the same way production read
// paths do: via the snapshot store, which materializes the op log against the
// clone-local state directory (stateDirFor(ctx, workerID), exposed as
// ctx.StateDir). This is the fix for the reconcile no-op — the previous code
// read raw JSON from ctx.IssuesDir/issues (<repo>/.armature/issues), a directory
// that never exists, so Reconcile always received zero issues.
func loadIssuesForReconcile(ctx *config.Context) (map[string]*materialize.Issue, error) {
	store := newSnapshotStore(ctx)
	snap, err := store.Load(context.Background())
	if err != nil {
		return nil, err
	}
	if snap.Issues == nil {
		return map[string]*materialize.Issue{}, nil
	}
	return snap.Issues, nil
}
