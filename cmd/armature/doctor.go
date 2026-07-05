package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var strict bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run repo health checks (D1-D6)",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Fall through to root PersistentPreRunE for normal config loading.
			// This correctly sets the execution state in the command context.
			rootErr := cmd.Root().PersistentPreRunE(cmd, args)
			if rootErr == nil {
				return nil
			}

			// Root context resolution failed; check if this is a legacy
			// single-branch layout by looking for .armature/ops in the repo root.
			repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}

			absRepoPath, pathErr := filepath.Abs(repoPath)
			if pathErr != nil {
				return pathErr
			}

			legacyArmaturePath := filepath.Join(absRepoPath, ".armature")
			legacyOpsPath := filepath.Join(legacyArmaturePath, "ops")

			// Check if legacy layout exists
			info, statErr := os.Stat(legacyOpsPath)
			if statErr == nil && info.IsDir() {
				// Legacy layout detected: set up minimal execution state in the command context
				legacyCtx := &config.Context{
					RepoPath:  absRepoPath,
					IssuesDir: legacyArmaturePath,
					StateDir:  filepath.Join(legacyArmaturePath, "state"),
					// Note: WorktreePath is empty for legacy repos, which is expected
				}
				state := &executionState{ctx: legacyCtx}
				state.pusher, state.tracker = initPushDeps(legacyCtx)
				baseCtx := cmd.Context()
				if baseCtx == nil {
					baseCtx = context.Background()
				}
				cmd.SetContext(context.WithValue(baseCtx, executionStateKey{}, state))
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "legacy single-branch layout detected at %s; run `arm bootstrap` to migrate to dual-branch.\n", legacyOpsPath)
				return nil
			}

			// Neither modern nor legacy layout found; return the original context error
			return rootErr
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			issuesDir := appCtx.IssuesDir
			repoPath := appCtx.RepoPath

			report, err := doctor.Run(issuesDir, appCtx.StateDir, repoPath, verbose, time.Now())
			if err != nil {
				return err
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			if format == "json" {
				data, _ := json.MarshalIndent(report, "", "  ") //nolint:errcheck // report struct contains only serializable values
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
