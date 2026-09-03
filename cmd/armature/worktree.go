package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

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

			// Read all managed worktrees from git worktree list. Propagate a git
			// failure instead of proceeding on an empty inventory: an empty list
			// from a transient git error would mislabel every live claim a ghost
			// and make gc silently remove nothing.
			worktrees, err := worktree.ListManaged(ctx.RepoPath)
			if err != nil {
				return worktreeLifecycleError(
					"read managed worktrees", "git inventory unavailable",
					"verify --repo points at a git checkout, run `git worktree list --porcelain`, then retry `arm worktree list`", err)
			}
			registeredPaths, err := worktree.RegisteredPaths(ctx.RepoPath)
			if err != nil {
				return worktreeLifecycleError(
					"read registered worktree paths", "git inventory evidence is unavailable",
					"run `git worktree list --porcelain` and retry `arm worktree list`", err)
			}

			// Load current-truth issues via the snapshot store, exactly as
			// `arm list` does: it materializes from the op log against the
			// clone-local state dir (stateDirFor(ctx, workerID)), so we read the
			// same issues production read paths see. Reading raw JSON from
			// ctx.IssuesDir/issues pointed at a directory that never exists
			// (.armature/issues), yielding zero issues and a silent no-op.
			issues, err := loadIssuesForReconcile(ctx)
			if err != nil {
				return worktreeLifecycleError(
					"load claim state", "worktree inventory read succeeded but materialized claim state is unavailable",
					"run `arm materialize` (or repair the ops worktree) and retry `arm worktree list`", err)
			}

			// Reconcile, scoping ghost detection to worktrees this clone owns so a
			// live claim held by a remote clone (whose absolute WorktreePath can
			// never match this clone's git worktree list) is not a false ghost.
			result := worktree.ReconcileWithLocalEvidence(worktrees, issues, time.Now(), managedWorktreeRoots(ctx.RepoPath), registeredPaths)

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			if format == "json" || format == "agent" {
				jsonResult := map[string]interface{}{
					"bound":        result.BoundWorktrees,
					"orphans":      result.Orphans,
					"ghosts":       result.Ghosts,
					"gc_ready":     result.GCRemovalSet,
					"unrecognized": result.Unrecognized,
					"ambiguous":    result.GCAmbiguous,
				}
				if err := writeWorktreeJSON(cmd, jsonResult); err != nil {
					return err
				}
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
				if len(result.GCAmbiguous) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "AMBIGUOUS GC CANDIDATES (nothing removed):")
					for _, id := range result.GCAmbiguous {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
					}
				}

				if len(result.BoundWorktrees) == 0 && len(result.Orphans) == 0 &&
					len(result.Ghosts) == 0 && len(result.GCRemovalSet) == 0 &&
					len(result.Unrecognized) == 0 && len(result.GCAmbiguous) == 0 {
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

			// Read all managed worktrees; abort on a git failure rather than
			// proceeding on an empty inventory (see list command).
			worktrees, err := worktree.ListManaged(ctx.RepoPath)
			if err != nil {
				return worktreeLifecycleError(
					"read managed worktrees", "git inventory unavailable",
					"verify --repo points at a git checkout, run `git worktree list --porcelain`, then retry `arm worktree gc`", err)
			}
			registeredPaths, err := worktree.RegisteredPaths(ctx.RepoPath)
			if err != nil {
				return worktreeLifecycleError(
					"read registered worktree paths", "git inventory evidence is unavailable",
					"run `git worktree list --porcelain` and retry `arm worktree gc`", err)
			}

			// Load current-truth issues via the snapshot store (see list command).
			issues, err := loadIssuesForReconcile(ctx)
			if err != nil {
				return worktreeLifecycleError(
					"load claim state", "worktree inventory read succeeded but materialized claim state is unavailable",
					"run `arm materialize` (or repair the ops worktree) and retry `arm worktree gc`", err)
			}

			// Reconcile to find what should be removed, scoping ghost detection to
			// worktrees this clone owns (see list command for rationale).
			result := worktree.ReconcileWithLocalEvidence(worktrees, issues, time.Now(), managedWorktreeRoots(ctx.RepoPath), registeredPaths)

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			if dryRun {
				// Dry-run must report the same anomalous ambiguity that a real run
				// would refuse, so automation never treats this preview as all-clear.
				if format == "json" || format == "agent" {
					jsonResult := map[string]interface{}{
						"dry_run":            true,
						"would_remove":       result.GCRemovalSet,
						"would_remove_count": len(result.GCRemovalSet),
						"ambiguous":          result.GCAmbiguous,
						"ambiguous_count":    len(result.GCAmbiguous),
					}
					if err := writeWorktreeJSON(cmd, jsonResult); err != nil {
						return err
					}
				} else {
					if len(result.GCRemovalSet) > 0 {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would remove %d worktree(s):\n", len(result.GCRemovalSet))
						for _, id := range result.GCRemovalSet {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", id)
						}
					} else {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), "dry-run: no worktrees to remove")
					}
					if len(result.GCAmbiguous) > 0 {
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "AMBIGUOUS GC CANDIDATES (nothing would be removed):")
						for _, id := range result.GCAmbiguous {
							_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", id)
						}
					}
				}
				// gc already wrote its report (the JSON result, or the human
				// sections), so a nonzero dry-run exits on the report protocol
				// (ADR 0020 §6): handleRootError must not append a Command
				// Failure that would make --format=json stdout invalid JSON.
				return skipCommandFailure(gcExitError(nil, result.GCAmbiguous))
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

			for _, selected := range result.GCRemovals {
				issue, ok := issues[selected.Binding]
				if !ok {
					continue
				}

				outcome, err := removeWorktreeAtPathTracked(ctx.RepoPath, *issue, selected.Path, cmd.ErrOrStderr())
				switch {
				case err != nil:
					failed = append(failed, selected.Binding)
				case outcome == worktreeRemoved:
					removed = append(removed, selected.Binding)
				default:
					// worktreeSkipped: not found by branch, or binding mismatch.
					// Nothing was removed, so it must not be reported as removed.
					skipped = append(skipped, selected.Binding)
				}
			}

			// Report results
			if format == "json" || format == "agent" {
				jsonResult := map[string]interface{}{
					"removed":         removed,
					"removed_count":   len(removed),
					"skipped":         skipped,
					"skipped_count":   len(skipped),
					"failed":          failed,
					"failed_count":    len(failed),
					"ambiguous":       result.GCAmbiguous,
					"ambiguous_count": len(result.GCAmbiguous),
				}
				if err := writeWorktreeJSON(cmd, jsonResult); err != nil {
					return err
				}
				// Consistency with the human branch below: a removal failure or an
				// ambiguous terminal issue is a non-zero exit regardless of output
				// format, so gc never reports a misleading clean run.
				if exitErr := gcExitError(failed, result.GCAmbiguous); exitErr != nil {
					// Report already on the wire; see the dry-run branch above.
					return skipCommandFailure(exitErr)
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
				}
				// Surface ambiguous terminal issues that reconcile refused to GC.
				// list already renders these; gc must too, and must exit non-zero,
				// or an ambiguous candidate is silently dropped from a "clean" run.
				if len(result.GCAmbiguous) > 0 {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "AMBIGUOUS GC CANDIDATES (nothing removed):")
					for _, id := range result.GCAmbiguous {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", id)
					}
				}
				if exitErr := gcExitError(failed, result.GCAmbiguous); exitErr != nil {
					// Report already on the wire; see the dry-run branch above.
					return skipCommandFailure(exitErr)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview what would be removed without actually removing")

	return cmd
}

func managedWorktreeRoots(repoPath string) []string {
	abs := repoPath
	if resolved, err := filepath.Abs(repoPath); err == nil {
		abs = resolved
	}
	return []string{worktree.CanonicalRoot(repoPath), worktree.NormalizePath(abs)}
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

// gcExitError builds the non-zero exit for `arm worktree gc` from the two
// classes that must not be reported as a clean run: removal failures and
// ambiguous terminal issues reconcile refused to GC. Returns nil when both are
// empty. A removal failure takes precedence in the message.
func gcExitError(failed, ambiguous []string) error {
	if len(failed) > 0 {
		return fmt.Errorf("failed to remove %d worktree(s)", len(failed))
	}
	if len(ambiguous) > 0 {
		return fmt.Errorf("%d ambiguous GC candidate(s) not removed; resolve manually", len(ambiguous))
	}
	return nil
}

func worktreeLifecycleError(operation, state, next string, cause error) error {
	return fmt.Errorf("%s: %w\nstate: %s\nnext: %s", operation, cause, state, next)
}

func writeWorktreeJSON(cmd *cobra.Command, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worktree result: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(data)); err != nil {
		return fmt.Errorf("write worktree result: %w", err)
	}
	return nil
}
