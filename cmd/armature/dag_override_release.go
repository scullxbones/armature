package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/validate"
	"github.com/spf13/cobra"
)

// openControllingTTY opens the process controlling terminal. Tests replace
// this to simulate a missing TTY.
var openControllingTTY = func() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

func newDAGOverrideReleaseCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "override-release <issue-id>",
		Short: "Record a human Plan Release that skipped the validate gate",
		Long: `Record a Release Override for a draft subtree.

This is a human break-glass act, never a green release. It requires a
controlling terminal, an interactive type-the-id confirmation, and a
recorded reason. Agent verbs do not accept a skip flag.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(reason) == "" {
				return fmt.Errorf("--reason is required")
			}
			nonInteractive, err := cmd.Root().PersistentFlags().GetBool("non-interactive")
			if err != nil {
				return fmt.Errorf("non-interactive flag: %w", err)
			}

			issueID := args[0]
			state := mustState(cmd)
			ctx := state.ctx
			result, err := checkOverrideReleaseTarget(cmd, issueID)
			if err != nil {
				return err
			}
			if nonInteractive {
				return fmt.Errorf("release override requires a controlling terminal")
			}

			tty, err := openControllingTTY()
			if err != nil {
				return fmt.Errorf("release override requires a controlling terminal")
			}
			defer tty.Close() //nolint:errcheck

			if renderErr := output.RenderValidation(tty, result, false); renderErr != nil {
				return fmt.Errorf("render findings: %w", renderErr)
			}
			_, _ = fmt.Fprintf(tty, "Type the issue ID %q to confirm release override: ", issueID)
			line, err := bufio.NewReader(tty).ReadString('\n')
			if err != nil {
				return fmt.Errorf("read confirmation: %w", err)
			}
			if strings.TrimSpace(line) != issueID {
				return fmt.Errorf("typed id does not match %s", issueID)
			}

			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			op := ops.Op{
				Type:      ops.OpDAGTransition,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload: ops.Payload{
					IssueID:             issueID,
					To:                  "verified",
					SkippedValidateGate: true,
					Rationale:           reason,
				},
			}
			if err := appendOp(ctx, logPath, op); err != nil {
				return err
			}

			out := map[string]string{"issue": issueID, "promoted_to": "verified", "override": "recorded"}
			data, _ := json.Marshal(out) //nolint:errcheck // result struct contains only serializable values
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "recorded reason for the release override")
	return cmd
}

func checkOverrideReleaseTarget(cmd *cobra.Command, issueID string) (validate.Result, error) {
	appCtx := currentCtx(cmd)
	store := newSnapshotStore(appCtx)
	snap, err := store.Load(cmd.Context())
	if err != nil {
		return validate.Result{}, fmt.Errorf("load snapshot: %w", err)
	}
	issue, ok := snap.State.Issues[issueID]
	if !ok {
		return validate.Result{}, fmt.Errorf("issue %s not found", issueID)
	}
	if issue.Provenance.Confidence == "verified" {
		return validate.Result{}, fmt.Errorf("issue %s is already verified", issueID)
	}
	result, valErr := runGraphValidation(cmd, validate.Options{Strict: true})
	if valErr != nil {
		return validate.Result{}, valErr
	}
	if result.OK {
		return result, fmt.Errorf("issue %s has no blocking findings; override is unnecessary", issueID)
	}
	return result, nil
}
