package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/scullxbones/trellis/internal/claim"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/spf13/cobra"
)

// WorkerStatus describes the current activity state of a worker.
type WorkerStatus struct {
	WorkerID    string `json:"worker_id"`
	Status      string `json:"status"`       // "active", "stale", or "idle"
	LastOpTime  int64  `json:"last_op_time"` // Unix epoch of most recent op
	ActiveIssue string `json:"active_issue,omitempty"`
}

func newWorkersCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "workers",
		Short: "Show worker activity status",
		RunE: func(cmd *cobra.Command, args []string) error {
			opsDir := filepath.Join(appCtx.IssuesDir, "ops")
			defaultTTL := appCtx.Config.DefaultTTL
			if defaultTTL <= 0 {
				defaultTTL = 60
			}
			now := time.Now().Unix()

			workers, err := enumerateWorkers(opsDir)
			if err != nil {
				return fmt.Errorf("enumerate workers: %w", err)
			}

			statuses := make([]WorkerStatus, 0, len(workers))
			for workerID, allOps := range workers {
				s := buildWorkerStatus(workerID, allOps, defaultTTL, now)
				statuses = append(statuses, s)
			}

			// Sort by worker ID for stable output
			sort.Slice(statuses, func(i, j int) bool {
				return statuses[i].WorkerID < statuses[j].WorkerID
			})

			if jsonOut {
				for _, s := range statuses {
					data, _ := json.Marshal(s)
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				}
				return nil
			}

			if len(statuses) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No workers found.")
				return nil
			}
			for _, s := range statuses {
				lastSeen := ""
				if s.LastOpTime > 0 {
					lastSeen = time.Unix(s.LastOpTime, 0).UTC().Format("2006-01-02T15:04:05Z")
				}
				active := ""
				if s.ActiveIssue != "" {
					active = fmt.Sprintf(" (working on %s)", s.ActiveIssue)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-40s  %-8s  %s%s\n",
					s.WorkerID, s.Status, lastSeen, active)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSONL")
	return cmd
}

// enumerateWorkers reads all *.log files from opsDir and returns a map of
// workerID -> their ops.
func enumerateWorkers(opsDir string) (map[string][]ops.Op, error) {
	logFiles, err := filepath.Glob(filepath.Join(opsDir, "*.log"))
	if err != nil {
		return nil, err
	}

	result := make(map[string][]ops.Op)
	for _, logPath := range logFiles {
		workerID := ops.WorkerIDFromFilename(logPath)
		logOps, err := ops.ReadLog(logPath)
		if err != nil {
			continue
		}
		result[workerID] = logOps
	}
	return result, nil
}

// buildWorkerStatus determines the status of a worker based on their ops:
//   - active: has a live (non-stale) claim
//   - stale: had claims but all are stale
//   - idle: last op was within 2*defaultTTL minutes window (no active claim)
func buildWorkerStatus(workerID string, allOps []ops.Op, defaultTTLMinutes int, now int64) WorkerStatus {
	lastOp := lastOpTimestampFromLog(allOps)

	// Find active claims: look for claims not yet overtaken by a transition to done/merged
	// Track claimed issues and their last state
	claimedAt := make(map[string]int64)
	lastHeartbeat := make(map[string]int64)
	claimTTL := make(map[string]int)
	transitioned := make(map[string]bool)

	for _, op := range allOps {
		switch op.Type {
		case ops.OpClaim:
			claimedAt[op.TargetID] = op.Timestamp
			claimTTL[op.TargetID] = op.Payload.TTL
		case ops.OpHeartbeat:
			if op.Timestamp > lastHeartbeat[op.TargetID] {
				lastHeartbeat[op.TargetID] = op.Timestamp
			}
		case ops.OpTransition:
			if op.Payload.To == ops.StatusDone || op.Payload.To == ops.StatusMerged ||
				op.Payload.To == ops.StatusCancelled {
				transitioned[op.TargetID] = true
			}
		}
	}

	// Check each claimed issue
	for issueID, ca := range claimedAt {
		if transitioned[issueID] {
			continue
		}
		ttl := claimTTL[issueID]
		if ttl <= 0 {
			ttl = defaultTTLMinutes
		}
		if !claim.IsClaimStale(ca, lastHeartbeat[issueID], ttl, now) {
			return WorkerStatus{
				WorkerID:    workerID,
				Status:      "active",
				LastOpTime:  lastOp,
				ActiveIssue: issueID,
			}
		}
	}

	// Check if any claim was made (all stale)
	if len(claimedAt) > 0 {
		return WorkerStatus{
			WorkerID:   workerID,
			Status:     "stale",
			LastOpTime: lastOp,
		}
	}

	// Idle: no claims, but had recent ops within 2*TTL window
	idleWindowSeconds := int64(2 * defaultTTLMinutes * 60)
	if lastOp > 0 && now-lastOp <= idleWindowSeconds {
		return WorkerStatus{
			WorkerID:   workerID,
			Status:     "idle",
			LastOpTime: lastOp,
		}
	}

	return WorkerStatus{
		WorkerID:   workerID,
		Status:     "idle",
		LastOpTime: lastOp,
	}
}

// lastOpTimestampFromLog returns the timestamp of the most recent op in the list.
func lastOpTimestampFromLog(allOps []ops.Op) int64 {
	var last int64
	for _, op := range allOps {
		if op.Timestamp > last {
			last = op.Timestamp
		}
	}
	return last
}
