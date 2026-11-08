package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/externalref"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newRefCmd() *cobra.Command {
	var role string
	var withdraw bool

	cmd := &cobra.Command{
		Use:   "ref ISSUE URL",
		Short: "Record or withdraw a GitHub issue URL on an Armature issue",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if withdraw && cmd.Flags().Changed("role") {
				return fmt.Errorf("--role and --withdraw cannot be combined")
			}
			issueID := args[0]
			rawURL := args[1]
			parsed, err := externalref.ParseGitHubIssueURL(rawURL)
			if err != nil {
				return err
			}
			canonical := parsed.URL()

			state := mustState(cmd)
			appCtx := state.ctx
			workerID, logPath, err := resolveWorkerAndLogCmd(cmd)
			if err != nil {
				return err
			}

			payload := ops.Payload{}
			if withdraw {
				payload.WithdrawExternalRef = canonical
			} else {
				payload.ExternalRef = canonical
				payload.ExternalRole = role
			}
			op := ops.Op{
				Type:      ops.OpExternalRef,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   payload,
			}

			wrote, err := appendHighStakesOpIfAfter(state, logPath, op, func() (bool, error) {
				live, _, replayErr := replayIssueOps(appCtx.IssuesDir, issueID)
				if replayErr != nil {
					return false, replayErr
				}
				if live == nil {
					return false, fmt.Errorf("issue not found: %s", issueID)
				}
				if hostRefCommandIsNoOp(live.HostRefs, canonical, role, withdraw) {
					return false, nil
				}
				return true, nil
			}, skipAfterIntegrate())
			if err != nil {
				return err
			}
			if !wrote {
				writeRefNoOp(cmd, issueID, canonical, role, withdraw)
				return nil
			}
			writeRefResult(cmd, issueID, canonical, role, withdraw)
			return nil
		},
	}

	cmd.Flags().StringVar(&role, "role", "", "role label for the host name (for example support or engineering)")
	cmd.Flags().BoolVar(&withdraw, "withdraw", false, "withdraw a live host name")
	return cmd
}

func hostRefCommandIsNoOp(refs []materialize.HostRef, url, role string, withdraw bool) bool {
	if withdraw {
		return !hostURLIsLive(refs, url)
	}
	cur, ok := materialize.CurrentHostRef(refs)
	return ok && cur.URL == url && cur.Role == role
}

func hostURLIsLive(refs []materialize.HostRef, url string) bool {
	live := map[string]struct{}{}
	for _, ref := range refs {
		if ref.Withdrawn {
			delete(live, ref.URL)
			continue
		}
		live[ref.URL] = struct{}{}
	}
	_, ok := live[url]
	return ok
}

func writeRefNoOp(cmd *cobra.Command, issueID, url, role string, withdraw bool) {
	payload := map[string]any{"issue": issueID, "url": url, "noop": true}
	if withdraw {
		payload["withdrawn"] = true
	} else if role != "" {
		payload["role"] = role
	}
	writeCommandResult(cmd, payload, "no-op: identical payload, nothing appended\n")
}

func writeRefResult(cmd *cobra.Command, issueID, url, role string, withdraw bool) {
	if withdraw {
		writeCommandResult(cmd, map[string]any{"issue": issueID, "url": url, "withdrawn": true},
			"Withdrew %s from %s\n", url, issueID)
		return
	}
	payload := map[string]any{"issue": issueID, "url": url}
	if role != "" {
		payload["role"] = role
	}
	if role == "" {
		writeCommandResult(cmd, payload, "Named %s on %s\n", url, issueID)
		return
	}
	writeCommandResult(cmd, payload, "Named %s on %s (role %s)\n", url, issueID, role)
}
