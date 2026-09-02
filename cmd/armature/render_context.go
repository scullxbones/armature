package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	ctxpkg "github.com/scullxbones/armature/internal/context"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/spf13/cobra"
)

func newRenderContextCmd() *cobra.Command {
	var (
		rcIssue  string
		rcBudget int
		rcRaw    bool
		rcAt     string
	)

	cmd := &cobra.Command{
		Use:   "render-context [issue-id]",
		Short: "Render assembled context for an issue",
		Args: func(cmd *cobra.Command, args []string) error {
			return mapRenderContextError(cobra.MaximumNArgs(1)(cmd, args))
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() { err = mapRenderContextError(err) }()
			rcIssue, err = resolveIssueID(rcIssue, args)
			if err != nil {
				return err
			}

			appCtx := currentCtx(cmd)
			// token_budget in config.json is the single source of --budget's
			// default; an explicit --budget always overrides it.
			if !cmd.Flags().Changed("budget") && appCtx.Config.TokenBudget > 0 {
				rcBudget = appCtx.Config.TokenBudget
			}
			var state *materialize.State
			if rcAt != "" {
				// Time-travel: replay ops as they existed at the given commit SHA.
				opsRepoPath := appCtx.RepoPath
				if appCtx.WorktreePath != "" {
					opsRepoPath = appCtx.WorktreePath
				}
				gc := adapters.New(opsRepoPath)
				// In the collapsed layout, IssuesDir == WorktreePath, so the ops
				// prefix relative to the worktree root is just "ops"; in the
				// legacy dual-branch layout, IssuesDir is nested a level down.
				issuesRel := "."
				if appCtx.IssuesDir != "" && opsRepoPath != "" {
					if rel, relErr := filepath.Rel(opsRepoPath, appCtx.IssuesDir); relErr == nil {
						issuesRel = rel
					}
				}
				// Include the legacy nested prefix alongside the current one so
				// a commit predating a dual-branch-to-collapsed migration
				// still replays its ops.
				opsPrefix := filepath.Join(issuesRel, "ops")
				legacyOpsPrefix := filepath.Join(".armature", "ops")
				var err error
				state, err = materialize.MaterializeAtSHA(gc, rcAt, opsPrefix, legacyOpsPrefix)
				if err != nil {
					return fmt.Errorf("materialize at %s: %w", rcAt, err)
				}
			} else {
				snap, snapErr := newSnapshotStore(appCtx).Load(context.Background())
				if snapErr != nil {
					return fmt.Errorf("load snapshot: %w", snapErr)
				}
				for _, w := range snap.Warnings {
					_, _ = fmt.Fprintf(os.Stderr, "warning: %s\n", w)
				}
				state = snap.State
			}

			// Create an OSFileReader for file access
			repoRoot := ctxpkg.InferRepoRoot(appCtx.StateDir)
			reader := &ctxpkg.OSFileReader{Root: repoRoot}

			ctx, err := ctxpkg.Assemble(rcIssue, state, reader)
			if err != nil {
				return fmt.Errorf("assemble context: %w", err)
			}

			if !rcRaw {
				ctx = ctxpkg.Truncate(ctx, rcBudget)
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				out, err := ctxpkg.RenderAgent(ctx)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), out)
			} else {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), ctxpkg.RenderHuman(ctx))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&rcIssue, "issue", "", "Issue ID")
	cmd.Flags().IntVar(&rcBudget, "budget", 4000, "Token budget")
	cmd.Flags().BoolVar(&rcRaw, "raw", false, "Skip truncation")
	cmd.Flags().StringVar(&rcAt, "at", "", "Replay context as of this git commit SHA")
	return cmd
}

const codeRenderContext1 = "RENDER-CONTEXT-1"

func init() {
	armerrors.Register(codeRenderContext1)
}

func mapRenderContextError(err error) error {
	if err == nil {
		return nil
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return cf
	}
	msg := err.Error()
	if strings.Contains(msg, "issue ID is required") || strings.Contains(msg, "accepts at most") {
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm render-context --help"}, 2, err)
	}
	if strings.Contains(msg, "materialize at ") {
		return armerrors.Wrap(codeRenderContext1, msg, []string{
			"arm render-context --issue <issue-id> --at <reachable-sha>",
			"arm render-context --issue <issue-id>",
		}, 1, err)
	}
	if strings.Contains(msg, "load snapshot") {
		return armerrors.Wrap(codeRenderContext1, msg, []string{"arm doctor"}, 1, err)
	}
	return armerrors.Wrap(codeRenderContext1, msg, []string{"arm list", "arm show"}, 1, err)
}
