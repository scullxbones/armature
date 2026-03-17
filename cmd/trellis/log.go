package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/scullxbones/trellis/internal/audit"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	var issueID, workerID, sinceStr string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show the audit log of ops",
		RunE: func(cmd *cobra.Command, args []string) error {
			opsDir := appCtx.IssuesDir + "/ops"

			f := audit.Filter{
				IssueID:  issueID,
				WorkerID: workerID,
			}

			if sinceStr != "" {
				t, err := time.Parse(time.RFC3339, sinceStr)
				if err != nil {
					// Try date-only format
					t, err = time.Parse("2006-01-02", sinceStr)
					if err != nil {
						return fmt.Errorf("invalid --since format (use RFC3339 or YYYY-MM-DD): %w", err)
					}
				}
				f.Since = t
			}

			entries, err := audit.Load(opsDir, f)
			if err != nil {
				return fmt.Errorf("load audit log: %w", err)
			}

			if jsonOut {
				return printLogJSON(cmd, entries)
			}
			return printLogHuman(cmd, entries)
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "filter by issue ID")
	cmd.Flags().StringVar(&workerID, "worker", "", "filter by worker ID")
	cmd.Flags().StringVar(&sinceStr, "since", "", "filter entries since this time (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSONL")

	return cmd
}

func printLogHuman(cmd *cobra.Command, entries []audit.Entry) error {
	for _, e := range entries {
		ts := time.Unix(e.Timestamp, 0).UTC().Format("2006-01-02T15:04:05Z")
		lostRaceStr := ""
		if e.LostRace {
			lostRaceStr = " [lost race]"
		}
		summary := logPayloadSummary(e.Op)
		fmt.Fprintf(cmd.OutOrStdout(), "%s  %-20s  %-12s  %-10s  %s%s\n",
			ts, e.WorkerID, e.TargetID, e.Type, summary, lostRaceStr)
	}
	return nil
}

type logJSONEntry struct {
	Timestamp int64       `json:"timestamp"`
	WorkerID  string      `json:"worker_id"`
	TargetID  string      `json:"target_id"`
	Type      string      `json:"type"`
	Payload   ops.Payload `json:"payload"`
	LostRace  bool        `json:"_lost_race,omitempty"`
}

func printLogJSON(cmd *cobra.Command, entries []audit.Entry) error {
	for _, e := range entries {
		out := logJSONEntry{
			Timestamp: e.Timestamp,
			WorkerID:  e.WorkerID,
			TargetID:  e.TargetID,
			Type:      e.Type,
			Payload:   e.Payload,
			LostRace:  e.LostRace,
		}
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
	}
	return nil
}

// logPayloadSummary returns a short human-readable summary of an op's payload.
func logPayloadSummary(op ops.Op) string {
	switch op.Type {
	case ops.OpCreate:
		return fmt.Sprintf("%q (type=%s)", op.Payload.Title, op.Payload.NodeType)
	case ops.OpClaim:
		return fmt.Sprintf("ttl=%d", op.Payload.TTL)
	case ops.OpHeartbeat:
		return ""
	case ops.OpTransition:
		s := fmt.Sprintf("→ %s", op.Payload.To)
		if op.Payload.Outcome != "" {
			s += fmt.Sprintf(": %s", op.Payload.Outcome)
		}
		return s
	case ops.OpNote:
		return op.Payload.Msg
	case ops.OpLink:
		return fmt.Sprintf("%s %s", op.Payload.Rel, op.Payload.Dep)
	case ops.OpDecision:
		return fmt.Sprintf("%s → %s", op.Payload.Topic, op.Payload.Choice)
	case ops.OpAssign:
		if op.Payload.AssignedTo == "" {
			return "unassigned"
		}
		return fmt.Sprintf("→ %s", op.Payload.AssignedTo)
	default:
		return ""
	}
}
