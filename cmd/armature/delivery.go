package main

import (
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/delivery"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newDeliveryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delivery",
		Short: "Record a delivery snapshot for an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newDeliveryRecordCmd())
	return cmd
}

func newDeliveryRecordCmd() *cobra.Command {
	var issueID, base, tip string

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record branch/base/tip for an issue and mark it done",
		Long: `Write refs/armature/deliveries/<issue-id> at --tip and record the same
snapshot as arm transition --to done --base --tip. Both objects must exist,
base must ancestor tip, and the range must be non-empty.`,
		Args: func(cmd *cobra.Command, args []string) error {
			return mapDeliveryError(cobra.NoArgs(cmd, args))
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() { err = mapDeliveryError(err) }()
			if issueID == "" {
				return fmt.Errorf(`required flag(s) "issue" not set`)
			}
			if base == "" || tip == "" {
				return fmt.Errorf(`required flag(s) "base" and "tip" not set`)
			}

			state := mustState(cmd)
			appCtx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return err
			}

			liveIssue, allOps, replayErr := replayIssueOps(appCtx.IssuesDir, issueID)
			if replayErr != nil {
				return replayErr
			}

			currentStatus := ""
			if liveIssue != nil {
				currentStatus = liveIssue.Status
			}
			if leaseStatusAllowsOwnerGate(currentStatus) {
				if err := requirePublishedOwner(appCtx, issueID, workerID); err != nil {
					return err
				}
			}

			payload := ops.Payload{
				To:   ops.StatusDone,
				Base: base,
				Tip:  tip,
			}
			if liveIssue != nil {
				payload.Outcome = liveIssue.Outcome
				payload.Branch = liveIssue.Branch
				payload.PR = liveIssue.PR
				if liveIssue.Status == ops.StatusDone {
					payload.IntegrationBranch = liveIssue.IntegrationBranch
				}
			}
			if payload.IntegrationBranch == "" {
				payload.IntegrationBranch = appCtx.Config.IntegrationBranchOrDefault()
			}

			if replayErr == nil && liveIssue != nil {
				if ops.IdenticalTransition(allOps, issueID, liveIssue.Status, liveIssue.Outcome, liveIssue.Branch, liveIssue.PR, payload) {
					writeCommandResult(cmd, map[string]any{"issue": issueID, "status": ops.StatusDone, "noop": true},
						"no-op: identical payload, nothing appended\n")
					return nil
				}
			}

			if err := writeDeliverySnapshot(appCtx.RepoPath, issueID, liveIssueType(liveIssue), &payload); err != nil {
				return err
			}

			op := ops.Op{
				Type: ops.OpTransition, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID,
				Payload:  payload,
			}
			wrote, err := appendHighStakesOpIfAfter(state, logPath, op, func() (bool, error) {
				live, all, replayErr := replayIssueOps(appCtx.IssuesDir, issueID)
				if replayErr != nil || live == nil {
					return true, nil
				}
				if ops.IdenticalTransition(all, issueID, live.Status, live.Outcome, live.Branch, live.PR, payload) {
					return false, nil
				}
				return true, nil
			}, func() error {
				if !leaseStatusAllowsOwnerGate(currentStatus) {
					return nil
				}
				return requirePublishedOwner(appCtx, issueID, workerID)
			})
			if err != nil {
				return err
			}
			if !wrote {
				writeCommandResult(cmd, map[string]any{"issue": issueID, "status": ops.StatusDone, "noop": true},
					"no-op: identical payload, nothing appended\n")
				return nil
			}
			writeCommandResult(cmd, map[string]string{"issue": issueID, "status": ops.StatusDone},
				"%s → %s\n", issueID, ops.StatusDone)
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&base, "base", "", "delivery base SHA")
	cmd.Flags().StringVar(&tip, "tip", "", "delivery tip SHA")
	return cmd
}

const codeDelivery1 = "DELIVERY-1"

func mapDeliveryError(err error) error {
	if mapped, done := mappedCommandFailure(err); done {
		return mapped
	}
	if delivery.IsRecordError(err) {
		return armerrors.Wrap(codeDelivery1, err.Error(), []string{"arm doctor", "arm show"}, err)
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "required flag"),
		strings.Contains(msg, "accepts"):
		return armerrors.Wrap(armerrors.CodeUSAGE, msg, []string{"arm delivery record --help"}, err)
	case strings.Contains(msg, "NOT-CLAIM-OWNER"):
		return wrapNotClaimOwner(codeDelivery1, err)
	case isLocalArmatureTipPublishError(err):
		return wrapOpsPublishFailure(codeDelivery1, err)
	default:
		return armerrors.Wrap(codeDelivery1, msg, []string{"arm doctor", "arm show"}, err)
	}
}
