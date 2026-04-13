package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/trellis/internal/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var strict bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run repo health checks (D1-D6)",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			repoPath := appCtx.RepoPath

			report, err := doctor.Run(issuesDir, appCtx.StateDir, repoPath, verbose)
			if err != nil {
				return err
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			if format == "json" {
				data, _ := json.MarshalIndent(report, "", "  ")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				for _, f := range report.Checks {
					icon := "✓"
					switch f.Severity {
					case doctor.SeverityWarning:
						icon = "⚠"
					case doctor.SeverityError:
						icon = "✗"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", icon, f.Check, f.Message)
					items := f.Items
					if verbose && len(f.VerboseItems) > 0 {
						items = f.VerboseItems
					}
					for _, item := range items {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    - %s\n", item)
					}
				}
			}

			// Determine exit condition.
			if report.HasErrors() {
				return fmt.Errorf("doctor: %d error(s) found", countBySeverity(report, doctor.SeverityError))
			}
			if strict && report.HasWarnings() {
				return fmt.Errorf("doctor --strict: %d warning(s) promoted to errors", countBySeverity(report, doctor.SeverityWarning))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&strict, "strict", false, "promote warnings to errors")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "emit file path and line context for D3 violations; name uncited issue IDs for D6")
	return cmd
}

func countBySeverity(r doctor.Report, s doctor.Severity) int {
	n := 0
	for _, f := range r.Checks {
		if f.Severity == s {
			n++
		}
	}
	return n
}
