package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/trellis/internal/context"
	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/spf13/cobra"
)

func newRenderContextCmd() *cobra.Command {
	var (
		rcIssue  string
		rcBudget int
		rcRaw    bool
	)

	cmd := &cobra.Command{
		Use:   "render-context",
		Short: "Render assembled context for an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			_, err := materialize.Materialize(issuesDir, appCtx.Mode == "single-branch")
			if err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			state, err := loadStateFromIssuesDir(issuesDir)
			if err != nil {
				return fmt.Errorf("load state: %w", err)
			}

			ctx, err := context.Assemble(rcIssue, issuesDir, state)
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
				fmt.Fprintln(cmd.OutOrStdout(), out)
			} else {
				fmt.Fprint(cmd.OutOrStdout(), context.RenderHuman(ctx))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&rcIssue, "issue", "", "Issue ID (required)")
	cmd.Flags().IntVar(&rcBudget, "budget", 4000, "Token budget")
	cmd.Flags().BoolVar(&rcRaw, "raw", false, "Skip truncation")
	cmd.MarkFlagRequired("issue")

	return cmd
}

// loadStateFromIssuesDir reads all materialized issue JSON files and builds a State.
func loadStateFromIssuesDir(issuesDir string) (*materialize.State, error) {
	stateIssuesDir := filepath.Join(issuesDir, "state", "issues")
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
