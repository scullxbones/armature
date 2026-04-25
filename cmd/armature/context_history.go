package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/git"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/spf13/cobra"
)

func newContextHistoryCmd() *cobra.Command {
	var (
		chIssue string
		chLimit int
	)

	cmd := &cobra.Command{
		Use:   "context-history",
		Short: "Show commits where an issue's context changed",
		RunE: func(cmd *cobra.Command, args []string) error {
			opsRepoPath := appCtx.RepoPath
			if appCtx.Mode == "dual-branch" && appCtx.WorktreePath != "" {
				opsRepoPath = appCtx.WorktreePath
			}

			gc := git.New(opsRepoPath)

			branch, err := gc.CurrentBranch()
			if err != nil {
				return fmt.Errorf("get current branch: %w", err)
			}

			entries, err := gc.LogBranch(branch, chLimit)
			if err != nil {
				return fmt.Errorf("log branch: %w", err)
			}

			opsPrefix := filepath.Join(".armature", "ops")

			// Reverse entries to walk oldest-first
			for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
				entries[i], entries[j] = entries[j], entries[i]
			}

			type changeEntry struct {
				sha     string
				date    string
				subject string
			}

			var changes []changeEntry
			prevRendered := ""

			for _, entry := range entries {
				state, err := materialize.MaterializeAtSHA(gc, entry.SHA, opsPrefix)
				if err != nil {
					// Skip commits where materialization fails (e.g. before .armature existed)
					continue
				}

				ctx, err := context.Assemble(chIssue, appCtx.IssuesDir, state)
				if err != nil {
					// Issue doesn't exist at this commit — skip
					continue
				}

				rendered := context.RenderHuman(ctx)
				if rendered != prevRendered {
					changes = append(changes, changeEntry{
						sha:     entry.SHA,
						date:    entry.Date,
						subject: entry.Subject,
					})
					prevRendered = rendered
				}
			}

			if len(changes) == 0 {
				return fmt.Errorf("issue %q not found in any commit history", chIssue)
			}

			// Output newest-first
			for i := len(changes) - 1; i >= 0; i-- {
				c := changes[i]
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s\n", c.sha, c.date, c.subject)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&chIssue, "issue", "", "Issue ID (required)")
	cmd.Flags().IntVar(&chLimit, "limit", 100, "Maximum number of commits to scan")
	_ = cmd.MarkFlagRequired("issue")

	return cmd
}
