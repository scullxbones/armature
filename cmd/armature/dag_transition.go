package main

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/validate"
	"github.com/spf13/cobra"
)

func newDAGTransitionCmd() *cobra.Command {
	var issueID string
	var to string

	cmd := &cobra.Command{
		Use:   "transition",
		Short: "Promote all draft nodes in a subtree to verified",
		Long: `Promote the confidence of nodes from draft to verified.

This command transitions nodes in a subtree from draft (unverified) confidence
to verified confidence, setting the dag_confirmed flag. Promotion to verified
(plan release) requires a strict-green arm validate of the whole graph so a
planner cannot release a dirty plan. Demotion to draft is not gated.`,
		Example: `  # Promote a subtree to verified
  $ arm dag transition --issue STORY-001

  # Transition back to draft (if needed)
  $ arm dag transition --issue STORY-001 --to draft`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			targetConfidence := to
			if targetConfidence == "" {
				targetConfidence = "verified"
			}
			if targetConfidence != "draft" && targetConfidence != "verified" {
				return fmt.Errorf("invalid --to confidence value %q: must be one of draft, verified", targetConfidence)
			}
			if targetConfidence == "verified" {
				if err := refusePlanRelease(cmd); err != nil {
					return err
				}
			}

			op := ops.Op{
				Type:      ops.OpDAGTransition,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					IssueID: issueID,
					To:      targetConfidence,
				},
			}
			if err := appendOp(ctx, logPath, op); err != nil {
				return err
			}

			result := map[string]string{"issue": issueID, "promoted_to": targetConfidence}
			data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "root issue ID of the subtree to promote")
	cmd.Flags().StringVar(&to, "to", "", "target confidence level: draft or verified (default: verified)")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}

func refusePlanRelease(cmd *cobra.Command) error {
	result, valErr := runGraphValidation(cmd, validate.Options{Strict: true})
	if valErr != nil {
		return valErr
	}
	if result.OK {
		return nil
	}
	if renderErr := output.RenderValidation(cmd.OutOrStdout(), result, false); renderErr != nil {
		return fmt.Errorf("render validation: %w", renderErr)
	}
	// The findings were just rendered to stdout, so the refusal has already
	// completed its wire protocol; appending a Command Failure would leave a
	// structured consumer with report text followed by JSON (ADR 0020 §7).
	return skipCommandFailure(fmt.Errorf(
		"cannot promote to verified: validation failed with %d error(s) and %d warning(s); "+
			"withdraw the draft (arm dag revert / arm transition --to cancelled)",
		len(result.Errors), len(result.Warnings)))
}
