package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newHeartbeatCmd() *cobra.Command {
	var issueID string

	cmd := &cobra.Command{
		Use:   "heartbeat [issue-id]",
		Short: "Send heartbeat for an active claim",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if issueID == "" && len(args) > 0 {
				issueID = args[0]
			}
			if issueID == "" {
				return fmt.Errorf("issue ID is required (via --issue flag or positional argument)")
			}

			state := mustState(cmd)
			ctx := state.ctx
			workerID, logPath, err := resolveWorkerAndLog(ctx)
			if err != nil {
				return err
			}
			op := ops.Op{Type: ops.OpHeartbeat, TargetID: issueID, Timestamp: nowEpoch(),
				WorkerID: workerID}
			if err := appendLowStakesOp(state, logPath, op); err != nil {
				return err
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if format == "json" || format == "agent" {
				result := map[string]string{"issue": issueID, "heartbeat": "sent"}
				data, _ := json.Marshal(result) //nolint:errcheck // result struct contains only serializable values
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Heartbeat recorded for %s\n", issueID)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	return cmd
}

// heartbeatRateLimitState holds the last heartbeat time for debouncing.
type heartbeatRateLimitState struct {
	LastHeartbeatTime int64 `json:"last_heartbeat_time_unix"`
}

// readHeartbeatRateLimitState reads the rate-limit state from the OS temp directory
// for the given worker+issue combination. Returns a zero time if the state file
// doesn't exist or cannot be read.
func readHeartbeatRateLimitState(workerID, issueID string) time.Time {
	stateFile := rateLimitStateFilePath(workerID, issueID)
	data, err := os.ReadFile(stateFile)
	if err != nil {
		// File doesn't exist or can't be read; return zero time (no prior heartbeat)
		return time.Time{}
	}
	var state heartbeatRateLimitState
	if err := json.Unmarshal(data, &state); err != nil {
		// Malformed state; return zero time (assume no prior heartbeat)
		return time.Time{}
	}
	return time.Unix(state.LastHeartbeatTime, 0)
}

// writeHeartbeatRateLimitState writes the rate-limit state to the OS temp directory
// for the given worker+issue combination.
func writeHeartbeatRateLimitState(workerID, issueID string, heartbeatTime time.Time) error {
	stateFile := rateLimitStateFilePath(workerID, issueID)
	state := heartbeatRateLimitState{
		LastHeartbeatTime: heartbeatTime.Unix(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat state: %w", err)
	}
	// #nosec G304 - stateFile is derived from workerID and issueID, controlled by us
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		return fmt.Errorf("failed to write heartbeat state file: %w", err)
	}
	return nil
}

// rateLimitStateFilePath returns the path to the rate-limit state file in the OS
// temp directory for the given worker+issue combination.
func rateLimitStateFilePath(workerID, issueID string) string {
	// Create a unique filename based on worker ID and issue ID to avoid collisions
	filename := fmt.Sprintf("armature-heartbeat-%s-%s.json", workerID, issueID)
	return filepath.Join(os.TempDir(), filename)
}
