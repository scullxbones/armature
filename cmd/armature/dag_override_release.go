package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/validate"
	"github.com/spf13/cobra"
)

var openControllingTTY = func() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

func confirmOverrideRelease(tty *os.File, issueID string) error {
	_, _ = fmt.Fprintf(tty, "Type the issue ID %q to confirm release override: ", issueID)
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if strings.TrimSpace(line) != issueID {
		return fmt.Errorf("typed id does not match %s", issueID)
	}
	return nil
}

func releaseOverridePayload(issueID, reason string, issue *materialize.Issue) ops.Payload {
	payload := ops.Payload{
		IssueID:             issueID,
		To:                  "verified",
		SkippedValidateGate: true,
		Rationale:           reason,
	}
	if hasRecordedDelivery(issue) {
		payload.Base = issue.Base
		payload.Tip = issue.Tip
	}
	return payload
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
			defer bestEffortClose(tty)

			if renderErr := output.RenderValidation(tty, result, false); renderErr != nil {
				return fmt.Errorf("render findings: %w", renderErr)
			}
			if err := confirmOverrideRelease(tty, issueID); err != nil {
				return err
			}

			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			issue, err := snapIssueForOverride(cmd, issueID)
			if err != nil {
				return err
			}
			op := ops.Op{
				Type:      ops.OpDAGTransition,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   releaseOverridePayload(issueID, reason, issue),
			}
			if err := appendOp(ctx, logPath, op); err != nil {
				return err
			}

			out := map[string]string{"issue": issueID, "promoted_to": "verified", "override": "recorded"}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(mustMarshal(out)))
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "recorded reason for the release override")
	return cmd
}

func hasRecordedDelivery(issue *materialize.Issue) bool {
	return issue != nil && issue.Status == ops.StatusDone && issue.Base != "" && issue.Tip != ""
}

func snapIssueForOverride(cmd *cobra.Command, issueID string) (*materialize.Issue, error) {
	appCtx := currentCtx(cmd)
	store := newSnapshotStore(appCtx)
	snap, err := store.Load(cmd.Context())
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	if snap == nil || snap.State == nil {
		return nil, fmt.Errorf("load snapshot: empty state")
	}
	issue, ok := snap.State.Issues[issueID]
	if !ok || issue == nil {
		return nil, fmt.Errorf("issue %s not found", issueID)
	}
	return issue, nil
}

func checkOverrideReleaseTarget(cmd *cobra.Command, issueID string) (validate.Result, error) {
	issue, err := snapIssueForOverride(cmd, issueID)
	if err != nil {
		return validate.Result{}, err
	}
	if hasRecordedDelivery(issue) {
		return validate.Result{}, nil
	}
	if issue.Provenance.Confidence == "verified" {
		return validate.Result{}, fmt.Errorf("issue %s is already verified", issueID)
	}
	result, valErr := runGraphValidation(cmd, validate.Options{Strict: true, Now: nowEpoch()})
	if valErr != nil {
		return validate.Result{}, valErr
	}
	if result.OK {
		return result, fmt.Errorf("issue %s has no blocking findings; override is unnecessary", issueID)
	}
	return result, nil
}
