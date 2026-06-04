package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/spf13/cobra"
)

// resolveModel implements three-level model resolution:
// CLI flag > task.PreferredModel > config default.
// The first non-empty value wins.
func resolveModel(flagModel, taskModel, configDefault string) string {
	if flagModel != "" {
		return flagModel
	}
	if taskModel != "" {
		return taskModel
	}
	return configDefault
}

func newOrchestrateCmdForService(service orchestrate.Runner) *cobra.Command {
	var (
		issueID string
		dryRun  bool
	)

	cmd := &cobra.Command{
		Use: "orchestrate",
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" {
				return fmt.Errorf("--issue is required")
			}
			state, err := service.Run(cmd.Context(), orchestrate.RunInput{
				TaskID: issueID,
				Opts:   orchestrate.RunOptions{DryRun: dryRun},
			})
			if err != nil {
				var runErr *orchestrate.RunError
				if errors.As(err, &runErr) {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"orchestrate timeout/failure summary: elapsed=%dms phase=%s harness=%s retries=%d next=%s\n",
						runErr.Diagnostics.ElapsedMs,
						runErr.Diagnostics.LastPhase,
						runErr.Diagnostics.Harness,
						runErr.Diagnostics.Retries,
						runErr.Diagnostics.NextStep,
					)
				}
				return err
			}
			data, _ := json.Marshal(map[string]any{
				"issue": issueID,
				"phase": state.Phase,
				"run":   state.Run,
			})
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to orchestrate (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "exit before dispatch — inspect state without running the agent")
	return cmd
}

func newOrchestrateCmd() *cobra.Command {
	var (
		issueID         string
		harness         string
		model           string
		retries         int
		timeout         int
		dryRun          bool
		showNetworkPlan bool
		authCheck       bool
	)

	cmd := &cobra.Command{
		Use:   "orchestrate",
		Short: "Run the automated orchestration loop for a task",
		Long: `orchestrate dispatches an AI harness agent to implement a task, verifies
the result via the pipeline, and commits the changes using the zero-trust commit sequence.

Three-level model resolution:
  1. --model flag (highest priority)
  2. task.preferred_model (set when the issue was created)
  3. config.orchestrator.default_model (repository default)`,
		Example: `  # Run orchestration for issue E7-S1-T1 with the claude harness
  $ arm orchestrate --issue E7-S1-T1

  # Use a specific model and harness
  $ arm orchestrate --issue E7-S1-T1 --harness codex --model gpt-4o

  # Dry run — inspect state without dispatching the agent
  $ arm orchestrate --issue E7-S1-T1 --dry-run

  # Allow up to 5 retries with a 300-second timeout
  $ arm orchestrate --issue E7-S1-T1 --retries 5 --timeout 300`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			if issueID == "" {
				return fmt.Errorf("--issue is required")
			}

			// --- Resolve worker ID ---
			workerID, _, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return err
			}

			orcCfg := appCtx.Config.Orchestrator
			authPlan, authErr := orchestrate.ResolveAuthPlan(harness, orchestrate.AuthConfig{
				Mode:    orcCfg.Auth.Mode,
				EnvFile: orcCfg.Auth.EnvFile,
			})
			if showNetworkPlan || authCheck {
				payloadClasses := []string{
					"issue metadata (id/title/acceptance/scope)",
					"rendered task context",
					"harness/model selection",
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"network plan: harness=%s provider=%s endpoint_hint=%q auth_source=%s auth_mode=%s payload=%q repo_mutation=%t\n",
					harness,
					authPlan.Provider,
					authPlan.EndpointHint,
					authPlan.Source,
					authPlan.Mode,
					payloadClasses,
					!dryRun,
				)
			}
			if authErr != nil {
				return fmt.Errorf("orchestrate preflight auth: %w", authErr)
			}
			if authCheck {
				format, _ := cmd.Root().PersistentFlags().GetString("format")
				if format == "json" || format == "agent" {
					data, _ := json.Marshal(map[string]any{
						"issue":         issueID,
						"harness":       harness,
						"provider":      authPlan.Provider,
						"endpoint_hint": authPlan.EndpointHint,
						"auth_source":   authPlan.Source,
						"auth_mode":     authPlan.Mode,
						"auth_ok":       true,
					})
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "auth check ok: harness=%s source=%s provider=%s\n", harness, authPlan.Source, authPlan.Provider)
				}
				return nil
			}
			runner := orchestrate.NewRepoRunner(appCtx, workerID)

			// --- Run ---
			var cancelFn context.CancelFunc
			runCtx := cmd.Context()
			if runCtx == nil {
				runCtx = context.Background()
			}
			if timeout > 0 {
				runCtx, cancelFn = context.WithTimeout(runCtx, time.Duration(timeout)*time.Second)
				defer cancelFn()
			}

			runResult, err := runner.Run(runCtx, orchestrate.RunRequest{
				TaskID:        issueID,
				WorkerID:      workerID,
				Harness:       harness,
				ModelOverride: model,
				RetryBudget:   retries,
				Timeout:       time.Duration(timeout) * time.Second,
				DryRun:        dryRun,
				Progress: func(ev orchestrate.ProgressEvent) {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "orchestrate progress: kind=%s phase=%s elapsed=%s harness=%s msg=%s\n",
						ev.Kind, ev.Phase, ev.Elapsed.Truncate(time.Second), ev.Harness, ev.Message)
				},
			})
			if err != nil {
				var runErr *orchestrate.RunError
				if errors.As(err, &runErr) {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"orchestrate timeout/failure summary: elapsed=%dms phase=%s harness=%s retries=%d next=%s\n",
						runErr.Diagnostics.ElapsedMs,
						runErr.Diagnostics.LastPhase,
						runErr.Diagnostics.Harness,
						runErr.Diagnostics.Retries,
						runErr.Diagnostics.NextStep,
					)
				}
				return fmt.Errorf("orchestrate %s: %w", issueID, err)
			}

			// --- Output ---
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				payload := buildOrchestateJSONPayload(issueID, dryRun, runResult)
				data, _ := json.Marshal(payload)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), formatOrchestrateHumanOutput(issueID, runResult))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to orchestrate (required)")
	cmd.Flags().StringVar(&harness, "harness", "claude", "harness adapter: claude, codex, or devin")
	cmd.Flags().StringVar(&model, "model", "", "model name (overrides task.preferred_model and config default)")
	cmd.Flags().IntVar(&retries, "retries", 3, "maximum number of retry attempts on verify failure")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "per-invocation timeout in seconds (0 = no limit)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "exit before dispatch — inspect state without running the agent")
	cmd.Flags().BoolVar(&showNetworkPlan, "show-network-plan", false, "print harness auth/network disclosure before dispatch")
	cmd.Flags().BoolVar(&authCheck, "auth-check", false, "run harness auth preflight and exit without dispatch")

	return cmd
}

// formatOrchestrateHumanOutput returns the human-readable one-line summary of an orchestrate run.
// When the run is blocked, blocked_reason is included in the output.
func formatOrchestrateHumanOutput(issueID string, result orchestrate.RunResult) string {
	if result.BlockedReason != "" {
		return fmt.Sprintf("orchestrate %s: phase=%s run=%d blocked_reason=%s\n", issueID, result.Phase, result.Run, result.BlockedReason)
	}
	return fmt.Sprintf("orchestrate %s: phase=%s run=%d\n", issueID, result.Phase, result.Run)
}

// buildOrchestateJSONPayload assembles the JSON payload for the orchestrate command output.
// It always includes the core fields (issue, phase, run, dry_run, model, harness, auth_source)
// and conditionally includes fields that carry meaningful values when set:
// blocked_reason, would_dispatch, scope_conflicts, would_claim, and claim_owner.
func buildOrchestateJSONPayload(issueID string, dryRun bool, result orchestrate.RunResult) map[string]any {
	payload := map[string]any{
		"issue":          issueID,
		"phase":          result.Phase,
		"run":            result.Run,
		"dry_run":        dryRun,
		"model":          result.Model,
		"harness":        result.Harness,
		"auth_source":    result.AuthSource,
		"would_dispatch": result.WouldDispatch,
	}
	if result.CompletionMessage != "" {
		payload["completion_message"] = result.CompletionMessage
	}
	if result.BlockedReason != "" {
		payload["blocked_reason"] = result.BlockedReason
	}
	if len(result.ScopeConflicts) > 0 {
		payload["scope_conflicts"] = result.ScopeConflicts
	}
	if result.WouldClaim {
		payload["would_claim"] = result.WouldClaim
	}
	if result.ClaimOwner != "" {
		payload["claim_owner"] = result.ClaimOwner
	}
	return payload
}

// fileOpLog is a thin adapter that bridges ops.AppendOp with orchestrate.OpLog.
type fileOpLog struct {
	ctx     *config.Context
	logPath string
}

func (f *fileOpLog) ReadAll() ([]ops.Op, error) {
	return ops.ReadLog(f.logPath)
}

func (f *fileOpLog) Append(op ops.Op) error {
	return appendOp(f.ctx, f.logPath, op)
}
