package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/traceability"
	"github.com/scullxbones/armature/internal/validate"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var (
		ci     bool
		strict bool
		quiet  bool
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the issue graph for consistency",
		Args:  cobra.NoArgs,
		Long: `Check the issue graph for structural consistency and traceability coverage.

This command validates parent-child relationships, dependency links, field requirements,
and coverage metrics (% of issues cited in documentation). Validation is strict by default:
warnings are errors, a green run prints a single summary line, and any error exits
non-zero. The whole graph is validated; partial or scoped validation is not supported.
--ci is the CI alias for the same fail-closed contract (used by
make validate-graph / story integration, not make check).
Use --strict=false to keep warnings as warnings and print INFO lines.
Use --quiet to suppress INFO lines on a failing run.`,
		Example: `  # Validate the full issue graph (strict; silent when green)
  $ arm validate

  # Same fail-closed contract, for CI / make validate-graph
  $ arm validate --ci

  # Keep warnings as warnings (non-default)
  $ arm validate --strict=false

  # Suppress INFO lines on a failing run
  $ arm validate --quiet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ci && cmd.Flags().Changed("strict") && !strict {
				return fmt.Errorf("--ci and --strict=false are contradictory")
			}
			if ci {
				strict = true
			}
			opts := validate.Options{
				Strict: strict,
			}
			result, err := runGraphValidation(cmd, opts)
			if err != nil {
				return err
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" {
				payload := map[string]any{
					"errors":   result.Errors,
					"warnings": result.Warnings,
					"infos":    result.Infos,
				}
				if result.Coverage != nil {
					payload["coverage"] = result.Coverage
				}
				out, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			} else if strict && result.OK {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), validationSummary(result))
			} else if err := output.RenderValidation(cmd.OutOrStdout(), result, quiet); err != nil {
				return fmt.Errorf("render validation: %w", err)
			}

			if strict && !result.OK {
				return fmt.Errorf("validation failed with %d error(s) and %d warning(s)",
					len(result.Errors), len(result.Warnings))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&ci, "ci", false, "Exit non-zero if errors found (CI alias; contradicts --strict=false)")
	cmd.Flags().BoolVar(&strict, "strict", true, "Treat warnings as errors (default true)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress INFO lines on a failing run")

	// Add doc-examples as a subcommand
	cmd.AddCommand(newValidateDocExamplesCmd())

	return cmd
}

// runGraphValidation materializes state and runs validate.Validate with
// citations, coverage, and expanded scopes filled in. Callers set Options.Strict.
func runGraphValidation(cmd *cobra.Command, opts validate.Options) (validate.Result, error) {
	appCtx := currentCtx(cmd)
	store := newSnapshotStore(appCtx)
	snap, err := store.Load(cmd.Context())
	if err != nil {
		return validate.Result{}, fmt.Errorf("load snapshot: %w", err)
	}
	for _, w := range snap.Warnings {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}

	manifestData, err := adapters.ReadManifestFile(filepath.Join(appCtx.IssuesDir, "sources"))
	if err != nil {
		return validate.Result{}, fmt.Errorf("read manifest: %w", err)
	}

	coverageData, err := adapters.ReadCoverageFile(store.StatePath("traceability.json"))
	if err != nil {
		return validate.Result{}, fmt.Errorf("read coverage: %w", err)
	}
	var cov *traceability.Coverage
	if coverageData != nil {
		cov = &traceability.Coverage{}
		if err := json.Unmarshal(coverageData, cov); err != nil {
			return validate.Result{}, fmt.Errorf("parse coverage: %w", err)
		}
	}

	state := snap.State
	scopeGlobs := make(map[string][]string)
	for _, issue := range state.Issues {
		scopeGlobs[issue.ID] = issue.Scope
	}

	opts.ManifestData = manifestData
	opts.Coverage = cov
	opts.PreExpandedScopes = adapters.ExpandGlobs(scopeGlobs)
	return validate.Validate(state, materialize.GraphFromState(state), opts), nil
}

func validationSummary(result validate.Result) string {
	if line := output.CoverageLine(result); line != "" {
		return fmt.Sprintf("OK: no issues found (%s)", line)
	}
	return "OK: no issues found"
}
