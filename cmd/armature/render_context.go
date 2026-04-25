package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/git"
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

			issuesDir := appCtx.IssuesDir

			var state *materialize.State
			if rcAt != "" {
				// Time-travel: replay ops as they existed at the given commit SHA.
				opsRepoPath := appCtx.RepoPath
				if appCtx.Mode == "dual-branch" && appCtx.WorktreePath != "" {
					opsRepoPath = appCtx.WorktreePath
				}
				gc := git.New(opsRepoPath)
				opsPrefix := filepath.Join(".issues", "ops")
				var err error
				state, err = materialize.MaterializeAtSHA(gc, rcAt, opsPrefix)
				if err != nil {
					return fmt.Errorf("materialize at %s: %w", rcAt, err)
				}
			} else {
				_, err := materialize.Materialize(issuesDir, appCtx.StateDir, appCtx.Mode == "single-branch")
				if err != nil {
					return fmt.Errorf("materialize: %w", err)
				}
				state, err = loadStateFromStateDir(appCtx.StateDir)
				if err != nil {
					return fmt.Errorf("load state: %w", err)
				}
			}

			ctx, err := context.Assemble(rcIssue, appCtx.StateDir, state)
			if err != nil {
				return fmt.Errorf("assemble context: %w", err)
			}

			if !rcRaw {
				ctx = context.Truncate(ctx, rcBudget)
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				out, err := context.RenderAgent(ctx)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), out)
			} else {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), context.RenderHuman(ctx))
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

// loadStateFromStateDir reads all materialized issue JSON files and builds a State.
func loadStateFromStateDir(stateDir string) (*materialize.State, error) {
	stateIssuesDir := filepath.Join(stateDir, "issues")
	state := materialize.NewState()

	entries, err := os.ReadDir(stateIssuesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(stateIssuesDir, entry.Name())
		issue, err := materialize.LoadIssue(path)
		if err != nil {
			continue
		}
		issueCopy := issue
		state.Issues[issue.ID] = &issueCopy
	}

	return state, nil
}
