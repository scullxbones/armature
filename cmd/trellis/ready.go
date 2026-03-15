package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/scullxbones/trellis/internal/ready"
	"github.com/spf13/cobra"
)

func newReadyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Show tasks ready to be claimed",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir

			if _, err := materialize.Materialize(issuesDir, appCtx.Mode == "single-branch"); err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			index, err := materialize.LoadIndex(issuesDir + "/state/index.json")
			if err != nil {
				return err
			}

			issues := make(map[string]*materialize.Issue)
			for id := range index {
				issue, err := materialize.LoadIssue(fmt.Sprintf("%s/state/issues/%s.json", issuesDir, id))
				if err == nil {
					issues[id] = &issue
				}
			}

			entries := ready.ComputeReady(index, issues)

			format, _ := cmd.Flags().GetString("format")
			if format == "json" || format == "agent" {
				data, _ := json.MarshalIndent(entries, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				if len(entries) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No tasks ready.")
					return nil
				}
				for _, e := range entries {
					conf := ""
					if e.RequiresConfirmation {
						conf = " [requires confirmation]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s  (%s)%s\n", e.Issue, e.Title, e.Priority, conf)
				}
			}
			return nil
		},
	}

	return cmd
}
