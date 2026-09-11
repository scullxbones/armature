package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/output"
	"github.com/spf13/cobra"
)

type contextHistoryRow struct {
	SHA     string `json:"sha"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

func newContextHistoryCmd() *cobra.Command {
	var (
		chIssue string
		chLimit int
	)

	cmd := &cobra.Command{
		Use:   "context-history",
		Short: "Show commits where an issue's context changed",
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			opsRepoPath := appCtx.RepoPath
			if appCtx.WorktreePath != "" {
				opsRepoPath = appCtx.WorktreePath
			}

			gc := adapters.New(opsRepoPath)

			branch, err := gc.CurrentBranch()
			if err != nil {
				return fmt.Errorf("get current branch: %w", err)
			}

			entries, err := gc.LogBranch(branch, chLimit)
			if err != nil {
				return fmt.Errorf("log branch: %w", err)
			}

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
			// history spanning a dual-branch-to-collapsed migration replays
			// ops committed under either layout.
			opsPrefix := filepath.Join(issuesRel, "ops")
			legacyOpsPrefix := filepath.Join(".armature", "ops")

			// Reverse entries to walk oldest-first
			for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
				entries[i], entries[j] = entries[j], entries[i]
			}

			var changes []contextHistoryRow
			prevRendered := ""

			for _, entry := range entries {
				state, err := materialize.MaterializeAtSHA(gc, entry.SHA, opsPrefix, legacyOpsPrefix)
				if err != nil {
					// Skip commits where materialization fails (e.g. before .armature existed)
					continue
				}

				// Create an OSFileReader for file access
				reader := &context.OSFileReader{Root: appCtx.RepoPath}

				ctx, err := context.Assemble(chIssue, state, reader)
				if err != nil {
					// Issue doesn't exist at this commit — skip
					continue
				}

				rendered := context.RenderHuman(ctx)
				if rendered != prevRendered {
					changes = append(changes, contextHistoryRow{
						SHA:     entry.SHA,
						Date:    entry.Date,
						Subject: entry.Subject,
					})
					prevRendered = rendered
				}
			}

			if len(changes) == 0 {
				return fmt.Errorf("issue %q not found in any commit history", chIssue)
			}

			// Newest-first for both human and structured output.
			rows := make([]contextHistoryRow, 0, len(changes))
			for i := len(changes) - 1; i >= 0; i-- {
				rows = append(rows, changes[i])
			}

			if structuredFormat(cmd) {
				help := []string{"arm render-context --issue " + chIssue + " --at <sha> reconstructs context at a commit"}
				if cmd.Flags().Changed("limit") {
					help = []string{
						fmt.Sprintf("result is bounded by --limit %d; omit --limit to scan complete history", chLimit),
						help[0],
					}
				}
				env, err := output.NewEnvelope("commits", rows, help)
				if err != nil {
					return err
				}
				if cmd.Flags().Changed("limit") {
					if err := env.AddAdjunct("limit", chLimit); err != nil {
						return err
					}
				}
				return output.WriteEnvelope(cmd.OutOrStdout(), env)
			}

			for _, c := range rows {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s\n", c.SHA, c.Date, c.Subject)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&chIssue, "issue", "", "Issue ID (required)")
	cmd.Flags().IntVar(&chLimit, "limit", 0, "Maximum number of commits to scan (0 = complete history)")
	_ = cmd.MarkFlagRequired("issue")

	return cmd
}
