package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	ctxpkg "github.com/scullxbones/armature/internal/context"
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
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if rcIssue == "" && len(args) > 0 {
				rcIssue = args[0]
			}
			if rcIssue == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			var state *materialize.State
			if rcAt != "" {
				// Time-travel: replay ops as they existed at the given commit SHA.
				opsRepoPath := appCtx.RepoPath
				if appCtx.WorktreePath != "" {
					opsRepoPath = appCtx.WorktreePath
				}
				gc := adapters.New(opsRepoPath)
				opsPrefix := filepath.Join(".armature", "ops")
				var err error
				state, err = materialize.MaterializeAtSHA(gc, rcAt, opsPrefix)
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
