package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	claimpkg "github.com/scullxbones/armature/internal/claim"
	ctxpkg "github.com/scullxbones/armature/internal/context"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/oporder"
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
			if !cmd.Flags().Changed("budget") && appCtx.Config.TokenBudget > 0 {
				rcBudget = appCtx.Config.TokenBudget
			}
			workerID, _, idErr := resolveWorkerAndLog(appCtx)
			if idErr != nil {
				return idErr
			}
			if located, locErr := locatePublishedOps(appCtx); locErr == nil {
				lease := oporder.OwnerOf(oporder.Published(located), rcIssue)
				if lease.Holder != "" && lease.Holder != workerID {
					return claimpkg.ErrNotClaimOwner
				}
			}
			var state *materialize.State
			if rcAt != "" {
				opsRepoPath := appCtx.RepoPath
				if appCtx.WorktreePath != "" {
					opsRepoPath = appCtx.WorktreePath
				}
				gc := adapters.New(opsRepoPath)
				opsPrefix, legacyOpsPrefix := opsHistoryPrefixes(appCtx, opsRepoPath)
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
				emitSnapWarnings(os.Stderr, snap.Warnings)
				state = snap.State
			}

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
	if strings.Contains(msg, "NOT-CLAIM-OWNER") {
		return armerrors.Wrap(codeRenderContext1, claimpkg.ErrNotClaimOwner.Error(), []string{
			"arm claim --worktree",
			"arm show",
		}, err)
	}
	if strings.Contains(msg, "issue ID is required") || strings.Contains(msg, "accepts at most") {
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm render-context --help"}, err)
	}
	if strings.Contains(msg, "materialize at ") {
		return armerrors.Wrap(codeRenderContext1, msg, []string{
			"arm render-context --issue <issue-id> --at <reachable-sha>",
			"arm render-context --issue <issue-id>",
		}, err)
	}
	if strings.Contains(msg, "load snapshot") {
		return armerrors.Wrap(codeRenderContext1, msg, []string{"arm doctor"}, err)
	}
	return armerrors.Wrap(codeRenderContext1, msg, []string{"arm list", "arm show"}, err)
}
